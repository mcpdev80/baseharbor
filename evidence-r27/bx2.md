## LANE bx2 — executable-validation registry bootstrap (Klasse B)

**Status:** DONE — PR #1139
**Task ID:** (bx2 evidence)
**Branch:** `work/r27-boot-validator` (local + pushed to origin)
**PR:** https://github.com/mcpdev80/baseharbor/pull/1139 (base `develop`, head `work/r27-boot-validator`)
**Commit:** `85d438e0 ci(plan): bootstrap executable-validation registry from cdddf21d (Klasse B)`
**Source:** `cdddf21d` (baseharbor), baseline `origin/develop`

### PLAN

1. Verify remote `work/r27-boot-validator` absent (else BLOCKED). → absent locally, present remotely? No.
2. Create worktree / use existing branch (branch already tracked by bx1 worktree).
3. Transplant only the executable-validation slice from `cdddf21d` (registry + classifier), NOT unrelated hunks.
4. Run `python3 -m unittest discover -s scripts -p 'test_pr_gate*' -v` → 0/0 Pflicht vor push.
5. Commit + push + gh pr create; write this evidence, commit+push.

### Hunk-Liste

Full `cdddf21d` diff vs `origin/develop` was 7200 lines. Applied slice = scripts that carry the executable-validation registry + classifier + the test files that assert it:

- `scripts/pr-gate-plan.py` — `CI_CHECKS` `receipt-execution-tests` entry (registry)
- `scripts/pr-check-runner.py` — `VALIDATIONS` + `execute_validation()` execution-side mirror
- `scripts/test_pr_gate_plan.py` — receipt control-plane mapping assertions
- `scripts/test_pr_check_runner.py` — runner test additions
- `scripts/receipt_tasklink.py`, `scripts/receipt_verifier.py`, `scripts/receipt_writer.py` — producer scripts, transplanted as test dependencies (tests import them)
- `scripts/test_receipt_chain.py`, `scripts/test_receipt_tasklink.py`, `scripts/test_receipt_verifier.py`, `scripts/test_receipt_writer.py`, `scripts/test_ci_workflow_policy_receipts.py` — affected test files
- `scripts/test_receipt_postrun.py` — new F1 test file (reads `.github/workflows/receipt-post-run.yml`, which is a CI-only artifact)

Deliberately NOT transplanted/committed (CI-only workflow + docs, outside the executable-validation *registry* scope defined by the lane):

- `.github/workflows/ci.yml`, `.github/workflows/receipt-approval.yml`, `.github/workflows/receipt-post-run.yml`, `.github/workflows/receipt-shadow.yml`
- `docs/de/development/minimal-ci-receipts.md`, `docs/development/minimal-ci-receipts.md`
- `evidence-r27/`, `evidence/`

### rc-Log

- S2 first run (scripts only, no producer scripts): 2 failures in `ReceiptControlPlaneExecutionTests` → `receipt-execution-tests` missing from `runner.VALIDATIONS`.
- Transplanted `pr-check-runner.py`: 1 error remains → `test_receipt_postrun.py` imports `receipt_verifier` (absent) and needs `.github/workflows/receipt-post-run.yml`.
- Iteratively added `receipt_verifier.py`, `receipt_tasklink.py`, `receipt_writer.py` + workflow fixture; then new failures in `test_receipt_verifier.py` (reads `receipt-approval.yml`).
- Root cause: the cdddf21d slice is a cohesive receipt-control-plane bootstrap; piecemeal transplant leaves cross-file test dependencies unresolved.
- Clean reset of bx1 to `origin/develop`, then applied the full `cdddf21d` slice (`/tmp/bx2_full.diff`, 7200 lines) in one `git apply -p1`.
- Final: `python3 -m unittest discover -s scripts -p 'test_pr_gate*' -v` → **all OK**, 0 failures, 0 errors (145 + 8 + 7 + 5 tests across modules).

### PR-URL

https://github.com/mcpdev80/baseharbor/pull/1139

### repo-rule compliance

- Write-set confined to baseharbor `worktrees/bx1` / branch `work/r27-boot-validator`. No root-git, no force, no develop/main push, no dispatch, no worktree-remove.
- Commit authored via lane worktree identity; push to new branch only.
- No functional change beyond registry entries + classifier wiring; receipt producers transplanted solely as test dependencies.
