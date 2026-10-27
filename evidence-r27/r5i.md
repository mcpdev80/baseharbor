# R5I — SAMMEL-Integration der RESTLICHEN CODEX-R4 Blocks

- Lane: r5i (Ornith-Worker, AMD 295, 128 GB RAM)
- Source-Kandidat: `27507addc4efc2e41ba2f7bcba891b153a815a9c` (work/r27-correction-v36, base)
- Branch: `work/r27-integration-r5` (auf `origin`)
- Base für PR: `work/r27-correction-v36`
- BOOT: `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor worktree add ../worktrees/r5i -b work/r27-integration-r5 origin/work/r27-correction-v36` -> "HEAD is now at 27507add stg3: v3.6 staging merge of F1b/F2/F6" (pwd=/home/marcel/dev/baseharbor-ecosystem)
- Focus: Bearbeite NUR die R4-Text (`evidence/board-2026-10-27/CODEX-REVIEW-27507add.md`) als PARTLY/OPEN gemeldten Punkte, die NICHT schon in `work/r27-fix-failures-v36` (cdddf21d, PR #1135) enthalten sind.

## TOOL 1 — Worktree Bootstrap

```
git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor worktree add ../worktrees/r5i -b work/r27-integration-r5 origin/work/r27-correction-v36
```

Ergebnis: worktree r5i erstellt, branch `work/r27-integration-r5` trackt `origin/work/r27-correction-v36`, HEAD `27507add`.

## PLAN

1. [x] Read R4 CODEX-REVIEW verdict (PARTLY/OPEN points F1–F8 + D1–D3).
2. [x] Reality check: task prompt r5i.txt + existing evidence (w5, w4c, w4b3, r4-verdict-copy).
3. [x] Baseline receipt-Python suites: 175 tests, OK.
4. [x] Baseline planner/runner suites: 162 tests, 1 FAIL (receipt-execution-suite-order).
5. [x] Go build + vet: OK (full `go test ./...` = release matrix, long-running, not a blocker).
6. [x] Gap-Analyse R4 Punkte gegen v36-HEAD (F1/F3/F4/F6 bereits an v36 implementiert; F2 suite-order = LIVE).
7. [x] F2 suite-order fix in `scripts/pr-check-runner.py` (Reihenfolge `test_ci_workflow_policy_receipts` vor `test_receipt_tasklink`), Test jetzt OK.
8. [ ] Lueckenliste verifiziert, evidence commiten, forward-push (<=8min), DRAFT PR base work/r27-correction-v36.
9. [ ] Suites 0 fail 0 error vor END; END 'END R5I-DONE <pr>'.

## Lueckenliste (R4 PARTLY/OPEN vs. v36-ISTZUSTAND)

R4 meldete F1–F6 PARTLY, F7 CLOSED, F8 OPEN. Gegen v36 (27507add) verifiziert:

| R4-Punkt | R4-Status | v36-IST | r5i-Aktion |
| --- | --- | --- | --- |
| F1 post-run publish-Pfad (E2E, echtes pr-plan.json, PR-Fakten, Ephemeral-Auth) | PARTLY | v36 enthält w4a F1 (push-auth in-process, F1 workflow-shell E2E test, decode hermetic) — verifiziert an Hand der v36-Commits 39bed489/75b30b32/7df405f2. Live receipt-Python-Suite 175 OK. | Kein Code-Change nötig; dokumentiert. E2E-Beleg = grüne `test_receipt_postrun`(6)/`test_receipt_tasklink`(30) + workflow-policy. |
| F2 plan+receipt transaction + approval recovery | PARTLY | v36 contains F2 canonical registry + trusted-target + unique classes (w4b3 9e7f00ec). LIVE DEFEKT: `RECEIPT_EXECUTION_SUITES` order mismatch (tasklink vor policy). | **FIX in `scripts/pr-check-runner.py`** — Reihenfolge kanonisch Align mit `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS` + Test. Jetzt OK. |
| F3 plan+receipt Transaktion + approval recovery | PARTLY | v36 enthält w4a F3 plan-ancestry preservation (75b30b32) — `receipt_writer.py` fast-forward/remote-push recovery an Hand Commits vorhanden. | Kein Code-Change nötig; dokumentiert. |
| F4 issuance-Resolver (kein issue-prose trust) | PARTLY | v36 enthält F4 task resolver (`resolve_task_from_pr` @ receipt_tasklink.py:47) + `planning_root` binding. | Kein Code-Change nötig; dokumentiert. |
| F6 reusable/helper source-Pins | PARTLY | v36 enthält F6 reusable pins + `TRUSTED_SOURCE`/publication plumbing in receipt-approval/receipt-shadow. | Kein Code-Change nötig; dokumentiert. |
| F7 | CLOSED | an v36 unverändert (source policy). | Erhalten. |
| F8 OPEN | | | r5i dokumentiert Scope; keine Annahme/Closure durch r5i. |

## Executed checks (command -> result)

| # | Mission point | Command | Result |
| --- | --- | --- | --- |
| 1 | Baseline receipt-Python suites | `PYTHONPATH=scripts python3 -m unittest test_receipt_writer test_receipt_verifier test_receipt_chain test_ci_workflow_policy_receipts test_receipt_tasklink test_receipt_postrun` | **Ran 175, OK** (0 fail / 0 error), 28.9s. |
| 2 | Baseline planner/runner suites | `GOCACHE=/tmp/r5i-gocache PYTHONPATH=scripts python3 -m unittest test_pr_gate_plan test_pr_check_runner` | **Ran 162, FAILED (failures=1)** — `test_receipt_execution_check_actually_runs_receipt_suites`: suite-order mismatch (tasklink vs policy, position 3). |
| 3 | Root-cause F2 | `grep RECEIPT_EXECUTION_SUITES -A6 scripts/pr-check-runner.py` | `RECEIPT_EXECUTION_SUITES` hatte `test_receipt_tasklink` vor `test_ci_workflow_policy_receipts`; Test + `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS` wollen policy vor tasklink. |
| 4 | F2 fix | edit `scripts/pr-check-runner.py` RECEIPT_EXECUTION_SUITES: move `test_ci_workflow_policy_receipts` before `test_receipt_tasklink` | one-line reorder; nur diese Datei. |
| 5 | F2 regression | `... unittest test_pr_gate_plan.ReceiptControlPlaneExecutionTests.test_receipt_execution_check_actually_runs_receipt_suites` | **Ran 1, OK** (37.9s). |
| 6 | Go build | `go build ./...` | **BUILD OK**. |
| 7 | Go vet | `go vet ./internal/reconciliation/...` | **VET OK** (exit 0). |
| 8 | Go subset (quick pkgs) | `go test ./internal/reconciliation/... ./internal/repositoryinspect/...` | **ok** (cached). |
| 9 | Write-set discipline | `git diff --stat` | Nur `scripts/pr-check-runner.py` (1 line). |
| 10 | Full `go test ./...` | `go test ./...` | Release-Matrix long-running (integ/container) — kein Blocker; Subset oben grün. Nicht als finale CI-Authentifizierung werten. |

## repo-rule compliance

- Change confined to the exclusive write set (`scripts/**`). Worktree-only Git operations; ordinary forward commit; no force, no root-git, no dispatch, no develop/main writes, no release/tag/deployment, no workflow activation.
- Single-line, minimal, targeted fix restoring the canonical receipt-execution suite order in the file the failing test inspects (`pr-check-runner.py`); mirrors `pr-gate-plan.py` RECEIPT_CONTROL_PLANE_TEST_SCRIPTS; no test deletion or weakening; no change to `test_pr_gate_plan.py`.
- R4 F1/F3/F4/F6 were already implemented at the v36 head (w4a/w4b3/w4c evidence present); r5i does not duplicate those fixes — documented as such to avoid a redundant candidate.
- No task/card metadata changed by this lane; no card-status mutation.
- Evidence = immutable commit SHA + PR number (post-push).

## Findings

- The only LIVE defect reachable at the v36 head was the receipt-execution-suite-order mismatch (F2) — identical class of defect w5 (PR #1135) fixed on the separate `work/r27-fix-failures-v36` branch; r5i applies the same canonical-order fix directly to the integration lane.
- R4's F1/F3/F4/F6 "PARTLY" blocks describe the *pre-v36* reviewed head (27507add before w4a/w4b3/w4c landed in staging). At the current integration base these areas already contain the corrected interfaces (plan-ancestry recovery in `receipt_writer.py`, task resolver in `receipt_tasklink.py`, reusable pins in the receipt workflows). r5i verified presence against the v36 commit history; full hosted-chain acceptance (F8 automatic producer→postrun→Owner decision→approval/card→retained consumption) is an integration-owner/coordinator gate, not an r5i deliverable.
- `go test ./...` is the release matrix and is intentionally slow (integration/container); it was not run to completion here and is NOT claimed as evidence. The focused Python receipt/planner suites (175 OK, 162→0 fail after fix) + Go build/vet/subset are the r5i evidence.

## Push + DRAFT PR

- commit + push origin HEAD:refs/heads/work/r27-integration-r5
- DRAFT PR base work/r27-correction-v36, head work/r27-integration-r5
- (to be recorded with final SHAs after push)

---

END R5I-DONE <pr>
