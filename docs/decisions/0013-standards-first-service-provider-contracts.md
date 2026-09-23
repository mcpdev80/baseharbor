# ADR 0013: Standards-first service and provider contracts

Status: Accepted

Date: 2026-09-23

## Context

BaseHarbor already separates application intent from provider products and uses open building blocks such as gRPC/Protocol Buffers, OCI and OpenTelemetry. Several shipped v0.4 capability specifications were created before the full standards inventory was made explicit.

Without a permanent rule, later service kinds could accidentally duplicate existing standards or encode provider products into the BaseHarbor core.

## Decision

BaseHarbor is standards-first.

- Established open standards are preferred whenever they cover the required semantics.
- Established de-facto standards and ecosystem conventions are preferred where no suitable formal standard exists.
- BaseHarbor-specific contracts define only missing semantics.
- BaseHarbor extensions are explicit, versioned and provider-neutral.
- Product-specific provider contracts never become portable BaseHarbor application contracts.

The canonical architecture distinguishes:

```text
service contract
  -> protocol/semantic compatibility
  -> provider implementation
  -> product/engine realization
```

These layers are independently versioned.

JSON Schema 2020-12 is the machine-readable service/provider configuration schema language.

Service connection outputs use Service Binding Specification 1.1 well-known names where applicable.

Crossplane and Open Service Broker API are architecture/lifecycle references, not imported public BaseHarbor APIs.

OCI is the provider artifact/distribution boundary.

OpenTelemetry/OTLP is the observability data-plane standard.

OIDC/OAuth is the identity baseline.

AsyncAPI and CloudEvents are the messaging/event baselines.

RESP and S3 are protocol/de-facto compatibility declarations, not generic service identities.

## Compatibility

Existing v0.4 specification IDs are not silently renamed.

Before v0.5 freeze, each existing specification is classified as:

- canonical service contract;
- protocol/semantic capability under a broader service kind; or
- compatibility alias with an explicit migration path.

## Consequences

- Provider implementations remain replaceable.
- Existing BaseHarbor lifecycle/ownership/reconciliation semantics remain valid.
- `secure-binding/v1` remains for BaseHarbor security/lifecycle extensions but no longer competes with standard Service Binding connection names.
- Future providers must document standards adoption and deviations before implementation.
