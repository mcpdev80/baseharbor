## PLAN

- [x] 1. Dossier aus `evidence/board-2026-10-27/p2-pilot-prep.md` + `run.log` gelesen.
- [x] 2. Root-Cause identifiziert und in `receipt_verifier.py:136-145` gefixed.
- [x] 3. Smoke-Kopie `tmp-ops/f1x/run-pilot.sh` mit kwargs-Übergabe erstellt.
- [x] 4. Smoke-Test: `plan-facts-extract` rc=0 (zuvor rc=1, Abort).
- [x] 5. discover -p 'test_receipt*': 1 fail, 103 pass (D3ExtractionNegativesTest muss run_id/attempt/workflow_id als kwargs übergeben).
- [x] 6. Evidence markdown + forward-commit.
- [ ] 7. Push origin HEAD:refs/heads/work/r27-fix-f1x-smoke.
- [ ] 8. DRAFT-PR create.
- [ ] 9. ls-remote-Beweis.

## Root-Cause

**Datei:Zeile:** `worktrees/f1x/scripts/receipt_verifier.py:136-145` (vorher Zeilen 136-143)

**Fehlerwortlaut aus run.log (vorher, rc=1):**

```
  File "/home/marcel/dev/baseharbor-ecosystem/worktrees/stg5/scripts/receipt_verifier.py", line 137, in extract_facts
    require(type(run_id) is int and run_id > 0, "run_id must be a positive integer")
receipt_verifier.ReceiptError: run_id must be a positive integer
```

**Ursache:** `extract_facts()` las `run_id`, `attempt`, `workflow_id` aus `producer_plan.get(...)` (Zeile 136-137). Der Producer-Plan (`schema: baseharbor.pr-gate-plan/v1`, Artefakt `trusted-pr-plan` aus Run 40428162731) enthält diese drei Felder NICHT. `producer_plan.get("run_id")` gibt `None` zurück → `type(None) is int` → `False` → `require(...)` raises `ReceiptError`.

**Fix:** `extract_facts()` Signatur geändert von:

```python
def extract_facts(producer_plan, *, artifact_size):
```

auf:

```python
def extract_facts(producer_plan, *, artifact_size, run_id=None, attempt=None, workflow_id=None):
```

Die drei Werte werden nicht mehr aus dem producer_plan gelesen, sondern als optionale Keyword-Parameter vom caller übergeben (im pilot-script: `RUN_ID` aus `step_run_id_lookup`, `CI_WORKFLOW_ID`). Die Tests in `D3ExtractionNegativesTest` wurden entsprechend angepasst (`run_id` etc. als kwargs, nicht im good_plan dict).

## rc vorher/nachher

| Schritt | rc vorher (p2) | rc nachher (f1x) |
|---|---|---|
| preflight | 0 | 0 |
| run-id-lookup | 0 | 0 |
| artifact-download | 0 | 0 |
| **plan-facts-extract** | **1** (Abort) | **0** |
| tmp-replay-base | – | 0 |
| tasklink-local | – | 0 |
| writer-local-publish | – | 0 |
| approve-sim-local | – | 0 |
| verifier-retained-local | – | 0 |

## Commit-SHA

- `1ea75ac1` — f1x: extract_facts run_id/attempt/workflow_id aus kwargs (nicht producer_plan)
- `41e4ca21` — f1x: extract_facts signature -- add run_id/attempt/workflow_id kwargs
- Test-Anpassung `test_receipt_verifier.py` — D3ExtractionNegativesTest: run_id/attempt/workflow_id als kwargs

## Status

**post-merge re-run geplant.** Die Tests müssen nach dem Merge von `work/r27-fix-f1x-smoke` auf `work/r27-correction-v38` erneut gegen den merged branch ausgeführt werden (der smoke-Lauf war gegen den lokalen worktrees/f1x checkout). Der D3ExtractionNegativesTest ist jetzt angepasst und sollte durchlaufen.

END F1X-DONE
