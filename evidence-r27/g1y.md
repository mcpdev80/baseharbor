# g1y — F1 post-run ID/revision/extraction correction (WIP checkpoint)

- **Lane**: g1y (ornith), taken over from g1x (rotated, 40 min without commit)
- **Task**: `t-20261022-r27-evidence-publisher` — "Required correction" section (Owner 2026-10-27)
- **Branch**: `work/r27-fix-f1-postrun` (remote HEAD `211c687a` = local HEAD)
- **Role**: Worker — implement + local proof only. No Codex closing-abnahme (per AGENTS.md worker role).
- **Scope**: `.github/workflows/receipt-post-run.yml`, `scripts/receipt_postrun*.py`, `scripts/test_receipt_postrun.py`. Exclusive writes only. NO `tasklink.py`/`verifier.py` (W1b/g2x ownership).

## PLAN (abhakbar je D1/D2/D3)

- [x] **D1** — `run.id` vs `workflow_id` getrennt: `RUN_ID=${ needs.extract_plan.outputs.run_id }}` (authentificierter `workflow_run.id`), `workflow_id` separat aus `workflow_run.workflow_id`, gebunden an producer-run `workflow_id` (Gleichheits-Check, fail-closed), attempt!=1 explizit rejected.
- [x] **D2** — `plan_revision` = Tasklink-Rückgabe (Commit-SHA der Plan-Fixture), NICHT `candidate_sha`. tasklink step hat `id: plan_commit`, exportiert `plan_revision` via `GITHUB_OUTPUT` als letzte 40-hex Linie; publish step `PLAN_REVISION=${ steps.plan_commit.outputs.plan_revision }}`.
- [x] **D3** — keine `:-unknown`/literal `unknown`/fallbacks; Validierung von head/base/integration als 40-hex + run/workflow/attempt als positive int VOR `GITHUB_OUTPUT`-Export; `source`/`eval` auf artifact-daten entfernt; heredoc-Redirect auf Python-Invocation (`> plan.txt`), kein source/eval.
- [x] **restliche F1-Punkte** — `integration_sha` zwingend, attempt==1 Pflicht, 40-hex/positive-int Validierung, kein `unknown`-Fallback.

## Beweis-log

### Korrekturen am Workflow (.github/workflows/receipt-post-run.yml)

- g1x WIP hatte einen **base64-codierten `eval "$decoded"`**-Block (Injection-Vektor, genau das verbotene `source/eval on artifact-daten`) + `ref: 839e5cc9...` auf `develop` (nicht "immutable reviewed CI", sondern mutierbarer develop-SHA — W1b/g2x-Besitz, hier nur dokumentiert).
- Ersetzt durch sauberen, reviewbaren heredoc: Python-Code bei 10-space YAML-Indent (YAML chompt common indentation → bash empfängt body bei column 0, gültiger Python-Cod), öffnender Redirect `> plan.txt` auf der `python3 - <<'PY'`-Zeile, **abgeschlossener Delimiter `PY` bei column 0** (bash akzeptiert nur bare `PY`, nicht `PY > plan.txt` — getestet).
- Validierung in der Sequenz: `run_id` positive int, `attempt==1`, `head_sha`/`integration_sha`/`target_base_sha` je `^`[0-9a-f]{40}$`, `workflow_id` positive int; Bindung `artifact workflow_id == authenticated workflow_definition_id` (fail-closed bei Differenz).
- D2 tasklink step (`id: plan_commit`): `RESULT=$(python3 scripts/receipt_tasklink.py ...)`, `PLAN_COMMIT=$(printf '%s\n' "$RESULT" | grep -E '^[0-9a-f]{40}$' | tail -n 1)`, leer → exit 1, export `plan_revision=${PLAN_COMMIT}`.
- publish step: `RUN_ID=${ needs.extract_plan.outputs.run_id }}`, `PLAN_REVISION=${ steps.plan_commit.outputs.plan_revision }}`, `--plan "$PLAN_REVISION" --run "$RUN_ID" --attempt 1 --publish`.

### Lokale Prüfung (bare plan-repo fixture)

- `scripts/test_receipt_postrun.py` parst die YAML-run-blocks via `yaml.safe_load` (exakt wie der Runner die block-scalar indentation chompt → heredoc-body bei column 0), substituiert GH-Expressionen und führt die Blcke gegen eine temporäre bare Planning-Remote + fake producer artifact aus.
- Positive Run: `run.id` = LIVE-run-id (40541797478), `workflow_id` = definition-id (17) — nachweislich ungleich; receipt `plan`-field zeigt den echten Planning-Plan-Commit; plan behält `workflow_id`=17 + tested `integration_sha`.
- Negativ: missing artifact → decode block non-zero, keine GITHUB_OUTPUT-exports; foreign producer repo → writer fail-closed.

### Test-Ergebnis (2026-10-27, worktree g1-f1-fixture, SHA 211c687a)

```
$ python3 scripts/test_receipt_postrun.py
test_all_run_blocks_exist ... ok
test_decoder_block_is_bash_clean ... ok
test_writer_fails_closed_on_foreign_context ... ok
test_decode_fails_without_artifact ... ok
test_full_run_publishes_plan_and_receipt ... ok
Ran 5 tests in 0.373s
OK
```

- YAML-Parse: OK. `bash -n` auf allen run-blocks: OK. Kein `eval`/`source` (nur in Kommentaren), kein `:-unknown`.

## repo-rule compliance

- Exclusive Write-Set eingehalten: nur `receipt-post-run.yml` + `scripts/test_receipt_postrun.py`. `receipt_tasklink.py`/`receipt_verifier.py`/`receipt_writer.py` UNVERÄNDERT (W1b/g2x-Besitz).
- Pinned reviewed Quelle `839e5cc9` gelesen; Defekte darin bestätigt (D1/D2/D3 bestehen dort weiter), Korrektur als eigenständige W2-Korrektur implementiert.
- Kein Root-Git-/Force-/Push auf main; nur `origin HEAD:refs/heads/work/r27-fix-f1-postrun` (neue Branch, nicht remote-vorhanden, ls-remote geprüft).

## Zwischenstand / nächster Stop

- WIP-Commits + push erledigt (211c687a). D1/D2/D3 implementiert + lokal per 5 Tests beweist.
- NOCH NICHT: gesamter F1-Kontext (andere F1-F8-Punkte = nicht dieser Scope), external review, Codex-Schlussabnahme, DRAFT-PR merged, CI.
- Ende: END G1Y-<pr|blocked>
