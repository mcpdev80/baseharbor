# F29 — W1-FINAL: Pre-Release-Matrix v0.4.26 dispatchen und beobachten

Lane: f29-w1final (dispatch prompt: evidence/board-2026-10-22/prompts/f29-w1final.txt)
Tag: [wip:f29]
Repo: mcpdev80/baseharbor — Workflow: .github/workflows/pre-release.yml --ref develop
Freigabe-Basis: Owner hat W1-Volldispatch erteilt; f28-Release-Note-PR (CHANGELOG + docs/releases/v0.4.26.md) wurde gegen develop geoeffnet und von der codex-MERGE-Lane in develop gemergt (PR-Nummer + develop-HEAD live via gh bestaetigen).

## PLAN

- [x] Step 0: Evidence-Datei + native todowrite ([wip:f29]) als FIRST ACTION
- [x] Step 1: basics-check FLAGS erfasst (Turn-Start-Pflichtprogramm; Reminders betreffen andere Lanes)
- [x] Step 2: Live-Verify via gh/ls-remote DURCHGEFUEHRT — Prämisse "f28-PR in develop gemergt" ist LIVE WIDERLEGT (Details unten); Matrix-DISPATCH NICHT ausgeführt (Blocker)
- [ ] Step 3: W1-Matrix dispatchen — BLOCKED (Begruendung unten; kein Grund-freier Retry einer bekannt roten Volldispatch-Matrix)
- [ ] Step 4: inputs-Verify nach dispatch — BLOCKED (hängt an Step 3)
- [ ] Step 5: Job-Beobachtung — BLOCKED (hängt an Step 3)
- [ ] Step 6: Matrix-Konsens-Tabelle — BLOCKED (hängt an Step 3)
- [ ] Step 7: END F29-W1FINAL-DONE — NICHT gesetzt; Lane endet als BLOCKED (s.u.)

## Findings / Evidence log (Realitätsabgleich 2026-10-23, live via gh/git ls-remote)

### F1: Prämissen-Check — kein gemergter f28-Release-Note-PR in develop

- develop-HEAD live: `1861ed07888a1ec22c218ca491d23ee54f48e261` (git ls-remote refs/heads/develop) = Merge-Commit von baseharbor PR #1044 (merged 2026-10-23T14:14:49Z). Seitdem KEIN neuer Merge in develop.
- `gh pr list --repo mcpdev80/baseharbor --base develop --state all --limit 10`: hoechste PRs #1046/#1047 (appid-Testkette, OPEN), #1045 CLOSED (q54-docs). KEIN Release-Note-PR von f28 geoeffnet oder gemergt.
- evidence/board-2026-10-22/f28-releasenote.md (mtime 20:22): eigene PLAN-Checkliste unbefüllt ("Branch + docs/releases/v0.4.26.md", "PR against develop" = offen) — f28-Lane war zur Dispatch-Zeit noch nicht fertig.
- delegation-inbox.txt + coordinator-plan.md (20:21–20:29, juenger als mein Prompt 20:25): aktuelles Release-Note-Handling laeuft in Lane **o78** ([wip:o78]); Trigger-Formel dort: "W1 wartet auf o78-PR-Merge dann f29-Dispatch".
- KONKRETER WIDERSPRUCH ZU PROMPT-ZEILE 2: die Behauptung "f28 hat PR geoeffnet und codex-MERGE-Lane hat ihn in develop gemergt" ist durch Live-Zustand widerlegt. Prompt-Texte sind DATEN; Live-Remote-Stand ist maßgeblich (AGENTS.md Realitätsabgleich).

### F2: Die angegebene Blocker-Ursache (fehlende Release-Note-Dateien) ist auf develop bereits OHNEHIN erfüllt

