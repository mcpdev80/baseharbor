# F30 – W1 Run Forensics (READ-ONLY)

Task: Log-Forensik zu W1 run 40009690268 (mcpdev80/baseharbor, Pre-Release Validation, SHA 1861ed07888a1ec22c218ca491d23ee54f48e261). Read-only; KEINE Produktänderungen.
Evidence-Pfad: dieses File. Lane-Tag: [wip:f30]
Failed Jobs: 120610189771 (Integration docker/remote-target), 120610189906 (Integration podman/remote-target), 120610190003 (Integration static/live-console), 120610706811 + 120610706902 (Reference Journey Full E2E podman/docker), 120610189683 (v0.4.21 HA data podman), 120610707483 + 120610707544 (Heavy data-capabilities docker/podman); Aggregate 120616183866/120628755565 + Candidate Manifest als Folge.

## PLAN

- [x] 0. Evidence-File + PLAN/todowrite anlegen
- [x] 1. Run 40009690268 inventarisieren: Jobs, Status, Job-IDs (`gh run view`)
- [x] 2. Cluster A: private consumer auth – Jobs "Integration remote-target" (docker+podman) + "static/live-console" Log-Forensik
- [x] 3. Cluster B: wizard-hang + backup-restore-Kaskade – "Reference Journey Full E2E" (beide Runtimes)
- [x] 4. Cluster C: OpenBao – v0.4.21 "HA data gate" (podman) + "data-capabilities" (beide Runtimes)
- [x] 5. Repo-Stellen lokalisieren (Datei:Zeile der Fehlstellen) je Cluster
- [x] 6. Drei Diagnose-Abschnitte in dieses File schreiben (Log-Auszug, Fail-Grund, Root-Cause-Hypothese, Minimal-Fix, keine Umsetzung) – Pflichtblock gem. f30b.txt
- [x] 7. Abschluss: END F30-W1FORENSICS-DONE mit 3 clustern

## Cluster A – private consumer auth (Integration remote-target docker+podman; static/live-console)

Job-Log-Auszug (remote-docker.log Zeile 229–230; identisch in remote-podman.log und live-console.log):

```
19:21:28 Run python scripts/pre-release-private-integration.py --gate integration/docker/remote-target --candidate "$CANDIDATE_SHA" ...
19:21:30 Private integration rejected: full authenticated consumer qualification is required.
19:21:30 ##[error]Process completed with exit code 1.
```

Repo-Stellen (baseharbor@1861ed07): scripts/pre-release-private-integration.py:91 (sanitized SystemExit), :46 → scripts/private_consumer_evidence.py:181–188 (genau EIN successful `Integration · <origin-gate>` Job + non-expired Artifact gefordert), :173 ('no trusted exact-source push origin').

Fail-Grund: Die Verifikation des Connector-Pins df1f4378d8c7fdc3e174a1e9dc8ad0155a955571 findet dessen release-integration-Push-Run 40009467233 (completed, success), ABER der Qualifizierungs-Job ist dort SKIPPED: `gh api .../runs/40009467233/jobs` liefert nur `resolve` (success) + `Integration · ${{ matrix.gate }}` (skipped, leere Matrix); Run hat 0 Artifacts. Console-Pin 69d1d14007fa43dc0c0457c3347280be7f802818 hat gar keinen release-integration-Run. Damit schlägt jede der drei Qualifications fehl.

