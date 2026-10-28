# LANE bx3 — minimal executable-validation bootstrap (receipt-execution-tests)

- Lane: bx3 (ornith), evidence board `board-2026-10-22`
- Branch: `work/r27-boot-validator2` (worktree `worktrees/bx3`), based on `origin/develop` @ `3431b22c94b068f15f9bd7b31b9f87e846200f0f`
- Prompt: `evidence/board-2026-10-22/prompts/bx3.txt`
- Goal: develop's trusted runner must know the `receipt-execution-tests` classifier so
  `class-b ('unsupported executable validation')` stops failing in PR-CIs where the
  planner (as in fix commit `970ffcd4`/`cdddf21d`) emits that id.

## PLAN

1. Reality check: `origin/cdddf21d` is `w5: add evidence for receipt execution-suite
   order fix`, an executable-validation *registry/Classifier* is not a literal token
   in `pr-gate-plan.py` history (git log -S returns nothing). The real class-b guard
   lives in `scripts/pr-check-runner.py`; the classifier that must be transplanted is
   `receipt-execution-tests`.
2. Source of truth for the transplant: fix commit `970ffcd4` (identical net effect as
   `cdddf21d`'s CI_CHECKS emitting `receipt-execution-tests`).
3. Transplant minimal subset: add `receipt-execution-tests` to `VALIDATIONS`, add
   `RECEIPT_EXECUTION_SUITES`, add the `execute_validation` branch, guard whitelist,
   `import os`, plus the corresponding test.
4. Acceptance (a) scope, (b) pytest, (c) `pr-gate-plan.py --help`, (d) this file.

## Hunk table (adopted / discarded + reason)

| File | Hunk | Status |
| --- | --- | --- |
| pr-check-runner.py | `import os` (os.environ is used by the classifier) | ADOPTED — runtime requirement of the classifier, added by upstream 970ffcd4 |
| pr-check-runner.py | `receipt-execution-tests` in VALIDATIONS set | ADOPTED |
| pr-check-runner.py | guard `check not in (...) , 'receipt-execution-tests'` | ADOPTED — without it the classifier is unreachable (fail-closed) |
| pr-check-runner.py | `RECEIPT_EXECUTION_SUITES` tuple (module list, never glob) | ADOPTED |
| pr-check-runner.py | `if check == 'receipt-execution-tests':` branch (module-list run, fail-closed per-suite) | ADOPTED |
| test_pr_check_runner.py | `import os`, `import subprocess`, `import sys` | ADOPTED — required by the new test |
| test_pr_check_runner.py | `test_receipt_execution_proves_actual_suite_run` (real run + empty-stub fail-closed) | ADOPTED |

Discarded: the prompt's literal `grep -nE "executable"` in `pr-gate-plan.py` (zero
matches — stale premise). No receipt_*.py imports are needed on develop at runtime;
the classifier only shells out via subprocess, no new product imports.

Note on ordering: upstream 970ffcd4 uses RECEIPT_EXECUTION_SUITES order
`writer,verifier,chain,tasklink,ci_workflow_policy` and classifier error
`'did not run'`; the later `96d87064` reorders to
`writer,verifier,chain,ci_workflow_policy,tasklink` with error
`'no receipt test executed'`. This lane uses the develop-`pr-gate-plan.py` order
(`...chain, ci_workflow_policy, tasklink`) to mirror `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS`;
the classifier's negative-test error message was aligned to that branch's wording.

## repo-rule compliance

- baseharbor/AGENTS.md: edited only trusted validation scripts under `scripts/`; ran
  focused Python unit tests before push; no Go/native gate required for this change.
- Scope (acceptance a): `git diff --stat origin/develop` = exactly
  `scripts/pr-check-runner.py` + `scripts/test_pr_check_runner.py`.
- Evidence kept in the evidence board (not on the product branch), matching upstream
  commit `970ffcd4` which changed only `scripts/pr-check-runner.py`.

## Verification

- (b) `PYTHONPATH=scripts python3 -m pytest -q scripts/test_pr_check_runner.py` →
  `19 passed, 44 subtests passed` (0 fail, 0 error).
- (c) `python3 scripts/pr-gate-plan.py --help` → rc=0.
- (a) working-tree diff vs develop = 2 files only.
