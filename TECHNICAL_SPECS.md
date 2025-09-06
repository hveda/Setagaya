# Setagaya Load Testing Platform - Technical Specifications

## Project Overview

**Setagaya** is a distributed load testing platform that orchestrates JMeter engines across Kubernetes clusters. The system follows a controller-scheduler-engine pattern designed for scalable, enterprise-grade load testing.

- **Version:** 2.0.0-rc
- **Language:** Go 1.25.1
- **Runtime:** Kubernetes-native with Docker/Podman support
- **License:** See [LICENSE](LICENSE) file
- **Repository:** https://github.com/hveda/Setagaya

## Architecture Overview

### Core Components

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Web UI    │◄──►│ API Server  │◄──►│ Controller  │
└─────────────┘    └─────────────┘    └─────────────┘
                           │                   │
                           ▼                   ▼
                   ┌─────────────┐    ┌─────────────┐
                   │  Scheduler  │◄──►│   Engines   │
                   └─────────────┘    └─────────────┘
                           │                   │
                           ▼                   ▼
                   ┌─────────────┐    ┌─────────────┐
                   │ Kubernetes  │    │   JMeter    │
                   └─────────────┘    └─────────────┘
```

### Component Details

#### 1. **Controller** (`setagaya/controller/`)
- **Purpose:** Main orchestration service managing test execution lifecycle
- **Binary:** `setagaya-controller`
- **Entry Point:** `./controller/cmd/main.go`
- **Key Responsibilities:**
  - Test lifecycle management (Deploy → Trigger → Terminate → Purge)
  - Real-time metrics aggregation from engines
  - Collection and plan coordination
  - Prometheus metrics exposition

#### 2. **API Server** (`setagaya/api/`)
- **Purpose:** REST API for web UI and external integrations
- **Binary:** `setagaya` (main binary)
- **Entry Point:** `./main.go`
- **Key Features:**
  - RESTful endpoints for CRUD operations
  - Server-sent events for live dashboard updates
  - LDAP authentication and authorization
  - File upload handling for test plans

#### 3. **Scheduler** (`setagaya/scheduler/`)
- **Purpose:** Kubernetes resource management
- **Key Features:**
  - Pod, service, and ingress lifecycle management
  - Node affinity and resource constraints
  - Deployment garbage collection (15-minute intervals)
  - Multi-cloud Kubernetes support

#### 4. **Engines** (`setagaya/engines/`)
- **Purpose:** Load generation executors
- **Supported Types:**
  - **JMeter Engine** (`jmeter/`): Apache JMeter with agent sidecar
  - **Agent Binary:** `setagaya-agent`
  - **Version Support:** JMeter 3.3 (legacy) and 5.6.3 (modern)

## Domain Model

### Hierarchy Structure
```
Project → Collection → Plan → ExecutionPlan
```

#### **Project**
- Top-level organizational unit
- Owner-based access control via LDAP groups
- Contains multiple collections

#### **Collection**
- Execution unit containing multiple plans running simultaneously
- Results converge at collection level for unified reporting
- States: Deploy → Trigger → Terminate → Purge

#### **Plan**
- Individual test configuration (JMX file)
- Defines test scenarios and parameters
- Can be executed across multiple engines

#### **ExecutionPlan**
- Specifies engines and concurrency per plan
- Maps plans to specific engine configurations
- Controls resource allocation and scaling

## Container Infrastructure

### Docker Images

| Component | Dockerfile | Base Image | Purpose |
|-----------|------------|------------|---------|
| API Server | `Dockerfile` | `alpine:3.20` | Main API and UI server |
| API Server (Alt) | `Dockerfile.api` | `alpine:3.20` | Dedicated API build |
| Controller | `Dockerfile.controller` | `alpine:3.20` | Controller daemon |
| JMeter Engine (Modern) | `Dockerfile.engines.jmeter` | `eclipse-temurin:21-jre-alpine` | JMeter 5.6.3 + source build |
| JMeter Engine (Legacy) | `Dockerfile.engines.jmeter.legacy` | `eclipse-temurin:21-jre-alpine` | JMeter 3.3 + pre-built binary |
| Local Storage | `local_storage/Dockerfile` | `scratch` | Storage service |
| Ingress Controller | `ingress-controller/Dockerfile` | `scratch` | Ingress management |
| Grafana | `grafana/Dockerfile` | `grafana/grafana:latest` | Metrics visualization |

### Security Features
- **Multi-stage builds:** Separate build and runtime environments
- **Non-root users:** All containers run as `setagaya` user (UID 1001)
- **Static compilation:** CGO_ENABLED=0 with security flags
- **Minimal base images:** Alpine and scratch for reduced attack surface
- **No HEALTHCHECK:** Eliminates OCI format warnings, relies on Kubernetes health monitoring

## Technology Stack

### Core Technologies
- **Language:** Go 1.25.1 (latest stable)
- **Container Runtime:** Docker/Podman compatible
- **Orchestration:** Kubernetes (any CNCF-compliant distribution)
- **Build System:** Multi-stage Docker builds
- **Package Management:** Go modules

### Dependencies
- **Web Framework:** Native Go HTTP server
- **Metrics:** Prometheus client
- **Database:** MySQL (configurable)
- **Authentication:** LDAP integration
- **Storage:** Pluggable (GCP Buckets, Nexus, Local)
- **Load Testing:** Apache JMeter 3.3/5.6.3

### Monitoring Stack
- **Metrics Collection:** Prometheus
- **Visualization:** Grafana
- **Dashboards:** Pre-configured for Setagaya metrics
- **Real-time Updates:** Server-sent events

## Configuration System

### Configuration Structure (`setagaya/config/`)
- **File:** `config.json` (runtime configuration)
- **Template:** `config_tmpl.json` (example configuration)
- **Environment:** `env=local` for development mode
- **Validation:** Automatic validation and defaults in `init()`
- **Documentation Links:** Configurable URLs for project documentation and help guides

### Documentation Integration
The platform includes configurable documentation links that appear in the UI:
- **`project_home`**: Link to project documentation or wiki
- **`upload_file_help`**: Link to file upload instructions
- **Default Values**: Generic placeholder URLs that can be customized per deployment

Example customization in Helm values:
```yaml
runtime:
  project_home: "https://your-org.com/setagaya/docs"
  upload_file_help: "https://your-org.com/setagaya/upload-guide"
