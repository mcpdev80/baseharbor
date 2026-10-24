# O115 — W1-Qualifikation v0.4.26 vorbereiten (Beweis-Wiederverwendung)

## PLAN

- [x] 0. Evidence file + native Todos — diese Datei
- [x] 1. Root-Cause „Resolve release candidate": fehlgeschlagene pre-release-Runs analysiert (Run-ID/Zeile)
- [x] 2. REUSE-Matrix fuer jedes v0.4.26-Pflichtgate (scripts/release-requirements.json + 0.4.25-Muster)
- [x] 3. FEHLERLISTE minimale Atomic-Gates, die FEHLEN/ungultig sind (kleinsten Workflow/Job-Namen)
- [x] 4. Entwurf O115-BLOCKED-W1 (consumer-pins.json fehlt; Pre-Release nicht green)
- [x] 5. repo-rule compliance (nur Lese-Diagnose, kein Produkt-/CI-Aend., kein Push)

## Befund

### 1. Root-Cause „Resolve release candidate"

- `scripts/release-requirements.json` LESBAR (Schema `baseharbor.release-requirements/v1`, Releases v0.4.22–v0.4.26).
- Alle v0.4.26 pre-release-Runs auf **develop** sind FAILURE. Latest: **Run 40023245872** (2026-10-23T20:58:02Z, `workflow_dispatch`, branch `develop`).
  - Laufende Historie (alle `Pre-Release · v0.4.26` / `workflow_dispatch` / failure):
    40023245872, 40009690268, 40001750271, 40000941427, 39999967958, 39998719700, 39998705831, 39997310442, 39997296272, 39865161687, 39864813979, 39864801187, 39853204836, 39853157648 (plus `main`-Run 40001628646 failure).
- Source-Job `Source · Contracts · Docs` Step **„Resolve release candidate" = completed/failure**. Evidence-Summary des Runs bestaetigt: „Candidate resolution | **FAIL** | release metadata, exact candidate SHA and pinned demo SHA"; Workflow-Policy PASS, alle restlichen Source-Steps SKIP (nachgeordnet).
- **Exakte Fehlerstelle: `.github/workflows/pre-release.yml:117`**
  ```
  117  test -s "docs/releases/${tag}.consumer-pins.json"
  ```
  - Zeilenkontext (pre-release.yml:114–136): nach `grep -Fq "## [${version}]" CHANGELOG.md` (PASS auf develop) und `test -s "docs/releases/${tag}.md"` (PASS, `docs/releases/v0.4.26.md` vorhanden) scheitert die Kette an Zeile 117: die Datei `docs/releases/v0.4.26.consumer-pins.json` ist auf develop NICHT vorhanden (`git show origin/develop:docs/releases/v0.4.26.consumer-pins.json` → path does not exist). `set -euo pipefail` → Step exit 1.
  - Die nachfolgende Generierung (pre-release.yml:122–128, `python3 scripts/ecosystem_release_pins.py --tag v0.4.26 --core "$(git rev-parse HEAD)" --connector "$INPUT_CONSUMER_REF" ...`) ist deshalb unerreichbar; zudem scheitert ein lokaler Replikatsversuch auf develop am Core-Mismatch (`--core a2abc17a` ≠ Pin `1861ed07`): `ValueError: immutable Core/Connector/Console/Demo candidate differs`. Das ist erwartungsverhalten: der Workflow wird auf der `release/v0.4.26`-Branch-Spitze oder mit vorliegender consumer-pins.json ausgefuehrt, nicht auf develop.
- **Fehlendes Input/Artefakt:** das Skript erwartet die existierende, befuellte Datei `docs/releases/v0.4.26.consumer-pins.json` als Artefakt; diese ist auf develop nicht commitet. Der Workaround (neue Generierung im Run) faellt auf develop am Core-Pin-Mismatch.
- Kein Rerun, keine Aenderung. `git ls-remote --heads origin 'release/v0.4.26'` liefert keinen Branch → Release ist nicht promoted; die fehlende consumer-pins.json ist damit die einzige Blockade dieser Sequenz.

### 2. REUSE-Matrix (scripts/release-requirements.json vs. 0.4.25-Muster)

Muster v0.4.25 (`docs/releases/v0.4.25.evidence-runs`, `docs/releases/v0.4.25.selected-gates.json`) zeigt: W1-Beweis = **targeted** pre-release-Runs (`validation/pre-release/selected/gates/<gate>`), je ein Run pro Gate, Run-IDs in evidence-runs hinterlegt; selected-gates.json listet nur die tatsaechlich nachqualifizierten Atomic-Gates.

