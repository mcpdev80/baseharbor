# Runtime Explorer Contract v1

## Scope

Runtime Explorer is the provider-neutral inspection and bounded low-level operations contract for concrete runtime resources.

It sits below BaseHarbor Application intent:

```text
Application / Deployment / Component-or-Provider
        -> runtime realization
        -> Runtime Explorer resource
```

Portable Application intent MUST NOT contain Docker, Podman, Kubernetes or OpenShift resource objects.

Contract identifier:

```text
baseharbor.runtime-explorer/v1
```

## Stable resource reference

Every resource MUST expose a stable reference containing:

- runtime provider identity;
- BaseHarbor Target;
- provider-neutral resource kind;
- stable runtime resource id.

Runtime-native display names are metadata, not BaseHarbor logical identity.

Clients MUST treat resource IDs as opaque. Local Podman containers managed by
Quadlet use the existing protected unit definition as their stable reference,
because a supervised stop can remove the native container. Start, stop and
restart operate through that unit's systemd supervisor; logs, exec and terminal
resolve its current verified native container. An absent container is reported
as stopped only when the existing unit is observed inactive or failed. Native
running state and application readiness remain distinct observations.

## Resource kinds

v1 defines additive resource kinds for:

- `container`;
- `image`;
- `volume`;
- `network`;
- `pod`.

Future Kubernetes/OpenShift kinds MAY be added without changing portable Application intent.

## Ownership

Clients MUST use authoritative ownership returned by Core/runtime evidence.

```text
managed
external
unmanaged
platform
```

Clients MUST NOT infer ownership from names, prefixes or labels alone.

A `managed` resource MUST carry a BaseHarbor relationship to an Application, Deployment, Component or Provider.

## Relationships

Where known, Runtime Explorer exposes stable relationships:

```text
Application
  -> Deployment
    -> Component / Provider
      -> Runtime resource
```

Runtime-specific project/service labels MAY be exposed as references, but MUST NOT replace stable BaseHarbor identities.

## State and health

Common resource state can include:

- desired state where meaningful;
- observed state;
- health;
- readiness;
- creation/update timestamps when the runtime exposes them.

Provider-specific details belong in the extension field.

## Capabilities

Clients MUST negotiate capabilities and MUST NOT hard-code behavior by provider name.

The capability vocabulary includes:

```text
resources.inspect
resources.metrics
logs
container.lifecycle
container.exec
pod.inspect
```

Providers advertise only capabilities they actually implement. Unsupported capabilities fail explicitly.

## Operations

Bounded v1 container operations are:

- `start`;
- `stop`;
- `restart`;
- `exec`.

The Machine Interface exposes `runtime.start`, `runtime.stop` and `runtime.restart` as separate authorized semantic operations. This keeps safety/policy metadata stable and prevents a generic operation string from changing meaning after authorization.

Exec requires an explicit command and uses the protected runtime exec stream boundary. It does not imply a host shell and is not exposed as a generic command passthrough.

Runtime Explorer MUST NOT expose a generic provider command passthrough.

## Mutation safety

Direct low-level mutation is distinct from the preferred semantic BaseHarbor operation.

Managed resources expose reconciliation metadata such as:

- preferred semantic operation;
- whether direct mutation may be superseded by reconciliation;
- human-readable detail.

Unmanaged and external resources MUST NOT be treated as BaseHarbor-owned for mutation.

All machine-facing mutations remain subject to the shared Machine Operator Authorization, policy, safety and audit boundaries.

Before a direct `start`, `stop` or `restart`, the configured Runtime Explorer
service requires the existing Target/deployment consequence authority. Both
`runtime.operate` and the individual machine operations accept
`provider_change_consent` with the current displayed scope `plan_id`, `first`
and `second` acknowledgements. Unknown cross-consumer, transient-data, access,
renewal and recovery effects require **Achtung** and both acknowledgements;
ordinary operation approval does not replace either stage. Changed native
state, Target binding or recovery-owner bytes invalidate the prior plan.

The result includes the shared `provider_change_result`. It reports observed
native state and retained owner evidence; unknown function/material effects
remain partial. A failed backend command still triggers observation and partial
result projection. Direct service callers cannot omit the authority hook. This
contract does not claim that native stop restores authentication, data recovery
or availability, or that restart preserves in-flight work.

## Logs and metrics

Log access uses a stable runtime resource reference and optional bounded history selectors.

The protected Machine HTTP API from the sibling HTTP contract may stream Runtime Explorer logs after shared authorization.

Metrics are represented through a provider-neutral handle when supported. Absence of runtime metrics is explicit and not synthesized.

For an admitted remote Connector, Core advertises `resources.metrics` only when
the live session advertises `runtime.metrics`. Core validates the selected Target,
provider and observed container before dispatch. A successful response includes
an optional `sample` with `resource_id`, Core receipt time `observed_at`, and the
native `cpu_percent`, `memory_usage` and `network_io` strings. Native units remain
unchanged; these observations do not assert normalized capacity or health. A
missing native sample returns `available: false`; identity mismatches and failed
requests return errors.

## Docker and Podman reference realization

Docker and Podman reuse the existing BaseHarbor Runtime Provider implementations.

The reference explorer:

- inventories Docker containers, images, volumes and networks;
- inventories Podman containers, images, volumes, networks and pods;
- uses stable runtime ids where the engine exposes them; volume names are the runtime-native stable identifiers;
- retains unmanaged resources rather than dropping them;
- exposes state and health where the runtime supplies them;
- keeps image/volume/network/pod inventory read-only in v1;
- delegates bounded container logs/lifecycle/exec through resource-scoped runtime commands;
- never falls back to a host shell.

Managed container ownership is supplied by BaseHarbor deployment evidence through a separate resolver rather than guessed from display names. Inventory resources without authoritative ownership evidence remain `unmanaged`; the explorer does not promote them to managed based on naming conventions.

## Kubernetes/OpenShift compatibility

v1 is intentionally open to additive resource kinds including:

- namespaces;
- workloads;
- pods;
- services;
- gateways/ingress;
- ConfigMaps;
- Secret metadata;
- PVCs;
- events;
- nodes;
- OpenShift-specific resources.

Adding those projections MUST NOT require changes to the portable Application contract.
