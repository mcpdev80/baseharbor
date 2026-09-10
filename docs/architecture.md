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

## Capability versus product

The portable application contract describes capabilities, not infrastructure product choices. Provider selection belongs to the environment/platform side.

```text
Application Contract
        ↓
Capabilities
        ↓
Environment / Policy
        ↓
Provider selection
        ↓
Concrete product/runtime
```

For example, an application may require a SQL database, S3-compatible object storage and secrets without depending on whether an environment realizes them through PostgreSQL, SeaweedFS and OpenBao or through customer-managed PostgreSQL, Ceph RGW and Vault.

This is a hard architecture rule. Every bundled/default component must have a provider boundary and a documented replacement path. See [Capability and provider model](capability-provider-model.md) and ADR [0005](decisions/0005-capabilities-not-products.md).

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
3. Application contracts describe capabilities, not concrete infrastructure products.
4. Provider selection is environment/platform-owned and replaceable.
5. Every default BaseHarbor component has a defined provider boundary and documented replacement path.
6. Native protocols are preferred where they already provide a stable ecosystem boundary.
7. Secure defaults, least privilege, deny by default and fail closed.
8. Mature open-source components are composed instead of reimplemented.
9. Desired state is validated before mutation and verified after mutation.
10. Container-running is not equivalent to service-ready.
11. Secrets never belong in application manifests.
12. Compose is the complete current provider and remains first-class; future Kubernetes/OpenShift providers must preserve logical application requirements rather than redefine them.
13. Environment/risk policy and deployment topology are separate concepts.
14. Observability is integrated through open standards rather than a proprietary telemetry stack.

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

The product-oriented field names that exist in Manifest v1 are the current pre-v1 Compose contract; they do not override the capability/provider rule for future contract evolution.

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
