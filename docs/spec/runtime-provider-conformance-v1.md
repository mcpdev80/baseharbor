# Runtime Provider v1 Conformance

## Status

Foundation draft. This suite defines what an external Runtime Provider must prove before it can be treated as conforming.

## Required checks

A conforming provider MUST prove all of the following.

### Identity and protocol

- `Describe` returns a stable provider ID.
- provider version is independent from protocol version.
- protocol identifies `baseharbor.runtime.provider/v1`.
- advertised runtime capabilities are explicit.

### Preflight and fail-closed behavior

- unsupported workload semantics fail before mutation.
- missing OCI artifacts fail before mutation.
- invalid/empty target scope fails before mutation where the provider requires a scope.
- capability-provider or delivery-provider concerns are not accepted as runtime workload fields.

### OCI artifact boundary

- every runnable service resolves to an OCI image reference before `Apply`.
- repository build/source instructions are rejected at the runtime boundary.
- immutable digest references are accepted.
- tags may be accepted as discovery aliases where deployment policy permits them.

### Workload semantics

- service name identity is preserved.
- image CMD override semantics map from `args`, not by blindly mapping Compose `command` to a runtime-native ENTRYPOINT field.
- container ports remain container endpoints; host/public exposure is not invented by the runtime provider.
- provider-native object names do not become portable application identity.

### Bindings and secrets

- public bindings and secret references remain distinguishable.
- plaintext secret material is not emitted in diagnostics.
- secret references are resolved only at the trusted workload projection boundary.

### Lifecycle

- repeated `Apply` with the same idempotency key does not duplicate managed resources.
- `Observe` reports provider-neutral found/running/ready state.
- `Logs` and `Exec` operate on logical workload service identity.
- `Destroy` deletes only resources owned by the BaseHarbor application/environment identity.
- repeated `Destroy` is safe and idempotent.

### Ownership

- foreign resources are not destructively mutated.
- provider-native ownership metadata is sufficient to select only BaseHarbor-owned runtime resources.
- application/environment identity survives provider-native name normalization.

### Isolation from other provider axes

The Runtime Provider MUST NOT provision application capabilities merely because the selected runtime can host them.

Examples:

```text
SQL
cache
object storage
secrets
telemetry
exposure
```

remain capability-provider concerns.

The Runtime Provider MUST NOT decide direct versus delegated/GitOps reconciliation ownership. That remains Delivery Provider state.

## Initial reference evidence

The bundled Kubernetes implementation is the first reference implementation for the new normalized workload/runtime lifecycle semantics.

Docker and Podman must later pass equivalent semantic conformance before the runtime contract can be considered frozen.

OpenShift must pass the same portable semantics while remaining free to use OpenShift-native realization behind the provider boundary.
