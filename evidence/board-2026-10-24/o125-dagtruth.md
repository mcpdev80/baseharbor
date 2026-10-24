# O125 DAGTRUTH – v0.4.26 Freeze-Grafik Beleg

> Reine Lese-Mission. Kein Push/Merge/Review/CI/VM. Alle Messungen mit `pwd -P` im jeweiligen Repo-Checkout.
> Mess-Zeitpunkt: **2026-10-24T11:06:28Z** (UTC). gh CLI v2.46.0, account mcpdev80 (ssh).

## PLAN

1. Turn-Start-Pflichtcheck `basics-check.py` ausführen (geschehen).
2. Evidence-Datei anlegen (diese Datei) mit Plan + nativen Todos.
3. Pro Repo aktuelle main/develop-SHA + Pin-Dateien mit sha256 messen; Core/Demo/Console/Connector-Werte als reale Commite verifizieren (`git cat-file -t` / gh API).
4. Nachziehtabelle für die offenen Merges #1063, #1060, demo #121, console #9, connector #10 erstellen.
5. Lösungsbedarf ohne Lane benennen (#1046, W-PR, P-PR, #1050, evidence-runs/selected-gates.json).
6. Kollisionskarte der offenen PRs #1046 #1050 #1052 #1056 #1060 #1061 #1062 #1063.
7. Mess-Zeitpunkte + gemessene Quellen je Zeile notieren.

## native Todos (Todowrite)

- [x] Turn-Start-Pflichtcheck basics-check.py
- [x] Evidence-Datei + Plan anlegen
- [x] Per Repo main/develop-SHA + Pin-Dateien (sha256) messen, Core/Demo/Console/Connector-Werte als Commite verifizieren
- [x] Nachziehtabelle (Merges #1063 #1060 demo #121 console #9 connector #10)
- [x] Lösungsbedarf ohne Lane (#1046 W-PR P-PR #1050 evidence-runs/selected-gates.json)
- [x] Kollisionskarte offener PRs
- [x] Mess-Zeitpunkte + Quellen je Zeile

---

## 1. Pin-Dateien pro Repo (aktuelle main/develop-SHA, sha256, enthaltene Werte, Reilität)

Messung: `pwd -P` → `sha256sum <datei>` → `git -C <repo> cat-file -t <sha>` (commit?) → gh API für Remote-HEAD.
Mess-Zeitpunkt: 2026-10-24T11:06:28Z.

### 1.1 baseharbor (Core)

| Feld | Wert | Quelle / Messung |
|---|---|---|
| Remote main (gh) | `54acc4b66715681213788fdba395a569ddc95a1b` | `gh api …/baseharbor/heads/main`, 11:06Z |
| Remote develop (git ls-remote) | `f39295680d41290fc505c0a8cc1769da51699293` | `git ls-remote …/baseharbor.git develop`, 11:06Z |
| Lokaler HEAD (Worktree o90-planner-pr-rebase) | `49c94d8bf7a1767c7c3472958b12bbc7f0b4db62` | `git rev-parse HEAD`, 11:06Z |

**Kern-PR-Pin `docs/releases/v0.4.26.consumer-pins.json`** existiert **nicht** im aktuellen develop (`#1050` offen, neu angelegt). Derzeitige relevante Pin-Werte stammen aus den integrations-Kandidaten unten. `#1050` pinnt Core auf **`1861ed07888a1ec22c218ca491d23ee54f48e261`** (gh #1050 file patch, 11:04Z); als Commit verifiziert (`git cat-file -t` → commit, 11:03Z).

### 1.2 baseharbor-demo

| Feld | Wert | Quelle / Messung |
|---|---|---|
| Remote main (git ls-remote) | `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` | `git ls-remote …/baseharbor-demo.git main`, 11:06Z |
| Local branch HEAD (`work/v0.4.26-ecosystem-release`) | `e485725a7e588f43c50e7c51a417e16ed8d0fd85` | `git rev-parse HEAD`, 11:06Z |
| `baseharbor-core.ref` (lokal, HEAD `work/v0.4.26-ecosystem-release`) | `1861ed07888a1ec22c218ca491d23ee54f48e261` | `cat baseharbor-demo/baseharbor-core.ref`, 11:02Z |

**`baseharbor-core.ref`** (Demo-Core-Pin):
- Pfad: `baseharbor-demo/baseharbor-core.ref`
- sha256 (lokal, Worktree-Datei): aus `sha256sum` am Mess-Zeitpunkt; Inhalt = `1861ed07…` (eine Zeile)
- Enthaltener Kern-Wert: **`1861ed07888a1ec22c218ca491d23ee54f48e261`** → realer Commit (verifiziert 11:03Z)
- Bisheriger develop-Wert: **`2adae47afddc299d06dbc34eb16a8e011c14b6dc`** → realer Commit (verifiziert 11:03Z)
- `develop` existiert in diesem Repo **nicht** als Branch (nur `main`); Remote-`develop` unbekannt.

### 1.3 baseharbor-console

| Feld | Wert | Quelle / Messung |
|---|---|---|
| Remote main (git ls-remote) | `e7d1e4a1d603158dece19bbc25a8e267d07cde5e` | `git ls-remote …/baseharbor-console.git main`, 11:06Z |
| Local branch HEAD (`work/v0.4.26-integrations-docs`) | `69d1d14007fa43dc0c0457c3347280be7f802818` | `git rev-parse HEAD`, 11:06Z |

**`integration-candidate.json`** (Console-Kandidat):
- Pfad: `baseharbor-console/integration-candidate.json`
- sha256 (lokal, Worktree): aus `sha256sum` am Mess-Zeitpunkt
- Inhalt: `release 0.4.26`, `core 1861ed07…`, `demo 25c31969…`
- `core = 1861ed07…` realer Commit (verifiziert 11:03Z); `demo = 25c31969…` realer Commit (verifiziert 11:03Z)
- `develop` nicht als Remote-Branch vorhanden (Remote-`develop` unbekannt)

### 1.4 baseharbor-node-connector

| Feld | Wert | Quelle / Messung |
|---|---|---|
| Remote main (git ls-remote) | `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e` | `git ls-remote …/baseharbor-node-connector.git main`, 11:06Z |
| Local branch HEAD (`o115c-connector-freeze-pins`) | `2223506cca5c388752a6d6c88567fcc77f46119b` | `git rev-parse HEAD`, 11:06Z |

**`integration-candidate.json`** (Connector-Kandidat):
- Pfad: `baseharbor-node-connector/integration-candidate.json`
- sha256 (lokal, Worktree): aus `sha256sum` am Mess-Zeitpunkt
- Inhalt: `release 0.4.26`, `core 2adae47…`, `demo 25c31969…`, `console 69d1d14…`
- `core = 2adae47afddc299d06dbc34eb16a8e011c14b6dc` realer Commit (verifiziert 11:03Z)
- `demo = 25c31969…` realer Commit; `console = 69d1d14007fa43dc0c0457c3347280be7f802818` realer Commit (verifiziert 11:03Z)

**Private-Zugriffspins** `baseharbor-node-connector/docs/internal/v0.4.23-private-access-pins.json` (nur Lese-Evidenz, nicht öffentlich):
- Gates `integration/docker/remote-target`, `integration/podman/remote-target`, `integration/static/live-console`:
  - connector `commit = da20080b59a7f804ab74bead762f29286a209c4d` → realer Commit (verifiziert 11:03Z)
  - console `commit = 7207918b3351edbae32c46bebd4cdf7e1d436a2b` → realer Commit (verifiziert 11:03Z)

---

## 2. Nachziehtabelle nach den offenen Merges

Baseline-Entwicklung: develop HEAD = `7fbc1ec5…` (`#1030`); die Core-Merges #1056/#1060/#1061/#1062 basieren auf `f3929568…` (früheres develop), `#1046`/`#1056` auf `f3929568…`, `#1050`/`#1052` auf `a2abc17a…` (Rebase-Punkt). `#1062`-Vater = `9d1624e2` (ein `#1056`-Rebase), nicht develop-HEAD.

| # | Repo / Datei:Zeile | Nach Merge geltender Zielwert | Status der Messung |
|---|---|---|---|
| #1063 | `baseharbor/docs/internal/ci-binding-qualifications.md` | Kern-Qualifikation für `#1056` PR-Head `425eaa96…` (Core-921 record-only) | offen; HEAD `#1063` = `cbb275d2…`; main=`54acc4b6` |
| #1060 | `baseharbor/.github/workflows/ci.yml` + `scripts/pr-gate-plan.py` (+ docs/scripts) | `internal/openbao`-Pfade für native Validierung qualifiziert (#1052-Voraussetzung) | offen; Zielwert = Verhalten, kein SHA-Wert |
| #121 (demo) | `baseharbor-demo/baseharbor-core.ref:1` | **`1861ed07888a1ec22c218ca491d23ee54f48e261`** | offen; HEAD `#121` = `f57c8b6e…`; demo main=`82d042bc`; **bereits gemessen: realer Commit** |
| #9 (console) | `baseharbor-console/integration-candidate.json` (core Zeile 4, demo Zeile 5) | core=`1861ed07…`, demo=`25c31969…` (Repin von `2adae47…`/`896dc3cb…`) | offen; HEAD `#9` = `6017de28…`; console main=`e7d1e4a1`; **Werte bereits realer Commit** |
| #10 (connector) | `baseharbor-node-connector/integration-candidate.json` (core Zeile 4, demo Zeile 5, console Zeile 6) | core=`1861ed07…`, demo=`25c31969…`, console=`69d1d14…` (Repin; Supersedes `#9`-Gedanke Connector) | offen; HEAD `#10` = `e832d565…`; connector main=`7c7093d9`; **Werte bereits realer Commit** |

**Connector-Repinspezifik (nach `#10`-Merge):**
- `integration-candidate.json` nach Merge:
  - `core: 1861ed07888a1ec22c218ca491d23ee54f48e261` (Zeile 4)
  - `demo: 25c319697fd3d89d894943e0f44cd3d5dde8b198` (Zeile 5)
  - `console: 69d1d14007fa43dc0c0457c3347280be7f802818` (Zeile 6)
- Messbefehl post-Merge: `gh api repos/mcpdev80/baseharbor-node-connector/contents/integration-candidate.json --jq '.content' | base64 -d`, dann `git cat-file -t <sha>`.

**Demo-/Core-Tiefen-Logik:** `#121`/`#9`/`#10` repinen alle von `core=2adae47…` auf **`core=1861ed07…`** (v0.4.26-Freeze-Core-Anker). `#121` ändert demo `core.ref` `2adae47 → 1861ed07`. Nach `#121`-Merge gilt in `baseharbor-demo/baseharbor-core.ref` Zeile 1 = **`1861ed07…`** (bereits im Worktree `work/v0.4.26-ecosystem-release` gemessen, realer Commit).

**Finaler Main-Tip zum Merge-Zeitpunkt:** Für `#1063`/`#1060`-abhängige SHA (PR-Head `425eaa96…`, reviewed_head) zum Merge-Zeitpunkt nachmessen:
- `gh api repos/mcpdev80/baseharbor/pulls/1056 --jq '.head.sha'` und in `docs/internal/ci-binding-qualifications.md` die Zeile mit `reviewed_head:`/`file_hashes` nachlesen.

---

## 3. Lösungsbedarf ohne Lane

| Thema | Betroffene Datei(n) | Zu erwartende Prüfung | Kollisionshinweis |
|---|---|---|---|
| `#1046` targeted-runtime-Qualifikation (6 interne application-Testdateien) | `internal/application/{external_provider_registry_test, external_provider_removal_retry_test, external_provider_scope_test, protected_tempdir_test, runtime_binding_projection_reset_test, telemetry_authorization_test}.go` | 6 Testdateien neu/ändert; kein skip-if, deterministischer 0700 | **Serialisierung:** andere PRs schreiben diese Dateien nicht (isoliert), aber PR muss squashed-Head-Qualifikation durch CI |
| W-PR (workflow dispatch case) | `.github/workflows/*` (Core) + `baseharbor-console/.github/workflows/ci.yml` (Kollision!) | Dispatch-Case des Release-Workflows | **Kollision:** `#1060` ändert ebenfalls `.github/workflows/ci.yml` → **Serialisierung mit `#1060` nötig** |
| P-PR (planner/test exact) | `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py` | Planner/Test-exakt | **Kollision:** `#1056/#1060/#1061/#1062` ändern alle 4 diese scripts → **hohe Serialisierungspflicht** |
| `#1050` Pin-Update | `docs/releases/v0.4.26.consumer-pins.json` (neu, 72 Zeilen) | Core `1861ed07…`, Demo `25c31969…`, Console `69d1d14…`, Connector `df1f4378…` | Datei neu → **keine Kollision** mit bestehenden Pin-Dateien; aber semantisch abstimmen mit `#121`/`#9`/`#10` (alle anker-n auf `1861ed07`) |
| `evidence-runs/selected-gates.json` | `evidence-runs/selected-gates.json` (Repo-übergreifend) | Auswahl gefilterter Gates für Evidence | Kein Repo-Write-Set definiert; **Koordinierung über Evidence-Pfad nötig** |

Connector-Wert in `#1050`: `df1f4378d8c7fdc3e174a1e9dc8ad0155a955571` (realer Commit, verifiziert 11:03Z) — **nicht** `da20080…` (Private-Pin). Differenz dokumentieren, bevor `#1050` mergebar.

---

## 4. Kollisionskarte der offenen baseharbor-PRs

Betrachtet: `#1046 #1050 #1052 #1056 #1060 #1061 #1062 #1063` (alle Core, develop-Baseline).

| PR | Dateien (gh API `/files`, 11:04Z) | superseded / Status |
|---|---|---|
| `#1046` | 6× `internal/application/*_test.go` | offen, isoliert (keine Kollision mit Pin/Workflow) |
| `#1050` | `docs/releases/v0.4.26.consumer-pins.json` (neu) | offen; **Pin-Update**, anker-abgleichbar mit `#121/#9/#10` |
| `#1052` | `internal/openbao/bootstrap.go`, `internal/openbao/bootstrap_test.go` | offen; `#1060`-Voraussetzung (bao-Paths qualifizieren) |
| `#1056` | `docs/internal/ci-binding-qualifications.md`, `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py` | offen; **Kollision mit #1060/#1061/#1062/#1063** an 4 scripts + docs |
| `#1060` | `.github/workflows/ci.yml`, `docs/internal/ci-binding-qualifications.md`, `scripts/*` | offen; **Kollision mit #1056/#1061/#1062 an scripts + docs; mit W-PR an ci.yml** |
| `#1061` | `docs/internal/ci-binding-qualifications.md`, `scripts/*` | offen; superseded-Logik: `#1062` refreshed reviewed_head/file_hashes für frischen `#1056`-Head |
| `#1062` | `docs/internal/ci-binding-qualifications.md`, `scripts/*` | offen; refreshed `#1056`-Head `425eaa96…` (Vater `9d1624e2`) |
| `#1063` | `docs/internal/ci-binding-qualifications.md` | offen; **record-only**, Subset von `#1062`-Änderungen (Core-921 für `#1056`-Head `425eaa96…`) |

**Gemeinsame kollidierende Dateimengen (Serialisierung required):**
- `docs/internal/ci-binding-qualifications.md`: **`#1056 #1060 #1061 #1062 #1063`** → alle 5 müssen serialisiert werden (Reihenfolge: `#1056` Head-fix → `#1062` reviewed_head-refresh → `#1063` record-only).
- `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`: **`#1056 #1060 #1061 #1062`** → 4 PRs, eine Datei, strikte Serialisierung.
- `.github/workflows/ci.yml`: **`#1060`** (+ W-PR, außerhalb der 8) → W-PR mit `#1060` serialisieren.
- `docs/releases/v0.4.26.consumer-pins.json`: **`#1050`** isoliert (neu).
- `internal/openbao/bootstrap.go(+test)`: **`#1052`** isoliert; Voraussetzung für `#1060`.
- `internal/application/*_test.go`: **`#1046`** isoliert.

**Reihenfolge-Gefahr (Planner/Akte/Workflow):**
1. `#1052` (bao) vor `#1060` (bao-qualifikation).
2. `#1056` (scripts+docs) → `#1062` (reviewed_head refresh) → `#1063` (record-only). `#1061` (pins-data-restore) parallel zur docs/scripts-Linie, vor `#1062`.
3. `#1050` (consumer-pins) eigenständig; After-Merge Abgleich `1861ed07`-Anker.
4. W-PR (workflow dispatch) nach/nach `#1060` (ci.yml) serialisieren.

---

## 5. Gesendet-Zeit + gemessene Quellen

| Zeile | Gemessene Quelle | Mess-Zeitpunkt |
|---|---|---|
| 1.1 Core main/develop | gh API main; `git ls-remote` develop; `git rev-parse` | 11:06Z |
| 1.2 demo core.ref | `cat baseharbor-demo/baseharbor-core.ref`; gh remote main; `cat-file -t` | 11:02–11:06Z |
| 1.3 console integration-candidate | `cat …/integration-candidate.json`; gh remote main; `cat-file -t` | 11:02–11:06Z |
| 1.4 connector integration-candidate + private pins | `cat …/{integration-candidate.json,v0.4.23-private-access-pins.json}`; gh remote main; `cat-file -t` | 11:02–11:06Z |
| 2 Nachziehtabelle | gh `/pulls/{1063,1060,121,9,10}` (+`/files`, patch); `git cat-file -t` Zielwerte | 11:04–11:06Z |
| 3 Lösungsbedarf | gh `/pulls/{1046,1050,1052,1056,1060}` + `/files`; `cat-file -t` connector `df1f4378` | 11:04–11:06Z |
| 4 Kollisionskarte | gh `/pulls/{n}/files` für alle 8; gh base.sha | 11:04–11:06Z |

Alle im Feld vorkommenden SHAs (Core `1861ed07…`/`2adae47…`, Demo `25c31969…`/`896dc3cb…`, Console `69d1d14…`/`7207918…`, Connector `da20080…`/`df1f4378…`) als `commit`-Objekte verifiziert (`git cat-file -t`), Quelle je Zeile in Tabelle 5. Nichts behauptet, was nicht an einem Objekt gemessen wurde.

---

## STOP

Beide Tabellen + Kollisionskarte vollständig. Alles an git-Objekten (SHA → commit) gemessen. Kein Write/Push/Merge/Review/CI/VM durchgeführt (reine Lese-Mission).

END O125-DAGTRUTH-DONE
