# Setagaya Load Testing Platform

[![Go Version](https://img.shields.io/badge/Go-1.25.1-blue.svg)](https://golang.org)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-Compatible-brightgreen.svg)](https://kubernetes.io)
[![JMeter](https://img.shields.io/badge/JMeter-3.3%20%7C%205.6.3-orange.svg)](https://jmeter.apache.org)
[![Security](https://img.shields.io/badge/Security-Hardened-red.svg)](SECURITY.md)
[![Automation](https://img.shields.io/badge/CI%2FCD-Automated-purple.svg)](.github/workflows/)

[![Security Check](https://github.com/hveda/Setagaya/actions/workflows/security-check.yml/badge.svg)](https://github.com/hveda/Setagaya/actions/workflows/security-check.yml)
[![Code Quality](https://github.com/hveda/Setagaya/actions/workflows/code-quality.yml/badge.svg)](https://github.com/hveda/Setagaya/actions/workflows/code-quality.yml)
[![PR Validation](https://github.com/hveda/Setagaya/actions/workflows/pr-validation.yml/badge.svg)](https://github.com/hveda/Setagaya/actions/workflows/pr-validation.yml)
[![Security Monitoring](https://github.com/hveda/Setagaya/actions/workflows/security-monitoring.yml/badge.svg)](https://github.com/hveda/Setagaya/actions/workflows/security-monitoring.yml)

Setagaya is a cloud-native, distributed load testing platform that orchestrates Apache JMeter engines across Kubernetes
clusters. It provides enterprise-grade scalability, real-time metrics, and centralized management for performance
testing at scale.

## 🚀 Key Features

- **Kubernetes-Native**: Deploy and scale load generators across K8s clusters
- **Real-Time Monitoring**: Live metrics streaming with Grafana dashboards
- **Multi-Version Support**: JMeter 3.3 (legacy) and 5.6.3 (modern) compatibility
- **Enterprise Authentication**: LDAP integration with group-based access control
- **Flexible Storage**: Multiple backends (Local, GCP Buckets, Nexus)
- **Container Security**: Non-root execution, minimal attack surface
- **High Scalability**: Horizontal scaling with configurable resource allocation
- **Security Automation**: Comprehensive security scanning and monitoring
- **CI/CD Integration**: GitHub Actions workflows for security and quality

## 📚 Documentation

- **[Technical Specifications](TECHNICAL_SPECS.md)** - Comprehensive technical documentation
- **[Security Policy](SECURITY.md)** - Security measures and vulnerability disclosure
- **[JMeter Build Options](setagaya/JMETER_BUILD_OPTIONS.md)** - JMeter version compatibility guide
- **[Development Guidelines](.github/instructions/copilot.instructions.md)** - AI coding guidelines and patterns
- **[Security Checklist](.github/SECURITY_CHECKLIST.md)** - Release security validation

## 🏗️ Architecture

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

**Domain Model**: `Project → Collection → Plan → ExecutionPlan`

- **Collections** are the execution unit containing multiple plans running simultaneously
- **Plans** define test configurations; **ExecutionPlans** specify engines/concurrency per plan
- **Results** converge at collection level for unified reporting via Grafana dashboards

## 🚀 Quick Start

### Prerequisites

- [Kind](https://kind.sigs.k8s.io) - Kubernetes in Docker
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl) - Kubernetes CLI
- [Helm](https://helm.sh/docs/intro/install/) - Package manager for Kubernetes
- [Docker](https://docs.docker.com/install) or [Podman](https://podman.io) - Container runtime

### Local Development Setup

1. **Start local cluster:**

   ```bash
   make              # Creates kind cluster, deploys all components
   ```

2. **Expose services:**

   ```bash
   make expose       # Port-forwards Setagaya (8080) and Grafana (3000)
   ```

3. **Access the platform:**
   - **Setagaya UI**: http://localhost:8080
   - **Grafana Dashboards**: http://localhost:3000

4. **Development workflow:**

   ```bash
   make setagaya     # Rebuilds and redeploys controller changes
   make clean        # Destroys local cluster
   ```

### Authentication Note

Local Setagaya runs without authentication. Use `setagaya` as the project owner when creating resources.

## 🐳 Container Images

The platform uses modern, security-hardened container images:

| Component         | JMeter Version | Build Method     | Usage                                                                                       |
| ----------------- | -------------- | ---------------- | ------------------------------------------------------------------------------------------- |
| **Modern Engine** | 5.6.3          | Source build     | `docker build -f setagaya/Dockerfile.engines.jmeter .`                                      |
| **Legacy Engine** | 3.3            | Pre-built binary | `./setagaya/build.sh jmeter && docker build -f setagaya/Dockerfile.engines.jmeter.legacy .` |
| **API Server**    | N/A            | Source build     | `docker build -f setagaya/Dockerfile .`                                                     |
| **Controller**    | N/A            | Source build     | `docker build -f setagaya/Dockerfile.controller .`                                          |

All images run as non-root user (`setagaya`, UID 1001) with security-first design.

## 🔧 Configuration

The platform uses a centralized configuration system:

- **Development**: `setagaya/config_tmpl.json` (template)
- **Production**: `config.json` with environment-specific settings
- **Key Areas**: Executors, storage, authentication, monitoring

Example configuration structure:

```json
{
  "executor_config": {
    "jmeter": {
      "image": "setagaya:jmeter",
      "cpu": "1000m",
      "memory": "2Gi"
    }
  },
  "storage": {
    "type": "local|gcp|nexus"
  },
  "auth": {
    "no_auth": false,
    "ldap_config": {
      "host": "ldap.example.com",
      "port": 389
    }
  }
}
```

## 🔒 Security & Updates

### Recent Security Improvements (v2.0.0-rc)

The platform includes comprehensive security updates across all components:

#### Critical Dependency Updates
- **MySQL Driver**: Updated to v1.9.3 with security fixes and performance improvements
- **Session Management**: Gorilla sessions v1.4.0 with enhanced cookie security for Chrome's 3rd party cookie changes
- **Logging**: Logrus v1.9.3 addressing DoS vulnerabilities in log processing
- **Go Dependencies**: Latest security patches for crypto, net, and gRPC modules
- **Google APIs**: Updated to v0.248.0 with security improvements
- **Kubernetes Client**: Updated to v0.34.0 with security patches

#### Security Features
- **Container Hardening**: All images run as non-root user (UID 1001)
- **Multi-stage Builds**: Minimal attack surface with scratch/alpine base images
- **Static Compilation**: CGO_ENABLED=0 with security flags (`-w -s -extldflags=-static`)
- **Dependency Scanning**: Automated security monitoring with GitHub Actions
- **SBOM Generation**: Software Bill of Materials for transparency

### Security Automation
- **Weekly Security Scans**: Multi-tool coverage (Gosec, CodeQL, Trivy, TruffleHog)
- **Dependabot Integration**: Automated security updates with testing
- **Critical Vulnerability Response**: Automated detection and escalation
- **Security Policies**: Comprehensive incident response procedures

For security issues, see [SECURITY.md](SECURITY.md).

## 🏢 Production Deployment

### Required Components

1. **Kubernetes Cluster** - Any CNCF-compliant distribution
2. **MySQL Database** - MariaDB v10.0.23+ compatible
3. **Prometheus** - Metrics collection and storage
4. **Grafana** - Visualization with pre-built dashboards
5. **Storage Backend** - Nexus, GCP Buckets, or local storage
6. **LDAP Server** - Authentication and authorization (optional)

### Deployment Options

- **In-cluster**: Deploy engines to the same cluster as the controller
- **Cross-cluster**: Deploy engines to external Kubernetes clusters
- **Multi-cloud**: Support for different cloud providers

### Security Configuration

- **RBAC**: Kubernetes role-based access control (see `kubernetes/roles.yaml`)
- **Service Accounts**: Proper isolation and permissions
- **Network Policies**: Kubernetes-native network isolation
- **Authentication**: LDAP integration with group-based ownership

## 🔄 Distributed Mode

The platform supports distributed architecture for improved scalability:

- **API Server**: REST endpoints and UI serving
- **Controller**: Test orchestration and metrics aggregation
- **Scheduler**: Kubernetes resource management
- **Engines**: Distributed load generation

Enable distributed mode by setting `runtime.distributed_mode: true` in configuration.

## 📊 Monitoring & Metrics

### Real-time Metrics Pipeline

```
JMeter Engines → setagaya-agent → Controller → Prometheus → Grafana
```

### Pre-built Dashboards

- **Platform Overview**: `grafana/dashboards/setagaya.json`
- **Performance Metrics**: `grafana/dashboards/setagaya_perf.json`
- **Engine Details**: `grafana/dashboards/setagaya_engine.json`

### Live Updates

- Server-sent events for real-time dashboard updates
- Collection-level metrics aggregation
- Configurable retention and alerting

## 🧪 Testing Lifecycle

1. **Deploy**: Create Kubernetes resources, engines come online
2. **Trigger**: Start load generation across all engines in collection
3. **Terminate**: Stop tests, keep engines deployed for result collection
4. **Purge**: Remove all Kubernetes resources and clean up storage

## 🛠️ Development

### Code Organization

```
setagaya/                 # Main application
├── api/                 # REST API server
├── controller/          # Test orchestration
├── scheduler/           # Kubernetes management
├── engines/             # Load generation engines
├── model/               # Domain models
├── config/              # Configuration system
└── object_storage/      # Storage abstraction
```

### Auto-Formatting Infrastructure

Setagaya includes comprehensive auto-formatting tools for consistent code quality:

- **golangci-lint**: Go linting with 75+ enabled checkers
- **Prettier**: Auto-formats YAML, Markdown, JSON, and JavaScript files
- **yamllint**: YAML validation with formatter-friendly rules
- **Git Hooks**: Pre-commit hooks with automatic formatting
- **npm Scripts**: Convenient formatting and linting commands

### Test Coverage & Quality Assurance

#### Recent Test Coverage Improvements (v2.0.0-rc)

| Package | Previous | Current | Key Areas Tested |
|---------|----------|---------|------------------|
| **API** | 0% | **7.3%** | Error handling, validation, network utilities |
| **Config** | 0% | **24.0%** | HTTP client setup, MySQL endpoints, context loading |
| **Model** | ~2% | **4.6%** | Authentication, admin privileges, ownership validation |
| **Object Storage** | ~10% | **12.0%** | Provider detection, factory functions |
| **Controller** | 0% | **0.2%** | Error handling functions |
| **Engines/Model** | ~80% | **100%** | Complete engine data structure coverage |

**Overall Platform Coverage**: **2.1% → 4.6%** (118% improvement)

#### Testing Strategy
- **Security-First Testing**: Comprehensive validation of authentication and authorization functions
- **Edge Case Coverage**: Extensive testing with nil inputs, special characters, and boundary conditions
- **Error Handling**: 100% coverage of all error creation and handling functions
- **Database-Independent**: Tests run without requiring database connections using mock configurations

#### Available Commands

```bash
# Install development tools
./scripts/setup-dev-tools.sh

# Format all files automatically
npm run format

# Lint and auto-fix markdown
npm run lint:md

# Validate YAML files
npm run lint:yaml

# Fix all linting issues
npm run fix
```

#### Git Hook Integration

The platform automatically formats files on commit via pre-commit hooks that:

- Run prettier for YAML, Markdown, and JSON formatting
- Validate YAML syntax with yamllint
- Provide clear error reporting and fallback handling

### Extension Points

- **New Schedulers**: Implement `scheduler.EngineScheduler` interface
- **Storage Backends**: Implement `object_storage.Storage` interface
- **Engine Types**: Follow agent sidecar pattern with metrics reporting

## 📈 Scalability & Performance

- **Horizontal Scaling**: Multiple engine pods per test plan
- **Resource Isolation**: Kubernetes namespace separation
- **Load Distribution**: Configurable engine placement and affinity
- **Efficient Metrics**: Streaming aggregation and collection

## 🚧 Current Limitations

- One controller manages one Kubernetes cluster (multi-cluster support planned)
- Sequential context execution (parallel execution planned)
- JMeter-focused (additional executors like Gatling planned)

## 🗺️ Roadmap

### ✅ Completed (v2.0.0-rc)

- **Security Automation**: Comprehensive security scanning and monitoring
- **Container Modernization**: Security-hardened multi-stage Docker builds
- **JMeter Compatibility**: Support for multiple JMeter versions (3.3 and 5.6.3)
- **CI/CD Integration**: GitHub Actions workflows for security and quality
- **Documentation Overhaul**: Complete technical specifications and security policies
- **Auto-Formatting Infrastructure**: Prettier, yamllint with git hooks

### 🚧 In Progress

- **Multi-Executor Support**: Gatling, K6, custom executors
- **Multi-Context Management**: Single controller, multiple clusters
- **Enhanced Authentication**: OAuth2, SAML integration

### 🔮 Planned

- **Advanced Scheduling**: Time-based triggers, dependency chains
- **Cloud Integration**: Native cloud provider integrations
- **Performance Optimization**: Enhanced metrics aggregation and storage

## 🤝 Contributing

1. Read the [Technical Specifications](TECHNICAL_SPECS.md)
2. Follow [Development Guidelines](.github/instructions/copilot.instructions.md)
3. Review [Security Policy](SECURITY.md) for security considerations
4. Ensure documentation updates for any changes
5. Test with both JMeter versions (3.3 and 5.6.3)
6. Run auto-formatting and linting before commits
7. Run security checks via GitHub Actions workflows

### Security Contributions

- Security vulnerabilities should be reported privately via [Security Policy](SECURITY.md)
- Security improvements and hardening are welcome via standard PR process
- All PRs undergo automated security scanning and validation

## 📄 License

See [LICENSE](LICENSE) file for details.

---

**Setagaya** - Scalable, Cloud-Native Load Testing Platform