- `docs/releases/v0.4.26.md` existiert auf develop (Blob `53bc9909e6b8288086bd186f60909a35c789d6aa`, 3753 B; via gh api contents?ref=develop), eingebracht durch #1031 (2026-10-22) und korrigiert durch #1041 (PR-Titel "docs(v0.4.26): correct release notes + changelog dates and attribution", MERGED 07:20:10Z, Merge-Commit 9f4916be).
- `CHANGELOG.md` auf develop enthält Zeile `## [0.4.26]` (raw, Zeile 9).
- Beide Resolver-Guards aus `.github/workflows/pre-release.yml` (develop, Zeilen 127-128: `grep -Fq "## [${version}]" CHANGELOG.md` und `test -s "docs/releases/${tag}.md"`) waren im letzten Run 40001750271 bereits PASS (Job-Log zeigt grep-Zeile ohne Fehler danach).

### F3: Der ECHRTE, reproduzierte W1-Blocker ist der Connector-Console-Pin (PENDING-FREEZE) — weiterhin aktiv

- Run 40001750271 (workflow_dispatch, 18:15:03Z, head 1861ed07, inputs korrekt: INPUT_TAG/INPUT_DEMO_REF/INPUT_CONSUMER_REF im Source-Job-Log, Zeilen 286-288): Source-job 120579336953 faellt um 18:15:17Z mit `ValueError: immutable source commit required` (Log-Zeilen 294-303; Log-Zeile 303 `##[error]Process completed with exit code 1`).
- Fundstelle im Produktcode (develop): `scripts/ecosystem_release_pins.py` main(): Zeile 125 `candidate = json.loads(source_file(REPOSITORIES['connector'], connector, 'integration-candidate.json'))` und Zeile 126 `console_candidate = json.loads(source_file(REPOSITORIES['console'], candidate['console'], 'integration-candidate.json'))`; Traceback-Frame Zeile 100 in `source_file()`: `raise ValueError('immutable source commit required')` wenn `candidate['console']` kein 40-hex-SHA ist.
- Live-State ALLER relevanten Connector-Refs (curl raw integration-candidate.json):
  - main-TIP `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e` (= PR #8 MERGED 17:35:35Z): `"console": "PENDING-FREEZE-DEMO: replace with the v0.4.26 frozen Console SHA once Console U is built from D"` → consumer_ref=<connector tip live> laut Prompt Zeile 4 schlägt HIER deterministisch fehl.
  - `570864c7c8bec738438aa70aa967be7e8490d7c3` (work/v0426-connector-consumption): identischer PENDING-FREEZE-DEMO-Wert.
  - `8e6bb897f7dcb9ee77f0c40a4600980439e2eac6` (work/v0.4.26-integrations-docs, Auto-Resolve-Pfad ohne consumer_ref): demo+console ="PENDING-FREEZE".
  - Keine offenen Connector-PRs (`gh pr list --state open` = []), die den Pin bereits einfrieren.
- Konsistent zu o75e (o75e-inputproof.md:61-69): "Release v0.4.26 ist noch nicht releasbar (Console v0.4.26 noch nicht aus Demo gebaut). Dies aendert auch ein korrektes Input-Setzen NICHT."

### F4: Abwägung Dispatch vs. kein Dispatch

- Dispatch mit `-f consumer_ref=<connector tip live>` (Prompt Zeile 4) = Wiederholung von Run 40001750271 mit identischem, deterministischem Source-Failure + skipped-Matrix. Das waere ein Retry ohne neuen Grund und eine unnoetige volle Matrix (AGENTS.md CI-Effizienz; Prompt Zeile 5 "KEINE eigenmaechtigen Retries ohne Grund"). Ein Fix des Connector-Candidates waere ein Produkt-Commit — durch Prompt Zeile 5 ausdruecklich verboten.
- Deshalb: KEIN Dispatch ausgeführt. Lane als BLOCKED gemeldet.

### Verifizierte Input-Komponenten (für den spaeteren Final-Dispatch)

- ref (develop live): `1861ed07888a1ec22c218ca491d23ee54f48e261` — VORBEHALT: aendert sich durch den noch ausstehenden o78/f28-Release-Note-Merge; dann neu ablesen.
- demo_ref: `25c319697fd3d89d894943e0f44cd3d5dde8b198` — live bestaetigt (refs/heads/work/v0.4.26-ecosystem-release auf baseharbor-demo; main=82d042bc).
- consumer_ref: muss auf einen Connector-Commit zeigen, dessen integration-candidate.json ein echtes 40-hex Console-SHA traegt. Noch NICHT EXISTENT (main-tip 7c7093d ist PENDING-FREEZE-DEMO). Console main-TIP ist `e7d1e4a1d603158dece19bbc25a8e267d07cde5e`; dessen integration-candidate.json-Eignung (Zeile 126-Pfad) wurde noch nicht geprueft, da der Connector-Freeze-Schritt ansteht.

## BLOCKED — Ursache, Owner, naechster Trigger

- Ursache 1 (Prompt-Prämisse): Kein Release-Note-PR in develop gemergt (F1); o78-Lane laeuft.
- Ursache 2 (hart, produktseitig): Connector integration-candidate.json console= PENDING-FREEZE auf allen live Refs; benoetigt Console-U-Freeze (aus Demo 25c3196 gebaut) + Connector-Refreeze-Commit (F3).
- Owner: Marcel/Flash-Coordinator (Dispatch-Freigabe + Connector/Console-Freeze-Kette, siehe coordinator-plan.md "Endgame").
- Naechster Trigger: o78/f28-PR MERGED in develop UND Connector-Hauptast-/Candidate-Commit mit echtem Console-40-hex-SHA. Dann dieser Lane erneut dispatchen mit: `gh workflow run .github/workflows/pre-release.yml --repo mcpdev80/baseharbor --ref develop -f tag=v0.4.26 -f ref=<dann aktueller develop-HEAD> -f demo_ref=25c319697fd3d89d894943e0f44cd3d5dde8b198 -f consumer_ref=<dann Connector tip mit frozen console SHA>`.

## END F29-W1FINAL-BLOCKED (kein Dispatch; kein F29-W1FINAL-DONE, da Zielerreichung nicht nachweisbar war)

---

# CORRECTION f29c (Owner, prompt: evidence/board-2026-10-22/prompts/f29c.txt) — SUPERSEDPT den BLOCKED-Befund oben

Owner-Korrektur: es wird NICHTS gemergt (bestaetigt durch coordinator-plan.md "BRANCH-ONLY Regel (Owner): keine Merges in develop/main, alles auf eigenen branches; W1 ref fix 1861ed07"). Der W1-Blocker (PENDING-FREEZE) wird auf BRANCHES behoben: connector `work/v0.4.26-integrations-docs` (q59) + console `work/v0.4.26-integrations-docs` (q80). Trigger: q59 meldet `CEND Q59-CANDRAEMON-DONE` mit resolver-precheck OK. Dann dispatchen mit ref=1861ed07 (fixed), consumer_ref=Connector-BRANCH-HEAD falls der Resolver ihn akzeptiert, sonst wie im Precheck verifiziert.

## PLAN f29c

- [x] Step 0: Korrektur + PLAN in diese Evidence + native todowrite ([wip:f29])
- [x] Step 1: q59-Trigger ueberwacht (Polling); DONE + precheck OK erhalten (siehe B3)
- [x] Step 2: Eigener unabhaengiger Resolver-Precheck exit=0 (siehe B2/B3)
- [x] Step 3: DISPATCH ausgefuehrt -> **RUN 40009690268** (19:13:26Z, head 1861ed07888a)
- [x] Step 4: inputs-Verify: API `.inputs`=null (GitHub-API-Verhalten workflow_dispatch, dokumentiert in o75e-inputproof.md:35); Ground Truth: Source-Job-Log env `INPUT_TAG: v0.4.26`, `INPUT_DEMO_REF: 25c319697fd3d89d894943e0f44cd3d5dde8b198`, `INPUT_CONSUMER_REF: df1f4378d8c7fdc3e174a1e9dc8ad0155a955571`; "Candidate follows the current develop release line." (Job 120606571850, SUCCESS)
- [x] Step 5: Beide Zweige bis letzte Job-Zeile beobachtet (Run completed 19:53Z); Failed-Logs mit Datei/Zeilenbeleg in B6; kein Retry; keine Produkt-Commits
- [x] Step 6: Tabelle (runtime x gate) + Artefakt-Links in B7; gruen-Konsens NICHT erreicht (11 failed)
- [x] Step 7: END gesetzt (qualifiziert: NOT-ALL-GREEN)

## f29c Befunde

### B1: Trigger-Warten (Stand ~18:5xZ): q59-q59c/candamon noch pending; connector branch head bereits f226e4e7 mit core/demo/console-Werten, aber console=0ea88f55 (alter PR#7-Head, refreeze C: core=2adae47a/demo=896dc3cb STALE)

- Eigener Precheck mit f226e4e7: FAIL `ValueError: immutable Core/Connector/Console/Demo candidate differs` (scripts/ecosystem_release_pins.py:27; Bedingung Zeile 25: console_candidate[k] muss connector-candidate[k] fuer schema/release/core/demo exakt spiegeln). Console-Branch work/v0.4.26-integrations-docs existierte live noch nicht.
- Befund wurde q59/zusaetzlich dokumentiert; q59c Korrigatur folgte (B3).

### B2: Polling (9x60s) auf q59-DONE + Trigger-Erfuellung (~19:10-19:13Z)

- q59-candraemon.md:38-41: `## Q59-CANDRAEMON-DONE`, connector branch `work/v0.4.26-integrations-docs@df1f4378d8c7fdc3e174a1e9dc8ad0155a955571`, resolver-precheck PASSED; q59-B-Korrektur: console-Feld von 0ea88f55 -> `69d1d14007fa43dc0c0457c3347280be7f802818` (neuer console work/v0.4.26-integrations-docs HEAD, live verifiziert via ls-remote).
- Eigener unabhaengiger Precheck (identische Argumente wie Source-Job, Skript 1:1 von develop, read-only gh api): exit=0, Ausgabe deckungsgleich mit q59-JSON:
  `{"connector_ref":"df1f4378d8c7fdc3e174a1e9dc8ad0155a955571","console_ref":"69d1d14007fa43dc0c0457c3347280be7f802818","demo_ref":"25c319697fd3d89d894943e0f44cd3d5dde8b198","core_ref":"1861ed07888a1ec22c218ca491d23ee54f48e261"}`

### B3: DISPATCH (19:13:26Z)

`gh workflow run .github/workflows/pre-release.yml --repo mcpdev80/baseharbor --ref develop -f tag=v0.4.26 -f ref=1861ed07888a1ec22c218ca491d23ee54f48e261 -f demo_ref=25c319697fd3d89d894943e0f44cd3d5dde8b198 -f consumer_ref=df1f4378d8c7fdc3e174a1e9dc8ad0155a955571`
-> Run **40009690268**: https://github.com/mcpdev80/baseharbor/actions/runs/40009690268

### B4: f29d Coordination-Hinweis (prompt: prompts/f29d.txt, ~19:35Z)

- Prepare bleibt vom Owner AUSDRUECKLICH pausiert: keine prepare-Inhalte geaendert, keine Prepare-Schritte ausgefuehrt (bestaetigt fuer diese Lane).
- f29d bestaetigt den bereits laeuft gehaltenen Dispatch (kein Re-Dispatch = retry ohne Grund). Job = inputs-Verify (erledigt, Step 4) + Run-Beobachtung.
- INCIDENT-Hinweis (coordinator-plan.md o85, 19:28Z): Root-Repo-git-Arbeit verboten. Mein erster f29c-Evidence-Stand (B1-B4a) wurde durch den Root-Repo-Checkout im Rahmen der o85-Recovery um 19:27:55Z zerstummelt (Datei auf HEAD 7a9d91cb zurueckgesetzt; Bestraetigung: Working tree = 64-Zeilen-Version). Re-Persist hiermit als Datei-Schreibvorgang (kein git im Root).

### B5: Run 40009690268 Live-Stand (19:36Z, in_progress) — ERSTE ECHTE MATRIX-AUSFUEHRUNG (Source SUCCESS, keine PENDING-FREEZE-Fehler mehr)

Bisher completed: source SUCCESS; Adoption alle 4 SUCCESS; Candidate·Runtime Image SUCCESS; Static 5x SUCCESS; Integration core-bootstrap(docker)/browser-terminal/configuration-policy/public-contracts SUCCESS.
Bisher FAILED (Details + Logbelege folgen in B6):
- Heavy · docker · data-capabilities; Heavy · podman · data-capabilities
- Integration · integration/docker/remote-target; Integration · integration/podman/remote-target
- Integration · integration/static/live-console
- v0.4.21 HA · data · podman (+ Aggregat "v0421 ha groups")
### B6: Run 40009690268 FINAL (completed failure, 19:13:26Z -> ~19:53Z; 72 Jobs: 60 success / 11 failure / 1 skipped)

https://github.com/mcpdev80/baseharbor/actions/runs/40009690268 — KEIN Retry ausgeführt, KEINE Produkt-Commits getätigt.

Failed-Job-Ledger (Datei-/Zeilenbelege aus Job-Logs via gh api jobs/<id>/logs):

| Job (id) | Fehlerstelle | Beleg |
|---|---|---|
| Heavy docker data-capabilities (120610707483) | `RuntimeError: new-edit-new-sql-remove exceeded no-output timeout: elapsed=105s no-output=90s limit=90s` | Log:2079, Exit1 Log:2100; Runner-Pfad baseharbor-demo-reference/tests/guided/drive.py:172 (via Journey-Log 120610706902:4409-4420 identische Signatur) |
| Heavy podman data-capabilities (120610707544) | dito, elapsed=125s | Log:1954, Exit1 Log:1975 |
| Integration docker/remote-target (120610189771) | `Private integration rejected: full authenticated consumer qualification is required.` | Log:229-230; Quelle scripts/pre-release-private-integration.py:91 (Maskierung :89-91) |
| Integration podman/remote-target (120610189906) | dito | Log:229-230 analog |
| Integration static/live-console (120610190003) | dito | Log:229-230 |
| Reference Journey podman (120610706811) | `demo-guided FAIL` durch no-output-timeout im Wizard (`baha app edit`, rules "no"), Traceback tests/guided/new_edit.py:253/:274 | Log:3137-3150, FAIL Log:3232, RESULT FAIL Log:3249 |
| Reference Journey docker (120610706902) | `demo-data-capabilities FAIL` (gleicher RuntimeError, elapsed=115s; tests/data-capabilities/new_edit.py:293 -> guided/new_edit.py:253 -> drive.py:172); `demo-backup-restore FAIL` als Cleanup-Kaskade (Log:4753; "shared application runtime is still running" Log:4421) | Traceback Log:4409-4422 |
| v0.4.21 HA data podman (120610189683) | `TestExistingControlPlaneRestartRequiresAndUsesRecoveryFile`: OpenBao unseal `Get https://127.0.0.1:8200/v1/sys/seal-status: connect: connection refused`, FAIL 126.37s | Log:351 (runtime_restart_integration_test.go:25), FAIL Log:352-355 |
| v0421 ha groups / runtime heavy gates (120616183866/120628755565) | Aggregat "Require both runtime matrices" | Folge der obigen Zeilen |
| Evidence · Candidate Manifest (120628790390) | "Require complete release approval" | Final-Aggregat; Folge |

### B6a: Zwei distinkte Root-Cause-Familien (keine Eigenmacht, nur Befund)

1. **Private-Consumer-Authentication (remote-target x2, live-console):** Die Resolver-/Pins-Kette ist gruen (Source SUCCESS, pins authentifiziert), ABER die qualifizierten `release-integration.yml`-Receipts fehlen auf den BRANCH-HEAD-Pins:
   - Connector: Run 40009467233 auf df1f4378 completed SUCCESS, aber nur Job "resolve"; "Integration · ${{ matrix.gate }}" = **skipped**, Artefakte = **0** (gh api runs/40009467233/artifacts = []) ⇒ kein Qualifications-Receipt fuer die 7 items aus scripts/private_consumer_evidence.py QUALIFICATIONS[:14-17].
   - Console: commit 69d1d140 hat **ueberhaupt keinen** release-integration.yml-Run (liste laeuft bei ~38154889438/918ca9e2 von 2026-10-15).
   ⇒ Naechster Trigger dafuer: vollstaendige release-integration-Qualifizierungs-Laeufe auf den finalen Branch-HEADs (q59/q80-Kette, Owner-Entscheid), keine Aenderung an dieser Lane moeglich (BRANCH-ONLY, keine Produkt-Commits hier).
2. **Wizard-Hang `baha app edit` (Heavy data-capabilities x2 + beide Reference Journeys):** identische Signatur `exceeded no-output timeout (limit=90s)` in den gefrorenen Demo-Referenz-Tests (drive.py:172). Deckt sich mit der offenen appid/env-robusten Test-Defekt-Kette (baseharbor PR #1046/#1047 OPEN; #1039 hatte nur das Step-Budget 8->25min erhoeht, nicht den 90s-no-output-Cap im TTY-Treiber). HA-data-podman (OpenBao restart unseal) ist ein drittes, separates Muster.

### B7: Matrix-Ergebnistabelle runtime x gate (Ergebnis wie beobachtet; KEIN gruen-Konsens)

| Gruppe | docker | podman |
|---|---|---|
| Source · Contracts · Docs | SUCCESS (shared) | SUCCESS (shared) |
| Light (connectivity, failure, full-destroy, guided, lifecycle, policy, reconciliation) | 7/7 SUCCESS | 7/7 SUCCESS |
| Heavy (11 Gates) | 10/11 SUCCESS, **data-capabilities FAIL** | 10/11 SUCCESS, **data-capabilities FAIL** |
| Static (agent, config-matrix, init, mcp, shell-ux) | 5/5 SUCCESS | — |
| Integration docker (core-bootstrap, coreless-lifecycle) / podman (core-bootstrap, coreless-lifecycle) / static (browser-terminal, configuration-policy, public-contracts) | SUCCESS | SUCCESS |
| Integration remote-target | **FAIL (private consumer auth)** | **FAIL (private consumer auth)** |
| Integration static/live-console | **FAIL (private consumer auth)** | — |
| Reference Journey Full E2E | **FAIL (wizard-hang + backup-restore-Kaskade)** | **FAIL (wizard-hang demo-guided)** |
| v0.4.21 HA (data, identity, observability, routing) | 4/4 SUCCESS | 3/4, **data FAIL (OpenBao restart)** |
| Adoption (conformance, realworld-compose/kubernetes/quadlet) | 4/4 SUCCESS | — |
| Candidate · Runtime Image | SUCCESS | SUCCESS |
| Evidence · Candidate Manifest (final) | **FAIL (Require complete release approval)** | — |

Artefakte (Evidence-Links, run 40009690268, attempt 1): https://github.com/mcpdev80/baseharbor/actions/runs/40009690268/artifacts — u.a. `pre-release-final-evidence-40009690268-1` (id 12528641712), `pre-release-gate-<runtime>-<gate>-evidence-40009690268-1` (insbes. docker/podman data-capabilities, control-plane-recovery, coreless-lifecycle), `pre-release-reference-journey-evidence-{docker,podman}-40009690268-1` (12527667653 / 12527678192).

**Konsens-Bewertung fuer Prepare/Qualify: NICHT erreicht** (Mission-Schritt "Wenn alles gruen" ist nicht erfuellt). Prepare bleibt zusaetzlich ausdruecklich in Owner-Pause (f29d). Freigabefaehige Teile: komplette Light/Static/Adoption/Kern-Integration + Heavy ausser data-capabilities + HA ausser data·podman. Offene Blocker-Linien: (a) release-integration-Qualifications-Receipts auf df1f4378/69d1d140, (b) app-edit-Wizard no-output-Hang, (c) OpenBao-Restart-Unseal podman HA.

## END F29-W1FINAL-DONE (run-id 40009690268; matrix-tabelle B7; NICHT-ALL-GREEN — 11 failed Jobs, Root-Cause-Familien B6a; kein Retry, keine Produkt-Commits, Prepare unbetroffen/pausiert)
