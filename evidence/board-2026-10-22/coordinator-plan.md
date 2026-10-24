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
- Review-Welle 26: c7n8 ACCEPT + MERGE laeuft; docs/appid PRs in Runde 2; D2 done; W1-Input final ueber o74\n

ROLLENKLARTEXT (Owner 2026-10-23): Coordinator fuehrt KEINE Produkt-/CI-Kommandos selbst aus (kein gh run/workflow dispatch, keine Merges) – alles ueber Worker-Lanes oder Codex-ROLL-Session. Worker-Lanes starten ihrerseits keine anderen Sessions/Explorer.ANKER-CHECK vor compact: dieser Text steht in evidence/board-2026-10-22/coordinator-plan.md (Regelblock + F29/o78/q57 Queue) und HANDOFF.md im plan-repo. Endgame: D2 gruen 25c3196, develop 1861ed07, W1 wartet auf o78-PR-Merge dann f29-Dispatch (--ref develop + alle 4 inputs 40-hex!), danach prepare/qualify, Publish nur mit Marcel-GO, 27 pausiert.
26-FENCE (Owner 2026-10-23): Marcel macht 0.4.27 parallel selbst. Finger weg von 27-Karten/PRs. W1 ref/Resolver core = IMMER 1861ed07 fest, develop-move unerheblich. Planning: nur 26-Karten editieren.

BRANCH-ONLY Regel (Owner): keine Merges in develop/main, alles auf eigenen branches; W1 ref fix 1861ed07.

INCIDENT o85 (2026-10-23): Lane fuehrte git checkout -b + commit IM OPS-ROOT-REPO statt baseharbor aus (branch-Name kollidierte, ganzer Tree auf Core-Inhalt). Konsequenz: alle kuenftigen Missionen erhalten die Zeile "ALLE git-Arbeit NUR in /home/marcel/dev/baseharbor-ecosystem/baseharbor[-plan|-console|-node-connector|-demo], NIEMALS im Root-Repo". Recovery: master-Checkout, Branch geloescht, 39/39 Dateien + Daemon ok.

WORKTREE-PFLICHT: alle Produkt-Missionen nutzen git worktree add <pfad> -b <branch> <base-sha>; main-Worktrees NIE switchen (Kollisionsquelle, o88-Fund).

INCIDENT#2 (q87/q88): git checkout -b IM ROOT statt baseharbor (cwd-Rutsch) -> root auf core-branch. Recovery: checkout master, branch geloescht. NEUE MISSIONS-REGEL: Prompt-Prafix "pwd-PFLICHT: vor jedem git-Befehl pwd pruefen; in /home/marcel/dev/baseharbor-ecosystem ROOT ist KEIN git erlaubt (kein -C-Vergessen)."
LAN-REGEL: pro branch nur EINE lane - #1056-Kollision (f36+o95) koordinativ geloest (f36 stop, o95 uebernimmt remote-spitze).

SELBSTKOMM (23:5x): AGENTS.md 13-19 enthaelt bereits Vollauslastung + no-sleep/Delegationspflicht - Regel ignoriert statt Regel fehlend. Abhilfe: vor jedem "sleep"-Impuls -> events-check + Basis-Logik; Idle-Slot sofort mit naechstem Inbox-Auftrag (keine Diskussion mit sich selbst).

MISSION-PFLICHT (ab sofort): Auftragsdateien liegen in ops/missions/, NIE unter evidence/ (worker-reachable = injection-attack-surface). Worker-Hand-Offs nur ueber Karten/Evidence-Eigaenge.

INCIDENT#3 (o112): Root-git erneut beschlagnahmt. Neue HART-Regel: Missionen bekommen ein Prefix "FALLS pwd ROOT: cd baseharbor PLAN SOFORT". Und ich starte keine root-git-Erlaubnis-Lane mehr ohne wtx-Zwang.

RULESET-ERKENNTNIS: required_status_checks als aktives Ruleset (id 25223521) auf develop - nicht bypassbar ohne Owner-Sonderanordnung, Bypass verboten per AGENTS.

