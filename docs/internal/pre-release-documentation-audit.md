# Pre-release audit

Run this audit before every BaseHarbor release.

This is the canonical release-readiness checklist. A release is not ready merely because feature work is merged or normal CI is green.

The audit covers the BaseHarbor repository, the external `baseharbor-demo`, GitHub Issues/Roadmap, documentation/GitHub Pages, release evidence, publication and post-release verification.

## 1. Release scope and issue closure

- [ ] Confirm the intended release version and versioning rule (SemVer for normal releases; documented four-part extension for v0.4 hotfixes).
- [ ] Read the current roadmap issue and identify every issue explicitly assigned to this release.
- [ ] Verify every release-scoped implementation issue is complete.
- [ ] Do not treat broader future/freeze issues as release blockers unless the roadmap explicitly assigns them to this release.
- [ ] Review open PRs/issues mentioning the target release for forgotten blockers or deferred acceptance.
- [ ] Confirm deferred work is assigned to a later issue/release and is not silently dropped.
- [ ] Update release-scoped issue acceptance checklists/evidence when implementation or external acceptance is complete.
- [ ] Close release-scoped issues only when their own acceptance/evidence requirements are satisfied.

## 2. Final BaseHarbor implementation review

- [ ] Review the final `develop` implementation against `docs/DEVELOPMENT_GUIDELINES.md`.
- [ ] Ownership/isolation rules remain correct and fail closed.
- [ ] Secret values, private keys, tokens and credential-bearing URLs do not leak into normal output, machine results, logs or evidence.
- [ ] Mutating/destructive operations preserve validation, policy, ownership, approval and post-mutation verification.
- [ ] CLI, JSON and MCP expose the same semantic truth where machine access is applicable.
- [ ] Runtime-specific implementation details do not leak into portable application intent.
- [ ] Docker/Compose and Podman/Quadlet behavior remain semantically equivalent where supported.
- [ ] No unrelated feature/refactor work is mixed into release preparation.

## 3. Standards-first contract review

When a release adds or changes a service kind, capability/provider contract, runtime contract or workload binding:

- [ ] Existing Standards are documented.
- [ ] Adopted Standards are documented.
- [ ] Deviations are explicit.
- [ ] BaseHarbor Extensions are explicit, versioned and provider-neutral.
- [ ] Compatibility Impact is documented.
- [ ] JSON Schema 2020-12 remains the machine-readable portable schema language.
- [ ] Service connection outputs use Service Binding 1.1 well-known names when semantically applicable.
- [ ] Provider/product-specific fields do not enter portable application intent.

## 4. BaseHarbor documentation audit

### Human docs

- [ ] User-visible behavior is documented where needed.
- [ ] Human docs are concise and understandable.
- [ ] Examples match shipped behavior.
- [ ] Future behavior is clearly marked.
- [ ] EN/DE variants are consistent where both exist.

### Reference

- [ ] CLI reference/help matches implementation.
- [ ] Manifest/configuration reference matches schema/types.
- [ ] API/MCP reference matches the implemented surface.
- [ ] Agent/machine-interface docs match the implemented operation set and safety metadata.
- [ ] Typed errors/outcomes match implementation.
- [ ] Release/versioning documentation reflects the new release.

### Specs and architecture

- [ ] Changed public contracts have matching spec changes.
- [ ] Compatibility/version effects are documented.
- [ ] Security invariants remain explicit.
- [ ] Architecture describes current architecture, not release history.
- [ ] Provider/runtime-specific details did not leak into portable concepts.
- [ ] New decisions have an ADR when rationale must be preserved.
- [ ] Current runtime docs distinguish Docker Compose from Podman Quadlet execution.
- [ ] Podman release validation explicitly blocks fallback to `podman compose`.

### Roadmap, release history and staleness

