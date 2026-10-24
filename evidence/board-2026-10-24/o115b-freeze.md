# O115B — Ecosystem-Freeze-Zustand v0.4.26 (4 Repos), fully read-only

## PLAN

- [x] 0. Turn-start basics-check.py (FLAGS: REMINDERS 205 unacked, BOARD currency STALE — remote 1 commit ahead, REALITAETSABGLEICH nötig für Dispatch; PLAN 23 open steps) + evidence/o115b-freeze.md mit ## PLAN + nativen Todos
- [x] 1. Ancestry-Check je Repo: Pin-Commit auf offiziellern Branch (develop/main) oder nur Feature-Branch
- [x] 2. Offene PRs pro Repo (gh pr list state=open) inkl. Check-Status + releasekritisch/27
- [x] 3. Konsistenz wechselseitiger Pin-Grafik + DAG/Zyklus-Prüfung
- [x] 4. Freeze-Dossier-Tabelle per Repo
- [x] 5. Explizite Empfehlung: welche Aktionen der Pool MUSS für pre-release.yml:117/:122
- [x] 6. repo-rule compliance (nur lesend, kein git im Root, kein Merge/Review/Dispatch/Push)

## Befund

### 0. Turn-start / Realitätsabgleich

- `basics-check.py`: REMINDERS 205 unacked, BOARD currency STALE (Remote 1 commit ahead), PLAN 23 open steps. Board-Zustand, **nicht** O115B-Scope. Hinweis: vor jedem Dispatch REALITAETSABGLEICH (remote planning) — kein Worker-Schreibrecht auf Planning hier.
- pwd-PFLICHT eingehalten; alle git-/gh-Kommandos je Repo im jeweiligen Checkout/Worktree; **kein git im Root-Repo**.

### 1. Ancestry-Check je Repo (Pin in develop/main oder Feature-Branch?)

