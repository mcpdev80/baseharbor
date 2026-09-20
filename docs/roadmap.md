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

## Current v0.4.8 portable application foundation

Docker/Podman Compose remains the complete runtime implementation. v0.4 adds the architecture seams required to evolve beyond it without redefining the application contract.

Implemented foundations include:

- `baha` as the primary lifecycle CLI;
- repository-owned Manifest v1 `baseharbor.yaml` as the supported compatibility surface;
- a provider-neutral `PortableContract` adapter for application intent;
- a shared capability/provider/resource/binding domain core with fail-closed provider negotiation and machine-readable lifecycle results;
- a provider registry with shared, application-scoped and external/BYO placement plus explicit lifecycle ownership;
- `metrics/v1` with Prometheus 3.14.0 as the first Compose reference provider, shared/application placement, isolated metrics networks and real scrape/ingestion verification;
- explicit directional cross-application connectivity through `baha connect`, separate from provider sharing and realized in Compose through a hardened BaseHarbor relay;
- Provider Integration Contract v1 as the mandatory boundary for subsequent providers, with versioned Capability Specifications and future gRPC/Protobuf + OCI external-provider direction;
- one or multiple named logical PostgreSQL resources;
- one or multiple named logical Valkey/Redis-protocol resources;\n- one or multiple logical S3 buckets through `object-storage.s3/v1`;\n- SeaweedFS as the current lazy shared Compose S3 reference provider with bucket-scoped credentials and authenticated Put/Get readiness;
- provider-neutral `telemetry.otlp/v1` export binding with standard OpenTelemetry workload configuration;
- OpenTelemetry Collector as the current lazy shared Compose OTLP reference provider plus external OTLP endpoint binding;
- real OTLP HTTP/protobuf export verification without implicit Prometheus/Loki/Tempo/Grafana provisioning;
- managed/generated secret intent without embedding secret values in the application contract;
- provider-neutral `secure-binding/v1` semantics for workload identity, credential/trust/secret references, least-privilege authorization metadata and security lifecycle declarations;
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
- shared logical endpoint/exposure semantics for application-owned publishers and managed exposure;
- explicit provider-neutral `exposure.http/v1` intent with application-scoped Caddy as the current Compose reference provider;
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

- additional capability-provider implementations and additional S3/object-storage providers/provider-selection policy;\n- BaseHarbor recovery of object-storage contents (v0.4.6 backup/restore fails closed for S3 applications instead of claiming incomplete recovery);
- additional traffic/exposure providers and broader provider-neutral TLS/certificate lifecycle beyond the current `exposure.http/v1` + existing/BYOC path;
- BaseHarbor-managed ACME issuance/renewal;
- OpenBao PKI issuance/rotation for application ingress certificates;
- managed environment/policy profiles;
- topology/HA profiles;
- managed-production OIDC/RBAC/JIT policy;
- actual Kubernetes and OpenShift runtime implementations.

## Provider integration prerequisite before v0.4.3+

Before additional provider/capability implementations are added, BaseHarbor standardizes the provider boundary through Provider Integration Contract v1.

Every subsequent reference provider must become evidence for the same open ecosystem contract a future vendor/community provider can implement. BaseHarbor will continue to build the important providers itself; vendor participation is not assumed.

The prerequisite deliberately does not implement a dynamic plugin loader. It defines versioned capability semantics, provider lifecycle metadata, conformance and the future language-neutral/registry-neutral boundary first.

## v0.4.3 repository inspection

v0.4.3 makes repository inspection a shared, read-only core capability.

- `baha app inspect [PATH]` exposes human-readable evidence.
- `--json` exposes the same structured result for future control surfaces.
- evidence is classified as Detected / Suggested / Possible;
- detectors are registered behind a shared detector interface so later capability releases can extend inference incrementally;
- current PostgreSQL/Valkey detection and guided `app init` reuse the same engine;
- inspection never mutates repository/runtime state and never emits environment secret values.

## v0.4.8 metrics / Prometheus track

