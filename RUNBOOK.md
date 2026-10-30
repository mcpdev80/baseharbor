# BaseHarbor Ecosystem – operatives RUNBOOK

Kanonischer Ort: `/home/marcel/dev/baseharbor-ecosystem/RUNBOOK.md`
`AGENTS.md` definiert den Soll-Prozess und die Rollen (Coordinator vs. Worker); hier steht das WIE inkl. verifizierter Kommandos, gegliedert nach denselben Rollen. Stand: 2026-10-29.

## 1. Sessions & Pools

- Pool-Ziel: 1 Coordinator (Modell fest: Qwen Flash) + 5 Worker (1 Flash, 2 Ornith15, 2 Qwen3.6) = 6 Sessions; Codex-Abnahme-Lane separat. Alle Modelle ausschließlich über AgentGateway (`http://192.168.6.132:4000/v1`, Modell-IDs via `/v1/models`).
- Hardware-Zuordnung ist ZIELARCHITEKTUR (noch nicht verifiziert migriert): Coordinator + Flash-Worker → NVIDIA Spark; Ornith → AMD 295; Qwen3.6 → AMD-295-Laptop. Bis zur Verifikation nicht als Fakt behandeln. Host-CPU/RAM sind KEINE Spawn-Gates für Modellfragen; Gesundheit des Coordinator-Hosts (Prozess-Headroom der TUI-Sessions, freier RAM > 2 Gi als Daumengrenze für Prozesstätigkeit, Uhr-Sync) bleibt Betriebskriterium.
- Gemessene Gateway-Parallelität: 4-Stream tok/s flash 22,8 / ornith 23,2 / qwen3.6 15–19; VOLLLAST 6 simultane Streams (Pool 1 Coord + 5 Worker): 23,9 s Wandzeit, alle ok — qwen3.6 langsamster Kanal, kein Limit gefunden (2026-10-22).

## 2. Coordinator-Betrieb

### 2.1 Spawn + Dispatch (Pflichtpfad)

- **Spawn:** `evidence/board-2026-10-19/scripts/spawn-lane.sh <name> [--qwen|--flash]`
  - Modell-Overrides laufen NUR noch als Flag; intern flock-serialisiert (set→spawn→unset an `/home/marcel/.codeman/.spawn-lane.lock`) — keine überlappenden globalen tmux-Env-Änderungen mehr; kein manueller Env-Trick nötig. Root-Cause f8: geerbte Daemon-Env `OPENCODE_CONFIG` — Override zwingend in der flock-Sektion.
  - Auto-Compact: 200.000 Standard, 400.000 für die Flash-Konfiguration (Defaults; die effektive Schwelle nach Spawn gegen das vom Gateway gemeldete Kontextfenster verifizieren, vgl. AGENTS.md 5.5).
  - TUI-Readiness-Gate (Welcome-Erkennung) + automatische Watcher-Registrierung (Watch-Add kann nicht mehr vergessen werden).
  - POOL-Guard VOR Spawn (exit 4) + Identitäts-Guard NACH Spawn (exit 3, Lane bleibt); Modellidentität im Pane-Footer einmal verifizieren.
- **Auftrag:** Prompttext als Datei nach `evidence/board-2026-10-22/prompts/<lane>.txt` (arbeitsverzeichnis-lokale Pfade, /tmp/opencode ok), dann `evidence/board-2026-10-19/scripts/dispatch-lane.sh <id> <prompt-datei>` = Einzeiler-Pointer-Nachricht + 5s-Busy-Check + Auto-Resend + Auto-watch-add. Hintergrund: Codeman verwirft Multi-Line-Sends still; `--wait` blockiert bis Abschluss.
- **Nach jedem Erfolg:** `codeman agent rm <id>` + `wait-workers.py watch remove <id>`.
- **Lane unterbrechen statt töten:** `codeman agent interrupt <id>` (nur ESC). Raw `tmux send-keys C-c` beendet den OpenCode-Prozess (q18-Verlust 2026-10-22). Mehrzeilige Inhalte immer als Promptdatei unter `evidence/board-2026-10-22/prompts/`.
- **Watchdog-Layer (freigegeben 2026-10-22):** `wait-workers.py` prüft alle 60 s Pool-Belegung nach echtem Modell (Pane-Header `Build · …`, Coordinator/codex ausgenommen) und mappt `basics-check.py --json`-Flags in deduplizierte Reminders (Cooldown 15 min, kritische sofort) via Queued-`agent send`; Skripte erinnern nur, entscheiden nie. Compact-Erfolg = Kontext unter Schwelle ODER Message-Event plus Rückgang (kein starrer 20 %-Abschlag); Delegate-Reminder nur bei bekanntem ausführbarem Task (`plan.py list --ready` unclaimed oder Zeilen in `evidence/board-2026-10-22/delegation-inbox.txt`).
- **Daemon-Neustart:** IMMER Pid aus `watcher-state.json['watcher']['pid']` killen (`watch start` forked — Wrapper-Pid töten lässt das Altkind weiterlaufen; NIEMALS `pkill -f wait-workers`, das tötet die aufrufende Shell selbst).

