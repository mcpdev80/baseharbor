# BaseHarbor architecture

BaseHarbor is a secure, modular, self-hosted application platform foundation. It manages backend infrastructure and application lifecycle for independent applications without forcing those applications into a BaseHarbor-specific SDK.

The long-term lifecycle target is continuous growth from local development and homelab deployments to stricter production environments and, where needed, Kubernetes/OpenShift enterprise deployments without redefining the logical application contract.

## Product boundary

BaseHarbor is developer-first, but it is not a source-code build service or proprietary application framework. Applications keep their own source, business logic and native ecosystem interfaces. BaseHarbor owns the operational realization around them: managed dependencies, lifecycle, isolation, security boundaries, recovery and provider-specific deployment mechanics.

```text
Application contract
        |
        v
BaseHarbor lifecycle / policy / provisioning
        |
        +-- Compose (current complete provider)
        +-- Kubernetes (future provider)
        +-- OpenShift (future enterprise provider)
```

The application should continue to consume standard interfaces such as PostgreSQL, Redis/Valkey, S3, OIDC/OAuth2, OpenBao/Vault-compatible secrets and OpenTelemetry/OpenMetrics rather than BaseHarbor-specific data protocols.

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
```

A dedicated application service does not imply a dedicated physical host. On Compose it normally means separate containers, networks and volumes. Later deployment engines can map the same logical desired state to other runtimes.

## Identity versus deployment context

`app.name` is the stable logical application identity. `app.environment` is deployment context. The same application can exist simultaneously in development, test, staging, production and customer-specific environments.

Provider/runtime identities may include the environment to preserve isolation, but applications must not depend on those generated names.

## Principles

1. One operational entry point through the `baha` binary.
2. Applications remain independent and keep all business/domain logic.
3. Native protocols are preferred where they already provide a stable ecosystem boundary.
4. Secure defaults, least privilege, deny by default and fail closed.
5. Mature open-source components are composed instead of reimplemented.
6. Desired state is validated before mutation and verified after mutation.
7. Container-running is not equivalent to service-ready.
8. Secrets never belong in application manifests.
9. Compose is the complete current provider and remains first-class; future Kubernetes/OpenShift providers must preserve logical application requirements rather than redefine them.
10. Environment/risk policy and deployment topology are separate concepts.
11. Observability is integrated through open standards rather than a proprietary telemetry stack.

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
Mutate through current provider
   ↓
Verify actual state
   ↓
Ready / Failed truthfully
```

The current v0.2.0 application manifest is intentionally small and Compose-focused:

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

Provider-specific details such as Compose project names, networks, host ports, generated overrides, Kubernetes resource names or OpenShift Routes are implementation details, not portable application requirements.

## `baha` as the primary product interface

`baha` is not a thin wrapper around Compose. It is the stable operator/developer interface for BaseHarbor lifecycle, diagnostics and application resources. Compose is the current implementation target behind that interface.

## Security and operations direction

The target platform capabilities include:

- environment-aware access policy
- OIDC/OAuth2 identity and authorization for managed/enterprise deployments
- OpenBao-backed secrets and PKI
- ACME, internal PKI and imported/BYOC certificates
- backup and verified restore
- lifecycle audit and retention
- metrics, logs and traces through OpenTelemetry/OpenMetrics-compatible pipelines
- health/readiness/doctor semantics
- upgrade and rollback safety
- standard and HA deployment profiles without changing logical resource identity

These are future capabilities unless explicitly documented as implemented in the current release.

## Non-goals

BaseHarbor does not aim to implement its own database, cache protocol, secret store, OIDC protocol, S3 protocol, ACME server or monitoring database. It also does not own application schemas or business data models.
