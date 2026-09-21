# Releases and versioning

BaseHarbor uses Semantic Versioning with Git tags prefixed by `v`.

Examples: `v0.3.0`, `v0.4.0`, `v0.4.1`, `v1.0.0`.

## Stability policy

BaseHarbor is currently in the `0.x` series.

During `0.x`:

- patch releases (`0.4.0` -> `0.4.1`) stay backward-compatible within the active minor line and may contain fixes, security hardening and additive capabilities;
- minor releases (`0.4.x` -> `0.5.0`) may contain intentionally documented breaking changes or larger contract shifts;
- every breaking change must be called out in `CHANGELOG.md` and GitHub Release notes;
- consumers should pin an explicit compatible range instead of tracking `main`.

After `v1.0.0`, breaking public CLI, manifest, persisted-state or supported integration changes require a new major version.

## Compatibility contract

The versioned contract includes documented `baha` commands and flags, the `baseharbor.yaml` schema, persisted BaseHarbor state, supported backup/restore formats, application-facing environment/service contracts, and runtime image tags coupled to CLI versions.

Internal Go packages are not public API unless explicitly documented.

Manifest v1 remains the supported v0.4 compatibility surface. v0.4 adds provider-neutral contract/runtime seams behind that surface rather than forcing applications to rewrite their manifest for future providers.

## Development versus releases

BaseHarbor uses two long-lived branches with distinct responsibilities:

- `main` is the released source line. It should represent the currently published release and should only advance through a release PR from `develop` or an explicitly documented hotfix.
- `develop` is the integration branch for the next release. Normal feature, fix, chore and dependency branches target `develop`.

Published channels:

- GitHub tag/release `vX.Y.Z`: immutable supported release created from `main`;
- `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z`: matching runtime image;
- `ghcr.io/mcpdev80/baseharbor-runtime:latest`: latest stable release;
- `ghcr.io/mcpdev80/baseharbor-runtime:edge`: moving development build from `develop`.

Normal development branches must not target `main` directly. Hotfix branches start from `main`, are released through `main`, and must then be merged/backported into `develop` so the next release retains the fix.

## Release preparation

Every release is prepared on `develop` and promoted to `main` only after the release candidate is proven.

1. Review the final implementation on `develop` against `docs/DEVELOPMENT_GUIDELINES.md`, including ownership, isolation, secret-safety, fail-closed behavior, tests and documentation consistency.
2. Review and update all affected canonical documentation, including both EN/DE variants where they exist. Search explicitly for stale version numbers, implementation-status claims, examples and future-work statements.
3. Move relevant entries from `[Unreleased]` into a dated `## [X.Y.Z] - YYYY-MM-DD` section in `CHANGELOG.md`.
4. Write human-readable release notes at `docs/releases/vX.Y.Z.md`. They must explain what changed, why it matters, compatibility/upgrade impact, security implications and intentionally deferred work; a raw commit list or generated Git log is not an acceptable release message.
5. Review compatibility impact and select the SemVer increment.
6. Run local/Hugging Face validation first where practical.
7. Run the mandatory GitHub pre-release workflow against the exact `develop` release-candidate SHA and fix/repeat on `develop` until the gate is green.
8. Open one release PR from `develop` to `main`. Do not mix unrelated changes into this PR.
9. Merge `develop -> main` only after the pre-release gate is green and the release diff is understood.
10. Create an immutable tag `vX.Y.Z` on the resulting `main` release commit and push it.
11. The release workflow must successfully validate that the tag is contained in `main`, retest the tagged source, publish the GitHub Release, artifacts and provenance.
12. Verify the resulting GitHub Release, binaries, checksums, provenance and matching runtime image before declaring the release usable. A pushed tag without a successful published release is not release completion.

Never move a published version tag. Fix a bad release with a new patch release.

## Release artifacts

Each release publishes:

- `baseharbor_linux_amd64.tar.gz`
- `baseharbor_linux_arm64.tar.gz`
- `checksums.txt`
- GitHub build-provenance attestations
- matching versioned GHCR runtime images

Verify binary metadata with:

```bash
baha version
```

Verify checksums with:

```bash
sha256sum -c checksums.txt --ignore-missing
```

GitHub provenance can additionally be verified with:

```bash
gh attestation verify baseharbor_linux_amd64.tar.gz -R mcpdev80/baseharbor
```

## Consumer guidance

A real application should never silently follow `main`. During the `0.x` series, an application validated against `v0.4.9` should normally constrain itself to the compatible minor line, for example `>=0.4.0 <0.5.0`, unless it intentionally validates against a newer minor release.

Applications moving through the v0.4 line keep Manifest v1 and the existing Compose developer journey. v0.4.3 added the open Provider Integration Contract and deterministic read-only repository inspection. v0.4.4 added provider-neutral managed HTTP/HTTPS exposure. v0.4.5 added provider-neutral secure-binding/workload-identity semantics. v0.4.6 added `object-storage.s3/v1` with a replaceable SeaweedFS Compose reference provider, bucket-scoped credentials and authenticated S3 readiness. v0.4.7 added provider-neutral `telemetry.otlp/v1`, a lazy shared OpenTelemetry Collector reference provider, external OTLP binding and real OTLP export verification. v0.4.8 added `metrics/v1`, Prometheus placement/isolation, Runtime Resource API execution and directed cross-application connectivity. v0.4.9 adds provider-neutral centralized workload logs through Loki/Alloy, rendered-Compose workload security preflight and executable provider conformance/fault injection while retaining Manifest v1.