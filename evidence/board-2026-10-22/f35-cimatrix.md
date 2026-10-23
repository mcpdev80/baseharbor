# F35 – CI-Matrix Analyse PR #1050 / #1052 / #1053 (v0.4.26)

[done:f35] — READ-ONLY Analyse, KEINE Aenderung an Produkt oder PRs. F35-CIMATRIX-DONE.

## Auftrag
CI-Kompatibilitaet der drei Rest-PR-Pfade fuer v0.4.26 analysieren:
- PR #1050 (data, docs/releases/*.consumer-pins.json)
- PR #1052 (vault retry, internal/vault)
- PR #1053 (cli wizard, internal/cli)

Fuer jeden PR: aktuelle Required-Check-Status live (gh pr checks) + welche Checks
laufen/skippen/failen und WARUM (Rule-Kategorisierung via scripts/pr-gate-plan.py
nachvollziehbar). Ziel: Tabelle, die nach o90-Merge voraussagt, welche PRs sofort
gruen werden.

## PLAN (final)
- [x] Evidence-Datei mit PLAN angelegt (pwd-PFLICHT: gh/git nur in baseharbor/)
- [x] Live-Erfassung gh pr checks #1050 / #1052 / #1053 (Required-Checks)
- [x] pr-gate-plan.py Rule-Tracing: docs/releases/*.consumer-pins.json / internal/vault(=openbao) / internal/cli(=cmd/baha)
- [x] Je PR: Checks running/skipped/failing + Begruendung
- [x] o90-Merge-Kontext klären (PR #1054, work/v0.4.26-gateplan-pins @ 4f294811)
- [x] Vorhersage-Tabelle: welche PRs nach o90 sofort gruen
- [x] Abschluss F35-CIMATRIX-DONE mit Tabelle

## Kontext (verifiziert)
- develop-Tip = `1861ed07888a1ec22c218ca491d23ee54f48e261` (= EXPECTED_BASE aller vier PR-Runs).
- o90 = PR **#1054** `work/v0.4.26-gateplan-pins`, Head `4f294811d2e4c6ac1d6da5bb052cb04fba40064d`, OPEN, MERGEABLE, enthaelt Merge von #1050-Branch (`d84583a8`); Pins-JSON-Blob identisch zu #1050 (sha256 `ba77fc36…8452`).
- Branch-Schutz: Ruleset „develop - required checks" (id 25223521, enforcement active, strict=true): **einziger Required-Kontext = `PR Selected Checks`**. Alle anderen Jobs sind informativ.
- Planner-Mechanik (`.github/workflows/ci.yml` pr_plan-Job, checkout `ref: pull_request.base.sha`, Zeile ~451/458):
  `scripts/pr-gate-plan.py` und `scripts/pr-check-runner.py` laufen **immer vom Target-Base-SHA, nie aus PR-Inhalt**.
  Regeländerungen greifen fuer einen PR also erst, NACHDEM sie in develop gemerged sind (Bootstrap-Semantik,erklaert warum #1054 an seinen eigenen Regeln scheitert).

## Live-Status (gh pr checks, 2026-10-24)
| PR | Head | Run | Required-Check `PR Selected Checks` | Nicht-required |
|---|---|---|---|---|
| #1050 (data) | `e551bc8b` | 40028406804 | **fail** (11s, job 120670776669) | Trusted Plan/Focused Python/Workflow policy pass; Validation=`not-required` pass; alle HA/Consumer/Demo/Exact/Runner/Public/Selected/validate/release = skipping |
| #1052 (openbao retry) | `e371ebdc` | 40030635102 | **fail** (8s, job 120678310352) | wie #1050 (Validation=`not-required` pass) |
| #1053 (cli wizard) | `8eef2b8e` | 40031386546 | **pass** (8s, job 120680840632) | Validation: cli-build, cli-gofmt, cli-vet, source-contracts, wizard-controller-tests alle pass; uebrige skipping |
| #1054 (o90, Referenz) | `4f294811` | 40034327382 | **fail** (job 120690543638) | Validations ci-policy/pr-planner-tests/ci-executor-verifier-tests pass |

Fehlermeldungen (log-failed, `pr-required-final.py:25,32`):
- #1050: `BLOCKED: coverage-unresolved: unknown impact for docs/releases/v0.4.26.consumer-pins.json` + `no required gates or validations`
- #1052: `BLOCKED: coverage-unresolved: unknown impact for internal/openbao/bootstrap.go; unknown impact for internal/openbao/bootstrap_test.go` + `no required gates or validations`
- #1054: `BLOCKED: coverage-unresolved: unknown impact for docs/releases/v0.4.26.consumer-pins.json; CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py`
- PR-Inhalte (gh pr diff --name-only): #1050 = nur die eine Pins-JSON-Addition; #1052 = `internal/openbao/bootstrap{,_test}.go`; #1053 = `cmd/baha/app_wizard_controller{,_test}.go`.

## Rule-Tracing (develop@1861ed07 `scripts/pr-gate-plan.py`)
Klassifikationskette ~Zeilen 1040–1185; sonst-Zweig `else: unresolved.append('unknown impact for '+path)` bei :1183.
- `docs/releases/*.consumer-pins.json` → **keine Rule** (Docs-Regel :1108 greift nur `.md/.rst`/site) → else → unresolved. o90 fuegt die `pins-data`-Rule ein (diff :592ff, :1060ff: area `pins-data`, `require(PINNED_PINS_VALIDATION)`, verlangt path-Status `added`, sonst unresolved) + Runner-Validierung `validate_pins_data` (pr-check-runner.py, +72).
- `internal/openbao/**` → **keine Rule** (es existieren nur `internal/capability/` :1132, `internal/application/` :1137, `internal/repositoryinspect/` :1150) → else → unresolved. **Auch der o90-Branch enthaelt keine openbao-Rule** (git grep: kein `internal/openbao` in scripts/ des o90-Diffs). Hinweis Prompt-Diskrepanz: Auftrag sagte „internal/vault", real ist `internal/openbao`.
- `cmd/baha/app_wizard_controller{,_test}.go` → WIZARD_COMPONENTS-Map :99–100 → `wizard-controller-tests` + CODE_CHECKS + WIZARD_BASE_CHECKS, Status modified → **resolved, keine native Gates** → exakt das gruene Bild von #1053.
- `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py` → PLANNING_SCRIPTS (area `ci-planner`, CI_CHECKS); `scripts/pr-check-runner.py` → Zweig :1171ff `ci-executor-unresolved` → unresolved (benoetigt Base-seitige Qualifikation; Praezedenz: hartkodierte #953/#1008 Reviewed-Chain-Ausnahmen ~:1000–1060, die selbst im Base existieren muessen).

## Lokale Reproduktion (read-only, /tmp)
`validate_pins_data` aus o90-Runner gegen Merge-Tree-Simulation (develop-Pins + #1050-Pins):
`validated: ['v0.4.23.consumer-pins.json', 'v0.4.26.consumer-pins.json']` → **PASS** (rglob findet alle docs/releases/*.consumer-pins.json; beide erfuellen Schema/Connector-Invarianten).

## Vorhersage-Tabelle post-o90-Merge (Rerun mit Base=develop+#1054)
| PR | Regel im neuen Base | Erwartete Checks | Prognose Required-Check | Sofort gruen? |
|---|---|---|---|---|
| #1050 | pins-data-Rule greift (Addition ✓); Validation `pins-data-validation` (lokal PASS) | Trusted Plan, Focused Python, Workflow policy, **Validation · pins-data-validation** laufen; HA/Consumer/etc. skippen | **pass** | **JA** – aber NUR wenn develop das Pins-File noch nicht enthaelt. Da #1054 den identischen Blob mitfuehrt, waere nach o90-Merge der Diff von #1050 leer → `no required gates or validations` → erneut FAIL (Sonderfall, siehe Befund 2). Sauberer Pfad: #1050 vor/gleichzeitig mit o90 mergen oder o90 ohne Pins-File. |
| #1052 | weiterhin keine openbao-Rule (auch nicht in o90) | wie jetzt; Selected Checks failt | **fail** (unveraendert coverage-unresolved) | **NEIN** – braucht eigene Planner-Rule/Qualifikation fuer `internal/openbao` (secrets-Runtime-Impact; vermutlich selected-runtime/HA-Qualifikation noetig). |
| #1053 | unveraendert WIZARD_COMPONENTS | gleiche gruene Matrix | **pass** (nach Rerun wegen strict-Base-Move) | **JA** – bereits gruen; Base-Bewegung erzwungen nur einen Rerun, Klassifikation bleibt resolved. |
| #1054 (o90) | n/a (Self-Bootstrap) | fails an eigenem Base | **fail wie gepusht** | NEIN in aktuellem Stand – muss vor Merge selbst gruen werden: Pins-File raus (lokaler WIP-Commit `3c02f48c` loescht es bereits) + `scripts/pr-check-runner.py`-Unresolved aufloesen. |

## Befunde
1. **ABWEICHUNG Prompt vs. Realitaet:** „internal/vault" existiert nicht; #1052 aendert `internal/openbao`. Beide Pfad-Kategorien (consumer-pins, openbao) haben in develop KEINE Rule → fail-closed BLOCKED; „no required gates or validations" ist die Folge (else-Zweig require nichts), nicht ein eigener Fehler.
2. **o90-Bootstrap-Dilemma:** Weil der Planner strikt aus dem Base läuft, kann #1054 weder seine eigene pins-Rule noch eine `pr-check-runner.py`-Qualifikation fuer seinen eigenen Lauf aktivieren. Solange #1054 das Pins-JSON mitführt, bleibt es zweifach unresolved. Der gelebte Praezedenz (CORE_752/#1008 reviewed-exact-chain) zeigt den ueblichen Weg: exakte, im Base hartkodierte Reviewed-Bundle-Ausnahmen.
3. **Parallel-Lane aktiv:** `baseharbor/worktrees/o90` HEAD (`3c02f48c`) != gepushter PR-Head (`4f294811`); Lane arbeitet am Branch. F35 hat nichts angefasst (read-only).
4. Basics-FLAGS (Coordinator-Ebene): 107 Reminder unread, PLAN 65min untouched, 16 offene Steps, ornith 1<2 — ausserhalb F35-Scope, nicht abgearbeitet. Kein Events-Quitting (nur idle-Event 74e10b98), daher kein --ack noetig.

## repo-rule compliance
READ-ONLY eingehalten: keine Aenderungen in baseharbor/, kein Checkout/Reset/Commit/Push; gh nur lesend (pr view/checks/diff --name-only, api GETs); Git nur log/show/diff/ls-tree/merge-base/cat-file; Validierungs-Reproduktion ausschliesslich in /tmp/opencode. Keine Secrets geloggt (GH_TOKEN im Runner-Log ausgeblendet).
