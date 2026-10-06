# Target Access wire v1

This is the canonical language-neutral transport artifact for Core and a managed
Connector. Access-provider descriptors describe routing and supported structure;
they are not these authenticated session records. Acquire this directory from an
immutable Core commit. Schema identity uses the controlled repository namespace
from ADR 0018; resolving a schema must never fetch an unpinned HEAD document.

`v1/wire.schema.json` contains named definitions for Hello, negotiated capability
sets, bounded requests/results/problems, cancellation, enrollment, bootstrap
authorization and stream control/output. `v1/wire.golden.json` contains synthetic
structural examples. Their placeholder identifiers and data are not certificates,
credentials, runtime observations or release evidence.

The packaged `contracts.ValidateTargetAccessRecord` resolves definitions offline
and rejects duplicate JSON keys, trailing values, unexpected fields, unbounded
frames/nesting, unsupported operations, invalid calendar values and malformed
base64. Bundle data must match its declared digest. Live capability names are
unique. Resource-scoped argv is bounded to 64 arguments and 16 KiB. Binary stream
chunks are at most 16 KiB; complete transport frames are at most 4 MiB.

## Required session semantics

- Authenticate mutual TLS and exact peer/node/tenant/Target/runtime binding before
  Hello or capability admission. Structural descriptors alone never prove a live
  capability or grant execution authority.
- Carry the original Core execution correlation, an absolute UTC deadline and a
  unique request identifier. A cancellation applies only to that active request.
- Persist side-effect admission and request content identity before execution.
  Reusing an identifier with different content fails. A disconnect or lost result
  never silently replays a destructive operation; report an ambiguous outcome and
  let Core reconcile observed state through a new authorized operation.
- Accept only the operation-specific bounded payload. There is no generic host
  shell, runtime-command, portable-intent, placement or policy channel.
- Resolve Compose/artifact references below the protected staging root and check
  traversal, symlinks, digest integrity and interruption cleanup at the filesystem
  boundary. A lexical schema check does not replace that confinement.
- Terminal input and output use separate monotonic sequences. Terminal admission
  cannot resume/replay; logs follow the existing Runtime Log Source Contract.
- Enforce deadlines, cancellation, write/backpressure bounds and identity expiry
  throughout execution. Revalidate/terminate affected active sessions after
  revocation and retain CA overlap only through verified rotation completion.

## Qualification status

The artifact specifies the shared wire boundary. It does not establish working
Core session routing or a private consumer's conformance. Both implementations
must decode the same golden fixtures and pass negative drift tests before live
remote capability is advertised. Existing descriptor/enrollment/source checks are
not the exact-ref remote Docker/rootless Podman release gates.
