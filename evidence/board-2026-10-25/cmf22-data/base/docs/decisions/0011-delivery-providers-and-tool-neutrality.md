# ADR 0011: Delivery providers, delegated reconciliation and tool neutrality

## Status

Accepted for the post-v0.4 architecture and required before the v0.5 contract freeze.

## Context

BaseHarbor already separates portable application intent from Runtime Providers and Capability Providers. The complete current local runtime path uses Docker Compose for Docker and native Quadlets for Podman while preserving Compose-based repository compatibility; Kubernetes and OpenShift are later runtime implementations. Capability providers realize logical needs such as SQL, S3, secrets, observability and exposure.

A second question is independent from both axes: **who delivers and reconciles the desired runtime realization?**

For the current local runtime, BaseHarbor performs runtime mutation directly: Docker through Docker Compose and Podman through generated Quadlet units. On Kubernetes/OpenShift, direct API mutation is valid, but established GitOps systems such as Argo CD or Flux may own continuous reconciliation and provide valuable native operational views.

Making Argo CD, Flux, Helm, Git or Kubernetes resources part of portable application intent would create product/runtime lock-in. Making BaseHarbor implement every mature operational controller itself would duplicate established OSS and make the BaseHarbor Core responsible for product-specific integration churn.

At the same time, BaseHarbor is intended to become an open ecosystem. Community, vendors and companies must be able to implement integrations without patching BaseHarbor Core. The existing Provider Integration Contract direction based on versioned semantics, gRPC/Protocol Buffers where a process boundary is required, OCI distribution, conformance and open supply-chain standards remains strategically important.

## Decision

### 1. Runtime, Capability and Delivery are independent axes

```text
                         BaseHarbor Core
                               |
             +-----------------+-----------------+
             |                 |                 |
             v                 v                 v
       Runtime Provider  Capability Provider  Delivery Provider
             |                 |                 |
 Docker Compose / Podman   SQL / S3         direct
            Quadlet         secrets          delegated/GitOps
        Kubernetes          telemetry
         OpenShift
```

Hard rule:

```text
runtime != capability != delivery
```

- Runtime Provider: where/how workload runtime primitives are realized.
- Capability Provider: how a logical application capability is realized.
- Delivery Provider: how desired runtime realization is applied/reconciled to the selected runtime.

None of these selections is portable application intent.

### 2. Direct and delegated delivery are first-class semantics

Direct delivery:

```text
BaseHarbor -> Runtime API
```

BaseHarbor owns mutation/reconciliation for the managed resource set.

Delegated delivery:

```text
BaseHarbor
    -> desired runtime realization
    -> Delivery Provider
    -> external reconciler
    -> Runtime
```

BaseHarbor still owns portable semantics, policy, provider selection, lifecycle intent, observation, semantic verification and evidence. The delegated reconciler owns runtime mutation for the delegated resource set.

### 3. Exactly one reconciliation owner

For one managed resource set there is exactly one active mutation/reconciliation owner.

BaseHarbor must not directly mutate fields/resources that a delegated reconciler owns during normal operation. Ambiguous or competing reconciliation ownership fails closed.

### 4. Placement semantics are consistent across provider families

Where applicable, Delivery Providers use the same canonical placement semantics as Capability Providers:

```text
application
shared
external
```

The semantics remain consistent:

- application: one app/environment, BaseHarbor-owned lifecycle where supported;
- shared: explicitly shared provider with an explicit sharing boundary;
- external: provider lifecycle is owned outside BaseHarbor.

Consistent placement semantics do not merge provider responsibilities.

### 5. Contracts, not tools

BaseHarbor owns stable semantics and contracts. Products and tools are implementations.

Argo CD may be a reference implementation of a GitOps Delivery Provider. Flux or another conforming implementation must remain possible without portable-contract changes.

Likewise, provider implementations may internally use mature OSS, open standards, standard APIs/SDKs, controllers/operators/CRDs or managed-service APIs. These mechanisms stay behind the provider boundary.

### 6. Open provider ecosystem remains strategic

BaseHarbor must not become the integration bottleneck.

Third parties must be able to implement providers without modifying BaseHarbor Core.

The strategic open ecosystem direction remains:

```text
BaseHarbor versioned contract/specification
        ↓
Provider Integration Contract
        ↓
gRPC / Protocol Buffers where a process boundary is required
        ↓
OCI distribution
        ↓
independent community/vendor/company provider
```

OCI registries, digest pinning, signatures/provenance and conformance are preferred over a proprietary BaseHarbor marketplace/package format.

A provider may internally delegate to another OSS tool or platform API. That is an implementation choice, not a reason to bypass the BaseHarbor provider boundary.

### 7. BaseHarbor-first developer experience

The normal developer and agent interfaces remain:

```text
baha
JSON
MCP
```

Users should not need `argocd`, Flux-specific CLIs, `kubectl`, Helm or provider-native infrastructure CLIs for normal BaseHarbor lifecycle operations.

Native tool UIs and CLIs remain available for platform engineers and expert drill-down. BaseHarbor hides operational complexity, not operational capability.

### 8. Compose compatibility and Podman Quadlet remain first-class

Compose-based repository/runtime definitions must not depend on Kubernetes, GitOps, Argo CD, Flux, CRDs, cloud APIs or other later-runtime mechanisms.

The complete current local paths remain conceptually:

```text
runtime: docker  -> Docker Compose
runtime: podman  -> generated Quadlet + systemd --user
delivery: direct
```

The Delivery Provider abstraction must be runtime-neutral enough not to prohibit a future non-Kubernetes delegated-delivery use case, but BaseHarbor does not implement speculative paths without demonstrated need.

## Consequences

Positive:

- BaseHarbor can integrate with GitOps without making Argo CD/Flux part of application intent.
- Argo CD UI and native platform tooling remain useful.
- Compose remains independent and first-class.
- vendors/community can ship integrations without Core patches.
- mature OSS can be reused behind providers instead of reimplemented.
- product/version churn stays outside BaseHarbor Core.
- the same placement, ownership, lifecycle, verification and evidence principles continue across provider families.

Trade-offs:

- BaseHarbor must define delivery semantics carefully enough to avoid false abstraction.
- delegated delivery adds synchronization/revision/ownership states that direct delivery does not need.
- BaseHarbor must distinguish tool-reported sync/health from BaseHarbor semantic verification.
- provider conformance becomes more important because integrations can live outside the Core.

## Acceptance implications

Before v0.5 freezes the core contract:

- deployment/operator state can represent Delivery Provider selection and reconciliation ownership;
- direct versus delegated delivery is expressible without product names in portable intent;
- typed result models can represent delivery synchronization/conflict/unavailable states;
- application/shared/external placement semantics remain reusable;
- Compose direct delivery remains unchanged.

The real delegated/GitOps reference implementation is scheduled for v0.7.8 (#325), with Argo CD as reference evidence rather than a contract dependency.