Root-Cause-Hypothese: baseharbor-node-connector/.github/workflows/release-integration.yml:366 (am Pin df1f4378) gated die Matrix mit `if: contains(github.event.head_commit.message, '[ecosystem-runtime]')`. Der Pin wurde auf einen manuellen Freeze-Commit gesetzt („q59b: freeze console field with FROZEN branch-HEAD 69d1d140", 19:11:40Z) OHNE Marker → Qualifizierungs-Matrix lief für den exakten Source nie. Der Pin-Record-Prozess (q59b/scripts.ecosystem_release_pins) ist nicht mit dem Trigger-Vertrag des Connector-CI synchronisiert.

Minimal-Fix-Vorschlag (KEINE Umsetzung): Connector-Pin neu auf einen Commit-recorden, dessen push-run `[ecosystem-runtime]` im Message trägt und die remote-target-Jobs + `private-integration-*`-Artifacts produzierten; alternativ/ergänzend freeze-/release-Commits im Connector standardmäßig mit Marker erstellen ODER release-integration.yml:366 auf push-Events an release-gebundenen Branches unabhängig vom Message-Marker laufen lassen. Dann alle drei A-Gates ohne Produktänderung neu dispatchen.

## Cluster B – wizard-hang + backup-restore-Kaskade (Reference Journey Full E2E, beide Runtimes)

Job-Log-Auszüge (journey-podman.log 3137–3155, journey-docker.log 4409–4419):

```
[progress] new-edit-new-zero-no: still running · elapsed=45s · no-output=45s   (podman, Start-Stall)
... needs-menu + Footer, "enter", Menu-Redraw, dann Stille ...
RuntimeError: new-edit-new-zero-no exceeded no-output timeout: elapsed=139s no-output=90s limit=90s
RuntimeError: new-edit-new-sql-remove exceeded no-output timeout: elapsed=115s no-output=90s limit=90s  (docker)
```

Backup-Restore-Kaskade (journey-docker.log 4677):

```
19:51:20 duplicate shared default provider: postgresql: 2 physical members across projects
19:51:42 demo-backup-restore failed, but the shared application runtime is still running; continuing full acceptance.
```

Repo-Stellen: baseharbor-demo@25c319697fd3d89d894943e0f44cd3d5dde8b198: tests/guided/new_edit.py:253 (run_tty `baha app edit`), tests/guided/drive.py:172 (no-output-Kill), tests/data-capabilities/new_edit.py:293 ("remove"-Pfad), tests/native-default-topology.py:77 (Invarianz-Guard). Baseharbor-Kern: cmd/baha/app_edit.go:79–96 (refresh → appMachinePropose mit Read/Choices/Plan/Preview), cmd/baha/app_wizard_controller.go:202–245 (Step→freshFacts), :327–334 (View rendert Needs MENÜ OHNE Provider-Zeilen, wenn choicesForSelection Fehler liefert — genau das Bild im letzten Redraw).

Fail-Grund: Nach Enter auf dem Needs-Picker bleibt `baha app edit` >90 s komplett stumm; der „Read-only decision preview" erscheint nie → TTY-Driver killt per no_output_timeout. Podman: demo-guided FAIL → alle Folgenden scopes BLOCKED. Docker: Hang im data-capabilities-scope; dessen Runtime bleibt stehen (Harness „continuing full acceptance"), und der backup-restore-scope zählt dann 2 physische postgresql-Members über zwei Projekte → Guard-SystemExit → FAIL. Kaskade = Fail-Weiterlaufen mit latentem Shared-Provider-State, nicht primär Backup-Logik.

Root-Cause-Hypothese: Die Read-only-Preview-Autoritäten (authorities.Read/Plan; app_edit.go:89) sammeln live den kompletten Target-Footprint inkl. authentifizierten Provider-Health-Runds (S3 Put/Get, OpenBao AppRole, broker-mTLS, postgres-Rollengrenzen — identisch dem Checks-Block im baha-up-JSON im selben Job) SERIELL und ohne Progress-Ausgabe; auf geladenen CI-Runnern mit vollem Daten-Stack überschreitet das 90 s. Der 45-s-Stall beim Podman-Start und die Provider-losen Redraws (choicesForSelection-Fehlerpfad) stützen „langsame/fehlgeschlagene Runtime-Enumeration" statt Deadlock in der Bubbletea-UI selbst.

Minimal-Fix-Vorschlag (KEINE Umsetzung): In Step/freshFacts (app_wizard_controller.go:202ff) ein kurzes „[wizard] computing read-only preview…" Heartbeat ausgeben und jede Provider-Abfrage mit eigenem bounded Context belegen (Parallelisierung oder Skip+Degradation im read-only Pfad statt voller Auth-Health-Checks). Test-seitige Alternative (nicht bevorzugt): no_output_timeout in demo drive.py erhöhen — verdeckt nur die Produkt-Latenz.

## Cluster C – OpenBao (v0.4.21 HA data gate podman; data-capabilities beide Runtimes)

