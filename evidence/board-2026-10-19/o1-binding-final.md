# o1-binding-final (2026-10-19)

## Task: t-20261019-core921-coverage-binding — FINALIZED

Branch: `work/ci-core921-coverage-binding` in `worktrees/ci-core921-coverage-binding`
Base: develop `bb91fbc327414952b36a88271153db43970280cc` (merged into branch)
Head SHA (committed, pushed): `417b8cbb03d585a2607663f44eb38c1c9e13250d`
PR: **#1007** — https://github.com/mcpdev80/baseharbor/pull/1007
  - title: `ci(core-921): bind reviewed registration coverage to 08d8f498`
  - base: `develop`, head: `work/ci-core921-coverage-binding`, non-draft, OPEN
  - body cites o4-998-rereview.md (review 5569055023 closed), ci-finding-integration-sha-mismatch.md

## Binding head SHA

`CORE_921_PRODUCTION_REVIEWED_HEAD = 08d8f498fd82992e762d764967ac892721d9adeb`
Final reviewed head after fresh independent full-source + delta re-review of the
corrected #998 source (o4-998-rereview.md). This is the head the clause now
binds.

## Exact file/status table (CORE_921_PRODUCTION_REVIEWED_FILES)

Derived from the exact two-dot diff `origin/develop..08d8f498` (15 files):

| status | path |
| --- | --- |
| modified | cmd/baha/app_machine_adapters.go |
| modified | cmd/baha/app_provider_choice.go |
| added | cmd/baha/app_registration_921.go |
| modified | cmd/baha/commands.go |
| modified | cmd/baha/machine_coverage.go |
| modified | cmd/baha/mcp_tools.go |
| added | cmd/baha/registration_921_test.go |
| modified | contracts/machine/v1/control.golden.json |
| modified | docs/cli/command-index.md |
| modified | docs/de/cli/command-index.md |
| modified | docs/de/reference/cli-machine-coverage.md |
| modified | docs/de/reference/mcp.md |
| modified | docs/reference/cli-machine-coverage.md |
| modified | docs/reference/mcp.md |
| modified | internal/machine/contract.go |

Excluded from the bundle: the 3 unrelated process-doc changes in the same two-dot
diff (`docs/DEVELOPMENT_GUIDELINES.md`, `docs/de/DEVELOPMENT_GUIDELINES.md`,
`docs/internal/review-protection.md`) — not core-921 registration work.
Counts: 13 modified, 2 added.

## Clause

One exact-match clause in `scripts/pr-gate-plan.py` mirroring the #974
registration-CI clause: `len(paths)`/`set(paths)`/per-path status must equal the
15-file set, plus `core752_reviewed_merge_chain(target, head, 08d8f498)` clean
trusted-target merge. Selects `CI_CHECKS` + `CODE_CHECKS` + `CLI_CHECKS` +
`DOC_CHECKS`; fails closed for any non-exact set. All other accepted coverage
clauses on develop preserved (no removals/weakening): #991, #1005, #974, #982,
#917, #918, #893, #751.

## Regression tests (scripts/test_pr_gate_plan.py, Core921ProductionRegistrationCoverageTests)

- `test_exact_reviewed_bundle_resolves_with_full_check_set` — exact clean merge
  resolves with full CI+CODE+CLI+DOC check set.
- `test_mutated_reviewed_head_and_dirty_merge_chain_do_not_resolve` — dirty
  unreviewed head + forced-failing merge chain stays unresolved.
- `test_changed_status_and_extra_path_do_not_resolve` — deleted added path stays
  unresolved.
- `test_mutated_status_and_extra_missing_path_do_not_resolve` (new) — modified
  handler re-marked as added (wrong status) plus a dropped reference doc (missing
  path) stays unresolved.

## Test totals (all green)

- `python3 -m py_compile` on both scripts: pass.
- `python3 scripts/test_pr_gate_plan.py`: 82 tests total
  (62 + 8 + 7 + 5 across 4 unittest modules), OK.
- `python3 scripts/test_pr_check_runner.py`: 18 tests, OK.

## Commits

- `be82d726` ci(core-921): bind reviewed registration coverage (inert pending re-review) — placeholder guard.
- `417b8cbb` ci(core-921): bind reviewed registration coverage to 08d8f498 — FINAL head, tests, push, PR.

Owned files only across both commits: `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`.

## Next (Maintainer-only, not done by Flash)

PR #1007 is open for develop. Merge / release qualification / plan closing are
Maintainer decisions with evidence (ACCEPTED + green mandatory CI).

## Codex round 1

PR #1007 at `417b8cbb` returned CHANGES_REQUESTED (verdict
`codex-verdict-core921-round1.md`, round 1 of 2). Fixed both P1 and P2 on the
binding PR (CI green); #998 product findings are a separate task.

- **P1 — required coverage omitted.** The clause now returns `resolved` only
  after `require()` adds `wizard-registration-tests`,
  `wizard-machine-adapter-tests`, `wizard-provider-choice-tests` and the new
  `wizard-core921-tests` family (each `TestCore921` regression) on top of
  CI/CODE/CLI/DOC. Added `wizard-core921-tests` to `scripts/pr-check-runner.py`
  `WIZARD_TEST_FAMILIES` (5 patterns, `$`-anchored so zero-match fails closed).
  `cli-guided-tests` only runs `^TestGuided`, so the affected families are now
  explicitly named lanes that actually execute.
- **P2 — negative regressions.** Added `test_real_unreviewed_commit_on_reviewed_bundle_fails_real_verifier`
  (real `core752_reviewed_merge_chain` ancestry, no stubbed `False`, with an
  explicit sanity assertion the verifier rejects the unreviewed head) and
  `test_extra_missing_path_and_wrong_status_do_not_resolve` (real git rename
  flips status + drops a required doc + adds an extra path). The existing
  wrong-status test now re-adds via a real rename.

### Commit / head SHA

`79da28347218cebec53c34c303f59ac844b83cc7` — `ci(core-921): require affected
coverage families and strengthen negatives (Codex round 1)`. Branch
`work/ci-core921-coverage-binding` pushed to origin. PR #1007 updated.

### Gate list required by the clause (now actually selected)

`CI_CHECKS` (pr-planner-tests, ci-policy, ci-executor-verifier-tests) +
`CODE_CHECKS` + `CLI_CHECKS` + `DOC_CHECKS` +
`wizard-registration-tests` + `wizard-machine-adapter-tests` +
`wizard-provider-choice-tests` + `wizard-core921-tests`.

### Test totals (all green)

- `python3 -m py_compile` on all three scripts: pass.
- `python3 scripts/test_pr_gate_plan.py`: 84 tests total
  (64 + 8 + 7 + 5 across 4 unittest modules), OK. Core-921 suite: 6 tests
  (positive full-check-set + 5 negative: dirty head with real verifier, wrong
  status + extra path + missing path, extra/missing via rename, deleted added
  path).
- `python3 scripts/test_pr_check_runner.py`: 18 tests, OK.

### Pending (not done by Flash)

The binding head constant stays marked for final re-pin once #998's corrected
SHA lands. #998 requires its own correction, fresh current-base plan, actual
green mandatory CI and final acceptance before merge; #1007 merges first.
