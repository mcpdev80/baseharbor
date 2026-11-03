# Delivery Provider Contract v1

## Contract

Protocol identity:

```text
baseharbor.delivery/v1
```

Delivery is independent from Runtime and Capability Providers.

```text
runtime != capability != delivery
```

The Delivery Provider decides **how** a chosen desired runtime realization is applied
and reconciled to the selected Runtime Provider. It never decides the Runtime
Provider, nor what portable application intent is.

This page maps the applied standards, narrows the unavoidable BaseHarbor
extensions, and lists the tool feature/version checks that gate each path. The
typed owner/selection contract and its fixtures live in
`internal/delivery/contract.go` and `internal/delivery/contract_test.go`.

## Modes

### direct

BaseHarbor owns reconciliation.

- owner: `baseharbor`
- provider: `direct`

### delegated

An external reconciler owns reconciliation.

- owner: `external`
- provider: implementation identifier such as a future Argo CD or Flux provider

Exactly one reconciliation owner is valid for a managed resource set. A conflicting
observation (two active owners for the same resource set) fails closed at the
deployment apply path, enforced by internal/delivery guard
(internal/deployment.SaveDeploymentRecord) before any state is written.

## Placement

Where applicable Delivery uses the same placement vocabulary as other provider axes:

- `application` — one app/environment, BaseHarbor-owned lifecycle where supported;
- `shared` — explicitly shared provider with an explicit sharing boundary;
- `external` — provider lifecycle owned outside BaseHarbor.

## Adopted standards

BaseHarbor reuses the tools and standards operators already run behind the
Delivery Provider boundary. It does not introduce a universal bundle, deployment
package, stage DSL, builder, registry server, scheduler, approval engine or
pipeline engine.

| Applied standard | Where it lives | BaseHarbor role |
| --- | --- | --- |
| OCI Image / OCI Distribution + image-layout | image references, digests, pinning | resolve and pin; never host a registry |
| Docker Compose | `runtime: docker` | emit compose definitions; Compose remains independent and first-class |
| Podman Quadlet / systemd `--user` | `runtime: podman` | emit quadlet units; systemd `--user` remains supported |
| Kubernetes / OpenShift native APIs | delegated or direct K8s path | emit portable manifest; platform controllers reconcile |
| Git / Helm / Kustomize | delegated manifest sources | consume as input; not portable application intent |
| Existing CI / GitOps (Argo CD, Flux, …) | external reconciler | optional delegation target, never portable intent |

Hard rule: none of these products enter portable application intent, and the
selection provenance and immutable revision may be recorded without changing it.

Argo CD and Flux are possible implementations, not contract semantics.

## Unavoidable BaseHarbor extensions

The extensions BaseHarbor retains are the ones portable intent requires; every
other operational concern stays behind the provider boundary.

- Portable selection model: mode, provider, owner, placement, revision, provenance.
- Exactly-one reconciliation ownership per managed resource set.
- Portable lifecycle intent, policy and selection for delivery.
- Semantic observation and verification of the realized resource.
- Operation receipts and observations BaseHarbor owns for its own bindings.

Direct mode remains available without any GitOps prerequisite.

## Tool feature/version checks

Each applied path is gated by a tool feature/version check before it is used.
A path whose feature or version is absent is reported as unsupported and fails
closed; it is never silently downgraded.

- Compose: a `compose` CLI (or equivalent) at the feature set the emitted
  definitions require.
- Quadlet: a `podman` CLI at the feature set generated units require, plus a
  `--user` systemd instance for `runtime: podman`.
- Kubernetes/OpenShift: a platform API reachable at the resource subset emitted;
  CRDs and controllers stay behind the provider boundary.
- OCI: digest pinning and provenance support where the path relies on them.

## Contract fixtures

The typed selection contract is exercised by positive/negative fixtures in
`internal/delivery/contract_test.go`. The minimal valid selections are:

```json
{
  "mode": "direct",
  "provider": "direct",
  "owner": "baseharbor",
  "placement": "application"
}
```

```json
{
  "mode": "delegated",
  "provider": "gitops/argocd",
  "owner": "external",
  "placement": "external"
}
```

Denials the typed contract enforces:

- direct delivery requires owner `baseharbor`;
- delegated delivery requires owner `external`;
- delegated delivery cannot use provider `direct`;
- direct delivery cannot use a non-`direct` provider;
- empty or unknown owner is rejected.

## Hard invariants

- Delivery Provider products never enter portable Application capability intent.
- Runtime Provider does not decide direct/delegated ownership.
- Capability Provider does not decide direct/delegated ownership.
- direct and delegated ownership cannot be active simultaneously for the same
  managed resource set.
- provider-native GitOps resources remain below the Delivery Provider boundary.
- selection provenance and immutable revision may be recorded without changing
  portable Application Intent.

Argo CD and Flux are possible implementations, not contract semantics.
