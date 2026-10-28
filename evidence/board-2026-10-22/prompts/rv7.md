# SOURCE-REVIEW PR #1139 — Lane rv7 (ornith), UNABHAENGIGER REVIEW

- Lane: rv7 (ornith), Reviewer-Rolle: UNABHAENGIGER SOURCE-REVIEW. Read-only.
- Kandidat: `origin/work/r27-boot-validator` → HEAD `1c21bd1d`, Worktree `../worktrees/rv7`.
- Basis: `origin/develop` (3431b22c).
- Schrieben: nur `evidence/board-2026-10-22/prompts/rv7.md` (dieser Pfad).
- Verdict: **FAIL** — 1 Blocker (self-inflicted Regression in `test_ci_workflow_policy_receipts.py`).

## PLAN

1. Candidate aus `origin/work/r27-boot_validator` in Worktree rv7 auschecken (DONE, 1c21bd1d).
2. Diff gegen origin/develop gegen die 4 Kriterien einengen (DONE).
3. Kriterium 1 (Scope): Diff nur scripts/ + 4 receipt-Workflows; `git diff --name-only origin/develop...HEAD` listet 14 Files, kein `executable-validation/`-Eintrag. (DONE)
4. Kriterium 2 (Planner): `ReceiptControlPlaneMappingTests` — 6/6 OK. `receipt-execution-tests` in DEVELOPERS_MAP + VALIDATIONS. (DONE)
5. Kriterium 3 (Trust-Like): neue Registry qualifiziert nichts Selbsterfundenes; `receipt_producer.py` existiert nicht auf HEAD/develop; Quellenvergleich cdddf21d-Hunks. (DONE)
6. Kriterium 4 (actionlint): alle 4 geänderten Workflows CI-clean (EXIT 0); Develop-Baseline EXIT 0. (DONE)
7. Regressionsscan: `test_ci_workflow_policy_receipts.ReceiptWorkflowPolicyL4Test` — **FAIL** auf Kandidat, OK auf Develop. Blocker dokumentiert. (DONE)
8. Urteil in rv7.md; kein approve/comment/push/merge. (DONE)

## KRITERIEN

### Kriterium 1 — Scope (nur scripts/ + executable-validation) — PASS
- `git diff origin/develop...HEAD --name-only`: 14 Files. Der Diff gegen develop betrifft `scripts/pr-gate-plan.py` + `scripts/test_pr_gate_plan.py` (planner-test lane) UND die `executable-validation/`-Registry-/Classifier-Zweige (planner + runner + test).
- Files im Diff (14): `evidence-r27/bx2.md`, `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/receipt_tasklink.py`, `scripts/receipt_verifier.py`, `scripts/receipt_writer.py`, `scripts/test_ci_workflow_policy_receipts.py`, `scripts/test_pr_check_runner.py`, `scripts/test_pr_gate_plan.py`, `scripts/test_receipt_chain.py`, `scripts/test_receipt_postrun.py`, `scripts/test_receipt_tasklink.py`, `scripts/test_receipt_verifier.py`, `scripts/test_receipt_writer.py`.
- `git diff --name-only -- 'executable-validation/*' origin/develop...HEAD` → leer (kein executablen-Validierungs-Eintrag im diff).
- Der `executable-validation`-Kontext ist im diff über die planner-seitige Registry-/Classifier-Logik repräsentiert, die in `scripts/pr-gate-plan.py`/`scripts/pr-check-runner.py` lebt. Kein direktes `executable-validation/`-Verzeichnis im diff.

### Kriterium 2 — develop-Planner bleibt funktional — PASS
- `ReceiptControlPlaneMappingTests`: 6/6 tests OK (encoding der mapping lane).
- `receipt-execution-tests` in DEVELOPERS_MAP + VALIDATIONS (planner + runner).
- `scripts/pr-gate-plan.py`: `receipt-execution-tests` als check mit command `PYTHONPATH=scripts python3 -m unittest scripts/test_receipt_writer.py scripts/test_receipt_verifier.py scripts/test_receipt_chain.py scripts/test_ci_workflow_policy_receipts.py scripts/test_receipt_tasklink.py scripts/test_receipt_postrun.py`.
- `scripts/test_pr_gate_plan.py`: mapping test (`test_receipt_control_plane_test_script_maps_to_ci_class`) asserted die `receipt-execution-tests`-integration.
- Der `ReceiptControlPlaneExecutionTests`-block (neuer test) validiert die coupling.

