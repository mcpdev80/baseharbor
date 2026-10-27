# rv3 — unabhängige Vorprüfung v3.5-Kandidat (ornith)

- Lane: rv3 (ornith), Read-Only-Review des v3.5-Kandidaten
- Kandidat: `f57b30ed48f19e62452ab793801a02ccd2658414` (PR #1129, branch `work/r27-correction-v35`, base develop)
- Ziel: Vorprüfung der drei Owner-Korrekturen D1/D2/D3 aus `t-20261022-r27-evidence-publisher.md`
- Schreibpfade: `evidence-r27/rv3.md` + `tmp-ops/rv3-scratch`

## PLAN

1. Worktree `worktrees/rv3` gegen `f57b30ed` (--detach) angelegt.
2. Task-Vertrag gelesen (`baseharbor-plan/tasks/t-20261022-r27-evidence-publisher.md`).
3. Diff-Matrix gegen develop `3431b22c` + v34 `839e5cc9` aufgebaut.
4. Core-Dateien geprüft: `pr-gate-plan.py`, `receipt_tasklink.py`, `receipt_writer.py`, `receipt_verifier.py`, `.github/workflows/receipt-post-run.yml`.
5. Fokussierte Tests: `test_receipt*`, `test_pr_gate*`, `test_receipt_chain`, `test_receipt_postrun`, yaml/bash-Syntax.
6. End-to-End-Proof in `tmp-ops/rv3-scratch` (D1/D2/D3-Bindungen, fail-closed).

## Diff-Matrix (candidate `f57b30ed` vs develop `3431b22c`)

- 16 Dateien touched (scripts/ + .github/workflows/). Core-Korrekturnatur:
  - `receipt_tasklink.py` (M) — PR->task Resolver, `produce_plan_fixture` (D2 plan_revision)
  - `receipt_writer.py` (M) — `write()` `plan_revision`/`run_id`/`attempt`, `main()` `--plan`/`--run` (D1)
  - `receipt_verifier.py` (M) — `load_plan` 7-Felder, `live()`, `task_binding` (F4/F5)
  - `.github/workflows/receipt-post-run.yml` (M) — D1 (run_id/workflow_id, attempt==1), D2 (plan_revision), D3 (Extraktions-Validierung)
  - `pr-gate-plan.py` — D2-Bezug ci-binding-qualification (`akte_finalhead_checks`), nicht Kern-D1/D3
- Gegen `839e5cc9` (v34): D1/D2/D3-Defekte sind im v3.5-Kandidat behoben (Quelle+Test-Ebene, siehe unten).

## D1 — run identity vs workflow-definition identity (receipt_writer.py + workflow)

- `receipt_writer.py` `write(plan_revision, run_id, attempt)`: `run_id` landet als `run.id`, `attempt` als `run.attempt`; attempt==1 via `live()` (receipt_verifier.py:241) geprüft; candidate_sha aus `plan["candidate_sha"]` (receipt_writer.py:243-253).
- Workflow `receipt-post-run.yml`:
  - `RUN_ID` = `${{ github.event.workflow_run.id }}` (authentifizierter Run), `WORKFLOW_DEFINITION_ID` = `${{ github.event.workflow_run.workflow_id }}` (Definition) — getrennt, beide geprüft (Zeilen 89, 94, 119).
  - attempt-1-Pilot: `if [ "$ATTEMPT" != "1" ]; then exit 1` (Zeile 98-101), Rerun wird ausdrucklich abgewiesen, nicht auf attempt 1 umgelabelt.
  - `--run "$RUN_ID" --attempt 1 --plan "$PLAN_REVISION"` (Zeilen 300-305); writer's own GITHUB_*-Kontext wird nicht überschrieben.
- VERDICT: PASS. Quelle + Bindung korrekt.

## D2 — plan_revision = tatsächlicher Planning-Plan-Commit (receipt_tasklink.py + workflow)

- `produce_plan_fixture` gibt den Commit-SHA des neuen plan-fixture-Commits zurück (receipt_tasklink.py:258); CLI gibt das Ergebnis als letzte Zeile aus (Dokumentation + E2E-Proof).
- Workflow: tasklink-Step `id: plan_commit` extrahiert `PLAN_COMMIT` als letzte `^[0-9a-f]{40}$`-Zeile aus RESULT (Zeilen 260-273), schreibt `echo "plan_revision=${PLAN_COMMIT}" >> "$GITHUB_OUTPUT"`.
- Publish-Step: `PLAN_REVISION: ${{ steps.plan_commit.outputs.plan_revision }}`, `--plan "$PLAN_REVISION"` (Zeilen 286, 302). candidate_sha bleibt separat (Zeile 204).
- E2E-Proof: `plan_revision != candidate_sha`, plan.workflow_id int (getrennt), candidate_sha/head_sha/base_sha korrekt (tmp-ops/rv3-scratch/e2e_proof.py).
- VERDICT: PASS. plan_revision identifiziert einen Planning-Commit, nie candidate_sha/run_id.

## D3 — ungültige Extraktion an der Quelle verwerfen (receipt-post-run.yml)

- Heredoc ausführbar: `RUN_ID="${RUN_ID}" python3 - <<'PY' > plan.txt` (Umleitung auf Python-Invocation, nicht auf Delimiter) — Zeile 125.
- Validierung: `is40hex` (40 lowercase hex), `is_pos_int` (positive int), `integration_sha` erforderlich (`bool(isha)` + is40hex), run_id artifact==authenticated, attempt==1.
- `sys.exit(...)` bei fehlenden/ungültigen Werten → kein Export, kein writer/publish weiter. Kein `${value:-unknown}`, kein literal `unknown`, kein success/warning-nach-missing.
- `set -euo pipefail` aktiv; `GITHUB_OUTPUT`-Write weckt keine Shell-Variable.
- E2E-Proof: malformed `base_sha` → tasklink exit=2 (fail-closed), kein Writer/Aufruf.
- VERDICT: PASS. Fail-closed belegt.

## Prüfungen (Forts.)

- bash -n: alle drei Workflow-Step-Blöcke syntaktisch ok; heredoc `PY` schließt Zeile 171 korrekt.
- D3 fail-closed belegt: malformed base_sha → tasklink exit=2; fehlender plan.zip → TaskLinkError (kein Writer/Publish-Aufruf).
- Full receipt suite: 114/117 ok. 3 ERRORS (`test_approve_pr_happy_path`, `test_already_approved_is_idempotent_and_never_edits_twice`, `test_retry_after_publish_failure_creates_no_second_edit`) = Umgebungsfehler: `publish_evidence()` ruft `_git ... push origin` auf, der einen lokalen `origin`-Remote im planning-Worktree erwartet — im abgetrennten detached-Worktree fehlt dieser Remote. Reproduziert identisch auf develop `3431b22c` (kein Candidate-Regression).
- `test_ci_workflow_policy_receipts.py`: 2 FAILs (`receipt-approval.yml`, `receipt-post-run.yml`). Grund: Parser `_find_dormancy_guard` klassifiziert nur `if:`-Werte mit den Schlüsselwörtern never/-dormant/''/workflow_dispatch als "job-level"; die v3.5-Korrekturen ersetzen den develop-"dormant"-Guard durch einen reinen `conclusion == 'success'`-Guard, den der Parser fälschlich als "workflow-level if guard" meldet. Beide Wände sind faktisch job-level (indented unter `jobs:`). Reproduziert auf develop (identisch) → Parser-Limitation, kein sicherheitskritisches Regress; GitHub-wirklich job-level bestätigt (line 25/72/194 unter `jobs:`).

## Prüfungen (Forts. 2)

- yaml.safe_load: alle drei receipt-*.yml parse-ok.
- bash -n: jeder `run:`-Block der drei Jobs (verify_producer, extract_plan, publish_receipt) syntaktisch ok; heredoc `PY` schließt Zeile 171 korrekt (D3: Umleitung auf Python-Invocation).

## repo-rule compliance

- Produkt-Branch: `develop` (3431b22c); Branch `work/r27-correction-v35` (f57b30ed) = v3.5-Kandidat.
- Read-Only-Review; kein Produkt-Push, kein PR-Kommentar, root-git unangetastet.
- Lizenziert/Regeln: keine Produkt-Aktionen ausgeführt; Evidence nur in `evidence-r27/` + `tmp-ops/rv3-scratch`.

## Ende

END RV3-ACCEPT


## Coordinator relocation note
- rv3.md lag im ROOT evidence-r27/ (FehlAblage wie rv1); verschoben nach baseharbor-plan/evidence/board-2026-10-27/rv3-review-f57b30ed.md.
- rv3-Vorpruefung meldete END RV3-ACCEPT auf f57b30ed; CODEX-R3 dokumentierte am selben SHA reproduzierte PARTLY/OPEN-Blocker (D2 shell-Pfade bypassed, lost-ack discard). Gemäss RUNBOOK 5 ist der Lane-Verdict KEINE Abnahme: ACCEPT-Historie gilt nur als Vorpruefungs-Dokument, Abnahme bleibt CODEX-Runde auf v3.6.
