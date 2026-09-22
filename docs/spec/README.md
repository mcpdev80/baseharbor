# BaseHarbor specifications

This directory contains normative contracts.

Human guides explain how to use BaseHarbor. These specifications define what implementations must guarantee.

BCP 14 terms such as MUST, MUST NOT, SHOULD and MAY are used only where interoperability, compatibility or security requires them.

Prefer machine-readable authority where practical:

- JSON Schema / typed schema for manifests;
- OpenAPI for REST;
- Protobuf for provider process boundaries;
- typed machine result/error schemas;
- executable conformance fixtures.

Current specs:

- [Application contract v1](application-contract-v1.md)
- [Provider contract v1](provider-contract-v1.md)
- [Reconciliation v1](reconciliation-v1.md)
- [Machine interface v1](machine-interface-v1.md)
- [Security invariants](security-invariants.md)
