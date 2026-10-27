# BaseHarbor Ecosystem – operatives RUNBOOK (Flash/Codeman)

Kanonischer Ort: `/home/marcel/dev/baseharbor-ecosystem/RUNBOOK.md`
Diese Datei enthält die bewährten Betriebsdetails. `AGENTS.md` definiert den
Soll-Prozess; hier steht das WIE inkl. verifizierter Kommandos. Stand: 2026-10-22.

## 1. Sessions & Pools

- Pool-Ziel: 1 Coordinator (Flash) + 5 Worker (1 Flash, 2 Ornith15, 2 Qwen3.6) = 6 Sessions;
  Codex-Abnahme-Lane separat. alle Modelle ausschließlich über AgentGateway
  (`http://192.168.6.132:4000/v1`, Modell-IDs via `/v1/models`).
- Hardware-ZUORDNUNG IST ZIELARCHITEKTUR (noch nicht verifiziert migriert):
  Coordinator+Flash-Worker → NVIDIA Spark; Ornith → AMD 295; Qwen3.6 → AMD-295-Laptop.
  Bis zur Verifikation nicht als Fakt behandeln. Host-CPU/RAM sind KEINE Spawn-Gates
  fuer Modellfragen; Gesundheit des Codeman-Hosts (Prozess-Headroom der TUI-Sessions,
  freier RAM > 2 Gi als Daumengrenze fuer Prozesstätigkeit, Uhr-Sync) bleibt Betriebskriterium.
- - Gemessene Gateway-Parallelität: 4-Stream tok/s flash 22,8 / ornith 23,2 / qwen3.6 15-19;
  VOLLLAST 6 simultane Streams (Pool 1 Coord + 5 Worker): 23,9 s Wandzeit, alle ok -
  qwen3.6 langsamster Kanal, kein Limit gefunden (2026-10-22).

## 2. Spawn + Dispatch (Pflichtpfad)

- **Spawn:** `evidence/board-2026-10-19/scripts/spawn-lane.sh <name> [--qwen|--flash]`
  - Modell-Overrides laufen NUR noch als Flag; intern flock-serialisiert
    (set→spawn→unset an `/home/marcel/.codeman/.spawn-lane.lock`) - keine
    überlappenden globalen tmux-Env-Änderungen mehr; kein manueller Env-Trick nötig.
  - Auto-Compact: 200.000 Standard, 400.000 für Flash-Konfiguration.
  - TUI-Readiness-Gate (Welcome-Erkennung) + automatische Watcher-Registrierung
    (Watch-Add kann nicht mehr vergessen werden).
  - Nach dem Spawn ist die Modellidentität im Pane-Footer einmal zu verifizieren.
- **Auftrag:** Prompttext als Datei nach `evidence/board-2026-10-22/prompts/<lane>.txt`
  (arbeitsverzeichnis-lokale Pfade, /tmp/opencode ok), dann
  `evidence/board-2026-10-19/scripts/dispatch-lane.sh <id> <prompt-datei>`
  = Einzeiler-Pointer-Nachricht + 5s-Busy-Check + Auto-Resend + Auto-watch-add.
  Hintergrund: Codeman verwirft Multi-Line-Sends still; `--wait` blockiert bis Abschluss.
- **Nach jedem Erfolg:** `codeman agent rm <id>` + `wait-workers.py watch remove <id>`.
- **Watchdog-Layer (freigegeben 2026-10-22):** `wait-workers.py` prüft alle 60s Pool-Belegung nach
  echtem Modell (Pane-Header `Build · …`, Coordinator/codex ausgenommen) und mappt `basics-check.py --json`
  Flags in deduplizierte Reminders (Cooldown 15min, kritische sofort) via Queued-`agent send`; Skripte
  erinnern nur, entscheiden nie. Compact-Erfolg = Kontext unter Schwelle ODER Message-Event plus Rückgang
  (kein starrer 20%-Abschlag); Delegate-Reminder nur bei bekanntem ausführbarem Task
  (`plan.py list --ready` unclaimed oder Zeilen in `evidence/board-2026-10-22/delegation-inbox.txt`). spawn-lane.sh: POOL-Guard VOR Spawn (exit 4) + Identitäts-Guard NACH
  Spawn (exit 3, Lane bleibt); Root-Cause f8: geerbte Daemon-Env `OPENCODE_CONFIG` – Override jetzt
  zwingend in der flock-Sektion. **Daemon-Neustart: immer Pid aus `watcher-state.json['watcher']['pid']`
  killen (`watch start` forked – Wrapper-Pid töten lässt Altkind weiterlaufen).**
