# Standards-first architecture

BaseHarbor prefers established standards before defining BaseHarbor-specific contracts.

## Rule

1. An established open standard MUST be used when it covers the required semantics.
2. An established de-facto standard SHOULD be used when no suitable formal standard exists.
3. Established architecture/API patterns SHOULD shape BaseHarbor models where they improve interoperability without importing platform-specific concepts.
4. BaseHarbor-specific contracts define only semantics not reasonably covered by an existing standard.
5. BaseHarbor extensions are explicit, versioned and provider-neutral.
6. Provider-specific contracts stay behind the provider boundary.

Every new service kind or provider documents:

- Existing Standards
- Adopted Standards
- Deviations
- BaseHarbor Extensions
- Compatibility Impact

before implementation.

## Adopted standards and patterns

| Concern | BaseHarbor rule |
| --- | --- |
| Schema/validation | JSON Schema 2020-12 |
| Workload service connection outputs | Service Binding Specification 1.1 well-known entry names where semantically applicable |
| Provider/resource architecture | Crossplane resource/provider/reconciliation patterns as an architecture reference, without importing Kubernetes API objects into portable intent |
| Catalog/provision/bind lifecycle | Open Service Broker API concepts where applicable |
| Provider process boundary | gRPC + Protocol Buffers |
| Provider artifact distribution | OCI Image/Distribution; digest-first identity |
| Observability | OpenTelemetry and OTLP |
| Identity | OpenID Connect/OAuth where applicable |
| Messaging API description | AsyncAPI |
| Event envelope | CloudEvents where event semantics apply |
| Redis/Valkey-compatible cache | RESP compatibility declared by the provider |
| Object storage | S3 API compatibility is a de-facto protocol capability, not the generic service identity |

## Service, protocol, provider and product are separate

```text
Application intent
        |
        v
BaseHarbor service contract
        |
        v
protocol / semantic requirements
        |
        v
provider resolution
        |
        v
provider implementation
        |
        v
product / engine realization
```

Example:

```text
service contract: sql/v1
provider:         baseharbor/postgresql@1.3.0
product:          PostgreSQL 18.x
```

These versions are independent.

## Compatibility with existing v0.4 contracts

Existing specification IDs remain supported while the pre-v0.5 contract migration is designed.

Current IDs such as:

- `database.sql/v1`
- `cache.key-value/v1`
- `object-storage.s3/v1`
- `telemetry.otlp/v1`

are not renamed blindly. The freeze work must classify each as either:

- canonical service contract;
- protocol/semantic capability under a broader service contract;
- compatibility alias requiring migration.

No existing application contract is changed until its compatibility path is documented and tested.
