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

| Capability | Portable interface / intent | BaseHarbor default | Status in v0.4.1 | Replacement paths / alternatives | Architecture note |
| --- | --- | --- | --- | --- | --- |
| Relational SQL database | Manifest v1 PostgreSQL compatibility input normalized to `database.sql` in `PortableContract` | PostgreSQL | Implemented | external PostgreSQL, managed PostgreSQL/RDS-style services, compatible enterprise PostgreSQL platforms; other SQL engines only where the declared capability permits their semantics | PostgreSQL is the current reference provider, not the permanent conceptual capability name |
| Cache / key-value | Manifest v1 Redis/Valkey compatibility input normalized to `cache.key-value` in `PortableContract` | Valkey | Implemented | Redis, Dragonfly, managed Redis/Valkey; other KV systems only through a capability with matching semantics | Protocol/feature requirements must be explicit enough to avoid false interchangeability |
| Secrets | `secrets` / required secret names + policy-controlled delivery | OpenBao | Implemented | Vault, cloud secret stores, external provider adapters | Application contract declares required secret intent, never OpenBao paths/AppRoles |
| HTTP ingress / reverse proxy | future `ingress.http` | no BaseHarbor-managed ingress provider in v0.4; application-owned Compose exposure is observed | Portable capability planned; app-owned HTTP/HTTPS readiness implemented | Caddy, Traefik, nginx, HAProxy, Kubernetes Gateway/Ingress, OpenShift Route | v0.4 verifies conventional app-owned publishers but does not claim ownership of ingress provisioning |
| Object storage | future `object-storage.s3` / S3 API | SeaweedFS planned reference/default | Planned | Garage, Ceph RGW, AWS S3 and compatible managed services | S3 is the application boundary; storage topology and implementation remain provider-owned |
| TLS certificate lifecycle | future `tls.certificate` / X.509 identity | provider-specific | Deployment-specific existing/BYOC lifecycle implemented; portable capability planned | existing/BYOC certificates, OpenBao PKI, ACME provider, cert-manager, OpenShift Service CA, cloud-native certificate services | v0.4 validates/imports/updates existing certificates for repository Compose deployment state; ACME/PKI/provider-neutral intent remain future work |
| External secret projection | provider integration, not a portable app product | none required globally; ESO may be an adapter | Planned/optional | External Secrets Operator, Secrets Store CSI, Vault/OpenBao native workload identity, platform-native secret projection | ESO must never become part of the application contract |
| Identity / SSO | future `identity.oidc` / OIDC/OAuth2 | no hard-wired product; Keycloak is a possible self-hosted reference | Planned | Authentik, Zitadel, Entra ID, Google Workspace, GitHub or other compliant OIDC providers | BaseHarbor should consume identity claims; it should not require applications to depend on Keycloak-specific APIs |
| Metrics | future `metrics.openmetrics` / OpenMetrics-compatible scrape/export | Prometheus as reference/default candidate | Planned | VictoriaMetrics, Mimir and compatible backends | Keep collection/query/storage backend replaceable |
| Traces and telemetry transport | future `telemetry.otel` / OpenTelemetry | OpenTelemetry | Planned | vendor-specific backends behind OTel-compatible exporters | OTel is the standard interface, not merely one selectable product |
| Logs | structured application/runtime logs with provider-defined transport | Loki as reference/default candidate | Trusted-local Compose log access implemented; backend abstraction planned | OpenSearch, Elasticsearch, VictoriaLogs and compatible stacks | `baha app logs` is a local operator workflow, not a commitment to one log storage backend |

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
ingress.http        -> Caddy / Kubernetes Gateway / OpenShift Route
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
  ingress: caddy
```

Enterprise mapping:

```yaml
providers:
  database: customer-postgres
  cache: managed-redis
  objectStorage: ceph-rgw
  secrets: vault
  ingress: openshift-route
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

## v0.4.1 boundary

v0.4.1 remains Compose-only at runtime. In addition to the v0.4.0 `PortableContract` and runtime-provider seam, it introduces the shared capability/provider/resource/binding domain core, fail-closed capability-provider negotiation, machine-readable lifecycle result types, and protected runtime metadata for resolved PostgreSQL, Valkey and OpenBao capability bindings. It does not introduce a new generic public capability/provider manifest schema.

It preserves the concrete operational completeness of the Compose provider and adds the architectural seams required for future providers without changing the public Manifest v1 compatibility surface.

These seams must not be misread as Kubernetes/OpenShift support. A new public capability schema, additional capability-provider implementations, HA profiles, managed ingress, ACME/PKI automation and Kubernetes/OpenShift runtime providers remain future work.


## Provider registry in v0.4.2

Provider placement remains protected deployment/operator state, not portable application intent.

- `shared`: BaseHarbor-managed provider reusable by multiple applications.
- `application`: BaseHarbor-managed provider dedicated to one application.
- `external`: existing/BYO provider referenced by BaseHarbor but lifecycle-owned elsewhere.

Current reference mapping:

```text
OpenBao              -> shared
PostgreSQL instances -> application-scoped
Valkey instances     -> application-scoped
```

Logical resources remain application-owned regardless of provider scope. Shared providers are retained during application lifecycle operations, external providers are never lifecycle-mutated by BaseHarbor, and only BaseHarbor-owned application-scoped providers are provider-lifecycle-owned by an application.
