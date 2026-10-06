# Runtime Provider Contract v1

## Status

Pre-freeze v0.4.19 architecture contract. This defines the portable Runtime Provider boundary and is not a claim of Kubernetes production support.

## Scope

Runtime Provider is a distinct provider axis.

```text
runtime != capability != delivery != workload source
```

It is intentionally separate from:

- capability/service provider lifecycle
- Delivery Provider selection and reconciliation ownership
- repository workload source detection and build
- application-facing Runtime Broker APIs
- local Docker/Podman project-file/container mechanics

## Standards

BaseHarbor adopts existing standards where they fit:

- OCI Image / Distribution for runtime artifacts
- Compose Specification as one repository workload source
- Kubernetes/OpenShift APIs as provider-native realization APIs
- gRPC + Protocol Buffers for the external Runtime Provider process boundary
- OCI distribution for provider packaging and immutable provider identity

No existing standard defines the complete portable workload lifecycle BaseHarbor needs across Docker, Podman, Kubernetes and OpenShift without importing one platform's native object model into Core.

## Boundary

```text
Repository workload source
        |
        v
Development / source normalization
        |
        v
Build + artifact resolution
        |
        v
Resolved OCI workload plan
        |
        +-------------------------------+
        |                               |
        v                               v
bundled in-process adapter       external protocol adapter
        |                               |
        v                               v
semantic runtime lifecycle       baseharbor.runtime.provider/v1
                                         |
                                         v
                                   provider-native API
```

Hard invariants:

- repository source/build instructions end before the Runtime Provider boundary
- every runnable service reaches Runtime as an already-resolved OCI image
- `Target.scope` is opaque; `namespace`, Compose project and systemd unit naming are provider realization details
- runtime-native object names and rollout conditions never become Application identity
- container endpoints are workload semantics; public/host exposure remains a capability/delivery concern
- capability provisioning is not a Runtime Provider responsibility
- direct vs delegated/GitOps reconciliation is not a Runtime Provider responsibility
- plaintext secret material is not portable Runtime Provider state

## Lifecycle

Portable semantic operations are:

- Preflight
- Apply
- Wait/observe readiness
- Observe
- Logs
- Exec
- Destroy

Mutating operations must be idempotent. External providers use operation IDs for asynchronous mutation.

## Local realization

Docker and Podman may retain richer local mechanics below the semantic boundary, including:

- project files
- Compose input
- Quadlets
- containers
- volumes
- local networks
- runtime-specific diagnostics

Those mechanics are implementation details and MUST NOT expand the portable Runtime Provider contract.

## Kubernetes/OpenShift compatibility

A Kubernetes/OpenShift implementation must be able to consume the same portable runtime plan without adding Kubernetes/OpenShift fields to portable Application Intent.

Namespace-scoped operation without cluster-admin, Namespace creation, CRDs or Operators must remain possible for the adoption path. Provider-native enhancements can be added below this boundary later.


## Availability negotiation

Runtime Providers declare the availability guarantee their realization can actually satisfy and verify.

The portable application request remains global/sparse availability intent. Runtime-native replica objects, scheduling constraints, failure domains and platform topology remain provider realization state.

A Runtime Provider that cannot satisfy an effective HA request returns a typed actionable UNSUPPORTED result before mutation. It MUST NOT silently downgrade the request.

One logical workload component may be observed as 0..N runtime instances. Observation retains all instances and aggregates readiness against the requested guarantee without changing component identity.

The bundled Docker Compose and Podman Quadlet providers are explicitly UNSUPPORTED for verified HA workload orchestration in v0.4.21. Future Kubernetes/OpenShift providers can implement the same semantics without changing portable Application Intent.
