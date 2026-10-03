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

Capability providers use the common placement vocabulary where the underlying product and provider implementation can satisfy the required semantics:

```text
shared
application
external
```

A provider descriptor MUST declare every placement scope it actually implements. BaseHarbor MUST reject an unsupported requested scope before mutation and MUST NOT silently downgrade isolation or ownership.

For `shared` placement:

- provider infrastructure lifecycle MUST belong to the Target/provider boundary rather than to one application;
- application data/logical resources, credentials, identities and Service Bindings MUST remain application-scoped;
- one application MUST NOT be able to read, mutate or destroy another application's logical resources through the standard application binding;
- application destroy MUST remove only application-owned resources and MUST preserve the shared provider and sibling applications;
- provider/Target destroy MAY remove the shared provider after application ownership has been released;
- a provider MUST NOT claim `shared` support if these isolation and lifecycle guarantees cannot be met.

Sharing provider infrastructure is explicitly a resource-efficiency mechanism. It MUST NOT be implemented by merely reusing one global application credential or one unpartitioned application data namespace.

`application` placement provides a dedicated provider lifecycle for one application/environment and remains valid when stronger physical isolation or product limitations require it.

`external` placement keeps provider lifecycle ownership outside BaseHarbor. BaseHarbor MUST NOT destructively mutate foreign/external resources it does not own.

A named sharing boundary MAY subdivide `shared` placement without introducing a fourth scope.

## Credential ownership taxonomy

Provider implementations MUST use the normative [Credential and access ownership v1](credential-access-v1.md) taxonomy.

Providers MUST preserve the distinction between Human/management identity, Application-service credentials and BaseHarbor-internal machine identity.

A provider MUST NOT replace internal machine credentials with shared human/developer credentials, and shared provider infrastructure MUST NOT imply shared application-service credentials.

Application business users, groups, roles and permissions remain application/IdP-owned and outside the provider credential model.

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


## Standards-first provider metadata

Provider metadata MUST keep these version axes independent:

```text
BaseHarbor service contract version
Provider protocol version
Provider implementation version
Product/engine version
Artifact digest
```

A provider descriptor SHOULD identify:

- stable provider ID and provider version;
- supported service kinds and BaseHarbor service/capability contract versions;
- protocol/semantic capabilities;
- supported product/engine versions where relevant;
- supported platforms;
- OCI artifact reference and immutable digest when distributed as an artifact;
- JSON Schema 2020-12 provider configuration schema;
- standard signature/SBOM/provenance references where available.

The canonical target schema is `contracts/provider/v1/provider-descriptor.schema.json`.

## Service bindings

Provider connection outputs MUST use Service Binding Specification 1.1 well-known entry names where their semantics apply.

BaseHarbor-specific binding extensions MUST remain in a distinct versioned extension namespace and MUST NOT redefine standard names.

Secret-bearing values may remain opaque references until the trusted workload projection boundary; this does not justify alternative field names.

## Service versus protocol

A provider implements a BaseHarbor service contract and MAY declare protocol compatibility.

Examples:

- `cache` is the service; RESP is a protocol compatibility property.
- `object-storage` is the service; S3 API compatibility is a protocol/API property.
- `observability` is the service family; OpenTelemetry/OTLP is the standard telemetry protocol/data path.

Provider product names never become portable application service kinds.


## Management-surface access

Human-facing provider management surfaces follow [Management surface access v1](management-access-v1.md). Providers declare the effective authentication class and role mapping; BaseHarbor MUST NOT infer stronger authorization than the provider can enforce.

The normative shipped-provider/surface classification is [Provider and management-surface acceptance v1](provider-management-acceptance-v1.md).

## Availability

Capability providers negotiate the same portable availability requirement independently from the Runtime Provider.

Providers declare SUPPORTED, PARTIALLY_SUPPORTED or UNSUPPORTED with explicit limits. A required unsupported guarantee fails before provider mutation. Provider-native clustering, quorum, replica roles and managed-service product modes remain realization state and never enter portable Application Intent.
