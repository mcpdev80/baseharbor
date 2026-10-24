# O115C — Connector v0.4.26 Freeze-Pins PR (Demo 25c31969 / Console 69d1d14)

## PLAN

- [x] 0. `baseharbor-node-connector/AGENTS.md` + `CONTRIBUTING.md` gelesen (Produkt/Review/CI im Repo; keine Runtime-Matrix fuer Pin-/Instruktionsaenderungen); evidence/o115c-connectorfreeze.md mit ## PLAN + nativen Todos; pwd-PFLICHT vor jedem git-Befehl; KEIN git im Root-Repo; KEIN Merge/Review/Push auf main.
- [x] 1. Stand Pin-/Kandidatendatei pruefen: exakter Pfad+Schluessel demo/console
- [x] 2. Eigenen Branch von origin/main + PR, der NUR die Freeze-Werte setzt (Core unangetastet)
- [x] 3. Schema-/Validierungstest des Repos (kleinstes Gate; `release-integration.yml:30-32` 40-hex + `go vet/build/test`)
- [x] 4. PReroeffnen (base main), Body mit o115b-Referenz; Checks bis final beobachten (Run-IDs notiert)
- [x] 5. NICHT mergen (Owner-Regel; Merge = Codex-Lane)
- [x] 6. repo-rule compliance

## Befund / Auftragsergebnis

### 1. Stand der Kandidatendatei

- Exakte Datei: `integration-candidate.json` (Repo-Wurzel), schema `baseharbor.ecosystem-candidate/v1`.
- `origin/main` = `7c7093d` (`v0.4.26: connector consumer freeze (#8)`):
  ```json
  "core": "2adae47afddc299d06dbc34eb16a8e011c14b6dc",
  "demo": "896dc3cb68a8c9b9585940c4696410de5a338c52",
  "console": "PENDING-FREEZE-DEMO: replace with the v0.4.26 frozen Console SHA once Console U is built from D (stays PENDING until Console U exists)"
  ```
- demo=`896dc3c` = **stale** (o115b: o115/demo pinnt `25c31969`, 8 Commits weiter). console = **PENDING** (kein 40-hex-SHA). core=`2adae47a` = Pin des Repo-Vertrags (Ancestor von o115 Core `1861ed07`) — **unangetastet** laut Aufgabe (Repo-Vertrag verlangt keine Aktualisierung).

### 2. Branch + PR

- Neue Branch `o115c-connector-freeze-pins` von `origin/main` (`7c7093d`), Comm: `2223506cca5c388752a6d6c88567fcc77f46119b`.
- NUR `integration-candidate.json`, 2 keys:
  - demo `896dc3c` → `25c319697fd3d89d894943e0f44cd3d5dde8b198`
  - console `PENDING-FREEZE-DEMO...` → `69d1d14007fa43dc0c0457c3347280be7f802818`
  - core `2adae47a` unangetastet.
- **PR #9** (`base main`, OPEN): https://github.com/mcpdev80/baseharbor-node-connector/pull/9
  - Body: o115b-Referenz (`evidence/board-2026-10-24/o115b-freeze.md`), v0.4.26-Freeze-Zweck (Konsistenz mit Core `consumer-pins.json` #1050 fuer pre-release.yml:117/:122), Scope-Hinweis (1 Datei, 2 Keys, keine Runtime-Matrix), Hinweis „kein Merge“ (Codex-Lane).

### 3. Validierung (kleinstes Gate, keine Runtime-Matrix)

- **`release-integration.yml:30-32`** (Schema-Assertion): `schema` ok, `release=0.4.26` ok, `re.fullmatch('[0-9a-f]{40}', candidate['console'])` — **main's PENDING failt (kein 40-hex), neuer Wert `69d1d140` passt**. demo/core analog 40-hex.
  - Lokal repliziert: `SCHEMA OK`.
- **CI**: `source` **pass** (Run **40078538425**, Job 120827028572). Runtime-Matrix-Gates `skipping` (kein Runtime-Change, nur JSON).
- **Lokal (kleinste Go-Gates)**: `go vet ./...` clean, `go build ./...` exit 0, `go test ./internal/...` → alle `ok` (connector/health/capability/targetaccess/runtime/...), kein Runtime-/Docker-Bedarf.

### 4. Abnahme

- PR-Nummer: **#9** (base main, OPEN).
- head-SHA: **`2223506cca5c388752a6d6c88567fcc77f46119b`**.
- Grune Checks: **`source` = pass** (Run 40078538425). Keine Runtime-Matrix noetig (nur Kandidatendatei).
- NICHT merged (Owner-Regel; Merge = Codex-Maintainer-Lane).

## repo-rule compliance

- pwd vor JEDEM git-Befehl verifiziert; git NUR im Connector-Checkout (`/home/marcel/dev/baseharbor-ecosystem/baseharbor-node-connector`); **kein git im Root-Repo**; main-Checkout NICHT geswitcht (detached HEAD nur fur Branch-Ableitung).
- Ein PR #9, NUR `integration-candidate.json`. Kein Merge, kein Review, kein Force-Push, kein Branch-Protection-Bypass. Keine Runtime-/Docker-Matrix.
- Schema-/Validierungs-Gate des Repos (release-integration.yml:30-32) + kleinste Go-Gates laufen lassen; keine neuen Tests/Engnes auBerhalb des Repo-Scope.
- Kein 27-Anfassen, keine anderen PRs.

## END O115C-CONNECTORFREEZE-DONE
