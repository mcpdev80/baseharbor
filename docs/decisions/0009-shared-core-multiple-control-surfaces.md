# ADR 0009: Shared core with multiple control surfaces

- Status: Accepted
- Date: 2026-09-18

## Context

BaseHarbor currently exposes its application lifecycle primarily through the `baha` CLI and realizes workloads through Compose.

The product roadmap also includes:

- a lightweight Web UI for normal application/platform operations;
- a stable HTTP API for automation and remote operation;
- Kubernetes and OpenShift runtime providers;
- a Kubernetes/OpenShift Operator that continuously reconciles BaseHarbor application desired state.

These surfaces must not become independent implementations of the same lifecycle.

If CLI commands, HTTP handlers and Kubernetes reconcilers each implement their own plan/apply/status/doctor/backup rules, BaseHarbor would accumulate divergent behavior, security policy and readiness semantics. It would also make the portable application contract much harder to preserve across local Compose and cluster environments.

## Decision

BaseHarbor will use **one shared domain/lifecycle core with multiple control surfaces**.

Conceptually:

```text
                         BaseHarbor Core
              +--------------------------------+
              | application/domain models      |
              | PortableContract                |
              | input resolution                |
              | lifecycle / convergence         |
              | plan / preflight / apply        |
              | verify / status / diagnostics   |
              | recovery / update semantics     |
              | provider selection/capabilities |
              +---------------+----------------+
                              |
          +-------------------+-------------------+
          |                   |                   |
          v                   v                   v
       baha CLI            HTTP API        Operator controllers
                              |
                              v
                          lightweight
                            Web UI
```

The control surfaces are adapters over the same application services and domain models.

### CLI

`baha` remains the primary local developer/operator interface.

Local operation may invoke the shared core in-process. A future remote/context mode may use the BaseHarbor HTTP API, but the CLI must not become the location of unique lifecycle business logic.

### HTTP API

The API is a transport and authorization boundary, not a second lifecycle implementation.

It exposes machine-readable plans, inputs, status, diagnostics and lifecycle actions backed by the same shared core used by `baha`.

The API must not implement lifecycle semantics by shelling out to `baha`.

### Web UI

The Web UI is intentionally lightweight and uses the BaseHarbor API.

It may render operations such as:

- application overview and readiness;
- plan/preflight/apply/up/down;
- doctor and remediation;
- logs;
- declarative deployment inputs;
- secret presence/usability metadata;
- backup/restore;
- update;
- provider/platform health.

The UI must not bypass normal policy, validation, preflight, verification or secret-handling rules.

Secret values are not displayed by default. Any future reveal/elevated operation requires explicit policy and authorization.

The Web UI must not invoke shell commands or reimplement lifecycle decisions in frontend code.

### Kubernetes/OpenShift Operator

A future BaseHarbor Operator is another control surface over the same domain/provider model.

The Operator continuously reconciles desired state:

```text
BaseHarbor Application CR
          |
          v
      Reconcile
          |
          v
shared contract/lifecycle logic
          |
          v
Kubernetes/OpenShift runtime provider
          |
          v
observe + verify
          |
          v
CR status / conditions
```

The Operator must use idempotent reconciliation and provider-native observation rather than wrapping imperative `baha` commands.

Kubernetes/OpenShift CRDs are cluster-facing representations of BaseHarbor desired state. They must preserve the same logical application intent and must not force applications to depend on Kubernetes/OpenShift implementation details.

### Shared result models

Lifecycle and diagnostic results should be machine-readable before they are rendered by a surface.

Examples include shared concepts such as:

- application state/readiness;
- plan actions;
- preflight failures;
- provider capability failures;
- diagnostic code/severity/message/remediation;
- resource status;
- lifecycle conditions;
- input definitions and unresolved values.

The CLI renders these for terminals, the Web UI renders them visually, the API serializes them, and the Operator maps them to Kubernetes status/conditions where appropriate.

No surface may invent a more optimistic readiness model than the shared verification result.

## Architectural rule

BaseHarbor Core must not depend on CLI, HTTP, browser or Kubernetes/OpenShift presentation concerns.

In particular:

- domain/application packages must not import Cobra command packages;
- domain/application packages must not depend on frontend code;
- shared lifecycle behavior must not require Kubernetes types unless the behavior is specifically inside a Kubernetes/OpenShift provider adapter;
- CLI, API and Operator adapters may depend on the core, never the reverse;
- provider-specific objects remain behind provider boundaries;
- security and policy decisions are shared or enforced below the presentation layer rather than duplicated per UI.

## Current and future scope

### Implemented in v0.4

- `baha` as the primary CLI;
- reusable provider-neutral `PortableContract`;
- runtime provider selection/capability negotiation;
- declarative input resolution separated from CLI prompting;
- shared runtime truth used by multiple CLI commands.

### Future

- stable BaseHarbor HTTP API;
- lightweight Web UI;
- contexts/remote targets for `baha`;
- Kubernetes Operator and runtime provider;
- OpenShift specialization;
- shared managed-production identity/RBAC/JIT policy across API, UI, CLI and Operator-facing actions.

This ADR defines architecture direction only. It does not claim that the Web UI, API or Operator already exists in v0.4.

## Consequences

### Positive

- CLI, GUI and Operator can expose the same behavior without three implementations.
- Tests for core lifecycle behavior apply to every future surface.
- Application contracts remain independent from presentation and runtime.
- A lightweight GUI remains feasible because it is primarily a renderer/client of structured API operations.
- Kubernetes/OpenShift support can reuse domain and provider logic instead of rebuilding BaseHarbor around CRDs.

### Trade-offs

- More behavior must move out of command handlers into reusable application services as BaseHarbor evolves.
- Result types need stable machine-readable structures rather than terminal-only strings.
- API authorization and Operator reconciliation introduce additional boundaries that need explicit testing.
- Some local-only CLI conveniences may remain surface-specific, but they must not become required application lifecycle semantics.
