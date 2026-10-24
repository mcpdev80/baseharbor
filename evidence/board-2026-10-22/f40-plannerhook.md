# f40-plannerhook — PR #1056: Qualifikations-Hook auf docs/internal/ci-binding-qualifications.md

Task-Vertrag: ops/missions/f40.txt (Coordinator/Marcel-Pool, Owner-freigegeben).
PR: baseharbor #1056, branch o90-planner-pr. Umsetzung des F39-Rezepts (f39-meta1056.md).
git NUR in baseharbor/, ROOT-git verboten. KEIN Merge durch diese Lane.

## PLAN

- [x] 0. Evidence-Datei + native Todos ([wip:f40]) anlegen
- [x] 1. Realitätsabgleich: PR #1056 / branch o90-planner-pr HEAD, develop-Stand, Status der Akte docs/internal/ci-binding-qualifications.md (Referenz o102-ciakten.md existiert lokal NICHT -> Remote via gh prüfen)
- [x] 2. Code-Basis lesen: trusted_binding_checks (scripts/pr-gate-plan.py:615-701) + Klassifikator-Fallback (Zeilen ~1175-1180) am aktuellen PR-HEAD
- [ ] 3. Fail-first-Test in test_pr_gate_plan.py: PR-only-CI-Scripts OHNE Akten-Eintrag bleibt unresolved; MIT Eintrag resolved (erst ROT nachweisen)
- [ ] 4. Hook in pr-gate-plan.py: Akte als Authorität für die drei CI-Script-Pfade, Wiederverwendung trusted_binding_checks, kein Neubau des Mechanismus
- [ ] 5. Fokussierte Tests lokal ausführen (grün)
- [ ] 6. Abhängigkeits-Gate: Akte in develop? PR-HEAD der Akten-PR prüfen. Push nur wenn Akte in develop liegt; Arbeit sonst vorbereiten und zurückhalten
- [ ] 7. Commit auf o90-planner-pr (nur baseharbor-git), Push gemäß Schritt 6, PR #1056 aktualisiert
- [ ] 8. Evidence finalisieren: repo-rule compliance, head + Abhängigkeits-Status, END F40-PLANNERHOOK-DONE

## Befunde / Status

- basics-check FLAGS (Turn-Start): DISK /tmp 1304MB (<1500, spawn-Gate — kein Spawn in dieser Lane),
  176 unread reminders (Coordinator-Railway), IDLE LANES o101-merge1053/q102-planpub (Coordinator-Scope),
  POOL qwen 1<2, PLAN coordinator-plan.md 267min — alles Coordinator-Actions, außerhalb F40-Scopes; hier dokumentiert, nicht ausgeführt.
- o102-ciakten.md lokal nicht vorhanden (nur ops/missions/o102.txt + watcher-state Hinweis); Akten-Fernzustand wird über gh verifiziert.
