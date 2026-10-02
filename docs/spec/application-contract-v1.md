# Application Contract v1

## Scope

This specification defines the stable portable application-intent boundary.

## Requirements

- Portable intent MUST describe application requirements rather than infrastructure product selection.
- Runtime-specific realization details MUST NOT be required in portable intent.
- Provider-specific configuration MUST NOT become portable intent unless it represents a demonstrated portable application semantic.
- Generated credentials and protected deployment state MUST NOT be stored as ordinary portable intent.
- Unsupported required semantics MUST fail before mutation.

## Stable identity

Portable/persisted manifests MUST contain a valid opaque UUIDv4 `app.id`.

`app.id` is the stable `application_id`.

Human-readable application name, environment, repository path, Compose project name and runtime-native object names MUST NOT replace `application_id` as durable ownership identity.

Changing a readable application name or repository path MUST NOT implicitly create a new application identity.

Deployment identity remains Target-local protected state and MUST NOT become portable Application Intent.

## Capability intent

Application capability requirements are product-neutral.

The v1 service family includes, among others:

- SQL;
- reconstructable key-value cache;
- durable key-value database;
- document database;
- queue messaging;
- pub/sub messaging;
- stream messaging;
- object storage;
- managed secrets;
- OIDC identity;
- HTTP exposure;
- metrics/logs/telemetry semantics.

Reference products MAY implement multiple logical contracts, but product reuse MUST NOT merge distinct application semantics. In particular, durable `database.key-value` remains distinct from `cache.key-value`.

## Provider/runtime separation

Portable Application Intent MUST NOT encode:

- Docker/Podman/Kubernetes/OpenShift realization details;
- provider product names where a portable capability exists;
- provider placement/runtime selection;
- Delivery Provider ownership;
- private keys, generated credentials or machine-local trust paths.

## Compatibility

Breaking changes to the public contract require explicit contract versioning once the contract is frozen.

Pre-freeze behavior that conflicts with the accepted v1 architecture may be replaced rather than preserved through compatibility aliases.
