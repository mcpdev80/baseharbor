# BaseHarbor specifications

This directory contains normative public contracts and invariants.

Human guides explain how to use BaseHarbor. Specifications define what implementations must guarantee.

BCP 14 terms such as MUST, MUST NOT, SHOULD and MAY are used only where interoperability, compatibility or security requires them.

Prefer machine-readable authority where practical:

- JSON Schema / typed schema for manifests and descriptors;
- OpenAPI for REST;
- Protocol Buffers for external provider process boundaries;
- typed machine result/error schemas;
- executable conformance fixtures.

## Core contracts

- [Application Contract v1](application-contract-v1.md)
- [Provider Contract v1](provider-contract-v1.md)
- [Runtime Provider Contract v1](runtime-provider-contract-v1.md)
- [Runtime Provider Conformance v1](runtime-provider-conformance-v1.md)
- [Delivery Provider Contract v1](delivery-provider-contract-v1.md)
- [Development Extension v1](development-extension-v1.md)
- [Reconciliation v1](reconciliation-v1.md)
- [Machine Interface v1](machine-interface-v1.md)
- [Audit & Evidence v1](audit-evidence-v1.md)
- [Security invariants](security-invariants.md)
- [Credential and access ownership v1](credential-access-v1.md)

## Runtime and access contracts

- [Control-plane runtime](control-plane-runtime.md)
- [Application Runtime Broker](application-runtime-broker.md)
- [Application runtime identity](application-runtime-identity.md)
- [Runtime broker security](runtime-secret-broker-security.md)
- [Service access](service-access.md)
- [Service access inventory](service-access-inventory.md)

## Standards

- [Standards-first contract rules](standards-first.md)
- [Standards matrix](standards-matrix.md)

Machine-readable service schemas and capability specifications live with the versioned contract artifacts in the repository.