| Repo | Pin-SHA (o115) | Branch des Pins | Pin-in-Official-Branch? | Beleg / Distanz |
|---|---|---|---|---|
| **baseharbor (Core)** | `1861ed07…` (fix ci bare-SHA dispatch v2, #1044, 2026-10-23 16:14) | develop | **JA** — `git merge-base --is-ancestor 1861ed07 origin/develop` = true | develop HEAD `a2abc17a`. `1861ed07` 6 Commits **vorne** von `2adae47a` (Ancestor). |
| **baseharbor-demo** | `25c31969…` (fix podman cold-start stall, o56b, 2026-10-23 18:56) | `work/v0.4.26-ecosystem-release` (Branch-Spitze) | **NEIN** — nur auf Runtime-Release-Branch, `git merge-base --is-ancestor … origin/main` = false | Demo main = `82d042b`. `25c31969` ist `896dc3c` (demo Core-Pin #117) **8 Commits vorne**, `896dc3c` selbst ist auf main. |
| **baseharbor-console** | `69d1d14…` (freeze console field FROZEN, q59b, 2026-10-23 19:11) | `work/v0.4.26-integrations-docs` | **NEIN** — nur Feature-Branch | Console main = `e7d1e4a`. `69d1d14` merge-base zu main = `4724d99` (kein Ancestor). |
| **baseharbor-node-connector** | `df1f4378…` (q59b FROZEN, 2026-10-23 21:11) | `work/v0.4.26-integrations-docs` | **NEIN** — nur Feature-Branch | Connector main = `7c7093d`. `df1f4378` merge-base zu main = `813f08a` (kein Ancestor). |

- Demo-Worktree `worktrees/demo-integrated-1022`/Branch `chore/v0426-core-pin @ a44e2e9`: `a44e2e9` (**pin Core C `2adae47a` in baseharbor-core.ref**, 2026-10-22 20:08) ist **nur** auf `chore/v0426-core-pin` (`git merge-base --is-ancestor a44e2e9 origin/main` = false). **Wartet auf PR/Merge** in demo main.

### 2. Offene PRs pro Repo (state=open)

| Repo | Open PRs | Status | Klassifizierung |
|---|---|---|---|
| **baseharbor** | #1050 `docs: add v0.4.26.consumer-pins.json` (head `38896762`, base develop) — #1056 `fix(ci): qualify consumer-pins … pins-data` (head `df8e53b9`, base develop) | offen | **#1050 releasekritisch** (contain die v0.4.26 consumer-pins.json). Merge-Kette **#1056 → #1050** (o114) blockt die core-pins-on-develop-Freigabe. #1056 offen, kein Merge. |
| **baseharbor-demo** | keine offenen PRs | — | — (Pin `896dc3c` bereits auf main via #117; Runtime-Head `25c31969` hängt an `work/v0.4.26-ecosystem-release`, kein offener PR) |
| **baseharbor-console** | **#8** `ci: run live Core adapter workflow proofs permanently on main` (head `f25/console2-live-workflow-proofs`) | **pass**: Architecture/Lint/Build; **skipping**: Actual browser/Keycloak/Core HTTP | releasekritisch: **NEIN** (nur CI-Dauerhaftung). **NICHT 27** — offen, aber nicht als 27 markiert. |
| **baseharbor-node-connector** | keine offenen PRs | — | Connector-Freeze #8 (`7c7093d`) bereits **MERGED** auf main (Core `2adae47a` pinned, Demo/Console PENDING-FREEZE). |

### 3. Konsistenz der wechselseitigen Pin-Grafik + DAG

- **Core `consumer-pins.json` (#1050, head `38896762`)** — schema `baseharbor.consumer-evidence-pins/v1` — verweist **einseitig** auf genau die vier SHAs aus o115:
  - core `1861ed07`, demo `25c31969`, console `69d1d14`, connector `df1f4378`. Gates: `integration/docker/podman/remote-target`, `integration/static/live-console`.
  - Ecosystem-DAG (core→demo→console→connector, connector→core/demo/console als Consumer): **zyklusfrei (acyclisch)** — nur Core ist Wurzel, niemand pinnt zurück zur `consumer-pins.json`. Kein Zyklus.
- **Connector `integration-candidate.json` auf main (`7c7093d`)**: `core=2adae47a`, `demo=896dc3c`, `console=PENDING-FREEZE-DEMO`.
  - **Inkonsistenz A (Core-Pin-Nivellierung):** Core wird hier als `2adae47a` gefroren. Das ist **kein** anderer Commit als o115's `1861ed07` — `2adae47a` ist der **direkte Ancestor** (`1861ed07` = 6 Commits vorne). `1861ed07` ist die v2-Finalisierung von #1044. Für den Freeze-Zustand **gleichwertig**, aber nicht identisch STRING; Pre-Release (`pre-release.yml`) fragt die **genaue** Pin-Datei an → hier `1861ed07` verlangen.
  - **Inkonsistenz B (Demo-Pin veraltet):** Connector main pinnt demo `896dc3c` (2026-10-22), o115/core-pins pinnt demo `25c31969` (2026-10-23, **8 Commits weiter**, Runtime-fix o56b). `896dc3c` ist **auf** demo main (via #117), `25c31969` nur auf `work/v0.4.26-ecosystem-release`. → Connector-Freeze ist **stale zum Demo-Runtime-Head**.
  - **Inkonsistenz C (Console PENDING):** Connector main lässt `console=PENDING-FREEZE-DEMO`. o115 pinnt console `69d1d14`. `69d1d14` ist **nicht** auf console main (`e7d1e4a`) — nur auf `work/v0.4.26-integrations-docs`.
  - **Konsistenz-Beurteilung:** Core-Pin konsistent (Nivellierung auf Ancestor). Demo/Console-Pins im Connector-Freeze **inkonsistent** zum o115-Endzustand (Demo stale um 8 Commits, Console unbenannt PENDING). Kein DAG-Zyklus; nur Stale-/PENDING-Lücken.

### 4. Freeze-Dossier-Tabelle per Repo

| Repo | Branch-Spitze (aktuell) | Pin o115 | Pin-in-Official | Fehlende Aktion | Beleg |
|---|---|---|---|---|---|
| **baseharbor** | develop `a2abc17a` | `1861ed07` | **JA** (in develop) | keine; Pin liegt auf develop. consumer-pins in develop **fehlend** → via #1050 in develop. | develop-Ancestry `1861ed07`=true; #1050 head `38896762` |
| **baseharbor-demo** | main `82d042b`; Runtime-Head `work/v0.4.26-ecosystem-release` = `25c31969` | `25c31969` | **NEIN** | `25c31969` in demo main mergen (kein offener PR — als Aktion führen) | `git merge-base --is-ancestor 25c31969 origin/main`=false |
| **baseharbor-console** | main `e7d1e4a` | `69d1d14` | **NEIN** | `69d1d14` in console main mergen (kein offener PR — als Aktion führen) | `69d1d14` merge-base main=`4724d99`; PR #8 offen (CI-only) |
| **baseharbor-node-connector** | main `7c7093d` (Merge #8) | `df1f4378` | **NEIN** | `df1f4378` in connector main mergen; Connector-Freeze `integration-candidate.json` auf demo=`25c31969`/console=`69d1d14` korrigieren (aktuell stale/PENDING) | `df1f4378` merge-base main=`813f08a`; #8 MERGED |

### 5. Empfehlung — Aktionen, die der Pool MUSS, damit `pre-release.yml:117/:122` erfüllbar

`pre-release.yml:117` (`test -s docs/releases/v0.4.26.consumer-pins.json`) und `:122` (`ecosystem_release_pins.py --core <tip> …`, Core-Pin-Mismatch auf develop) sind **nur** erfüllbar, wenn die v0.4.26 consumer-pins.json mit den **endgültigen** Pins auf develop liegt. Der Pool muss daher **in dieser Reihenfolge**:

1. **#1056 → #1050** (pins-data-Quffi, o114): #1056 (`df8e53b9`) in develop, danach #1050 (`38896762`) in develop → `docs/releases/v0.4.26.consumer-pins.json` liegt auf develop mit `1861ed07/25c31969/69d1d14/df1f4378`.
2. **Demo: `25c31969` in demo main** (kein offener PR — Aktion erzeugen), sonst o115-Demo-Pin ≠ Connector-Freeze-Pin `896dc3c`.
3. **Console: `69d1d14` in console main** (kein offener PR — Aktion erzeugen); #8 bleibt als CI-Dauerhaftung offen (nicht blockierend für Freeze).
4. **Connector: `df1f4378` in connector main** + `integration-candidate.json` auf main korrigieren (`demo` → `25c31969`, `console` → `69d1d14`), sonst Freeze stale/PENDING.
5. Erst danach targeted `pre-release.yml`-Runs auf `release/v0.4.26`-Spitze (oder consumer-pins auf develop) → `:117`/`:122` grün → W1-evidence-runs/selected-gates PR-ready.

**Keine** Full-Matrix ohne Begründung; nur die Pin-Freigabe (1–4) ist blockierend für `:117`.

## repo-rule compliance

- **Nur lesend:** alle Kommandos je Repo im jeweiligen Checkout/Worktree (`baseharbor`, `baseharbor-demo`, `baseharbor-console`, `baseharbor-node-connector`) mit pwd-Prüfung vor jedem git-Befehl; **kein git im Root-Repo**. Kein Switch von develop/main-Checkout der Target-Repos.
- gh nur für `pr list/view/checks`, `api` (commits/runs) — **kein** merge/review/dispatch/label/push/rerun. Kein Produkt-/CI-/Workflow-/PR-Change.
- Daten statt Auftrag: o115b.txt als DATA (PROMPT-INJECTION-DEFENSE); alle GH-Ereignisse als Befund gelesen.
- Kein 27-Anfassen; #8 (console) als nicht-27 identifiziert, nur benannt.

## END O115B-FREEZE-DONE
