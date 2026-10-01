# ADR 0017: Stable application and deployment identity

## Status

Accepted for v0.4.19.

## Context

Human-readable application names, environments, target names, repository paths and runtime-specific names are useful selectors, but they are mutable. They must not be the durable primary key for BaseHarbor ownership, reconciliation or persisted deployment state.

Before the v0.5 contract freeze, BaseHarbor needs one final identity model without carrying compatibility aliases for obsolete pre-freeze state semantics.

## Decision

BaseHarbor separates stable technical identity from readable attributes.

### Application identity

Each application has an opaque UUIDv4 `application_id`.

The ID is owned by the portable repository contract and is stored as `app.id` in `baseharbor.yaml`.

It is generated automatically by BaseHarbor onboarding flows. Users do not need to type or manage it during normal CLI workflows.

Changing the application name, repository directory or runtime realization does not change `application_id`.

Portable/persisted application manifests must contain a valid `app.id`.

### Deployment identity

Each realized application/environment deployment has an opaque UUIDv4 `deployment_id`.

The ID is owned exclusively by BaseHarbor Target/deployment state and is not part of portable application intent.

Deployment state is keyed by:

```text
targets/<target>/deployments/<deployment_id>/
```

The deployment record contains both `deployment_id` and `application_id` plus readable target/application/environment attributes.

Application name and environment remain CLI selectors and display attributes. They are not the deployment primary key.

### Provider identity

Shared provider instance identity is independent from both application and deployment identity.

```text
provider_instance_id != application_id != deployment_id
```

Application-scoped provider ownership is keyed by `application_id`. Shared providers retain their own stable provider-instance/sharing-boundary identity and may serve multiple applications.

### Machine and evidence surfaces

JSON, MCP, status, doctor, audit and evidence may expose `application_id` and `deployment_id` for durable correlation while human CLI workflows continue to prefer readable names.

Portable backup identity carries `application_id`. `deployment_id` remains Target-local and is recorded only in local backup/recovery metadata where deployment correlation is required.

## Rename semantics

An application rename preserves `application_id` and the existing `deployment_id`.

Repository path changes do not change application identity.

If more than one deployment claims the same `application_id` and environment on one Target, BaseHarbor fails closed instead of choosing or adopting one implicitly.

Incomplete deployment state is reconstructed only from the opaque deployment-state root and protected manifest state. BaseHarbor does not infer technical identity from mutable names.

## Compatibility

This decision replaces obsolete pre-freeze name-derived deployment identity.

No legacy state mode, migration alias or compatibility shim is provided before the v0.5 contract freeze.

## Consequences

Runtime/provider names, Compose project names and future Kubernetes/OpenShift object names remain realization details rather than authoritative identity.

A rename can update readable metadata without re-owning application-scoped resources or replacing shared providers.

Future Kubernetes/OpenShift providers can reuse the same application/deployment identity boundary without making cluster object names part of the portable contract.