The continuous application-evolution and runtime-resource prerequisites are complete. v0.4.8 adds the first metrics collection/storage provider while preserving the application/provider split.

Implemented in the v0.4.8 development track:

- versioned `metrics/v1` Capability Specification;
- provider-neutral application metrics source declarations using logical source name, workload service, target port and path;
- OpenMetrics-compatible HTTP exposition as the v1 signal format;
- repository inspection maps conventional `/metrics` evidence to the canonical `metrics` capability;
- deployment-owned collection policy instead of a portable `prometheus: true` requirement;
- development collection enabled by default, with test/staging/production requiring explicit operator opt-in;
- Prometheus 3.14.0 as the first lazy shared Compose reference provider;
- file-based automatic target discovery generated from BaseHarbor state, without manual Prometheus target editing;
- per-application isolated metrics networks with explicit Prometheus attachment only to registered application trust boundaries;
- deterministic collision-resistant target DNS aliases so identical service names across applications remain isolated;
- BaseHarbor application/environment/service/source attribution on scraped series;
- readiness based on a real successful scrape visible as `up=1`, not only process health;
- manual-only Docker acceptance with two isolated application targets sharing one Prometheus provider;
- no implicit Grafana, Loki or Tempo provisioning.

OTLP metrics export remains a separate `telemetry.otlp/v1` transport concern. v0.4.8 does not redefine OTLP or make Prometheus part of application identity.

## Next architecture tracks

### v0.5 – Compose platform capabilities

Expand the portable capability model while keeping Compose as the production implementation:

- capability-provider boundaries for relational SQL, cache/key-value, secrets, S3-compatible object storage and exposure;
- replaceable reference providers rather than product lock-in;
- provider conformance and explicit capability negotiation;
- additional object-storage implementations such as Ceph RGW/external S3 behind the implemented `object-storage.s3/v1` contract;
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

## Provider placement, sharing boundaries and runtime realization

Provider placement is a BaseHarbor-wide deployment/operator concern. It is independent from application intent, runtime topology and product choice.

```text
Application intent
        |
        v
Capability
        |
        v
Provider resolution
        |
        v
Provider placement
   +----+------------------+
   |                       |
application             shared ---------------- external
                           |
                           +-- optional sharing boundary
        |
        v
Runtime realization of the selected placement
```

The canonical placement scopes remain exactly `application`, `shared` and `external`. A sharing boundary is an optional property of `shared`; it is not a fourth scope.

A shared provider is never automatically reachable by every application. Access is explicit, least-privilege and deny-by-default. A sharing boundary allows an operator to intentionally reuse one provider instance for a selected set of applications while keeping unrelated applications outside that trust boundary.

Provider implementations declare the placements they support. If policy resolves to a placement that the selected provider cannot satisfy, BaseHarbor fails closed before mutation instead of silently changing placement.

The portable application contract never contains provider placement, sharing-boundary, lifecycle-ownership or runtime-realization mechanics. The developer continues to state only application capabilities. BaseHarbor and deployment policy resolve the infrastructure details.

Placement semantics are fixed before runtime realization. The runtime may choose platform-native mechanisms to implement those semantics, but it may not reinterpret them: `application` remains one dedicated provider instance for exactly one application/environment, `shared` remains BaseHarbor Platform/Core Runtime infrastructure, and `external` remains externally lifecycle-owned. Compose currently realizes these guarantees through dedicated/shared projects, networks and volumes. Future Kubernetes/OpenShift runtimes may use namespaces/projects, Operators, NetworkPolicies or other platform-native mechanisms without changing the placement meaning or application intent.

A future Operator's installation scope is not the same thing as provider placement or resource scope. A cluster-scoped Operator may legitimately manage application-scoped or sharing-boundary-scoped resources.

Multiple BaseHarbor installations are therefore not required merely to isolate groups of applications that share selected providers. Separate BaseHarbor control planes are reserved for genuine administrative, trust-domain, infrastructure or compliance boundaries.

Current implementation scope remains Docker/Podman Compose. Kubernetes/OpenShift mappings described here are architectural compatibility requirements only, not implemented runtime behavior.
