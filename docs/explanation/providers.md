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

## Placement

Where applicable:

- `application`: BaseHarbor owns the application-scoped provider lifecycle;
- `shared`: multiple applications use an explicitly shared provider boundary;
- `external`: BaseHarbor binds to infrastructure it does not own.

A provider must declare what it supports. Unsupported required semantics fail before mutation.

For implementation requirements, see [Provider contract v1](../spec/provider-contract-v1.md).