- [ ] EN roadmap matches the current GitHub roadmap issue.
- [ ] DE roadmap matches the current GitHub roadmap issue.
- [ ] Completed work is not still described as future work.
- [ ] Deferred work points to the correct later release/issues.
- [ ] No obsolete page contradicts current behavior.
- [ ] Historical release notes remain historical and are not rewritten as current-state docs.
- [ ] Navigation and links remain valid.
- [ ] Search for stale current-version numbers, old operation counts, old implementation-status claims and old release sequencing.

## 5. Changelog and release notes

- [ ] Move all release entries from `[Unreleased]` into `## [X.Y.Z] - YYYY-MM-DD` (or `X.Y.Z.H` for a v0.4 hotfix).
- [ ] Leave a clean `[Unreleased]` section for subsequent development.
- [ ] Changelog covers all notable Added/Changed/Fixed/Security behavior in the release, not only the last feature worked on.
- [ ] Create/update `docs/releases/vX.Y.Z.md` (or `vX.Y.Z.H.md` for a v0.4 hotfix).
- [ ] Release notes explain what changed and why it matters.
- [ ] Compatibility/upgrade impact is explicit.
- [ ] Security implications are explicit.
- [ ] Intentionally deferred work is explicit.
- [ ] Release notes are human-readable product notes, not a generated commit list.
- [ ] Add the release to `docs/releases/index.md` and any equivalent navigation/index surface.

## 6. External `baseharbor-demo` review

The external demo is part of release evidence and must be reviewed before pinning its SHA.

- [ ] README matches the target BaseHarbor release and current developer journey.
- [ ] Demo application consumes current standard service bindings without BaseHarbor-specific application SDK shortcuts.
- [ ] TLS/trust/certificate file bindings are exercised by the real workload where relevant.
- [ ] Metrics, logs and traces are exercised when part of the release scope.
- [ ] Agent/MCP discovery and operation expectations match the target machine contract.
- [ ] MCP/agent demo gates are registered in the full acceptance graph directly or through another registered gate.
- [ ] Machine safety metadata and secret-safety assertions remain current.
- [ ] Docker acceptance uses the normal Docker path.
- [ ] Podman acceptance proves native Quadlet behavior and blocks `podman compose` fallback.
- [ ] Guided pristine-repository flow remains covered: `baha app init -> baha up -> READY`.
- [ ] Lifecycle/capability/security/reconciliation/recovery gates remain coherent with the release.
- [ ] Demo `main` contains the exact test implementation intended for release validation before the BaseHarbor pre-release run begins.

## 7. GitHub Pages and published documentation

- [ ] Both EN and DE MkDocs builds pass with `--strict`.
- [ ] `scripts/documentation-audit.sh` passes.
- [ ] Pages navigation includes the new release notes and current references.
- [ ] Pages contains no stale roadmap/MCP/release claims.
- [ ] The Pages workflow is expected to publish from the release `main` commit.
- [ ] After promotion to `main`, the actual GitHub Pages workflow completes successfully.
- [ ] Published Pages content is spot-checked for the release notes, roadmap and key changed reference pages.

## 8. Final release candidate

- [ ] Finish all release-preparation docs/demo changes before selecting the candidate.
- [ ] Identify one exact final `develop` candidate SHA.
- [ ] Do not add feature changes after the candidate is selected.
- [ ] Verify the candidate is contained in `develop`.
- [ ] Review `develop` versus `main`, including any `main`-only hotfix/release commits.
- [ ] Understand the actual tree diff that will be promoted; branch-history divergence alone is not treated as content equivalence.

## 9. Mandatory pre-release gate

Run `.github/workflows/pre-release.yml` for the exact target tag and exact candidate SHA.

The gate must prove:

