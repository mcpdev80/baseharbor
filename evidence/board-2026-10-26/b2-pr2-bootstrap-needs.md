# B2 · PR2 Bootstrap-Needs (PR2-CI-Vorab-Simulation)

Mission: ops/missions/b2.txt
Repositorium: baseharbor | Worktree: /home/marcel/tmp-ops/wt-b2sim (6e170894, abgetrennt) | KEIN push

## PLAN

- [x] Step 0: pwd-PFLICHT + Worktree wt-b2sim aus 6e170894 (r2/ustrack)
- [x] Step 1: PR2-Diff gegen den develop-Planner (Kopie von origin/develop = c1c7db7b) simulieren
- [x] Step 2: ungelöte Pfade identifizieren, die Bootstrap-Abdeckung über W7s 3-Dateien-Satz HINAUS brauchen
- [x] Step 3: Deliverable evidence/board-2026-10-26/b2-pr2-bootstrap-needs.md
- [x] Step 4: wt-b2sim entfernen

## Kontext / Relations-Klärung

- 6e170894 = PR2-Zweig-Spitze (`feat(ci): bounded automatic task link...`, Branch r2/ustrack).
- c1c7db7b = develop-Kopie (`ci(pr2): resolve real base SHA...`); 6e170894 ist ANCESTOR von c1c7db7b.
- Kette linear: c1c7db7b → 6e170894 → 084b3abc (integrate L3) → 3103cc33 (integrate L2).
- Der von der Mission gemeinte „PR2-Diff" (develop...HEAD) = die 3 vom PR2-Zweig neu eingeführten/geänderten Dateien relativ zu 084b3abc:
  - `.github/workflows/receipt-post-run.yml` — modified
  - `scripts/receipt_tasklink.py` — added (183 Zeilen)
  - `scripts/test_receipt_tasklink.py` — added (244 Zeilen, inkl. `TestBaseShaResolution`, 6 fail-closed Tests)

## Simulation

Werkzeugkette (aus baseharbor/scripts, nicht aus dem PR2-Zweig): `pr-diff-base.py` → `pr-gate-plan.py`.

```
pr-diff-base.py --repository . --target-base 084b3abc --head 6e170894
  => merge_base 084b3abc, 3 Changes:
       .github/workflows/receipt-post-run.yml  modified
       scripts/receipt_tasklink.py             added
       scripts/test_receipt_tasklink.py        added

GITHUB_REPOSITORY=mcpdev80/baseharbor GITHUB_BASE_REF=develop GITHUB_HEAD_REF=r2/ustrack \
  pr-gate-plan.py --diff <obige 3>
  => coverage_status:  "unresolved"
  => broad_required:   true
  => selected_gates:   []
  => unresolved_coverage:
       - workflow execution/cleanup/evidence impact requires native qualification: .github/workflows/receipt-post-run.yml
       - unknown impact for scripts/receipt_tasklink.py
       - unknown impact for scripts/test_receipt_tasklink.py
```

Beachteter Zustand: **coverage_status = unresolved**, alle 3 Pfade ungedeckt, `selected_gates` leer. Der develop-Planner kann den PR2-Zweig allein nicht qualifizieren.

### Warum jeder der 3 Pfade ungedeckt ist (Entwurf im develop-Planner, baseharbor/scripts/pr-gate-plan.py)

