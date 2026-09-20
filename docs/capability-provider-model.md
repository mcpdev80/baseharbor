# Capability and provider model

BaseHarbor separates **what an application needs** from **which product provides it**.

This document is the working component matrix for the platform architecture. It records the portable capability, the current or planned BaseHarbor default provider, and documented replacement paths.

The governing rule is ADR [0005-capabilities-not-products](decisions/0005-capabilities-not-products.md).

## Rules

1. Application contracts describe capabilities and portable requirements.
2. Provider selection belongs to environment/platform configuration.
3. A bundled/default provider is an implementation choice, not application identity.
4. Every default provider must have at least one documented replacement path.
5. Provider substitution must preserve the requested contract or fail clearly; no silent downgrade of security, durability or availability.
6. Compose remains the complete runtime implementation in v0.4, now behind an explicit runtime-provider/capability-negotiation seam. Kubernetes/OpenShift provider mappings are future work and must preserve logical capability intent.
7. Deployment-specific implementation does not automatically create a new portable application capability. `PortableContract` changes only when that abstraction is deliberately designed and versioned.

## Component matrix

| Capability | Portable interface / intent | BaseHarbor default | Status in v0.4.8 | Replacement paths / alternatives | Architecture note |
| --- | --- | --- | --- | --- | --- |
| Relational SQL database | Manifest v1 PostgreSQL compatibility input normalized to `database.sql` in `PortableContract` | PostgreSQL | Implemented | external PostgreSQL, managed PostgreSQL/RDS-style services, compatible enterprise PostgreSQL platforms; other SQL engines only where the declared capability permits their semantics | PostgreSQL is the current reference provider, not the permanent conceptual capability name |
| Cache / key-value | Manifest v1 Redis/Valkey compatibility input normalized to `cache.key-value` in `PortableContract` | Valkey | Implemented | Redis, Dragonfly, managed Redis/Valkey; other KV systems only through a capability with matching semantics | Protocol/feature requirements must be explicit enough to avoid false interchangeability |
| Secrets | `secrets` / required secret names + policy-controlled delivery | OpenBao | Implemented; v0.4.5 maps identity/credentials/trust/authorization/secret refs through `secure-binding/v1` | Vault, cloud secret stores, external provider adapters | Application contract declares required secret intent, never OpenBao paths/AppRoles; security wiring is provider-neutral |
| HTTP/HTTPS exposure | `exposure.http/v1` | Caddy reference provider for Compose | Implemented in v0.4.4; application-owned publishers remain observed rather than lifecycle-owned | Traefik, Kubernetes Gateway API/Ingress, OpenShift Route, cloud traffic providers | Managed exposure is explicit portable intent; provider host ports, FQDNs, TLS files, networks and proxy configuration remain deployment/provider state |
| Object storage | `object-storage.s3/v1` / S3 API | SeaweedFS shared Compose reference provider | Deployment-time buckets implemented in v0.4.6; authorized runtime create/get/delete implemented after v0.4.7 through the Runtime Provider Executor | Ceph RGW, AWS S3 and conforming S3-compatible managed services | Logical buckets and S3 semantics are application-facing; SeaweedFS topology, physical bucket/user identity and provider endpoint placement remain provider state; provider-global credentials stay at the executor boundary |
| TLS certificate lifecycle | future `tls.certificate` / X.509 identity | provider-specific | Deployment-specific existing/BYOC lifecycle implemented; portable capability planned | existing/BYOC certificates, OpenBao PKI, ACME provider, cert-manager, OpenShift Service CA, cloud-native certificate services | v0.4 validates/imports/updates existing certificates for repository Compose deployment state; ACME/PKI/provider-neutral intent remain future work |
| External secret projection | provider integration, not a portable app product | none required globally; ESO may be an adapter | Planned/optional | External Secrets Operator, Secrets Store CSI, Vault/OpenBao native workload identity, platform-native secret projection | ESO must never become part of the application contract |
| Identity / SSO | future `identity.oidc` / OIDC/OAuth2 | no hard-wired product; Keycloak is a possible self-hosted reference | Planned | Authentik, Zitadel, Entra ID, Google Workspace, GitHub or other compliant OIDC providers | BaseHarbor should consume identity claims; it should not require applications to depend on Keycloak-specific APIs |
| Metrics | `metrics/v1` / OpenMetrics-compatible HTTP exposition | Prometheus 3.14.0 Compose reference provider; shared default, named shared boundaries and application-scoped placement | v0.4.8: source contract, generic placement resolution, policy-controlled collection, automatic target registration and real scrape/ingestion verification | VictoriaMetrics, Mimir and compatible backends | The application declares only its metrics source; provider placement, collection policy, retention, query and storage remain replaceable deployment/platform/provider state |
| OTLP telemetry transport | `telemetry.otlp/v1` / OTLP HTTP-protobuf export | OpenTelemetry Collector 0.161.0 shared Compose reference provider | Implemented in v0.4.7; external OTLP endpoints are supported without lifecycle ownership | any conforming OTLP HTTP/protobuf endpoint, managed or external | OpenTelemetry is the ecosystem; OTLP is the portable protocol boundary; the collector is a provider implementation, not application identity |
| Logs | `logs/v1` platform log-source lifecycle; application intent remains product-neutral | Loki 3.7.8 + Alloy 1.19.2 Compose reference provider | v0.4.9: policy-controlled workload collection, shared/application placement, protected registry ownership and real query verification | OpenSearch, Elasticsearch, VictoriaLogs and compatible stacks through conforming providers | Loki is provider state, not application intent; local `baha app logs` remains an independent trusted-local operator path |

