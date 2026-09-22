# Architecture

BaseHarbor turns portable application intent into verified runtime infrastructure.

```text
Application
    ↓
Portable Intent
    ↓
Environment + Policy
    ↓
BaseHarbor Core
    ↓
Runtime + Capability + Delivery Providers
    ↓
Verified Result
```

## Portable intent

The application describes what it needs, not which infrastructure product must provide it.

Examples are SQL, cache, object storage, secrets, identity, HTTP exposure and telemetry.

## Three provider axes

BaseHarbor keeps three concerns separate:

```text
runtime != capability != delivery
```

- Runtime Provider: where workloads run.
- Capability Provider: how a logical dependency is realized.
- Delivery Provider: how desired runtime state reaches and reconciles with the runtime.

Compose is the current complete runtime. Kubernetes and OpenShift are later runtime providers.

## Environment and policy

Environment describes deployment risk and policy context. It does not identify a runtime or provider product.

## Ownership and placement

Where applicable, providers use the same placement model:

```text
application
shared
external
```

BaseHarbor mutates only resources it owns.

## Lifecycle

Mutating operations follow:

```text
plan -> preflight -> apply -> verify
```

Reconciliation compares desired state with observed provider/runtime state and fails closed on ambiguity, ownership conflicts or unsupported requirements.

## Interfaces

CLI, JSON and MCP are adapters over the same semantic core. No interface may bypass policy, ownership, verification or secret safety.

For normative behavior, use [Specs](../spec/README.md). For design rationale, use [ADRs](../decisions/).
