# GX1 – ROOT-HYGIENE-DOSSIER (NUR lesen + dokumentieren, LOESCHEN verboten)

Erstellt: 2026-10-28, ornith Worker, Lane gx1.
Auftrag: evidence/board-2026-10-22/prompts/gx1.txt. NUR lesen + dokumentieren.
KEIN git rm/checkout/reset im Root, kein gh-schreib. Alle Vorschläge nicht ausgeführt.

## (a) git status --porcelain im ROOT

Root-CWD: /home/marcel/dev/baseharbor-ecosystem. HEAD nach Status-Lauf von 603e994c auf 2516b6f7 verschoben (jemand hat zwischen durch committet).

Modified (M), 28 Einträge:
- 4 Produkt-Submodules: baseharbor, baseharbor-console, baseharbor-demo, baseharbor-plan (Submodule-Pointer zeigen auf lokale/andere SHAs = unveröffentlichte Arbeit in den Subrepos).
- 1 Cross-Board-Evidence: evidence-r27/w4a.md (modified, non-standard location).
- 6 Script-Artefakte unter evidence/board-2026-10-19/scripts/ (.coord-state-cache.json, board-currency.json, events-ack.json, gateway-latency.json, reminders-ack.json, reminders.log.generation, tmp-sweep.log) — laufende Coordinator-Daemon-Zustands-/Log-Dateien.

Untracked (??), 86 Einträge:
- evidence-r27/: bx2.md, pcx.md, rv9.md, stg3.md, stg4.md, w4b2.md (+ nicht-tracked: stg4/ dir, ci1-repair.md, cy1.md, cy3.md, f1-fix.md, f3b-fix.md, rv3.md, rv8.md, s2-v3staging.md, stg2.md, stg6.md, w1.md, w3.md, w4b3.md, x1-fix.md).
- evidence/board-2026-10-19/scripts/: reminders.log.00000000000000000002 (rotierende Log-Generation, untracked).
- evidence/board-2026-10-22/prompts/: ~50 .txt-Promptdateien (b3,b4,bx1-bx3,bz1-bz3,ci38b,cmx-cmx3,codex-r27v4/v5, d3-d5,doc1,f1x,fx1,gx1,nt1,p1,p2,p2b,pcx,pl1,pl2,q2,r5i,rv4-rv9c,sr1,stg3-stg6,tb1,vb,w4b2,w4b3,w5) — alle Lane-Prompt-Ausgaben der board-2026-10-22 Serie.
- evidence/board-2026-10-27/: b3-board.md, cmx-1142.md, cmx3-merge-attempt.txt, cmx3.md, d3-mergecheck-v36.md, d4-ciwave4.md, d5-ci37.md, p1-pilot-prereq.md, p2-pilot-prep.md, q2-boardpatch.md, r4-verdict-copy.md, sr1-shadow-plan.md, tb1-trusted-base.md, vb-boardcheck.md.
- neu seit Status-Lauf: evidence/board-2026-10-28/ (Board-Arbeit von ~00:2xZ) und evidence/r27/rv7c.md/ (Verzeichnis — vermutlich fehlgeschlagener/nested commit).

## (b) Zuordnung je Gruppe (Lane/Incident)

### 1. Produkt-Submodule (M baseharbor /-console /-demo /-plan)
- Beleg: Standard-Submodule-Drift. baseharbor-plan ist ein eigenes Git-Repo (evidence-r27/w4a.md + PLANRELEASE belegen ständige Plan-Arbeit). Die anderen zeigen auf lokale SHAs, die noch nicht auf deren remote publiziert sind.
- Einordnung: Normale "out of sync"-Anzeige bei aktiver Submodule-Arbeit; kein einzelner Incident. Plan-Remote f8157ec laut coordinator-plan (25.10.) als aktiv dokumentiert.

### 2. evidence-r27/ (untracked + w4a.md modified)
- Beleg: evidence-r27/ ist ein versionierter Pfad (README: "evidence in R27, hierarchisch getrennt"). w4a.md ist dort modified; bx2/pcx/rv9/stg3/stg4/w4b2 sind frische R27-Evidence-Stücke.
- Einordnung: R27-Evidence-Produktion (Staging-Serie stg2/stg3/stg4/stg6, Belegsetzte rv8/rv9, w4-Fälle). Keine Lane im ops/missions/ greifbar; gehören zur r27-Dokumentationskette. stg4/ als Verzeichnis untracked = möglicher fehlgeschlagener Commit/nested-dir.

### 3. evidence/board-2026-10-19/scripts/ (modified + untracked log)
- Beleg: Laufende Coordinator-Betriebsdaten (.coord-state-cache.json, board-currency.json, events-ack.json, gateway-latency.json, reminders-ack.json, reminders.log.generation, tmp-sweep.log, reminders.log.*).
- Einordnung: Live-Daemon-Zustand/Logs (Watcher/Reminders/tmp-sweep). tmp-sweep.log war in coordinator-plan als OPS-Befund erwähnt. Kein Produkt-Code.