```

### Key Configuration Areas
```json
{
  "project_home": "https://docs.example.com/setagaya/project-home",
  "upload_file_help": "https://docs.example.com/setagaya/file-upload-guide",
  "executor_config": {
    "jmeter": {
      "image": "setagaya:jmeter",
      "cpu": "1000m",
      "memory": "2Gi"
    }
  },
  "storage": {
    "type": "local|gcp|nexus",
    "config": { /* storage-specific */ }
  },
  "auth": {
    "no_auth": false,
    "ldap_config": { /* LDAP settings */ }
  }
}
```

## Storage System

### Object Storage Interface (`setagaya/object_storage/`)
Abstracted storage system supporting multiple backends:

#### Supported Backends
- **Local Storage:** File system storage for development
- **GCP Buckets:** Google Cloud Storage for production
- **Nexus:** Artifact repository integration

#### Storage Operations
- **Test Plans:** Upload JMX files
- **Collections:** Upload YAML execution configurations
- **Results:** Store test output and metrics

#### Usage Pattern
```go
storage := object_storage.Client.Storage
storage.Upload(bucket, key, data)
data := storage.Download(bucket, key)
```

## Authentication & Authorization

### LDAP Integration (`setagaya/auth/`)
- **User Authentication:** LDAP directory services
- **Group Membership:** Project ownership via LDAP groups
- **Admin Users:** Bypass ownership checks
- **Development Mode:** `no_auth: true` for local testing

### Authorization Model
- **Project Ownership:** Based on LDAP group membership
- **Admin Override:** Admin users have full access
- **API Validation:** All endpoints validate ownership
- **Resource Isolation:** Users can only access owned resources

## Kubernetes Integration

### Resource Management
- **Namespaced Deployments:** Per collection/plan combination
- **Service Exposure:** Ingress controllers for engine metrics
- **Resource Constraints:** CPU/memory limits and requests
- **Node Affinity:** Configurable placement policies

### Deployment Lifecycle
1. **Deploy:** Create K8s resources, engines come online
2. **Trigger:** Start load generation across all engines
3. **Terminate:** Stop tests, keep engines for result collection
4. **Purge:** Remove all K8s resources and clean up storage

### RBAC Configuration
Located in `kubernetes/` directory:
- `clusterrole.yaml` - Cluster-level permissions
- `roles.yaml` - Namespace-specific roles
- `serviceaccount.yaml` - Service account definitions

## Development Workflow

### Local Development Setup
```bash
make              # Creates kind cluster, deploys all components
make expose       # Port-forwards Setagaya (8080) and Grafana (3000)
make setagaya     # Rebuilds and redeploys controller changes
make clean        # Destroys local cluster
```

### Build Process
```bash
# Multi-stage Docker builds (recommended)
docker build -f setagaya/Dockerfile .

