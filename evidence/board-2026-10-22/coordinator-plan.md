# Coordinator PLAN (live, 2026-10-23 00:4x)
## Critical Path C2 -> D2 -> W1
- [x] Watchdog + spawn guards + reminders (getestet, im Dienst)
- [x] T1 #1038 code-seitig PASS (f10 Review), T4 #1039 ready, Selector-Fix #1040 MERGED -> develop 281d1ae0
- [x] #1038 gemergt (C2=2f005813); #1039/#1042 Selector final in f14-Runde2
- [ ] Codex-MERGE-Welle #1038 + #1039 -> C2 definieren (develop-Tip)
- [x] NOTESFIX=#1041 im Merge, #118 MERGED (main 82d042b)
- [ ] D2 minten + im Board vermerken
- [ ] W1 Rerun (f9-Sheet, consumer_ref="", ref=develop+resolve statt kahl-SHA laut o33; F12-Patch abwarten)
- [ ] W2 gezielte Reselection aus W1-Fehlern
## Nebenpfade
- [x] o28 CMD-A 46 Zeilen (run 39865161687) - Ergebnis in W1-Aggregat aufnehmen
- [x] o29 NOTESQA fix-liste -> in o30 übergeben
- [ ] q25 PROVENANCE Befund einarbeiten (release-approved.json/coverage)
- [ ] q26 #118 CI-Verdict einarbeiten
- [ ] o26 END-Marker nachziehen (Nudge steht aus)
- [ ] Consumer #7/#8 Merge-Fenster nach D2 (U/N final auf C2/D2 prüfen: q24 Inputs)
## Abschluss
- [ ] Prepare-Karte + Codex-Review-Welle nach W1
- [ ] Jeder Meilenstein sofort auf Karten publizieren (Regel, Watchdog prüft)
- [x] DISZIPLINE ab jetzt: Missionen NUR noch via dispatch-lane.sh (spawn+sofort-send = Ghost-Quelle)
- [x] q32/o38: Repro rekonstruiert + #118-Head als ECHTER FAIL überführt (letzte-Row-Bug)
- [ ] o39: #118 owned_containers Row-Fix, Repro exit0 als Gate
- [ ] f14: Selector-Einträge #1039+#1042 grün
- [ ] o40: #118-Runde2 Set-Gleichheit (Codex MERGE-118-G Invariante), beide Repros exit0
- [ ] f14: Selector gruen #1039+#1042
- REGEL: jeder Codex-Befund = Repro/Rezept -> sofort Worker-Auftrag, keine Runde ohne Gate
- [x] #1041 merged -> develop 9f4916be (C2-final pending S39/S42/Q)
- [x] S39 gefroren: 0de29deb (#1039 source-prep)
- [x] S42 gefroren: 267152e8 (Evidence nach Nudge komplett)
- [x] f17 fertig(1-10)+rm, q39 uebernimmt 11-14 (audit fixtures) busy
- [x] tmpfs-Root-Cause: DB-Kopien in /tmp (fixed direct-ro) + Tripwire 1500MB
- [x] codex danger-full-access (YOLO) gesetzt
- [ ] nach q39: daemon restart, o46/q38 nachholen, dann C2-Kette: S39 fe17e6e5 S42 c34740fa -> Q-PR -> merges -> develop -> D2 pin (demo main 82d042b) -> W1 (consumer_ref=40hex tips! nicht leer)
- [ ] prepare/qualify naechste: codex-verdicts-2026-10-23.md 4x CHANGES, Rezepte in Arbeit
- [x] o57: core-923 W1-Read-only-Vorbereitung DONE (evidence o57-core923.md; Pins/Consumer-Flaeche verified gegen C2 1861ed07 + demo d2-pin f4c0806; Journeys/Semantik/Matrix = needs-W1-run; A1 release-Branch + A3/A5 pin-Files fehlen noch, P0-4 #7/#8 offen)
OPERATION: never push plan changes before check=0 (run check, on error fix+amend, THEN push) - codify in future dispatch prompts.
REGEL (Owner 2026-10-23): 5 Worker-Slots sind Dauer-Kontingent; bei frei werdendem Slot sofort naechsten Auftrag dispatchen (keine Idle-Slots, keine Scheinauftraege).

REGELN 2026-10-23 (Owner, verbindlich): 5 Worker-Dauerkontingent, Luecke sofort fuellen; faire Modellreihenfolge ornith>qwen>flash bei gleicher Eignung; Batch-Spawn => verify-delivery liefert ambiguous-window => Pane-Beweis ist verbindlich, nie Blind-Resend.
| 2026-10-23T15:56:34Z | lanes o56/o62/q49/f24 | backend qwen healed 1.1s | D2 run pending o56 |
- 27-Kette: f24+o64 Research done; f25 console-2 Umsetzung (PR folgt); o65 Stale-PR-Cleanup; q50 OPS-4/5; o56 Podman2
Kommunikationsregel (Marcel 2026-10-23): Bei Karten-Status IMMER die baseharbor-plan Karten-ID nennen (z.B. t-20261020-develop-appid-manifest-test-regression), nicht Lane-Kuerzel. Lane-IDs nur auf explizite Nachfrage.
