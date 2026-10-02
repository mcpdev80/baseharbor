# Providers

Providers realize BaseHarbor semantics without changing portable Application Intent.

## Runtime providers

Runtime Providers run application workloads.

Current reference runtimes:

- Docker;
- rootless Podman.

Compose remains a workload-source/input model for the local runtime path. It is not the portable Runtime Provider identity.

Future Runtime Providers include Kubernetes and OpenShift.

The v0.4.19 Runtime boundary deliberately does not require those future providers to imitate Compose, Docker or Podman mechanics.

## Capability providers

Capability Providers realize logical application dependencies.

Examples include:

- `database.sql`;
- `cache.key-value`;
- `database.key-value`;
- `database.document`;
- `messaging.queue`;
- `messaging.pubsub`;
- `messaging.stream`;
- `object-storage.s3`;
- secrets;
- OIDC identity;
- metrics, logs, traces and OTLP;
- HTTP exposure.

Reference products prove these contracts. Product names do not become portable capability names.

## Delivery providers

Delivery Providers control who owns reconciliation of desired runtime state.

The portable distinction is:

- `direct` — BaseHarbor owns reconciliation;
- `delegated` — an external reconciler owns reconciliation.

Runtime, capability and delivery remain separate axes:

```text
runtime != capability != delivery
```

## Bundled and external providers

Bundled providers ship with BaseHarbor but retain their own provider identity/version.

External/BYO providers let BaseHarbor consume infrastructure it does not own.

Removing an external provider registration removes the BaseHarbor reference/binding and must not destroy the foreign service.

## Placement

Capability providers use the common placement model:

- `shared` — Target/provider-owned infrastructure serving isolated application resources;
- `application` — dedicated provider lifecycle for one application/environment;
- `external` — infrastructure lifecycle remains outside BaseHarbor.

Shared infrastructure never means shared application credentials, ownership or unpartitioned data.

For example:

```text
one shared PostgreSQL provider
├── app-a database + least-privilege role
├── app-b database + least-privilege role
└── app-c database + least-privilege role
```

The same ownership rule applies to other provider families where the product can safely implement shared placement.

## Stable identity

Provider ownership does not depend on container names, repository paths or future Kubernetes object names.

```text
provider_instance_id != application_id != deployment_id
```

Application-scoped provider ownership follows the stable `application_id`.

## Related documentation

- [Provider CLI](../cli/providers.md)
- [External / BYO providers](../how-to/external-providers.md)
- [Provider Contract v1](../spec/provider-contract-v1.md)
- [Runtime Provider Contract v1](../spec/runtime-provider-contract-v1.md)
- [Delivery Provider Contract v1](../spec/delivery-provider-contract-v1.md)
