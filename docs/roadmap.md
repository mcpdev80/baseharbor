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

## Current v0.4.12 portable application foundation

Docker/Podman Compose remains the complete runtime implementation. v0.4 adds the architecture seams required to evolve beyond it without redefining the application contract.

Implemented foundations include:

- `baha` as the primary lifecycle CLI;
- repository-owned Manifest v1 `baseharbor.yaml` as the supported compatibility surface;
- a provider-neutral `PortableContract` adapter for application intent;
- a shared capability/provider/resource/binding domain core with fail-closed provider negotiation and machine-readable lifecycle results;
- a provider registry with shared, application-scoped and external/BYO placement plus explicit lifecycle ownership;
- `metrics/v1` with Prometheus 3.14.0 as the first Compose reference provider, shared/application placement, isolated metrics networks and real scrape/ingestion verification;
- `logs/v1` platform log-source lifecycle with Loki 3.7.8 + Alloy 1.19.2, shared/application placement and real query verification;
- rendered-Compose workload security preflight plus executable Provider Integration Contract conformance/fault injection;
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

Delivery providers are a third independent axis. They decide how desired runtime realization reaches/reconciles against the selected runtime. Direct delivery and delegated/GitOps delivery must preserve the same BaseHarbor semantics without making Argo CD, Flux, Git or Kubernetes objects portable application requirements.

Where applicable, Runtime, Capability and Delivery Provider families reuse the same canonical `application | shared | external` placement/ownership semantics while keeping their responsibilities separate.

BaseHarbor remains the normal developer/agent interface. Mature OSS and open standards are reused behind provider boundaries rather than reimplemented, while native tool UIs/CLIs remain available for platform/expert drill-down.

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

## v0.4.9 logs / Loki and hardening track

Implemented in v0.4.9:

- centralized application workload log collection remains deployment/platform policy rather than Loki application intent;
- selected repository workload services are logical `logs/v1` resources owned by the application and recorded in the protected provider registry;
- Loki 3.7.8 and Alloy 1.19.2 are the Compose reference provider/collector, with shared or application placement and optional named shared boundaries;
- Loki/Alloy use no Docker/Podman socket and host-facing listeners remain loopback-only;
- readiness includes a real Loki query for application/environment/service streams;
- rendered repository Compose is checked before mutation for isolation-breaking privileges;
- Provider Integration Contract v1 now has executable lifecycle conformance plus a deterministic fake provider and fault injection;
- Kubernetes/OpenShift remain later runtime tracks.

## v0.4.11 developer experience and repository adoption

Released/completed in v0.4.11. This release makes the existing provider-neutral runtime foundation easier to adopt without moving the agent-native machine interface forward from v0.4.12.

Implemented:

- local and remote Git repository inspection using the shared deterministic evidence engine;
- `baha up -e/--environment` deployment-context selection without rewriting portable intent;
- repository-aware `baha plan`, `baha status` and `baha doctor` shortcuts;
- shared secret-safe structured output for inspect/plan/status/doctor;
- optional bounded/idempotent `AGENTS.md` guidance;
- first-class local Compose Playground documentation;
- five-minute onboarding.

The structured result work was deliberately a foundation for the next machine-facing layer.

## v0.4.12 agent-native machine interface and MCP

Implemented/completed in v0.4.12:

- versioned BaseHarbor machine contract `v1` for inspect/plan/status/doctor results;
- structured secret-safe error envelopes for JSON automation failures;
- explicit semantic operation safety metadata;
- `baha agent describe` / `baha agent describe -o json` discovery;
- official Model Context Protocol Go SDK v1.8.0;
- MCP specification `2026-07-28` as the current protocol target, with SDK-negotiated `2025-11-25` compatibility;
- local stdio-only `baha mcp serve`;
- four read-only MCP tools: `baseharbor.inspect`, `baseharbor.plan`, `baseharbor.status`, `baseharbor.doctor`;
- explicit MCP read-only/destructive/open-world annotations;
- shared typed status+TLS and doctor result paths consumed directly by CLI, TUI and MCP;
- generic MCP-client acceptance proving discovery and semantic execution without a BaseHarbor-specific plugin;
- secret-leak acceptance and explicit rejection of generic shell/Docker/Compose execution primitives;
- bounded/idempotent `AGENTS.md` guidance extended toward structured BaseHarbor interfaces.

Remote MCP transport/authentication, mutating/destructive MCP tools, embedded LLM logic, vendor-specific agent integrations and application-provided MCP capabilities remain out of scope.


## Remaining v0.4 sequence

v0.4 finishes the runtime-neutral BaseHarbor language and lifecycle semantics before the compatibility freeze.

### v0.4.13 – Environment and policy semantics

- deterministic environment resolution;
- typed allow/warn/deny policy;
- secure defaults and bounded operator overrides;
- environment remains distinct from runtime, topology and availability.

### v0.4.14 – Reconciliation, security and lifecycle semantics

- desired/observed/diff/ownership model;
- typed drift, conflict, foreign ownership, unsupported and degraded states;
- idempotent convergence and minimal repair;
- ownership-safe destroy and verified recovery.