# Legacy pre-built binary approach
./setagaya/build.sh api        # API server binary
./setagaya/build.sh controller # Controller daemon binary  
./setagaya/build.sh jmeter     # JMeter agent sidecar binary
```

### Testing
- **Unit Tests:** `*_test.go` files in model packages
- **Integration Tests:** Full deployment testing via kind
- **Database Tests:** Using `test_utils.go` utilities
- **Container Tests:** Multi-platform builds

## JMeter Engine Compatibility

### Version Support Matrix

| JMeter Version | Dockerfile | Build Method | Status |
|----------------|------------|--------------|--------|
| **5.6.3** | `Dockerfile.engines.jmeter` | Source build | ✅ Modern (Recommended) |
| **3.3** | `Dockerfile.engines.jmeter.legacy` | Pre-built binary | ✅ Legacy Support |
| **Future versions** | Custom build with version ARG | Source build | ✅ Compatible |

### Agent Architecture
- **Binary:** `setagaya-agent` (version-agnostic)
- **Path Detection:** Automatic via `JMETER_BIN` environment variable
- **Fallback:** Hardcoded JMeter 3.3 paths for backward compatibility
- **Executables:** `jmeter` and `stoptest.sh` discovery

### Environment Variables
```dockerfile
ENV JMETER_HOME=/opt/apache-jmeter-${JMETER_VERSION}
ENV JMETER_BIN=${JMETER_HOME}/bin
ENV PATH=${JMETER_BIN}:${PATH}
```

## Metrics and Monitoring

### Metrics Pipeline
```
Engine → Controller → API → WebUI/Grafana
```

### Real-time Metrics Flow
1. **Engines** stream metrics via HTTP to controller endpoints
2. **Controller** aggregates and forwards to Prometheus
3. **API** provides server-sent events for live dashboard updates
4. **Collection Metrics** identified by `collection_id` + `plan_id` labels

### Grafana Dashboards
Pre-configured dashboards in `grafana/dashboards/`:
- `setagaya.json` - Main platform dashboard
- `setagaya_perf.json` - Performance metrics
- `setagaya_engine.json` - Engine-specific metrics

## Error Handling

### Error Types
```go
// Typed errors from model package
var dbe *model.DBError
if errors.As(err, &dbe) {
    // Handle database-specific errors
}
```

### API Error Mapping
- `s.handleErrors(w, err)` - Maps internal errors to HTTP status codes
- Consistent error responses across all endpoints
- Detailed logging for debugging

## Database Integration

### MySQL Configuration
- **Migrations:** Stored in `setagaya/db/` with timestamp prefixes
- **Connection:** Global access via `config.SC.DBC`
- **Patterns:** Active record pattern in `setagaya/model/`
- **Testing:** Isolated test database setup

### Model Architecture
- **Base Models:** Common functionality in `model/common.go`
- **Validation:** Built-in validation for all models
- **Relationships:** Proper foreign key management
- **Transactions:** Support for complex operations

## Performance Characteristics

### Scalability
- **Horizontal Scaling:** Multiple engine pods per plan
- **Resource Isolation:** Kubernetes namespace separation
- **Load Distribution:** Configurable engine placement
- **Metrics Aggregation:** Efficient collection and storage

### Resource Requirements
- **Minimum:** 2 CPU cores, 4GB RAM for control plane
- **Engine Resources:** Configurable per test plan
- **Storage:** Dependent on test data and result retention
- **Network:** Low latency between components preferred

## Security Considerations

### Container Security
- **Non-root Execution:** All containers run as UID 1001
- **Minimal Images:** Reduced attack surface
- **Static Binaries:** No dynamic dependencies
- **Network Policies:** Kubernetes-native isolation

### Data Security
- **Authentication:** LDAP integration
- **Authorization:** Group-based access control
- **Data Isolation:** Owner-based resource separation
- **Audit Logging:** Comprehensive operation logging

## Deployment Options

### Development
- **Local Kind Cluster:** Full stack deployment
- **Hot Reloading:** Fast iteration via `make setagaya`
- **Port Forwarding:** Direct access to services

### Production
- **Kubernetes Cluster:** Any CNCF-compliant distribution
- **Helm Charts:** Available in `helm_utils/`
- **High Availability:** Multi-replica deployments
- **Persistent Storage:** For test data and results

## Extension Points

### Adding New Schedulers
Implement `scheduler.EngineScheduler` interface:
```go
type EngineScheduler interface {
    Deploy(collection *model.Collection) error
    Trigger(collection *model.Collection) error
    Terminate(collection *model.Collection) error
    Purge(collection *model.Collection) error
}
```

### Adding Storage Backends
Implement `object_storage.Storage` interface:
```go
type Storage interface {
    Upload(bucket, key string, data []byte) error
    Download(bucket, key string) ([]byte, error)
    Delete(bucket, key string) error
}
```

### Adding Engine Types
Follow `setagaya/engines/jmeter/` structure:
- Agent sidecar pattern
- Metrics reporting endpoint
- Configuration file management
- Lifecycle management

## Migration and Upgrade

### Database Migrations
- **Automated:** Run on startup
- **Versioned:** Timestamp-based naming
- **Rollback:** Manual process via SQL scripts

### Container Updates
- **Rolling Updates:** Kubernetes-native
- **Zero Downtime:** Service mesh compatibility
- **Health Checks:** Kubernetes liveness/readiness probes

---

**Last Updated:** September 6, 2025
**Document Version:** 2.0
**Next Review:** Quarterly or on major releases
