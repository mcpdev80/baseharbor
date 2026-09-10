# BaseHarbor roadmap

BaseHarbor is a developer-first application platform foundation for independent applications.

The product goal is a continuous operational path from a first local or homelab deployment to production and, where required, Kubernetes/OpenShift enterprise operation without forcing the application to adopt a new logical contract at each stage.

```text
idea / local development
        ↓
homelab / single host
        ↓
Compose production
        ↓
future Kubernetes / OpenShift
        ↓
enterprise deployment profiles
```

The application declares logical requirements. BaseHarbor resolves, provisions, secures and operates those requirements through the selected runtime/deployment provider while applications continue to use standard protocols and native clients.

## Current v0.2.0 foundation

The current complete runtime target is Docker/Podman Compose.

Implemented foundations include:

- `baha` as the primary lifecycle CLI;
- repository-owned `baseharbor.yaml` application contracts;
- project-aware `baha app init` and the one-command `baha up` happy path;
- one or multiple named PostgreSQL resources;
- one or multiple named Valkey/Redis-protocol resources;
- OpenBao-backed required/generated secrets;
- standard environment and file bindings;
- application workload attachment through generated Compose overrides;
- isolated application backend networks;
- scoped runtime identity and mTLS secret broker;
- status, doctor and recovery behavior;
- encrypted application backup/restore;
- release/runtime-image version coupling and real MailFlow acceptance coverage.

Multiple logical service instances are not HA. HA is future topology behind one stable logical resource.

## Architecture rule for future work

Compose is not disposable prototype code; it remains a first-class provider. At the same time, Compose-specific details must not leak into portable application requirements.

Provider-specific implementation details include:

- Compose project/network/container names;
- allocated host ports and generated overrides;
- Kubernetes object names and namespace mechanics;
- OpenShift Routes/SCC-specific realization;
- provider-specific storage and secret projection mechanics.

Applications should continue to consume stable interfaces such as PostgreSQL, Redis/Valkey, S3, OIDC/OAuth2, OpenBao/Vault-compatible secrets and OpenTelemetry/OpenMetrics.

## Next architecture tracks

The post-v0.2 roadmap is intentionally split into independent tracks rather than one large rewrite.

### Application contract and input resolution

- evolve the application contract without baking in Compose-only assumptions;
- declarative inputs, defaults, generated values and conditional questions;
- one resolver used by CLI, future GUI/API and automation;
- clear separation between app requirements and operator/platform policy.

### Developer access

- convenient access to managed resources through `baha`;
- standard database/cache shells, logs, exec and credential handoff;
- environment-aware behavior without making applications depend on BaseHarbor-specific runtime APIs.

### Environment, identity and policy

- development remains fast and low-friction;
- test/staging/production can apply progressively stricter access policy;
- future OIDC login, RBAC, audit and just-in-time/elevated production access;
- raw secret reveal becomes exceptional in managed production deployments.

### Exposure, TLS and PKI

- public/internal exposure as a platform capability;
- ACME, internal OpenBao PKI and existing/BYOC certificates;
- certificate discovery, validation, rotation and health checks.

### Runtime provider expansion

The intended provider evolution is:

```text
Application Contract
        ↓
Runtime / Deployment Provider
   ┌─────────┼─────────────┐
 Compose   Kubernetes    OpenShift
```

Kubernetes and OpenShift are future providers, not v0.2.0 features. They should map the same logical application resources and lifecycle concepts to their native primitives rather than requiring a separate application model.

### Deployment profiles and HA

Environment/risk and topology are separate dimensions.

A future deployment may combine, for example:

```text
environment: production
deployment profile: enterprise-ha
provider: openshift
```

while the application continues to request one logical `primary` PostgreSQL resource.

Future profiles may include:

- single-node/standard;
- HA control plane;
- replicated/managed service topologies;
- Kubernetes/OpenShift enterprise policy integration;
- external managed PostgreSQL/Valkey/S3/OpenBao adapters;
- backup/DR and observability requirements.

## Long-term success criterion

BaseHarbor succeeds when an application can start with a developer saying:

```text
"I am quickly building something."
```

and later reach:

```text
"This now has to run for an enterprise customer on Kubernetes/OpenShift."
```

without a second operational rewrite of the application.

The exact infrastructure may change substantially; the logical application requirements and standard application-facing interfaces should change as little as possible.
