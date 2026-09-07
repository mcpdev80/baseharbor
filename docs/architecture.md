# BaseHarbor architecture

BaseHarbor is a secure, modular, self-hosted application backend runtime. It manages backend infrastructure for independent applications without forcing those applications into a BaseHarbor-specific SDK or hosting model.

## Product boundary

BaseHarbor is not a general-purpose PaaS and does not compete by rebuilding Git deployment, buildpacks, source builds or frontend hosting. Applications can run anywhere. BaseHarbor manages the backend services those applications depend on.

```text
Application
   │
   ├── PostgreSQL        native protocol/client
   ├── Redis/Valkey      native protocol/client
   ├── Object Storage    S3-compatible API
   ├── Identity          OIDC/OAuth2
   ├── Secrets           OpenBao/Vault-compatible API
   └── Telemetry         OpenTelemetry/OpenMetrics
            │
            ▼
       BaseHarbor
```

## Control plane and application stacks

The BaseHarbor control plane is shared. Application data-plane resources are isolated by default.

```text
BaseHarbor control plane
├── baha CLI
├── platform metadata
├── platform secrets / trust bootstrap
├── lifecycle and convergence engine
└── shared observability infrastructure

Application: mailflow
├── isolated network
├── dedicated PostgreSQL
├── dedicated Redis/Valkey
├── isolated secret scope or optional dedicated secret service
└── application-owned data

Application: awc
├── isolated network
├── dedicated PostgreSQL
├── dedicated Redis/Valkey
└── isolated secret scope
```

A dedicated application service does not imply a dedicated physical host. On Compose it normally means separate containers, networks and volumes. Later deployment engines can map the same desired state to other runtimes.

## Principles

1. One operational entry point through the `baha` binary.
2. Applications remain independent and keep all business/domain logic.
3. Native protocols are preferred where they already provide a stable ecosystem boundary.
4. Secure defaults, least privilege, deny by default and fail closed.
5. Mature open-source components are composed instead of reimplemented.
6. Desired state is validated before mutation and verified after mutation.
7. Container-running is not equivalent to service-ready.
8. Secrets never belong in application manifests.
9. Single-node Compose deployments are first-class; Kubernetes is optional and later.
10. Observability is integrated through open standards rather than a proprietary telemetry stack.

## Application lifecycle model

BaseHarbor follows an explicit convergence flow:

```text
Manifest
   ↓
Validate
   ↓
Resolve dependencies
   ↓
Preflight
   ↓
Build desired-state plan
   ↓
Mutate
   ↓
Verify actual state
   ↓
Ready / Failed truthfully
```

The current application manifest is intentionally small:

```yaml
version: 1
app:
  name: demo
  environment: dev
services:
  postgres:
    enabled: true
  redis:
    enabled: false
  secrets:
    enabled: false
```

## `baha` as the primary product interface

`baha` is not a thin wrapper around Compose. It is the stable operator interface for BaseHarbor lifecycle, diagnostics and application resources.

Command families are hierarchical and self-documenting:

```text
baha --help
baha app --help
baha app create --help
```

CLI guarantees:

- one binary
- stable command hierarchy
- actionable help and errors
- exit code 0 for success
- exit code 1 for operational/runtime failure
- exit code 2 for invalid command usage
- read-only `plan` and `preflight` before future mutation commands

## Security and operations capabilities

The target platform capabilities include:

- OIDC/OAuth2 identity and authorization
- tenant-aware control-plane metadata and PostgreSQL RLS
- OpenBao-backed secrets and PKI
- ACME, internal PKI and imported/BYOC certificates
- backup and verified restore
- lifecycle audit and retention
- metrics, logs and traces through OpenTelemetry/OpenMetrics-compatible pipelines
- health/readiness/doctor semantics
- upgrade and rollback safety
- optional jobs, realtime, MCP, RAG and AI integrations after the runtime foundation is stable

## Non-goals

BaseHarbor does not aim to implement its own database, cache protocol, secret store, OIDC protocol, S3 protocol, ACME server, monitoring database or deployment PaaS. It also does not own application schemas or business data models.