### v0.4.15 – Audit and evidence semantics

- secret-safe lifecycle/policy/verification events;
- desired/enforced/observed/verified distinction;
- generic evidence export boundary without vendor lock-in.

### v0.4.16 – Capability Provider SDK, starter kit and conformance

- practical third-party capability-provider authoring path;
- conformance against the now-complete lifecycle, ownership, security and evidence semantics;
- no runtime-provider SDK and no Kubernetes-specific types.

### v0.4.17 – Runtime boundary and semantic full-stack acceptance

- classify all important state as portable, deployment/operator, runtime, provider or protected/generated;
- prove the full Compose reference lifecycle;
- prove CLI/JSON/MCP semantic consistency;
- close all blockers before the v0.5 freeze.

## Provider ecosystem direction

The open provider ecosystem remains strategic. gRPC/Protocol Buffers are the language-neutral external process boundary where required, and OCI is the registry-neutral distribution path for independently implemented community/vendor/company providers.

Provider implementations may internally use mature OSS, standard APIs/SDKs, controllers/operators/CRDs or managed-service APIs. Those mechanisms stay behind the provider boundary; BaseHarbor Core must not become a catalog of product-specific integrations.

## v0.5 – Contract freeze and compatibility

v0.5 adds no new platform primitive. It freezes, versions and proves the contracts completed in v0.4.

- **v0.5.0** complete agent-native core and contract freeze;
- **v0.5.1** state versioning and migration compatibility;
- **v0.5.2** cross-component compatibility contracts;
- **v0.5.3** upgrade, recovery and deprecation behavior;
- **v0.5.4** portability and compatibility acceptance.

Before v0.5 closes, the deployment/runtime boundary must be able to represent a future restricted runtime target, secret-safe runtime access references and platform-owned resource references without adding Kubernetes-specific fields to portable application intent.

The BaseHarbor MCP/JSON control surface must also cover the complete useful Compose lifecycle before the machine contract is considered frozen.

## v0.6 – Availability, topology and portable guarantees

v0.6 defines portable availability semantics before any Kubernetes implementation.

- **v0.6.0** availability and portable guarantee semantics;
- **v0.6.1** runtime and capability guarantee negotiation;
- **v0.6.2** truthful Compose realization and verification;
- **v0.6.3** topology and portability acceptance.

Environment, runtime and availability remain independent. Compose may satisfy only the guarantees it can actually prove; there is no silent downgrade or invented HA.

Human OIDC/RBAC/JIT access, remote-management UI and other platform-access concerns are separate later tracks and do not define v0.6.

## v0.7 – Kubernetes Runtime

v0.7 implements Kubernetes as a BaseHarbor Runtime Provider. Namespace-only operation with a pre-provisioned target namespace is the primary enterprise-compatible design target.

- **v0.7.0** Runtime foundation and namespace-only access model;
- **v0.7.1** namespaced security, identity and networking;
- **v0.7.2** platform-owned HTTP exposure with Gateway API;
- **v0.7.3** namespace-only persistent storage realization;
- **v0.7.4** stateful runtime support without capability-provider leakage;
- **v0.7.5** availability realization with permission-aware verification;
- **v0.7.6** Runtime conformance across restricted-access profiles;
- **v0.7.7** Compose-to-Kubernetes portability proof under namespace-only constraints;
- **v0.7.8** delegated delivery and GitOps provider architecture, with Argo CD as the first reference provider and no Argo-specific portable application contract.

Cluster-admin, namespace creation and cluster-wide discovery are not normal application-lifecycle requirements. Platform-owned resources such as namespaces, Gateway/GatewayClass, StorageClass, CRDs and admission policy remain consumable without BaseHarbor owning them.

The normal path uses the Kubernetes API directly. kubectl, Helm, CRDs and a BaseHarbor Operator are not required runtime engines for v0.7.

## v0.8 – Kubernetes Complete

v0.8 closes the product-level parity gap and makes Kubernetes a fully first-class BaseHarbor production runtime.

- **v0.8.0** full application lifecycle parity;
- **v0.8.1** capability and provider-placement parity;
- **v0.8.2** backup, restore and disaster-recovery parity;
- **v0.8.3** observability, diagnostics and evidence parity;
- **v0.8.4** update, migration and recovery parity;
- **v0.8.5** agent-native and developer-experience parity;
- **v0.8.6** production hardening and complete conformance matrix;
- **v0.8.7** Kubernetes Complete first-class runtime acceptance.

The completion gate is that every BaseHarbor core feature applicable to Kubernetes works through BaseHarbor semantics, including namespace-only operation, without a Kubernetes-specific portable application contract or normal raw-Kubernetes fallback.

Only after Kubernetes Complete does the roadmap move into OpenShift/Enterprise-specific behavior. Operator/OLM, SCC/Route-specific integration, enterprise proxy/registry/disconnected flows and human OIDC/JIT access remain separate later tracks unless a demonstrated prerequisite is discovered.

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
