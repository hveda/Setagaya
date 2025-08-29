# Shibuya Load Testing Platform - Sequence Diagram

This sequence diagram shows the complete workflow of how Shibuya orchestrates load testing from project creation to test execution and results collection.

## PlantUML Sequence Diagram

```plantuml
@startuml Shibuya Load Testing Workflow

actor User
participant "UI (Vue.js)" as UI
participant "API Server" as API
participant "Controller" as Controller
participant "Database" as DB
participant "Object Storage" as Storage
participant "Scheduler" as Scheduler
participant "Kubernetes" as K8s
participant "JMeter Engine" as Engine
participant "JMeter Agent" as Agent
participant "Prometheus" as Prometheus
participant "Grafana" as Grafana

== Project Setup ==
User -> UI: Create Project
UI -> API: POST /api/projects
API -> DB: Store project metadata
API -> UI: Project created
UI -> User: Show project details

User -> UI: Create Plan
UI -> API: POST /api/plans
API -> DB: Store plan metadata
API -> UI: Plan created

User -> UI: Upload JMX test file
UI -> API: PUT /api/plans/{id}/files
API -> Storage: Store JMX file
API -> UI: File uploaded

User -> UI: Upload test data (CSV)
UI -> API: PUT /api/plans/{id}/files
API -> Storage: Store test data
API -> UI: Data uploaded

== Collection Configuration ==
User -> UI: Create Collection
UI -> API: POST /api/collections
API -> DB: Store collection metadata
API -> UI: Collection created

User -> UI: Configure Execution Plans
note right: Define engines, concurrency,\nduration for each plan
UI -> API: PUT /api/collections/{id}/config
API -> DB: Store execution configuration
API -> UI: Configuration saved

== Deployment Phase ==
User -> UI: Deploy Collection
UI -> API: POST /api/collections/{id}/deploy
API -> Controller: DeployCollection()
Controller -> Scheduler: Deploy engines
Scheduler -> K8s: Create JMeter pods
K8s -> Engine: Start JMeter + Agent containers
Engine -> Agent: Initialize agent process
Agent -> Controller: Subscribe for commands
Controller -> API: Deployment started
API -> UI: Engines deploying
UI -> User: Show deployment status

== Test Execution ==
User -> UI: Trigger load test
UI -> API: POST /api/collections/{id}/trigger
API -> Controller: TriggerCollection()
Controller -> Agent: POST /start (with test config)
Agent -> Storage: Download JMX and data files
Agent -> Engine: Start JMeter process
Engine -> Agent: Stream test metrics
Agent -> Controller: Stream real-time metrics
Controller -> Prometheus: Export metrics
Controller -> API: Stream metrics via SSE
API -> UI: Real-time test data
UI -> User: Live test dashboard

Prometheus -> Grafana: Pull metrics
Grafana -> User: Advanced analytics dashboard

== Test Monitoring ==
loop During test execution
    Engine -> Agent: JMeter progress/results
    Agent -> Controller: Metrics stream
    Controller -> Prometheus: Update metrics
    Controller -> UI: Real-time updates via SSE
    UI -> User: Live progress display
end

== Test Completion ==
Engine -> Agent: Test finished
Agent -> Controller: Final results
Controller -> DB: Store run history
Controller -> API: Test completed
API -> UI: Test finished
UI -> User: Show final results

== Cleanup ==
User -> UI: Stop/Purge Collection
UI -> API: POST /api/collections/{id}/purge
API -> Controller: TermAndPurgeCollection()
Controller -> Agent: Stop JMeter
Controller -> Scheduler: Delete pods
Scheduler -> K8s: Remove engine deployments
K8s -> Engine: Terminate containers
Controller -> API: Cleanup completed
API -> UI: Resources cleaned up
UI -> User: Collection purged

@enduml
```

## Key Components Interaction

### 1. Web Interface Layer
- **Vue.js UI**: Provides user-friendly interface for managing projects, collections, and plans
- **Real-time updates**: Uses Server-Sent Events (SSE) for live test monitoring

### 2. API Layer
- **REST API**: Handles CRUD operations for all entities
- **Authentication**: Integrates with LDAP for user permissions
- **File handling**: Manages upload/download of JMX files and test data

### 3. Orchestration Layer
- **Controller**: Central coordinator that manages engine lifecycle
- **Scheduler**: Abstracts Kubernetes/Cloud Run operations
- **Engine Management**: Tracks connected engines and their states

### 4. Execution Layer
- **JMeter Engines**: Run actual load tests in isolated containers
- **Shibuya Agent**: Sidecar process that manages JMeter and streams metrics
- **Kubernetes**: Provides scalable container orchestration

### 5. Data Layer
- **Database**: Stores metadata for projects, collections, plans, and runs
- **Object Storage**: Holds JMX files, test data, and artifacts
- **Prometheus**: Collects and stores real-time test metrics

## Workflow Phases

1. **Setup Phase**: Create organizational structure (projects) and test definitions (plans)
2. **Configuration Phase**: Group plans into collections with execution parameters
3. **Deployment Phase**: Provision JMeter engines in Kubernetes cluster
4. **Execution Phase**: Run distributed load tests with real-time monitoring
5. **Analysis Phase**: View results in Grafana dashboards and UI
6. **Cleanup Phase**: Terminate engines and free cluster resources

## Key Features

- **Distributed Testing**: Scales beyond single JMeter instance limitations
- **Real-time Monitoring**: Live metrics streaming during test execution
- **Resource Management**: Automatic provisioning and cleanup of test infrastructure
- **Multi-tenancy**: Project-based isolation with LDAP authentication
- **Cloud Native**: Designed for Kubernetes with support for multiple cloud providers