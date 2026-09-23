# Provider Contract v1

## Scope

This specification defines the common rules for providers.

## Provider axes

Runtime, Capability and Delivery Provider are independent axes.

```text
runtime != capability != delivery
```

Implementations MUST NOT make one axis a hidden requirement of another.

## Standards-first interoperability

Providers MUST use an established open standard when it already defines the required interoperable semantics. De-facto standards and established ecosystem conventions SHOULD be used where no suitable formal standard exists.

Provider-specific contracts MUST stay behind the provider boundary. A provider MUST NOT require BaseHarbor Core or portable application intent to adopt product-specific configuration merely because that provider needs it.

Before a new service kind or provider is implemented, its design records:

- Existing Standards
- Adopted Standards
- Deviations
- BaseHarbor Extensions
- Compatibility Impact

## Provider identity and versioning

A provider implementation MUST expose a stable provider ID and implementation version independently from BaseHarbor and independently from capability specification versions.

For example:

```text
provider: baseharbor/postgresql
provider version: 0.1.0
protocol: baseharbor.provider/v1
capability: database.sql/v1
```

A concrete product version or image digest is realization metadata. It MUST NOT replace the provider implementation version or the capability specification version.

## Capabilities

A provider MUST declare the semantics it supports.

BaseHarbor MUST reject a provider before mutation when a required semantic is unsupported.

Nominal capability-name equality alone MUST NOT be treated as compatibility.

## Service connection outputs

Provider connection outputs align with Service Binding Specification 1.1 well-known entry names whenever the semantics match:

```text
type
provider
host
port
uri
username
password
certificates
private-key
```

Alternative aliases such as `hostname`, `connectionHost`, `user`, `pass` or `connectionString` MUST NOT be introduced when the standard name applies.

Secret-bearing entries may be represented internally through protected references and resolved only at the trusted workload projection boundary. Standard naming does not permit plaintext secrets in diagnostics, registry state or portable intent.

BaseHarbor-specific binding metadata uses an explicit versioned extension namespace and remains separate from Service Binding fields.

## Placement and ownership

Where applicable, providers use:

```text
application
shared
external
```

BaseHarbor MUST NOT destructively mutate foreign/external resources it does not own.

## Verification

A provider reporting process health is not sufficient when the capability requires protocol/data-flow verification.

## Registry boundaries

BaseHarbor distinguishes two registries:

- **Runtime Provider Registry** — deployed provider instances, placement, ownership and application/resource bindings.
- **Provider Catalog** — installable provider distributions, versions, supported service contracts/capabilities, product compatibility, platforms, OCI artifact identity, digest, schema and provenance.

The Runtime Provider Registry MUST NOT become a package/distribution catalog.

Provider artifacts SHOULD use OCI-compatible distribution. Artifact digest is immutable identity; tags are discovery aliases.

## Extensibility

Provider implementations MAY use mature OSS, standard APIs/SDKs, controllers/operators or managed-service APIs behind the BaseHarbor contract.
