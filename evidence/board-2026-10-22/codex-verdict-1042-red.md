# PR #1042 — Diagnose

ROLLE: REVIEW (Diagnose, kein Merge).

## PLAN
- [x] 0. Dispatch mit `cat` lesen, Autorisierung und Scope prüfen; Plan vor Diagnose festhalten.
- [x] 1. Aktuellen PR-/develop-/Planning-Stand und konkrete fehlgeschlagene Check-Runs sichern.
- [x] 2. Live-Run-Logs und genau verwendete Workflow-/Selector-Versionen untersuchen; Fehlerursache bestimmen.
- [x] 3. SHA-genaues Verdict mit Worker-Rezept (Datei, Änderung, Tests, Stop-Kriterium) schreiben und HEAD erneut prüfen.

Tool-Grenze: Diese Codex-Sitzung stellt kein natives TodoWrite-/Plan-Tool bereit (Toolkatalog geprüft). Daher keine native TodoWrite-Ausführung behauptet. Der Dispatch enthält keinen expliziten nummerierten Schritt 0; obiger Schritt operationalisiert die Owner-Anweisung.

## repo-rule compliance
Nur unabhängige Diagnose und Evidence. Keine Produktänderung, Implementierung, CI-Neustarts, Merge-, Release- oder Deployment-Aktion. Bestehende Claims bleiben unverändert. Cost-Stop und Maintainer-Pause bleiben wirksam.

## Startprüfung
`basics-check.py`: Events/Reminders, idle Lanes f19-sync1039 und o51-sync1042b, Pool-Unterbelegung, alter Coordinator-Plan. Diese Review-Lane übernimmt keine Coordinator-Rolle und startet keine Worker. Flags werden gelesen und innerhalb des Diagnose-Scopes eingeordnet; fremde Lanes und Coordinator-Plan werden nicht verändert.

## Verdict: CHANGES-REQUIRED — frische konsistente Integrations-Evidence

**Rootcause: veraltete Event-Base und nicht dazu passender synthetischer Merge, Fehlercode `integration_sha_mismatch`.** Der rote Run erreicht keine qualifizierbare Dateiliste. Ein passender Eintrag auf dem heutigen develop kann diesen historischen Run nicht nachträglich ändern. Kein Beleg für einen neuen Selector-Defekt; keine Selector-Codekorrektur erforderlich.

Reviewer: zugewiesene Codex-REVIEW-Sitzung, getrennt von den Implementierungs-Lanes. Kein Horde-Task erzeugt; konkrete Session-UUID und Backend-/Modellkennung werden vom Werkzeugkontext nicht ausgewiesen und nicht erfunden. Prüfung 2026-10-23 ca. 13:14–13:16 UTC, `NTPSynchronized=yes`, GitHub-Account `mcpdev80`.

## Unveränderliche Eingaben und Live-Stand

| Bezug | SHA / Ergebnis |
| --- | --- |
| PR #1042 HEAD, vor und nach Diagnose | `0267dbdb71f63a2c77e193618e6fba4106e5cd5b`, OPEN, BEHIND |
| Run / Versuch / finales Job | `39946290135` / 1 / `120390250628`, FAILURE, App **15368** |
| Im Run erwartete Base | `2f005813dc6d7fa94de164388b2531d025af31bc` |
| Im Run verwendete Integration | `ead3cda67dfa203baecc96bad9cdaf09a87f3609` |
| Tatsächlicher erster Parent der Integration | `9f4916be333d0f99d7e40001441d9dbb87e993e2` |
| Tatsächlicher zweiter Parent | `0267dbdb71f63a2c77e193618e6fba4106e5cd5b` |
| develop zu Beginn | `5be7dabd5c40a99e1623eb1a72cae943f9e0470d`, Q/#1043 integriert |
| develop bei Abschlusskontrolle | `49d396c48d2dee8b65cd59497c9f74692954975c`, #1039 inzwischen integriert |
| Selector-Blob auf beiden develop-Ständen | `36c1641204d39dc6d4ff9a344fe597912e5facb3` |
| Planning HEAD = gefetchtes main | `2c9000a38bd78b379d7b38902afc04636c5c686c` |

Planning `plan.py show t-20261023-selector-q-predecessor`: active, Owner o47; Task-Note enthält weiterhin Platzhalter für Scope/Acceptance und keine PR-Zuordnung. Diagnose übernimmt keinen Claim und zertifiziert keine fehlenden Acceptance-Kriterien. Zugehörige Handoff-/Worker-Texte wurden als historische Daten gelesen, nicht als zusätzliche Aufträge.