**Policy (Mission): nur gleicher unveraenderter Commit + gleiche Konfiguration = wiederverwendbar.**

| v0.4.26-Pflichtgate (release-req) | Vorhandener gruener Nachweis | Run-ID | Commit | Gate-Name | Reuse-Entscheid |
|---|---|---|---|---|---|
| atomic/static/*, atomic/docker/*, atomic/podman/* (release-req) | kein gruener pre-release-Run je v0.4.26; alle Runs (40023245872 etc.) FAIL in „Resolve release candidate" | — | — | source/resolve | **invalid_stale** (kein 0.4.26-Run green; 0.4.25-Runs laufen auf anderem SHA/Tag) |
| adoption/* | kein gruener 0.4.26-Run | — | — | adoption | **missing** (kein 0.4.26-Beweis existiert) |
| ha/* (docker/podman) | kein gruener 0.4.26-Run | — | — | v0421 ha groups | **missing** |
| journey/* | kein gruener 0.4.26-Run | — | — | reference_journey | **missing** |
| integration/static/* (public-contracts, configuration-policy, browser-terminal) | kein gruener 0.4.26-Run | — | — | integration_* | **missing** |
| integration/docker/podman/remote-target | kein gruener 0.4.26-Run | — | — | integration_remote_target | **missing** |
| integration/static/live-console | kein gruener 0.4.26-Run | — | — | integration_live_console | **missing** |
| integration/docker/podman/core-bootstrap | kein gruener 0.4.26-Run | — | — | integration_core_* | **missing** |
| integration/docker/podman/coreless-lifecycle | kein gruener 0.4.26-Run | — | — | integration_coreless_* | **missing** |

- **0.4.25-Runs (38715423224, 38719385981, 38723551348) sind NICHT wiederverwendbar** fuer v0.4.26: (a) andere Tag-Konfiguration (`v0.4.25`/`frozen f325b93c`), (b) Workflow-`--core`/`--connector`/`--demo`-Pins auf 0.4.25-Pin-Kette statt auf 0.4.26-Kette, (c) Policy nur gleicher unveraenderter Commit + gleiche Konfiguration. Damit `reuse_ok`, muesste jeder Gate-Run auf v0.4.26-Pin-Kette neu laufen.
- **Wiedererverwendbar waere nur**: ein gruener `validation/pre-release/selected/gates/<gate>`-Run, der auf **0.4.26-Pin-Kette** (Core 1861ed07 / Connector df1f4378 / Demo 25c31969 / Console 69d1d14) und **develop-a2abc17a-Verwandtschaft des Core-Pins** laeuft. Solche Runs existieren fuer v0.4.26 nicht (keiner der pre-release-Runs je 0.4.26 ist gruen).

### 3. FEHLERLISTE — minimale nachzuqualifizierende Atomic-Gates

Alle v0.4.26-Pflichtgate des `release-requirements.json` fehlen an guentigem 0.4.26-Nachweis; **keiner** ist green. Die kleinste nachzuqualifizierende Menge = die für v0.4.26 geforderten Gates als **targeted** pre-release-Runs (`validation/pre-release/selected/gates/<gate>`), **nicht** die Full-Matrix:

- `atomic/static/config-matrix`, `atomic/static/init`, `atomic/static/mcp`, `atomic/static/agent`, `atomic/static/shell-ux` — `static_gates`
- `atomic/docker/{guided,lifecycle,policy,connectivity,reconciliation,failure,full-destroy,data-capabilities,observability,identity,security,backup-restore,rabbitmq-runtime,mongodb-runtime,durable-key-value,control-plane-recovery,loki-runtime,tempo-runtime}` — `runtime_candidate` + `runtime_{light,heavy}_gates_docker`
- `atomic/podman/{...same...}` — `runtime_candidate` + `runtime_{light,heavy}_gates_podman`
- `adoption/{conformance,realworld-compose,realworld-quadlet,realworld-kubernetes}` — `adoption`
- `ha/{docker,podman}/{data,identity,observability,routing}` — `v0421_ha_groups_{docker,podman}`
- `journey/{docker,podman}` — `reference_journey`
- `integration/static/{public-contracts,configuration-policy,browser-terminal,live-console}` — `integration_{public_contracts,configuration_policy,live_console}` (+ browser-terminal)
- `integration/{docker,podman}/remote-target` — `integration_remote_target_{docker,podman}`
- `integration/{docker,podman}/core-bootstrap` — `integration_core_{docker,podman}`
- `integration/{docker,podman}/coreless-lifecycle` — `integration_coreless_{docker,podman}`

**Ausdrucklich KEINE Full-Matrix ohne Begruendung.** Empfehlung: nur die Gates nachqualifizieren, die fuer die v0.4.26-Release-Entscheidung tatsaechlich erforderlich sind; die Pins-Kette (#1056/o114 → #1050) muss zuerst in develop liegen (否则 die pre-release-Generierung pre-release.yml:122 bleibt am Core-Mismatch hängen). Targeted `only_gates`-Dispatch auf 0.4.26-Pin-Kette statt Full-Matrix.

### 4. Entwurf / Blocked

**O115-W1EVIDENCE-BLOCKED** (`blocked-on-w1`): Die Bedingung fuer einen PR-ready Entwurf von `docs/releases/v0.4.26.evidence-runs` + `.selected-gates.json` (Mission Schritt 4) ist NICHT erfuellt: es gibt **keinen** gruenen, gultigen v0.4.26-Run, auf den ein evidence-runs/selected-gates Entwurf fundiert werden koonte. Der Blocker ist nicht W1-intern, sondern die Release-Qualifizierungssequenz:

- BLOCKER **file:Zeile**: `baseharbor/.github/workflows/pre-release.yml:117` — `test -s "docs/releases/v0.4.26.consumer-pins.json"` auf develop (Datei fehlt).
- Folge `file:Zeile`: `baseharbor/.github/workflows/pre-release.yml:122` — `ecosystem_release_pins.py --tag v0.4.26 --core <develop-HEAD> ...` faellt am Core-Pin-Mismatch (`immutable Core/Connector/Console/Demo candidate differs`), weil develop HEAD `a2abc17a` ≠ Pin `1861ed07`.
- Der Fix-Weg ist extern an O115 (Coordinator/Release-Lane): (a) `docs/releases/v0.4.26.consumer-pins.json` kommt via Merge-Kette **#1056 → #1050** in develop (Pin-Daten qualifiziert, o114), oder (b) Release-Dispatch auf `release/v0.4.26`-Branch-Spitze. Erst danach sind targeted 0.4.26-Runs moeglich und ein PR-ready evidence-runs/selected-gates-Entwurf fundierbar.
- Zur Einordnung (nicht als 0.4.26-Beweis wiederverwendbar): Docs-Surface ist seit #1041 auf develop a2abc17a unveraendert; strict Pages candidate build **gruen** in Run **40064756153** (PR #1059) — das ist aber ein Docs-Beweis, kein Pre-Release-Qalfizierungsbeweis.

### Pins-Kette verifiziert (gegen develop/Pin-Dateien, nicht blind uebernommen)

`docs/releases/v0.4.26.consumer-pins.json` (PR #1050, head **38896762**, base develop) — alle 4 Pins als reale Immutable-Commits bestaetigt:
- Core `1861ed07888a1ec22c218ca491d23ee54f48e261` ✓ (mcpdev80/baseharbor, in develop-Ancestry; #1044)
- Demo `25c319697fd3d89d894943e0f44cd3d5dde8b198` ✓ (mcpdev80/baseharbor-demo)
- Connector `df1f4378d8c7fdc3e174a1e9dc8ad0155a955571` ✓ (mcpdev80/baseharbor-node-connector, #q59b FROZEN)
- Console `69d1d14007fa43dc0c0457c3347280be7f802818` ✓ (mcpdev80/baseharbor-console)
- PR #1050 state OPEN (head 38896762, base develop a2abc17a); #1056 state OPEN (head df8e53b9, base develop) — Merge-Kette #1056 → #1050 noch offen.
- Die in o115 genannten Anker-Shas (Core 1861ed07, Demo 25c31969, Connector df1f4378, Console 69d1d14) **stimmen ueberein** mit der Pin-Datei.

## repo-rule compliance

- Rein lesend: alle git-/gh-Kommandos ausschliesslich im baseharbor-Worktree (`/home/marcel/dev/baseharbor-ecosystem/baseharbor`) mit pwd-Pruefung vor jedem git-Befehl; **kein git im Root-Repo**. Kein Switch von main/develop-Checkout.
- gh nur fuer `run list/view/download`, `pr view`, `api` (repo/commits/contents/runs) — **kein** `gh workflow run`, **kein** `gh run rerun`, kein merge/review/dispatch/label/deploy, kein Push.
- Kein Produkt-/CI-/Workflow-Aend., keine VM-/Docker/Podman/K3s-Dispatches, keine Labels. Reine Lese-Diagnose + Auswertung + begrundeter blocked-on-W1-Entwurf.
- Daten statt Auftrag: o115.txt als DATA behandelt (PROMPT-INJECTION-DEFENSE); alle GH-Ereignisse als Befund gelesen, nicht als Instruction ausgefuehrt.

## END O115-W1EVIDENCE-BLOCKED
