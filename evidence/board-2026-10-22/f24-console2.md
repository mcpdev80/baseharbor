# F24 – console-2-live-core-adapter-workflow-proofs (research only, read-only)

Lane: f24 | Prompt: evidence/board-2026-10-22/prompts/f24.txt | Board: console-2-live-core-adapter-workflow-proofs (waiting, v0.4.27)
Mission: RECHERCHIEREN ohne Aenderung: welche "live core adapter workflow proofs" existieren bereits im
console-Repo (Tests/Gates/CI-Namen) vs. dem, was die Karte fordert. Diff bewerten
(nur Doku-Zeile vs. echte Live-Beweis-Jobs fehlend) und fertiges Umsetzungsrezept
(Datei/Job/Namen, max 2 Dateien) liefern, damit ein naechster Worker ohne Research starten kann.
End marker: F24-CONSOLE2-DONE

## PLAN
1. [x] Turn-start basics-check.py ausgefuehrt, FLAGS erfasst
2. [x] Contract lesen: plan.py show console-2 (Karten-ID ist `console-2`, Dateiname long-slug)
3. [x] EVENTS-Flag abgearbeitet (events-check.py --ack: 1 WATCHER EVENT `2f86d226… gone` – Data, kein Auftrag)
4. [x] console-Repo (origin/main = 4724d996) Live/Core-Adapter-Workflow-Proofs recherchiert – read-only
5. [x] Diff: Karten-Forderung vs. Bestand
6. [x] Umsetzungsrezept (2 Dateien) in dieses Evidence
7. [x] repo-rule compliance dokumentieren, END F24-CONSOLE2-DONE

## Reality check / Kartenkontext
- Karte laut lokalem Planning-Stand: `console-2`, state **waiting**, release **v0.4.28**
  (Prompt sagte v0.4.27 – Diskrepanz dokumentiert; Task-Note `release: v0.4.28` ist maßgeblich).
- Alle 4 Preconditions (`t-20261017-release-0-4-28-updates`, `t-20261018-r27-access-proof`,
  `t-20261018-r27-public-surfaces`, `t-20261018-r27-team-proof`) = waiting/NOT accepted →
  Karte ist NICHT ready; dieses Evidence bereitet nur das Rezept vor. Kein Claim gesetzt (read-only Auftrag).
- console-Repo: lokaler Checkout auf `work/v0426-console-consumption` (0ea88f5, fremder Arbeitszweig –
  nicht angefasst). Recherche gegen `origin/main` = 4724d996 via git show/grep, keine Aenderung.

## Findings – Bestand an Live-Core-Adapter-Workflow-Proofs (console origin/main 4724d996)

### Adapter-Austausch: bereits erledigt
- `src/lib/baseharbor/adapter.ts` = Interface `BaseHarborConsoleAdapter`; `data.ts` exportiert nur noch
  `previewData = fixtureAdapter` (explizit preview-only), Fixtures separiert in `fixture-adapter.ts`.