### 4. evidence/board-2026-10-22/prompts/*.txt
- Beleg: Prompt-Ablage der board-2026-10-22-Serie (gx1 selbst inkl.). ops/missions/ enthält keine Treffer auf diese Namen.
- Einordnung: Archivierte Lane-Prompt-Ausgaben (Daten/Evidence). Nichts davon aktive Mission; gx1 ist das aktuelle Arbeitsblatt.

### 5. evidence/board-2026-10-27/*.md
- Beleg: Frische Evidence des board-2026-10-27-Tages (b3-board, cmx3-merge-attempt, d3-mergecheck-v36, d4-ciwave4, d5-ci37, p1/p2-pilot, q2-boardpatch, r4-verdict-copy, sr1-shadow-plan, tb1-trusted-base, vb-boardcheck).
- Einordnung: Tages-Evidence diverser Lanes (cmx/merge, d-series CI, pilot, q2 board-patch, r4 verdict, tb1 trusted-base). Keine destruktive Aktion nötig; laufende Dokumentation.

### 6. evidence/board-2026-10-28/ + evidence/r27/rv7c.md/
- Beleg: Seit dem letzten git status neu entstanden (~00:2xZ bzw. nested dir).
- Einordnung: Very-frische Arbeiten des laufenden Tages. rv7c.md/ als Verzeichnis = Verdacht auf fehlgeschlagenen/nested git-Commit im r27-Pfad.

## (c) Vorschlag je Gruppe (loeschen | committen | behalten)

1. Produkt-Submodule (4x): **BEHALTEN.** Out-of-sync-Pointer = unveröffentlichte Submodule-Arbeit. Kein Root-Cleanup — in den jeweiligen Subrepos committen/publizieren. Kein git rm/ignore im Root.
2. evidence-r27/ (w4a.md modified + untracked Stacks): **BEHALTEN / in R27 committen.** Versionierter Evidence-Pfad; frische R27-Stücke gehören dort hin. stg4/ nested-Verzeichnis: zuerst prüfen, ob beabsichtigt (keine Auto-Aktion).
3. evidence/board-2026-10-19/scripts/ (Zustand/Logs): **BEHALTEN.** Laufende Daemon-Betriebsdaten; Löschen würde Coordinator-Zustand/Log-Kontinuität brechen. Nur falls explizite Owner-Cleanup-Regel.
4. evidence/board-2026-10-22/prompts/*.txt: **BEHALTEN** (Archiv) — oder in eine einzige Bundle-Publishing-Action committen. Kein einzelnes Löschen ohne Owner-Go; könnten als Lane-Nachweis benötigt werden.
5. evidence/board-2026-10-27/*.md: **BEHALTEN / committen.** Frische Tages-Evidence; dokumentarisch. In normalem Bundle committen (kein manuelles Edit der generierten Views).
6. evidence/board-2026-10-28/ + evidence/r27/rv7c.md/: **BEHALTEN + rv7c.md/ VERDACHT dokumentieren.** Neu/frisch; rv7c.md/ Verzeichnis auf fehlgeschlagenen nested-Commit prüfen (git status -uall / git add -N), erst dann entscheiden.

## (d) worktrees-Verzeichnis — noch gemountete Branches

`git worktree list` (11 Einträge):
- baseharbor-ecosystem (root)            2516b6f7 [master]
- worktrees/f4c                          66555394 [work/r27-fix-g45-linkage-v3]
- worktrees/r1b                          ad4d0f28 [work/r27-correction-candidate]
- worktrees/w2-writer                    bb2ed8c8 [w2/r27-writer]
- worktrees/y1-shadow-probe              af57a26d [y1/shadow-probe]
- /home/marcel/worktrees/bz1             64098158 [work/r27-belegsetz]
- /home/marcel/worktrees/f3              38771771 [work/r27-fix-g3-publication]
- /home/marcel/worktrees/stg6            94f35d02 [work/r27-correction-v38]
- /home/marcel/worktrees/tb1             3431b22c (detached HEAD)
- /home/marcel/worktrees/w4b             2e137e45 [work/r27-fix-f2-akte2]
- /home/marcel/worktrees/w4b2            2e137e45 (detached HEAD)
- /tmp/opencode/mt-a1i                   433a5702 (detached HEAD)  ← externer /tmp-Worktree (opencode)

Beobachtet: mehrere worktrees hängen an r27-Fix-/Staging-Branches (f4c, r1b, w2-writer, y1-shadow-probe, bz1, f3, stg6, w4b). tb1 und w4b2 stehen auf detached HEAD. /home/marcel/worktrees/* liegen außerhalb des Root-Worktrees (externe Pfade). /tmp/opencode/mt-a1i ist ein opencode-Worktree, kein Produkt-Worktree.

## Fazit
Alles oben genannte ist entweder aktive Produkt-/Submodule-Arbeit, laufende Daemon-Betriebsdaten oder frische/archivierte Evidence — nichts ist eindeutig als weggeworfen zu klassifizieren. **Keine destruktive Aktion ausgeführt** (Auftrag nur-lesen). Bei Cleanup: Bundle-Approach + Owner-Go, rv7c.md/ zuerst auf nested-Commit prüfen.

END GX1-DONE
