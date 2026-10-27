# W7 · plansync (PR · Trusted Plan bootstrap entry)

Mission: ops/missions/w7.txt
Repository: baseharbor | Branch: w7/plansync | Base: origin/develop | KEIN push

## PLAN

- [ ] Step 0: pwd-PFLICHT + Evidence init
- [ ] Worktree worktrees/w7-plansync aus origin/develop anlegen
- [ ] Develop-Planner inspizieren (scripts/pr-gate-plan.py, RUNTIME_STEP_CAP_REVIEWED_HEAD, DISPATCH_GATE_REF_REVIEWED_HEAD, pr-diff-base.py, test_pr_gate_plan*.py)
- [ ] git log -S: Commit-Struktur der Praezedenz-Eintraege klaeren (Self-Referenz-Auflösung)
- [ ] Schritt 1: SIMULATIONSVORBEFUND - Planner von develop gegen Diff der 3 L6-Dateien; coverage_status / selected_gates / unresolved dokumentieren
- [ ] Schritt 2: bei unresolved/unknown: R27_PLANSYNC_REVIEWED_HEAD/_REVIEWED_FILES Bootstrap-Eintrag exakt nach Vorbild
- [ ] Schritt 3: Ziel-Diff = nur 3 L6-Dateien + Plannerdatei; PR1-Core-Dateien unangetastet
- [ ] Schritt 4: Tests (test_pr_gate_plan.py + neue Sync-Assertions + betroffene Develop-Suiten)
- [ ] EIN ordinary Commit im Worktree; Evidence finalisieren

## SIMULATIONSVORBEFUND (Schritt 1)

(pending)

## Gewaehlter Mechanismus

(pending)

## Self-Referenz-Aufloesung

(pending)

## Testausgaben

(pending)

## repo-rule compliance

(pending)