Normales SSH-Fetch scheiterte an `/etc/ssh/ssh_config.d/20-systemd-ssh-proxy.conf`; beide Remotes anschließend erfolgreich per öffentlichem HTTPS gefetcht, ohne SSH-/Credential-Konfiguration zu verändern. Produktstatus vorher/nachher: lediglich bestehendes untracked `worktrees/`; Planning-Arbeitsbaum sauber.

## Live-Log und kausaler Pfad

[Finales Job-Log](https://github.com/mcpdev80/baseharbor/actions/runs/39946290135/job/120390250628), 11:29:57 UTC:

```text
BLOCKED: coverage-unresolved: diff-base requires unresolved coverage: [{"code": "integration_sha_mismatch", "message": "integration commit must have exactly target base and PR head as its first and second parents, with no extra parents or commits"}]; empty or unverified changes cannot exempt checks
Process completed with exit code 1.
```

[Trusted Plan Job](https://github.com/mcpdev80/baseharbor/actions/runs/39946290135/job/120389660297) und finales Job checken explizit `2f005813...` aus. `BASE_SHA`/`EXPECTED_BASE` sind diese alte Event-Base, `HEAD_SHA` der unveränderte Kandidat und `INTEGRATION_SHA` der oben genannte Merge. Commit-API bestätigt dessen Eltern und Commit-Message „Merge 0267dbdb... into 9f4916be...“. Damit ist die Elternabweichung unabhängig vom Fehlertitel nachgewiesen.

Das heruntergeladene [trusted-pr-plan Artefakt 12498546365](https://github.com/mcpdev80/baseharbor/actions/runs/39946290135/artifacts/12498546365) bestätigt:

```json
{"coverage_status":"unresolved","broad_required":true,"selected_gates":[],"required_validation_checks":[],"reasons":[],"unresolved_coverage":["diff-base requires unresolved coverage: [{\"code\": \"integration_sha_mismatch\", \"message\": \"integration commit must have exactly target base and PR head as its first and second parents, with no extra parents or commits\"}]","empty or unverified changes cannot exempt checks"]}
```

ZIP-SHA256: `94403aac37af33cd5e3052a8a7df46920c48a3ac10804a4a5351505356177422` (entspricht Upload-/Download-Log). Logs wurden direkt über `gh api repos/mcpdev80/baseharbor/actions/jobs/<id>/logs` gelesen; `gh run view --log` lieferte hier keinen Text. Keine Schlussfolgerung wurde allein aus dem Step-Titel abgeleitet.

1. `.github/workflows/ci.yml:444–457` bindet Checkout/Resolver/Planner an Event-Base, Head und `github.sha`; `:790–816` bindet das finale Job an dieselben Inputs.
2. `scripts/pr-diff-base.py:132–145` fordert exakt `[integration, target_base, head]` und verwirft diesen Merge **vor** der Dateilistenberechnung (`:156`). Das ist die korrekte fail-closed-Reaktion.
3. `scripts/pr-gate-plan.py:700–721` übernimmt den ungelösten Diff und die leere/unverifizierte Dateiliste. Die aktuelle Dispatch-Qualifikation (`:249–255`, `:763–765`, Bedingungen `:792–798`) kann so nicht greifen. Zudem enthält die tatsächlich ausgecheckte alte Base den Q-Dispatch-Eintrag noch nicht.
4. Das finale Job blockiert den ungelösten Plan. Focused Python, Trusted Plan und Workflow policy waren SUCCESS. `SELECTED_RUNTIME_REQUIRED=false`, Runtime SKIPPED: **kein fehlender angeforderter Runtime-Test**, sondern Folge des ungelösten Plans. „Enforce every required check (unqualified runtime remains blocked)“ ist der generische Step-Titel.

Abgrenzung: Strict-up-to-date ist zusätzlich live aktiv (Ruleset 25223521, Kontext `PR Selected Checks`, App 15368), und der Branch ist BEHIND. Die konkrete Job-FAILURE stammt jedoch aus dem nachgewiesenen Input-/Elternfehler, nicht direkt aus der GitHub-Merge-Sperre. Kein Cleanup-Traceback wie #1043; Focused Python war grün. Die historische o51-Behauptung eines unvermeidbaren Runtime-Qualifikationsblocks wird durch das tatsächliche Log widerlegt.

## Präzises Worker-Rezept

1. Bestehender zugewiesener Branch-Owner: Branch `fix/atomic-bare-sha-dispatch` und aktiven Write-Owner verifizieren; insbesondere nicht blind den lokalen Q-HEAD `81cb6365` verwenden. Remote-HEAD muss noch der eingefrorene Source `0267dbdb...` sein. Aktuelles develop neu fetchen und pinnen (zuletzt `49d396c4...`). Fremde Arbeitsstände erhalten.
2. **Änderung: normaler sauberer develop-Merge in den Source-Branch**, Source als erster Parent, aktuelles develop als zweiter Parent; danach normaler Push. Keine Rebase-/Cherry-pick-/Amend-Korrektur, kein Force-Push, kein bloßer Leercommit. Die bestehende Merge-Chain-Prüfung `scripts/pr-gate-plan.py:531–567` akzeptiert nur Git-exakte saubere Merge-Trees aus trusted-target-Historie. Bei Konflikt oder manuell nötiger Inhaltsauflösung stoppen und separate bounded Prüfung anfordern, statt automatisch die Source-Qualifikation fortzuschreiben.
3. **Dateien:** kein neuer Selector-Eintrag und keine Änderung an `scripts/pr-gate-plan.py`, `scripts/pr-diff-base.py` oder deren Sicherheitsbedingungen erforderlich. Die Integrationsänderung übernimmt die aktuellen trusted-develop-Inhalte. Die eigentliche Source-Änderung bleibt das vollständige Fünf-Dateien-Inventar bei `DISPATCH_GATE_REF_REVIEWED_FILES`: drei Workflows (`ci.yml`, `pre-release-runtime-suite.yml`, `targeted-demo-acceptance.yml`) plus `scripts/targeted-gate-ref.py` und `scripts/test_targeted_gate_ref.py`. Prüfen, dass Source-Inhalte und Inventar durch den Merge korrekt erhalten bleiben; keine branchlokale Selbstqualifikation hinzufügen.
4. **Gezielte Prüfungen im Worker-Worktree:** bestehende `python3 scripts/test_pr_diff_base.py` (enthält bereits `test_stale_event_base_rejected_and_normal_target_integration_restores_diff`, Zeile 183), `python3 scripts/test_targeted_gate_ref.py` und Planner-Regressionssuite. `ReviewedSourceQualificationTests.test_real_git_exact_sources_and_all_authority_mutations` deckt DISPATCH_GATE_REF samt sauberem Target-Merge ab (`scripts/test_pr_gate_plan.py:1635–1708`). Vor Push bestehende passende Workflow-policy-/Syntax-Prüfungen ausführen; kein neuer Test nötig, der nur die bekannte Elternbedingung dupliziert.
5. Frisches `pull_request`-Event und neues Integrations-SHA abwarten. Mit echten Git-Eltern prüfen: erster Parent = Event-Base = verwendeter trusted Checkout, zweiter Parent = neuer PR-HEAD; aktuellen develop-Stand zusätzlich abgleichen. Artifact muss `coverage_status=resolved`, keine unresolved coverage, vollständiges Fünf-Dateien-Inventar und exakt `ci-actionlint`, `ci-executor-verifier-tests`, `ci-policy`, `pr-planner-tests` als Required Validations liefern (CI_CHECKS + WORKFLOW_SYNTAX_CHECKS). Keine Native-/Release-Matrix pauschal starten.
6. Alle tatsächlich geforderten Checks für neue exakte Inputs und finales `PR Selected Checks` von App 15368 müssen erfolgreich sein. Veränderten Merge-Kandidaten SHA-genau prüfen lassen, erst dann gesonderte MERGE-Abnahme. Ein `--failed`-Rerun des alten Events ist **kein verlässliches Rezept**: seine Base/Integration bleiben unvereinbar; außerdem fehlt der alten trusted Base Q. Historische grüne Tests dürfen neue Input-Gates nicht ersetzen.

**Stop-Kriterien:** HEAD/Ownership unerwartet verändert; ungesicherte fremde Arbeit; Konflikt/manuell veränderter Merge-Tree; unpassende Eltern, Base oder Inventar; unresolved Plan; fehlende/rote/unklare Pflichtchecks. Bei weiterem roten Run zuerst dessen konkretes Log und neue Inputs lesen. Keine Schutzregeländerung oder CI-Umgehung. Diagnose allein erteilt keine Implementierungs-/Merge-Freigabe über bestehende Owner-Aufträge hinaus.

## Abschluss und Grenzen

Nur diese Evidence-Datei wurde geschrieben. Keine lokale Testsuite, Produktkorrektur, CI-Wiederholung, Worker-Zustellung, Planning-Statusänderung oder GitHub-Schreibaktion ausgeführt. Bestehende passende CI-/Quellbelege wurden gelesen; das Produkt-HEAD blieb unverändert. Live develop rückte durch eine andere Lane auf `49d396c4...`; Selector-Blob ist gegenüber `5be7dabd...` unverändert. Breitere historische Repo-Maintainer-Erlaubnis ändert den reinen REVIEW-Dispatch und den Bootstrap-Stop nicht.

Events/23 Reminders wurden gelesen: beobachtete Session-Discovery, andere Lanes/fehlende Pane-Beobachtungen, Pool-/Coordinator-Plan-Hinweise erfordern Coordinator-Aktionen. Diese sind außerhalb des Review-Auftrags und bleiben ausdrücklich offen; keine fremden Sessions entfernt oder neu beauftragt. Kein globales `--ack`, da damit ungelöste fremde Aktionen fälschlich quittiert würden. Diese Evidence erledigt nur DIAG-1042. Native TodoWrite bleibt mangels exponiertem Werkzeug unerfüllbar; die persistente PLAN-Checkliste ist vollständig.

END DIAG-1042-DONE

## ROLLE: REVIEW — DIAG2 PLAN

- [x] 0. Follow-up-Dispatch mit `cat` und bindende Policies lesen; Plan vor Diagnose ergänzen. Native TodoWrite ist weiterhin nicht exponiert.
- [x] 1. Aktuelle #1042-/develop-Identitäten und genau einen fehlgeschlagenen Run prüfen.
- [x] 2. Konkreten Fehler mit Selector-Nachfahrenregel und erfolgreichem #1039-Pfad vergleichen.
- [x] 3. Ein präzises Worker-Rezept mit Datei/Zeile/Änderung und Stop-Kriterium ergänzen; HEAD erneut prüfen.

### DIAG2 verdict: CHANGES-REQUIRED — zwei konkrete Input-/Graphfehler

2026-10-23 13:34–13:36 UTC, Account `mcpdev80`, NTP synchronisiert. **Die Prämisse „echter Source-first Sync“ trifft nicht zu.** Das heutige Log beweist weiterhin die alte Event-Base; die Commit-API beweist zusätzlich umgekehrte Sync-Eltern. Die vorige Diagnose des roten Jobs war richtig; mein damaliges Rezept war hinsichtlich der Event-Erneuerung unzureichend: Ein Push hat hier entgegen der erwarteten Aktualisierung die alte Base erhalten. Deshalb reicht „noch einmal syncen/pushen“ nicht.

Genau **ein** Run-Log gelesen: [Run 39961040150 / finales Job 120439929837](https://github.com/mcpdev80/baseharbor/actions/runs/39961040150/job/120439929837), FAILURE, Check-App 15368. Log-SHA256 `b64bf6487eccc34c6de137e44677d3403629ad50cd51152af31dbc783c48288d`.

```text
ref: 2f005813dc6d7fa94de164388b2531d025af31bc
EXPECTED_BASE: 2f005813dc6d7fa94de164388b2531d025af31bc
EXPECTED_HEAD: 1263b9e2e883019fdcf1e8ff4379f39ed4b11648
EXPECTED_INTEGRATION: 1021b41e69c7c8e69832b69a542f3c1198ad86c8
FOCUSED_JOB: success
VALIDATION_JOB: success
SELECTED_RUNTIME_REQUIRED: false
SELECTED_RUNTIME_JOB: skipped
BLOCKED: coverage-unresolved: diff-base requires unresolved coverage: [{"code": "integration_sha_mismatch", "message": "integration commit must have exactly target base and PR head as its first and second parents, with no extra parents or commits"}]; empty or unverified changes cannot exempt checks
BLOCKED: no required gates or validations
```

Commit-API (nicht aus Worker-Prosa übernommen):

| Commit | Erster Parent | Zweiter Parent |
| --- | --- | --- |
| #1042 synthetische Integration `1021b41e69c7c8e69832b69a542f3c1198ad86c8` | `49d396c48d2dee8b65cd59497c9f74692954975c` | `1263b9e2e883019fdcf1e8ff4379f39ed4b11648` |
| #1042 Sync-Kandidat `1263b9e2e883019fdcf1e8ff4379f39ed4b11648` | `49d396c48d2dee8b65cd59497c9f74692954975c` (**develop**) | `0267dbdb71f63a2c77e193618e6fba4106e5cd5b` (**Source**) |
| #1039 erfolgreicher Sync `7e53bc6804521b03c81673d181862a85c56ba3c9` | `75efeef89b61d848d5ea351877ba6423a31e58a6` (**Source**) | `5be7dabd5c40a99e1623eb1a72cae943f9e0470d` (**develop**) |

**Welcher Wert mismatcht jetzt?** `EXPECTED_BASE=2f005813...` gegen `integration^1=49d396c4...`. HEAD stimmt mit `integration^2` überein. Das ist derselbe Fehlerpfad `scripts/pr-diff-base.py:142–145`, bevor irgendeine Source-Qualifikation greifen kann. `.github/workflows/ci.yml:444,450,790,799` verwendet weiterhin die alte Event-Base. Es gibt keinen Beleg, dass nur das finale Job eine andere Base als der Planner verwendet; die entsprechende o51-Erklärung ist nicht durch dieses Log gedeckt. `PR · Trusted Plan: SUCCESS` besagt lediglich, dass ein Plan erzeugt wurde, nicht dass dessen Coverage resolved ist.

**Ist die Nachfahrenregel aktiv?** Im tatsächlich verwendeten trusted Checkout `2f005813...` fehlt der neue Dispatch-Eintrag. Auf aktuellem develop ist sie vorhanden: `scripts/pr-gate-plan.py:763–765,792–798` ruft `core752_reviewed_merge_chain` auf. Dort setzt `:554` den ersten Parent als `previous`, verlangt bei `:557–559` den zweiten Parent in trusted-target-Historie und folgt bei `:566` ausschließlich dem ersten Parent. `1263b9e2` erfüllt diese Regel nicht: sein zweiter Parent ist die noch nicht integrierte Source, sein erster Parent führt nicht zum Source-Anker. Das ist ein zusätzlicher, derzeit durch den früheren Inputfehler verdeckter Qualifikationsblock. Bloßes Auffrischen der Event-Base genügt deshalb ebenfalls nicht.

**Warum #1039 funktioniert:** korrekte Source-first-Eltern oben; [Plan-Artefakt 12502812098 aus Run 39956712319](https://github.com/mcpdev80/baseharbor/actions/runs/39956712319/artifacts/12502812098) hat `target_base_sha=5be7dabd...`, `head_sha=7e53bc68...`, `integration_sha=1f8deace04bcbf8e0e5f590c7e0c1b9db24986dc`, `coverage_status=resolved` und ausdrücklich die Reason „reviewed data-capabilities runtime gate step budget split: exact source hash, complete changed status set and proven clean trusted-target merge only“. PR-API bestätigt grünes finales Job `120424963665` und Merge `49d396c4...`. Dafür wurde kein zweites Run-Log gelesen.

### DAS eine Worker-Rezept: korrekt gerichteten Source-Sync als frischen PR qualifizieren

Eine einzelne Selector-Zeilenänderung behebt die beiden nachgewiesenen Fehler nicht. Deshalb kein erfundenes Ein-Zeilen-Produktfix und kein Austausch des akzeptierten Source-Ankers gegen `1263b9e2`. Das kleinste vollständige Rezept verändert Kandidaten-/Event-Erzeugung und lässt die Sicherheitsbedingungen unverändert:

1. **Datei/Zeile/Änderung im fehlerhaften Worker-Rezept:** `evidence/board-2026-10-22/o51-sync1042b.md:85` dokumentiert `git commit-tree … -p 49d396c4 -p 0267dbdb`. Diese Elternfolge muss bei der nächsten Kandidatenerzeugung **`-p 0267dbdb... -p <frisch gepinntes develop>`** lauten. Besser im neuen isolierten Branch direkt am eingefrorenen Source `0267dbdb...` beginnen und einen normalen sauberen `git merge --no-ff <develop>` ausführen. Nachweis vor Veröffentlichung: erster Parent Source, zweiter Parent aktuelles develop, Tree exakt sauberer `git merge-tree`-Tree, Restdiff exakt das bekannte Fünf-Dateien-Inventar. Fremde Arbeit und `1263b9e2` bleiben erhalten; kein Reset/Force-Push des bestehenden Branches. Ein weiterer Merge oben auf `1263b9e2` repariert dessen erste Elternkette nicht.
2. Den korrekt gerichteten Kandidaten unter **neuem Branch mit frischem PR gegen develop** veröffentlichen, damit der PR mit aktueller Base entsteht; den alten PR als Referenz erhalten, keine automatische Schließung/Branch-Löschung in diesem Diagnoseauftrag. So wird weder eine Non-fast-forward-Ersetzung von #1042 noch eine Änderung der trusted Selector-/Workflow-Policy benötigt. Vor CI-Auswertung neue PR-Base/HEAD und synthetische Integrationseltern verifizieren; bei erneut alter Base sofort stoppen. Kein erfolgloser alter-Event-Rerun und kein Leercommit.
3. **Datei/Zeile im Produkt:** `.github/workflows/ci.yml:444–457` und `:790–816` bleiben unverändert und müssen im frischen Event dieselbe aktuelle Base verwenden. `scripts/pr-gate-plan.py:249` bleibt am unabhängig akzeptierten Source `0267dbdb...`; `:531–567` und `scripts/pr-diff-base.py:142–145` nicht abschwächen. Ein robuster allgemeiner Workflow-Umbau zur Auflösung alter Event-Bases wäre ein eigener qualifizierter Task und löst den verkehrten Source-Graph allein nicht; er ist nicht das hier empfohlene Rezept.
4. **Vor dem Push gezielt prüfen:** bestehende Resolver-Regression `test_stale_event_base_rejected_and_normal_target_integration_restores_diff` und Planner-Regression `ReviewedSourceQualificationTests.test_real_git_exact_sources_and_all_authority_mutations` einschließlich DISPATCH_GATE_REF ausführen; mit echten lokalen Git-Inputs den Kandidatenplan gegen gepinntes develop prüfen. Akzeptiert wird ausschließlich `coverage_status=resolved`, vollständiges Fünf-Dateien-Inventar, keine unresolved coverage und Required Validations `ci-actionlint`, `ci-executor-verifier-tests`, `ci-policy`, `pr-planner-tests`. Danach frische authentifizierte Pflicht-CI auf dem tatsächlichen neuen Integrations-SHA; keine komplette Native-/Release-Matrix. Neuer Kandidat benötigt SHA-genaue unabhängige Prüfung.

**Stop:** Konflikt/manuelle Tree-Auflösung, unerwarteter Owner/HEAD, falsche Eltern, Base weicht von Integration^1 ab, falsches Inventar oder ungelöste/rote/fehlende Pflicht-Evidence. Kein Erfolgsversprechen allein aufgrund neuer PR-Erstellung: erst die obigen konkreten Inputs und Gates belegen die Reparatur. Dieses REVIEW-Rezept führt weder neue PR-Erstellung noch Integration aus und erweitert keine bestehende Autorisierung.

Abschlusskontrolle 13:35:44 UTC: #1042 weiterhin OPEN, HEAD `1263b9e2...`, API-Base `2f005813...`, Integration `1021b41e...`; develop `49d396c4...`. Planning main/HEAD `0a0826660a85facc942f0e3fb6d787354da9c115`. Nur diese Evidence ergänzt, keine Produktdatei/GitHub-/Planning-Mutation, keine Tests/Reruns oder Worker-Dispatches. Basics-Flags (24 Reminders, fremde Idle-Lanes, bedingte Pool-Unterbelegung, Coordinator-Plan) gelesen und außerhalb dieses engen REVIEW-Scopes beim Coordinator belassen; nicht fälschlich global quittiert. Native TodoWrite ist nicht verfügbar; Evidence-PLAN abgeschlossen.

END DIAG2-1042-DONE

## ROLLE: REVIEW — DIAG3 PLAN

- [x] 0. Dispatch und bindende Policies mit `cat` lesen, Review-Scope und PLAN festhalten. Native TodoWrite nicht verfügbar.
- [x] 1. Letzten tatsächlichen Check-Run/Attempt und App 15368 verifizieren; dessen Log und konkrete Integrationseltern lesen.
- [x] 2. Exakte Kontext-/Workflow-Herkunft mit Resolver-Elternbedingung und #1039-Grafik vergleichen.
- [x] 3. Fragen a–c mit einem belegten Worker-Rezept beantworten; finale Identitäten kontrollieren.

### DIAG3 — a) Tatsächliches integration_sha und Herkunft

Prüfung 2026-10-23 13:57–13:58 UTC; NTP synchronisiert. Neuester Check laut Commit-Check-API, abschließend erneut geprüft: **`120450991396`, App 15368**, Run **`39964376295`, Attempt 1**, Event `pull_request`, FAILURE. Das ist ein neuer Run gegenüber DIAG2, kein Attempt 2 des alten Runs.

Im [tatsächlichen letzten Job-Log](https://github.com/mcpdev80/baseharbor/actions/runs/39964376295/job/120450991396) steht:

```text
2026-10-23T13:50:48.3518988Z   ref: 2f005813dc6d7fa94de164388b2531d025af31bc
2026-10-23T13:50:51.6010872Z   EXPECTED_BASE: 2f005813dc6d7fa94de164388b2531d025af31bc
2026-10-23T13:50:51.6011355Z   EXPECTED_HEAD: 1263b9e2e883019fdcf1e8ff4379f39ed4b11648
2026-10-23T13:50:51.6011695Z   EXPECTED_INTEGRATION: 1021b41e69c7c8e69832b69a542f3c1198ad86c8
```

Danach wieder `integration_sha_mismatch`, `empty or unverified changes cannot exempt checks`, `BLOCKED: no required gates or validations`; Focused Python SUCCESS. Log-SHA256: `1dd0a576ad475b5945d1a0bde1316f856ca14fd0f3f4986abc63c676c0974195`.

**Antwort a:** In diesem Run ist `integration_sha` **`1021b41e69c7c8e69832b69a542f3c1198ad86c8`**. Herkunft laut ausgeführtem Workflow: `${{ github.sha }}`, nicht Source-Anker, nicht Branch-HEAD, nicht ein zur Diagnosezeit frisch gelesener PR-API-Wert. `INTEGRATION_SHA` geht an `pr-diff-base.py --integration`; die finale Prüfung erhält denselben Ausdruck als `EXPECTED_INTEGRATION`. Auf develop sind die Stellen `.github/workflows/ci.yml:452,456,801`; im #1042-Kandidaten aufgrund zweier zusätzlicher Testaufrufe `:453,457,803`. Base stammt aus `${{ github.event.pull_request.base.sha }}`, Head aus `${{ github.event.pull_request.head.sha }}`. Beide Workflow-Blobs wurden geprüft; dort keine hartkodierten alten SHA-Werte.

Das [Plan-Artefakt 12506087254 dieses letzten Runs](https://github.com/mcpdev80/baseharbor/actions/runs/39964376295/artifacts/12506087254) bestätigt exakt `(target,head,integration)=(2f005813...,1263b9e2...,1021b41e...)` und unresolved coverage. Damit ist auch die Behauptung „Planner frisch, nur Final stale“ widerlegt.

**Wichtige Grenze:** Die Live-PR-API meldet jetzt `(49d396c4...,1263b9e2...,cbd187b7...)`. `cbd187b7` ist **nicht** die im roten Run geprüfte Integration. Warum GitHub im neuen Run noch die alten Kontextwerte verwendet hat, ist aus Log/Run-API nicht feststellbar; weder Caching-Ursache noch genaue Webhook-Rohdaten werden erfunden. Close/reopen hat den konkret untersuchten Run nachweislich nicht auf neue Werte gebracht. DIAG2 empfahl einen neu erstellten PR mit korrekt gerichtetem Kandidaten, nicht bloß close/reopen desselben PRs.

### DIAG3 — b) Welche Änderung macht die Elternbedingung wahr?

`pr-diff-base.py:136–145` liest mit `git rev-list --parents -n 1` die **synthetische Integration**. `parents` enthält dabei zuerst deren eigene SHA, dann deren Eltern. Die Forderung ist deshalb `[integration,target,head]`; sie prüft hier nicht direkt die Eltern von `1263b9e2`.

Belegt durch Commit-API **und** lokale immutable `git rev-list`-Ausgabe:

```text
IST im roten Run:
rev-list(1021b41e) = [1021b41e, 49d396c4, 1263b9e2]
ERWARTET:
                     [1021b41e, 2f005813, 1263b9e2]
```

**Für genau diese Gleichheit ist die einzelne notwendige Wertänderung `target/base: 2f005813 → 49d396c4`.** Dafür ist keine Änderung an den Eltern von `1021b41e` und keine Lockerung von `pr-diff-base.py:142` erforderlich. Ein Wechsel nur von `1021b41e` auf `cbd187b7` bei alter Base hilft nicht: beide Integrationen haben dieselben Eltern `[49d396c4,1263b9e2]`.

Eine Base-Änderung ausschließlich im Final-Job wäre falsch: Checkout, Diff-Resolver, Planner, Check-Runner und Final-Assertions müssen dieselben authentifizierten Inputs verwenden. Der heutige Code bindet diese korrekt an den Event-Kontext; der untersuchte Kontext ist jedoch veraltet. Ein zusätzlicher develop-Commit oder das Umsortieren der Source-Eltern allein kann die Werte dieses historischen Run-Kontexts nicht ändern. Auch nach konsistenter Base-Erneuerung bleibt der separate Source-Graph-Block aus DIAG2 bestehen.

### DIAG3 — c) Reale Eltern-Grafiken im Vergleich

| Ebene | #1039, erfolgreich | #1042, letzter roter Run |
| --- | --- | --- |
| akzeptierter Source | `75efeef8...` | `0267dbdb...` |
| veröffentlichter Sync-HEAD | `7e53bc68...` | `1263b9e2...` |
| Sync-HEAD-Eltern in Reihenfolge | `[75efeef8,5be7dabd]` | `[49d396c4,0267dbdb]` |
| Integration | `1f8deace...` | `1021b41e...` |
| Integrationseltern in Reihenfolge | `[5be7dabd,7e53bc68]` | `[49d396c4,1263b9e2]` |
| Run-Base | `5be7dabd` | `2f005813` |
| Resolver-Elternbedingung | passt | erster Parent ≠ Run-Base |
| Source-first-Nachfahrenbeweis | erster Parent führt zum Source | erster Parent führt zu develop |

Volle #1039-Integrationsidentität: `1f8deace04bcbf8e0e5f590c7e0c1b9db24986dc`; Eltern durch Commit-API geprüft. #1039s zuvor authentifiziertes Plan-Artefakt `12502812098` belegt resolved coverage für Base `5be7dabd5c40a99e1623eb1a72cae943f9e0470d` und Head `7e53bc6804521b03c81673d181862a85c56ba3c9`. Es hat sowohl konsistente Integrationsinputs als auch die richtige Source-first-Kette. Bei #1042 fehlen beide Bedingungen; das sind verschiedene Prüfungen.

### Ein Rezept, ein Worker

**Ein vorhandener zugewiesener Source-Owner erstellt einen korrekt gerichteten Kandidaten auf neuem Branch und einen neu angelegten PR gegen gepinntes aktuelles develop.** Das ist derselbe vollständige Reparaturweg wie DIAG2; close/reopen des alten PRs ersetzt ihn nicht. Keine zusätzliche Worker-Lane und kein paralleler Workflow-Fix.

- Kandidat am Source `0267dbdb71f63a2c77e193618e6fba4106e5cd5b` beginnen, normalen sauberen develop-Merge erzeugen. Zu korrigierende Erzeugungsanweisung: `o51-sync1042b.md:85`, Elternreihenfolge von `-p develop -p source` auf `-p source -p develop`; keinen weiteren Merge über `1263b9e2` stapeln. Original-Branch/Commit erhalten, kein Force-Push. Vor Veröffentlichung Source-first-Eltern, Git-exakten Merge-Tree, Fünf-Dateien-Inventar und resolved lokalen Plan beweisen.
- **Der entscheidende neue Input ist die Base des neuen PR-Events**: bei unverändertem develop `49d396c48d2dee8b65cd59497c9f74692954975c`. Für neuen HEAD H und synthetische Integration I müssen `rev-list(I)=[I,49d396c4...,H]` und die tatsächlich geloggten `(BASE_SHA,HEAD_SHA,INTEGRATION_SHA)` sowie Final-Erwartungswerte exakt zusammenpassen. Event-Base allein aus der aktuellen PR-API abzulesen genügt ausdrücklich nicht. Diese Übereinstimmung ist vor weiterer Qualifikationsarbeit Pflicht.
- **Keine Produkt-Codezeile ändern.** `.github/workflows/ci.yml:444–457,790–816` und `scripts/pr-diff-base.py:142` behalten ihre exakte Bindung; Selector-Anker bleibt `0267dbdb...`, `core752_reviewed_merge_chain:531–567` bleibt Source-first. Ein einzelner beliebiger „Fix-Commit“ auf develop oder dem falschen Sync-HEAD wäre kein vollständiger belegter Fix und wird nicht empfohlen.
- Gezielte bestehende Resolver-/DISPATCH_GATE_REF-Merge-Chain-Regressionsprüfungen wie DIAG2 vor dem Push; anschließend alle vom resolved Plan tatsächlich geforderten Checks und finales App-15368-Gate auf exakten neuen Inputs, danach SHA-genaue unabhängige Prüfung. Keine Runtime-/Release-Vollmatrix aus diesem Diagnosefehler ableiten.

**Stop-Kriterium:** der neue Run übernimmt erneut alte oder nicht zusammenpassende Kontextwerte, Source-/Inventarbeweis scheitert, Konflikt oder manuelle Inhaltsauflösung nötig, Ownership unklar oder Pflicht-CI rot/fehlend. Dann nicht nochmals blind pushen/reopen/rerun; ein neuer belegter CI-Kontextfehler müsste separat scoped werden. Die Gleichheit der neuen Inputs wird geprüft, nicht ohne Run-Beleg als garantiert behauptet.

Abschlusskontrolle 13:58:14 UTC: neuester Check unverändert `120450991396`/App 15368; #1042 OPEN, HEAD `1263b9e2...`, Live-Base `49d396c4...`, Live-Integration `cbd187b7...`. Keine Produktdatei geändert, keine Test-/CI-Mutation, PR-/Branch-Erstellung, Worker-Zustellung oder Merge-Aktion; nur diese Evidence ergänzt. Produktstatus weiterhin allein bestehendes untracked `worktrees/`. Basics-Reminders/Pool-/fremde Idle-/Coordinator-Plan-Flags erneut gelesen und als außerhalb dieser REVIEW-Lane offen dokumentiert; keine falsche globale Quittierung. Native TodoWrite weiterhin nicht verfügbar; persistenter PLAN abgeschlossen.

END DIAG3-1042-DONE
