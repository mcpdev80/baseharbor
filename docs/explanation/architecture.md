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

The current local runtime path supports Docker through Docker Compose and Podman through native Quadlets generated from the same Compose-based workload/runtime model. Kubernetes and OpenShift are later runtime providers.

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


## Documentation boundaries

Documentation follows the same responsibility discipline as code:

```text
Human docs      -> explain use and concepts
Reference       -> exact public behavior
Specs           -> normative contracts
Schemas/code    -> machine-readable authority
ADRs            -> decisions and rationale
GitHub Issues   -> future planning
Releases        -> delivered history
```

A detailed fact should have one authoritative home. Human documentation links to deeper reference/spec material instead of duplicating it.
