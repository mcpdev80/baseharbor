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
6. Compose remains the complete implementation focus in v0.3. Kubernetes/OpenShift provider mappings are future work and must preserve logical capability intent.
7. Deployment-specific implementation in v0.3 does not automatically create a new portable application capability. The portable contract changes only when that abstraction is deliberately designed and versioned.

## Component matrix

| Capability | Portable interface / intent | BaseHarbor default | Status in v0.3.0 | Replacement paths / alternatives | Architecture note |
| --- | --- | --- | --- | --- | --- |
| Relational SQL database | current Manifest v1 PostgreSQL requirement; future `database.sql` capability | PostgreSQL | Implemented | external PostgreSQL, managed PostgreSQL/RDS-style services, compatible enterprise PostgreSQL platforms; other SQL engines only where the declared capability permits their semantics | PostgreSQL is the current reference provider, not the permanent conceptual capability name |
| Cache / key-value | current Manifest v1 Redis/Valkey requirement; future `cache.key-value` capability | Valkey | Implemented | Redis, Dragonfly, managed Redis/Valkey; other KV systems only through a capability with matching semantics | Protocol/feature requirements must be explicit enough to avoid false interchangeability |
| Secrets | `secrets` / required secret names + policy-controlled delivery | OpenBao | Implemented | Vault, cloud secret stores, external provider adapters | Application contract declares required secret intent, never OpenBao paths/AppRoles |
| HTTP ingress / reverse proxy | future `ingress.http` | no BaseHarbor-managed provider in v0.3; application-owned Compose exposure is observed | Portable capability planned; app-owned HTTP/HTTPS readiness implemented | Caddy, Traefik, nginx, HAProxy, Kubernetes Gateway/Ingress, OpenShift Route | v0.3 verifies conventional app-owned publishers but does not claim ownership of ingress provisioning |
| Object storage | future `object-storage.s3` / S3 API | SeaweedFS planned reference/default | Planned | Garage, Ceph RGW, AWS S3 and compatible managed services | S3 is the application boundary; storage topology and implementation remain provider-owned |
| TLS certificate lifecycle | future `tls.certificate` / X.509 identity | provider-specific | Deployment-specific existing/BYOC lifecycle implemented; portable capability planned | existing/BYOC certificates, OpenBao PKI, ACME provider, cert-manager, OpenShift Service CA, cloud-native certificate services | v0.3 validates/imports/updates existing certificates for repository Compose deployment state; ACME/PKI/provider-neutral intent remain future work |
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

The exact environment file name/schema is intentionally not fixed by v0.3.0. That is future contract work. The architectural separation is fixed now.

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

## v0.3.0 boundary

v0.3.0 remains Compose-only and does **not** introduce the future generic capability/provider manifest schema.

It does add concrete operational completeness around the current provider: trusted-local developer access, health/exposure truth, verified backup/restore, guarded updates, workload-only applications, deployment FQDN/TLS runtime state, existing/BYOC TLS lifecycle and safe port fallback.

Those features must not be misread as permission to encode Compose product/runtime details into the future portable application contract. The provider seam, generic capability schema, Kubernetes/OpenShift providers, HA profiles, managed ingress, ACME/PKI automation and object-storage provider implementation remain future work.
