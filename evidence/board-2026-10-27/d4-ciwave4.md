# D4 – Host-CI-Analyse Welle 4 (PRs #1129, #1131, #1132, #1133)

LANE: d4 (flash) · Datum: 2026-10-27 · Modus: READ-ONLY (nur Dossier, product-git kein fetch nötig, keine Schreibaktion)

## PLAN

- [x] TOOL 1 Boot: `pwd` + `gh run list` auf `work/r27-correction-v35` → OK (2 Runs, beide failure)
- [x] PR #1129: Head + Jobs + Log-Kernaussagen (Run 40569750737, Vergleich Run 40568836691)
- [x] PR #1131/#1132/#1133: Head + Run-Status je Branch und je Commit geprüft
- [x] Muster extrahiert (Suite-Einheitlichkeit, Regression zwischen Runs)
- [x] Dossier geschrieben

## Matrix

| PR | Branch | Head-SHA | Run(s) | Fehlgeschlagene Jobs | Suite | Kernmeldung (gekürzt) |
|---|---|---|---|---|---|---|
| #1129 | work/r27-correction-v35 | `f57b30ed48f19e62452ab793801a02ccd2658414` | 40569750737 (pull_request, completed/failure) | PR · Focused Python | receipt-Suite (pr_gate-Focused) | `FAILED (failures=2, errors=3)` → Details unten (A)/(B)/(C) |
| #1129 | " | " | 40569750737 | PR · Validation · pr-planner-tests | identische receipt-Suite, doppelt ausgeführt | `Ran 129 tests ... FAILED (failures=2, errors=3)`; nested `Ran 138 tests ... FAILED (errors=1)` |
| #1129 | " | " | 40569750737 | PR Selected Checks (Aggregator) | Gate-/Coverage-Logik | `BLOCKED: required authenticated job not successful: PR · Focused Python`; `required validation not successful: pr-planner-tests (blocked)`; `coverage-unresolved: ... unknown impact for scripts/test_receipt_postrun.py` |
| #1129 | " | `839e5cc9fa9b3da3908ee65f26596c2b3dfaaca7` (Vorhead) | 40568836691 | PR · Focused Python | Runner-Dispatch, NICHT receipt-Suite | `ValueError: unsupported executable validation` (pr-check-runner.py:107, Aufruf aus `_temp/pr-check-runner.py`); Receipt-Tests当时 OK |
| #1129 | " | " | 40568836691 | PR Selected Checks | Gate-Aggregation | `coverage-unresolved: CI execution/evidence impact requires specific qualification` für `.github/workflows/ci.yml`, `evidence-r27/{ci1-repair,cy1,cy3,f1-fix,f3b-fix,s2-v3staging,w1,w3,x1-fix}.md`, `scripts/pr-check-runner.py`, `scripts/test_pr_check_runner.py` |
| #1131 | work/r27-fix-f2-akte2b (base v35) | `3dcc58c7849a4ca6c448debd9a8527f576739fb7` | KEINER | – | – | Draft-PR; `gh pr checks` = "no checks reported"; keine Runs pro Branch/Commit |
| #1132 | work/r27-fix-f6-outputs2 (base v35) | `418acbfd39005bbcade535088851fb31a8baa4c0` | KEINER | – | – | Draft-PR; wie oben |
| #1133 | work/r27-fix-f1b-recovery (base v35) | `39bed489ccc70283deec6ff0ca1618d369ff3469` | KEINER | – | – | Draft-PR; wie oben |

### Detail-Fehlstellen im aktuellen Head f57b30ed (identisch in beiden Test-Jobs)

(A) `test_receipt_verifier` – 3 ERRORs (`PrApprovalResolutionTest.test_already_approved_is_idempotent_and_never_edits_twice`, `PrApprovalResolutionTest.test_approve_pr_happy_path`, `PublishInterfaceTest.test_retry_after_publish_failure_creates_no_second_edit`):
Kette: `receipt_verifier.approve_pr` (scripts/receipt_verifier.py:505) → `publish_approval` (:489) → `receipt_tasklink.publish_evidence` (scripts/receipt_tasklink.py:453) → `_git(..., "push", "origin", "<sha>:refs/heads/main")` → `TaskLinkError: git push failed: fatal: 'origin' does not appear to be a git repository`.
→ `publish_evidence` führt `git push origin` gegen den Planning-Root aus, ohne dass der Test-Fixture ein `origin` hat (fehlender Fixture-Remote oder fehlende No-Remote-/Dry-Run-Schiene im Produktionspfad).

