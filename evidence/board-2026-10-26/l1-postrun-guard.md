# L1 – Post-Run Dormancy Guard auf Job-Ebene (Worker Lane L1)

Mission: ops/missions/l1.txt — Fix für CI Run 40386116932
(PR·Focused Python actionlint): `.github/workflows/receipt-post-run.yml`
Zeile 5 trug `if: false` auf WORKFLOW-Ebene (syntaktisch ungueltig).
Worktree: worktrees/l1-ustrack, Branch l1/ustrack, Base 78efc623
("integrate: W2 writer"). Ergebnis-Commit (ordinary, KEIN push):
9b185fd2 — "fix(ci): move post-run dormancy guard to job level",
1 Datei, +4/-3.

## PLAN

- [x] 0. Turn-Start-Pflichtprogramm basics-check.py ausgefuehrt; Flags siehe
      Befund 3; Evidence + PLAN angelegt
- [x] 1. Worktree worktrees/l1-ustrack (Branch l1/ustrack) aus 78efc623 angelegt
- [x] 2. Befund mit repo-verankertem actionlint reproduziert
      (go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck=):
      receipt-post-run.yml:5:1 unexpected key "if" for "workflow" section
- [x] 3. FIX ausschliesslich in receipt-post-run.yml: Workflow-level
      `if: false` entfernt (Dormancy-Kommentar sinngemaess erhalten, jetzt
      mit Hinweis auf Job-Ebene); einziger Job `post_run_writer` auf
      never-true Guard nach shadow-Muster gestellt
- [x] 4. Beleg: actionlint v1.7.12 auf der Zieldatei = PASS;
      zusaetzlich YAML-Parse (kein Workflow-level `if`, Job-Guard aktiv)
- [x] 5. EIN ordinary Commit 9b185fd2, KEIN push, Tree clean
- [x] 6. Abschluss: END L1-DONE

## Aenderung im Detail (Diff-Auszug)

- Zeile 5 `if: false` (workflow-level) entfernt; Dormancy-Kommentar bleibt
  sinngemaess mit Klarstellung, dass die Dormancy jetzt auf Job-Ebene sitzt.
- Job `post_run_writer` bisher `if: github.event.workflow_run.conclusion ==
  'success'`; neu analog receipt-shadow.yml (Zeile 23, Vorlage):
  `if: github.repository == 'mcpdev80/receipt-post-run-dormant' &&
  github.event.workflow_run.conclusion == 'success'`
  mit DORMANT-Kommentar. Begruendung der Verknuepfung mit `&&`: erhaelt die
  dokumentierte Laufzeitsemantik (nur nach erfolgreichem Source-&-Quality-Run)
  und laesst den Aktivierungs-Pfad in PR2 einheitlich zum shadow-Muster
  (Guard-Ausdruck ersetzen, Bedingung behalten).

## Beleg

- Repro VOR Fix (exakte CI-Meldung, identisches Tool/Kommando wie
  scripts/pr-gate-plan.py:657-658 / ci.yml:644):
  `.github/workflows/receipt-post-run.yml:5:1: unexpected key "if" for
  "workflow" section. expected one of "concurrency", "defaults", "env",
  "jobs", "name", "on", "permissions", "run-name" [syntax-check]` -> exit 1
- Nach Fix: `actionlint .github/workflows/receipt-post-run.yml` -> exit 0 (PASS).
- YAML-Parse (python3+PyYAML): Workflow-level `if` nicht mehr vorhanden;
  Job-if = `github.repository == 'mcpdev80/receipt-post-run-dormant' && ...`
  (never true auf mcpdev80/baseharbor).
- Diff enthaelt den job-level Guard ausschliesslich in der einen Datei;
  keine weiteren Aenderungen.

## Befunde / Entscheidungen

1. Scope-Grenze eingehalten: nur receipt-post-run.yml geaendert (Mission
   Zeile 3 "Keine other changes"). Kein push, kein PR, kein Tag.
2. **Ausserhalb dieses Lanes liegender Restbefund**: dasselbe Muster
   Workflow-level `if: false` existiert in .github/workflows/
   receipt-approval.yml:48 (im Base 78efc623 enthalten). Ein VOLLES
   Repo-actionlint laeuft dort weiterhin auf exit 1. Nicht korrigiert:
   Mission erlaubt nur die eine Datei. Noetige Folgereparatur ( identisches
   Rezept) sollte Coordinator/Codex zuordnen — vermutlich
   L2/L3-Parallellanes (worktrees/l2-ustrack, l3-ustrack existieren auf
   demselben Base); keine Selbsthilfe in dieser Lane.
3. Turn-Start basics-check FLAGS: 523 unread reminders, POOL ornith 1<2 und
   qwen 1<2 (nur bei vorhandener sinnvoller Arbeit zu fuellen), PLAN 32 open
   steps. Coordinator-seitige Punkte; als Worker-L1 hier nicht handlungsfaehig,
   an Coordinator weitergeleitet.
4. actionlint ist im Repo verankert (keine Make-Target-Noetigkeit gefunden;
   Pin v1.7.12 ueber `go run`, scripts/pr-gate-plan.py:658) — damit ist der
   Belegweg CI-identisch, das yaml-Fallback wurde zusaetzlich ausgefuehrt.

END L1-DONE
