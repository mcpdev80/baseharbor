# ADR 0007: Runtime provider selection is deployment-owned

## Status

Accepted for v0.4 incremental implementation.

## Context

BaseHarbor v0.3 runs application workloads through Docker/Podman Compose. The long-term runtime targets also include Kubernetes and OpenShift, while the portable application contract must remain stable across those environments.

A runtime provider answers where and through which runtime primitives an application workload is operated. This is separate from capability providers such as PostgreSQL, Valkey, object storage or secrets. It is also separate from the later Delivery Provider axis defined by ADR 0011, which determines how desired runtime realization is applied/reconciled.

If runtime selection leaks into the application contract, an application would need to be rewritten when moving from Compose to Kubernetes or OpenShift. If BaseHarbor guesses a runtime implicitly from host state, production behavior can become ambiguous.

## Decision

BaseHarbor introduces a typed runtime-provider seam in `internal/runtime`.

The provider exposes:

- a stable provider kind;
- explicit runtime capabilities needed by orchestration.

The current local provider seam preserves the existing Compose-based application/runtime model. Docker executes that model through Docker Compose; Podman translates it into native Quadlet units managed through rootless `systemd --user`. Future provider kinds may include `kubernetes` and `openshift`.

Runtime provider selection is deployment/environment-owned state. It is not an application capability and must not be selected from portable `baseharbor.yaml` requirements.

During the v0.4 migration, existing `DetectCompose` callers remain supported. Detection is routed through the central provider-selection seam so callers can be migrated incrementally rather than through a repository-wide rewrite.

## Capability rule

A runtime provider must advertise behavior that orchestration depends on. The initial Compose capability set records support for:

- workload lifecycle;
- service exec;
- published-port inspection;
- provider-owned resource ownership checks.

A future provider must either support a required runtime capability or fail clearly before mutation. BaseHarbor must not silently reduce lifecycle, security or verification guarantees because a different runtime was selected.

Provider capabilities are runtime mechanics, not application requirements. For example, `database.sql` remains an application capability regardless of whether its selected implementation runs in Compose, Kubernetes, OpenShift or outside the workload runtime entirely.

## Kubernetes and OpenShift implication

Future selection should be explicit through deployment/environment configuration, for example conceptually:

```text
runtime: compose
runtime: kubernetes
runtime: openshift
```

The exact public environment schema is intentionally not fixed by this ADR.

Kubernetes/OpenShift implementations may realize lifecycle with Deployments, StatefulSets, Services, Jobs, Gateway/Ingress, Routes, PVCs, NetworkPolicies or other native resources. Those objects remain provider implementation details and must not be added to the portable application contract merely to support the provider.

OpenShift remains a distinct runtime specialization where its security, Route, SCC, registry or enterprise behavior differs materially from generic Kubernetes.

## Compatibility

This step does not change:

- manifest v1;
- CLI syntax;
- persisted application state;
- Compose lifecycle behavior;
- backup/restore formats;
- application-facing environment or binding contracts.

The local runtime implementation supports Docker Compose and Podman Quadlet execution behind the same portable runtime boundary. Kubernetes and OpenShift remain future provider implementations.

## Consequences

Positive:

- one selection point exists before Kubernetes/OpenShift implementation begins;
- existing Compose input compatibility is preserved while Podman execution is native Quadlet;
- runtime, capability-provider and delivery-provider axes remain separate;
- provider capability failures can become explicit and fail closed.

Trade-off:

- existing concrete Compose callers remain during the migration;
- the provider interface stays intentionally small until a second implementation proves which lifecycle operations are genuinely portable.

That trade-off is deliberate. BaseHarbor should not invent a large generic runtime API before Kubernetes/OpenShift supply real requirements for it.