- `.github/workflows/receipt-post-run.yml` → `elif path.startswith('.github/workflows/')` → `runtime-workflow-unresolved` (nur CI- + actionlint-Pflicht, kein abgedeckter Gate).
- `scripts/receipt_tasklink.py` → kein Set trifft zu (nicht `scripts/pr-`/`scripts/ci-`/`scripts/test_pr_`/`scripts/verify-`, nicht in `RECEIPT_PILOT_SCRIPTS`, nicht in `PLANNING_SCRIPTS`) → Fallthrough `unresolved` („unknown impact").
- `scripts/test_receipt_tasklink.py` → gleich: beginnt mit `scripts/test_` aber nicht `scripts/test_pr_`, nicht in `RECEIPT_PILOT_SCRIPTS` → Fallthrough `unresolved`.

Der existierende `RECEIPT_PILOT_SCRIPTS`-Eintrag deckt nur `{receipt_writer.py, receipt_verifier.py, test_receipt_chain.py}` ab — die PR2-Dateien sind bewusst/nicht enthalten. Es existiert **kein** Bootstrap-Eintrag für receipt_tasklink / test_receipt_tasklink / receipt-post-run.

### Abgrenzung zu W7 (die 3-Dateien-Satz ist NICHT dieselbe Menge)

- W7 (`ops/missions/w7.txt`) bearbeitet die Planner-Selbständerung: `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`, `scripts/test_pr_gate_plan_receipt_paths.py` (+ exakter `R27_PLANSYNC_REVIEWED_HEAD/REVIEWED_FILES`-Bootstrap-Eintrag nach Vorbild der Nachbarn).
- W7s 3 Dateien sind **Planner-intern**; der PR2-Diff sind **3receipt-Dateien**. Beide Sätze sind disjunkt.
- W7s Bootstrap-Eintrag löst **nur** W7s 3 Plannerdateien; er fählt `receipt_tasklink.py` / `test_receipt_tasklink.py` / `receipt-post-run.yml` **nicht** (kein Set/Entry trifft zu, kein early-return im Planner).

## Exakte Menge der Pfade, die der Bootstrap-Eintrag (develop-Kopie) abdecken MUSS

Damit PR1 UND PR2 ohne dritte Schleife durchkommen, muss der entwickelte Bootstrap-Eintrag **alle 3 PR2-Pfade** abdecken:

1. `.github/workflows/receipt-post-run.yml` (modified)
2. `scripts/receipt_tasklink.py` (added)
3. `scripts/test_receipt_tasklink.py` (added)

Das sind exakt **3 Pfade**. Jeder einzelne muss per exaktem HEAD+Files-Eintrag (Vorbild `MCP_PUBLIC_BOOTSTRAP_REVIEWED_HEAD/REVIEWED_FILES`, `RUNTIME_STEP_CAP_*`, `DISPATCH_GATE_REF_*`) im develop-Planner erkannt werden und das jeweilig nötige Gate (Workflow: CI + actionlint; Scripts: PR-Planner-/Receipt-Suite) selektieren. Fehlt ein Pfad, bleibt `coverage_status=unresolved` und der finale Check schließt fail-closed.

## Muss W7s Scope erweitert werden? **NEIN**

Begründung:

- W7s 3 Dateien (`pr-gate-plan.py`, `test_pr_gate_plan.py`, `test_pr_gate_plan_receipt_paths.py`) sind Planner-Infrastruktur; der PR2-Diff enthält **keine** dieser Plannerdateien.
- Der PR2-Bedarf ist eine **andere Datei-Menge** (Receipt-/Workflow-Pfade) und gehört in einen **eigenen, separaten** Bootstrap-Eintrag (PR2-spezifisch, eigener `*_REVIEWED_HEAD`/`REVIEWED_FILES`), nicht in W7s Plansync-Eintrag.
- Eine Erweiterung von W7s Scope um die Receipt-Pfade würde W7s isoliertes Write-Set (nur Planner-Dateien) aufbrechen und den „andere Datei-Menge"-Kontext von b2 verletten. Richtiger Ort: ein gesonderter PR2-Bootstrap-Task/Trag (parallel zu W7, disjunkt).

## repo-rule compliance

- Nur lokaler abgetrennter Worktree /home/marcel/tmp-ops/wt-b2sim (6e170894); KEIN Push, nichts am develop-/r2-Strang geändert.
- Analyse nur gegen develop-Kopie (c1c7db7b) und PR2-Zweig (6e170894) via `pr-diff-base.py`/`pr-gate-plan.py` aus baseharbor/scripts; keine Produkt-/Planner-Dateien geändert.
- Keine PR1-Kerndateien berührt; W7-Write-Set unangetastet.
- Alle SHAs lokal verifiziert (`git cat-file -t` → commit, 40-hex).

## END B2-DONE
