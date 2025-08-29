# Shibuya Developer Guide

## Quick Start

### Prerequisites
- Docker
- Kind (Kubernetes in Docker)
- kubectl
- Helm
- Go 1.19+
- Node.js (for UI development)

### Local Development Setup

1. **Clone and Start Local Cluster**
   ```bash
   git clone https://github.com/hveda/Setagaya.git
   cd Setagaya
   make  # This will set up the entire local environment
   ```

2. **Expose Services**
   ```bash
   make expose  # Exposes Shibuya on :8080 and Grafana on :3000
   ```

3. **Access the Application**
   - Shibuya UI: http://localhost:8080
   - Grafana: http://localhost:3000
   - Default username for projects: `shibuya` (when auth is disabled)

### Development Workflow

#### Building Shibuya Controller
```bash
cd shibuya
go build .  # Build the binary
make shibuya  # Build and deploy to local cluster
```

#### Running Tests
Tests require database configuration. For unit tests that don't need DB:
```bash
cd shibuya
go test ./utils -v
go test ./scheduler -v
```

For model tests (require DB setup):
```bash
# Set up test database first
cp config_tmpl.json config_env.json
# Edit config_env.json with test DB settings
go test ./model -v
```

#### Working with Frontend
```bash
cd shibuya/ui/static
# Frontend uses Vue.js 2 with plain JavaScript
# Edit files in js/ directory
# Changes are picked up on next page reload
```

### Project Structure

```
shibuya/
├── main.go              # Application entry point
├── api/                 # REST API handlers
├── controller/          # Core orchestration logic
├── model/              # Database entities and operations
├── scheduler/          # Kubernetes/Cloud Run management
├── ui/                 # Web interface (Vue.js)
├── engines/jmeter/     # JMeter engine agent
├── auth/               # Authentication logic
├── config/             # Configuration management
├── db/                 # Database schema and migrations
└── object_storage/     # File storage abstraction
```

### Configuration

Configuration is loaded from `config_env.json`. Copy from `config_tmpl.json` and modify:

```json
{
  "auth_config": {
    "no_auth": true,  // Disable auth for local dev
    "admin_users": ["your-username"]
  },
  "db": {
    "host": "localhost",
    "user": "root", 
    "password": "password",
    "database": "shibuya_test"
  },
  "executors": {
    "namespace": "shibuya-executors",
    "max_engines_in_collection": 5
  }
}
```

### Common Development Tasks

#### Adding a New API Endpoint
1. Add route in `api/main.go` `InitRoutes()`
2. Implement handler function following existing patterns
3. Add authentication if needed: `r.HandlerFunc = s.authRequired(r.HandlerFunc)`

#### Adding a New Model
1. Define struct in `model/`
2. Implement CRUD operations with prepared statements
3. Add database migration in `db/`
4. Create tests in `*_test.go`

#### Modifying UI
1. Edit Vue.js components in `ui/static/js/`
2. Update templates in `ui/templates/`
3. Test with live reload during development

#### Adding New Engine Type
1. Create new directory under `engines/`
2. Implement agent interface compatible with controller
3. Update scheduler to support new engine type
4. Add Docker image configuration

### Database Schema

Key tables:
- `project` - Top-level organization unit
- `collection` - Group of test plans
- `plan` - Individual test configuration
- `execution_plan` - Runtime configuration for plans
- `run_history` - Test execution records

### API Examples

```bash
# Create project
curl -X POST http://localhost:8080/api/projects \
  -d "name=test-project&owner=shibuya"

# Create plan
curl -X POST http://localhost:8080/api/plans \
  -d "name=test-plan&project_id=1"

# Upload JMX file
curl -X PUT http://localhost:8080/api/plans/1/files \
  -F "planFile=@test.jmx"

# Create collection
curl -X POST http://localhost:8080/api/collections \
  -d "name=test-collection&project_id=1"

# Deploy collection
curl -X POST http://localhost:8080/api/collections/1/deploy

# Trigger load test
curl -X POST http://localhost:8080/api/collections/1/trigger
```

### Debugging

#### Controller Logs
```bash
kubectl logs -f deployment/shibuya-controller -n shibuya-executors
```

#### Engine Logs
```bash
kubectl logs -f -l app=jmeter-engine -n shibuya-executors
```

#### Database Access
```bash
kubectl exec -it deployment/db -n shibuya-executors -- mysql -u root -p shibuya
```

#### Check Prometheus Metrics
```bash
curl http://localhost:8080/metrics
```

### Testing

#### Unit Tests
```bash
go test ./... -short  # Skip integration tests
```

#### Integration Tests
```bash
# Requires running cluster
make test
```

#### Load Testing the Platform
```bash
# Create multiple collections and trigger simultaneously
# Monitor resource usage in Grafana
```

### Troubleshooting

#### Common Issues

1. **Engines not starting**
   - Check namespace permissions
   - Verify image pull secrets
   - Check resource limits

2. **Database connection errors**
   - Verify DB credentials in config
   - Check DB pod status
   - Test connection manually

3. **File upload failures**
   - Check object storage configuration
   - Verify storage pod status
   - Check file permissions

4. **UI not loading**
   - Check static file serving
   - Verify proxy configuration
   - Check browser console for errors

### Performance Considerations

- Controller can handle ~100 concurrent engines
- Database connection pooling is configured automatically
- Object storage supports concurrent uploads
- Metrics collection scales with engine count

### Contributing

1. Follow Go conventions and gofmt
2. Add tests for new functionality
3. Update documentation for API changes
4. Test locally before submitting PR
5. Check existing issues and PRs first

### Environment Variables

Key environment variables for configuration:
- `SHIBUYA_CONFIG_PATH` - Path to config file
- `KUBECONFIG` - Kubernetes configuration
- `GOOGLE_APPLICATION_CREDENTIALS` - GCP credentials

For local development, these are usually set automatically by the Makefile.