- [ ] Release tag format.
- [ ] Changelog release section exists.
- [ ] Release notes exist.
- [ ] Go formatting.
- [ ] Source/package tests.
- [ ] `go vet`.
- [ ] CLI build.
- [ ] Control-plane restart and operator-held OpenBao recovery.
- [ ] Real Loki/Alloy log ingestion.
- [ ] Real OTLP -> Tempo trace ingestion.
- [ ] EN/DE strict documentation builds.
- [ ] Documentation audit.
- [ ] Workflow syntax/actionlint.
- [ ] GoReleaser snapshot.
- [ ] linux/amd64 and linux/arm64 release archives/checksums.
- [ ] Runtime image builds without publishing.
- [ ] Docker runtime acceptance.
- [ ] Podman/Quadlet runtime acceptance.
- [ ] Exact external `baseharbor-demo` SHA is pinned.
- [ ] Full Docker demo acceptance passes.
- [ ] Full Podman/Quadlet demo acceptance passes.
- [ ] Combined release-gate evidence succeeds.
- [ ] Immutable `release-approved.json` is produced with candidate SHA, demo SHA, workflow run and success result.

Do not launch duplicate release validation while an equivalent run is queued or in progress.

## 10. Evidence-driven issue/audit completion

After the exact candidate pre-release gate is green:

- [ ] Update release audit documents with the actual candidate SHA, demo SHA and relevant workflow/run evidence.
- [ ] Check off external acceptance items that could not be proven before pre-release.
- [ ] Close any remaining release-scoped issues whose final acceptance depended on that evidence.
- [ ] Reconfirm that all explicitly release-scoped issues are closed/completed before promotion.

## 11. Promotion: `develop -> main`

- [ ] Open exactly one release PR from `develop` to `main`.
- [ ] Release PR contains only the understood release delta.
- [ ] Reference the successful immutable pre-release approval/evidence.
- [ ] Review mergeability and the resulting tree.
- [ ] Merge only after the complete pre-release gate is green.
- [ ] Verify the resulting `main` release commit contains the intended release tree.

## 12. Tag and publish

- [ ] Create the immutable release tag on the resulting `main` release commit.
- [ ] Never move a published version tag.
- [ ] Confirm the tag commit is contained in `main`.
- [ ] Release workflow locates and validates the matching immutable pre-release approval.
- [ ] Release-only/supplemental validation succeeds when required.
- [ ] Matching multi-arch runtime image is published.
- [ ] GoReleaser publishes the expected archives and checksum file.
- [ ] GitHub Release is created.
- [ ] Build/runtime provenance attestations are created.

## 13. Post-release verification

A pushed tag is not release completion.

Verify:

- [ ] GitHub Release exists and points to the correct tag/commit.
- [ ] `baseharbor_linux_amd64.tar.gz` exists.
- [ ] `baseharbor_linux_arm64.tar.gz` exists.
- [ ] `checksums.txt` exists and validates.
- [ ] Binary `baha version` metadata reports the expected version/commit.
- [ ] GitHub provenance/attestations are present and verifiable.
- [ ] The matching versioned `ghcr.io/mcpdev80/baseharbor-runtime:<version>` image exists.
- [ ] expected compatible/minor/latest runtime tags were published.
- [ ] runtime image provenance/attestation is present.
- [ ] release references the correct pre-release evidence.
- [ ] GitHub Pages workflow from `main` is green.
- [ ] Published Pages shows the correct release notes, roadmap and current reference docs.
- [ ] External release-mode demo/dispatch is reviewed if the release workflow or project policy triggers it.
- [ ] No release-scoped issue remains accidentally open.
- [ ] Roadmap/current-version messaging now points to the next release.

## Result

Record a concise release audit with exact immutable identifiers:

```text
Release audit: vX.Y.Z

Scope/issues             PASS
BaseHarbor implementation PASS
Contracts/standards      PASS
Docs EN/DE               PASS
Roadmap/staleness        PASS
Changelog/release notes  PASS
baseharbor-demo          PASS
GitHub Pages source      PASS
Pre-release gate         PASS
Candidate SHA            <sha>
Demo SHA                 <sha>
Pre-release run          <run>
Release PR/main          PASS
Tag/publish              PASS
Artifacts/provenance     PASS
GitHub Pages published   PASS

Notes:
- only meaningful exceptions or intentionally deferred work
```

Any materially incorrect documentation, stale machine/demo contract, incomplete release-scoped issue, failed mandatory acceptance/evidence gate, or missing published artifact blocks release completion.