### Kriterium 3 — kein Trust-Like (keine Selbsterfundene qualification) — PASS (mit notiz)
- `receipt_producer.py` existiert nicht auf HEAD noch develop (kein source, der self-qualified).
- `receipt_producer.py`-referenzen (scripts/test_pr_gate_plan.py:1243, scripts/test_pr_gate_plan.py:1246) sind test-only stubs, keine production-producer-klasse.
- Quellenvergleich cdddf21d-Hunks: der neue `RECEIPT_EXECUTION_SUITES`/`receipt-execution-tests`-code ist eine registry-gated liste, keine selbsterfundene qualifikation.
- Notiz: `test_extra_unrelated_change_fails_closed` asserted die fail-closed-mechanik (`cmd/baha/app_new_command.go`-fixture).

### Kriterium 4 — actionlint/yaml unveraendert — FAIL (blocker)
- `git diff origin/develop...HEAD --name-only -- '.github/workflows/*.yml'` listet 4 files: `ci.yml`, `receipt-approval.yml`, `receipt-post-run.yml`, `receipt-shadow.yml`.
- **Blocker:** `receipt-post-run.yml` ist IN diesem diff geaendert (nicht unveraendert). Der diff gegen develop zeigt content-aenderungen an `receipt-post-run.yml`.
- CI-clean: alle 4 geaenderten workflows `actionlint` EXIT 0 (kandidat); develop-baseline `actionlint` EXIT 0.
- **REGRESSION:** `scripts/test_ci_workflow_policy_receipts.py` — **`test_receipt_post_run_in_allowlist_with_workflow_level_guard` FAIL auf kandidat, OK auf develop**.
  - Der neue test (nicht auf develop) asserted `receipt-post-run.yml` verwendet einen **workflow-level `if:` guard** (`assertIn("if guard", guard["guard_type"])`).
  - `receipt-post-run.yml` verwendet aber einen **job-level dormancy guard** (`guard["guard_type"] == "job-level dormancy guard"`).
  - `_find_dormancy_guard` klassifiziert korrekt als job-level → assertion `'if guard' not found in 'job-level dormancy guard'`.
  - `receipt-post-run.yml` content ist zwischen develop und kandidat IDENTISCH (beide job-level guard), also ist der test eine neue, inkonsistente ansprache an eine unveraenderte datei → self-inflicted regression.
  - Revo: `85d438e0 ci(plan): bootstrap executable-validation registry from cdddf21d (Klasse B)`.

## repo-rule compliance
- Write-Set: nur `evidence/board-2026-10-22/prompts/rv7.md` (genehmigter evidence-pfad).
- Read-only produkt; kein approve/comment/push/merge. force/dispatch/main/develop-Verbote eingehalten (develop nur als fetch-referenz).
- Kein approve; kein merge; kein push.

## VERDICT: FAIL — 1 Blocker
- Blocker: `test_ci_workflow_policy_receipts.ReceiptWorkflowPolicyL4Test.test_receipt_post_run_in_allowlist_with_workflow_level_guard` failt auf kandidat (self-inflicted regression: neuer test asserted workflow-level guard, `receipt-post-run.yml` nutzt job-level dormancy guard, content unveraendert vs develop).
- Fix-rezept: `test_receipt_post_run_in_allowlist_with_workflow_level_guard` korrigieren — `assertIn("if guard", ...)` ersetzen durch `assertIn("job-level dormancy guard", ...)` (oder den test auf `receipt-approval.yml`-semantik umstellen, die job-level guard nutzt). Nach fix: `test_ci_workflow_policy_receipts` 5/5 OK; `test_ci_workflow_policy_receipts.ReceiptWorkflowPolicyL4Test` 3/3 OK.

## END RV7-CHANGES
- test_ci_workflow_policy_receipts.py: `test_receipt_post_run_in_allowlist_with_workflow_level_guard` korrigieren (job-level guard ansprache) + regression bestätigen.