Job-Log-Auszug (ha-data-podman.log 335–337):

```
19:24:51 runtime_restart_integration_test.go:25: restart with persisted target recovery reference:
  unseal OpenBao after control-plane start: inspect OpenBao HA member openbao-member-1:
  podman exec bh-podman-restart-ci-shared-openbao-member-1: Error checking seal status:
  Get "https://127.0.0.1:8200/v1/sys/seal-status": dial tcp 127.0.0.1:8200: connect: connection refused
19:25:21 --- FAIL: TestExistingControlPlaneRestartRequiresAndUsesRecoveryFile (126.37s)
```

datacap-podman.log 1937–1954 / datacap-docker.log 2069–2079: identischer Wizard-Hang wie Cluster B (`new-edit-new-sql-remove exceeded no-output timeout`, 125s/105s vs. 90s). In diesen beiden Jobs gibt es KEINE OpenBao-Fehlermeldung — OpenBao-Setup läuft vorher durch („OpenBao initialized and unsealed", AppRole-Check OK). Die data-capabilities-FAILs sind also derselbe Cluster-B-Bug im Heavy-Gate, nicht eigenständig OpenBao.

Repo-Stellen: cmd/baha/runtime_restart_integration_test.go:25 (Fehlerstelle Test), cmd/baha/runtime_start.go:434 (Inspect) und :454 (Unseal-Fehlerpfad), internal/openbao/bootstrap.go:466 (memberState-Fehler), :93–116 (einzelner 5-s-Exec-Probe gegen 127.0.0.1:8200, KEIN Retry; Exit≠0/2 → error).

Fail-Grund: Im Restart-Akzeptanztest gelingt der status-Probe in `Inspect` (Sonst käme die Inspect-Fehlermeldung), aber unmittelbar danach im `Unseal`-Pfad beantwortet openbao-member-1 denselben In-Container-Probe mit connection refused → fail-fast, Task-FAIL.

Root-Cause-Hypothese: Race-Fenster nach Control-Plane-Restart unter rootless Podman: OpenBao bindet den 127.0.0.1:8200-Listener erst spät/vorübergehend neu (Storage-/Raft-Rejoin, nachdem der Postgres-HA-Endpunkt wieder da ist — genau die Dependency-Order, die der Kommentar in runtime_start.go:427–430 beschreibt). `unsealAllMembersWithKey` (bootstrap.go:472ff) behandelt den Einmal-Probe als autoritativ; eine wackelnde Sekunde zwischen Inspect und Unseal tötet den Restart-Flow.

Minimal-Fix-Vorschlag (KEINE Umsetzung): memberState/inspectMemberState im Unseal-Pfad mit bounded Retry (z.B. gleicher Zeitbudget-Ansatz wie waitForOpenBaoExecReady, 5s-Intervall bis Deadline) statt Einmal-Probe aufrufen, oder `Unseal` erst nach N aufeinanderfolgend erfolgreichen seal-status-Probes starten. Für die data-capabilities-FAILs gilt der Cluster-B-Fix (Preview-Latenz/Heartbeat) — kein separater OpenBao-Fix nötig.

## Quellen / Verifikation

- Logs: `gh api repos/mcpdev80/baseharbor/actions/jobs/<id>/logs` (Kopien: /tmp/opencode/f30/, temporär).
- Consumer-Runs/Jobs: `gh api .../actions/workflows/release-integration.yml/runs` + `/runs/40009467233/jobs|artifacts` (read-only).
- Code-Stellen: baseharbor@1861ed07 via `git show/grep`, baseharbor-demo@25c31969 via `git show` im Workspace-Checkout.
- Kein Produktbaum verändert; nur diese Evidence-Datei geschrieben.

END F30-W1FORENSICS-DONE mit 3 clustern

## f30c – Verifikation + W1-Dispatch (Gemäß dispatch-Prompt evidence/board-2026-10-22/prompts/f30c.txt)

### PLAN (f30c)

- [x] 0. PLAN + todowrite ([wip:f30]) in diesem File ergaenzt
- [x] 1. gh api contents: docs/releases/v0.4.26.consumer-pins.json in ref abbcc9642118 verifiziert (blob aec9276c, schema baseharbor.consumer-evidence-pins/v1, gates remote-target docker/podman + live-console, connector df1f4378 / console 69d1d140; branch work/v0.4.26-restfix HEAD == abbcc964, compare zu 1861ed07: status=ahead, ahead_by=1)
- [x] 2. Workflow-Dispatch-Schema (develop-Quelle .github/workflows/pre-release.yml): inputs tag/ref/demo_ref/consumer_ref/only_gates bestaetigt; kein aktiver Run in concurrency-Gruppe baseharbor-release-qualification
- [x] 3. W1 dispatchen: gh workflow run pre-release.yml --repo mcpdev80/baseharbor --ref develop -f tag=v0.4.26 -f ref=abbcc964... -f demo_ref=25c3196... -f consumer_ref=df1f4378... → Run 40023245872
- [x] 4. Run-ID/Status verifizieren, Parameter dokumentieren → **Run 40023245872 FEHLERT nach ~13s im Job "Source · Contracts · Docs" / Step "Resolve release candidate"** (job id 120652899679)

### Ergebnis f30c (Wahrheit, kein Scheinerfolg)

Dispatch wie vorgeschrieben ausgefuehrt; die Vorab-Verifikation (Punkt 1) war erfuellrt. Der Run ist aber deterministisch in Source fehlgeschlagen:

```
20:58:14 fatal: couldn't find remote ref refs/heads/release/v0.4.26   (Toleriert: "Candidate follows the current develop release line.")
20:58:15 File ".../scripts/ecosystem_release_pins.py", line 27, in resolve_pins
20:58:15 ValueError: immutable Core/Connector/Console/Demo candidate differs
20:58:15 ##[error]Process completed with exit code 1.
```

Kette: Source-Step ruft `ecosystem_release_pins.py --core "$(git rev-parse HEAD)"` auf, HEAD = inputs.ref = abbcc964. Die Validatorin vergleicht core mit den consumer-seitigen Kandidaten-Dokumenten (scripts/ecosystem_release_pins.py:21–27): integration-candidate.json im Connector df1f4378 (gelesen via gh api, read-only) pinnt `"core": "1861ed07888a1ec22c218ca491d23ee54f48e261"` und baseharbor-core.ref in Demo 25c3196 enthaelt dieselbe SHA. `candidate['core'] != core` → ValueError. (Das 1861ed07 in der Evidence-Zeile des fehlgeschlagenen Runs ist nur der github.sha-Fallback des ci-evidence-Aufrufs, weil steps.release.outputs.candidate_sha leer blieb.)

Diagnose zur f30c-Praemisse: Die committed consumer-pins.json im restfix-Branch behebt Cluster A nicht auf diesem Weg. Das Feld wird im gesprueften Lauf sowieso zur Source-Laufzeit neu erzeugt; dass es in 1861ed07 fehlte, war nicht die Bruchstelle (Cluster-A-Befund: Connector-Quell-Laeufe mit `[ecosystem-runtime]`-Marker und erfolgreichen remote-target-Jobs + Artifacts fehlen). Mit ref=abbcc964 kann W1 nicht durch Source kommen, weil das Candidate-DAG core==1861ed07 verlangt. Ein W1-Dispatch mit restfix-ref ist nur konsistent, wenn gleichzeitig Connector (und Demo baseharbor-core.ref) auf abbcc964 um pinsiert werden und deren release-integration-Push-Runs das Qualifizierungs-Matrix-Ergebnis liefern — das ist ein Planning/Owner-Entscheid, hier nicht umgesetzt.

Run: https://github.com/mcpdev80/baseharbor/actions/runs/40023245872 (workflow_dispatch, head develop, ref input abbcc964). Endergebnis: completed/failure — eigentliche Bruchstelle nur "Source · Contracts · Docs"; die Aggregate (light/heavy/v0421 gates, Evidence · Candidate Manifest) sind Folgefehler durch den fehlenden Candidate.

END F30C-W1DISPATCH-VERIFIED — Dispatch gem. Prompt ausgefuehrt, Vorab-Contents-Check bestanden, Run 40023245872 fails deterministisch an resolve_pins core-Mismatch (1861ed07 != abbcc964).

