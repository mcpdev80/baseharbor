# CI38B – NUR-LESE CI-Dossier Kandidat 94f35d02 (#1138)

Stand: 2026-10-28 (Lane ci38b, flash). PR #1138 `ci(r27): v3.8 staging - v3.7 + R5-Integration` (base=develop, head=work/r27-correction-v38, OPEN).
Quellen: `gh pr view/checks`, `gh api .../check-runs`, `gh api .../actions/jobs/<id>/logs`. Keine Repo-Writes, keine gh-Schreibaktion, kein rerun.

## PLAN
- [x] BOOT: `gh pr view 1138 --json headRefOid` -> 94f35d0293f8e237895afe84ae7aa9b26722ffed (Kandidat bestätigt)
- [x] Check-Tabelle aller Checks zu 94f35d02 (API check-runs, 22 Einträge, alle completed)
- [x] Jede FAILURE: Run -> Job -> Log-Kernmeldung + Klassifizierung A/B gegen d5 (`evidence/board-2026-10-27/d5-ci37.md`)
- [x] Wiederverwendbare grüne Evidence für 94f35d02 feststellen
- [x] Blockliste (vor #1142-Merge nicht grün werdbar) erstellen
- [x] Post-Check: headRefOid unverändert (read-only-Review ohne Kandidatenänderung)

## 1) Check-Matrix (Run 40614194159, SHA 94f35d02)

| Check | Status | Conclusion | Link |
|---|---|---|---|
| Workflow policy | completed | success | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572723910 |
| PR · Trusted Plan | completed | success | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572724138 |
| PR · Validation · ci-policy | completed | success | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572794808 |
| PR · Validation · ci-executor-verifier-tests | completed | success | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572794842 |
| PR · Validation · ci-actionlint | completed | success | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572794849 |
| PR · Validation · mkdocs-de-strict | completed | success | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572794824 |
| PR · Validation · mkdocs-en-strict | completed | success | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572794927 |
| PR · Validation · documentation-audit | completed | success | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572794924 |
| PR · Validation · pages-candidate-build | completed | success | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572794915 |
| PR · Validation · pr-planner-tests | completed | success | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572794832 |
| PR · Focused Python | completed | **failure** | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572794895 |
| PR Selected Checks | completed | **failure** | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122573343025 |
| PR · Consumer Pins | completed | skipped | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572797804 |
| PR · Demo Pin | completed | skipped | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572797437 |
| PR · Exact Candidate | completed | skipped | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572797147 |
| PR · Public Contracts | completed | skipped | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572798795 |
| PR · Runner Interface | completed | skipped | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572798008 |
| PR · Selected Runtime | completed | skipped | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572798266 |
| Release Source Preflight | completed | skipped | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572725301 |
| Targeted HA · docker | completed | skipped | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572725281 |
| Targeted HA · podman | completed | skipped | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572725169 |
| validate | completed | skipped | https://github.com/mcpdev80/baseharbor/actions/runs/40614194159/job/122572776165 |

Aggregat: 10 success, 2 failure, 10 skipped, 0 pending. Einziger Run auf dem SHA: 40614194159.

## 2) FAILURE-Analyse (Run 40614194159)

### F1 – PR · Focused Python (job 122572794895) – Klasse B
Kernmeldung (1 Zeile): `ValueError: unsupported executable validation` in `pr-check-runner.py:107 execute_validation`, ausgelöst durch `execute --check receipt-execution-tests` nachdem der Runner per `git fetch 3431b22c94b068f15f9bd7b31b9f87e846200f0f` (trusted BASE-Runner, develop) geladen wurde.
WICHTIG: Alle Python-Suiten VOR dem Fehler grün (`Ran 138 tests ... OK` u. weitere OK-Suiten; KEIN `FAILED (failures=2, errors=3)` mehr). Die d5-Klasse-A-Fehler (test_receipt_verifier Origin-ERRORs, test_ci_workflow_policy_receipts Dormancy-FAILs) sind auf diesem Kandidaten **behoben** – Gegenstück zu d5-Matrixzeile #1129/v35.
Klassifizierung gegen d5: exakt Muster d5-Muster 2 Klasse B / Empfehlung R4-3 – neuer Check gegen unveränderten Base-Runner auf develop (3431b22) = zirkulär, vor Base-Merge strukturell nicht grün.

### F2 – PR Selected Checks (job 122573343025) – Klasse B (abgeleitet)
Kernmeldungen (1 Zeile je Block):
1. `BLOCKED: required authenticated job not successful: PR · Focused Python` (FOCUSED_JOB: failure) – reine Downstream-Sperre wie d5-Muster 3.
2. `BLOCKED: coverage-unresolved: ... .github/workflows/ci.yml; evidence-r27/{ci1-repair,cy1,cy3,f1-fix,f1x,f3b-fix,g1x,g1y,g2x,r5i,s2-v3staging,w1,w3,w4b3,w4c,w5,x1-fix}.md; evidence/g1/evidence-r27/g1.md; scripts/pr-check-runner.py; scripts/test_pr_check_runner.py; unknown impact for scripts/test_receipt_postrun.py` – erzeugt von `pr-required-final.py` pinned bei c6db56c4 (Base-Pfad). Die Liste deckt sich 1:1 mit den von #1138 geänderten Dateien; die Qualifikations-Mappings des alten Base kennen die v3.8-Kette noch nicht. Ebenfalls Klasse B (alter Base), kein Substanzbefund im Code.

Keine weitere FAILURE vorhanden.

## 3) Grüne, wiederverwendbare Evidence für 94f35d02 (AGENTS CI-Effizienz)
Für denselben unveränderten Commit 94f35d02 bereits grün und in einer künftigen Welle NICHT erneut auszuführen:
Workflow policy; PR · Trusted Plan; PR · Validation · ci-policy; ci-executor-verifier-tests; ci-actionlint; mkdocs-de-strict; mkdocs-en-strict; documentation-audit; pages-candidate-build; **pr-planner-tests** (war auf v35 rot – jetzt grün, Klasse-A-Beweis erbracht).
Skip-Checks (Consumer Pins, Demo Pin, Exact Candidate, Public Contracts, Runner Interface, Selected Runtime, Release Source Preflight, Targeted HA docker/podman, validate) sind laut Plan nicht erforderlich (SELECTED_RUNTIME_REQUIRED=false, PUBLIC_CONTRACTS_REQUIRED=false) – kein Nachholbedarf.

## 4) Blockliste – vor #1142-Merge nicht grün werdbar
- **PR · Focused Python**: bleibt rot, solange ci.yml den `receipt-execution-tests`-Step gegen den trusted BASE-Runner (develop 3431b22) fährt, dieser den Check aber nicht kennt. Auflösung exakt über #1142 `ci(plan): minimal executable-validation bootstrap` (OPEN, base=develop, head b9c3ba05, mergeable) – Bootstrap in develop warten, danach re-run des Steps auf diesem Kandidaten.
- **PR Selected Checks**: nicht unabhängig reparierbar; braucht (a) grünes Focused Python (Blockliste-Punkt 1) und (b) Base-seitige Coverage-Mappings (`pr-required-final.py`@c6db56c4 / release-requirements.json), die die v3.8-Änderungsdateien qualifizieren – beides hängt an develop-Merge-Stand (#1142 bzw. v3.8-Kette), vorab strukturell blockiert.
- Alles andere ist entweder bereits grün (Abschnitt 3) oder planmäßig skipped; keine weiteren Mandatory-Checks blockiert.

## Fazit
Der Kandidat 94f35d02 ist substanzseitig clean: Klasse A aus d5 ist behoben, nur die beiden Klasse-B-Base-Zirkularitäten (F1/F2) verhindern Grün. Freigabeweg: #1142 -> develop mergen, dann Focused-Python/Selected-Checks für 94f35d02 erneut ausführen (von anderer Lane, hier nicht ausgelöst).

Post-Check: headRefOid nach dem Lese-Review erneut verifiziert: 94f35d0293f8e237895afe84ae7aa9b26722ffed (unverändert, Kandidat nicht modifiziert).

END CI38B-DONE
