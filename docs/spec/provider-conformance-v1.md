# Public provider conformance v1

Status: implementation candidate for v0.4.22 (#772).

The [Provider Contract v1](provider-contract-v1.md) has a public Go conformance entry point: `github.com/mcpdev80/baseharbor/conformance/provider/v1`. It delegates to the existing executable Provider Integration Contract harness.

Profile identity is `provider-contract/v1`; report identity is `baseharbor.provider-conformance/v1`; the tested protocol is `baseharbor.provider/v1`. An implementer can supply its driver and fixtures from a separate Go module without importing Core internals. The repository includes and executes an independent-module proof.

Required fixtures, mutation scope, deterministic invocation, JSON/exit semantics and a sample runner are documented in `conformance/provider/v1/README.md` in the source repository. Pin the tested module commit; the public package becomes part of the v0.4.22 source artifact when that release is published.

The lifecycle suite checks descriptor validity, driver identity, optional unsupported placement, side-effect-free preflight through a required state fingerprint, provision/bind, readiness and repeated convergence. Full acceptance additionally executes the fault, recovery, drift and ownership scenarios below using the same harness.

Artifact signature/provenance trust is an independent result described in [Extension Artifact Trust v1](extension-artifact-trust-v1.md). Conformance success does not grant trust to an artifact or its publisher. Runtime-provider and other contract families retain independently versioned profiles.

## Full semantic acceptance

Use `provider.RunFull(ctx, target)` for `suite=full`. In addition to the unchanged lifecycle harness, this runs deterministic drift/repair, outage/recovery, provision/malformed-binding/verification failure, retry idempotency, foreign ownership and ownership-safe destroy checks. The fixture implements typed reconciliation and the state/drift/destroy/fault/ownership hooks documented in the public package. Missing hooks fail acceptance.

`provider.Run` remains an explicitly labeled `suite=lifecycle` diagnostic run. Its pass result does not claim the full fault and ownership suite. Both suites redact provider error text and emit deterministic named checks.

Public discovery metadata is `contracts/conformance/provider/v1/profile.json`. The provider contract spec, discovery metadata and executable public Go package share the repository revision. External implementers pin the released module version or immutable tested commit; no private infrastructure is required.
