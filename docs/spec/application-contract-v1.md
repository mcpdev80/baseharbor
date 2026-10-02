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

## Workload component identity

Portable workload identity is expressed as stable logical components, for example `api`, `worker` or `web`.

Logical component identity MUST remain independent from repository-source-native identity:

```text
logical component: api

Compose:      services.api
Quadlet:      api.container
Kubernetes:   Deployment/api
```

Repository workload-source kind, source path, Kubernetes kind/name, Quadlet filename and Compose service syntax are source provenance and MUST NOT be required as portable Application Intent.

Repository adoption MAY use a separate safe-to-commit `baseharbor.repository.yaml` only when an authoritative source selection must be persisted. That metadata is repository authoring information, not Application Intent.

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
