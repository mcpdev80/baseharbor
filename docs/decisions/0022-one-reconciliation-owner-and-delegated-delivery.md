# ADR 0022: One reconciliation owner and delegated delivery boundaries

## Status

Accepted for the post-v0.4 architecture and required before the v0.5 contract freeze.

## Context

ADR 0011 established that Runtime, Capability and Delivery are independent axes and
that BaseHarbor separates portable application intent from how a desired runtime
realization is applied. The v0.4.27 contract layer
(`t-20261018-r27-delivery-boundary`) pins the Delivery Provider selection model into
a typed `internal/delivery/contract.go` with positive/negative fixtures, and the
delivery standards map lands in `docs/spec/delivery-provider-v1.md`.

The remaining question is operational safety: when a reconciler mutates a managed
resource set, whose intent wins if two owners appear at once?

## Decision

### 1. Exactly one active reconciliation owner per managed resource set

For one managed resource set there is exactly one active mutation/reconciliation
owner at any observed moment.

- Direct delivery: the owner is `baseharbor`.
- Delegated delivery: the owner is `external`.

### 2. Ambiguous ownership fails closed

If a selection observes both a direct `baseharbor` owner and a delegated
`external` owner active for the same resource set, the observation fails closed:
BaseHarbor reports the conflict and refuses to apply or observe until exactly one
owner is expressible in portable intent.

### 3. Direct delivery stays without GitOps

Direct delivery requires no GitOps prerequisite. Delegated delivery adds
synchronization, revision and ownership states that direct delivery does not need,
and those states stay below the Delivery Provider boundary.

## Consequences

- Two competing reconcilers for one resource set are rejected before application, not after partial mutation.
- BaseHarbor's own bindings/operation receipts remain BaseHarbor-owned regardless of delivery mode.
- GitOps systems (Argo CD, Flux) stay implementations; the one-owner invariant is portable intent, not a tool.

Trade-offs:

- Delegated delivery carries extra revision/ownership state that direct delivery omits.
- The invariant is checked at observation time; it does not prevent an operator from configuring two systems, only from BaseHarbor silently accepting the result.
