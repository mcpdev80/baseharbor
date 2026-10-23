# o92 Metagate — Evidence

State: [wip:o92]
Date: 2026-10-24
Scope: baseharbor only, git via baseharbor paths, ROOT-git forbidden

## PLAN

### Step 0: Turn-start / context
- Ran `python3 evidence/board-2026-10-19/scripts/basics-check.py`: EVENTS 1 un-acked, IDLE LANE o90-gateplan-pins, PLAN 16 open steps. Gateway healthy (0.01s).
- o92 metagate task: diagnose "PR Selected Checks" failure in PR #1055 (head `work/ci-explicit-candidate-trigger-20261024`) and PR #1056 (`o90-planner-pr`), find why scripts+workflows-only PRs get skipped instead of selected, deliver 1-line recipe.

### Step 1: Fetch meta-job logs for both PRs (gh api jobs/steps)
### Step 2: Inspect current `scripts/pr-gate-plan.py` + `ci.yml` job-if on both branches (git show <head>)
### Step 3: Determine Planner runtime in meta job (BASE vs HEAD) and exact skip condition
### Step 4: Produce 1-line fix recipe for #1056 meta-gate
### Step 5: END O92-METAGATE-DONE

## repo-rule compliance
- git used only under baseharbor paths; ROOT-git forbidden.
