# o1-binding-rebase (2026-10-19)

Task: t-20261019-core921-coverage-binding
Branch: `work/ci-core921-coverage-binding` in `worktrees/ci-core921-coverage-binding`
(develop base `f2bad5ef` -> advanced to `19548bf0f06757bc8d7acc9f3f4001417ddd34ed`)
HEAD (committed): `be82d726821deedf607a608e207c73587999cc8a`

## Steps taken

1. `git fetch origin`; merged `origin/develop` (f2bad5ef .. 19548bf0) preserving
   my clause work and EVERY accepted coverage clause on develop (no removals, no
   weakening). Merge was clean via stash/merge/pop: local changes protected by
   git, auto-merged back, clause intact. Confirmed all accepted coverage clauses
   still present: #991 (CORE_991, x3), #1005 SQL three-file clause (x4), #974
   REGISTRATION_CI_REVIEWED (x6), #982 (x3), #917 (x9), #918 (x9), #893 (x9),
   #751 (x2). My core-921 clause remains between #974 and the WIZARD clause.
   develop diffs this round: `runtime_environment.go`, `runtime_postgres.go`
   (481/474 line refactor — split), podman/acceptance CI, `ci-podman-typed-consent.py`,
   plus `scripts/pr-gate-plan.py` +56 (the #1005 exact three-file SQL clause +
   layout ratchet) and `scripts/test_pr_gate_plan.py` +181 (new develop suites).

2. Guarded the rejected candidate: `CORE_921_PRODUCTION_REVIEWED_HEAD` keeps the
   placeholder `756b22dc8967d0daa7393065cd5bc15f262c7050` with a clear
   `TODO(core-921)` comment that it is the REJECTED #998 candidate and MUST NOT
   resolve coverage until the final independently re-reviewed #998 SHA is set.
   Added an inline guard comment on the exact-match clause stating it is inert
   while the head pin is the placeholder and can never resolve for a head that is
   not the final re-reviewed SHA. Verified live: with the REAL
   `core752_reviewed_merge_chain` and pin `756b22dc`, coverage stays
   `unresolved`.

3. Fixed the one failing test from the pause note (`test_mutated_reviewed_head_and_dirty_merge_chain_do_not_resolve`): the git fixture now builds `target`/`source`
   from the same baseline, `head = merge(source, target)` (first parent target,
   second source), and `integration` via `--no-commit` then `--commit` for exact
   `[target, head]` parent order; `core752_reviewed_merge_chain` is forced to
   return False. All three core-921 regression tests now pass.

4. Committed coherently (no `git add -A`): `be82d726 ci(core-921): bind reviewed
   registration coverage (inert pending re-review)`. Did NOT push, did NOT open a
   PR — the final head/status set freezes only after #998's corrected source
   passes a fresh independent review.

## Test totals (committed state)
- `python3 -m py_compile` on both scripts: pass.
- `python3 scripts/test_pr_gate_plan.py`: 81 tests total
  (61 + 8 + 7 + 5 across 4 unittest modules), all OK. Includes the 3 new
  `Core921ProductionRegistrationCoverageTests` (exact reviewed bundle resolves
  with full CI+CODE+CLI+DOC check set; dirty/unreviewed head + forced-failing
  merge chain stays unresolved; deleted added path stays unresolved).
- `python3 scripts/test_pr_check_runner.py`: 18 tests, OK.

## State
- Working tree clean after commit.
- No push, no PR, no CI run (per coordinator hold).
- Next step (unblocked): after #998 corrected source passes a fresh independent
  review, replace the placeholder `CORE_921_PRODUCTION_REVIEWED_HEAD` with the
  final re-reviewed SHA, re-run the tests, then commit, push, and open the PR
  `gh pr create --repo mcpdev80/baseharbor --base develop
  --head work/ci-core921-coverage-binding` titled "ci(core-921): bind reviewed
  registration coverage".
- Evidence: o1-core921-review.md (read-only review, APPROVE), o1-binding-pause-note.md (pause note).
