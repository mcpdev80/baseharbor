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

## Current v0.3.0 Compose foundation

The current complete runtime target is Docker/Podman Compose.

Implemented foundations include:

- `baha` as the primary lifecycle CLI;
- repository-owned `baseharbor.yaml` application contracts;
- detect-first guided `baha app init`, `--quick` and deterministic explicit flags;
- one or multiple named PostgreSQL resources;
- one or multiple named Valkey/Redis-protocol resources;
- explicit workload-only repository applications without artificial managed backend dependencies;
- OpenBao-backed required/generated secrets and scoped runtime identity;
- standard environment/file bindings and optional app-scoped runtime secret references;
- application workload attachment through generated Compose overrides;
- isolated application backend networks where managed backends exist;
- trusted-local developer access through database/cache clients, logs, shell and exec;
- health-aware service-level workload truth plus app-owned HTTP/HTTPS exposure readiness;
- coherent `show`, `status` and `doctor` operator views;
- guided encrypted backup/restore with verified recovery metadata and fail-closed post-restore readiness;
- strict fast-forward Git-backed application update with explicit recovery policy for durable state;
- guarded BaseHarbor self-update with artifact verification, atomic replacement and rollback;
- repository deployment initialization for public FQDN and TLS mode;
- existing/BYOC certificate validation, protected installation, update checking and reload verification;
- automatic persisted host-port fallback for configurable Compose publishers, including IPv4/IPv6 bind-conflict forms;
- release/runtime-image version coupling and real-product acceptance coverage including MailFlow.

Multiple logical service instances are not HA. HA is future topology behind one stable logical resource.

## Architecture rule for future work

Compose is not disposable prototype code; it remains a first-class provider. At the same time, Compose-specific details must not leak into portable application requirements.

Provider-specific implementation details include:

- Compose project/network/container names;
- allocated host ports and generated overrides;
- deployment FQDN/TLS realization for the current provider;
- Kubernetes object names and namespace mechanics;
- OpenShift Routes/SCC-specific realization;
- provider-specific storage and secret projection mechanics.

Applications should continue to consume stable interfaces such as PostgreSQL, Redis/Valkey, S3, OIDC/OAuth2, OpenBao/Vault-compatible secrets and OpenTelemetry/OpenMetrics.

## v0.3 boundary: implemented versus future

v0.3 deliberately implements only the current Compose realization needed for a complete local/self-hosted operational lifecycle.

Implemented now:

- application-owned HTTP/HTTPS exposure verification;
- deployment public FQDN persisted as protected runtime state;
- existing/BYOC certificate lifecycle for repository deployments;
- local Compose workload port fallback;
- backup/restore and update verification against the real application boundary.

Still future:

- provider-neutral `ingress.http` or `tls.certificate` application capabilities;
- BaseHarbor-managed ACME issuance/renewal;
- OpenBao PKI issuance/rotation for application ingress certificates;
- Kubernetes Gateway/Ingress and OpenShift Route realization;
- topology/HA profiles;
- managed-production OIDC/RBAC/JIT policy.

## Next architecture tracks

The post-v0.3 roadmap remains split into independent tracks rather than one large rewrite.

### Portable application contract and provider seam

- evolve the application contract without baking in Compose-only assumptions;
- move from product-oriented v1 fields toward capability-oriented requirements where justified;
- define capability negotiation and fail-closed provider selection;
- separate application requirements from operator/environment policy;
- preserve the v0.3 Compose developer journey while introducing the runtime/provider seam incrementally.

### Input resolution

- declarative inputs, defaults, generated values and conditional questions;
- one resolver usable by CLI, future GUI/API and automation;
- explicit ownership of app requirements versus deployment/operator inputs;
- no universal application business-configuration framework.

### Developer access and managed policy

Trusted-local access is implemented in v0.3. Future work adds stricter policy without changing the application runtime contract:

- environment-aware access policy;
- future OIDC login, RBAC, audit and just-in-time/elevated production access;
- raw secret reveal becoming exceptional in managed production deployments;
- policy-controlled access to logs, shells, credentials and destructive lifecycle actions.

### Exposure, TLS and PKI

Existing/BYOC certificate lifecycle and app-owned HTTP/TLS readiness are implemented for Compose v0.3. Future work includes:

- public/internal exposure as an explicit portable platform capability;
- provider-neutral ingress/gateway intent;
- ACME lifecycle managed by the selected provider;
- internal OpenBao PKI issuance where appropriate;
- automatic renewal/rotation and health policy;
- Kubernetes cert-manager / Gateway integrations and OpenShift-native realization.

### Capability providers

Default products remain replaceable implementation choices. Planned tracks include provider boundaries for:

- relational SQL;
- cache/key-value;
- secrets;
- S3-compatible object storage;
- ingress/exposure;
- identity;
- observability.

Substitution must preserve the requested contract or fail clearly; BaseHarbor must not silently downgrade security, durability or availability.

### Runtime provider expansion

The intended provider evolution is:

```text
Application Contract
        ↓
Runtime / Deployment Provider
   ┌─────────┼─────────────┐
 Compose   Kubernetes    OpenShift
```

Kubernetes and OpenShift are future providers, not v0.3.0 features. They should map the same logical application resources and lifecycle concepts to their native primitives rather than requiring a separate application model.

### Environment, identity and topology profiles

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
