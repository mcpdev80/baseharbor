# LANE rv5 — narrow re-check of the three F2 negative cases that rv4 dropped (rv4 END RV4-CHANGES ohne Liste)

Scope (read-only product): only write to `worktrees/rv5/evidence-r27/rv5.md` and `tmp-ops/rv5-scratch`.
Worktree `worktrees/rv5` created at detached `origin/work/r27-fix-f2-akte2b` = `3dcc58c7`.

## PLAN

- A) `scripts/test_pr_gate_plan.py`: ReceiptControlPlaneMappingTests — two classes, same name, unique per class?
- B) Real positive `plan()` call with non-empty valid diff vs head planner: registered path still delivered as unresolved even though tests exist?
- C) Akte-Fallback: old/unknown reviewed_head with partial inventory → expect REJECT; document actual behavior file:line.

## A) ReceiptControlPlaneMappingTests — class collision / uniqueness

Command (grep class definitions):
```
grep -n "class ReceiptControlPlaneMappingTests\|class .*Tests" scripts/test_pr_gate_plan.py
```
rc=0. Result: two class headers with identical name `ReceiptControlPlaneMappingTests`:
- line 1188: `class ReceiptControlPlaneMappingTests(unittest.TestCase):` — 100 `def ` members in its block.
- line 3540: `class ReceiptControlPlaneMappingTests(unittest.TestCase):` — 5 members (first: `check_ids`, then 4 test methods), with docstring `"Bootstrap regressions: persistent receipt-class mappings (v0.4.27)."`.

Command (unittest loader, unique class object — returns the SHADOWING one, i.e. line 3540):
```
cd scripts && python3 -c "import unittest; from test_pr_gate_plan import ReceiptControlPlaneMappingTests as R; s=unittest.TestLoader().loadTestsFromTestCase(R); print(len(s._tests))"
```
rc=0 → loads **4** test methods (the line-3540 class): `test_active_native_workflow_still_requires_native`, `test_complete_control_plane_inventory_resolves`, `test_receipt_execution_check_actually_runs_receipt_suites`, `test_unknown_receipt_like_path_stays_unresolved`.

Command (module-level load, exercises BOTH definitions → first class wins → 129 tests total):
```
cd scripts && timeout 90 python3 -m unittest test_pr_gate_plan -v 2>&1 | tail
```
rc!=0. Result: `Ran 129 tests ... FAILED (failures=2, errors=3)` (the 129 belong to the shadowed line-1188 class). Direct class reference `test_pr_gate_plan.ReceiptControlPlaneMappingTests` runs only **4** (line-3540 class).

Auswertung A: **Kein einzigartiger Name.** Beide Klassen tragen exakt `ReceiptControlPlaneMappingTests`. Der erste Block (L1188, 100 Methoden) wird durch den zweiten (L3540, 5 Methoden) überschrieben; der Name ist nicht klassenweit eindeutig. `unittest`-Load von `ReceiptControlPlaneMappingTests` (direkter Zugriff) lädt die zweite/aktuelle Klasse (4 Tests); der Suite-Laden (129 Tests) referenziert die erste. Der zweite Block ist die autoritative Definition (Docstring + neuer v0.4.27-Bezug); die erste Definition ist tot/überschrieben.

## B) Positiver plan() mit echtem, nicht-leerem validen Diff vs Head-Planner

