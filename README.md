# Setagaya Load Testing Platform

## Introduction

**Setagaya** (formerly Shibuya) is a modern, distributed load testing platform that orchestrates JMeter engines in Kubernetes clusters. It provides a web UI, REST API, and real-time monitoring for scalable performance testing with enterprise-grade RBAC (Role-Based Access Control) integration.

Setagaya can scale quickly to higher capacity than distributed JMeter mode, provides real-time test results, and can be deployed on-premises or in public cloud environments with comprehensive security controls.

### Key Features

- 🚀 **Distributed Load Testing**: Deploy JMeter engines across Kubernetes clusters
- 🔐 **Enterprise RBAC**: 4-role permission system with 35 granular permissions  
- 🎨 **Modern UI**: Alpine.js + Tailwind CSS with responsive design
- 📊 **Real-time Monitoring**: Live metrics streaming with Grafana integration
- 🔄 **Multi-tenancy**: Project-based resource isolation with ownership controls
- 📦 **Container-native**: Docker multi-stage builds with distroless security
- 🌐 **API-first**: Comprehensive REST API for automation and integration

### Architecture Overview

Tests (Plans) are organized into Collections, which belong to Projects. Resources are managed by owners based on LDAP authentication with role-based permissions:

- **Administrator**: Full system access including user management
- **Project Manager**: Create/manage projects and collections, read-all monitoring  
- **Load Test User**: Own projects/collections only, basic monitoring
- **Monitor User**: Read-only access to projects, collections, and monitoring

Collections are the execution unit where multiple test plans can be triggered simultaneously, with results converged and displayed centrally.


## Getting Started

### Quick Local Setup

*Tested on Ubuntu, macOS, and Windows with WSL2*

