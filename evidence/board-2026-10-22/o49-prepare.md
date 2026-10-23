# O49-PREPARE — v0.4.26 Prepare-Vorbereitung (W1-Sheet, Qualify-Checkliste, D2-Pin-Draft)

**Recorded:** 2026-10-23 UTC
**Lane:** o49-prepare (wip:o49). Mode: DRAFT / NO-PUSH — no dispatches, no pushes to develop/main, no PR created.
**Owner:** o49 (Codeman, prepare lane)
**Source prompt:** `prompts/o49-prepare.txt`

> Paralleloefen — NICHT auf die laufende Selector-Kette eintreten (o48/#1043/#1039/#1042 bleiben unangetastet).

---

## PLAN (todowrite mirror)

- [x] 0. Turn-start Pflichtprogramm: `basics-check.py` (2 un-acked → `events-check.py --ack`, 1792758557 idle events); task contract read.
- [x] 1. Read inputs: `prompts/o49-prepare.txt`, task `t-20261017-release-0-4-26-prepare.md` (state active), o45-sheet-sync.md (W1 sheet lineage), o24-matrix-gonogo.md (GO table), o16-consumer-pins.md (consumer receipts/refreeze), o41-approved-json.md (gap list), o44-cmdb-triage.md (immutable-40-hex finding), RELEASE_WORKFLOW.md#prepare.
- [x] 2. Reality check (read-only gh api): develop = `9f4916be` (C2-final, #1041 merged), demo main = `82d042b`, console work branch `work/v0426-console-consumption` = `0ea88f55…` (40-hex), connector work branch `work/v0426-connector-consumption` = `570864c7…` (40-hex), PR #7/#8 still OPEN/DRAFT (CLEAN merge-state).
- [ ] 3. (1) W1-Matrix-Sheet finalisieren: `consumer_ref`-Felder = LIVE 40-Hex-Tipps der Consumer-Braesche (o44-Befund), develop-Referenz 9f4916be als C2-final-Platzhalter mit TODO-C2-final-Marker.
- [ ] 4. (2) Qualify-Package-Checkliste aus Task-Contract ableiten.
- [ ] 5. (3) D2-Pin-PR-Draft: demo main 82d042b, `baseharbor-core.ref` auf C2-final-Platzhalter; lokaler NICHT gepushter Branch `work/v0426-d2-pin` in baseharbor-demo plus Script zur SHA-Einsetzung nach C2-final-Bekanntwerden.
- [ ] 6. ## repo-rule compliance + END O49-PREPARE-DONE (Dateipfaden der Drafts).

---

## 0) Ref resolution (live @ o49, 2026-10-23)

| Symbol | Wert | Verifiziert (gh api .sha) | Anmerkung |
| --- | --- | --- | --- |
| C2 / develop | `9f4916be333d0f99d7e40001441d9dbb87e993e2` | ✓ develop HEAD | C2-final (#1041 merged). Als **Platzhalter** in W1-Sheet mit TODO-C2-final-Marker (nicht als gefroren behandelt). |
| D2 / demo | `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` | ✓ demo main | Demo main. `baseharbor-core.ref` im Draft = **C2-final-Platzhalter** (kein echtes C2). |
| U / console | `0ea88f553fdac2097e26dce7a5203233a5530527` | ✓ work/v0426-console-consumption | PR #7 OPEN/DRAFT, merge-state CLEAN. |
| N / connector | `570864c7c8bec738438aa70aa967be7e8490d7c3` | ✓ work/v0426-connector-consumption | PR #8 OPEN/DRAFT, merge-state CLEAN. |
| consumer_ref | U-N-Tipps (siehe §2) | ✓ | o44-Befund: leerer consumer_ref war Run-Fehlerursache. W1-Sheet = LIVE 40-Hex-Tipps. |

Predecessor status: S39/#1039 = `0de29deb…`, S42/#1042 = `267152e8…` (frozen Q-predecessor anchors, **nicht angefasst** — o48-Kette läuft).

---

## 1) consumer_ref — o44-Befund und LIVE-Wert

**o44 root cause (run 39865161687):** `scripts/ecosystem_release_pins.py` `source_file()`-Wach (`scripts/ecosystem_release_pins.py:96-100`) verlangt `exact(commit)` = 40 hex für **alle** Repos. Ein `console`-Wert mit <40 Zeichen (z.B. der Short-SHA `896dc3cb` oder leerer `consumer_ref`) löst `ValueError: immutable source commit required` aus → Source-Job failt, alle 5 Jobs (1–5) als Kaskade.

**Konsequenz für W1-Sheet:** `consumer_ref`/`demo`/`console`-Pins müssen zwingend 40-Hex-Voll-SHAs sein. Die LIVE-Tipps der Consumer-Braesche (per `git ls-remote` / `gh api`):

| Receipt-Pfad (Braesche) | Repo | LIVE 40-Hex-Tipp (o49) | Status |
| --- | --- | --- | --- |
| Console `integration-candidate.json` `console` | baseharbor-console | `0ea88f553fdac2097e26dce7a5203233a5530527` | PR #7 DRAFT (nicht gemergt) |
| Connector `integration-candidate.json` `console` | baseharbor-node-connector | `570864c7c8bec738438aa70aa967be7e8490d7c3` | PR #8 DRAFT (nicht gemergt) |
| Demo main (`demo`-Ref) | baseharbor-demo | `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` | demo main (C2-final-Ref fehlt) |

> Hinweis: o16-Refreeze C (2026-10-22) setzt Core=`2adae47a`, Demo=`896dc3c`. Seit C2-final (#1041) ist develop = `9f4916be`; die Consumer-Braeschen zeigen noch auf die C-Reihe (U/N nicht gemergt). Der **LIVE-40-Hex-Tipp** oben ist der aktuelle Branch-Spitzenwert; er ist für die W1-Ausführung als `consumer_ref` verwendbar, wenn der Qualify-Owner die Braesche als gültigen Consumer-Freeze anerkennt. `git ls-remote`-Neulesen vor jeder Ausführung.

---

## 2) W1-MATRIX-SHEET (finalisierter Entwurf)

Quelle: `f9-rerun-prep.md` → o45-sheet-sync.md → o24-matrix-gonogo.md. **Einziger W1-Dispatch** (`pre-release.yml` `workflow_dispatch --ref develop`), alle 65 Rows in einem Lauf.

**Common inputs (jede Row):**
- repo `mcpdev80/baseharbor` · workflow `.github/workflows/pre-release.yml`
- event `workflow_dispatch --ref develop` — **C2-Platzhalter `9f4916be333d0f99d7e40001441d9dbb87e993e2`** (TODO-C2-final: echter gefrorener develop-Head der Release-Phase einsetzen)
- inputs `tag=v0.4.26, ref=C2-Platzhalter, demo_ref=D2, consumer_ref=<LIVE 40-Hex>`
- `consumer_ref` = LIVE 40-Hex-Tipps (siehe §1), **NICHT leer** (o44-Befund)
- `only_gates=""` (CMD-B ⇒ vollständiger Lauf ⇒ `release-approved.json` + coverage-provenance; `RELEASE_WORKFLOW.md:46`)
- concurrency group `baseharbor-release-qualification` (`pre-release.yml:41-43`, queue, `cancel:false`)

**Legend:** YES = C2-nutzbare Flip / nie-executed-first-C2-run. BLOCKED = gated auf Q-predecessor-Merge (#1039/#1042 frozen, unmerged) OR P0-4 (U #7 + N #8 merged) OR pending C2-final demo pin (D2). AT-RISK = C2 allein nicht hinreichend bewiesen.

| # | Gate | Route job (@ C2-Platzhalter) | runtime | dispatch-ready | blocker |
|---|---|---|---|---|---|
| 1 | atomic/static/config-matrix | static_gates `:520` | static | YES | none |
| 2 | atomic/static/init | static_gates `:520` | static | YES | none |
| 3 | atomic/static/mcp | static_gates `:520` | static | YES | f7 GREEN 39831982468 @C → digest-void bei C2 → erneut; kein bekannter C2-Regression |
| 4 | atomic/static/agent | static_gates `:520` | static | YES | none |
| 5 | atomic/static/shell-ux | static_gates `:520` | static | YES | none |
| 6 | atomic/docker/guided | runtime_light_gates_docker `:537` | docker | YES | #1038/#1040 in C2: f7 RED 39832388802 new-edit-new-zero-no = `app_edit_apply_result.go:338-340` in C2 entfernt |
| 7-12 | atomic/docker/{lifecycle,policy,connectivity,reconciliation,failure,full-destroy} | runtime_light_gates_docker `:537` | docker | YES | nie ausgeführt; erster echter C2-Lauf |
| 13 | atomic/docker/data-capabilities | runtime_heavy_gates_docker `:591` | docker | YES | #1038/#1040: f7 RED 39834803429 gleiche Preview-Assert-Klasse in C2 gefixt |
| 14-16 | atomic/docker/{observability,identity,security} | runtime_heavy_gates_docker `:591` | docker | YES | nie ausgeführt; kein f7 RED |
| 17 | atomic/docker/backup-restore | runtime_heavy_gates_docker `:591` | docker | YES | f7 demo-push docker SUCCESS @tree(D); Route jetzt native heavy Matrix; C2 neutral (Fixture-Pfad unverändert) |
| 18-23 | atomic/docker/{rabbitmq,mongodb}-runtime, durable-key-value, control-plane-recovery, loki-runtime, tempo-runtime | runtime_heavy_gates_docker `:591` | docker | YES | nie ausgeführt; erster echter Lauf |
| 24 | atomic/podman/guided | runtime_light_gates_podman `:555` | podman | YES | #1038/#1040: f7 RED 39833309541 identische Klasse in C2 gefixt |
| 25-30 | atomic/podman/{lifecycle,policy,connectivity,reconciliation,failure,full-destroy} | runtime_light_gates_podman `:555` | podman | YES | nie ausgeführt; echter Lauf |
| 31 | atomic/podman/data-capabilities | runtime_heavy_gates_podman `:609` | podman | AT-RISK | sql-no Preview-Klasse in C2 gefixt; Restproblem: f7 8-Min `Run runtime gate` Step-Cap nach Assertion — bei Fix ohne Cancel-Loop passt Row, sonst Step-Cap-Regression (monitor) |
| 32-34 | atomic/podman/{observability,identity,security} | runtime_heavy_gates_podman `:609` | podman | YES | nie ausgeführt |
| 35 | atomic/podman/backup-restore | runtime_heavy_gates_podman `:609` | podman | AT-RISK | f7 GATE PASS / CLEANUP-PROVENANCE FAIL (`destroy=0 preparation=1`) mit q15-Fix #116 bereits in D merged; q15-lineage-Owner muss Further-Fix landen; C2 behebt es nicht |
| 36-41 | atomic/podman/{rabbitmq,mongodb}-runtime, durable-key-value, control-plane-recovery, loki-runtime, tempo-runtime | runtime_heavy_gates_podman `:609` | podman | YES | nie ausgeführt |
| 42-45 | adoption/{conformance,realworld-compose,realworld-quadlet,realworld-kubernetes} | adoption `:427` | ubuntu-latest | YES | f7 RED-adjacent? nein — nie ausgeführt (skipped); in W1 ⇒ F-2 one-at-a-time-Moot |
| 46-49 | ha/docker/{data,identity,observability,routing} | v0421_ha_groups_docker `:645` | docker | BLOCKED (braucht P0-4) | native Route tot (o21 §1-3); HA-Gates fressen Pins ⇒ U #7 merged + Pin-File aktualisiert (P0-4) |
| 50-53 | ha/podman/{data,identity,observability,routing} | v0421_ha_groups_podman `:740` | podman | BLOCKED (braucht P0-4) | wie 46-49, ubuntu-26.04 |
| 54 | journey/docker | reference_journey `:853` | docker | AT-RISK | f7 attempt-2 39845349366 = Preview-Assert-Klasse (C2 gefixt) PLUS `cleanup: incomplete (destroy=0 preparation=1)` (q15-lineage, unfixed) ⇒ prompt-Assert flips, Cleanup-Provenance bleibt riskant |
| 55 | journey/podman | reference_journey `:853` | podman | AT-RISK | f7 39836753432 = F5-HANG-Klasse `no-output timeout` (`tests/guided/drive.py:172`) — f7 B-Owner Terminal-Name/Cancellable-Reader-Abdeckungslücke, NICHT #1038 ⇒ kein bewiesener C2-Flip |
| 56-58 | integration/static/{public-contracts,configuration-policy,browser-terminal} | `:1056/:1096/:1136` | static | YES | nie ausgeführt; echter Lauf |
| 59 | integration/static/live-console | integration_live_console `:1458` | static | BLOCKED (braucht P0-4) | deps core+console+connector+demo Pins (o21 §3 Precondition) |
| 60 | integration/docker/remote-target | integration_remote_target_docker `:1366` | docker | BLOCKED (braucht P0-4) | connector=N (#8) merged + Pins nötig |
| 61-62 | integration/docker/{core-bootstrap,coreless-lifecycle} | `:1176/:1264` | docker | YES | nie ausgeführt |
| 63 | integration/podman/remote-target | integration_remote_target_podman `:1412` | podman | BLOCKED (braucht P0-4) | N (#8) + Pins nötig |
| 64-65 | integration/podman/{core-bootstrap,coreless-lifecycle} | `:1220/:1315` | podman | YES | nie ausgeführt |

W1-Job-Kaskade (q24): `v0421 ha groups · All runtime results`, `runtime heavy gates · All runtime results`, `runtime light gates · All runtime results` — flipten grün, wenn Quell-Job grün. `consumer_ref` (§1) + P0-3-Candidate-Content-Fix sind die eigentlichen Root-Blocker.

### TODO-C2-final-Marker (W1-Sheet)

> ⚠️ **Der obige `ref=C2-Platzhalter 9f4916be…` ist ein Platzhalter für C2-final, NICHT der gefrorene develop-Head der Release-Phase.** Vor W1-Aufführung: (a) echten C2-final-develop-Head als `ref` einsetzen; (b) `release/v0.4.26`-Branch == C2-final-Head erstellen; (c) `demo_ref=D2` auf echtes C2-final-demo (D2=C2) rekeyen; (d) `consumer_ref` auf den LIVE 40-Hex-Tipp der Consumer-Braesche setzen (Nicht-leer, o44-Befund); (e) `docs/releases/v0.4.26.consumer-pins.json` U-Pins {core C2, demo D2, console U} auf 40-Hex setzen. Keine dieser Aktionen ist hier ausgeführt — o49 = NICHT gepushter Entwurf.

### Q-predecessor-Blocker (diese Rows durch Merge gesperrt)

C2-Qualifikations-Lineage, die Q verbraucht, ist auf die zwei gefrorenen Q-predecessor-PRs gesperrt. Bis **beide** gemergt sind, ist der Qualifikations-Anchor nicht geschlossen:

| PR | gefrorener Anchor | aktueller Zustand | Block |
| --- | --- | --- | --- |
| #1039 (S39) | `0de29debf4f91ee3de735ef6d2008eeec880bcdb` | OPEN; CHANGES | Source-Prep (Qualifikations-Tupel, IndentationError, Timeout-Priorität, PR-Inventar) |
| #1042 (S42) | `267152e8ef1c8873bb1ec6518d4d714052ad8e10` | OPEN; CHANGES; Owner-HOLD | verbleibende Leerzeilen in 2 Planner-Dateien; HOLD pending |

> o49 fasst die Q-predecessor-Kette (#1039/#1042) NICHT an (Paralleloefen).

### W2-Diagnostik-Fahrzeug (explizit, nur für W1-Ausfälle)

repo `mcpdev80/baseharbor` · workflow `targeted-demo-acceptance.yml` · event `workflow_dispatch --ref develop` — inputs `ref=C2, demo_ref=D2, runtime=docker|podman, gate=guided|data-capabilities|mcp|reference-journey`; gleiche `baseharbor-release-qualification`-Gruppe → one-at-a-time + terminal-authentiziert vor nächstem.

---

## 3) QUALIFY-PACKAGE-CHECKLISTE (aus Task-Contract abgeleitet)

Aus `t-20261017-release-0-4-26-prepare.md` §Scope/acceptance + o41 gap list. **Alle Punkte sind Qualify-Preconditions (P0), keine W1-Rows.**

### A. Freeze / immutable SHAs
- [ ] A1. `release/v0.4.26`-Branch existiert und == C2-final-develop-Head (immutable source commit) — aktuell FEHLT (404 in "Resolve release candidate").
- [ ] A2. `ref={C2}` = gefrorener develop-Head der Release-Phase — aktuell Platzhalter 9f4916be.
- [ ] A3. `demo_ref={D2}` = exakter 40-Hex-baseharbor-demo-SHA, passend zu `docs/releases/v0.4.26.demo-ref` — aktuell FEHLT / nicht rekeyed auf C2.
- [ ] A4. `consumer_ref` = immutable 40-Hex (v0.4.24/25/26 verlangt es, `pre-release-resume.py ~690`) — o44-Befund: **nie leer**, LIVE 40-Hex-Tipps (siehe §1).
- [ ] A5. `docs/releases/v0.4.26.consumer-pins.json` U-Pins {core C2, demo D2, console U} alle 40-Hex — aktuell FEHLT.

### B. Consumer-Braeschen / U-N-Merge
- [ ] B1. Console PR #7 (`work/v0426-console-consumption`) gemergt → U = console main SHA (aktuell DRAFT, CLEAN merge-state).
- [ ] B2. Connector PR #8 (`work/v0426-connector-consumption`) gemergt → N = connector main SHA (aktuell DRAFT, CLEAN merge-state).
- [ ] B3. `integration-candidate.json` (Console + Connector) `core`/`demo`/`console` = alle 40-Hex (P0-3).
- [ ] B4. U-Pins in `ci.yml`/source.yml auf C2-final gesetzt, wenn Merge (o16 §7c/7d bereits für C-Reihe ausgeführt).

### C. Q-predecessor (durch andere Lane — o49 blockiert nur)
- [ ] C1. #1039 (S39) gemergt (frozen 0de29deb).
- [ ] C2. #1042 (S42) gemergt (frozen 267152e8).

### D. Release-Run / Evidence
- [ ] D1. Ein vollständiger CMD-B-Lauf (`only_gates=""`, alle 65) mit `release-approved.json` + coverage-provenance — aktuell NICHT gelaufen (alle 5 Runs fehlgeschlagen).
- [ ] D2. `release-approved.json` (approval/v2 + consumer_ref + coverage_digest) existiert.
- [ ] D3. `evidence-coverage.json` (coverage/v2 provenance) existiert.
- [ ] D4. `scripts/pre-release-resume.py` @ C2-final + `pr-gate-plan.py trusted_binding_checks()` (line 570) binden.

### E. Product-Abnahmekriterien (Task-Contract §Scope)
- [ ] E1. Core/BaseHarbor, Demo, Console, Node Connector Kompatibilität nachgewiesen.
- [ ] E2. CLI/JSON/MCP-Parität (core-919/920/921).
- [ ] E3. Concise EN/DE Docs/reference/specs, Navigation/Links, Changelog.
- [ ] E4. Human-Release-Nachricht, Kompatibilitäts-/Upgrade-/Deferred-Hinweise.
- [ ] E5. Strict MkDocs Pages candidate builds **ohne** Deployment.

### F. Prozess-Regeln
- [ ] F1. Prozess-Regeln aus Produkt-README entfernen.
- [ ] F2. Erstellter readiness dossier + authorized-run plan; Prep autorisiert **nicht** Release/Promotion/Tag/Publikation/Pages-Deploy.

---

## 4) D2-PIN-PR-DRAFT (baseharbor-demo)

**Zweck:** Demo auf C2-final rekeyen (`baseharbor-core.ref` → C2-final-Platzhalter), wenn C2-final bekannt. Draft ist **NICHT gepusht**, kein PR erstellt. Späterer normaler Push durch MERGE-Lane.

### 4a. Lokaler Branch

- Branch: `work/v0426-d2-pin` in `baseharbor-demo` — **lokal, nicht gepusht**.
- Basis: demo main `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`.

### 4b. Draft-Aenderung

`baseharbor-core.ref`:
```
{C2-FINAL:TODO}
```
Platzhalter statt echtem C2 — wird durch `scripts/apply_d2_pin_sha.sh` ersetzt, sobald C2-final bekannt.

### 4c. SHA-Einsetzungsscript

`evidence/board-2026-10-22/scripts/apply_d2_pin_sha.sh`:
```bash
#!/usr/bin/env bash
# Setzt den C2-final Core-SHA in baseharbor-demo/baseharbor-core.ref (D2-Pin-Draft).
#usage: apply_d2_pin_sha.sh <C2-final-40-hex-sha>
set -euo pipefail
if [ "$#" -ne 1 ]; then
  echo "usage: $0 <C2-final-40-hex-sha>" >&2
  exit 2
fi
C2="$1"
if ! printf '%s' "$C2" | grep -Eq '^[0-9a-f]{40}$'; then
  echo "fehler: <C2-final> muss ein 40-hex SHA sein" >&2
  exit 2
fi
repo="${1:+baseharbor-demo}"
file="baseharbor-demo/baseharbor-core.ref"
printf '%s\n' "$C2" > "$file"
echo "geschrieben: $file = $C2"
echo "Nächstes: git -C baseharbor-demo add baseharbor-core.ref && git commit -m 'demo: pin core auf C2-final' && (späterer normaler Push durch MERGE-Lane)"
```
> Hinweis: Script nur im evidence/.../scripts/-Pfad (keine Produktbranch). C2-final-Sha wird nie hier eingetragen — erst bei Bekanntwerden.

### 4d. Draft-Zustand (Stand o49)

| Feld | aktueller Draft-Wert | bei C2-final-Bekanntwerden |
| --- | --- | --- |
| demo main | `82d042bc…` | unverändert (Demo main bleibt Basis) |
| `baseharbor-core.ref` | `{C2-FINAL:TODO}` | echter C2-final-40-Hex (via script) |
| Branch | `work/v0426-d2-pin` (lokal, unpushed) | commit + späterer normaler Push (MERGE-Lane) |
| PR | keiner | MERGE-Lane erstellt |

---

## repo-rule compliance

- **NO-PUSH lane:** alle 3 Deliverables (W1-Sheet, Qualify-Checkliste, D2-Pin-Draft + script) sind **Drafts im evidence board / lokaler unpushed demo branch**. Kein push an develop/main, kein PR erstellt, kein Workflow dispatcht.
- **Paralleloefen:** Q-predecessor-Kette o48/#1043/#1039/#1042 (S39/S42) **nicht angefasst**; D2-Pin-Draft nur demo-lokal, kein Core-/Connector-/Console-Eingriff.
- **Read-only reality check:** gh api .sha gegen remote HEADs — develop 9f4916be, demo main 82d042b, console work 0ea88f55, connector work 570864c7, PR #7/#8 OPEN/DRAFT (CLEAN) — alle verbatim verifiziert.
- **consumer_ref = LIVE 40-Hex:** o44-Befund eingehalten (leerer consumer_ref war Run-Fehlerursache; 40-Hex-Wach `scripts/ecosystem_release_pins.py:96-100`). W1-Sheet consumer_ref-Felder = LIVE Tipps, nicht leer.
- **Platzhalter-Klarheit:** develop-Referenz 9f4916be als C2-final-Platzhalter mit explizitem TODO-C2-final-Marker (nicht als gefroren behandelt); `baseharbor-core.ref` im Draft = `{C2-FINAL:TODO}`, nicht echtes C2.
- **Evidence-Pfad:** script nur in `evidence/board-2026-10-22/scripts/` (kein Produktbranch); D2-Pin-Draft branch nur lokal.
- **Keine Secrets:** keine Tokens/Credentials/PR-Passwörter geloggt.
- **Turn-start Pflichtprogramm:** basics-check FLAGS (2 un-acked) → events-check acked; Watcher-idle-events als Task-Zustand korrekt ignoriert.

---

**END O49-PREPARE-DONE.**

Draft-Dateien:
- W1-Matrix-Sheet + Qualify-Checkliste: `evidence/board-2026-10-22/o49-prepare.md` (§2, §3)
- D2-Pin-Draft script: `evidence/board-2026-10-22/scripts/apply_d2_pin_sha.sh` (§4c)
- D2-Pin-Draft demo-branch: `baseharbor-demo` branch `work/v0426-d2-pin` (lokal, unpushed; `baseharbor-core.ref` = `{C2-FINAL:TODO}`) (§4d)
