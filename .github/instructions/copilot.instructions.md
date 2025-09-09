---
applyTo: '**'
---

# Setagaya Load Testing Platform - AI Coding Guidelines

## Pull Request Title and Description Requirements

### CRITICAL: Follow Conventional Commit Format for PR Titles

All pull request titles MUST follow the conventional commit format enforced by the PR validation workflow:

**Format**: `<type>: <description>`

**Allowed Types**:
- `feat` - New features or capabilities
- `fix` - Bug fixes, issue resolution, workflow failures
- `docs` - Documentation updates, README changes
- `style` - Code style changes (formatting, missing semicolons, etc.)
- `refactor` - Code refactoring without changing functionality
- `perf` - Performance improvements
- `test` - Adding or updating tests
- `build` - Changes to build system, dependencies, package management
- `ci` - Changes to CI/CD configuration, GitHub Actions workflows
- `chore` - Maintenance tasks, housekeeping
- `revert` - Reverting previous commits

**Title Requirements**:
- Subject (description part) must NOT start with an uppercase letter
- Use lowercase for the first word after the colon and space
- Keep descriptions concise but meaningful
- Do not use `release` as a scope (disallowed)

**Examples**:
- ✅ `fix: resolve TruffleHog secret scanning workflow failures`
- ✅ `feat: add new JMeter engine scheduler interface`
- ✅ `docs: update TECHNICAL_SPECS.md with security automation details`
- ✅ `build: update dependencies and consolidate Dependabot PRs`
- ✅ `ci: improve GitHub Actions workflow error handling`
- ❌ `Fix: TruffleHog issues` (uppercase after colon)
- ❌ `Consolidate dependency updates` (missing type prefix)
- ❌ `feat(release): new feature` (disallowed scope)

**Multiple Change Types**: When a PR contains multiple types of changes, choose the primary/most impactful type:
- Security fixes and workflow failures → `fix:`
- New features with documentation → `feat:`
- Dependency updates with CI changes → `build:`

**Dependabot Configuration**: 
- Dependabot PRs use `build:` prefix for Go module and Docker dependency updates
- Dependabot PRs use `ci:` prefix for GitHub Actions updates
- This is configured in `.github/dependabot.yml` and ensures PR validation passes
- Never manually change Dependabot PR titles - fix the configuration instead

**Test Environment Setup**:
- Tests require `SETAGAYA_TEST_MODE=true` environment variable to use test configuration
- This is configured in the PR validation workflow (`pr-validation.yml`)
- Test mode provides a minimal config without requiring external files

### PR Description Requirements

- Minimum 10 characters in length
- Provide meaningful description of changes
- Include breaking changes indicator if applicable (`BREAKING CHANGE` or `!` in title)
- Document impact and validation performed

## Documentation Maintenance Requirements

### CRITICAL: Always Update Documentation

When making any changes to the codebase, you MUST update relevant documentation:

1. **Technical Specifications** (`TECHNICAL_SPECS.md`):
   - Update for any architectural changes
   - Update version compatibility matrices
   - Update configuration examples
   - Update API endpoints or interfaces
   - Update security features or container changes

2. **README.md**:
   - Update for new features or capabilities
   - Update installation/setup instructions
   - Update quick start guides
   - Update feature lists and badges
   - Update roadmap items when completed

3. **Security Documentation**:
   - Update `SECURITY.md` for security policy changes
   - Update `.github/SECURITY_CHECKLIST.md` for release procedures
   - Update GitHub Actions workflows for security automation changes
   - Update security issue templates for new vulnerability types

4. **Component-Specific Documentation**:
   - Update `setagaya/JMETER_BUILD_OPTIONS.md` for JMeter-related changes
   - Update inline code comments for complex logic changes
   - Update configuration templates (`config_tmpl.json`) for new config options
   - Update `CHANGELOG.md` for all significant changes

5. **Version Information**:
   - Update version numbers in both TECHNICAL_SPECS.md and README.md
   - Update "Last Updated" timestamps in technical documentation
   - Update compatibility matrices for supported versions
   - Update security policy timestamps and version support matrices

### Documentation Update Checklist

Before completing any task, verify:

- [ ] Technical specs reflect current architecture
- [ ] README.md features list is current
- [ ] Version numbers are consistent across all docs
- [ ] New configuration options are documented
- [ ] Breaking changes are clearly noted
- [ ] Security changes are properly documented
- [ ] GitHub Actions workflows are updated for security/linting changes
- [ ] CHANGELOG.md includes all significant changes

## Architecture Overview

Setagaya is a distributed load testing platform that orchestrates JMeter engines across Kubernetes clusters. The system
follows a controller-scheduler-engine pattern:

