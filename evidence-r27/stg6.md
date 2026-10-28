## PLAN

- [x] 1. Remote-Zustand abgefragt (ls-remote `work/r27-correction-v38` = `cb7d8213`).
- [x] 2. Worktree `../worktrees/stg6` auf `work/r27-correction-v38` bereit (stg5 stale worktree freigegeben).
- [x] 3. `git merge --no-ff origin/work/r27-fix-pushcreds-pin` → clean (ort), 1 Datei.
- [x] 4. `git merge --no-ff origin/work/r27-fix-f1x-smoke` → clean (ort), 2 Dateien. Keine Konflikte (beide Intentionen übernommen).
- [x] 5. `discover -p 'test_receipt* test_pr_gate*' + 'test_ci_workflow_policy*'`: 320 passed, 480 subtests passed, 0 failed (BEFORE FIX: 4 failed).
- [x] 6. 2 Defekte gefixt und verifiziert (siehe ## repo-rule compliance / Defekte).
- [x] 7. YAML-Lint ok (`python3 -c yaml.safe_load`).
- [x] 8. `git commit` auf `work/r27-correction-v38` → `94f35d02`.
- [x] 9. `push origin HEAD:refs/heads/work/r27-correction-v38` → `cb7d8213..94f35d02`.
- [x] 10. ls-remote AFTER = `94f35d02...`.
- [ ] 11. PR #1138 ist Draft; Coordinator schließt mergebare Drafts (Lane stg6 schließt keine PR).

## repo-rule compliance

- Owner-Regel (Integration = Commits auf dem Kandidaten, keine weiteren PRs): eingehalten. Kein neuer PR, Commit direkt auf `work/r27-correction-v38`.
- force/main/develop/dispatch: verboten, nicht verwendet.
- Keine fremden Arbeitsbäume überschrieben; nur lokaler stg6-worktree + ein Commit auf dem Kandidaten.

## Defekte + Fixes (BEFORE/after Fix, Datei:Zeile)

**Defekt 1 — `scripts/receipt_verifier.py` (f1x):**
- BEFORE: `extract_facts()` referenzierte undefiniertes `kwargs` in `require(run_id ...)` an Zeilen ~144/151 (f1x Commits `1ea75ac1`/`41e4ca21` brachten die Signatur-Änderung, aber KEINEN Binding-Austausch). Ergebnis: `ReceiptError: run_id must be a positive integer`.
- Betroffene Tests vorher: `D3ExtractionNegativesTest::test_extraction_accepts_well_typed_object`, `F1PositiveRun::test_full_run_publishes_plan_and_receipt`, `ExtractBlocksTest::test_all_run_blocks_exist`, `ReceiptControlPlaneExecutionTests::test_receipt_execution_check_actually_runs_receipt_suites`.
- FIX: `run_id = producer_plan.get("run_id")` / `attempt = ...get("attempt")` / `workflow_id = ...get("workflow_id")` wieder eingeführt (contract: Tests übergeben diese als Dict-Keys, nicht als kwargs). Duplikat-Block (2× identical require) entfernt.

**Defekt 2 — `.github/workflows/receipt-post-run.yml` (pcx FIX A):**
- BEFORE (pcx Commit `e4ec565a`, "FIX A only"): der `Checkout immutable controller (reviewed CI)`-Checkout stand auf `ref: main` — der Review-Pin `839e5cc9fa9b3da3908ee65f26596c2b3dfaaca7` war entfernt. Sicherheitsregression (mutable Controller-Ref statt reviewed SHA).
- pcx war scope-überschreitend: FIX A = nur `persist-credentials` des Planning-Checkouts; der Pin gehört NICHT dazu.
- FIX: `ref: 839e5cc9...` auf dem Controller-Checkout wiederhergestellt, 2-Zeilen-Kommentar zur Review-Pflege ergänzt. Testing: `test_all_run_blocks_exist` verlangt den Pin (`self.assertIn("839e5cc9...", text)`).

## discover-Zählungen (stg6, nach Fix, auf `94f35d02`)

- `test_receipt_writer.py` + `test_receipt_tasklink.py` + `test_receipt_chain.py` + `test_receipt_postrun.py` + `test_receipt_verifier.py` + `test_pr_gate_dependencies.py`: 175 passed, 11 subtests.
- `test_pr_gate_plan.py`: full suite green (inkl. `ReceiptControlPlaneExecutionTests`).
- `test_ci_workflow_policy_receipts.py`: 5 passed.
- Kombiniert: **320 passed, 480 subtests passed, 0 failed**.

## push-Beweis

- ls-remote BEFORE: `cb7d821302b239bb04172527b2716f361afd79e5`
- Commit: `94f35d02`
- push: `cb7d8213..94f35d02 HEAD -> work/r27-correction-v38`
- ls-remote AFTER: `94f35d0293f8e237895afe84ae7aa9b26722ffed`

END STG6-DONE 94f35d02
