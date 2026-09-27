# Providers

Providers realize BaseHarbor semantics without changing application intent.

## Runtime providers

Run application workloads.

Current: Compose.

Later: Kubernetes and OpenShift.

## Capability providers

Realize logical capabilities such as SQL, cache, object storage, secrets, identity or observability.

## Delivery providers

Control how desired runtime state is delivered and reconciled. Direct mutation and delegated/GitOps delivery are separate mechanisms behind the same application contract.

## Bundled and external providers

BaseHarbor currently ships first-party providers in the main repository, but they have their own provider IDs and implementation versions. The BaseHarbor release version, provider implementation version, capability specification version and concrete product version are separate facts.

Bundled providers are resolved through the same provider contract boundary that future external providers use. Moving a provider to its own repository later is therefore a packaging change, not a change to application intent.

## Placement

BaseHarbor uses one common placement model for capability providers:

- `shared`: BaseHarbor owns a provider lifecycle that can serve multiple applications;
- `application`: BaseHarbor owns a provider instance dedicated to one application/environment;
- `external`: BaseHarbor binds to infrastructure it does not own.

`shared` is the resource-efficient default where the provider can safely isolate applications. Shared provider infrastructure never means shared application data, credentials or ownership. Logical resources and bindings remain application-scoped.

For example:

```text
one shared PostgreSQL provider
├── app-a database + least-privilege role
├── app-b database + least-privilege role
└── app-c database + least-privilege role
```

BaseHarbor therefore does not need to start ten PostgreSQL providers merely because ten applications request SQL. An application can still request/demand application-scoped placement when a dedicated provider is required.

Valkey follows the same lifecycle rule while preserving stronger data isolation: the Target owns the shared provider lifecycle, while applications receive isolated cache resources and credentials. A provider MUST NOT claim `shared` support if it cannot prevent cross-application access with the normal ecosystem client boundary.

A provider declares its supported scopes. Unsupported placement fails before mutation; BaseHarbor never silently weakens isolation or changes requested ownership semantics.

### Shared lifecycle versus application resources

A shared provider owns infrastructure at the Target/provider boundary. Applications own only their logical resources and bindings.

Application destroy therefore:

1. removes that application's logical resource, identity and binding;
2. preserves sibling applications;
3. preserves the shared provider while it is still in use.

Target/provider destroy removes the provider itself.

This ownership rule applies consistently to databases, caches, object storage, secrets, identity and observability providers where their underlying products support safe multi-application realization.

For normative requirements, see [Provider contract v1](../spec/provider-contract-v1.md).
