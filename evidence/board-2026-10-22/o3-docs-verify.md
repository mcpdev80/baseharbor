# o3-docs-verify — BaseHarbor docs lane (v0.4.26)

Recorded 2026-10-22 UTC. Coordinator: Flash. Worker lane: o3 (docs).
Task: unpublished docs for the v0.4.26 New/Edit + provider-selection change.

## Outcome

Docs candidate built, validated, and pushed as DRAFT PR #1031 to develop.
All documentation content and the four DOC_CHECKS pass locally. One CI
blocker remains that is outside the docs lane (see Blocker below).

## Remote state verified

- `develop` = `30e112431b04a7e24e7e7e9ea76a3600e439a988` (matches handoff
  accepted-integration head). `213c15e4` (podman #1027) is a clean ancestor
  of develop; the patch base is correct.
- Local `0c8a26c8e9c9be6952f3d5864a99d2dfed9d8e2b` ABSENT (as the handoff
  predicted). Fallback to the handoff patch.
- An upstream `docs/v0426-new-edit` already existed (`origin` head
  `e1206a6a`), but it was based on an unrelated `61d6ab56` CI commit and
  diverged heavily (scripts/tests). No open PR on it. Not reused. A fresh,
  disjoint branch `docs/v0426-new-edit-16` was used instead.

## Patch application

- Local commit `0c8a26c8` absent -> applied
  `docs-16-paths-unpublished.patch`.
- Patch SHA256: `63ac7494aafc661b74455e96ff5282376825122b7d5a6e6e6aac8897c10be227`
- Applied clean against `origin/develop` (30e11243); no overlap with the two
  post-213c develop commits (`00882c3c`, `30e11243`, both CI/Docker). No
  conflict resolution was needed.
- 16 files, 446 insertions / 11 deletions: CHANGELOG.md, README.md,
  docs/cli/applications.md, docs/de/cli/applications.md,
  docs/de/how-to/application-needs.md, docs/de/index.md, docs/de/releases/index.md,
  docs/de/releases/v0.4.26.md, docs/de/tutorials/getting-started.md,
  docs/how-to/application-needs.md, docs/index.md, docs/releases/index.md,
  docs/releases/v0.4.26.md, docs/tutorials/getting-started.md, mkdocs.de.yml,
  mkdocs.yml.
- Commit `ef41f079` (16 paths).

## Validation trio (local, before push)

- `scripts/documentation-language-audit.py`: German navigation + human-guide
  coverage PASS (41 pages).
- `--built-site` mode: German rendered links and assets PASS (17749 references).
- `scripts/check_docs_locale_parity.py`: EN/DE page coverage OK (158 matched
  Markdown pages).
- `scripts/check_de_translation_integrity.py`: DE technical integrity PASS
  (158 paired pages).
- `bash scripts/documentation-audit.sh`: all sections PASS (structure,
  human-doc size, no legacy stubs, German scope, internal evidence, ADR
  identifiers, governance, runtime docs, task/domain IA, Podman release CI,
  brand identity).
- Go generated-reference tests PASS:
  `TestHumanCommandIndexTracksExactCommandMetadata` (EN+DE),
  `TestHumanDocumentationUsesFinalInitializationPath`,
  `TestMachineDocumentationTracksRegistryAndCommandTree`.
- `mkdocs build --strict` EN: exit 0. `mkdocs build --strict` DE: exit 0.
  (mkdocs 1.6.1 + material 9.7.7 installed to bootstrap the build locally;
  CI pins the same versions via requirements-docs.txt.)

## Unreleased / published-claim check

- `CHANGELOG.md`: `## [Unreleased]` is empty; the new `## [0.4.26]` candidate
  entry sits directly beneath it. Correct.
- v0.4.26 is only ever described as "candidate" / "not yet published"
  (`docs/releases/v0.4.26.md`, `docs/releases/index.md`). README says
  "not yet release-ready". The published baseline is v0.4.25 in EN and DE.
- grep for `published` near v0.4.26: only legitimate "not yet published" and
  "published baseline is v0.4.25" strings. No v0.4.26 release claim.

## Coverage gap found and fixed

`scripts/pr-gate-plan.py` mapped `docs/**/*.md`, `README.md`,
`CONTRIBUTING.md` and both `mkdocs*.yml` to the `documentation` coverage area
(requiring DOC_CHECKS), but did NOT list `CHANGELOG.md`. As a result
`CHANGELOG.md` fell through to the catch-all `unresolved` area and produced
`coverage-unresolved: unknown impact for CHANGELOG.md`, failing the final
`PR Selected Checks` gate.

Fix: added `CHANGELOG.md` to the documentation-area file list at
`scripts/pr-gate-plan.py:1005`. Verified locally with the real 17-change diff:
`coverage_status: resolved`, `unresolved_coverage: []`, required
`required_validation_checks` = documentation-audit, mkdocs-de-strict,
mkdocs-en-strict, pages-candidate-build (all four pass on the runner). Existing
`scripts/test_pr_gate_plan.py` still passes (5 tests).

This is a docs-lane-adjacent verifier fix, not a product change.

## Blocker: integration_sha verifier (outside docs lane)

The final `PR Selected Checks` gate fails with
`coverage_status: unresolved`. Root cause is NOT the docs content and NOT the
CHANGELOG fix. It is the `integration_sha` requirement in
`scripts/pr-diff-base.py:132-146`:

- CI passes `--integration github.sha`. For a `pull_request` event
  `github.sha` is the branch tip, a 1-parent commit.
- `pr-diff-base.py` requires the integration commit to have exactly
  `[target_base_sha, head_sha]` as its two parents (a synthetic merge commit).
  A 1-parent branch tip triggers `integration_sha_mismatch`, which short-
  circuits to a 0-change diff and then `coverage_status: unresolved`
  ("empty or unverified changes cannot exempt checks").
- This is a pre-existing verifier/CI interaction: the accepted PR heads from
  the handoff (`fe2d0c4a`, `213c15e4`, `00882c3c`, `30e11243`) are also
  1-parent commits, so they would hit the same gate. It is not introduced by
  this docs PR and is out of the docs lane to resolve (it would require a
  change to the shared `pr-diff-base.py` verifier and/or `ci.yml`, which is
  product scope).

CI run evidence (repository mcpdev80/baseharbor, workflow CI):
- Run `39790289491` @ `c1a43f3b` (13:49 UTC): failed, `unknown impact for
  CHANGELOG.md` — a 17-change diff; this run occurred during a rapid double
  push and read intermediate state.
- Run `39792347965` @ `f1ebaa70` (14:03 UTC): failed, generic
  `coverage_status: unresolved` — the `integration_sha_mismatch` path (0-change
  diff). Deterministic for a 1-parent branch push.

All other checks pass on both runs: Workflow policy, PR Trusted Plan, Focused
Python, documentation-audit, ci-policy, pr-planner-tests, mkdocs-en-strict,
mkdocs-de-strict, pages-candidate-build, ci-executor-verifier-tests. Only the
final coverage-verifier step blocks.

## DRAFT PR

- URL: https://github.com/mcpdev80/baseharbor/pull/1031
- Branch `docs/v0426-new-edit-16` -> base `develop`. Title `docs(v0.4.26)`.
- State: DRAFT / OPEN. Body cites the handoff, the patch SHA256, the absent
  local `0c8a26c8`, and the local validation outputs.
- Commits: `ef41f079` (16 docs paths), `c1a43f3b` (CHANGELOG coverage fix),
  `f1ebaa70` (DE release-notes paragraph fix, genuine formatting correction
  in `docs/de/releases/v0.4.26.md`).

## Note for owner

The docs work is complete and correct; every documentation and generated-
reference check passes. The only open item is the `integration_sha` coverage
verifier, a pre-existing shared-verifier limitation that blocks every 1-parent
branch-push PR (including the accepted integration heads). It needs an owner
decision on whether the verifier should tolerate a non-merge branch tip for
draft PRs, or whether a merge-commit integration SHA is expected. This is
outside the docs lane; reporting per handoff protocol.

END O3-DOCS-DONE.