(B) `test_ci_workflow_policy_receipts` – 2 FAILs (`test_receipt_approval_in_allowlist_with_job_level_guard`, `test_receipt_post_run_in_allowlist_with_job_level_guard`):
`AssertionError: 'job-level' not found in 'workflow-level if guard' : receipt-approval.yml/receipt-post-run.yml uses workflow-level if guard (the allowed Dormancy-Form)`.
→ Test erwartet Job-Level-Guard in den Receipt-Workflows; die YAMLs nutzen Workflow-Level-if. Policy- vs. Test-Divergenz (Welches ist r27-Soll?).

(C) Wrapper: `scripts.test_pr_gate_plan.ReceiptControlPlaneMappingTests.test_receipt_execution_check_actually_runs_receipt_suites` → `RuntimeError: receipt suite execution failed: ...` (scripts/pr-check-runner.py:163) – reine Folge von (A)/(B) in der hochgeladenen Sub-Suite.

## Muster

1. **Eine einzige Suite schlägt fehl:** nicht pr_gate-Breitestest, nicht policy-Lint – ausschließlich die **Receipt-Control-Plane** (`test_receipt_verifier`, `test_ci_workflow_policy_receipts`, Receipt-Mapping im pr-planner). Alle übrigen Validation-Jobs (ci-actionlint, ci-executor-verifier-tests, ci-policy, documentation-audit, mkdocs, pages) sind GRÜN.
2. **Identisch über die beiden Testsuiten-Jobs** (Focused Python und pr-planner-tests führen dieselbe Receipt-Suite doppelt aus): 5 Fehlstellen, identische Tracebacks – ein Fix, zwei grüne Jobs.
3. **Regression innerhalb #1129:** Vorhead 839e5cc scheiterte technisch anders (`unsupported executable validation` im Runner-Dispatch; Receipt-Tests OK). Head f57b30ed hat den Runner-Dispatch repariert, aber die Receipt-Suite (A)+(B) neu rot gemacht → Änderung zwischen 839e5cc..f57b30ed hat `publish_evidence`-Push-Pfad und/oder Guard-Erwartungen eingeführt, ohne Tests/Fixtures nachzuziehen.
4. **Die drei Paket-PRs #1131/#1132/#1133 haben GAR KEINEN Host-CI-Lauf:** Draft + Base=v35 → Workflow nicht getriggert. Sie erben zwangsläufig das rote pr_gate von #1129, sobald sie ready sind.
5. **Der Aggregator `PR Selected Checks` ist sekundär:** er meldet nur BLOCKED (failed Jobs + `coverage-unresolved`-Qualifikationslücken für evidenz-/script-Pfade inkl. `unknown impact for scripts/test_receipt_postrun.py` im aktuellen Head). Er ist Symptom, nicht Ursache.
6. Kein PR-spezifischer Unterschied zu erwarten: #1131/#1132/#1133 sind Pakete AUF v35; ihre einzige eigene CI-Aussage wäre dieselbe v35-Basis-Rot.

## Empfehlung (R4-Checkliste)

1. **R4-Fix-Punkt 1 (Blocker):** `receipt_tasklink.publish_evidence` (scripts/receipt_tasklink.py:453) – `git push origin` nur, wenn ein konfiguriertes origin existiert bzw. Test-Fixtures in `test_receipt_verifier.PrApprovalResolutionTest`/`PublishInterfaceTest` einen Bare-Remote als origin einsetzen; Entscheidung dokumentieren, welche Seite (Code vs. Fixture) laut r27-Vertrag korrekt ist. Regression: die 3 ERRORs grün.
2. **R4-Fix-Punkt 2:** Guard-Form für `receipt-approval.yml`/`receipt-post-run.yml` vereinheitlichen: Job-Level-Guard in die YAMLs ODER Testerwartung auf Workflow-Level ("allowed Dormancy-Form") anpassen – maßgeblich ist die im r27-Vertrag/policy hinterlegte Form; die Divergenz (B) ist zu dokumentieren, nicht durch Testlöschung zu beheben.
3. **R4-Fix-Punkt 3:** Impact-Klassifikation für `scripts/test_receipt_postrun.py` ergänzen (`unknown impact` im Selected-Checks-Gate) und die `coverage-unresolved`-Qualifikationsverweise (evidence-r27/*.md, pr-check-runner-Pfade) vollständig machen, damit der Aggregator nicht separat rot bleibt.
4. **Paket-PRs:** #1131/#1132/#1133 frühestens nach grünem v35-Merge-Bestand ready schalten (Draft → Ready erzwingt Host-CI); vorher rebase auf den R4-korrigierten v35, dann EIN Lauf je PR statt Schein-Reviews gegen rote Basis.
5. **Kein doppeltes Review** des aktuellen f57b30ed-Kandidaten: identische Receipt-Fehlstellen sind bereits SHA-genau belegt (dieses Dossier).

Ende LANE d4.
