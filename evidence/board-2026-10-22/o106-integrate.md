# o106 – Integrate both CI changes for Meta-Gate-Grün, f39-Diagnose

**Mission** (`ops/missions/o106.txt`): Integrate two CI changes so Meta-Gate is green, plus the f39 diagnostic.
**Git scope**: `baseharbor` only, ABSOLUTE paths.
**Owner-Freigabe-Linie**: PR #1056 (Hook-PR) is review-records-verified + tests green; #1056 itself removes the blocking `generik` rule.

## PLAN

### Step 0 — Setup
- Evidence file created here (`evidence/board-2026-10-22/o106-integrate.md`).
- Native todowrite: `[wip:o106]`.

### Step 1 — Merge PR #1056 (baseharbor)
- Command: `gh pr merge 1056 --repo mcpdev80/baseharbor --merge`
- Rationale: Review-Records-geprüft + Tests grün; #1056 entfernt die blocking generik-Regel.

### Step 2 — Rebase #1055 branch onto new develop
- Branch: `work/ci-explicit-candidate-trigger-20261024` (PR #1055)
- Target: new `develop` (force-with-lease allowed)
- Tests must run: `test_ci_candidate_trigger.py` + `test_pr_gate_plan.py`

### Step 3 — Update PR #1055
- Push rebased branch, update PR.

### Step 4 — Post-Merge-CI check on develop (#1056)
- If #1056-Check after merge stays RED on develop: STOP immediately, report BLOCKED with log location.
- All actions documented in this evidence.

### Step 5 — END
- Record: `develop-sha` + `1055-status`.

## repo-rule compliance
- (to be filled)

## Evidence log

| Step | Timestamp (local) | Action | Result |
|------|-------------------|--------|--------|
| 0 | 2026-10-24 | Evidence + todowrite created | done |
| 1 | | merge 1056 | |
| 2 | | rebase 1055 | |
| 3 | | push PR 1055 | |
| 4 | | post-merge CI | |
| 5 | | END | |
