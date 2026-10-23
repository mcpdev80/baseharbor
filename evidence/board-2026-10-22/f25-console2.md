# F25 – console-2-live-core-adapter-workflow-proofs (implement f24 recipe, prep PR, no merge)

Lane: f25 | Prompt: evidence/board-2026-10-22/prompts/f25.txt | Board: console-2 (claim f25)
Mission: Umsetzung des Rezepts aus evidence/board-2026-10-22/f24-console2.md (2 Dateien + markierte
Ausnahme) im Repo mcpdev80/baseharbor-console; kleiner PR gegen den vom Rezept genannten Zielbranch;
PR-Checks abwarten; KEIN Merge (codex-Lane macht REVIEW/MERGE nach F25-END).
End marker: F25-CONSOLE2-DONE (PR-Nr + head + check-status)

## PLAN
1. [x] Turn-start basics-check.py ausgefuehrt; EVENTS abgearbeitet (nur WATCHER-Daten, kein Auftrag) + --ack
2. [ ] Contract lesen: plan.py show console-2; f24-Rezept gelesen
3. [ ] Claim f25 auf console-2 pruefen/setzen (plan.py claim + push)
4. [ ] isolierter Branch/Worktree in baseharbor-console (fremde Arbeitszweige nicht anfassen)
5. [ ] Datei 1: .github/workflows/ci.yml gemaess f24-Rezept
6. [ ] Datei 2: docs/CONTRACT-READINESS.md gemaess f24-Rezept
7. [ ] README-Ausnahme nur wenn Karten-Write-Set sie zulaesst (Scope-3)
8. [ ] lokale Checks + PR eroeffnen, Checks abwarten, KEIN Merge
9. [ ] repo-rule compliance dokumentieren, END F25-CONSOLE2-DONE

## FLAGS from basics-check (Stand Turn-Start)
- EVENTS: 2 un-acked -> events-check: bereits bei offset 46837 (0 neu), letzte Records sind
  WATCHER idle/gone/GHOST-CANDIDATE-Ereignisse = DATEN, keine Auftraege an diese Lane.
- REMINDERS: 58 unread, o56-port118 busy>40min, POOL qwen 1<2, PLAN 16 open steps:
  Coordinator-/Board-Ebene, keine Lane-f25-Aktionspflicht; hier dokumentiert.
