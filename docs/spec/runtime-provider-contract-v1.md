# Runtime Provider Contract v1

## Status

Foundation draft on `feature/kubernetes-runtime-foundation`. This contract is not yet frozen and is not a claim of released Kubernetes support.

## Scope

This specification defines the external, language-neutral Runtime Provider process boundary.

It is intentionally separate from:

- `spec/provider/v1/provider.proto`, which models capability/service provider lifecycle;
- Delivery Provider contracts, which own direct versus delegated reconciliation semantics;
- `spec/runtime-api/v1/openapi.yaml`, which is the application-facing Runtime Broker API;
- `internal/runtime/contract.InternalWorkloadProvider`, which is only an in-process Core seam.

Hard rule:

```text
runtime != capability != delivery
```

## Standards audit

### Existing Standards

- OCI Image / Distribution for workload artifacts.
- Compose Specification as a repository/workload source where applicable.
- Kubernetes API for Kubernetes-native realization.
- gRPC + Protocol Buffers for an external provider process boundary.
- OCI distribution for provider packaging and digest identity.

No existing standard defines the full cross-runtime BaseHarbor workload lifecycle semantics for Docker, Podman, Kubernetes and OpenShift without importing one platform's native API into the portable model.

### Adopted Standards

- OCI image references are the runtime artifact boundary.
- gRPC + Protocol Buffers define the external process protocol.
- Runtime-native APIs remain behind each provider.
- Provider packaging/distribution follows the existing OCI direction.

### BaseHarbor Extensions

The protocol defines only the missing portable semantics:

- provider identity and runtime capabilities;
- opaque deployment-owned target scope;
- normalized workload plan;
- apply / observe / logs / exec / destroy lifecycle;
- idempotent asynchronous mutation operations;
- provider-neutral diagnostics.

### Deviations

- Kubernetes objects are not used as the universal workload contract.
- Compose is not used as the universal internal IR.
- OCI Runtime/CRI are not used as BaseHarbor orchestration APIs.

### Compatibility Impact

The portable `baseharbor.yaml` contract does not change.

Repository-owned workload sources do not change.

Bundled in-process runtime providers may continue to implement the internal Go seam. An external provider is connected through an adapter between the versioned Runtime Provider protocol and that internal seam.

## Boundary

```text
Repository workload
        |
        v
Normalized Workload Model
        |
        v
Source -> OCI artifact resolution
        |
        v
Runtime Workload Plan
        |
        +------------------------------+
        |                              |
        v                              v
bundled in-process adapter      external protocol adapter
        |                              |
        v                              v
InternalWorkloadProvider        baseharbor.runtime.provider/v1
                                       |
                                       v
                                  gRPC / Protobuf
                                       |
                                       v
                                external provider
```

## Runtime plan rules

A Runtime Provider receives only runtime-consumable workload semantics.

It MUST NOT receive repository build instructions as runtime semantics. Source-backed services are resolved to OCI artifacts before this boundary.

`Target.scope` is deliberately opaque. A Kubernetes provider may realize it as a namespace, but `namespace` is not a universal BaseHarbor concept.

Bindings distinguish public values from opaque secret references. Plaintext secret material MUST NOT be persisted in portable application intent, diagnostics or provider registry metadata.

## Lifecycle

Mutating operations are idempotent and may be asynchronous:

- `Apply`
- `Destroy`

The protocol uses operation IDs and `GetOperation` rather than exposing provider-native rollout/job primitives.

Read/interactive operations are:

- `Observe`
- `Logs`
- `Exec`

`Preflight` MUST fail unsupported semantics before mutation.

## Provider implementations

The initial official runtime providers remain:

```text
Docker
Podman
Kubernetes
OpenShift
```

These are reference implementations, not a closed list.

Future community/vendor providers may implement the same protocol without patching BaseHarbor Core.
