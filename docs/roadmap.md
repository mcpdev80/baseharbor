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
Kubernetes / OpenShift
        ↓
enterprise deployment profiles
```

The application declares logical requirements. BaseHarbor resolves, provisions, secures and operates those requirements through the selected runtime and capability providers while applications continue to use standard protocols and native clients.

## Current v0.4.1 portable application foundation

Docker/Podman Compose remains the complete runtime implementation. v0.4 adds the architecture seams required to evolve beyond it without redefining the application contract.

Implemented foundations include:

- `baha` as the primary lifecycle CLI;
- repository-owned Manifest v1 `baseharbor.yaml` as the supported compatibility surface;
- a provider-neutral `PortableContract` adapter for application intent;
- a shared capability/provider/resource/binding domain core with fail-closed provider negotiation and machine-readable lifecycle results;
- one or multiple named logical PostgreSQL resources;
- one or multiple named logical Valkey/Redis-protocol resources;
- managed/generated secret intent without embedding secret values in the application contract;
- explicit separation between application requirements, runtime-provider selection and capability-provider/product selection;
- deployment-owned runtime-provider state with Compose as the current provider;
- runtime provider capability negotiation and fail-closed unsupported-provider behavior;
- centralized runtime guards preventing future provider selections from falling through into Compose-specific application operations;
- declarative input resolution with default, generated, external and conditional values;
- resolver-driven deployment inputs shared by `baha app init` and repository-aware `baha up`;
- automation-safe explicit non-secret input injection through `--input NAME=VALUE`;
- detect-first guided application initialization, `--quick` and deterministic explicit flags;
- explicit workload-only repository applications without artificial managed backend dependencies;
- OpenBao-backed required/generated secrets and scoped runtime identity;
- standard environment/file bindings and optional app-scoped runtime secret references;
- application workload attachment through generated Compose overrides;
- trusted-local developer access through database/cache clients, logs, shell and exec;
- health-aware workload and HTTP/HTTPS exposure readiness;
- coherent `show`, `status` and `doctor` operator views;
- guided encrypted backup/restore with verified recovery metadata;
- strict fast-forward application updates and guarded BaseHarbor self-update;
- public-FQDN/TLS deployment initialization and existing/BYOC certificate lifecycle;
- automatic persisted host-port fallback for configurable Compose publishers;
- release/runtime-image version coupling and real-product acceptance coverage including MailFlow.

Multiple logical service instances are not HA. HA remains a topology/availability concern behind stable logical resources.

## Architecture rules

Compose is not disposable prototype code; it remains a first-class runtime provider. Compose-specific details must not leak into portable application requirements.

Runtime providers and capability providers are independent axes. For example, a future OpenShift deployment may still use customer-managed PostgreSQL, Vault/OpenBao and Ceph RGW rather than requiring platform-native products for every capability.

Provider-specific implementation details include:

- Compose project/network/container names;
- allocated host ports and generated overrides;
- deployment FQDN/TLS realization for the current provider;
- Kubernetes object names, namespaces and storage classes;
- OpenShift Routes/SCC-specific realization;
- provider-specific storage, secret projection and ingress mechanics.

Applications should continue to consume stable interfaces such as PostgreSQL, Redis/Valkey, S3, OIDC/OAuth2, Vault/OpenBao-compatible secrets and OpenTelemetry/OpenMetrics.

Provider substitution must satisfy the requested contract or fail clearly. BaseHarbor must never silently downgrade requested security, durability or availability.

BaseHarbor also has one shared domain/lifecycle core with multiple control surfaces. `baha`, a future HTTP API/Web UI and a future Kubernetes/OpenShift Operator must reuse the same plan, validation, lifecycle, readiness, diagnostics and policy semantics rather than becoming separate implementations.

## v0.4 boundary: implemented versus future

Implemented in v0.4:

- Manifest v1 compatibility classification and one-way portable-contract translation;
- contract versioning/evolution rules;
- runtime-provider identity, state and capability negotiation;
- Compose as the current runtime provider implementation;
- deployment-selected application runtime guards;
- reusable declarative input resolution;
- shared TLS/FQDN deployment input reference flow;
- preserved v0.3 Compose developer journey and persisted-state compatibility.

Still future:

- additional capability-provider implementations and S3/object-storage realization;
- provider-neutral ingress/TLS capability implementation;
- BaseHarbor-managed ACME issuance/renewal;
- OpenBao PKI issuance/rotation for application ingress certificates;
- managed environment/policy profiles;
- topology/HA profiles;
- managed-production OIDC/RBAC/JIT policy;
- actual Kubernetes and OpenShift runtime implementations.

## Next architecture tracks

### v0.5 – Compose platform capabilities

Expand the portable capability model while keeping Compose as the production implementation:

- capability-provider boundaries for relational SQL, cache/key-value, secrets, S3-compatible object storage and exposure;
- replaceable reference providers rather than product lock-in;
- provider conformance and explicit capability negotiation;
- object storage reference implementation, with SeaweedFS/Ceph RGW/external S3-style providers evaluated behind the same logical contract;
- further provider-neutral exposure/TLS intent without leaking Compose details.

### v0.6 – Environments, policy, identity and topology intent

Add platform/operator policy while keeping it outside the application contract. This is also the natural phase for the first remote/API management surface and a lightweight Web UI backed by the shared core:

- named environment profiles and server-side policy;
- runtime/capability provider selection per environment;
- OIDC login, RBAC, audit and just-in-time/elevated production access where required;
- topology intent such as standard versus enterprise/HA without changing logical application resource identity;
- external/customer-managed provider bindings;
- stable machine-readable BaseHarbor API for application/platform operations;
- lightweight Web UI for plan/apply/status/doctor/logs/inputs/backup/restore/update without duplicating lifecycle logic;
- shared authorization/policy boundaries for CLI, API and Web UI.

### v0.7 – Kubernetes provider

Map the same portable application requirements to Kubernetes primitives where applicable and introduce the BaseHarbor Operator as the cluster-native control surface over the same shared core:

- Deployments and StatefulSets;
- Services;
- Gateway/Ingress;
- PVCs/storage classes;
- workload secret delivery/provider integration;
- NetworkPolicies;
- readiness/liveness probes;
- PodDisruptionBudgets where required by topology/policy;
- provider-conformance and migration tests;
- BaseHarbor CRDs/controller reconciliation;
- shared status/condition mapping from BaseHarbor readiness and diagnostics;
- `baha`/API interaction with cluster-managed applications without wrapping imperative CLI commands inside the Operator.

Applications keep the same `baha` lifecycle and logical resources rather than gaining a second Kubernetes-specific operational contract.

### v0.8 – OpenShift / enterprise provider

Add OpenShift-specific behavior where Kubernetes-generic mapping is insufficient:

- Routes/Gateway integrations;
- SCC/security constraints;
- Operator integrations where appropriate;
- OpenShift identity/policy integration points;
- enterprise registry, proxy and offline constraints;
- customer-managed infrastructure capability providers.

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

The infrastructure may change substantially; the logical application requirements and standard application-facing interfaces should change as little as possible.