- **Controller** (`setagaya/controller/`) - Main orchestration service managing test execution lifecycle
- **API Server** (`setagaya/api/`) - REST API for web UI and external integrations
- **Scheduler** (`setagaya/scheduler/`) - Kubernetes resource management (pods, services, ingress)
- **Engines** (`setagaya/engines/`) - Load generation executors (currently JMeter + agent sidecars)

### Core Domain Model

- **Project** → **Collection** → **Plan** → **ExecutionPlan** hierarchy
- Collections are the execution unit containing multiple plans running simultaneously
- Plans define test configurations; ExecutionPlans specify engines/concurrency per plan
- Results converge at collection level for unified reporting via Grafana dashboards

## Key Development Workflows

### Pull Request Workflow

Before creating any pull request:

1. **Validate PR Title**: Ensure it follows conventional commit format (see PR Title Requirements above)
2. **Check PR Size**: Keep PRs focused and under 1000 lines of changes when possible
3. **Run Local Tests**: Ensure all tests pass and code is properly formatted
4. **Update Documentation**: Follow documentation maintenance requirements
5. **Security Review**: Consider security impact for sensitive changes

The PR validation workflow will automatically check:
- Conventional commit title format
- PR description length and content
- Code formatting and style
- Security impact assessment
- Test coverage requirements
- Dependency license validation

### Local Development Setup

```bash
make              # Creates kind cluster, deploys all components
make expose       # Port-forwards Setagaya (8080) and Grafana (3000)
make setagaya      # Rebuilds and redeploys controller changes
make clean        # Destroys local cluster
```

### Component Build Process

The `setagaya/build.sh` script builds different targets:

- `build.sh api` - API server binary
- `build.sh controller` - Controller daemon binary
- `build.sh jmeter` - JMeter agent sidecar binary

Docker images are built via component-specific Dockerfiles and loaded into kind cluster.

### Configuration Pattern

All components use a central config system (`setagaya/config/init.go`):

- `config.json` defines runtime behavior (executors, storage, auth, etc.)
- Environment variable `env=local` switches to development mode
- Config validation and defaults applied during `init()`

## Critical Integration Points

### Object Storage Interface

The `setagaya/object_storage/` package abstracts test plan/data storage:

- Supports Nexus, GCP Buckets, and local storage backends
- Plans upload JMX files; collections upload YAML execution configs
- Storage client initialized globally as `object_storage.Client.Storage`

### Kubernetes Scheduler

`setagaya/scheduler/k8s.go` manages engine lifecycle:

- Creates namespaced deployments per collection/plan combination
- Handles node affinity, tolerations, and resource constraints
- Ingress controllers expose engine metrics endpoints
- Deployment GC runs every 15 minutes (`gc_duration` config)

### Metrics Pipeline

Real-time metrics flow: Engine → Controller → API → WebUI/Grafana

- Engines stream metrics via HTTP to controller endpoints
- Controller aggregates and forwards to Prometheus metrics
- API provides server-sent events for live dashboard updates
- Collection metrics identified by `collection_id` + `plan_id` labels

## Project-Specific Conventions

### Error Handling Pattern

```go
// Use typed errors from model package
var dbe *model.DBError
if errors.As(err, &dbe) {
    // Handle database-specific errors
}

// API layer wraps errors consistently
s.handleErrors(w, err) // Maps internal errors to HTTP status codes
```

### Database Patterns

- All models in `setagaya/model/` follow active record pattern
- MySQL migrations stored in `setagaya/db/` with timestamp prefixes
- Use `config.SC.DBC` for database connections globally
- Ownership validation via LDAP group membership (`account.MLMap`)

### Testing Lifecycle States

- **Deploy**: Creates K8s resources, engines come online
- **Trigger**: Starts load generation across all engines in collection
- **Terminate**: Stops tests, keeps engines deployed for result collection
- **Purge**: Removes all K8s resources and cleans up storage

## Authentication & Authorization

- LDAP integration for user authentication (`setagaya/auth/ldap.go`)
- Project ownership based on LDAP group membership (`owner` field)
- Admin users bypass ownership checks (`auth_config.admin_users`)
- Local dev mode: use `setagaya` as owner when `no_auth: true`

## Container Security & JMeter Compatibility

### Modern Container Architecture (2025)

All Dockerfiles use security-hardened, multi-stage builds:

- **Base Images**: `golang:1.25.1-alpine3.22@sha256:546...`, `alpine:3.22@sha256:beefd...`, scratch, or eclipse-temurin:21-jre-alpine
- **User Security**: All containers run as `setagaya` user (UID 1001)
- **Build Method**: Source compilation with Go 1.25.1 during Docker build
- **Security Flags**: CGO_ENABLED=0 with static linking (`-w -s -extldflags=-static`)
- **No HEALTHCHECK**: Eliminated to prevent OCI format warnings

