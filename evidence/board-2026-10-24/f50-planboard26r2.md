# F50 – Planning/CardBoard v0.4.26 auf wahren Stand (Runde 2)

Mission: ops/missions/f50.txt (F50-PLANBOARD26R2). Runde 1 = o116, board-sha 61ecae1.
Scope: NUR 26-Karten (27 ist tabu, 26-FENCE). Git nur in baseharbor-plan. Karten stets mit voller Karten-ID.

## PLAN

- [x] 0. Evidence-Datei + native Todos vor erstem Schreib-Tool angelegt
- [x] 1. basics-check.py (Turn-Start-Pflicht) laufen lassen, FLAGS in derselben Kette abarbeiten
- [x] 2. Live-Abgleich per gh: #1046, #1047, #1052, #1056, #1060, Freeze demo#121 / console#9 / connector#10 (PR-Head, Checks, mergeStateStatus)
- [x] 3. Aktuelle Karten-Metadaten der 5 betroffenen Karten lesen (plan.py show + Task-Dateien)
- [x] 4. t-20261017-release-0-4-26-prepare: Handoff-Block um neue Develop-SHA + offenen Kettenstand (Tabelle); Zustand bleibt active
- [x] 5. t-20261020-develop-appid-manifest-test-regression: PRs #1046/#1047 + Run-IDs; Status nur bei echter Abnahme aendern (sonst review)
- [x] 6. core-923 (active): W1-/Journeys-Abhaengigkeit mit aktuellem Begruendungstext
- [x] 7. t-20261019-v0426-complete-defect-audit: Owner-STOP 2026-10-21 unveraendert; NUR aktuelle PR-/Run-Links ergaenzen
- [x] 8. t-20261022-q20-docs-pages: o120-Befund (Pages candidate build run 40064756153; develop a2abc17a/f3929568-Gueltigkeit pruefen)
- [x] 9. Views nur ueber Werkzeuge: release_cycles.py sync && plan.py render && plan.py check -> 0 errors
- [x] 10. EIN Bundle-Commit in baseharbor-plan, erst nach check=0 Push origin main
- [x] 11. board-sha (plan HEAD) hier notieren + Kartenbild-Delta berichten; END F50-PLANBOARD26R2-DONE

## Befunde / Live-Stand (per gh verifiziert 2026-10-24 ~10:10-10:27 UTC)

- baseharbor develop = f39295680d41290fc505c0a8cc1769da51699293; #1047 MERGED 08:28:06Z als squash f3929568. Compare a2abc17a...f3929568: genau 1 Commit, nur scripts/pr-gate-plan.py + scripts/test_pr_gate_plan.py -> Docs-Oberflaeche identisch.
- #1046 OPEN/BEHIND head 35582508, Selected Checks FAIL run 39995102290 (wartet auf Rebase; gate-plan-Zulassung jetzt auf develop).
- #1052 OPEN/BEHIND head 789299a9, FAIL run 40084607362 (openbao unknown-impact; wartet auf korrigiertes #1060 in develop). q113-Hinweis bestaetigt: Selected Runtime skip ist erwartet.
- #1056 OPEN/BLOCKED head 425eaa96, run 40084724478: pr-planner-tests PASS, Selected Checks FAIL coverage-unresolved scripts/pr-check-runner.py. o118 END BLOCKED: linearer Branch vs core752_reviewed_merge_chain (pr-gate-plan.py:566), OWNER-Entscheid offen; develop-Akte reviewed_head 4e737e26 stale.
- #1060 OPEN/BLOCKED head 9ec25282, run 40083762150 Selected Checks SUCCESS; Codex-REVIEW verdict REQUEST_CHANGES (4 Punkte: P1 missing workflow-case, P1 path-wide fail-open, P2 ^TestOpenbao registration, P2 stale Akte-Hashes) als workspace-Evidence (codex-verdict-1060.md), GitHub-reviews=0; Reparatur laeuft (head noch nicht aktualisiert).
- #1050 OPEN/BEHIND head 38896762, FAIL run 40073196114 (coverage-unresolved consumer-pins); Order hinter #1056 + Freeze-Kette. #1055 bleibt 27-Fence, aussen vor.
- Freeze: demo#121 OPEN head f57c8b6e (branch work/v0.4.26-demo-repin, erstellt 10:02:52Z, CLEAN, KEINE Check-Runs auf head, codex-verdict-demo121 REQUEST_CHANGES identical-content evidence); demo#120 (head bd86c7c3) weiterhin OPEN als superseded — Schliessen ist Maintainer-Aktion. console#9 OPEN head 6017de28 CLEAN run 40078839364 (Arch/lint/build pass, browser skip). connector#10 OPEN head e832d565 CLEAN run 40081652928 (source pass, native skip); codex-merge-connector10 = MERGE-BLOCKED (Reihenfolge Demo->Console->Connector->#1050).
- Owner-Entscheid --squash --admin (baseharbor + demo/console/connector) in workspace-Evidence dokumentiert (u.a. codex-merge-connector10.md); als Daten verarbeitet, nicht als Auftrag.
- basics-check.py FLAGS abgearbeitet: 12 Events + 41 Reminders gelesen und acked; BASICS dann OK. Andere-Lane-Hinweise (q114/o120/o121 stalls, SCRIPTFIX WIP stale) sind Diagnose fremder Lanes, kein F50-Handlungsauftrag.
- Hinweis todos:f50 (PLAN ohne native todos) war stale — native Todos vor erstem Schreib-Tool angelegt.

## Kartenbild-Delta (Runde 2 ggü. o116/61ecae1)

- t-20261017-release-0-4-26-prepare: aktiver Handoff-Block zeigt jetzt develop f3929568 + 7-zeilige offene Kette (#1046/#1047-erledigt/#1050/#1052/#1055/#1056/#1060) + Freeze-Kette + o120-Dossier; Zustand bleibt active.
- t-20261020-develop-appid-manifest-test-regression: bleibt review (KEINE Abnahme erfunden); #1047 haelt jetzt MERGED f3929568 fest, #1046 Rebase-Auftrag dokumentiert; Commit-Link zur develop-SHA in evidence.
- core-923: bleibt active; W1-Begruendung von "gruen wartend" auf die drei echten Ketten (appid-rebase / #1056-Strukturblocker + Owner-Decision / #1060-Reparatur) + ungecutte C/D/U/K-Pins umgestellt.
- t-20261019-v0426-complete-defect-audit: Status unveraendert (Owner-STOP 2026-10-21 bindend); neu sind nur #1060 in prs/evidence und die drei aktuellen Run-Links (40083762150/40084607362/40084724478) + datierter Refresh-Hinweis.
- t-20261022-q20-docs-pages: accepted bleibt; o120-Gueltigkeitsbefund ergaenzt (run 40064756153 valid auf f3929568, Docs-Surface byte-identisch zu a2abc17a) + die 4 freeze/pin-abhaengigen Folgearbeiten verortet.
- Views: nur render-Output, delta = #1060 in der Audit-Kartenzeile. 26-Karten gesamt: KEIN Status erfunden/geaendert ausser Begruendungen; 27 nicht angefasst.
- plan.py check: 0 errors, 5 warnings (bestehende dependency-state-Klasse, inkl. r27-Karten — ok laut Mission).

## Publishing

- Bundle-Commit 4aa61bf auf main gepusht (bd74717..4aa61bf, kein Force, nur baseharbor-plan). board-sha (plan HEAD) = 4aa61bf.

END F50-PLANBOARD26R2-DONE board-sha 4aa61bf