### 2.2 Watcher & Event-Queue

- Daemon: `python3 evidence/board-2026-10-19/scripts/wait-workers.py watch start <coordinator-session-id>` (Coordinator-ID = aktuelle eigene Session aus `agent list --json`; nach Reboot/Session-Wechsel NEU starten).
- Kommandos: `watch add|remove|list <id>`; State: `scripts/watcher-state.json`; Log: `scripts/watcher.log`.
- **Event-Queue (Vergessenschutz):** jedes Event wird zusätzlich nach `scripts/events.log` angehängt. Zu Beginn jedes Turns `events-check.py` (zeigt un-quittierte Events), nach Abarbeitung `--ack`. Events aus dem Message-Stream sind flüchtig; die Queue-Datei ist maßgeblich. **Korrigendum 2026-10-27:** `--ack` ackt NUR events.log; die Reminder-Zähler brauchen `--reminders --ack` in EINEM Aufruf (zweitägiger Betriebsfehler: Basics-Flag „REMINDERS: n unread" war Pseudo-Spam, pos 235144→310863 nach Erstanwendung). Watchdog-Rauschen durch verwaiste watcher-state-Einträge → `prune-watch.sh` (cron */15).
- Meldet: Transitions (idle/exit/blocked/gone), CLOCK JUMP (Wall-vs-Monotonic), PERMISSION-DIALOG (Pane-Scan `Allow once|Permission required`, Re-Alert alle 5 min), busy>40 min, HEARTBEAT alle 60 min (Pflicht: fetch+PR-States+Pane-Checks). Offener Permission-Dialog zählt je nach API-Lage als busy/idle — Dialog-Scan ist primär, Busy-Heuristik Sekundärnetz.

### 2.3 Coord-Tick (Idle-only-Pflichtwecker)

`/home/marcel/tmp-ops/coord-tick.sh` (nohup/setsid; PID via `ps -eo pid,cmd | grep coord-tick[.]sh`). Prüft alle 600 s den Coordinator-Status über `agent list --json`; sendet den Tick NUR wenn der Coordinator `idle` ist — Zustellung per `tmux -L codeman send-keys` (NICHT `agent send`: das verweigert mit „is me“ und hätte den Tick 4,5 h still sterben lassen) (kein Interrupt bei laufender Arbeit). Tick-Text enthaelt zwingend Punkt 5: DREIZEILER-MELDUNG an den Owner (Was läuft / Was blockiert+Trigger / Nächste Aktion — genau 3 Zeilen). Tick-Nachricht enthält die vier Coordinator-Grundregeln (AGENTS.md 3.3). Neustart nach Reboot: Skript erneut mit setsid starten. Loggt nach coord-tick.log; letzter Fire in coord-tick.lastfire.

### 2.4 Permission-Dialoge auflösen (Coordinator)

1. Pane ansehen: `tmux -L codeman capture-pane -p -t codeman-<id8> | grep -aB6 "Allow once"`.
2. Legitimer Pfad (Workspace, /tmp/opencode, /home/marcel/dev/* — global vorfreigegeben in `~/.config/opencode/opencode.json` + `qwen36.json` + `flash2.json`: `permission.external_directory`: `/tmp/*`, `/home/marcel/dev/*`, `~/go/pkg/mod/*`, `~/.ssh/*` — die .ssh-Freigabe bleibt für VM-Diagnose-Lanes, aber Secrets gehören NIE in Evidence/Logs, und ssh-Bedarf ist pro Auftrag in dessen Scope-Zeile zu benennen): `tmux -L codeman send-keys -t codeman-<id8> Enter` (Allow once).
3. Optionsauswahl: Cursor startet auf „Allow once"; Right = nächste Option (Highlight 48;5;215 prüfen, capture -e), Enter. Down/Up funktioniert in diesem Dialog NICHT. „Allow always" schreibt nur in die Lane-Config dieser Session.
4. Danach Busy prüfen; Lane ggf. mit workspace-lokalen Pfaden weiterschicken.
5. **Frage-Dialoge** („Type your own answer", mehrteilig, Tab=Frage Wechsel/Enter=submit): erkennt der Dialog-Scan NICHT (Marker-Strings fehlen) und erscheinen als Ghost-Idle. Vor jedem Nudge bei IDLE-GHOST den Pane-Schwanz auf `?`/`tab  select` prüfen (o246, 29.10.). Antworten: ein Teilbeantworten mit Enter alone reicht nicht (Fragen bleiben „not answered"); sicherer Weg = Escape + reguläres `agent send` mit der ausformulierten Antwort.

### 2.5 Coordinator-Takeover (Session weggebrochen)

Reihenfolge in einer neuen Session im Ecosystem-Workspace, damit die Arbeit sofort weiterläuft:

1. `python3 evidence/board-2026-10-19/scripts/basics-check.py` (Pflichtprogramm, FLAGS abarbeiten)
2. `python3 evidence/board-2026-10-19/scripts/coord-state.py --resume` → Resume-Brief von Platte: laufende Lanes, je Lane Mission + END-Status (DONE/BLOCKED/dispatched), PR-Kandidaten mit Head/mergeStateStatus, Gesundheit (RAM/tmp/Watcher/Board-Currency), offene PLAN-Schritte.
3. `python3 evidence/board-2026-10-19/scripts/events-check.py --ack` und `--reminders --ack` (Nachrichten der toten Session).
4. Daemon auf die neue Session umbiegen: `kill $(jq -r .watcher.pid watcher-state.json)` (Pid aus der Datei, nie pkill), dann `wait-workers.py watch start <neue-session-id>`.
5. Native Todos aus dem letzten `## PLAN`-Block in `evidence/board-2026-10-22/coordinator-plan.md` rekonstruieren (todowrite).

Vor Schritt 1 die BETRIEBSREGELN im coordinator-plan.md lesen (Abschnitt „BETRIEBSREGELN BEI JEDEM TAKEOVER"): Rollentrennung, Vollauslastung ohne Scheinaufträge, Lane-Rotation statt Kontext-Wiederverwendung, keine langen Waits/Sleeps, Zustellbeweis per Pane, Evidence-Pflicht, Ressourcen- und Root-Git-Regeln, 26-FENCE, Planning-Publikation erst bei check=0.

Maßgebliche durable Quellen: `ops/missions/*.txt` (Aufträge), `evidence/board-*/<lane>*.md` (Evidence/END-Marker), `scripts/dispatch-log.jsonl` (Zustellbeweis), `coordinator-state-latest.json` (Snapshot, aktualisiert durch `coord-state.py` nach jedem Dispatch und alle 15 min durch den Watcher).

### 2.6 Codex-Lanes dispatchen (Coordinator-Seite)

- Spawn: `codeman agent spawn baseharbor -m codex -n codex-<auftrag>`; Auto-Compact separat auf 200.000 setzen. Netz-Login: `~/.codex/config.toml` braucht `sandbox_workspace_write.network_access = true` + Projekt-Trust (gesetzt 2026-10-22), sonst DNS-Fehler (exit 6).
- **YOLO-Modus (Owner-Direktive 2026-10-23):** `approval_policy = "never"` + `sandbox_mode = "danger-full-access"` in `~/.codex/config.toml` (wirkt pro neuem Spawn). Grenze: keine System-Ressourcen/Root — Aufträge darüber hinaus lehnt der Coordinator ab bzw. trägt Scope-Limits im Prompt. Dialog-Melder bleibt als Safety-Net aktiv.
- **Codex-Gate (Owner-Direktive 2026-10-29):** CODEX-REVIEW/-MERGE-Missionen erst NACH Dispatch, wenn die Exakt-Head-Hosted-Checks des Kandidaten GRÜN sind (bzw. bei base=main: Workflow policy SUCCESS). Rote/pending Checks zuerst per Worker-Runde + lokaler Voll-Suite (§4.3) heilen; Codex-Reviews auf Kandidaten mit bekannten Fehlfarben sind Ressourcenmissbrauch.
- BLOCKED-Meldungen der Merge-Lane sind Input, kein Ende: Ursache lane-seitig beheben, dann PR-Nummer/Kopf nachliefern.

## 3. Codex-Abschluss & Merge-Protokoll

- **END-Abnahme-Regel (Owner 2026-10-27):** Ein Lane-END plus Coordinator-Gegenprüfung (Disk/CI/Dossier) ist IMMER nur Zwischenstand. Die Abnahme eines Kandidaten (Karte review → accepted) trifft CODEX-REVIEW am SHA-genauen Kandidaten inkl. Evidence und Pflichtchecks; lane-seitige Reviews (rv-Muster) sind Vorprüfung, nie Abnahme. Merge erst nach Codex-ACCEPT über CODEX-MERGE. Historischer Anlass: rv1 meldete ACCEPT_WITH_NONBLOCKING auf #1111/ad4d0f28, das Owner-Audit fand am selben Stand sechs echte Abnahmefehler (G1–G6).
- Merge-Protokoll pro PR: Live-Gates zuerst (head-SHA-Wache, `gh pr checks`, Threads einzeln, Rulesets: code-owner approval + last-push approval + squash-only + up-to-date-Checks), dann approve+squash-merge, post-merge develop-Checks verifizieren, Abschnitt in `evidence/.../codex-merge-*.md` (UTC-Zeit, SHA before/after).
- Draft-PR-Merges: erst `gh pr ready <n>`, dann merge — GitHub weist squash --admin auf Drafts still ab (cmx-Serie 28.10.).
- **Merge-Ancestry bei main/develop-Divergenz (m1206-Serie, 2026-10-29):** main-only-Härtungen lassen merge-tree main→develop dauerhaft konfliktbehaftet erscheinen; Squash-Merges von develop-PRs erzeugen KEINE main-Ancestry. Regeln: (1) Beweisklasse „promotable" IMMER VOR der Merge-Entscheidung liefern. (2) Simulation ist legitim: S=commit-tree(&lt;union-tree&gt;,-p developTip), M2=commit-tree(&lt;union-tree&gt;,-p S,-p mainTip); merge-tree main×M2 clean + diff main..M2 ohne Defense-Minus-Zeilen = squash+branch-sync erfüllungsfähig. (3) Verlangt Kartenwortlaut eine andere Ancestry, ist das eine OWNER-Entscheidung (merge-commit vs. Klausel-Änderung) — nicht dreimal denselben Kandidaten neu reviewen. (4) Torverschiebungs-Bremse: Review-Missionen mit 3 Optionen (ACCEPT / Vertragszitat file:line / technischer Gegenbeweis).

## 4. Worker-/Lane-Betrieb

### 4.1 Todo- & Fortschrittssicht

- Verbindliche Quelle: `evidence/board-2026-10-19/scripts/todo-check.sh` (liest OpenCodes Todo-Tabelle = GUI-Grundlage). `ralphTodoStats/ralphTodos` in `agent list --json` ist ein ANDERES Feature (zeigt dort immer 0) — nie als Diagnose nutzen.
- Jede Lane pflegt PLAN doppelt: natives todowrite-Tool + `## PLAN` in der Evidence-Datei.
- **OPS-4 (q50):** Bevor evidence/... nach baseharbor-plan kopiert wird, durch pipe nach `bash evidence/board-2026-10-22/scripts/sanitize_evidence_for_plan.sh` (Markdown-Checkboxen `- [ ]`/`- [x]` → Unicode `- [☐]`/`- [✔]`), damit CardBoard keinen Checkbox-Render macht.

### 4.2 Evidence-Konventionen

- Ordner je Tag: `evidence/board-<JJJJ-MM-TT>/<lane>-<topic>.md`; Prompts: `.../prompts/`; Skripte NUR `evidence/board-2026-10-19/scripts/` — niemals im Produkt-Branch.
- Beweise als wiederholbare GitHub-Actions-Jobs (Muster: docker-runtime-acceptance.yml, targeted-integration.yml, runtime-validation/&lt;bereich&gt;/&lt;test&gt;/sha refs, validation/pre-release/selected/gates/&lt;ids&gt;), VMs nur zur Diagnose.
- Worker-Dateien landen gern relativ (`baseharbor-plan/evidence/...`); beim Einsammeln beide Orte prüfen: `evidence/board-<datum>/` UND `baseharbor-plan/evidence/board-<datum>/`.
- Qwen-Lanes: DONE/Datei-Erfindung möglich (b1: Geister-RUNBOOK-Commit, nichts auf Platte) — Disk-Artefakt-Check vor Abnahme ist Pflicht.

### 4.3 Voll-Suite receipt-execution-tests (Warnung vor dem 4-Suiten-Trugschluss)

- Die „unittest-Quartett"-Routine (postrun/chain/writer/tasklink) ist der TRUGSCHLUSS: Die HOSTED-Validation `receipt-execution-tests` führt MEHR Tests aus, darunter `pr-check-runner.py execute --check receipt-execution-tests` (lokal) UND `--check receipt-execution-tests-error` (Fehler-Fall). Jede Änderung an `scripts/receipt_*.py` braucht künftig den PASS der VOLLSTÄNDIGEN Suite (`scripts/receipt-suite.sh`). Exit-Code transparent, `--list` zeigt die Suiten.
- Definition: `.github/workflows/ci.yml:660` bzw. `:733` (case); 5 Dateien in origin/develop e0de0117: `.github/workflows/ci.yml`, `docs/de/development/minimal-ci-receipts.md`, `docs/development/minimal-ci-receipts.md`, `docs/internal/ci-binding-qualifications.md`, `evidence-r27/cy3.md`.
- Test-Evidence (origin/develop clean vs. PR #1213 HEAD c845caf6): develop clean EXIT 0 (runner-Script nicht im Produkt-Branch, OK); PR-HEAD: verifiers failen bis o230, dokumentiert in `evidence/r27-ci-replacement-2026-10-29/receipt-suite-manifest.md`.

### 4.4 Spawn-Boot-Stall (Serie 2026-10-29: o215, o220, o232)

Ornith-Lanes bleiben nach 1–2 Datei-Reads kommentarlos idle (Kontext <10 %, kein Error, keine Evidence; Muster Read→TodoWrite→Stall). Standardreaktion: EIN Nudge mit erstem konkreten AUSFÜHR-Befehl (kein erneutes „lies die Mission"); wirkte bisher immer. Zwei Nudge-Fehlschläge ⇒ Lane rotieren (rm + frisch spawnen), nicht weiter-nudgen.

## 5. Receipt-Kette Betrieb

### 5.1 Kann-jeder-Schritt & Pin-Register (vor JEDEM gehosteten Lauf)

- Kann-jeder-Schritt-Zeile in die Evidence: jede Auth/PIN/ENV-Quelle benennen; lane-gesetzte Prüfeingänge = Lauf ungültig. Controller-Pin immer = reviewed SHA des einen Kandidaten (Commit auf dem Kandidaten, nie Lane-ENV).
- Pin-Register-Regel: Änderung an `receipt-approval.yml` ODER `receipt_verifier.py` erfordert TRUSTED_SOURCE-PATCH auf den finalen MAIN-Commit nach dem Merge (Bytes-Gleichheit `pinned_source` :338–350), sonst fail-closed. Aktueller Pin: 46ed3e8c (= main-Tip seit #1212; Variable auf Voll-SHA).

### 5.2 Schattenlauf-Rezept (kopierbar, EIN Lauf)

```bash
# 0) Producer: PR mit contracts/**-Änderung (sonst wird 'PR · Public Contracts' SKIPPED)
gh run list --repo mcpdev80/baseharbor --branch <probe> --workflow "CI · Source & Quality" --limit 1
gh run download <RUN_ID> -n trusted-pr-plan -D <tmp>   # integration_sha = candidate
# 1) Fixture (Plan-Repository, von develop-Kopie):
python3 <wt-shadow>/scripts/receipt_tasklink.py --planning <plan> --workflow-id 352609139 \
  --head <40hex> --candidate <40hex-integration> --base <40hex> --task tasks/<card>.md
# 2) Receipt (veröffentlicht nach planning main):
GITHUB_EVENT_NAME=workflow_run GITHUB_REPOSITORY=mcpdev80/baseharbor GITHUB_REF=refs/heads/main \
  python3 <wt-shadow>/scripts/receipt_writer.py --planning <plan> --plan <FIXTURE> --run <RUN_ID> --attempt 1
# 3) Approval GEHOSTET (Owner-Direktive, Actor mcpdev80): EIN Input pr_number, --ref main
gh workflow run .github/workflows/receipt-approval.yml -R mcpdev80/baseharbor --ref main \
  -f pr_number=<PR>
# 4) Retained-Check:
python3 <wt-shadow>/scripts/receipt_verifier.py retained --planning <plan> --revision <plan-tip-40hex> \
  --receipt evidence/receipts/public-contracts/<RUN_ID>-1.json --candidate <40hex> --plan <FIXTURE>
```

### 5.3 Approval-Kette: Live-Fakten (2026-10-29)

- `receipt-approval.yml`-Interface: EIN Input `pr_number` (kein receipt_path/candidate/plan — der Approver löst alles selbst auf), MUSS `--ref main` laufen (POLICY_BRANCHES=("main",) + authentifizierter Run-Head als Byte-Quelle).
- `receipt_verifier.py` braucht PyYAML (Lazy-import :573 im Schema-Resolver, nur approve-Pfad; post-run/shadow-Pfade ohne yaml). Approve-Job: setup-python + `pip install 'PyYAML==6.0.2'` (Muster artifact-gc.yml:32). Behoben via #1209 (develop 676edc0a) + #1210 (main 0b0659f2).

### 5.4 Bekannte Fallen (live bewiesen 2026-10-27)

- **Trusted-Base-Planner:** `pr-gate-plan.py` läuft im CI aus der develop-Kopie — Qualifizierungs-/Mapping-Änderungen MÜSSEN vorher in develop landen (PR-B/#1106-Muster); PR-interne Mappings qualifizieren den eigenen PR nicht.
- **gh 2.102 auf Runnern:** braucht `GH_TOKEN: ${{ github.token }}` im Step-env (rc=4 sonst); GITHUB_*-Variablen sind per step-env NICHT überschreibbar → Contract-Werte in-shell setzen (`GITHUB_REF=refs/heads/main python3 ...`).
- **Planning-Klon im Approval-Job:** `fetch-depth: 0` (plan_revision zeigt auf älteren Introducer-Commit) + git user.name/email setzen (commit_file braucht Autor) + `--revision = git log -n1 --format=%H -- <receipt-path>`, NICHT clone-HEAD („introduce a new path"-Kontrakt).
- **Resolve-/Approve-Steps:** step-level env ist NICHT job-global — RECEIPT_PATH etc. in jedem betroffenen Step duplizieren.
- Repair-Kurve der Approval-Kette (8 Runden → nächstes Mal 1–2): #1097 Pins+env-Explizitheit · #1100 fetch-depth:0 · #1101 revision-Introducer · #1102 Resolve-env · #1103 GH_TOKEN · #1104 git-identity · #1105 Patch-Export (format-patch, upload warnt still). Alle gemergt (43ea0d0c..3431b22c-Ära).

## 6. Infrastruktur & Git-Betrieb

### 6.1 CI-Bootstrap-Familie (coverage-unresolved)

Root-Cause-Muster: PR-geänderter Planner/Workflow qualifiziert sich NICHT selbst; Behebung nur über (a) qualifizierten Commit auf develop (Muster #1026/#1029/#1030), (b) Classifier-Erweiterung als eigener kleiner PR (#1034-Muster), (c) docs-Klassifikation (DOC_CHECKS), (d) selected-gates-Pushformat als Beweisroute. Stale-Webhook-base.sha: Checks können an altem Base hängen — `gh run rerun <id> --failed` oder Branch-Sync-Commit als legitime Frischkur.

### 6.2 Runner-Infra (baha-docker)

- Rootless Docker für `github-runner` (uid 1001) als enabled User-Service + Linger=yes; Socket-Eigentümer-Fix über ExecStartPost-Override (chown github-runner:github-runner %t/docker.sock, 0660). Nach Reboot prüfen: Socket da? `docker version` als Runner-User?
- Destruktive Aktionen (Workspace-rm, `podman system reset`, Unit-Wipes) nur nach Drei-Punkte-Prüfung, in Evidence dokumentiert: (1) aktive Jobs/Runs auf dem Runner? (gh run list --status in_progress + Queue-Check), (2) laufende/beschlagnahmte Container/Volumes + Ownership (shared Stacks `bh-*-shared-*` NIE anfassen), (3) Wiederherstellungsfolge (Images/Cache-Aufbau, Laufzeitkosten). Bei Zweifel: nicht ausführen, sondern als Blocker mit Prüfergebnis melden.
- Runner-Watchlist: `gh api repos/mcpdev80/baseharbor/actions/runners` (baha-runner-docker-2 aktiv; Workspace-Pfad `/home/github-runner/actions-runner/_work`; bei „checkout fehlt git"-Fehlern `_work/<repo>` wipen).
- baha-podman: rootless podman; bei aardvark/netavark-Hängern (exit 126, „Cannot assign requested address") erst `loginctl/linger + stale netns` prüfen, zuletzt `podman system reset`.

### 6.3 Uhr & Realität

Nach JEDER Pause/Suspend: `timedatectl` + `git fetch` beider Repos + PR-Status-Check (`gh pr view --json state`), BEVOR delegiert wird. Clock-Jumps erkennt der Watcher (CLOCK JUMP); dann gelten: Handoff-Dokumente (`baseharbor-plan/evidence/codeman-*/HANDOFF.md`) zuerst lesen; OWNER STOP bindet sofort.

### 6.4 Git-/Repo-Regeln

- **Root-Repo ist ein baseharbor-Klon (Incident #6, 2026-10-29):** Der Workspace-Root teilt sich die Origin mit baseharbor; Root-`git add -A` committet ins PRODUKT-Repo (424 lokale Coord-Commits, nie gepusht — Remote sauber). Regeln: (1) Coord committet Root-Evidence NUR mit Dateipfaden (`git add <files>`), niemals -A. (2) Produkt-Branch-Transporte ausschließlich in baseharbor/ oder seinen Worktrees, nie im Root-CWD. (3) Vor jedem Transport-Commit: `git rev-parse --show-toplevel` gegen das intendierte Repo prüfen.
- **Worktree-Registratur (Befund 2026-10-28):** Vor jedem Spawn `git -C baseharbor worktree prune` und `worktree list | wc -l` notieren. >60 Einträge oder Zielbranch „already used" = Lane endet kommentarlos nach accepted-Toast. Altlanen-Entfernung nur bei physisch existierenden; der Registatureintrag ist das Gefährdungspotential, nicht das Verzeichnis.
- **Repo-Slug-Wechsel (2026-10-29):** GitHub-Transfer — khudari/* ist 404; alle gh-Aufrufe nutzen mcpdev80/baseharbor (Plan: mcpdev80/baseharbor-plan). Alte khudari-Links in Evidence redirecten im UI.
- **Card-Status-Wahrheit (plan.py, Befund 2026-10-29):** Der autoritative Status steht im Tag der Kartenzeile `#status/...` im Karten-Körper (plan.py:270), NICHT im Frontmatter-Feld `status:` (doppelt gepflegt = Fehlerquelle „status blocked requires a blocker"). Statuswechsel IMMER an der Tag-Zeile, danach render + check.

## 7. Bekannte Fehlerklassen und ihre Netze

| Klasse | Netz |
|---|---|
| Spawn-Race → Prompt verloren | Ready-Gate + dispatch-lane Auto-Resend |
| Multi-Line-Send still verworfen | Pointer-Prompts |
| Permission-Dialog ohne Event | Dialog-Scan + 5-min-Re-Alert + Pre-Approvals |
| watch add vergessen | dispatch-lane registriert automatisch |
| VM-Uhr driftet | CLOCK JUMP + Fetch-Pflicht + HEARTBEAT |
| Beweise an VM-Sessions hängend | CI-Job-Prinzip (§4.2) |
| Worker übergreift Status „accepted" | nur Owner/Codex; Lane-Kommandos verbieten es |
| Ornith-Boot-Stall | 1× konkreter Nudge, dann Lane rotieren (§4.4) |
| Root-`git add -A` ins Produkt-Repo | §6.4: Dateipfad-Only + show-toplevel-Check |
| Reminder-Ack-Pseudo-Spam | `--reminders --ack` in einem Aufruf + prune-watch.sh |
## 8. Skript-Katalog (Stand 2026-10-30, bereinigt — Einmal-Repros liegen in scripts/_archive-2026-10-30/)

### Coordinator
- `evidence/board-2026-10-19/scripts/basics-check.py` — Turn-Start-Pflichtprogramm (§3.2 AGENTS); Flags inkl. Pool/Currency/PLAN-Alter.
- `…/events-check.py` — Event-Queue ack (--ack / --reminders --ack; beides nötig, RUNBOOK-Beleg §7).
- `…/coord-state.py` — Resume-Brief (`--resume`, `--force`), Board-Währung.
- `…/wait-workers.py` — Watcher-Daemon (`watch start|add|remove|list|stop`); Pool-/Compact-/Dialog-Überwachung.
- `…/prune-watch.sh` — Registry-Müll aus watcher-state.json entfernen (nur bei gestopptem Daemon).
- `…/dispatch-lane.sh` / `…/spawn-lane.sh` — Standardpfad §3.4 (dispatch: Currency-Guard, Pointer-Send, Busy-Check, Auto-watch).
- `…/pool-admission.py` — POOL-/ROTATIONS-Gate vor Spawn (Modelle, Schichtfolgen).
- `…/lane-stalled.py`, `…/observed.py`, `…/build-record.sh`, `…/tmp-sweep.sh`, `…/handoff-to-codex.sh` — Innenleben von dispatch/watch (nicht direkt aufrufen).
- `/home/marcel/tmp-ops/coord-tick.sh` — Idle-only-Tick (§2.3).
- `evidence/board-2026-10-29/scripts/chain-run.sh` — Ketten-Dryrun/Mitschnitt; `ruleset-activation/apply.sh` — Ruleset-Paket DRYRUN default.

### Worker / gemeinsam
- `evidence/board-2026-10-19/scripts/todo-check.sh` — maßgeblich für GUI-Todos (§6 Alt).
- `evidence/board-2026-10-22/scripts/sanitize_evidence_for_plan.sh` — Checkbox-Maskierung OPS vor Plan-Kopie (Achtung: Validator prüft `\[.\]` — nummerierte Listen sind die robuste Form).
- `evidence/board-2026-10-29/scripts/receipt-suite.sh` — Voll-Suite receipt_* (5 Dateien, §4.3).
- `evidence/board-2026-10-29/scripts/negative-proofs.sh` — 3 Negative gegen echte Kette (PIN_MAIN = PLANNING-main!).
- Produktskripte (`scripts/receipt_*.py`, `pr-gate-plan.py`) gehören ins baseharbor-Repo, nie in evidence-Verzeichnisse (dorthin nur Repro-Helfer → jetzt `_archive-2026-10-30/`).

### 8.1 Hygiene-Routinen (cron-gepflegt, Stand 2026-10-30)
- `evidence/board-2026-10-19/scripts/workspace-hygiene.sh [ALTER_MIN]` — entfernt taeglich (cron 23:30Z = Tagesende, Alter 1440) nur saubere, stash-freie, inhaltsbasiert integrierte Worktrees nach Bundle-Beweis (`tmp-ops/worktree-graveyard/`); schuetzt aktive codeman-Lanes und REMOTE-Refs; Log `hygiene.log`.
- `tmp-sweep.sh 24` (cron */12h) — /tmp (tmpfs=RAM) Lane-Restbestände.
- `prune-watch.sh` (tmp-ops, cron */15m) + `ci-watch.sh` (*/10m) bestehen weiter.
- Betriebspflicht des Coordinators am Tagesende (AGENTS.md 3.9), keine Produkt-Karte (die fruehere Dauerkarte t-20261029-workspace-hygiene-rotine wurde auf Owner-Anweisung vom Product-Board entfernt).

## 9. Lane-Rotation in Korrekturschleifen (Pflicht, 2026-10-30)

ROTATIONS-PFLICHT: 3 Runden oder Kontext >70 % ⇒ frische Lane mit Missionsdigest, nicht weiter-nudgen (gilt auch für Permission-Dialog-Handoff in §2.4).
Kontext-Kosten der Schleifen: Eine Worker-Lane darf für Korrekturrunden desselben PRs höchstens dreimal nachgeladen werden ODER bis Kontext 70 % — dann ist ZWANGSROTATION angesagt: alte Lane rm, frische Lane mit 3–5-Zeilen-Missionsdigest (Branch, letzter guter Head, exaktes offenes Rezept aus dem Codex-Dossier, Verbot-Liste). Grund: endloses Nudgen bläht Sessions auf 80–90 % und produziert genau die Folgen, die Rotation vermeiden soll (Kontextverluste, wiedergelesene Dossiers, langsamere Turns). Reviewer-Kontinuität (§3.6) gilt für die CODEX-Session, nicht für Worker-Lanes.

## 10. Qwen-Flash-Worker ohne Thinking (Owner-Direktive 2026-10-30)
`spawn-lane.sh --flash` setzt pro Spawn OPENCODE_CONFIG auf `~/.config/opencode/flash-notthinking.json` (Kopie von flash2.json + Modell-Option `chat_template_kwargs: {enable_thinking: false}`, gateway-verifizert: keine reasoning_content). Die globale/coeigene Config (flash2.json, Root-opencode.json) bleibt UNANGETASTET — Thinking-Verbot gilt ausschließlich für neu gespawnte Flash-Worker-Lanes, nie für den Coordinator. Bei Flash-Lane-Rotation ist der Schalter automatisch dabei; Kontrolle im Pane-Header ist weiterhin Pflicht.