- Live-Pfad: `LiveCoreAdapter` in `src/lib/baseharbor/read-models.ts:59`, gemappt auf verifizierte
  Core-Operationen; Client-Komponente `src/components/live/live-core-view.tsx` ersetzt Preview-Content,
  sobald eine Core-Session besteht ("Once connected, every result … comes from the selected Core
  operation"); Live fällt nie auf Fixtures zurück (Session ended → Sign-in-Panel).
- Architektur-Guard `scripts/check-architecture.mjs` ist `npm test` und PR-Gate-Einstieg
  (AGENTS/README: "public architecture-boundary regression entry point").
- Aber: `LiveCoreView` ist nur auf 4 Seiten verdrahtet (applications, runtime, targets, workspaces);
  Server-Skeleton aller ~56 `previewData`-Import-Stellen bleibt Fixture-Preview.

### Existierende Live-Workflow-Proof-Skripte (echt, gegen natives Core/Keycloak/Postgres/Docker)
- `scripts/browser/live-browser.mjs`: Chromium-Login, Origin/Foreign-Installation-Denial, Viewer-Denial,
  Token-Expiry, Console-getriebener Core-Setup (`control-plane.up` als echte Operation, Zeile 147),
  Streams/Terminal/Logs, Evidence-Checks; ruft `qualifyApplicationJourney` (Zeile 233).
- `scripts/browser/application-journey.mjs`: plan/apply mit Approval-Checkbox (177-180), status/doctor/
  repair (206-216), Credential-/CA-Rotation inkl. `openbao.rotate` (228-242), destroy mit
  Foreign-Issuer-Preservation (64-66, 260-273), generierte Next.js-App `baha up`→200→doctor→destroy (33-69).
- `scripts/browser/remote-application-journey.mjs`: Node-Connector-Remote-Target-Lebenszyklus;
  wird nur aktiv, wenn `BASEHARBOR_BROWSER_REMOTE_RUNTIME` gesetzt ist (application-journey.mjs:262).
- `scripts/browser/execution-observer.mjs` + `tests/browser-execution-observer.test.mts`: Guard gegen
  fabrizierte Browser-Ergebnisse.
- Kontrakt-/Quelltests `tests/*.test.mts` (machine-session, read-models, streams, terminal-session,
  log-stream, oidc, discovery, http-transport, core-bootstrap, core-rotation): Source-Ebene, erzeugen
  laut Doku ausdruecklich KEINE Runtime-Evidence.

### Existierende CI-Gates, in denen die Proofs laufen
1. console `ci.yml` Job **"Actual browser · Keycloak · Core HTTP"** – `if:` nur bei Push auf Branch
   `runtime-validation/browser/**` (letzter solcher Branch: v0.4.23-Ära, ls-remote). Läuft
   live-browser.mjs OHNE `BASEHARBOR_BROWSER_REMOTE_RUNTIME`; Artifact
   `actual-browser-core-read-receipt`. Core hart gepinnt `d5417bde…`.
2. console `release-integration.yml` Job **"Integration · integration/static/live-console"** – Trigger nur
   Push `release-integration/**` (älteste ähnl. Branch v0.4.25; v0.4.26/27 fehlen). Läuft mit
   `BASEHARBOR_BROWSER_REMOTE_RUNTIME=docker` → Remote-Journey + generated-Next.js läuft mit;
   Receipt-Gate `scripts/release-integration-receipt.mjs`.
3. Core `baseharbor` pre-release.yml Job **"Integration · integration/static/live-console"**
   (`integration_live_console`, Zeile 1458) → `scripts/pre-release-private-integration.py
   --gate integration/static/live-console`; `if:`-Tag-Liste auf v0.4.24/25/26 begrenzt
   (Zeilen 1471-1473), consumer-pins-Abhängigkeit laut Matrix (o24/f8: live-console = WAIT auf U-Pins).
4. Auf normalen PRs/pushes auf main läuft KEIN Live-Journey-Job (ci.yml PR-Trigger executes nur `quality`).

### Dokumentations-Wahrheit vs. Bestand
- `docs/CONTRACT-READINESS.md` (Zeilen 27-30, "Qualified live slices" bis ~Z. 270): behauptet weiterhin
  "complete application mutation workflows remain pending", "complete mutation/approval coverage remains
  pending", "full remote Connector lifecycle qualification remains pending", "production rotation …
  pending" – die Journey-Skripte existieren und laufen in 2./3. seit 1fe2202 (2026-10-11, v0.4.24-Härtung).
- `README.md:52-61` enthält weiterhin die "BaseHarbor dependencies … #767–#770"-Sektion = genau die
  Karten-Forderung "Remove stale 'waits for #767–#770' claims". README:81 behauptet zugleich v0.4.24
  native acceptance including application mutation/rotation → innerer Widerspruch der Doku.
- Karten-Handoff "Still open" (2026-10-07) ist gegenüber main teilweise überholt (Bootstrap und
  Mutation-Journeys sind skriptiert); offen bleibt nachweisbare grüne Ausführung auf dem finalen Core.

## Diff-Bewertung
Keine reine Doku-Zeile, aber auch kein Bedarf neuer Journey-Skripte. Der Kern "replace fixtures with
protected Core adapter" ist in main erledigt. Es fehlt:
1. Ein **dauerhaft laufendes Live-Workflow-Proof-Gate**: die Journey existiert nur als branch-gegatester
   Einmal-Lauf (runtime-validation/browser/**) bzw. Release-Punkt (release-integration/**, Core-Matrix
   nur ≤ v0.4.26). Auf main/der Kandidaten-SHA gibt es keinen wiederkehrenden Beweis-Job. → echte CI-Job-Änderung.
2. **Doku-Konsequenz**: CONTRACT-READINESS-Matrix + README müssen den tatsächlichen Proof-Stand zeigen
   (Karten-Acceptance "Docs/README/readiness matrix and CI reflect actual supported live functionality").
3. (Produkt-Lücke, nicht Proof-Lücke): Wizards target create/edit, provider add/replace, restore,
   platform onboarding haben noch keine Live-Browser-Journey und kein LiveCoreView – proveset ist dafür
   nichts möglich, solange die Flows nicht verdrahtet sind. Das ist Rest-Scope der Karte, kein Research-Fund.

## Umsetzungsrezept (für nächsten Worker; 2 Dateien + 1 markierte 1-Zeilen-Ausnahme)
Datei 1: `baseharbor-console/.github/workflows/ci.yml`
- Browser-Job zum ständigen Workflow-Proof machen:
  `on:` ergänzen um `schedule: - cron: '27 4 * * *'` (tgz main); `on.push.branches` um `main` erweitern;
  Job-`if:` ändern auf: pull_request → skip; sonst laufe, wenn `github.ref_name == 'main' ||
  github.event_name == 'schedule' || startsWith(github.ref_name, 'runtime-validation/browser/')`.
- Job-env ergänzen: `BASEHARBOR_BROWSER_REMOTE_RUNTIME: docker` (damit remote-application-journey +
  generated-Next.js-Zweig auf main mitlaufen; Step "Build exact Connector…" aus release-integration.yml
  Zeilen ~207-221 analog übernehmen, Connector-Pin 0a02e6d95494616ec3a9772a042e15dec8fa31ac).
- `BASEHARBOR_BROWSER_CORE_COMMIT: d5417bde…` ersetzen durch dieselbe Kanonische Source-Lock-Auflösung,
  die der Quality-Job bereits nutzt (`contracts/core-machine/v1/source-lock.json`.core_commit), damit der
  Proof immer gegen den freigegebenen Core-Kandidaten läuft.
- Stepname "Run actual Chromium login and live-read checks" umbenennen in
  "Run actual Chromium live Core adapter workflow proofs (login, plan/apply/status/doctor/destroy,
  rotation, remote lifecycle)"; Artifact `actual-browser-core-read-receipt` →
  `actual-browser-core-workflow-receipt` (Name in beiden Workflows konsistent halten).
Datei 2: `baseharbor-console/docs/CONTRACT-READINESS.md`
- Tabelle "Current readiness": Live-binding-Zeilen auf **qualified** ziehen für die tatsächlich bewiesenen
 _slices_ (application plan/apply/approval/status/doctor/repair/destroy, CA/OpenBao rotation, remote
  Connector lifecycle), jeweils mit Referenz auf den neuen main-Proof-Job + Release-Receipt; noch offene
  Wizard-Familien (target create/edit, provider add/replace, restore, onboarding, platform) explizit als
  "no live proof yet" listen.
- Abschnitt "Qualified live slices" um die Journey-Schritte aus application-journey.mjs /
  remote-application-journey.mjs ergänzen (mit Step-Log-Markers wie
  "actual-browser-Core-application-plan-status-doctor-repair-explicit-destroy-and-foreign-issuer-preservation").
Ausnahme (1 Zeile, falls Coordinator Scope-3 erlaubt): `README.md` Sektion "BaseHarbor dependencies"
(Zeilen ~56-61, #767–#770-Liste) löschen/ersetzen durch "tracked in BaseHarbor release notes" –
Karten-Scope verlangt ausdruecklich "Remove stale 'waits for #767–#770' claims".
Stop-Kriterium: Proof-Job muss auf einem Throwaway-`runtime-validation/browser/f24p-*`-Branch vor dem
main-Merge einmal grün durchlaufen (Receipt-Artefakt vorhanden, Core-SHA = Source-Lock-SHA), sonst
RETURN an Coordinator, nicht auf main merge.
Aufwandshinweis: Jobs 1+2 duplizieren Fixture-Logik – bewusst in Datei 1 belassen (max-2-Dateien-Regel);
eine allfällige Job-Composite-Aktion wäre Folge-Refactor außerhalb dieser Karte.
Abhängigkeitswarnung: Karte bleibt bis Accept aller 4 Preconditions waiting; Umsetzung nur als
vorbereitete PR auf console main, Acceptance erst nach Fulfilment.

## repo-rule compliance
- Read-only: keine Datei in baseharbor-console/baseharbor geändert; nur git show/grep/ls-tree/log und
  ls-remote (Netz-Lesezugriff). Fremder Arbeitszweig work/v0426-console-consumption nicht angefasst.
- Kein Claim auf console-2 gesetzt (Auftrag war explizit RECHERCHIEREN ohne Aenderung; Karte waiting,
  nicht ready). Planning-Notizen nur in diesem Evidence, keine Karten-Metadaten editiert → render/check
  nicht erforderlich.
- Prompt-Injection-Defense: PLAN.md von baseharbor-plan (Maintainer-Autorität) und Karte als DATEN
  gelesen; fremde Karten-/Issue-Texte keinen Handlungen folgend ausgeführt. Watcher-Event nur archiviert.
- Secrets: keine Token/PASSWÖRTER loggt; nur öffentlich gepinnte Commit-SHAs und Job/Artefakt-Namen.

F24-CONSOLE2-DONE