## COORD-INHIBIT 2026-10-24 07:35-08:05Z (neuer Coordinator adebf29e, Qwen Flash/Gateway)
- Alter Coordinator 73202350 wurde 09:04:39 local per `user_delete` entfernt (Session+Pane weg). Wiederherstellung: Letzter持久 Stand = coordinator-plan.md (mtime 08:59, Root-Commit cf4280bb) + codex-verdict-1050/-1052 (08:46). Transcript nicht mehr vorhanden (server 404), OpenCode-DB liefert fuer diese Sessions keine message-Zeilen.
- Pool-Hygiene: /tmp Sweep >24h (612M->1.9G frei). root-eigene /tmp/baha-*-bin (4x ~59M) NICHT loeschbar (root) -> DISK-Flag bleibt grenzwertig, kein Spawn-Blocker bisher. Root-Repo hat KEINEN upstream (origin zeigt aufs Produkt-Repo!) -> ops-Commits bleiben lokal, NIE pushen.
- WATCHDOG-PFLICHT erfuellt: basics-check, events-check --ack (2x), Daemon mit eigener ID neu gestartet.
- BEFUND (kritisch): Codex-Daemon-Selbst-Wake war toend - `codeman agent send <eigene-session>` wird verweigert ("is me", rc=4) -> 237 Reminders nur als "[send-failed, will retry]" gelandet. Fix in wait-workers.py _send_wake: self-target gilt als zugestellt (Log-Pfad ist der Kanal). 78 Ops-Tests (test_f17_fixtures + test_o50_autodiscover) OK.
## REALITAETSABGLEICH v0.4.26 (P0, 2026-10-24 ~07:40Z)
- develop a2abc17a (#1053 #1058 #1059 gemergt). W1-Anker bleibt 1861ed07 (26-FENCE). v0.4.27-Karten/PRs (#1048 #1051 #1055) NICHT angefasst.
- Blocker-Kette A (pins): #1050 rot "unknown impact docs/releases/v0.4.26.consumer-pins.json". DER echte Fix existiert nur lokal: 3c02f48c auf o90-planner-pr (planner area pins-data + VALIDATION pins-data-validation) und fehlt im Live-Head df8e53b9 von #1056 -> #1056 wurde ohne den Kern-Commit gerebased.
- Blocker-Kette B (openbao): #1052 rot "unknown impact internal/openbao/bootstrap{,_test}.go" (planner else-Zweig ~:1219) + Codex REQUEST_CHANGES (4 Rezepte in bootstrap_test.go).
- Blocker-Kette C (appid): #1047 gruen (run 39998286306) aber keine Approval -> mergefaehig erst durch Codex-Maintainer; danach #1046 rebase.
- Bestehende Arbeit uebernommen statt neu gestartet: f46 hatte Rebases 1050@38896762 und 1052@736d73c6 bereits gepusht (merges=0, ruleset-Lehne korrekt) -> Lane auf_docs-audit umgestellt. o113 war fertig (board-sha b05ac3b) -> Lane auf Kette-A gesetzt.
## DELEGATION (Status 08:05Z)
- [x] o114 (ornith, Session 9348b449 formerly o113): pins-data-Commit 3c02f48c in #1056-Head wiederherstellen + Akte-hash + Tests. ops/missions/o114.txt
- [x] q110 (qwen 51337f62): neuer kleiner CI-PR "qualify internal/openbao paths" (Planner+Runner+ci.yml+Akte+Tests). ops/missions/q110.txt
- [x] q111 (qwen 166e39f5): #1052 bootstrap_test.go nach CX-1052 Rezept 1-4, worktree o91-fix. ops/missions/q111.txt
- [x] cmx1047 (codex 116ae0ba, ROLLE MERGE): Live-Gates + Approval + squash merge #1047, post-merge develop-Check. ops/missions/cmx1047.txt
- [x] f46b (flash 268b4825): Stop aller Merge-Aktionen (bestaetigt STOPPED), docs/release-surface Dossier nur lesend. ops/missions/f46b.txt
## NAECHSTE SCHRITTE (nach Lane-END)
- [ ] Nach #1056-Gruen: #1050 rebase auf neuen develop + Atomic-Gates (KEINE Full-Matrix), unabhaengiges Review, Merge ueber Codex-Lane.
- [ ] Nach q110-PR gruen: #1052 rebase (q111-Aenderungen enthalten) -> Gates -> Review -> Merge.
- [ ] Nach #1047-Merge: #1046 rebase -> Gates -> Review -> Merge (appid-Karte t-20261020-develop-appid-manifest-test-regression).
- [ ] W1-Qualifikation: vorhandene gruene Evidence gegen Pins validieren lassen, nur fehlende Atomic-Gates beauftragen (f29-Muster: --ref develop + 4 inputs 40-hex, Core-Anker 1861ed07).
- [ ] Ecosystem-Freeze (Core/Demo/Console/Connector SHAs) + Doku-Audit EN/DE/Pages/Changelog -> Prepare-Karte t-20261017-release-0-4-26-prepare.
- [ ] Planning/CardBoard: 26-Karten ueber Prepare-Kette nachziehen lassen (o-Kette), nur 26.
- [ ] Konflikt-Doku: Pausevermerk 2026-10-18 (Maintainer-Handover-Pause) in t-20261017-release-0-4-26-prepare ist ueberholt durch Owner-P0 2026-10-24; t-20261019-v0426-complete-defect-audit bleibt Owner-STOP 2026-10-21 -> ZWINGENDE OWNER-ENTSCHEIDUNG vor Release-Freigabe (Release-Readiness haengt an dieser abhaengigen Karte).
- 08:10Z o115-BEFUND (W1): Root-Cause pre-release.yml:117 (consumer-pins.json fehlt auf develop) + :122 (Core-Pin-Mismatch a2abc17a != 1861ed07). evidence-runs/selected-gates-Entwurf ist blocked-on-W1, es gibt noch KEEN gueltigen gruenen v0.4.26-Run. Gate-Katalog + reuse-Liste liegt in evidence/board-2026-10-24/o115-w1evidence.md. Pins-Kette 4/4 als reale Commits verifiziert (Core 1861ed07, Demo 25c31969, Connector df1f4378, Console 69d1d14).
- 08:12Z SYSTEMBEFUND (beide Ketten): Jede Lane, die scripts/{pr-check-runner,pr-gate-plan,test_pr_gate_plan}.py aendert, braucht einen Trusted-Binding-Akte-Eintrag auf ihren FINAL-Head. Hash-Drift durch Rebase/Push macht 'PR Selected Checks' rot mit 'coverage-unresolved: CI execution/evidence impact ... scripts/pr-check-runner.py'. Betroffen: #1056@7a220d26 (Koord-Hinweis an o114 gesendet) und #1060@3fa4989e (q110b repariert). Regel kuenftig in jede CI-Script-Mission schreiben.
- 08:14Z Delegation: cmx1047b (Codex: #1047@40351fd3 approve+squash-merge, Rebase-war-inhaltlich-leer-Pruefung), o115b (Ornith: OEkosystem-Freeze-Dossier 4 Repos), o116 (Flash: Planning/CardBoard 26 laufend). Alle vier Worker + Codex beschaeftigt.
- 08:20Z OWNER-ENTSCHEIDUNG (Marcel, ausduecklich): "Nutze den bereits bestehenden Owner-Bypass in Ruleset 23754541 (`mcpdev80`, `pull_request`) fuer v0.4.26." Verifiziert: bypass_actors=[{22537373=mcpdev80, User, pull_request}], approvals=1, squash-only.
  KONFLIKT-DOKUMENTATION: AGENTS.md "kein Schutzregel-Bypass" vs. neue Owner-Freigabe. Handel gemaeß Owner-Entscheidung (Freigaben sind Owner-Kompetenz), aber NUR uber den bereits existierenden per-PR-Bypass - kein Ruleset-Edit, kein --admin, kein Force, kein zweiter Account. Jede Merge-Evidence muss das Zitat + API-Nachweis enthalten.
  Betroffene Kandidaten: #1047 (freigegeben, Codex Runde 3 laeuft), danach #1056/#1050/#1052/#1060 jeweils nach Gruen+Review.
- 08:22Z Betrieb: o114 bei 75% Kontext (Compact-Schwelle), keine ausstehenden Permission-Dialoge im Pane; Akte-Reparatur laeuft. Dispatch-Pfade kuenftig root-absolut (relative ../ops/... Pfade erzeugen exit-1 Umwege).
- 08:26Z OWNER-FREIGABE ERGANZT (Mechanik): "**Freigabe: Option 1 - `gh pr merge --squash --admin`.**" Hintergrund: der konfigurierte bypass_mode=pull_request greift beim CLI-Merge ohne --admin nicht (GitHub: "base branch policy prohibits the merge", belegt in codex-merge-1047.md RUNDE 3). mcpdev80 permissions.admin=true (API-verifiziert). Scope-Bindungen fuer jede Anwendung: nur v0.4.26-Kandidaten mit gruener Mandatory-CI auf dem finalen Head + dokumentiertem unabhaengigem Review + Freshness; pro Merge Owner-Zitat + API-Nachweis in der Evidence; kein force-push, keine Ruleset-Aenderung, kein zweiter Account. Merge-Lauf: ops/missions/cmxwave.txt (Reihenfolge #1047 -> #1056 -> #1050 -> #1060 -> #1052).
- 08:4xZ OWNER-REGEL NEU (Marcel): fertige Worker-Lanes NICHT wiederverwenden - Session loeschen (`agent rm` + `watch remove`) und frische Lane spawnen, sonst wachsen die Kontexte (o114 war bei 89 %, q110 65 %). Umsetzung: q110->rm+q112 (qwen), o113/o114->rm+o117 (ornith), codex-1047merge->rm+codex-merge26.
- BEFUND o114 (END O114-BLOCKED, Root-Cause Kette 1): Planner liest die Trusted-Binding-Akte aus der TARGET-BASE (develop). Auf develop steht ein Block mit reviewed_head=4e737e26, der KEIN Ancestor von #1056 head 7a220d26 ist -> register None -> 'coverage-unresolved ... scripts/pr-check-runner.py'. #1060 (head 7a3c4520) ist dagegen COMPLETE GREEN (Selected Checks pass, run 40078059133). o117 klaert die Register-Semantik und liefert ggf. einen separaten docs-only Akte-PR (Muster #1058).
- FREEZE-Kette: Worker lieferten demo#120 (bd86c7c), console#9 (6017de2, green 40078839364), connector#9 (2223506c, source pass 40078538425). Abweichung dokumentiert: connector 7c7093d ist NICHT Vorfahr von df1f4378 (diverged) - pins.json nutzt df1f4378; nach den Merges muessen die 4 Pin-Felder in #1050 auf die finalen Main-Tips nachgezogen werden (cherry-pick SHA-Drift). Review laeuft (cmf1).
- MERGE-STATUS: #1047 GEMERGED -> develop f3929568 (Owner --admin-Freigabe, Evidence RUNDE 4). Naechste Merge-Kandidaten: #1060 (gruen, Freshness pruefen), danach #1056 (nach o117), #1050, #1052, #1046.
- OPS-DEFEKT (behoben): Daemon erzeugte pro Zyklus hundertfach 'PANE observation UNKNOWN' fuer Ghost-Registrations (417 unread). wait-workers.py: _prune_ghosts mit GHOST_GRACE=600, gated auf bereits beobachtetes 'gone' (unterdrueckt kein echtes gone-Event). 78 Ops-Tests OK, Daemon neu gestartet.
- 08:5xZ FREEZE-REVIEWS (cmf1, END CX-FREEZE-REVIEW-DONE): demo#120 REQUEST_CHANGES (Diff enthaelt ausführbare Timeout-Aenderungen, kein reines Pin-Change; kein exact-candidate CI) -> cmf2 klaert minimales Re-Pin-Rezept; console#9 ACCEPT_WITH_NONBLOCKING (nur aktueller SHA, nach Demo-Merge repin + frische Review noetig); connector#9 REQUEST_CHANGES (integration-candidate.json:4 falsche Core-SHA, kanonischer Resolver verwirft) -> Korrektur an f48 uebergeben, PR #9 wird superseded.
- ROTATION: f46 (41% ctx, 3 Missionen) geloescht, f48 frisch (flash). q111 (#1052 transient class) laeuft, q112 (#1060 rebase+akte), o115d (#1046 rebase), o117 (Register/Akte #1056).
- OFFENER OWNER-PUNKT: Merges der Freeze-PRs zielen auf main von demo/console/connector. Bisherige Regeln: "--admin pro PR fuer v0.4.26-Kandidaten" (erteilt) vs. "keine Merges in develop/main" (BRANCH-ONLY). Freigabe fuer consumer-repo mains wird separat erfragt.
- 09:0xZ OWNER-FREIGABE ERWEITERT (Marcel, ausduecklich): `--squash --admin` gilt fuer ALLE drei Consumer-Repos (demo, console, node-connector) zusaetzlich zu baseharbor - Ziel: Ecosystem-Freeze auf deren main, damit pre-release.yml:117/:122 und W1 durchkommen. Umsetzung ueber die Merge-Lane, pro PR mit Owner-Zitat + API-Nachweis in der Evidence; weiterhin nur bei gruener Required-CI + unabhaengigem Review auf dem finalen Head.
## INCIDENT+RESSOURCEN-BLOCK 2026-10-24 (~09:15-09:35Z)
- INCIDENT#4 (root-git): q112 rebaste #1060 IM ROOT-Repo statt worktree -> root auf Branch work/v0.4.26-openbao-gateplan, ops-Dateien aus Working Tree verdraengt, REBASE_HEAD-Leiche. Recovery: WIP als stash 'rootincident4-q112-wip' gesichert, checkout master (8d42e20f), ops-Dateien+Skripte wieder da. Produkt-Historie NICHT kontaminiert (die 3 involveden Commits enthalten nur scripts/*, pruefbar: git show --stat). Alle Missionen bekommen jetzt die harte Zeile "WORKTREE-PFLICHT: pwd -P vor jedem git, Root-Repo fuer git VERBOTEN".
- RESSOURCEN-URSACHE (vom Owner nachgefragt, gemessen): used 12.4G = ~5.6G tmpfs /tmp + ~6.1G Prozess-RSS (opencode 3.8G, vscode-server 1.2G, node 0.4G, codex 0.25G) + Slab. Kein deleted-but-open. Groester Einzelposten: 569 versteckte /tmp/.<hex>-*.so (bun/V8-Payloads, je ~14 MB) = ~5 GB RAM. /tmp war 72-80% voll, MemAvailable <1G, kein Swap -> Kernel-Reclaim + cs-Sturm (cs ~80k/s) erklaeren Load ~20 bei nur ~3 Kernen Rechenarbeit.
- MASSNAHMEN (dauerhaft): TMPDIR der Lanes auf Disk (`tmux set-environment -g TMPDIR /home/marcel/tmp-ops`), scripts/tmp-sweep.sh mit Worktree-Schutzliste + bereinigtem .so-Muster, Auto-Sweep vor jedem Spawn (30-Min-Stamp) und staendlich vom Watchdog (1800s), basics-check bekommt RAM-Flags (MemAvailable<2048, TMPDIR auf tmpfs). Effekt: /tmp 1.3G used, MemAvailable 8G.
- POOL-FOLGE: q111+o115+o114+codex-alt wurden rotiert/geloescht; aktuell q112(repair), o118, f48, codex(cmf3). Volle Auslastung (5 Worker) ist wieder moeglich, Druck ist weg.
