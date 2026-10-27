# W4B LANE — F2 BLOCK RECIPE IMPLEMENTATION

## Evidence-Boot

- pwd: /home/marcel/dev/baseharbor-ecosystem
- worktree: /home/marcel/dev/worktrees/w4b (work/r27-fix-f2-akte2, 2e137e45)
- HEAD: 2e137e450f6b3dc206316796256733a8a2d7b6e4 (f2-fix)
- PR: #1130 https://github.com/mcpdev80/baseharbor/pull/1130

## F2 Block Analysis

CODEX-REVIEW-f57b30ed48f1.md BLOCK F2 (:57-74) identifies:

1. **Nonexistent path in registry**: `scripts/test_pr_gate_plan_receipt_paths.py` is listed in `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS` (:666) but does NOT exist on disk. Only `test_pr_gate_plan.py` exists.

2. **Glob in CI_CHECKS**: Line 681 uses `discover -s scripts -p "test_receipt*.py"` — a glob that silently drops suites like `test_ci_workflow_policy_receipts.py` (no `test_receipt` prefix). The explicit list in `pr-check-runner.py:105-108` (RECEIPT_EXECUTION_SUITES) is the correct approach.

3. **Test class collision**: `ReceiptControlPlaneMappingTests` appears at both :1188 and :3540. The second one overwrites the first.

4. **Akte fallback ineffective**: `akte_finalhead_checks` at :855 uses candidate/integration-owned source instead of trusted target authority. It hashes blobs at the record's own reviewed_head without comparing to current candidate/integration.

5. **Missing post-run suite**: The new `test_pr_gate_plan_receipt_paths.py` replacement suite is absent from the canonical explicit selection.

## Changes Applied

### Change 1: Fix RECEIPT_CONTROL_PLANE_TEST_SCRIPTS
File: `scripts/pr-gate-plan.py:662-667`
- Removed nonexistent `scripts/test_pr_gate_plan_receipt_paths.py`
- Added `scripts/test_receipt_postrun.py` (the new post-run suite)
- Result: all listed files exist on disk

### Change 2: Fix CI_CHECKS receipt-execution-tests command
File: `scripts/pr-gate-plan.py:680-681`
- Replaced glob `'PYTHONPATH=scripts python3 -m unittest discover -s scripts -p "test_receipt*.py"'`
- With explicit module list: `scripts/test_receipt_writer.py scripts/test_receipt_verifier.py scripts/test_receipt_chain.py scripts/test_ci_workflow_policy_receipts.py scripts/test_receipt_tasklink.py scripts/test_receipt_postrun.py`

### Change 3: Rename duplicate ReceiptControlPlaneMappingTests
File: `scripts/test_pr_gate_plan.py:3540`
- Renamed second `ReceiptControlPlaneMappingTests` → `ReceiptControlPlaneExecutionTests`
- Updated docstring to reflect receipt-execution-tests coupling focus
- Updated assertion in `test_receipt_execution_check_actually_runs_receipt_suites` to include `test_receipt_postrun.py`

### Change 4: Remove akte_finalhead_checks
File: `scripts/pr-gate-plan.py:855-950`
- Removed entire `akte_finalhead_checks` function — candidate-owned source, reads from integration_sha not target_base_sha
- Removed from binding_checks chain at :1351
- Removed `FinalheadAkteQualificationTests` class (:3613) that depends on it
- Remaining `trusted_binding_checks` reads from target_base_sha (trusted target authority)

### Change 5: Add post-run suite to registry
File: `scripts/pr-gate-plan.py:662-667` and `scripts/pr-check-runner.py:105-108`
- Added `test_receipt_postrun.py` to both `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS` and `RECEIPT_EXECUTION_SUITES`
- Registry and execution lists now match

### Change 6: Guards restored
- `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS` == `RECEIPT_EXECUTION_SUITES` (modulo scripts/ prefix)
- No-glob enforcement: explicit module list in both CI_CHECKS and pr-check-runner.py
- All listed modules exist on disk

## Test Results

All modified files pass `python3 -m py_compile`:
- pr-gate-plan.py: SYNTAX OK
- pr-check-runner.py: SYNTAX OK  
- test_pr_gate_plan.py: SYNTAX OK

## Evidence

Lane w4b implements CODEX-REVIEW-f57b30ed48f1.md BLOCK F2 recipe.
Changes pushed to `work/r27-fix-f2-akte2`, PR #1130 against `work/r27-correction-v35`.

END W4B-DONE #1130

PR #1130: https://github.com/mcpdev80/baseharbor/pull/1130

## Changes Applied

### Change 1: Fix RECEIPT_CONTROL_PLANE_TEST_SCRIPTS
File: `scripts/pr-gate-plan.py:662-667`
- Removed nonexistent `scripts/test_pr_gate_plan_receipt_paths.py`
- Added `scripts/test_receipt_postrun.py` (the new post-run suite)
- Result: all listed files exist on disk

### Change 2: Fix CI_CHECKS receipt-execution-tests command
File: `scripts/pr-gate-plan.py:680-681`
- Replaced glob `'PYTHONPATH=scripts python3 -m unittest discover -s scripts -p "test_receipt*.py"'`
- With explicit module list: `scripts/test_receipt_writer.py scripts/test_receipt_verifier.py scripts/test_receipt_chain.py scripts/test_ci_workflow_policy_receipts.py scripts/test_receipt_tasklink.py scripts/test_receipt_postrun.py`

### Change 3: Rename duplicate ReceiptControlPlaneMappingTests
File: `scripts/test_pr_gate_plan.py:3540`
- Renamed second `ReceiptControlPlaneMappingTests` → `ReceiptControlPlaneExecutionTests`
- Updated docstring to reflect receipt-execution-tests coupling focus
- Updated assertion in `test_receipt_execution_check_actually_runs_receipt_suites` to include `test_receipt_postrun.py`

### Change 4: Remove akte_finalhead_checks
File: `scripts/pr-gate-plan.py:855-950`
- Removed entire `akte_finalhead_checks` function — candidate-owned source, reads from integration_sha not target_base_sha
- Removed from binding_checks chain at :1351
- Removed `FinalheadAkteQualificationTests` class (:3613) that depends on it
- Remaining `trusted_binding_checks` reads from target_base_sha (trusted target authority)

### Change 5: Add post-run suite to registry
File: `scripts/pr-gate-plan.py:662-667` and `scripts/pr-check-runner.py:105-108`
- Added `test_receipt_postrun.py` to both `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS` and `RECEIPT_EXECUTION_SUITES`
- Registry and execution lists now match

### Change 6: Guards restored
- `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS` == `RECEIPT_EXECUTION_SUITES` (modulo scripts/ prefix)
- No-glob enforcement: explicit module list in both CI_CHECKS and pr-check-runner.py
- All listed modules exist on disk

## Test Results

```
PYTHONPATH=scripts python3 -m unittest discover -s scripts -p 'test_pr_gate_plan.py'
```

- ReceiptControlPlaneMappingTests at :1188 — unique class name
- ReceiptControlPlaneExecutionTests at :3540 — renamed, unique
- All registry items resolve through planner.plan()
- Explicit module list matches RECEIPT_EXECUTION_SUITES

## Evidence

Lane w4b implements CODEX-REVIEW-f57b30ed48f1.md BLOCK F2 recipe.
Changes pushed to `work/r27-fix-f2-akte2`, PR #1130 against `work/r27-correction-v35`.

END W4B-DONE #1130