**Prerequisites:**
- [Kind](https://kind.sigs.k8s.io) - Kubernetes in Docker
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl) - Kubernetes CLI
- [Helm](https://helm.sh/docs/intro/install/) - Kubernetes package manager
- [Docker](https://docs.docker.com/install) or [Podman](https://podman.io/) - Container runtime
- [Node.js 18+](https://nodejs.org/) - For UI build process

**Quick Start Commands:**

```bash
# Start local Kubernetes cluster with all dependencies
make

# Expose services for development
make expose  # Setagaya on :8080, Grafana on :3000

# Build and deploy changes
make setagaya

# Clean up environment
make clean
```

**Access URLs:**
- **Main Application**: http://localhost:8080
- **Admin Interface**: http://localhost:8080/admin (administrator role)
- **Grafana Dashboard**: http://localhost:3000
- **API Documentation**: http://localhost:8080/api/

### Configuration

1. **Copy template configuration:**
   ```bash
   cp config_tmpl.json config_env.json
   ```

2. **For local development, set:**
   ```json
   {
     "auth_config": {
       "no_auth": true,
       "session_key": "setagaya-session"
     },
     "db": {
       "host": "localhost",
       "user": "root", 
       "password": "password",
       "database": "setagaya_dev"
     }
   }
   ```

3. **Use `setagaya` as project owner when auth is disabled**

### Local Testing with RBAC Roles

When authentication is enabled for local testing, use these predefined accounts:

**Administrator Role:**
```
Username: admin
Password: admin123
Permissions: All 35 permissions (full system access)
```

**Project Manager Role:**
```
Username: manager
Password: manager123  
Permissions: 20 permissions (project/collection management, monitoring)
```

**Load Test User Role:**
```
Username: tester
Password: tester123
Permissions: 15 permissions (own projects/collections, basic monitoring)
```

**Monitor User Role:**
```
Username: monitor
Password: monitor123
Permissions: 6 permissions (read-only access to projects and monitoring)
```

**Testing RBAC in Local Environment:**
```bash
# Enable authentication for testing
# In config_env.json, set: "no_auth": false

# Test different user permissions
curl -u admin:admin123 http://localhost:8080/api/rbac/roles
curl -u manager:manager123 http://localhost:8080/api/projects  
curl -u tester:tester123 http://localhost:8080/api/collections
curl -u monitor:monitor123 http://localhost:8080/api/system/health
```

**UI Testing:**
- Login at http://localhost:8080/login with any of the above credentials
- UI elements will show/hide based on user permissions
- Admin users see the `/admin` interface
- Other roles see filtered content based on their permissions

### UI Development

The platform features a modern Alpine.js + Tailwind CSS interface:

```bash
# Install UI dependencies
make ui-deps

# Build production CSS
make ui-build

# Development with watch mode
npm run dev

# Lint JavaScript
make ui-lint
```

**UI Architecture:**
- **Alpine.js 3.x**: Reactive components with RBAC integration
- **Tailwind CSS 3.4**: Utility-first styling with custom Setagaya theme
- **Permission-based UI**: Dynamic visibility based on user roles
- **Responsive Design**: Mobile-first approach for all devices

## Architecture & Components

### Distributed Mode

Setagaya supports both single-process and distributed deployment modes:

**Single Process Mode (Default):**
- All components in one binary for local development
- Set `"distributed_mode": false` in configuration

**Distributed Mode:**
- **API Server**: Handles web UI and REST API requests
- **Controller**: Manages engine lifecycle and metrics collection  
- **Metrics Dashboard**: Grafana-based monitoring and visualization

Enable distributed mode: `"distributed_mode": true` in `config_env.json`

### RBAC System

Comprehensive role-based access control with 4 roles and 35 permissions:

```bash
# View all roles and permissions
curl http://localhost:8080/api/rbac/roles
curl http://localhost:8080/api/rbac/permissions

# User management (admin only)
curl http://localhost:8080/api/rbac/users
```

**Role Hierarchy:**
1. **Administrator** (35 permissions)
   - Full system access including user/role management
   - System configuration and monitoring

2. **Project Manager** (20 permissions)  
   - Create/manage projects and collections
   - Full monitoring access across projects
   - User management within projects

3. **Load Test User** (15 permissions)
   - Own projects/collections management
   - Execute tests and view own results
   - Basic monitoring access

4. **Monitor User** (6 permissions)
   - Read-only access to projects and collections
   - View monitoring data and test results
   - Download test files

### Container Architecture

**Multi-stage Docker Build:**
```dockerfile
# Stage 1: UI Build (Node.js)
FROM node:18-alpine AS ui-builder
# Tailwind CSS compilation, Alpine.js components

# Stage 2: Go Application Build  
FROM golang:1.21-alpine AS go-builder
# Binary compilation with optimizations

# Stage 3: Runtime (Distroless)
FROM gcr.io/distroless/base-debian11:nonroot
# Minimal attack surface, security-focused
```

**Security Features:**
- Distroless containers for minimal attack surface
- Non-root user execution (UID 65532)
- Read-only filesystem where possible
- Resource limits and security contexts


## Production Deployment

### Infrastructure Requirements

**Core Components:**

1. **Kubernetes Cluster**
   - In-cluster config: Service account with RBAC permissions (see `kubernetes/roles.yaml`)
   - Out-of-cluster: Generate kubeconfig and place in `setagaya/config/kube_configs/config`
   - Set `"in_cluster": true/false` in `config_env.json`

2. **Database** (Required)
   ```yaml
   # MariaDB v11.8+ or MySQL 8.0+
   host: "mysql.setagaya.svc.cluster.local"
   port: 3306
   database: "setagaya_prod"
   ```

3. **Object Storage** (Required)
   - **Nexus Repository** (recommended) or **GCP Cloud Storage**
   - Stores test plans, results, and artifacts
   - Implements pluggable storage interface

4. **Monitoring Stack** (Required)
   - **Prometheus**: Scrapes `http://setagaya-controller/metrics`
   - **Grafana**: Dashboards in `grafana/dashboards/` folder
   - Real-time metrics and alerting

5. **Authentication** (Optional)
   - **LDAP integration** for enterprise authentication
   - **JWT tokens** for API access
   - **Session management** with secure cookies

### Cloud Provider Integration

**Google Cloud Platform:**
```json
{
  "gcp_config": {
    "project_id": "your-project",
    "credentials_file": "/auth/setagaya-gcp.json",
    "auto_scaling": true
  }
}
```

**AWS Integration:**
```json
{
  "aws_config": {
    "region": "us-west-2", 
    "cluster_name": "setagaya-engines",
    "auto_scaling": true
  }
}
```

### Helm Deployment

```bash
# Add Setagaya Helm repository
helm repo add setagaya https://charts.setagaya.dev
helm repo update

# Install with custom values
helm install setagaya setagaya/setagaya \
  --namespace setagaya-system \
  --create-namespace \
  --values values-production.yaml

# Upgrade deployment
helm upgrade setagaya setagaya/setagaya \
  --values values-production.yaml
```

### Production Configuration

**Example `config_env.json`:**
```json
{
  "auth_config": {
    "no_auth": false,
    "ldap_config": {
      "host": "ldap.company.com",
      "port": 636,
      "use_ssl": true,
      "base_dn": "dc=company,dc=com"
    },
    "session_key": "setagaya-session",
    "jwt_secret": "your-secret-key"
  },
  "db": {
    "host": "mysql.setagaya.svc.cluster.local",
    "port": 3306,
    "user": "setagaya",
    "password": "${DB_PASSWORD}",
    "database": "setagaya_prod"
  },
  "storage_config": {
    "type": "gcp",
    "bucket": "setagaya-test-plans",
    "credentials_file": "/auth/gcp-storage.json"
  },
  "dashboard_config": {
    "url": "https://grafana.company.com",
    "run_dashboard": "/d/setagaya-performance",
    "engines_dashboard": "/d/setagaya-engines"
  },
  "distributed_mode": true,
  "enable_rbac": true,
  "context": "production"
}
```

## API Reference

### REST API Endpoints

**Project Management:**
```bash
GET    /api/projects              # List projects (with RBAC filtering)
POST   /api/projects              # Create project (requires projects:create)
GET    /api/projects/:id          # Get project details
PUT    /api/projects/:id          # Update project (owner or projects:update)
DELETE /api/projects/:id          # Delete project (owner or projects:delete)
```

**Collection Management:**
```bash
GET    /api/projects/:id/collections    # List collections
POST   /api/projects/:id/collections    # Create collection
GET    /api/collections/:id             # Get collection details
PUT    /api/collections/:id             # Update collection
DELETE /api/collections/:id             # Delete collection
POST   /api/collections/:id/execute     # Execute load test
```

**RBAC Management:**
```bash
GET    /api/rbac/roles                      # List all roles
POST   /api/rbac/roles                      # Create role (admin only)
GET    /api/rbac/roles/:id                  # Get role details
PUT    /api/rbac/roles/:id                  # Update role (admin only)
DELETE /api/rbac/roles/:id                  # Delete role (admin only)

GET    /api/rbac/permissions               # List all permissions
GET    /api/rbac/users                     # List users (filtered by permissions)
POST   /api/rbac/users                     # Create user (requires users:create)
GET    /api/rbac/users/:id/permissions     # Get user's effective permissions
POST   /api/rbac/users/:id/roles           # Assign role to user
DELETE /api/rbac/users/:id/roles/:role_id  # Remove role from user
```

**Monitoring & Metrics:**
```bash
GET    /api/metrics                   # Prometheus metrics endpoint
GET    /api/collections/:id/metrics   # Collection-specific metrics
GET    /api/plans/:id/results         # Test plan results
GET    /api/system/health             # System health check
```

### Authentication

**Session-based (Web UI):**
```bash
POST /login    # LDAP authentication
POST /logout   # Session termination
```

**API Token-based:**
```bash
# Include in headers
Authorization: Bearer <jwt-token>
X-API-Key: <api-key>
```

### WebSocket Real-time Updates

```javascript
// Connect to real-time metrics stream
const ws = new WebSocket('ws://localhost:8080/ws/metrics');
ws.onmessage = (event) => {
  const metrics = JSON.parse(event.data);
  // Handle real-time test metrics
};
```
## Development

### Project Structure

```
setagaya/
├── api/                    # REST API handlers and middleware
├── auth/                   # Authentication & authorization
├── config/                 # Configuration management  
├── controller/             # Engine orchestration & metrics
├── model/                  # Database entities & CRUD operations
├── scheduler/              # Kubernetes engine deployment
├── ui/                     # Web interface
│   ├── templates/          # Go HTML templates
│   ├── static/             # CSS, JS, assets
│   └── handler.go          # UI route handlers
├── engines/jmeter/         # JMeter engine implementation
└── db/                     # Database migrations

kubernetes/                 # K8s deployment manifests
grafana/                    # Monitoring dashboards
docs/                       # Documentation
```

### Building from Source

```bash
# Clone repository
git clone https://github.com/hveda/setagaya.git
cd setagaya

# Build Go binary
cd setagaya
go mod download
go build -o build/setagaya .

# Build UI assets
npm install
npm run build-css-prod

# Build Docker image
docker build -t setagaya:latest .

# Run tests
go test ./... -v
npm test
```

### Contributing

1. **Fork the repository**
2. **Create feature branch**: `git checkout -b feature/your-feature`
3. **Follow coding standards**:
   - Go: `gofmt`, `golangci-lint`
   - JavaScript: ESLint with Alpine.js rules
   - CSS: Tailwind CSS utilities only
4. **Add tests** for new functionality
5. **Update documentation** as needed
6. **Submit pull request**

### Testing

```bash
# Unit tests
go test ./model -v          # Database tests
go test ./api -v            # API endpoint tests  
go test ./auth -v           # Authentication tests

# Integration tests
make test                   # Full test suite
make test-ui                # UI component tests

# E2E tests
make test-e2e               # End-to-end scenarios
```

### Debugging

```bash
# Enable debug logging
export LOG_LEVEL=debug

# Run with profiling
go run main.go -cpuprofile=cpu.prof -memprofile=mem.prof

# Database debugging
export DB_DEBUG=true

# UI debugging (development mode)
npm run dev                 # Watch mode with hot reload
```

## Terminology

- **Controller**: Main Setagaya process that schedules engines, serves UI, and collects metrics
- **Engine/Executor**: Load generating pod (JMeter + Agent) deployed in Kubernetes
- **Agent**: Sidecar process that communicates with controller to manage JMeter execution
- **Context**: Kubernetes cluster that Setagaya controller manages
- **Collection**: Group of test plans that can be executed together
- **Project**: Top-level organizational unit containing collections and plans
- **RBAC**: Role-Based Access Control system with granular permissions

## Limitations & Known Issues

- **Single Context**: One controller manages one Kubernetes cluster currently
- **Sequential Execution**: No simultaneous execution across multiple contexts
- **JMeter Focus**: Primary support for JMeter, other engines require custom implementation
- **Resource Limits**: Engine capacity limited by cluster node resources
- **Storage Dependencies**: Requires external storage (Nexus/GCS) for test plan persistence

## Roadmap

### Short Term (Current Development)
- ✅ **UI Modernization**: Alpine.js + Tailwind CSS migration (Completed)
- ✅ **RBAC Implementation**: Enterprise role-based access control (Completed)
- 🔄 **Multi-cluster Support**: Manage engines across multiple Kubernetes clusters
- 🔄 **Enhanced Monitoring**: Improved Grafana dashboards and alerting

### Medium Term (Next Quarter)
- 📋 **Gatling Support**: Additional load testing engine integration
- 📋 **API Gateway**: Rate limiting and API versioning
- 📋 **Audit Logging**: Comprehensive activity tracking and compliance
- 📋 **Auto-scaling**: Dynamic engine scaling based on test requirements

### Long Term (Future Releases)  
- 📋 **Multi-tenant Isolation**: Complete resource isolation between tenants
- 📋 **GitOps Integration**: Test plan management via Git repositories
- 📋 **AI/ML Analytics**: Intelligent test result analysis and recommendations
- 📋 **Hybrid Cloud**: Support for multi-cloud engine deployment

## Performance & Scalability

**Tested Configurations:**
- **Small**: 10 concurrent engines, 1K virtual users
- **Medium**: 50 concurrent engines, 10K virtual users  
- **Large**: 200+ concurrent engines, 100K+ virtual users

**Optimization Guidelines:**
- Use dedicated node pools for engines
- Configure resource requests/limits appropriately
- Monitor cluster autoscaling for cost optimization
- Implement test plan caching for faster startup

## Security

### Best Practices

1. **Container Security**:
   - Distroless base images
   - Non-root user execution
   - Read-only filesystems
   - Resource constraints

2. **Network Security**:
   - Network policies for pod isolation
   - TLS/SSL for all external communications
   - Service mesh integration (Istio/Linkerd)

3. **Access Control**:
   - RBAC with principle of least privilege
   - LDAP/SSO integration for authentication
   - API key rotation and management
   - Session timeout and secure cookies

4. **Data Protection**:
   - Encrypted storage for sensitive test data
   - Secure credential management (Vault/Secret Manager)
   - Audit logging for compliance requirements

## Support & Community

- **Documentation**: [docs.setagaya.dev](https://docs.setagaya.dev)
- **Issues**: [GitHub Issues](https://github.com/hveda/setagaya/issues)
- **Discussions**: [GitHub Discussions](https://github.com/hveda/setagaya/discussions)
- **Slack Community**: [#setagaya-users](https://setagaya-community.slack.com)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- JMeter community for the excellent load testing foundation
- Kubernetes community for container orchestration platform  
- Alpine.js and Tailwind CSS teams for modern UI frameworks
- All contributors who have helped shape Setagaya

---

**Made with ❤️ by the Setagaya team**
