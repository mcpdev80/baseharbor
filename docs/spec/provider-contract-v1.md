# Provider Contract v1

## Scope

This specification defines the common rules for providers.

## Provider axes

Runtime, Capability and Delivery Provider are independent axes.

```text
runtime != capability != delivery
```

Implementations MUST NOT make one axis a hidden requirement of another.

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

## Extensibility

Provider implementations MAY use mature OSS, standard APIs/SDKs, controllers/operators or managed-service APIs behind the BaseHarbor contract.
