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

## Modes

### direct

BaseHarbor owns reconciliation.

- owner: `baseharbor`
- provider: `direct`

### delegated

An external reconciler owns reconciliation.

- owner: `external`
- provider: implementation identifier such as a future Argo CD or Flux provider

Exactly one reconciliation owner is valid for a managed resource set.

## Placement

Where applicable Delivery uses the same placement vocabulary as other provider axes:

- `application`
- `shared`
- `external`

## Hard invariants

- Delivery Provider products never enter portable Application capability intent.
- Runtime Provider does not decide direct/delegated ownership.
- Capability Provider does not decide direct/delegated ownership.
- direct and delegated ownership cannot be active simultaneously for the same managed resource set.
- provider-native GitOps resources remain below the Delivery Provider boundary.
- selection provenance and immutable revision may be recorded without changing portable Application Intent.

Argo CD and Flux are possible implementations, not contract semantics.
