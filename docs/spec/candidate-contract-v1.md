# Candidate Contract v1

## Scope

This specification defines the immutable source and image submission boundary for a
BaseHarbor candidate. A candidate is an **exact submission**: a fixed source
revision, image reference or native compose/quadlet unit that is applied to a
target. It is not a proprietary compulsory bundle, catalog, archive protocol,
stage DSL, new lifecycle service or generic migration engine.

Read [Delivery Provider Contract v1](delivery-provider-contract-v1.md) for the
mode/provider/owner/placement contract and [Application Contract v1](application-contract-v1.md)
for portable application intent.

## Immutable submission

A candidate submission is exact and never inferred:

- A Git source is a fixed `repository` + `revision`, where `revision` is a pinned
  immutable identity only: a 40/64-hex Git commit object ID or an opaque
  `algo:hex` digest of at least 32 hex chars. A mutable branch or tag (e.g.
  `main`, `release-1`) is never a fixed identity and is rejected. An absent
  source is valid when only an image is submitted.
- An image submission is a fixed OCI sha256 digest (never a mutable tag alone)
  with an optional image-layout path.
- Native Compose and Quadlet units are consumed as authored inputs; they are
  not rebuilt by BaseHarbor at consume time.
- A Git revision may be unknown or absent; OCI annotations alone do not prove
  provenance and are not substituted for it.

Readable release or component versions map to actual artifact references. A human
version string is never used as the immutable identity.

## Application / target / consent binding

The candidate binds an application and a target through an explicit consent:

- `app.*` fields bind the submission to a specific application.
- `target.*` fields bind it to a specific target.
- `consent.*` carries the explicit operator/automation acknowledgement that a
  fixed submission is applied: `consent.by` names the principal, `consent.at`
  is the RFC 3339 acknowledgement time, and `consent.digest` is the canonical
  content-hash of the validated submission. A candidate can only be applied
  when `consent.digest` matches that exact submission; a stale, partial, or
  otherwise mismatched acknowledgement is rejected.

The binding is declarative intent only. It does not build, promote or migrate
anything on its own.

## Build versus consume

Local source build and consumption of a CI-produced release are separate:

- A **local build** compiles source into runtime realization. It never consumes a
  release.
- A **release consume** applies an existing CI-produced image/revision. It never
  builds.

Release consumption and promotion never build. The two paths are explicit and
never merged into a single ambiguous operation.

## Migration boundary

The application or its existing migration tool owns schema changes and
compatibility evidence. BaseHarbor checks only the declared prerequisites it can
actually verify.

- A backup never proves schema compatibility.
- An image rollback never implies a database rollback.

Missing required verifiable migration evidence blocks that applicable operation.
BaseHarbor reports unsupported or unknown migration capability; it does not infer
compatibility automatically, does not execute migrations and does not ship a
generic migration engine. Unsupported migrations fail before mutation.

## Stage versions

A deployment release/configuration record may extend the existing environment
axis with an optional, non-normative stage label for observability. Stage
information reuses existing source/applied/observed record fields; it never
introduces a new normative stage axis or DSL.

## Consent in persisted deployment records

The persisted, environment-keyed deployment record (the `deployment.json` record
under `internal/deployment/deployment_store.go`) gains one additive field:
`consent`, an explicit `CandidateConsent{by, at, digest}` that pins the exact
validated candidate submission. This is a minimal, backward-compatible extension
of the existing record — it adds a field and does not change any existing field,
so the retention and normalization paths are unchanged.

**Migration:** existing deployment records written before this field existed
remain loadable; the first repository-owned apply mutation after deployment
requires an explicit `consent`, and the apply fails closed (writes no state)
without it.

This slice is constrained by the [Native delivery, artifacts and independent stage
versions](https://github.com/mcpdev80/baseharbor-plan/blob/HEAD/baseharbor-plan/docs/decisions/2026-10-18-r27-native-delivery-and-stages.md)
decision (`2026-10-18-r27-native-delivery-and-stages`).

## Standards boundary

Reuse native tools and existing owners. No private release, registry, build,
pipeline or controller framework. No default CI or GitOps prerequisite for local
work. Runtime-specific unsupported behavior remains explicit. This slice is
constrained by the [Native delivery, artifacts and independent stage
versions](https://github.com/mcpdev80/baseharbor-plan/blob/HEAD/baseharbor-plan/docs/decisions/2026-10-18-r27-native-delivery-and-stages.md)
decision (`2026-10-18-r27-native-delivery-and-stages`).

## Fixtures

Input and negative fixtures that exercise malformed, digest, path and
secret-reference submissions live under `contracts/candidate/v1`. They assert
that a candidate submission is exact and fail closed on missing or invalid
identity; they never prove provenance, runtime health or release evidence.

<!-- generated:card -->