## Runtime provider versus capability provider

These are separate axes.

A **runtime provider** decides where/how workloads run:

```text
Compose
Kubernetes
OpenShift
```

A **capability provider** decides which implementation satisfies a resource requirement:

```text
database.sql        -> PostgreSQL / managed PostgreSQL
cache.key-value     -> Valkey / Redis / managed Redis
object-storage.s3   -> SeaweedFS / Ceph RGW / AWS S3
secrets             -> OpenBao / Vault / cloud secret store
exposure.http       -> Caddy / Traefik / Kubernetes Gateway / OpenShift Route
```

An OpenShift runtime therefore does not imply that every capability must also be OpenShift-native. An enterprise customer may combine OpenShift workloads with an external PostgreSQL cluster, Vault and Ceph RGW.

## Application contract versus environment definition

Conceptual target model:

```yaml
# conceptual future application-owned contract
apiVersion: baseharbor.io/v1
kind: Application
metadata:
  name: mailflow
spec:
  resources:
    database:
      main:
        type: sql
    cache:
      main:
        type: key-value
    objectStorage:
      attachments:
        type: s3
  secrets:
    - OPENAI_API_KEY
  tls:
    required: true
```

Environment/provider mapping:

```yaml
# conceptual future environment-owned configuration
providers:
  database: postgres
  cache: valkey
  objectStorage: seaweedfs
  secrets: openbao
  exposure: caddy
```

Enterprise mapping:

```yaml
providers:
  database: customer-postgres
  cache: managed-redis
  objectStorage: ceph-rgw
  secrets: vault
  exposure: openshift-route
```

The exact future environment/provider configuration schema is intentionally not fixed by v0.4.0. Deployment-owned runtime provider/profile state exists, while broader environment policy remains future work.

## Provider conformance

A future provider interface must describe more than a product name. Providers need to advertise and be tested against capabilities such as:

- protocol/API compatibility;
- durability/persistence;
- backup/restore support;
- HA/replication support;
- encryption in transit/at rest where required;
- identity/credential model;
- rotation and revocation behavior;
- observability hooks;
- lifecycle operations;
- supported runtime/environment constraints.

