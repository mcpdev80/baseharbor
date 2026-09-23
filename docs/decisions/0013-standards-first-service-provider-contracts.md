# ADR 0013: Standards-first service and provider contracts

Status: Accepted

Date: 2026-09-23

## Context

BaseHarbor already uses several open standards and mature ecosystem patterns, but some capability and binding semantics evolved locally before the service/provider boundary was fully frozen.

The v0.5.x line is the contract-freeze track. The architecture therefore needs one explicit rule for deciding when BaseHarbor adopts an existing standard and when it defines BaseHarbor-specific semantics.

## Decision

BaseHarbor is standards-first.

- Established open standards are preferred over proprietary contracts.
- Established de-facto standards and ecosystem conventions are preferred where no suitable formal standard exists.
- BaseHarbor-specific contracts define only semantics not reasonably covered by an existing standard.
- BaseHarbor extensions are explicit, versioned and provider-neutral.
- Provider-specific contracts stay behind the provider boundary.

The initial adopted baseline is:

- JSON Schema 2020-12 for portable schemas and provider configuration;
- Service Binding Specification 1.1 well-known names for service connection outputs;
- Crossplane resource/provider/reconciliation concepts as architecture guidance;
- Open Service Broker lifecycle concepts where applicable;
- gRPC/Protocol Buffers for external provider process boundaries;
- OCI for provider artifact distribution and digest identity;
- OpenTelemetry/OTLP for observability data;
- OIDC/OAuth for identity;
- AsyncAPI and CloudEvents for messaging/event contracts where applicable;
- RESP as a compatibility property of Redis/Valkey-like cache providers;
- S3 API compatibility as a property of object-storage providers.

Service contract, provider protocol, provider implementation version, product/engine version and artifact digest are separate compatibility axes.

Existing v0.4 capability IDs remain supported until an explicit migration is documented and tested. Standards-first alignment is not permission to perform breaking renames.

## Consequences

- Service kinds stay product-neutral.
- Protocol compatibility does not become service identity.
- Runtime provider-instance state remains separate from provider catalog/distribution metadata.
- New service kinds/providers require a standards audit before implementation.
- Conformance can test adopted standards separately from BaseHarbor extensions.
- Future provider repositories can be extracted without changing application intent.
