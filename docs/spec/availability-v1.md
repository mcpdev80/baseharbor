# Availability and high availability v1

## Scope

BaseHarbor availability is one resolved semantic of the existing Application, Runtime Provider and Capability Provider contracts.

It is not a second scheduler, cluster manager, replication protocol or provider registry.

## Portable intent

The normal decision is one boolean:

```yaml
ha: true
```

or:

```yaml
ha: false
```

When HA is enabled, sparse component/capability overrides may change only the effective HA request and, where meaningful, fixed desired cardinality:

```yaml
ha: true

availability:
  sql:
    instances: 5
  identity:
    instances: 3
  logs:
    ha: false
```

Resolution order is:

```text
component/capability override
    -> global ha
    -> explicit instances
    -> provider/runtime recommended topology
    -> generic 3 only when no stronger provider/runtime rule exists
```

Instance count is topology input, never application, component, resource, binding or consumption identity.

## Negotiation

Runtime and capability-provider availability are negotiated independently.

A selected provider/runtime declares one of:

```text
SUPPORTED
PARTIALLY_SUPPORTED
UNSUPPORTED
```

A required HA guarantee that is unsupported MUST fail before mutation with a typed actionable result. There is no silent downgrade.

A provider may remain unsupported in v0.4.21. BaseHarbor does not invent clustering merely to make every shipped provider claim HA.

## Observation

One logical component/resource may resolve to zero, one or many runtime instances.

Observation retains every observed instance and aggregates readiness against the requested guarantee. It MUST NOT collapse several instances into one arbitrary representative.

Stable application-facing identity, binding and consumption references do not change when instance count, placement, rescheduling or replacement changes.

## Rolling changes

If a provider/runtime advertises HA support, certificate, secret, trust, config, image and member/instance changes must preserve verified healthy capacity.

The portable rule is:

```text
prepare/reload replacement
    -> verify readiness and required semantics
    -> retire only safe old capacity
    -> stop safely on failure
```

Where the selected realization cannot preserve continuity, BaseHarbor fails before disruptive mutation or requires an explicit surfaced disruption approval. It never silently restarts all healthy capacity while claiming HA.

## Current v0.4.21 reference boundary

The shipped Docker Compose and Podman Quadlet runtimes do not advertise verified HA workload orchestration.

The bundled capability-provider realizations are explicitly classified. Where the current reference topology is single-instance or lacks a verified native HA implementation, the classification is UNSUPPORTED for effective HA.

This is deliberate and freeze-safe: future Kubernetes, OpenShift and managed/cloud providers can implement stronger realization behind the same contract without changing portable intent.

## Portability review

The contract has been reviewed against these realization shapes:

- Compose single-host;
- Kubernetes multi-node;
- OpenShift enterprise cluster;
- AWS runtime with managed PostgreSQL/object storage;
- Azure runtime with managed PostgreSQL/storage;
- GCP runtime with managed SQL/storage.

None requires portable intent to contain Pod/container names, namespaces, node/zone names, StatefulSet/PDB concepts, cloud product modes, replica member roles or provider clustering vocabulary.

Runtime HA and capability-provider HA remain independent, so a strong managed database may coexist with a weaker workload runtime and vice versa.

Future autoscaling may replace fixed cardinality with dynamic desired state without changing logical application/component/resource/consumption identity.
