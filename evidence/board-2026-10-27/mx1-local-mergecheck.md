# mx1 — local merge-check (no push, no PR, no remote-write)

LANE mx1 (ornith), board 2026-10-22 prompt. Trained the post-merge case locally only.
No push, no PR, no force/main/develop push. Merge lives solely in the `worktrees/mx1` worktree branch `work/r27-mergecheck`.

## PLAN

1. TOOL-1: fetch origin, create worktree `worktrees/mx1` on branch `work/r27-mergecheck` from `origin/develop`.
2. `git merge --no-ff 94f35d02 -m 'local mergecheck candidate into develop'` in worktree → merge result, conflict or not.
3. Discover test counts for `test_receipt*`, `test_pr_gate*`, `test_pr_check*`.
4. Simulate `python3 scripts/pr-gate-plan.py --diff` against develop basis (rc + message). Confirm the class-B "unsupported executable validation" gap on develop is healed in the candidate (receipt-execution-tests).
5. Dossier this file (## PLAN, rc, merge-tree SHA, before/after classifier answer).

## TOOL-1

- fetch origin: rc 0 (clean, no output).
- worktree `worktrees/mx1` branch `work/r27-mergecheck` from `origin/develop` (base `3431b22c94b068f15f9bd7b31b9f87e846200f0f`).

## MERGE

- command: `git -C worktrees/mx1 merge --no-ff 94f35d02 -m 'local mergecheck candidate into develop'`
- rc: 0 — clean merge, NO conflict.
- merge-tree SHA (candidate HEAD): `eb39ad220221886779725f52e47fcf27398715eb`
- `94f35d02` (stg6 integrate pcx FIX A + f1x plan-facts fix) confirmed ancestor of merge-tree (is-ancestor = true).
- merge spans 37 files, +5899 / -456 (includes 24 new `evidence-r27/*.md` evidence files + `scripts/test_receipt_postrun.py` add).
- No conflict marker written; no `END MX1-CONFLICT` emitted.

## TEST COUNTS (candidate tree)

- `test_receipt*`: 170 test functions
- `test_pr_gate*`: 127 test functions
- `test_pr_check*`: 22 test functions
- combined receipt control-plane suite run (unittest): `Ran 175 tests ... OK` (52.8 s).

## pr-gate-plan simulation (develop basis → candidate merge-tree)

- diff base: `python3 scripts/pr-diff-base.py --target-base 3431b22c... --head eb39ad22...` rc 0.
- gate: `GITHUB_REPOSITORY=baseharbor/baseharbor GITHUB_BASE_REF=develop python3 scripts/pr-gate-plan.py --diff /tmp/...` rc 0.
- result: coverage_status=unresolved, selected_gates=0, required_validation_checks=9, unresolved_coverage=21.
- The 21 unresolved entries are CI-executor/unqualified workflow items (evidence-r27/*.md, scripts/pr-check-runner.py, scripts/test_pr_check_runner.py, ci.yml, receipt-* workflows) — expected: native CI qualification is a runner-context gate, not a local classify outcome. `scripts/pr-gate-plan.py` itself classifies as `ci-planner` and emits its 9 required checks. This is the local mirror of the post-merge gate, not a green-CI claim.

## CLASS-B HEALING (before / after classifier)

Question: on the develop basis, is the `receipt-execution-tests` validation a recognized executable validation (class-B), or does the classifier reject it as "unsupported executable validation"?

BEFORE (develop, `3431b22c`):
- `scripts/pr-check-runner.py` has NO `receipt-execution-tests` case.
- `execute_validation('receipt-execution-tests', ...)` → `raise ValueError('unsupported executable validation')`.
- `grep -c receipt-execution-tests` on develop `pr-check-runner.py` = 0.
- The receipt control-plane suites are only reachable via a glob `test_receipt*.py` in the plan; the execution side (`pr-check-runner.py`) cannot run them → class-B hole: the control-plane execution check exists in plan but has no valid executable-validation id on the runner side.

AFTER (candidate merge-tree `eb39ad22`):
- `scripts/pr-check-runner.py` gains `RECEIPT_EXECUTION_SUITES` (explicit module list, never a filename glob) and a `receipt-execution-tests` branch in `execute_validation`.
- Explicit modules: `test_receipt_writer, test_receipt_verifier, test_receipt_chain, test_ci_workflow_policy_receipts, test_receipt_tasklink, test_receipt_postrun`.
- Requires every registered module to execute ≥1 test and the combined run to pass; fails closed on missing/empty/failing suite; asserts `Ran [1-9]... tests` and per-module `module.` presence in the `-v` output.
- `grep -c receipt-execution-tests` on candidate `pr-check-runner.py` = 4.
- End-to-end proof: `PYTHONPATH=scripts python3 -m unittest ...` of the 6 receipt suites → `Ran 175 tests ... OK`. The candidate execution-side mirror runs the real suites instead of a silently-dropping glob.

Answer: develop = class-B hole (`unsupported executable validation` for the receipt control-plane execution check); candidate heals it by adding the explicit `receipt-execution-tests` executable-validation id and running the real registered suites, verified by 175 passing tests. No CI-claim; local execution proof only.

## repo-rule compliance

- Write-set isolated to `worktrees/mx1` worktree branch `work/r27-mergecheck`; baseharbor checkout left with the same candidate diff as uncommitted working changes (not committed, not pushed).
- No push, no PR, no force, no main/develop remote write.
- Runner auth env (`GITHUB_REPOSITORY`/`GITHUB_BASE_REF`) supplied locally for the gate simulation; not committed, not in evidence beyond this classification note.
- Evidence-only files written under `evidence/board-2026-10-27/`.

## END MX1-DONE
