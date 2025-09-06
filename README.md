# Setagaya Load Testing Platform

[![Go Version](https://img.shields.io/badge/Go-1.25.1-blue.svg)](https://golang.org)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-Compatible-brightgreen.svg)](https://kubernetes.io)
[![JMeter](https://img.shields.io/badge/JMeter-3.3%20%7C%205.6.3-orange.svg)](https://jmeter.apache.org)

Setagaya is a cloud-native, distributed load testing platform that orchestrates Apache JMeter engines across Kubernetes clusters. It provides enterprise-grade scalability, real-time metrics, and centralized management for performance testing at scale.

## 🚀 Key Features

- **Kubernetes-Native**: Deploy and scale load generators across K8s clusters
- **Real-Time Monitoring**: Live metrics streaming with Grafana dashboards
- **Multi-Version Support**: JMeter 3.3 (legacy) and 5.6.3 (modern) compatibility
- **Enterprise Authentication**: LDAP integration with group-based access control
- **Flexible Storage**: Multiple backends (Local, GCP Buckets, Nexus)
- **Container Security**: Non-root execution, minimal attack surface
- **High Scalability**: Horizontal scaling with configurable resource allocation

## 📚 Documentation

- **[Technical Specifications](TECHNICAL_SPECS.md)** - Comprehensive technical documentation
- **[JMeter Build Options](setagaya/JMETER_BUILD_OPTIONS.md)** - JMeter version compatibility guide
- **[Development Guidelines](.github/copilot-instructions.md)** - AI coding guidelines and patterns

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

| Component | JMeter Version | Build Method | Usage |
|-----------|----------------|--------------|-------|
| **Modern Engine** | 5.6.3 | Source build | `docker build -f setagaya/Dockerfile.engines.jmeter .` |
| **Legacy Engine** | 3.3 | Pre-built binary | `./setagaya/build.sh jmeter && docker build -f setagaya/Dockerfile.engines.jmeter.legacy .` |
| **API Server** | N/A | Source build | `docker build -f setagaya/Dockerfile .` |
| **Controller** | N/A | Source build | `docker build -f setagaya/Dockerfile.controller .` |

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
    "ldap_config": { /* LDAP settings */ }
  }
}
```

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

- **Multi-Executor Support**: Gatling, K6, custom executors
- **Multi-Context Management**: Single controller, multiple clusters
- **Enhanced Authentication**: OAuth2, SAML integration
- **Advanced Scheduling**: Time-based triggers, dependency chains
- **Cloud Integration**: Native cloud provider integrations

## 🤝 Contributing

1. Read the [Technical Specifications](TECHNICAL_SPECS.md)
2. Follow [Development Guidelines](.github/copilot-instructions.md)
3. Ensure documentation updates for any changes
4. Test with both JMeter versions (3.3 and 5.6.3)

## 📄 License

See [LICENSE](LICENSE) file for details.

---

**Setagaya** - Scalable, Cloud-Native Load Testing Platform
