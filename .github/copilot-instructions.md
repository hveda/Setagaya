# Shibuya Load Testing Platform - GitHub Copilot Instructions

## Project Overview

Shibuya is a distributed load testing platform that orchestrates JMeter engines in Kubernetes clusters. It provides a web UI, REST API, and real-time monitoring for scalable performance testing.

## Local Development Setup

### Quick Start
```bash
# Start local Kubernetes cluster with all dependencies
make

# Expose services for development
make expose  # Shibuya on :8080, Grafana on :3000

# Build and deploy changes
make shibuya

# Clean up environment
make clean
```

### Configuration
- Copy `config_tmpl.json` to `config_env.json`
- Set `"no_auth": true` for local development
- Use `shibuya` as project owner when auth is disabled

### File Structure

- **Language**: Go (backend), JavaScript/Vue.js (frontend)
- **Database**: MySQL/MariaDB for metadata
- **Storage**: Nexus/GCP Bucket for test files
- **Orchestration**: Kubernetes for engine deployment
- **Monitoring**: Prometheus + Grafana for metrics

## Core Components

### 1. Main Application (`shibuya/main.go`)
Entry point that initializes API server and UI routes, serves static files, and exposes Prometheus metrics.

### 2. API Layer (`shibuya/api/`)
- REST endpoints for CRUD operations
- Authentication middleware (LDAP integration)
- Error handling and validation
- File upload/download handling

### 3. Controller Layer (`shibuya/controller/`)
- Central orchestration logic
- Engine lifecycle management
- Metrics collection and streaming
- Background task management

### 4. Models (`shibuya/model/`)
- Database entities: Project, Collection, Plan, ExecutionPlan
- CRUD operations with MySQL
- File storage integration

### 5. Scheduler (`shibuya/scheduler/`)
- Kubernetes cluster management
- Pod deployment and monitoring
- Resource cleanup

### 6. UI (`shibuya/ui/`)
- Vue.js components for project management
- Real-time dashboards using SSE
- File upload interfaces

### 7. Engines (`shibuya/engines/jmeter/`)
- JMeter wrapper agent
- Metrics streaming to controller
- Test execution management

## Key Patterns & Conventions

### Error Handling
```go
// Use custom error types for different scenarios
type DBError struct {
    Message string
}

func (e *DBError) Error() string {
    return e.Message
}

// Handle errors consistently in API layer
func (s *ShibuyaAPI) handleErrors(w http.ResponseWriter, err error) {
    switch {
    case errors.As(err, &dbe):
        s.makeFailMessage(w, dbe.Error(), http.StatusNotFound)
    case errors.Is(err, noPermissionErr):
        s.makeFailMessage(w, err.Error(), http.StatusForbidden)
    default:
        s.makeFailMessage(w, err.Error(), http.StatusInternalServerError)
    }
}
```

### Database Operations
```go
// Use prepared statements and proper error handling
func GetProject(ID int64) (*Project, error) {
    db := config.SC.DBC
    q, err := db.Prepare("select id, name, owner from project where id=?")
    if err != nil {
        return nil, err
    }
    defer q.Close()
    
    project := new(Project)
    err = q.QueryRow(ID).Scan(&project.ID, &project.Name, &project.Owner)
    if err != nil {
        return nil, NewDBError(err.Error())
    }
    return project, nil
}
```

### Controller Patterns
```go
// Use goroutines for concurrent operations
func (c *Controller) readConnectedEngines() {
    for engine := range c.readingEngines {
        go func(engine shibuyaEngine) {
            ch := engine.readMetrics()
            for metric := range ch {
                // Process metrics and update Prometheus
                c.ApiMetricStreamBus <- &ApiMetricStreamEvent{
                    CollectionID: metric.collectionID,
                    PlanID:       metric.planID,
                    Raw:          metric.raw,
                }
            }
        }(engine)
    }
}
```

### API Routing
```go
// Define routes with proper HTTP methods and handlers
routes := Routes{
    &Route{"create_project", "POST", "/api/projects", s.projectCreateHandler},
    &Route{"get_project", "GET", "/api/projects/:project_id", s.projectGetHandler},
    &Route{"delete_project", "DELETE", "/api/projects/:project_id", s.projectDeleteHandler},
}

// Apply authentication middleware
for _, r := range routes {
    r.HandlerFunc = s.authRequired(r.HandlerFunc)
}
```

