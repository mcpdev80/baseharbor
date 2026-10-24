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
| new head | (pending) |
| push method | force-with-lease |
| CI checks | (pending) |

### #1056 — o90-planner-pr
| field | value |
|-------|-------|
| original head | 38ba06bb |
| base (develop) | a2abc17a |
| new head | (pending) |
| push method | force-with-lease |
| CI checks | (pending) |
| meta-fail root | scripts/pr-gate-plan coverage-unresolved |

## o109-Diagnose findings
(pending)
