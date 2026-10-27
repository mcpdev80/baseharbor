# W8 · Persistent receipt-class bootstrap in the develop-planner

Mission: `ops/missions/w8.txt`
Repository: `baseharbor` | Worktree: `/home/marcel/dev/baseharbor-ecosystem/worktrees/w8-plansync`
Branch: `w8/plansync` (base `433a5702` = `origin/develop`)
Candidate commit: `d5467808cb2b635670c5de1684ce7d95803c65ec`
"ci(plan): bootstrap persistent receipt-class mappings"
Push: NOT performed (mission: KEIN push)

## PLAN

- [x] Step 0: pwd-PFLICHT + worktree `w8/plansync` aus `origin/develop`
- [x] Step 1: Praezedenz studieren (`RUNTIME_STEP_CAP_REVIEWED_HEAD`/`DISPATCH_GATE_REF_REVIEWED_HEAD`
  in `scripts/pr-gate-plan.py`; `git log -S` zeigt, dass Praezedenzen auf den Commit zeigen,
  in dem der Eintrag landet; `core752_reviewed_merge_chain` verlangt saubere first-parent
  Integration von vertrauensvollem develop
- [x] Step 2: MECHANIK-Entscheidung — Mission fordert „dauerhafte Klasse" (Zeile 7), nicht
  reviewed-head-Bootstrap. Gewählt: `RECEIPT_CONTROL_PLANE_*`-Sätze + Abfang vor dem
  runtime-workflow/ci-executor Fallthrough
- [x] Step 3: Simulation BEIDER Diff-Mengen gegen den aktuellen Planner (Baseline)
- [x] Step 4: Abfang implementiert, Regressionen zu `test_pr_gate_plan.py` ergänzt
- [x] Step 5: Simulation BEIDER Mengen gegen den committed SHA `d5467808`
- [x] Step 6: `test_pr_gate_plan.py` (131), `test_pr_diff_base`, `ci-workflow-policy.sh` gruen
- [x] Step 7: Commit `d5467808`, KEIN Push
- [x] Step 8: Evidence diese Datei

## Kontext / Relations-Klaerung

- Die beiden PR1/PR2-Kandidaten sind externe PR-Spitzen, die nicht im Workspace klonbar sind:
  - PR1 = `3a0df114b8c83d950e8ad079c10dfaf9ec4a335b` (`integrate: L6 exact mapping`, Merge von
    `83d53e86` in `34159b1f`, auf dem reviewed/frozen PR1-Baseline `ec82d476`). Die missionierte
    Planner-Aenderungsmenge ist der **L6-Diff**: 3 geaenderte Planner-Dateien.
  - PR2 = `c1c7db7b8079ad8348a63d406261fc0fa725477b` (`ci(pr2): resolve real base SHA...`,
    Branch `r2/ustrack`); die missionierte Menge ist der Diff gegen `084b3abc` (L3):
    `.github/workflows/receipt-post-run.yml` (modified), `scripts/receipt_tasklink.py` (added),
    `scripts/test_receipt_tasklink.py` (added). Diese SHAs und die Mengen sind aus
    `evidence/board-2026-10-26/b2-pr2-bootstrap-needs.md`, `r3-reviews-pr2.md` und
    `pr1-acceptance-DRAFT.md` rekonstruiert; beide Kandidaten-SHAs sind 40-hex.
- Die vorangehenden Bootstrap-Vorbilder (RUNTIME_STEP_CAP / DISPATCH_GATE_REF) sind
  **reviewed-head-Eintraege**: Sie verifizieren clean trusted-target merges bis zu einem
  eingefrorenen HEAD plus exakter File-Hashes und decken damit nur EINENfesten Kandidaten ab.
  Die Mission verlangt explizit die dauerhafte Klasse (Zeile 7), weil jede spatere Receipt-PR
  ohne neuen Bootstrap abgedeckt sein soll.

## MECHANIK / Entwurf

Abfgng vor dem `runtime-workflow-unresolved` / `ci-executor-unresolved` Fallthrough in
`scripts/pr-gate-plan.py:1368-1390`. Drei Setze:

| Satz | Inhalte | Check-Klasse |
| --- | --- | --- |
| `RECEIPT_CONTROL_PLANE_SCRIPTS` | `receipt_writer.py`, `receipt_verifier.py`, `receipt_tasklink.py` | `CI_CHECKS` (pr-planner-tests, ci-policy, ci-executor-verifier-tests) |
| `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS` | `test_receipt_writer.py`, `test_receipt_verifier.py`, `test_receipt_chain.py`, `test_ci_workflow_policy_receipts.py`, `test_receipt_tasklink.py`, `test_pr_gate_plan_receipt_paths.py` | `CI_CHECKS` |
| `RECEIPT_CONTROL_PLANE_WORKFLOWS` | `receipt-approval.yml`, `receipt-post-run.yml`, `receipt-shadow.yml` | `CI_CHECKS + WORKFLOW_SYNTAX_CHECKS` (ci-policy, ci-actionlint, pr-planner-tests, ci-executor-verifier-tests) |

Alle vier Checks sind **bestehende** `CI_CHECKS`-/`WORKFLOW_SYNTAX_CHECKS`-Namen; keine
neuen Gate- oder Check-Namen, keine Runner-/ci.yml-/Evidence-Maschinerie-Aenderung.
Jeder Pfad, der nicht exakt einem dieser Satze gehoert, faellt auf den unveraenderten
fail-closed Fallthrough zurueck (`unresolved`).

## Simulation: PR1 (L6-Diff, 3 Planner-Dateien)

Diff gegen develop-Planner (target-base `34159b1f`, head `83d53e86`, merge-base `34159b1f`):

```
scripts/pr-gate-plan.py                  modified
scripts/test_pr_gate_plan.py             modified
scripts/test_pr_gate_plan_receipt_paths.py modified
```

Baseline (Planner vor W8):

| coverage_status | selected_gates | unresolved_coverage |
| --- | --- | --- |
| unresolved | [] | CI execution/evidence impact requires specific qualification: scripts/test_pr_gate_plan_receipt_paths.py |

Nach W8 (`d5467808`):

| coverage_status | selected_gates | unresolved_coverage |
| --- | --- | --- |
| resolved | [] | [] |

validations: `ci-executor-verifier-tests`, `ci-policy`, `pr-planner-tests`.

## Simulation: PR2 (tasklink-Diff, 3 Receipt-Pfade)

Diff gegen develop-Planner (target-base `084b3abc`, head `c1c7db7b`, merge-base `084b3abc`):

```
.github/workflows/receipt-post-run.yml     modified
scripts/receipt_tasklink.py                added
scripts/test_receipt_tasklink.py           added
```

Baseline (Planner vor W8):

| coverage_status | selected_gates | unresolved_coverage |
| --- | --- | --- |
| unresolved | [] | workflow execution/cleanup/evidence impact requires native qualification: .github/workflows/receipt-post-run.yml<br>unknown impact for scripts/receipt_tasklink.py<br>unknown impact for scripts/test_receipt_tasklink.py |

Nach W8 (`d5467808`):

| coverage_status | selected_gates | unresolved_coverage |
| --- | --- | --- |
| resolved | [] | [] |

validations: `ci-actionlint`, `ci-executor-verifier-tests`, `ci-policy`, `pr-planner-tests`.

## BELEG (lokal, Commit `d5467808`)

Simulation beider Mengen gegen den committed Planner (`git show d5467808` enthaelt beide
Aenderungen):

- PR1 L6-Diff -> `coverage_status=resolved`, `unresolved_coverage=[]`.
- PR2 tasklink-Diff -> `coverage_status=resolved`, `unresolved_coverage=[]`.
- Fail-closed-Negative: `scripts/receipt_producer.py` (nicht in einem Satz) ->
  `coverage_status=unresolved`, `unresolved=['unknown impact for scripts/receipt_producer.py']`.
- Fail-closed-Negative: `receipt_writer.py` + `cmd/baha/app_new_command.go` ->
  `coverage_status=unresolved` (CLI runtime-impact faellt nicht in den Receipt-Satz).

Testlaufe im Worktree (alle exit 0):
- `python3 scripts/test_pr_gate_plan.py` — 131 Tests OK (inkl. 5 neue
  `ReceiptControlPlaneMappingTests`: script, test, workflow, unlisted receipt, extra change).
- `python3 -m unittest scripts.test_pr_diff_base` — 13 OK.
- `bash scripts/ci-workflow-policy.sh` — PASS, 19 approved workflow files.

## repo-rule compliance

- Nur bestehende Checks/Validations referenziert (`CI_CHECKS`/`WORKFLOW_SYNTAX_CHECKS`);
  keine neuen Gates, keine Runner-Whitelist-, ci.yml- oder Evidence-Maschinerie-Aenderung.
- Kein reviewed-head-Eintrag nach Vorbild RUNTIME_STEP_CAP/DISPATCH_GATE_REF erzeugt; die
  Mission verlangt explizit die dauerhafte Klasse (Zeile 7). Der Abfang ersetzt/praevt den
  runtime-workflow/ci-executor Fallthrough, ohne ihn aufzuheben: nicht-listete Receipt-Pfade
  bleiben `unresolved` (Regressionen).
- Keine Funktion/Trigger auBerhalb des Mappings umgeschrieben; die Aenderungen an
  `test_pr_gate_plan.py` beschraenken sich auf die neue Test-Klasse.
- Exakte Satz-Mengen statt Prefix-Freigabe; das fail-closed Prinzip des Planners bleibt erhalten.
- KEIN push ausgefuehrt; Arbeit in isoliertem Worktree, ein Commit `d5467808`.

END W8-DONE
