# Runtime Provider v1 Conformance

A conforming Runtime Provider MUST prove the same portable semantics regardless of provider product.

## Identity and protocol

- stable provider identity
- provider version independent from protocol version
- protocol identifier `baseharbor.runtime.provider/v1`
- explicit advertised runtime capabilities

## Fail-closed preflight

Unsupported workload semantics MUST fail before mutation.

The provider MUST reject:

- missing resolved OCI image artifacts
- invalid target scope where a scope is required
- repository build/source instructions at the runtime boundary
- capability-provider fields
- delivery/GitOps ownership fields

## Runtime plan

The portable runtime plan contains only:

- Application identity
- environment identity
- opaque target scope
- logical workload service identity
- resolved OCI image
- args
- non-secret environment values
- workload-local container ports
- public bindings or opaque secret references

The plan MUST NOT contain provider-native fields such as Kubernetes Namespace/Deployment, Compose project, Podman Quadlet, Docker network or host path.

## Observation

READY/status/doctor/evidence semantics consume provider-neutral observation:

- found
- running
- ready
- logical service detail/diagnostics

Provider-native condition names are implementation details.

## Ownership and destroy

- only BaseHarbor-owned resources may be destroyed
- foreign resources are never destructively adopted
- Application/environment identity survives provider-native name normalization
- repeated Destroy is safe
- cleanup evidence must prove no owned resources remain

## Provider-axis isolation

Runtime Provider MUST NOT provision capabilities merely because the runtime can host them.

Capability Provider remains responsible for SQL, key-value, document database, messaging, object storage, secrets, identity, metrics, logs, traces, OTLP and exposure.

Delivery Provider remains responsible for direct versus delegated reconciliation ownership.

## Reference evidence

Docker and Podman are the complete v0.4.x reference realizations.

Kubernetes PR #616 is the pre-freeze architecture crash test. Its learnings are normative evidence for this contract:

- opaque target scope / Namespace separation
- stable Application identity
- ownership-safe cleanup
- namespace-scoped operation
- runtime-neutral bindings
- runtime-neutral READY/status/doctor/evidence
- no Kubernetes-specific Core semantics

Kubernetes production feature parity is not required for v0.4.19.
