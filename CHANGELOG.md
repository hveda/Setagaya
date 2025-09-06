# Changelog

## [2.0.0-rc](https://github.com/hveda/Setagaya/compare/v1.1.2...v2.0.0-rc) (2025-09-06)

### 🚀 Major Platform Modernization Release Candidate

#### ✨ Features
* **Complete Container Modernization**: Security-hardened Docker builds with Go 1.25.1
* **Multi-Stage Docker Builds**: Alpine 3.20 base images with minimal attack surface
* **JMeter Version Compatibility**: Support for both JMeter 3.3 (legacy) and 5.6.3 (modern)
* **GitHub Actions Security Suite**: Comprehensive security scanning and linting automation
* **Enterprise Documentation**: Complete technical specifications and security policies
* **Security-First Design**: All containers run as non-root user (UID 1001)
* **Auto-Formatting Infrastructure**: Prettier, yamllint with git hooks integration

#### 🔐 Security Enhancements
* **Container Security**: Multi-stage builds with static compilation and security flags
* **Automated Security Scanning**: Gosec, CodeQL, Trivy, secret scanning, SBOM generation
* **Security Policy Framework**: Comprehensive vulnerability disclosure and incident response
* **Continuous Monitoring**: Weekly security scans and dependency auditing
* **License Compliance**: Automated open source license verification

#### 🐳 Container Architecture Updates
* **Modern Dockerfiles**: 5 security-hardened Dockerfiles with Alpine 3.20
* **Version Agnostic Agent**: Dynamic JMeter path detection for version compatibility
* **Static Compilation**: CGO_ENABLED=0 with security linker flags
* **No HEALTHCHECK**: Eliminated OCI format warnings, Kubernetes-native health monitoring

#### 📚 Documentation Overhaul
* **Technical Specifications**: 430-line comprehensive technical documentation
* **Security Documentation**: SECURITY.md with disclosure procedures and best practices
* **AI Coding Guidelines**: Updated development patterns and modernization guidelines
* **JMeter Compatibility Guide**: Clear migration path between JMeter versions
* **Release Security Checklist**: 100+ security validation checkpoints

#### 🛠️ Development Tools & Quality
* **Auto-Formatting**: Prettier integration for YAML, Markdown, JSON, JavaScript
* **YAML Validation**: yamllint with formatter-friendly configuration
* **Git Hooks**: Pre-commit hooks with automatic formatting and validation
* **Development Scripts**: npm-based tool management and setup automation

#### 🤖 CI/CD Automation
* **Security Workflows**: Multi-tool security scanning with automated issue creation
* **Code Quality**: Comprehensive linting, testing, and validation automation
* **PR Validation**: Semantic validation, security impact assessment, coverage requirements
* **Dependency Management**: Automated dependency updates with Dependabot
* **Emergency Response**: Critical security advisory automation and escalation

#### 🔧 Configuration Enhancements
* **Organization Agnostic**: Configurable documentation links for any organization
* **Environment Detection**: Improved local development vs production configuration
* **Storage Flexibility**: Enhanced support for multiple storage backends
* **Security Configuration**: Comprehensive security settings and validation

### 🛠️ Technical Improvements
* **Go 1.25.1**: Latest stable Go version with security updates
* **Kubernetes Compatibility**: Enhanced RBAC and security policies
* **Metrics Pipeline**: Improved real-time metrics aggregation and streaming
* **Error Handling**: Enhanced error types and consistent API responses
* **Database Patterns**: Refined active record pattern with better validation

### 📦 Build System Updates
* **Component Builds**: Improved build.sh script with multiple targets
* **Kind Integration**: Enhanced local development with kind cluster automation
* **Image Management**: Efficient multi-platform builds and deployment
* **Security Scanning**: Integrated security scanning in build pipeline

### 🔄 Migration & Compatibility
* **Backward Compatibility**: Maintains compatibility with existing JMeter 3.3 deployments
* **Version Detection**: Automatic JMeter version detection and path configuration
* **Legacy Support**: Dedicated legacy Dockerfile for JMeter 3.3 environments
* **Smooth Migration**: Clear upgrade path from previous versions

## [1.1.2](https://github.com/hveda/Setagaya/compare/v1.1.1...v1.1.2) (2024-12-16)


### Bug Fixes

* fix metric dashboard repo url ([#131](https://github.com/hveda/Setagaya/issues/131)) ([161c5e6](https://github.com/hveda/Setagaya/commit/161c5e64208dcc5637aaf899d1b81298ee40adc3))

## [1.1.1](https://github.com/hveda/Setagaya/compare/v1.1.0...v1.1.1) (2024-12-16)


### Bug Fixes

* remove logging ([#129](https://github.com/hveda/Setagaya/issues/129)) ([83f9353](https://github.com/hveda/Setagaya/commit/83f93539c5b579ce1448fbfa752e254e7c8a2d8e))

## [1.1.0](https://github.com/hveda/Setagaya/compare/v1.0.0...v1.1.0) (2024-10-01)


### Features

* Enable engine metrics exposing in the agent ([#112](https://github.com/hveda/Setagaya/issues/112)) ([d7d25ad](https://github.com/hveda/Setagaya/commit/d7d25adcb96451bc33d1d536f5b7017a64e1f4ba))

## 1.0.0 (2024-08-30)


### Features

* add prefix to differ from main release ([70a38f5](https://github.com/hveda/Setagaya/commit/70a38f574ad5593c78d77456b6a83f735d62f3e4))
* introduce release please ([9f33ad0](https://github.com/hveda/Setagaya/commit/9f33ad0c7c22d1063b68fc22f7746e1ce748c86f))


### Bug Fixes

* add missing charts ([420cdf9](https://github.com/hveda/Setagaya/commit/420cdf94fa56d13b7bec7ce12dde20d14c1ffc39))
* add missing if ([fc5622c](https://github.com/hveda/Setagaya/commit/fc5622ca1a59ca3dec356039145bac5f6bf15c9c))
* better naming ([12a42de](https://github.com/hveda/Setagaya/commit/12a42de7e83c3e37f0e44a6fff923a5f59e48cfe))
* chart could not be generated due to tagging. Use gh cli directly instead of chart-releaser-action ([#111](https://github.com/hveda/Setagaya/issues/111)) ([8ab71bb](https://github.com/hveda/Setagaya/commit/8ab71bb47ce99c5c4d8e42976bcb277409f1354a))
* only build the image when it is a release ([d8fc0a1](https://github.com/hveda/Setagaya/commit/d8fc0a1496f591d6c9254460010b28e3187bf5d8))
* prevent fork polutting the officical release registry ([77906e5](https://github.com/hveda/Setagaya/commit/77906e5140365321eb881d7c1edf2db1a94e1ae9))
* should use release action from googleapi repo ([c38a4bb](https://github.com/hveda/Setagaya/commit/c38a4bb2aaeb172a4d1e44296715d950724f5008))
* wrong tag name ([4b33f75](https://github.com/hveda/Setagaya/commit/4b33f7506cf2863665052650b3744ec8505adf1e))
