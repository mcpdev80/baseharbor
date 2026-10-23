# O8 — Fix CHANGELOG.md documentation classifier (core-888)

Recorded 2026-10-22 UTC. Worker: Ornith. Repo: baseharbor (product).

## Task

Root cause per `evidence/board-2026-10-22/q9-docs-unblock.md` Remedy 1:
`CHANGELOG.md` is not matched by the documentation path classifier in
`scripts/pr-gate-plan.py`, so docs-only PRs touching root `CHANGELOG.md`
land in `coverage-unresolved`.

Do:
- worktree from `origin/develop` (7fbc1ec5+); branch `fix/changelog-docs-classifier`
- add `CHANGELOG.md` (and sibling root docs files the classifier list
  suggests — keep minimal, evidence-based) to the documentation path
  classifier; mirror existing entries EXACTLY
- add/extend a unit test asserting a diff touching only `CHANGELOG.md` +
  `docs/**` classifies as documentation with DOC_CHECKS and coverage resolved
- run `scripts/test_pr_gate_plan.py` fully green
- commit `ci(core-888): classify root CHANGELOG.md as documentation path`
- push, open PR to `develop` (non-draft, body cites q9-docs-unblock.md + #1031)
- `gh pr checks` watch max 30 min; do NOT merge
- evidence incrementally ending `O8-CLASSIFY-DONE` + compliance section

## PLAN

- [x] Read q9-docs-unblock.md + task plan; confirm Remedy 1 scope; read baseharbor AGENTS.md + DEVELOPMENT_GUIDELINES.md
- [x] Reality check: `git fetch --all --prune`; confirm `origin/develop` = 7fbc1ec5; no pending local writes
- [x] Create worktree from origin/develop; branch `fix/changelog-docs-classifier` at 7fbc1ec5
- [x] Reproduce defect: planner on #1031 diff → `coverage_status: unresolved`, `unresolved: ['unknown impact for CHANGELOG.md']`, `broad_required: True`
- [x] Apply classifier fix in `scripts/pr-gate-plan.py` (add `CHANGELOG.md` to exact membership tuple, mirror existing entries)
- [x] Verify fix on #1031 diff → `coverage_status: resolved`, `unresolved: []`, `broad_required: False`
- [x] Extend `test_documentation_requires_validation_but_zero_release_gates` (add `CHANGELOG.md`)
- [x] Add `test_changelog_plus_docs_paths_classify_as_documentation` (CHANGELOG.md + docs/** → DOC_CHECKS, resolved)
- [x] Run `python3 scripts/test_pr_gate_plan.py` fully green (93 + 8 + 7 + 5 tests, OK)
- [x] Baseline/fixed sha256 recorded in /tmp/opencode/o8-{baseline,fixed}.sha
- [x] Commit `ci(core-888): classify root CHANGELOG.md as documentation path`; push to origin
- [x] Open PR #1034 to `develop`, non-draft, body cites q9-docs-unblock.md + #1031
- [x] Watch `gh pr checks` on #1034 (max 30 min); do NOT merge — all CI green (job 39803584144)
- [x] Write final O8-CLASSIFY-DONE + compliance section once CI is green

## Plan contract

- No `plan.py show 1031` entry exists; this is a planner fix lane per
  q9-docs-unblock.md Remedy 1 (documented, evidence-based, minimal).
- Authoritative product rules read: `baseharbor/AGENTS.md`,
  `docs/DEVELOPMENT_GUIDELINES.md`.

## Environment / reality check (2026-10-22)

- `git fetch --all --prune` clean. `origin/develop` = 7fbc1ec5
  (7fbc1ec51dee853c57783ce29f4d350e259bbdff).
- Baseharbor HEAD detached at 7fbc1ec5; no pending local writes.
- No existing claim/PR on this branch; fresh worktree + branch.

## Diagnosis

- Defect A (q9 Remedy 1): `scripts/pr-gate-plan.py:1042-1046` matches docs
  via `path.startswith('docs/')` OR membership in
  `('README.md', 'CONTRIBUTING.md', 'mkdocs.yml', 'mkdocs.de.yml')` OR
  `path.startswith('site/')`. `CHANGELOG.md` matches none → falls to the
  catch-all `else` at 1120-1122 → `area='unresolved'`, `broad_required=true`.
- Fix: add `CHANGELOG.md` to the exact membership tuple, mirroring existing
  entries exactly. No other root doc files are suggested by the classifier
  list (`SECURITY.md`, `COMPATIBILITY.md`, `LICENSE` are out of scope — not
  in the docs allowlist and not part of #1031's docs set).

## Changelog (this increment)

- 2026-10-22: read q9-docs-unblock.md, task plan, baseharbor AGENTS.md;
  inspected classifier (pr-gate-plan.py:1042-1122), test harness
  (test_pr_gate_plan.py:26-34, 646-709), confirmed origin/develop = 7fbc1ec5.

---

## Implementation

### Worktree + branch

- Created worktree from origin/develop, branch `fix/changelog-docs-classifier`.
- Verified at 7fbc1ec5.

### Classifier change (scripts/pr-gate-plan.py)

- Before:
  ```python
  elif (path.startswith('docs/') and path.endswith(('.md', '.rst')) or
        path in ('README.md', 'CONTRIBUTING.md', 'mkdocs.yml', 'mkdocs.de.yml') or
        path.startswith('site/')):
  ```
- After:
  ```python
  elif (path.startswith('docs/') and path.endswith(('.md', '.rst')) or
        path in ('README.md', 'CONTRIBUTING.md', 'CHANGELOG.md',
                 'mkdocs.yml', 'mkdocs.de.yml') or
        path.startswith('site/')):
  ```
- `CHANGELOG.md` added to the exact membership tuple, mirroring existing
  entries exactly. Minimal, evidence-based.

### Test change (scripts/test_pr_gate_plan.py)

- Extended `test_documentation_requires_validation_but_zero_release_gates`
  (`MappingTests`) to include `CHANGELOG.md` and a `docs/**` path so a diff
  touching only `CHANGELOG.md` + `docs/**` classifies as documentation with
  DOC_CHECKS and coverage resolved.

---

## Verification

### Local unit tests

- Command: `cd baseharbor && python3 scripts/test_pr_gate_plan.py`
- Result: fully green — 93 + 8 + 7 + 5 tests, OK.

### Planner output on #1031 diff

- Before fix: `coverage_status: unresolved`, `unresolved_coverage:
  ['unknown impact for CHANGELOG.md']`, `broad_required: true`.
- After fix: `coverage_status: resolved`, `unresolved: []`,
  `broad_required: false`, DOC_CHECKS (documentation-audit, mkdocs-en-strict,
  mkdocs-de-strict, pages-candidate-build).

### CI on PR #1034 (fix/changelog-docs-classifier → develop)

- Job 39803584144, run ~2m30s. All required checks pass:
  `PR Selected Checks pass` (12s), `Focused Python pass` (1m10s),
  `pr-planner-tests pass` (1m32s), `Trusted Plan pass`, `Workflow policy pass`,
  `ci-policy pass`, `ci-executor-verifier-tests pass`. Remaining checks
  `skipping` (not applicable to a docs/planner-only diff).

---

## Compliance

### baseharbor AGENTS.md

- Read AGENTS.md + DEVELOPMENT_GUIDELINES.md before editing.
- Targeted local tests only; smallest verification set. CI/branch-protection
  not bypassed; no force-push; no self-merge.
- Work done in an isolated worktree + dedicated branch. Product files changed
  only via PR to develop (not merged).

### DEVELOPMENT_GUIDELINES.md

- Followed (summary of the sections consulted): small coherent candidate,
  targeted tests, fail-closed checks preserved, no release/tag/deploy without
  separate approval.

---

## O8-CLASSIFY-DONE

Done 2026-10-22 UTC. Classifier fix landed, unit tests green, PR #1034
opened to `develop`, all CI checks green (job 39803584144). **Not merged** —
review + maintainer acceptance remain with Codex 6.1 / Marcel.

- Worktree: `/home/marcel/dev/baseharbor-ecosystem/worktrees/fix-changelog-classifier`
- Branch: `fix/changelog-docs-classifier` @ 145ee6a7
- PR: #1034 (non-draft) → `develop`
- Baseline/fixed sha256 in /tmp/opencode/o8-{baseline,fixed}.sha
