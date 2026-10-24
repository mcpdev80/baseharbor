# O107 – EXCMERGE (PR 1055 → 1056 on develop)

State: `[wip:o107]`
Owner: Codeman (delegated Maintainer-Aktion / Akte-Hook-Kontext)
Start: 2026-10-24

## Mission (from ops/missions/o107.txt)
1. `gh pr checks 1055` bis alle final (nicht queue).
2. `gh pr merge 1055 --repo mcpdev80/baseharbor --merge --admin`.
3. `gh pr view 1056 --json headRefOid,mergeStateStatus`; PR auf develop 24c33afd+ rebasen. Merge von 1055 loest Konflikt? -> `gh pr merge 1056`. Base-Konflikt? -> branch o90-planner-pr auf neuen develop rebase (origin, force-with-lease), dann merge.
4. develop Post-Merge-CI bis final.
5. Alles in Evidence.

## Remote-Zustand
- remote #1055 head: 8f40ce4f (Push laengst done)

## Step 1 – checks 1055
- PR 1055 was DRAFT -> blocked API merge. Fixed with `gh pr ready 1055` (marked ready for review).
- Repository disallows merge commits; `--merge` failed ("Merge commits are not allowed"). Rebase disallowed too. Squash is the only path.
- Waited for all CI to finalize (not queue): Workflow policy=pass, ci-policy=pass, ci-executor-verifier-tests=pass, Trusted Plan=pass, GC tests=pass; others skipping. BUT the job "PR Selected Checks" (run 40068186658, job 120796049337) FAILED with BLOCKED: coverage-unresolved.

## BLOCKER – PR 1055 native-coverage gate fails (diagnosed)
- Squash merge blocked: `Required status check "PR Selected Checks" is expected.`
- "PR Selected Checks" is an independent fail-closed verifier (scripts/pr-required-final.py). It ran and returned exit 1:
  `BLOCKED: coverage-unresolved: workflow execution/cleanup/evidence impact requires native qualification: .github/workflows/artifact-gc.yml; .github/workflows/ci.yml; unknown impact for scripts/submit-pr-candidate.py; scripts/test_ci_candidate_trigger.py`
- Root cause reproduced locally with the real PR inputs:
  - base=1861ed07888a1ec22c218ca491d23ee54f48e261, head=8f40ce4f850c59048111d0c54a9a7276361e68cc, integration=91f1b206783c4d5a5420b0c79d8300eed40bd2d6 (parents exactly 1861ed + 8f40ce4f -> valid merge).
  - `scripts/pr-gate-plan.py` maps the 4 changed files to `runtime-workflow-unresolved`/`ci-executor-unresolved` areas with NO resolution path: CI files require native qualification that is absent.
  - The Core-921 `trusted_binding_checks` record (docs/internal/ci-binding-qualifications.md) covers DIFFERENT files (pr-check-runner.py, pr-gate-plan.py, test_pr_gate_plan.py) AND its reviewed_head (480060640...) is NOT an ancestor of PR #1055's base -> merge-chain check fails, record cannot apply.
- Scope: 18 of 28 PR CI runs failed (Oct 23–24) — this native-coverage gate blocks the whole develop branch, not an anomaly. PR #1055 changed 4 CI/executor files with no qualification binding -> genuinely unmergeable in current repo state.

## Step 2 – merge 1055
- BLOCKED: PR 1055 cannot be merged (squash) because the required "PR Selected Checks" fail-closed verifier reports coverage-unresolved. --admin cannot bypass a required status check. No merge commit issued.

## Step 2 – merge 1055
TODO: output

## Step 3 – PR 1056 (BLOCKED, cannot merge in current state)
- PR 1056 head=38ba06bb, base=24c33afd, integration=6e86c42b (parents exactly 24c33afd + 38ba06bb -> valid merge), 3 changed files: scripts/pr-check-runner.py, scripts/pr-gate-plan.py, scripts/test_pr_gate_plan.py.
- Same fail-closed gate blocks it: run 40066777947 job 120791913646 -> `BLOCKED: coverage-unresolved: CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py`.
- Root cause: The Core-921 binding record (docs/internal/ci-binding-qualifications.md) reviewed_head = 4e737e26af... is NOT in develop history (lives on a side branch). `core752_reviewed_merge_chain` therefore fails -> record cannot qualify these 3 CI-script paths.
- PR 1056's head commit 38ba06bb literally attempts to make the qualification register the sole authority for those paths — i.e. the intended resolution is not yet landed and gated by the same unverifiable coverage check.
- Cannot merge 1056 (squash) either: required "PR Selected Checks" stays red.

## Step 4 – develop Post-Merge-CI
- NOT RUN: no merge of 1055 or 1056 succeeded. develop HEAD unchanged at 24c33afd.

## Step 4 – develop Post-Merge-CI
TODO: output

## Summary / blocker for owner
- PR 1055: ready-for-review set, all runnable CI green, but squash BLOCKED by fail-closed native-coverage gate (4 CI/executor files, no reachable binding record). Owner-authorized --admin cannot bypass a required status check.
- PR 1056: same gate blocks it (3 CI-script paths; Core-921 reviewed_head not in develop history).
- Neither PR merged. develop stays at 24c33afd. Both are architecturally blocked, not operationally forgettable.
- Recommended owner decision (do NOT auto-proceed): either (a) land a reachable Core-921 binding record on develop with the exact file hashes for the current PR heads, or (b) extend the planner's qualification for these CI-script/workflow paths, or (c) explicitly waive the native-coverage gate for these two PRs. Any of these needs an explicit Owner stop before another attempt.

## END
develop-sha: 24c33afdc89f166bf4ed747445078fa47489b562 (unchanged)
1055 merge-commit: NONE (blocked)
1056 merge-commit: NONE (blocked)
