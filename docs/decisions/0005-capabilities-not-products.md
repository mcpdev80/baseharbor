# ADR 0005: Application contracts describe capabilities, not products

Status: Accepted

Date: 2026-09-10

## Context

BaseHarbor is intended to carry the same logical application from local development and homelab use through Compose-based production and, later, Kubernetes/OpenShift enterprise deployments.

Concrete infrastructure products will change across that lifecycle. A local deployment may use bundled PostgreSQL, Valkey, OpenBao and Caddy while an enterprise deployment may use a customer PostgreSQL service, managed Redis, Vault, Ceph RGW and OpenShift Routes.

If product names become part of the portable application contract, every such replacement becomes an application migration. That would couple application repositories to BaseHarbor implementation choices and undermine the goal of keeping the application contract stable across environments.

## Decision

BaseHarbor application contracts MUST describe infrastructure capabilities and portable application requirements, not concrete infrastructure products.

Concrete implementations MUST be selected through replaceable providers outside the portable application contract.

The architecture is:

```text
Application Contract
        |
        v
Capabilities / portable requirements
        |
        v
Environment + policy
        |
        v
Provider selection
        |
        v
Concrete implementation
```

Examples of portable capability intent include:

```text
database.sql
cache.key-value
object-storage.s3
secrets
identity.oidc
ingress.http
tls.certificate
telemetry.otel
metrics.openmetrics
logs
```

Examples of concrete providers include PostgreSQL, Valkey, OpenBao, SeaweedFS, Caddy, cert-manager, Keycloak, Prometheus and Loki.

### Application and environment configuration are separate

The repository-owned application definition answers:

> What does this application require?

Environment/platform configuration answers:

> How will those requirements be realized here?

Conceptual future example:

```yaml
# baseharbor.yaml
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

A local environment may select:

```yaml
providers:
  database: postgres
  cache: valkey
  objectStorage: seaweedfs
  secrets: openbao
  ingress: caddy
```

An enterprise environment may select:

```yaml
providers:
  database: customer-postgres
  cache: managed-redis
  objectStorage: ceph-rgw
  secrets: vault
  ingress: openshift-route
```

The portable application definition remains unchanged.

### Default providers are implementation choices

BaseHarbor MAY ship or recommend default providers for a capability. A default is not part of the application-facing contract.

Every default or bundled BaseHarbor component MUST have:

1. a defined capability/provider boundary;
2. a documented replacement path;
3. stable application-facing interfaces where an ecosystem standard exists;
4. explicit capability negotiation when another provider cannot satisfy the same requirement.

Provider substitution MUST NOT silently weaken requested security, durability, availability or protocol guarantees.

### Provider configuration is environment-owned

Provider selection, credentials, endpoints, cluster-specific settings and product-specific tuning belong to environment/platform configuration or operator-managed state.

Applications MUST NOT need product-specific configuration merely because one environment uses a different provider.

Provider-specific escape hatches may exist later, but they MUST be optional, explicitly namespaced and must not redefine the portable capability model.

### Standards are preferred provider boundaries

Where possible, BaseHarbor uses existing ecosystem contracts rather than inventing proprietary data protocols:

- SQL/PostgreSQL-compatible connection interfaces for relational databases;
- Redis protocol where appropriate for key-value/cache access;
- S3 API for object storage;
- OIDC/OAuth2 for identity;
- OpenTelemetry/OpenMetrics for telemetry and metrics;
- standard TLS/X.509 material and ACME/PKI integration;
- native secret files/environment/workload identity mechanisms according to provider policy.

BaseHarbor orchestration and policy may be proprietary to BaseHarbor; application data access should not be.

## Current v0.2.0 compatibility

This decision does not add a v0.2.0 feature and does not change Manifest v1.

The current v0.2.0 manifest still names PostgreSQL/Redis/Valkey-oriented services because Compose is the first complete implementation. Those fields are treated as the current v1 contract, not as a requirement that all future provider-neutral schemas expose product names.

Future contract evolution under issues such as #97 and #102 must preserve migration compatibility appropriate for the pre-v1 `0.x` series while moving product choices behind provider boundaries.

## Consequences

### Positive

- applications can move from homelab to enterprise infrastructure without product-specific rewrites;
- BaseHarbor can replace a default component when project health, licensing, security or operational requirements change;
- customer-managed services become first-class without forking application manifests;
- Kubernetes/OpenShift implementations can use native platform services while preserving application intent;
- provider-specific lifecycle and support policies do not leak into the application contract.

### Costs

- capability semantics must be defined precisely enough that providers are actually interchangeable;
- provider capability negotiation and conformance testing become necessary;
- not every product is a drop-in replacement for every capability;
- environment/provider configuration becomes a separate versioned concern.

## Non-goals

This ADR does not require BaseHarbor to implement multiple providers immediately.

Compose remains the current focus. PostgreSQL, Valkey and OpenBao remain the current concrete implementations. Future Caddy, SeaweedFS, Kubernetes/OpenShift, identity and observability work should follow this boundary from the start.