### Security Automation (2025)

The platform includes comprehensive security automation:

- **GitHub Actions Security Suite**: 3 main workflows for security, quality, and PR validation
- **Continuous Monitoring**: Weekly security scans with multi-tool coverage (Gosec, CodeQL, Trivy, TruffleHog)
- **Dependency Management**: Automated security updates with Dependabot
- **Security Documentation**: Comprehensive security policies and incident response procedures
- **Emergency Response**: Automated critical vulnerability detection and escalation

### GitHub Actions Workflows

Located in `.github/workflows/`:

- `security-check.yml` - Comprehensive security scanning and SBOM generation
- `code-quality.yml` - Go linting, testing, Dockerfile validation, YAML checking
- `pr-validation.yml` - PR title validation, diff analysis, security impact assessment
- `security-monitoring.yml` - Continuous security monitoring with automated issue creation
- `security-advisory.yml` - Security advisory management and emergency response

### Security Configuration Files

Supporting configuration files for security automation:

- `.golangci.yml` - Comprehensive Go linting configuration
- `.yamllint.yml` - YAML linting standards
- `.github/dependabot.yml` - Automated dependency updates
- `.github/SECURITY_CHECKLIST.md` - 100+ point security release checklist

### JMeter Version Compatibility

The platform supports both legacy and modern JMeter versions:

#### Modern Approach (Recommended)

- **Dockerfile**: `Dockerfile.engines.jmeter`
- **JMeter Version**: 5.6.3 (latest)
- **Build**: Source compilation of setagaya-agent
- **Usage**: `docker build -f setagaya/Dockerfile.engines.jmeter .`

#### Legacy Approach (Backward Compatibility)

- **Dockerfile**: `Dockerfile.engines.jmeter.legacy`
- **JMeter Version**: 3.3 (legacy)
- **Build**: Pre-built setagaya-agent binary
- **Prerequisites**: Run `./build.sh jmeter` before building
- **Usage**: `docker build -f setagaya/Dockerfile.engines.jmeter.legacy .`

#### Agent Version Compatibility

The `setagaya-agent` automatically detects JMeter paths:

- **Environment Detection**: Uses `JMETER_BIN` environment variable
- **Fallback**: Hardcoded JMeter 3.3 paths for backward compatibility
- **Dynamic Paths**: `JMETER_EXECUTABLE` and `JMETER_SHUTDOWN` set via `init()`

## Common Development Patterns

When adding new schedulers: Implement `scheduler.EngineScheduler` interface When adding storage backends: Implement
`object_storage.Storage` interface
When adding engine types: Follow `setagaya/engines/jmeter/` structure with agent sidecar pattern When modifying API
endpoints: Update both `api/main.go` routes and ownership validation middleware When updating container builds: Ensure
both README.md and TECHNICAL_SPECS.md reflect changes

## Development Principles for Future Improvements

### Simplicity First

- Always choose the simplest possible approach that solves the problem
- Prefer composition over inheritance, clear interfaces over complex implementations
- Follow the principle: "Make it work, make it right, make it fast" - in that order
- Avoid premature optimization and over-engineering

### Test-Driven Development (TDD)

- Write tests first, especially for new features in `setagaya/model/` and `setagaya/controller/`
- Follow existing test patterns in `*_test.go` files (e.g., `setagaya/model/collection_test.go`)
- Use `setagaya/model/test_utils.go` for database test setup and teardown
- Test at appropriate levels: unit tests for models, integration tests for scheduler interactions

### Domain-Driven Design (DDD)

- Respect the existing domain boundaries: Project → Collection → Plan → ExecutionPlan
- Keep business logic in domain models (`setagaya/model/`), not in API or controller layers
- Use ubiquitous language: "deploy engines", "trigger collection", "purge resources"
- Aggregate roots should control access to their entities (e.g., Collection manages ExecutionPlans)

### Security Best Practices

- Always validate ownership before operations using `hasProjectOwnership()` and `hasCollectionOwnership()`
- Sanitize file uploads in plan/collection file handlers
- Use parameterized queries in all database operations (existing pattern in models)
- Validate LDAP group membership for authorization (`account.MLMap` checks)
- Follow least-privilege principle for Kubernetes RBAC configurations
- Never log sensitive data (passwords, tokens) in controller or API layers

## Key Files for Understanding

- `setagaya/main.go` - Application entry point and HTTP routing
- `setagaya/config/init.go` - Configuration loading and validation
- `setagaya/controller/main.go` - Core orchestration logic and metrics handling
- `setagaya/scheduler/k8s.go` - Kubernetes resource management
- `setagaya/model/` - Domain models and database interactions
- `TECHNICAL_SPECS.md` - Comprehensive technical documentation
- `README.md` - Project overview and quick start guide
