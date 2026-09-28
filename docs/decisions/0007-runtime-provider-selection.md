# ADR 0007: Runtime provider selection is deployment-owned

## Status

Accepted for the v0.4.17 pre-freeze runtime boundary.

## Context

BaseHarbor must keep one portable application contract while allowing the same desired workload to be realized by different runtime mechanisms.

Runtime selection is therefore a deployment concern, not an application capability. It is separate from capability providers such as PostgreSQL, Valkey, object storage, secrets or identity, and separate from the Delivery Provider axis defined by ADR 0011.

Repository workload source and runtime provider identity are also separate concepts. A repository may use the Compose Specification as workload input without making "Compose" a runtime provider.

## Decision

BaseHarbor uses a versioned provider-neutral runtime contract in `internal/runtime`.

The runtime boundary consists of:

- a normalized provider identity;
- a versioned `ProviderDescriptor`;
- explicit provider capabilities;
- declared workload-source compatibility;
- declared runtime realization;
- a provider registry that maps descriptors to factories;
- one provider-neutral `RuntimeProvider` execution contract.

The current contract version is:

```text
baseharbor.runtime/v1
```

Provider selection comes from Target/deployment state. Missing provider metadata for a new local Target resolves to `docker`.

The two implemented local providers are:

```text
Portable application/workload semantics
                 |
        RuntimeProvider contract
          /                 \
         /                   \
DockerProvider           PodmanProvider
     |                        |
Docker Compose        Quadlet + systemd --user
```

Repository Compose remains a workload-source standard:

```text
Repository workload source
        |
Compose Specification
        |
normalized semantics
        |
selected RuntimeProvider
```

It is not a provider identity.

## Provider registry

Provider IDs are normalized extensible identifiers rather than a closed product enum. First-party Docker and Podman providers are registered in the default registry, but the registry contract can accept an additional provider descriptor and factory without changing portable application intent.

A provider registration fails closed when:

- the provider ID is invalid or not normalized;
- the runtime contract version is incompatible;
- the provider version is missing;
- workload-source compatibility is undeclared;
- runtime realization is undeclared;
- the provider factory is missing;
- a provider ID is registered twice;
- the realized provider descriptor does not match its registration.

Kubernetes and OpenShift remain named future provider targets but are not registered as executable providers in v0.4.17.

## Capability rule

A runtime provider advertises behavior orchestration may depend on. The initial contract includes:

- workload lifecycle;
- service exec;
- published-port inspection;
- provider-owned resource ownership checks.

Capability negotiation is versioned and fail-closed. BaseHarbor does not silently reduce lifecycle, security or verification guarantees when a provider lacks required behavior.

Provider capabilities are runtime mechanics, not application requirements. For example, `database.sql` remains the same portable application capability regardless of whether its implementation is application-scoped, shared, external, Docker-backed, Podman-backed or later Kubernetes-backed.

## Docker

`docker` is the default local runtime provider.

Its current workload-source compatibility is `compose-spec`, realized through Docker Compose.

Docker-specific execution stays inside the runtime implementation. Portable orchestration must not branch on Docker product identity.

## Podman

`podman` is realized natively through Quadlet and the user `systemd` manager.

The provider consumes Compose Specification workload input, renders the required Quadlet units and operates them through `systemd --user` and Podman.

There is no `podman compose` fallback. If Podman, Quadlet or the required user-systemd environment is unavailable, provider detection fails closed.

## Kubernetes and OpenShift implication

Kubernetes and OpenShift are later Runtime Provider implementations, not changes to portable application intent.

A future provider may realize lifecycle through native resources such as Deployments, StatefulSets, Services, Jobs, PersistentVolumeClaims, NetworkPolicies, Gateway API resources or OpenShift-specific resources. Those objects remain provider implementation details.

Kubernetes/OpenShift support must register a compatible provider descriptor/factory and satisfy the same portable runtime contract. It must not require Docker/Podman branches in Core.

## Boundary enforcement

A static architecture test scans productive Core packages and rejects:

- legacy `ProviderCompose` / `DetectCompose` identifiers;
- concrete `runtime.Compose`, `DockerProvider` or `PodmanProvider` references from Core;
- direct concrete Docker/Podman runtime-package imports;
- product `Engine()` access from portable Core.

The runtime package also carries a reusable conformance harness for first-party providers.

## Compatibility

v0.4.17 intentionally does not retain the old `compose` runtime-provider identity. There are no production BaseHarbor installations requiring that migration path.

This decision does preserve:

- portable application manifest semantics;
- repository Compose workload input;
- application-facing environment and Service Binding contracts;
- backup/restore identity;
- Docker Compose realization;
- Podman Quadlet realization.

## Consequences

Positive:

- runtime provider identity is no longer conflated with Compose input;
- Docker and Podman use the same portable execution boundary;
- Podman has no hidden Compose fallback;
- provider discovery is registry/descriptor driven rather than a product switch;
- later providers can extend the registry without changing portable intent;
- architectural regression is guarded by tests.

Trade-offs:

- `RuntimeProvider` remains a substantial lifecycle contract because Docker and Podman already prove those operations are shared;
- Kubernetes and OpenShift must demonstrate conformance before they can be registered;
- provider-specific realization still exists internally, as intended, but may not leak back into Core.
