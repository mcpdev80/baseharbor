# B1RV · PR-B #1091 Review (read-only)

ROLLE: REVIEW (read-only Produkt + PR #1091 Diff; Evidence only)
SHA: 8f8e5547281781d7dafd284bf18f98c31049e9e1 (l7/ustrack HEAD)
PR:  #1091 `ci(plan): bootstrap persistent receipt-class mappings` — base origin/develop 433a5702, head l7/ustrack 8f8e5547, STATE open (nicht merged)
UTC: 2026-10-26T22:54Z
Gegenstand: l7/ustrack @ 8f8e5547 (Parent d5467808-Linie) = persistente RECEIPT_CONTROL_PLANE-Klassen im develop-Planner.

## VERDICT: REWORK

Die Klassen-Mappings sind **strukturell korrekt, vollständig und fail-closed** — der
geprüfte Candidate 8f8e5547 ist aber **kein akzeptierbarer Kandidat**: er enthält
eine **import-time Regression**, die die neue Testdatei
`scripts/test_pr_gate_plan_receipt_paths.py` am Import hindert und damit die in
Mission b1rv.txt geforderte Selbst-Qualifikation (`coverage_status resolved`)
technisch **nicht nachweisbar** macht. Die Abbildungslogik selbst ist richtig und
wird durch die bestehende, laufende `ReceiptControlPlaneMappingTests`-Klasse in
`test_pr_gate_plan.py` (6 Tests, OK) bereits vollständig abgedeckt. Der Fehler ist
eine einzelne Datei; Behebung = REWORK, kein Architektur-Fundament.

## Beweise (jeder Prüfpunkt)

### (1) Klassen-Mappings eng & vollständig — ERFULLT
`git diff 433a5702 8f8e5547 --stat` = 3 Dateien (`pr-gate-plan.py`, `test_pr_gate_plan.py`,
`test_pr_gate_plan_receipt_paths.py`), exakt wie PR #1091 review-diff. Mapping-Sätze in 8f8e5547:
- `RECEIPT_CONTROL_PLANE_SCRIPTS` = {`receipt_writer.py`, `receipt_verifier.py`, `receipt_tasklink.py`, `ci-workflow-policy.sh`}
- `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS` = {`test_receipt_writer.py`, `test_receipt_verifier.py`, `test_receipt_chain.py`, `test_ci_workflow_policy_receipts.py`, `test_receipt_tasklink.py`, `test_pr_gate_plan_receipt_paths.py`}
- `RECEIPT_CONTROL_PLANE_WORKFLOWS` = {`receipt-approval.yml`, `receipt-post-run.yml`, `receipt-shadow.yml`}
- Dispatch in `plan()` (pr-gate-plan.py:1371-1380) vor `RECEIPT_PILOT_SCRIPTS`/runtime-workflow/ci-executor-Fallthroughs → Doc-Comment-Anpruch "not masked" bestätigt.
Empirisch gegen develop-Basis (REQ/MAP, `integration/static/public-contracts`): alle 13 überdeckten Pfade lösen individuell `resolved` auf; `ci-workflow-policy.sh` → `ci-policy` + `pr-planner-tests` + `ci-executor-verifier-tests`.

### (2) Reversed-Check (keine übermäßige Klassifikation) — ERFULLT, mit Testfallen
`pr-gate-plan.py:643-676` prüft exakte frozenset-Mitgliedschaft, kein Präfix — kein
Pfad-Diffusität. Fallen gegen 8f8e5547:
- `scripts/pr-check-runner.py`, `scripts/verify-pr-gate-plan.sh`, `evidence/receipts/pilot.json` → **UNRESOLVED** (ci-executor-Fallthrough).
- `scripts/receipt_publisher.py`, `scripts/test_receipt_publisher.py`, `scripts/receipt_chain_test.py` → **UNRESOLVED** ("unknown impact").
- `.github/workflows/runtime-s3-acceptance.yml` → **UNRESOLVED** + "native qualification".
- `scripts/receipt_producer.py` (neu in Test) → **UNRESOLVED**.
Kein Nicht-Receipt-Pfad wird durch die neuen Zeilen falsch als `resolved` klassifiziert.

### (3) Parallele zum Präzedenzz 5be7dabd — strukturell identisch
5be7dabd (#1043): 2 Dateien (`pr-gate-plan.py` +26/-… , `test_pr_gate_plan.py` +18),
Marcel Pfingstgräf, "anchoring accepted sources". 8f8e5547: gleiches Muster —
`pr-gate-plan.py` (Mapping-Klassen + Dispatch), `test_pr_gate_plan.py`
(`ReceiptControlPlaneMappingTests`), Self-Qual über existierenden Planner-Pfad.
Beide: neue fail-closed Mapping-Klasse an bestehende CI/WIZARD/PYTEST gebunden,
kein neuer Matrix/Runner. Strukturrelevanz bestätigt; die neue standalone
`test_pr_gate_plan_receipt_paths.py` ist neu und fehlerhaft (siehe 4).

### (4) `python3 scripts/test_pr_gate_plan.py` + Receipt-Suiten — TEILWEISE GRÜN
- `python3 -m unittest scripts.test_pr_gate_plan` → **OK** (alle Suiten, inkl.
  `ReceiptControlPlaneMappingTests.test_ci_workflow_policy_script_maps_to_ci_class`,
  `test_receipt_control_plane_test_script_maps_to_ci_class` listet
  `test_pr_gate_plan_receipt_paths.py`, 6 Tests OK).
- `python3 -m unittest scripts.test_pr_gate_plan_receipt_paths` → **CRASH beim Import**
  (`test_pr_gate_plan_receipt_paths.py:18`): `AttributeError: module 'planner' has no
  attribute 'RECEIPT_PILOT_SCRIPTS'`. Der Candidate 8f8e5547 definiert
  `RECEIPT_CONTROL_PLANE_*`, nicht mehr `RECEIPT_PILOT_*` — die standalone Datei wurde
  beim API-Rename (Commits 0b6f098d→83d53e86) nie aktualisiert. Module-Scope-Zugriffe
  `planner.RECEIPT_PILOT_SCRIPTS`, `planner.RECEIPT_PILOT_CHECKS` sowie die nie
  definierten `CI_POLICY_SOURCES`, `RECEIPT_DORMANT_WORKFLOWS` brechen den Import.
- Logik der `ReceiptControlPlaneMappingTests` in der Datei gegen die kommitte API
  neu ausgeführt (Policy-Script → CI_IDS, 3 Dormant-Workflows → CI_IDS∪SYNTAX_IDS,
  Active-Workflow → UNRESOLVED+native, Complete-Inventory → resolved,
  checks ⊆ runner.VALIDATIONS): **ALLE PASS**. Die Abbildung ist korrekt; nur der
  Datei-Bootstrap ist tot.

### (5) Selbst-Qualifikation / coverage_status resolved — NICHT nachweisbar via neuer Datei
- Basis 433a5702 → 8f8e5547 ( echter 2-Parent-Integrationspfad, Merge-SHA 542cb296):
  Planner auf die 3 geprüften Dateien → `coverage_status = resolved`,
  `unresolved_coverage = []`, `broad_required = False`. Die Behauptung ist **wahr**.
- Nachweisbarkeit über die in b1rv.txt gefuerte neue Suite scheitert am Import-Fehler
  (siehe 4). Die bestehende `ReceiptControlPlaneMappingTests` in `test_pr_gate_plan.py`
  (lauffähig, OK) bestätigt dieselbe Abbildung. Die in der Bootstrap-Notiz
  `l7-final-bootstrap.md` referenzierte "Simulation nach Anleitung" ist durch die
  lauffähige Kernsuite replizierbar; die standalone Datei ist kein tragfähiger Beweis.

## Fundort der Blockade
- `scripts/test_pr_gate_plan_receipt_paths.py:18` — `SCRIPTS = sorted(planner.RECEIPT_PILOT_SCRIPTS)` (Import-Time).
- Stale Module-Scope: `RECEIPT_PILOT_SCRIPTS`, `RECEIPT_PILOT_CHECKS`, `CI_POLICY_SOURCES`, `RECEIPT_DORMANT_WORKFLOWS`.
- Produktions-API 8f8e5547: `RECEIPT_CONTROL_PLANE_SCRIPTS/TEST_SCRIPTS/WORKFLOWS` (pr-gate-plan.py:643-676).

## REWORK-Anforderung (umsetzbar)
`scripts/test_pr_gate_plan_receipt_paths.py` so korrigieren, dass sie gegen die
kommitte `RECEIPT_CONTROL_PLANE_*`-API läuft und die `ReceiptControlPlaneMappingTests`
gegen 8f8e5547 importierbar **und** grün ist:
1. Module-Scope `SCRIPTS`/`PILOT_CHECK`/`CI_POLICY_SOURCES`/`RECEIPT_DORMANT_WORKFLOWS`
   durch `planner.RECEIPT_CONTROL_PLANE_SCRIPTS` bzw. `RECEIPT_CONTROL_PLANE_WORKFLOWS`
   ersetzen; `PILOT_CHECK` auf den Planner-lane-check (`pr-planner-tests`) setzen oder
   durch die laufende `ReceiptControlPlaneMappingTests`-Assertions ersetzen.
2. `python3 -m unittest scripts.test_pr_gate_plan_receipt_paths` lokal GRÜN.
3. `python3 -m unittest scripts.test_pr_gate_plan` weiterhin GRÜN (kein Rückbau).
4. Selbst-Qualifikation 433a5702→8f8e5547 erneut gegen den Planner verifizieren
   (`coverage_status=resolved`, `unresolved=[]`, `broad_required=False`).
5. Kein Force, kein Rebase; Diff gegen origin/develop 433a5702 bleibt auf die 3 Dateien.
6. Nach Approve: PR-B-Merge (8f8e5547), dann für PR1 den ordinary Adopt-Commit der
   `l7-final-bootstrap.md` (develop-Fassung von pr-gate-plan.py + Tests).

## Prüfungskette
Worktree `/tmp/b1rv_wt` aus 8f8e5547 (`git worktree add -f … 8f8e5547`, kein Push,
keine Write-Änderung im Produkt-Branch). Alle Assertions gegen den kommitten
Candidate; `git diff 433a5702 8f8e5547 --stat` und PR #1091 review-diff als Quelle.

END B1RV-REWORK