If the selected provider cannot satisfy a requested guarantee, BaseHarbor must reject the plan rather than silently reduce the guarantee.

## v0.4.8 boundary

v0.4.8 remains Compose-only at runtime. The v0.4 line now includes the shared capability/provider/resource/binding core, protected provider placement/ownership, the Provider Integration Contract v1, deterministic repository inspection, managed traffic through `exposure.http/v1`, the shared `secure-binding/v1` security boundary, `object-storage.s3/v1`, provider-neutral `telemetry.otlp/v1` export binding, and `metrics/v1` with Prometheus as the first Compose reference provider. It also adds explicit directional cross-application connectivity as a separate deny-by-default platform policy.

Manifest v1 remains the supported compatibility surface. Managed exposure and metrics sources are additive and explicit; application-owned publishers remain application-owned observation/readiness state. Provider sharing never implies cross-application connectivity.

These seams must not be misread as Kubernetes/OpenShift support. Additional S3/object-storage providers and public provider-selection policy, HA profiles, managed ACME/OpenBao-PKI certificate issuance and Kubernetes/OpenShift runtime providers remain future work.


## Provider registry in v0.4.2

Provider placement remains protected deployment/operator state, not portable application intent.

- `shared`: BaseHarbor-managed provider reusable by multiple applications.
- `application`: BaseHarbor-managed provider dedicated to one application.
- `external`: existing/BYO provider referenced by BaseHarbor but lifecycle-owned elsewhere.

Current reference mapping:

```text
OpenBao              -> shared
SeaweedFS            -> shared
PostgreSQL instances -> application-scoped
Valkey instances     -> application-scoped
```

Logical resources remain application-owned regardless of provider scope. Shared providers are retained during application lifecycle operations, external providers are never lifecycle-mutated by BaseHarbor, and only BaseHarbor-owned application-scoped providers are provider-lifecycle-owned by an application.


## Provider Integration Contract v1

All capability providers added after v0.4.2 must implement the shared [Provider Integration Contract v1](provider-integration-contract.md).

The contract separates three concerns:

```text
Capability Specification
  = what the application is guaranteed

Provider Integration Contract
  = lifecycle/ownership/diagnostic semantics

Provider implementation
  = PostgreSQL, Tempo, RabbitMQ, cloud service, etc.
```

Current reference claims are versioned:

- PostgreSQL implements `database.sql/v1`;
- Valkey implements `cache.key-value/v1`;
- OpenBao implements `secrets/v1`;
- Caddy implements `exposure.http/v1`;
- SeaweedFS implements `object-storage.s3/v1`.
- OpenTelemetry Collector implements `telemetry.otlp/v1`.

Additional S3, telemetry, observability, messaging, AI/MCP and vector providers must define/implement versioned capability specifications rather than introduce product-specific application contracts.

The future external transport is gRPC/Protocol Buffers and distribution direction is OCI. Those are open-standard transport/package mechanisms; BaseHarbor capability semantics and conformance remain authoritative.

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

Placement has strict provider-instance semantics:

- `application`: one BaseHarbor-managed provider instance dedicated to exactly one application/environment. In the Compose runtime this means a dedicated provider container/project and dedicated provider state; it is never reused by another application.
- `shared`: one BaseHarbor Platform/Core Runtime provider instance, created lazily when first required and reusable by one or more explicitly authorized applications. A provider remains `shared` even while it currently has only one consumer.
- `external`: a provider instance operated outside BaseHarbor. BaseHarbor may bind to it, but does not own or provision its lifecycle.

A shared provider is never automatically reachable by every application. Access is explicit, least-privilege and deny-by-default. A sharing boundary allows an operator to intentionally reuse one platform provider instance for a selected set of applications while keeping unrelated applications outside that trust boundary. Sharing the provider process never implies sharing logical application resources, credentials, data or network access.

Shared providers are on-demand platform infrastructure rather than unconditional bootstrap dependencies. If a compatible shared instance already exists in the BaseHarbor Platform/Core Runtime, BaseHarbor reuses it instead of starting another provider instance.