### Vue.js Components
```javascript
// Follow Vue.js patterns for component definition
var Project = Vue.component("project", {
    mixins: [DelimitorMixin, CollectionAttrs, PlanAttrs],
    template: "#project-tmpl",
    props: ["project"],
    data: function () {
        return {
            creating_collection: false,
            creating_plan: false
        }
    },
    methods: {
        newCollection: function () {
            this.creating_collection = true;
        },
        remove: function (e) {
            e.preventDefault();
            var r = confirm("You are going to delete the project " + this.project.name + ". Continue?");
            if (!r) return;
            // API call logic
        }
    }
});
```

## Development Guidelines

### When Working on API Endpoints
1. Always validate input parameters
2. Check user permissions using ownership functions
3. Handle database errors appropriately
4. Return proper HTTP status codes
5. Use JSON responses consistently

### When Working on Controllers
1. Use sync.Map for thread-safe operations
2. Implement proper goroutine cleanup
3. Handle engine disconnections gracefully
4. Stream metrics efficiently
5. Implement retry logic for external operations

### When Working on Models
1. Use prepared statements for SQL operations
2. Implement proper validation
3. Handle file operations with object storage
4. Use database transactions for multi-table operations
5. Implement soft deletes where appropriate

### When Working on UI Components
1. Use Vue.js mixins for common functionality
2. Implement proper error handling for API calls
3. Use SSE for real-time updates
4. Follow existing naming conventions
5. Implement loading states for async operations

## Configuration Management

Configuration is handled through JSON files and environment variables:

```go
// Access configuration through global config object
config.SC.ExecutorConfig.MaxEnginesInCollection
config.SC.DistributedMode
config.SC.DevMode
```

## Testing Patterns

### Unit Tests
Tests use the `testify/assert` framework. Model tests require database configuration.

```go
func TestCreateAndGetProject(t *testing.T) {
    name := "testproject"
    projectID, err := CreateProject(name, "tech-rwasp", "1111")
    if err != nil {
        t.Fatal(err)
    }
    
    p, err := GetProject(projectID)
    if err != nil {
        t.Fatal(err)
    }
    
    assert.Equal(t, name, p.Name)
    
    // Cleanup
    p.Delete()
    
    // Verify deletion
    p, err = GetProject(projectID)
    assert.NotNil(t, err)
    assert.Nil(t, p)
}
```

### Running Tests
```bash
# Tests requiring database (need config_env.json)
go test ./model -v

# Unit tests without external dependencies
go test ./utils -v
go test ./scheduler -v

# All tests (requires full environment)
make test
```

### Test Configuration
Create `config_env.json` from `config_tmpl.json` for tests:
```json
{
  "auth_config": {"no_auth": true},
  "db": {
    "host": "localhost",
    "user": "root",
    "password": "password", 
    "database": "shibuya_test"
  }
}
```

## Security Considerations

1. **Authentication**: All API endpoints require LDAP authentication
2. **Authorization**: Project ownership checks for resource access
3. **Input Validation**: Validate all user inputs
4. **File Uploads**: Restrict file types and sizes
5. **SQL Injection**: Use prepared statements only

## Performance Guidelines

1. **Database**: Use connection pooling and prepared statements
2. **Goroutines**: Limit concurrent operations to prevent resource exhaustion
3. **Memory**: Clean up resources in defer statements
4. **File Operations**: Stream large files instead of loading into memory
5. **Metrics**: Use efficient data structures for high-frequency operations

## Debugging Tips

1. **Logging**: Use structured logging with logrus
2. **Metrics**: Check Prometheus endpoints for system health
3. **Database**: Enable SQL logging in development
4. **Kubernetes**: Use kubectl to inspect engine pods
5. **Networking**: Monitor SSE connections for UI updates

## Common Patterns to Follow

1. **Error Wrapping**: Wrap errors with context information
2. **Resource Cleanup**: Always use defer for cleanup operations
3. **Null Handling**: Use github.com/guregu/null for nullable database fields
4. **Context**: Pass context through long-running operations
5. **Graceful Shutdown**: Implement proper shutdown handlers

This project emphasizes reliability, scalability, and ease of use for distributed load testing scenarios.