- **Lane unterbrechen statt töten:** `codeman agent interrupt <id>` (nur ESC). Raw `tmux send-keys C-c` beendet den OpenCode-Prozess (q18-Verlust 2026-10-22). Mehrzeilige Inhalte immer als Promptdatei unter `evidence/board-2026-10-22/prompts/` (codeman verwirft Multi-Line still).

## 3. Watcher

- Daemon: `python3 evidence/board-2026-10-19/scripts/wait-workers.py watch start <coordinator-session-id>`
  (Coordinator-ID = aktuelle eigene Session aus `agent list --json`; nach Reboot/Session-Wechsel NEU starten).
- Kommandos: `watch add|remove|list <id>`; State: `scripts/watcher-state.json`; Log: `scripts/watcher.log`.
- **Event-Queue (Vergessenschutz):** jedes Event wird zusätzlich nach `scripts/events.log` angehängt.
  Coordinator-Fixregel: zu Begin jedes Turns `python3 evidence/board-2026-10-19/scripts/events-check.py`
  (zeigt un-quittierte Events) und nach Abarbeitung `--ack`. Events aus dem Message-Stream sind
  flüchtig; die Queue-Datei ist maßgeblich.
- Meldet: Transitions (idle/exit/blocked/gone), **CLOCK JUMP** (Wall-vs-Monotonic),
  **PERMISSION-DIALOG now** (Pane-Scan `Allow once|Permission required`, Re-Alert alle 5 min),
  **busy>40min**, **HEARTBEAT** alle 60 min (Pflicht: fetch+PR-States+Pane-Checks).
- Daemon-Neustart: Pid aus `watcher-state.json` → kill → `watch start` (NIEMALS
  `pkill -f wait-workers` – das tötet den aufrufenden Shell selbst).
- Der Daemon zählt ein Pane mit offenem Permission-Dialog als busy/idle je nach
  API-Lage – Dialog-Scan ist primär, Busy-Heuristik Sekundärnetz.
- **Coord-Tick (Idle-only-Pflichtwecker):** `/home/marcel/tmp-ops/coord-tick.sh`
  (nohup/setsid,PID via `ps -eo pid,cmd | grep coord-tick[.]sh`). Prüft alle 600 s den
  Coordinator-Status über `agent list --json`; sendet die Tick-Nur wenn der Coordinator
  `idle` ist (kein Interrupt bei laufender Arbeit). Tick-Nachricht enthält die vier
  Coordinator-Grundregeln. Neustart nach Reboot: Skript erneut mit setsid starten.

## 4. Permission-Dialoge auflösen

