# Releases and versioning

[Pre-release readiness](../development/pre-release-readiness.md)
BaseHarbor uses Semantic Versioning for normal releases with Git tags prefixed by `v`. During the v0.4 line, emergency hotfixes use the documented four-part `MAJOR.MINOR.PATCH.HOTFIX` extension.

Examples: `v0.3.0`, `v0.4.15`, `v0.4.15.1`, `v1.0.0`.

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

- GitHub tag/release `vX.Y.Z` or v0.4 hotfix `vX.Y.Z.H`: immutable supported release created from `main`;
- `ghcr.io/mcpdev80/baseharbor-runtime:X.Y.Z` or `X.Y.Z.H`: matching runtime image;
- `ghcr.io/mcpdev80/baseharbor-runtime:latest`: latest stable release;
- `ghcr.io/mcpdev80/baseharbor-runtime:edge`: moving development build from `develop`.

Normal development branches must not target `main` directly. Hotfix branches start from `main`, are released through `main`, and must then be merged/backported into `develop` so the next release retains the fix.

## Hotfix releases

During the current v0.4 line, an emergency correction to an already published patch uses a four-part BaseHarbor hotfix version:

```text
MAJOR.MINOR.PATCH.HOTFIX
```

Example:

```text
v0.4.15 -> v0.4.15.1
```

Hotfix workflow:

1. Start the hotfix release branch from the exact released `main` line, never from in-progress `develop`.
2. Link every correction to the hotfix release parent issue.
3. Keep the branch defect-only: no unrelated features, refactors or dependency updates.
4. Use focused validation while implementing individual fixes.
5. Update changelog, release notes and any affected operator/reference documentation.
6. Run the complete pre-release gate exactly once at the release boundary against the final hotfix candidate.
7. Open the hotfix release PR to `main` only after the candidate checks are green.
8. Publish the immutable four-part tag and matching runtime image from the merged `main` commit.
9. Verify release artifacts/provenance, then forward-port the same fixes to `develop`.
10. Remove temporary hotfix/validation branches only after publication and forward-port are complete.

If a proposed change alters runtime identity, persisted-state migration, public contracts or architecture beyond what is required to correct the released defect, move it back to the normal roadmap instead of expanding the hotfix.

Rollback rule: do not move or rewrite a published tag. If a hotfix itself is defective, revert the release change on the released line as appropriate and publish a new hotfix version.

Source rule: the release issue and release notes must identify the exact BaseHarbor candidate SHA, the external demo revision used by pre-release validation, and the successful release-boundary test results.

## Release preparation

Normal releases are prepared on `develop` and promoted to `main` after validating the immutable candidate. Hotfixes start from `main`. The [complete release audit](https://github.com/mcpdev80/baseharbor/blob/develop/docs/internal/pre-release-documentation-audit.md) covers implementation, ecosystem consumers, EN/DE docs, the Pages candidate build, changelog and human-readable release notes.

1. Complete implementation, contracts, documentation and release notes.
2. Freeze exact Core, Demo, Console and Connector commits where required by the release scope.
3. Run affected tests during development and fix diagnosed failures.
4. Run complete pre-release validation on the immutable candidate only after explicit authorization. Normal required jobs must succeed; failing, cancelled or missing required work blocks.
5. Open a release PR containing the validated release content against `main`. Promotion, tagging and publication each require the authorized scope.
6. After authorization create an immutable tag on the explicit resulting `main` commit. The release workflow executes reusable full Pre-Release checks on that exact tag, then publishes the artifacts.
7. Verify binaries, checksums, version/commit metadata, runtime image, provenance and published documentation.

CI, Pre-Release and Release do not use gate manifests, evidence reuse, completed-run/job re-verification or a separate receipt approval chain. The only CI record contains merge SHA and PR number; see [minimal CI receipts](../development/minimal-ci-receipts.md). Build/image inputs, immutable consumer pins and published artifact checksums remain required.

### Historical ecosystem inputs

Published v0.4.24/v0.4.25 records and immutable consumer pins remain historical data. Their former browser receipts, run artifacts and approval manifests are not active CI prerequisites. Current checks use the actual pinned source commits and execute their required tests directly.

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

A real application should never silently follow `main`. During the `0.x` series, an application validated against `v0.4.15` should normally constrain itself to the compatible minor line, for example `>=0.4.0 <0.5.0`, unless it intentionally validates against a newer minor release.

Applications moving through the v0.4 line keep Manifest v1 and the existing Compose developer journey. v0.4.3 added the open Provider Integration Contract and deterministic read-only repository inspection. v0.4.4 added provider-neutral managed HTTP/HTTPS exposure. v0.4.5 added provider-neutral secure-binding/workload-identity semantics. v0.4.6 added `object-storage.s3/v1` with a replaceable SeaweedFS Compose reference provider, bucket-scoped credentials and authenticated S3 readiness. v0.4.7 added provider-neutral `telemetry.otlp/v1`, a lazy shared OpenTelemetry Collector reference provider, external OTLP binding and real OTLP export verification. v0.4.8 added `metrics/v1`, Prometheus placement/isolation, Runtime Resource API execution and directed cross-application connectivity. v0.4.9 added provider-neutral centralized workload logs through Loki/Alloy, rendered-Compose workload security preflight and executable provider conformance/fault injection. v0.4.10 added generic provider observability and trace storage, v0.4.11 hardened repository adoption and lifecycle UX, v0.4.12 added the agent-native machine/MCP surface, v0.4.13 added explicit environment and policy semantics, v0.4.14 completed typed reconciliation/ownership/convergence semantics, v0.4.15 added first-class Targets, native Podman Quadlet execution, the TLS/service-access baseline, complete provider/runtime observability semantics, the full agent-native MCP/JSON lifecycle and installation-wide full destroy, v0.4.16 added selectable application recovery units plus shared Audit/Evidence v1 semantics, v0.4.17 added managed identity/operator authentication/shared-provider lifecycle and the versioned Runtime Provider boundary, and v0.4.18 adds the greenfield Development Extension path, multi-repository workspaces, capability-subset acceptance and local Docker/Podman host-resource preflight while retaining Manifest v1.