Scratch setup (tmp-ops/rv5-scratch): bare2 repo + full clone, HEAD = `13c8af2d` (baseharbor main), branch `main`.
Probe `tmp-ops/rv5-scratch/probe_plan.py`:
```
python3 probe_plan.py
```
rc=0. Ausgabe:
```
CASE1 (valid+unreviewed): unresolved | unresolved: ['CLI runtime impact needs qualified targeted native coverage: cmd/baha/app_main_command.go']
CASE2 (single valid): resolved | unresolved: []
CASE2 checks: ['ci-executor-verifier-tests', 'ci-policy', 'pr-planner-tests', 'receipt-execution-tests']
```
Auswertung B: Ja — ein echter positiver `plan()`-Aufruf mit NICHT-LEEREM validem Diff liefert eine **registered path als UNRESOLVED**, obwohl Tests existieren. Konkret: `cmd/baha/app_main_command.go` ist eine im PR-gate-map registrierte, aber nicht geprüfte (unreviewed) Path; selbst bei gültigem Diff (`coverage_status: unresolved`, `selected_gates: []`) bleibt sie in `unresolved_coverage` stehen. Erst die Single-Valid-Path-Änderung (`scripts/receipt_writer.py`) löst auf (`resolved`, 4 checks). Der Planner resolves also nicht „über Tests hinweg", sondern nur bei sauberem Diff ohne nicht-qualifizierte registered Path.

## C) Akte-Fallback: alter/unbekannter reviewed_head mit Teillinventar → REJECT

Betrreffende Stelle: `scripts/pr-gate-plan.py:855` (`akte_finalhead_checks`) + Aufruf in `plan()` bei `scripts/pr-gate-plan.py:1351` (`... if unresolved else None`). Validierung des Reviews-Head:
- L880-891: Akte-Record wird aus `integration_sha:docs/internal/ci-binding-qualifications.md` geladen (FINALHEAD), nicht Target-Base.
- L913-924: `set(record) != expected_keys` / `reviewed_head` Regex `[0-9a-f]{40}` / `file_hashes` SHA-256 / `files` vs `file_hashes` Schlüssel-Übereinstimmung — bei Abweichung `return None`.
- L926-931: `file_hashes` werden gegen echte Blob-Shas auf `reviewed_head` verifiziert, mismatch → `return None`.
- L937-946: nur bei `record_files` nicht leer UND `required_checks` abbildbar → `return checks` (sonst `None`).
- L948: alle Ausnahmen (OSError/CalledProcessError/ValueError/TypeError/KeyError) → `return None` (fail-closed).

Probe `tmp-ops/rv5-scratch/probe_akte.py`: `plan()` mit Diff containing `cmd/baha/app_main_command.go` (→ unresolved, also Akte-Fallback aktiv) und unbekanntem/altem `reviewed_head`.
```
python3 probe_akte.py
```
rc=0. Ausgabe:
```
C coverage_status: unresolved
C unresolved_coverage: ['CLI runtime impact needs qualified targeted native coverage: cmd/baha/app_main_command.go']
C selected_gates: []
C RESULT: OK - old/unknown reviewed_head REJECTS (fail-closed)
```
Auswertung C: Tatsächlich **REJECT** — fail-closed. Der Akte-Fallback löst nur bei vollständigem, validem Inventory mit übereinstimmenden Blob-Shas; ein alter/unbekannter `reviewed_head` mit Teillinventar liefert `None` (L948 / L926-931), `coverage_status` bleibt `unresolved`, `selected_gates` bleibt `[]`. Erwartung erfüllt; dokumentiert an `scripts/pr-gate-plan.py:913-948`.

## repo-rule compliance

- Produktcode nur gelesen (read-only). Keine Änderungen in baseharbor/Produktbranch.
- Schreiben ausschließlich in `worktrees/rv5/evidence-r27/rv5.md` (dieses File) und `tmp-ops/rv5-scratch/*` (Scratch-Repos + 2 Probes). Kein push, kein push zu root-git.
- Commits im worktree/rv5 ohne push; root-git/force/dispatch/main nicht verwendet.
- Git-Schwere (shallow update / unpacker error) durch frvollen Clone + fehlende Objects per `git fetch` aus dem Originalrepo ergänzt; bare2 neu angelegt. Kein Force-Push, keine Branch-Protection-Umgehung.
- Kein Evidence außerhalb evidence/.../scripts/-Pfad produziert; Proben liegen in tmp-ops/rv5-scratch.

## END RV5-ACCEPT