1. Pane ansehen: `tmux -L codeman capture-pane -p -t codeman-<id8> | grep -aB6 "Allow once"`
2. Legitimer Pfad (Workspace, /tmp/opencode, /home/marcel/dev/* – global vorfreigegeben in
   `~/.config/opencode/opencode.json` + `qwen36.json` + `flash2.json`:
   `permission.external_directory`: `/tmp/*`, `/home/marcel/dev/*`, `~/go/pkg/mod/*`,
   `~/.ssh/*` - die .ssh-Freigabe bleibt (VM-Diagnose-Lanes), aber Secrets gehören
   NIE in Evidence/Logs und ssh-Bedarf ist pro Auftrag in dessen Scope-Zeile zu
   benennen):
   `tmux -L codeman send-keys -t codeman-<id8> Enter` (Allow once).

3b. Optionauswahl: Cursor startet auf 'Allow once'; Right = naechste Option (Highlight 48;5;215 pruefen, capture -e), Enter. Down/Up funktioniert in diesem Dialog NICHT. Allow always schreibt nur in die Lane-Config dieser Session.
3. Danach Busy prüfen; Lane ggf. mit workspace-lokalen Paden weiterschicken.

## 5. Codex-Maintainer-Lane

- Spawn: `codeman agent spawn baseharbor -m codex -n codex-<auftrag>`; Auto-Compact separat
  auf 200.000 setzen. Netz-Login-Erfordernis: `~/.codex/config.toml` braucht
  `sandbox_workspace_write.network_access = true` + Projekt-Trust (beides gesetzt 2026-10-22),
  sonst DNS-Fehler (exit 6) in der Session.
- **Codex startet grundsätzlich im YOLO-Modus (Owner-Direktive 2026-10-23):**
  `approval_policy = "never"` + `sandbox_mode = "danger-full-access"` in
  `~/.codex/config.toml` (gesetzt 2026-10-23; wirkt pro neuem Spawn). Grenze: keine
  System-Ressourcen/Root – Aufträge, die darüber hinausgehen, lehnt der Coordinator
  ab bzw. tragen Scope-Limits im Prompt. Dialog-Melder bleibt als Safety-Net aktiv.
- Merge-Protokoll pro PR: Live-Gates zuerst (head-SHA-Wache, `gh pr checks`, Threads einzeln,
  Rulesets: code-owner approval + last-push approval + squash-only + up-to-date-Checks),
  dann approve+squash-merge, post-merge develop-Checks verifizieren, Abschnitt in
  `evidence/.../codex-merge-*.md` (UTC-Zeit, SHA before/after).
- BLOCKED-Meldungen der Merge-Lane sind Input, kein Ende: Ursache lane-seitig beheben,
  dann PR-Nummer/Kopf nachliefern.
- **END-Abnahme-Regel (Owner 2026-10-27):** Ein Lane-END plus Coordinator-Gegenprüfung
  (Disk/CI/Dossier) ist IMMER nur Zwischenstand. Die Abnahme eines Kandidaten
  (Karte review -> accepted) trifft CODEX-REVIEW am SHA-genauen Kandidaten inklusive
  Evidence und Pflichtchecks; Lane-seitige Reviews (rv-Muster) sind Vorprüfung, nie
  Abnahme. Merge erst nach Codex-ACCEPT über CODEX-MERGE. Historischer Anlass: rv1
  meldete ACCEPT_WITH_NONBLOCKING auf #1111/ad4d0f28, das Owner-Audit fand am
  selben Stand sechs echte Abnahmefehler (G1-G6).

## 6. Todo-/Fortschritts-Sicht

- Verbindliche Quelle: `evidence/board-2026-10-19/scripts/todo-check.sh` (liest OpenCodes
  Todo-Tabelle = GUI-Grundlage). `ralphTodoStats/ralphTodos` in `agent list --json`
  ist ein ANDERES Feature (zeigt dort immer 0) – nie als Diagnose nutzen.
- Jede Lane pflegt PLAN doppelt: natives todowrite-Tool + `## PLAN` in der Evidence-Datei.
- **OPS-4 (q50):** Bevor evidence/... nach baseharbor-plan kopiert wird, durch pipe nach
  `bash evidence/board-2026-10-22/scripts/sanitize_evidence_for_plan.sh`, um Markdown-Checkboxen
  (`- [ ]`, `- [x]`) zu Unicode-Pendants (`- [☐]`, `- [✔]`) zu maskieren und
  CardBoard-Checkbox-Render zu verhindern.

## 7. Evidence-Konventionen

- Ordner je Tag: `evidence/board-<JJJJ-MM-TT>/<lane>-<topic>.md`; Prompts: `.../prompts/`;
  Skripte: NUR `evidence/board-2026-10-19/scripts/` – niemals im Produkt-Branch.
- Beweise als wiederholbare GitHub-Actions-Jobs (Muster: docker-runtime-acceptance.yml,
  targeted-integration.yml, runtime-validation/<bereich>/<test>/sha refs,
  validation/pre-release/selected/gates/<ids>), VMs nur zur Diagnose.
- Worker-Dateien landen gern relativ (`baseharbor-plan/evidence/...`); beim Einsammeln
  beide Orte prüfen: `evidence/board-<datum>/` UND `baseharbor-plan/evidence/board-<datum>/`.

## 8. CI-Bootstrap-Familie (coverage-unresolved)

- Root-Cause-Muster: PR-geänderter Planner/Workflow qualifiziert sich NICHT selbst;
  Behebung nur über (a) qualifizierten Commit auf develop (Muster #1026/#1029/#1030),
  (b) Classifier-Erweiterung als eigener kleiner PR (#1034-Muster),
  (c) docs-Klassifikation (DOC_CHECKS), (d) selected-gates-Pushformat als Beweisroute.
- Stale-Webhook-base.sha: Checks koennen an altem Base haengen – `gh run rerun <id> --failed`
  oder Branch-Sync-Commit als legitime Frischkur.

## 9. Runner-Infra (baha-docker)

- Rootless Docker für `github-runner` (uid 1001) als enabled User-Service + Linger=yes;
  Socket-Eigentümer-Fix über ExecStartPost-Override (chown github-runner:github-runner
  %t/docker.sock, 0660). Nach Reboot prüfen: Socket da? `docker version` als Runner-User?
- Destruktive Aktionen (Workspace-rm, `podman system reset`, Unit-Wipes) sind
  nur nach Drei-Punkte-Prüfung erlaubt und in der Evidence zu dokumentieren:
  (1) aktive Jobs/Runs auf dem Runner? (gh run list --status in_progress +
  queue-Check), (2) laufende/beschlagnahmte Container/Volumes + Ownership
  (shared Stacks `bh-*-shared-*` NIE anfassen), (3) Wiederherstellungsfolge
  (Images/Cache-Aufbau, Laufzeitkosten). Bei Zweifel: nicht ausführen, sondern
  als Blocker mit Prüfergebnis melden.
- Runner-Watchlist: `gh api repos/mcpdev80/baseharbor/actions/runners` (baha-runner-docker-2
  ist der Aktive; Workspace-Pfad `/home/github-runner/actions-runner/_work`; bei
  Checkout-fehlt-git-Fehlern `_work/<repo>` wipe-rennen).
- baha-podman: rootless podman; bei aardvark/netavark-Hängern (exit 126, "Cannot assign
  requested address") erst `loginctl/linger + stale netns` prüfen, zuletzt `podman system reset`.

## 10. Uhr & Reallität

- Nach JEHER Pause/Suspend: `timedatectl` + `git fetch` beider Repos + PR-Status-Check
  (`gh pr view --json state`), BEVOR delegiert wird. Clock-Jumps erkennt der Watcher
  und meldet CLOCK JUMP; dann gilt: Handoff-Dokumente (`baseharbor-plan/evidence/codeman-*/HANDOFF.md`)
  zuerst lesen; OWNER STOP bindet sofort.

## 11. Bekannte Fehlerklassen (heute diagnostiziert) und ihre Netze

| Klasse | Netz |
|---|---|
| Spawn-Race → Prompt verloren | Ready-Gate + dispatch-lane Auto-Resend |
| Multi-Line-Send still verworfen | Pointer-Prompts |
| Permission-Dialog ohne Event | Dialog-Scan + 5-min-Re-Alert + Pre-Approvals |
| watch add vergessen | dispatch-lane registriert automatisch |
| VM-Uhr driftet | CLOCK JUMP + Fetch-Pflicht + HEARTBEAT |
| Beweise an VM-Sessions hängend | CI-Job-Prinzip (Abschnitt 7/8) |
| Worker übergreift Status 'accepted' | nur Owner/Codex; Lane-Kommandos verbieten es |

## 7. Coordinator-Takeover (Session weggebrochen)

Reihenfolge in einer neuen Session im Ecosystem-Workspace, damit die Arbeit sofort weiterläuft:
1. `python3 evidence/board-2026-10-19/scripts/basics-check.py` (Pflichtprogramm, FLAGS abarbeiten)
2. `python3 evidence/board-2026-10-19/scripts/coord-state.py --resume`
   -> Resume-Brief aus Platte: laufende Lanes, je Lane Mission + END-Status (DONE/BLOCKED/dispatched),
      PR-Kandidaten mit Head/mergeStateStatus, Gesundheit (RAM/tmp/Watcher/Board-Currency), offene PLAN-Schritte.
3. `python3 evidence/board-2026-10-19/scripts/events-check.py --ack` und `--reminders --ack` (Nachrichten der toten Session)
4. Daemon auf die neue Session umbiegen: `kill $(jq -r .watcher.pid watcher-state.json)` (Pid aus der Datei, nie pkill)
   dann `python3 evidence/board-2026-10-19/scripts/wait-workers.py watch start <neue-session-id>`
5. Native Todos aus dem letzten `## PLAN`-Block in `evidence/board-2026-10-22/coordinator-plan.md` rekonstruieren (todowrite).
Maßgebliche durable Quellen: `ops/missions/*.txt` (Aufträge), `evidence/board-*/<lane>*.md` (Evidence/END-Marker),
`evidence/board-2026-10-19/scripts/dispatch-log.jsonl` (Zustellbeweis), `coordinator-state-latest.json` (Snapshot,
aktualisiert durch `coord-state.py` nach jedem Dispatch und alle 15 min durch den Watcher).

Vor Schritt 1 jede Session die BETRIEBSREGELN im coordinator-plan.md lesen
(Abschnitt "BETRIEBSREGELN BEI JEDEM TAKEOVER"): Rollentrennung, Vollauslastung
ohne Scheinauftraege, Lane-Rotation statt Kontext-Wiederverwendung, keine
langen Waits/Sleeps, Zustellbeweis per Pane, Evidence-Pflicht, Ressourcen-
und Root-Git-Regeln, 26-FENCE, Planning-Publikation erst bei check=0.

## 12. Receipt-Kette Betrieb (v0.4.27 Schattenlauf, Stand 2026-10-27)

### Schattenlauf-Ablauf (kopierbar, EIN Lauf)
```bash
# 0) Producer: PR mit contracts/**-Aenderung (sonst wird 'PR · Public Contracts' SKIPPED)
gh run list --repo mcpdev80/baseharbor --branch <probe> --workflow "CI · Source & Quality" --limit 1
gh run download <RUN_ID> -n trusted-pr-plan -D <tmp>   # integration_sha = candidate
# 1) Fixture (Plan-Repositorium, von develop-Kopie):
python3 <wt-shadow>/scripts/receipt_tasklink.py --planning <plan> --workflow-id 352609139 \
  --head <40hex> --candidate <40hex-integration> --base <40hex> --task tasks/<card>.md
# 2) Receipt (veroeffentlicht nach planning main):
GITHUB_EVENT_NAME=workflow_run GITHUB_REPOSITORY=mcpdev80/baseharbor GITHUB_REF=refs/heads/main \
  python3 <wt-shadow>/scripts/receipt_writer.py --planning <plan> --plan <FIXTURE> --run <RUN_ID> --attempt 1
# 3) Approval GEHOSTET (Owner-Direktive, Actor mcpdev80):
gh workflow run .github/workflows/receipt-approval.yml -R mcpdev80/baseharbor --ref develop \
  -f receipt_path=evidence/receipts/public-contracts/<RUN_ID>-1.json -f candidate_sha=<40hex> -f plan_revision=<FIXTURE>
gh run download <DISPATCH_RUN> -n approve-commit-patch -D <tmp>
cd <plan> && git am <tmp>/approve-shadow.patch && git push origin HEAD:main
# 4) Retained-Check:
python3 <wt-shadow>/scripts/receipt_verifier.py retained --planning <plan> --revision <plan-tip-40hex> \
  --receipt evidence/receipts/public-contracts/<RUN_ID>-1.json --candidate <40hex> --plan <FIXTURE>
```

### Bekannte Fallen (jeweils live bewiesen 2026-10-27)
- **Trusted-Base-Planner:** pr-gate-plan.py laeuft im CI aus der develop-Kopie — Qualifizierungs-/Mapping-Aenderungen MUESSEN vorher in develop landen (PR-B/#1106-Muster); PR-interne Mappings qualifizieren den eigenen PR nicht.
- **gh 2.102 auf Runnern:** braucht `GH_TOKEN: ${{ github.token }}` im Step-env (rc=4 sonst); GITHUB_*-Variablen sind per step-env NICHT ueberschreibbar → Contract-Werte in-shell setzen (`GITHUB_REF=refs/heads/main python3 ...`), PR3 entfernt den Hack durch echtes main.
- **Planning-Klon im Approval-Job:** `fetch-depth: 0` (plan_revision zeigt auf aelteren Introducer-Commit) + git user.name/email setzen (commit_file braucht Autor) + --revision = `git log -n1 --format=%H -- <receipt-path>`, NICHT clone-HEAD ('introduce a new path'-Kontrakt).
- **Resolve-/Approve-Steps:** step-level env ist NICHT job-global — RECEIPT_PATH etc. in jedem betroffenen Step duplizieren.
- **Qwen-Lanes:** DONE/Datei-Erfindung (b1 heute: Geister-RUNBOOK-Commit, nichts auf Platte) — Disk-Artefakt-Check vor Abnahme ist Pflicht.
### Repair-Kurve der Approval-Kette (8 Runden -> naechstes Mal 1-2)
#1097 Pins+env-Explizitheit · #1100 fetch-depth:0 · #1101 revision-Introducer · #1102 Resolve-env · #1103 GH_TOKEN · #1104 git-identity · #1105 Patch-Export (format-patch, upload warnt still). Alle gemergt 43ea0d0c..3431b22c-Aera.

### Reminder-Ack Korrigendum (2026-10-27, zweitaegiger Betriebsfehler)
`events-check.py --ack` ackt NUR events.log; die REMINDERS-Zaehler braucht `--reminders --ack` in EINEM Aufruf. Basics-Flag 'REMINDERS: n unread' war deshalb tagelang Pseudo-Spam (pos 235144->310863 nach Erstanwendung). Watchdog-Rauschen zusaendlich durch verwaiste watcher-state-Eintraege -> prune-watch.sh (cron */15).