Provider implementations declare the placements they currently support. If policy resolves to a placement that the selected provider adapter cannot truthfully realize, BaseHarbor fails closed before mutation instead of silently changing placement.

The portable application contract never contains provider placement, sharing-boundary, lifecycle-ownership or runtime realization mechanics. The developer continues to state only application capabilities. BaseHarbor and deployment policy resolve the infrastructure details.

The placement semantics are runtime-independent even though realization differs. Compose realizes an `application` provider as a dedicated container/project and a `shared` provider as BaseHarbor Platform/Core Runtime infrastructure. Future Kubernetes/OpenShift runtimes may realize the same semantics with dedicated/shared platform-native resources, namespaces/projects, Operators or other isolation mechanisms without changing application intent.

A future Operator's installation scope is not the same thing as provider placement or resource scope. A cluster-scoped Operator may legitimately manage application-scoped or sharing-boundary-scoped resources.

Multiple BaseHarbor installations are therefore not required merely to isolate groups of applications that share selected providers. Separate BaseHarbor control planes are reserved for genuine administrative, trust-domain, infrastructure or compliance boundaries.

Current implementation scope remains Docker/Podman Compose. Kubernetes/OpenShift mappings described here are architectural compatibility requirements only, not implemented runtime behavior.

## Progressive disclosure and explicit operator control

BaseHarbor must be simple by default without becoming restrictive.

The normal developer path should require only application intent and should use safe, explainable defaults:

```text
developer declares capability
        |
        v
BaseHarbor detects/resolves sensible defaults
        |
        v
plan -> preflight -> apply -> verify
```

Advanced users and operators must still be able to override deployment decisions explicitly where the platform supports them, including provider selection, provider placement, optional sharing boundary, lifecycle ownership where applicable, external provider references, isolation/deployment policy and supported provider/runtime options.

The control model is therefore progressive disclosure:

```text
simple path
  -> automatic safe defaults

advanced path
  -> explicit deployment/operator policy

expert path
  -> fully specified supported provider/runtime realization
```

Explicit control must not require polluting the portable application contract with infrastructure details. Portable application intent remains product-neutral; concrete infrastructure choices belong to deployment/operator configuration and control surfaces.

BaseHarbor must show the resolved plan before mutation so users can see what defaults were selected and can override supported decisions deliberately. Explicit user/operator configuration wins over defaults, but never bypasses capability conformance, security boundaries, validation or fail-closed behavior.

The goal is: easy when the user does not care about infrastructure details, precise when the user does.

## Convention by default, configuration by choice

BaseHarbor follows one UX and architecture principle across all capabilities and runtimes:

> **Convention by default, configuration by choice.**

The default path minimizes decisions. BaseHarbor detects what it can, chooses safe and explainable defaults, shows the resolved plan and proceeds through the normal validation lifecycle.

Users who want more control may progressively override supported deployment decisions without changing portable application intent.

```text
default
  -> capabilities only
  -> safe automatic provider/placement/runtime defaults

advanced
  -> explicit provider / placement / sharing / external references

expert
  -> supported naming, topology, runtime and provider realization hints
```

Examples of optional expert control may include stable resource prefixes, logical hostnames, Compose project/network/volume names, DNS aliases and later Kubernetes/OpenShift namespace/project naming. Ephemeral runtime-generated identities such as replica or Pod instance names remain runtime-owned unless the runtime explicitly supports a safe stable override.

Every configurable field must have explicit semantics:

- stable and safely overridable;
- hint/template only;
- generated/runtime-owned and not overridable.

Overrides are accepted only when the active runtime/provider can honor them safely and deterministically. They must never bypass security, ownership, reconciliation, conformance or fail-closed validation.

The simple path and expert path must use the same core model. Advanced flexibility must not create a second application contract or parallel lifecycle implementation.
