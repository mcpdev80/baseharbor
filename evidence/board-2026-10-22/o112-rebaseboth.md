# o112-REBASEBOTH — Evidence

Status: `[wip:o112]`
Owner: Codeman (dispatch session)
Base: baseharbor (submodule), origin remote
Mission source: `ops/missions/o112.txt`

## PLAN

### Step 0 — Setup
- [x] Read mission `ops/missions/o112.txt`
- [x] Verify git topology: rebase inside baseharbor submodule only, never rebase the repo root
- [x] Create this evidence file (first action per PLAN step 0)
- [x] todowrite([wip:o112])

### Step 1 — Diagnose (o109)
- [ ] Confirm root cause of #1056 meta-fail = `scripts/pr-gate-plan` coverage-unresolved
- [ ] Confirm #1055 trigger branch is clean rebase candidate onto develop a2abc17a

### Step 2 — Rebase #1055 (work/ci-explicit-candidate-trigger-20261024)
- [ ] `git fetch origin develop` (fresh a2abc17a)
- [ ] rebase branch onto develop
- [ ] force-with-lease push
- [ ] collect CI checks for new head

### Step 3 — Rebase #1056 (o90-planner-pr, head 38ba06bb)
- [ ] rebase branch onto develop
- [ ] force-with-lease push
- [ ] collect CI checks for new head

### Step 4 — PR bodies with Akten-Referenz-Kommentar
- [ ] Both PRs: add body comment referencing `docs/internal/ci-binding-qualifications.md` + #1059
- [ ] Rationale: per diagnosis, PR body lacks explicit doc-reference; rebase-only, no code fix

### Step 5 — Close
- [ ] o112-REBASEBOTH-DONE with both new heads + check-tables

## Repo-rule compliance
- Rebase confined to baseharbor submodule; root repo untouched by rebase
- No merges, no code fixes — only rebase + PR comments + evidence (per mission)

## Check-tables

### #1055 — work/ci-explicit-candidate-trigger-20261024
| field | value |
|-------|-------|
| original head | 8f40ce4f |
| base (develop) | a2abc17a |
| new head | cf0fedd0e8863d73cb5715fcb4f7f3544f688c73 |
| push method | force-with-lease |
| push time | 2026-10-24 |
| CI run id | 40072074499 |
| PR Selected Checks | FAILURE — coverage-unresolved |
| block reason | .github/workflows/ci.yml, .github/workflows/artifact-gc.yml, scripts/submit-pr-candidate.py, scripts/test_ci_candidate_trigger.py lack qualification-register entry |
| other required checks | Trusted Plan SUCCESS, Focused Python SUCCESS, Workflow policy SUCCESS, Validation ci-policy/pr-planner-tests/ci-executor-verifier-tests/ci-actionlint SUCCESS |

### #1056 — o90-planner-pr
| field | value |
|-------|-------|
| original head | 38ba06bb |
| base (develop) | a2abc17a |
| new head | df8e53b9e1305b7281d47df141bd5d1696252508 |
| push method | force-with-lease |
| push time | 2026-10-24 |
| CI run id | 40070909163 (old ecde3f92, failed integration_sha_mismatch) |
| PR Selected Checks | SUCCESS |
| other required checks | Trusted Plan SUCCESS, Focused Python SUCCESS, Workflow policy SUCCESS, Validation ci-policy/pr-planner-tests/ci-executor-verifier-tests SUCCESS |

## o109-Diagnose findings
- #1056 meta-fail root cause confirmed: `scripts/pr-gate-plan.py` trusted-develop policy emits `coverage_status: unresolved` for the three CI-script paths (`scripts/pr-gate-plan.py`, `scripts/pr-check-runner.py`, `scripts/test_pr_gate_plan.py`) because the qualification register (`docs/internal/ci-binding-qualifications.md`) lacked an entry. Fail-closed `PR Selected Checks` FAILURE.
- o90 PR fixes exactly this: accepts qualification-register entries as sole authority for the three CI-script paths; new head `df8e53b9` passes `PR Selected Checks` = SUCCESS.
- #1055 (trigger branch) CI-impact files (`ci.yml`, `artifact-gc.yml`, `submit-pr-candidate.py`, `test_ci_candidate_trigger.py`) are NOT in the qualification register → `coverage-unresolved` fail-closed. This is the expected/required fail-closed state, not a rebase defect. Rebase-only mission: no code fix.

## o112-REBASEBOTH-DONE
- Both branches rebased onto develop `a2abc17a` and force-with-lease pushed.
- #1055 new head `cf0fedd0` (base a2abc17a). CI run 40072074499: PR Selected Checks FAILURE (coverage-unresolved, correct fail-closed — register entry pending).
- #1056 new head `df8e53b9` (base a2abc17a). CI: PR Selected Checks SUCCESS (qualification register resolves the previously-failing gate).
- Both PR bodies carry explicit Akten-Referenz-Kommentar referencing `docs/internal/ci-binding-qualifications.md` and PR #1059.
- No merges, no code fixes — rebase + comments + evidence only, per mission `ops/missions/o112.txt`.
