# Setagaya Documentation Index

Welcome to the Setagaya Load Testing Platform documentation. This index helps you find the right documentation for your needs.

## 📚 Core Documentation

### [📖 README.md](README.md)
- **Purpose**: Project overview and quick start
- **Audience**: First-time users, general overview
- **Contents**: Features, architecture overview, installation, basic usage

### [🔧 Technical Specifications](TECHNICAL_SPECS.md)
- **Purpose**: Comprehensive technical documentation
- **Audience**: Developers, system administrators, architects
- **Contents**: Detailed architecture, configuration, deployment, APIs

### [🔒 Security Documentation](SECURITY.md)
- **Purpose**: Security policies and vulnerability disclosure
- **Audience**: Security teams, operators, researchers
- **Contents**: Vulnerability reporting, security measures, best practices

### [📋 Security Checklist](.github/SECURITY_CHECKLIST.md)
- **Purpose**: Release security validation checklist
- **Audience**: Release managers, security officers
- **Contents**: 100+ security checkpoints for releases

## 🚀 Getting Started

### Quick Start Path
1. **Start Here**: [README.md](README.md) - Overview and local setup
2. **Deep Dive**: [Technical Specifications](TECHNICAL_SPECS.md) - Complete technical details
3. **JMeter Setup**: [JMeter Build Options](setagaya/JMETER_BUILD_OPTIONS.md) - Engine configuration

### For Different Audiences

#### 👩‍💻 **Developers**
- [Development Guidelines](.github/copilot-instructions.md) - AI coding guidelines and patterns
- [Technical Specifications](TECHNICAL_SPECS.md) - Architecture and extension points
- [JMeter Build Options](setagaya/JMETER_BUILD_OPTIONS.md) - Engine compatibility

#### 🏢 **Operations Teams**
- [README.md](README.md) - Production deployment overview
- [Technical Specifications](TECHNICAL_SPECS.md) - Infrastructure requirements
- [Security Documentation](SECURITY.md) - Security policies and procedures
- [Kubernetes Configs](kubernetes/) - RBAC and deployment manifests

#### 🧪 **Test Engineers**
- [README.md](README.md) - Platform capabilities and workflow
- [Technical Specifications](TECHNICAL_SPECS.md) - Test lifecycle and monitoring

#### 🔒 **Security Teams**
- [Security Policy](SECURITY.md) - Vulnerability disclosure and security measures
- [Security Checklist](.github/SECURITY_CHECKLIST.md) - Release security validation
- [Security Workflows](.github/workflows/) - Automated security scanning and monitoring

## 📁 Component-Specific Documentation

### Container & Build System
- **[JMeter Build Options](setagaya/JMETER_BUILD_OPTIONS.md)** - JMeter version compatibility (3.3 vs 5.6.3)
- **[Dockerfiles](setagaya/)** - Multi-stage, security-hardened container builds
- **[Makefile](makefile)** - Local development and build automation

### Configuration & Deployment
- **[Config Template](setagaya/config_tmpl.json)** - Example configuration structure
- **[Kubernetes Manifests](kubernetes/)** - Production deployment configs
- **[Grafana Configuration](grafana/)** - Pre-built monitoring dashboards and configuration

### Development & Extensions
- **[Copilot Instructions](.github/copilot-instructions.md)** - AI coding guidelines
- **[Model Tests](setagaya/model/)** - Database and domain model patterns
- **[API Documentation](setagaya/api/)** - REST endpoint implementations

## 🔍 Finding Specific Information

### Architecture & Design
```
Project Structure: README.md → Core Components
Detailed Architecture: TECHNICAL_SPECS.md → Architecture Overview
Domain Model: TECHNICAL_SPECS.md → Domain Model
```

### Installation & Setup
```
Quick Setup: README.md → Quick Start
Local Development: README.md → Local Development Setup
Production Deployment: TECHNICAL_SPECS.md → Deployment Options
```

### Configuration
```
Basic Config: README.md → Configuration
Detailed Config: TECHNICAL_SPECS.md → Configuration System
Examples: setagaya/config_tmpl.json
```

### JMeter & Engines
```
Overview: README.md → Container Images
Version Support: setagaya/JMETER_BUILD_OPTIONS.md
Technical Details: TECHNICAL_SPECS.md → JMeter Engine Compatibility
```

### Development
```
Getting Started: .github/copilot-instructions.md
Coding Patterns: TECHNICAL_SPECS.md → Extension Points
Testing: TECHNICAL_SPECS.md → Development Workflow
```

## 🆘 Common Questions

**Q: Which JMeter version should I use?**
→ See [JMeter Build Options](setagaya/JMETER_BUILD_OPTIONS.md)

**Q: How do I deploy to production?**
→ See [Technical Specifications](TECHNICAL_SPECS.md) → Deployment Options

**Q: How do I extend the platform?**
→ See [Technical Specifications](TECHNICAL_SPECS.md) → Extension Points

**Q: What are the security considerations?**
→ See [Security Policy](SECURITY.md) and [Technical Specifications](TECHNICAL_SPECS.md) → Security Considerations

**Q: How do I report a security vulnerability?**
→ See [Security Policy](SECURITY.md) → Reporting a Vulnerability

**Q: How do I set up monitoring?**
→ See [Technical Specifications](TECHNICAL_SPECS.md) → Metrics and Monitoring

**Q: What security automation is available?**
→ See [GitHub Actions Workflows](.github/workflows/) and [Security Checklist](.github/SECURITY_CHECKLIST.md)

## 📝 Documentation Standards

All documentation follows these principles:
- **Comprehensive**: Technical specs cover all aspects
- **Current**: Updated with every code change (enforced by copilot instructions)
- **Layered**: README for overview, Technical Specs for details
- **Practical**: Examples and usage patterns included
- **Secure**: Security considerations documented

---

**Need help?** Start with the [README.md](README.md) for overview, then dive into [Technical Specifications](TECHNICAL_SPECS.md) for detailed information.
