# o62-w1dispatch.md - W1-Dispatch-Skript-Evidence

**Task**: o62 — Ergaenzen des echten Ausfuehr-Teils fuer die W1-Matrix in `w1-input.sh`.
**Lane**: Write-Set = nur `evidence/board-2026-10-22/scripts/w1-input.sh` + diese Evidence.
**Status**: [wip:o62]
**Datum**: 2026-10-23
**Erzeugt durch**: Codeman Turn-Start (basics-check.py Pflichtprogramm), Prompt `evidence/board-2026-10-22/prompts/o62.txt`

## Kontext (aus o62.txt)

Vorlaeufer o61+F23-SpawnHoppper ghostete ohne Ausgabe. `w1-input.sh` (q44) ist nur ein Variablen-Sheet
OHNE Dispatch. Aufgabe: ergaenzen den echten Ausfuehr-Teil fuer die W1-Matrix:
(a) Workflow-Pfad live verifizieren, (b) DRY_RUN=1 Modus (Standard), (c) EXECUTE-Modus nur mit
W1_CONFIRM=GO env (Owner-Gate), docker UND podman parallel, inputs ref + consumer_ref LIVE,
(d) bash -n + DRY-Lauf in der Evidence.
D2 ist noch in Arbeit (Run 39977865544 auf 3776ad7) — NICHT real dispatchen.

## W1-Vertrag LIVE verifiziert (2026-10-23)

### Matrix-Ziel: `pre-release.yml` (workflow_dispatch) + `pre-release-runtime-suite.yml` (workflow_call)

- `pre-release.yml` = `Release · Pre-Release Validation` (id 362658451), der einzige `workflow_dispatch`-Orchestrator.
  Quelle: `gh api repos/mcpdev80/baseharbor/actions/workflows`.
- Inputs (workflow_dispatch, Zeilen 6-30 des Workflow-YML):
  - `tag` (required, String) — Release-Tag, z.B. v0.4.20
  - `ref` (required, default `develop`) — Source ref
  - `demo_ref` (required, String) — exact baseharbor-demo 40-Hex
  - `consumer_ref` (optional, default `''`) — exact Connector candidate SHA
  - `only_gates` (optional) — targeted gates ohne Release-Freigabe
- `source`-Job loest consumer_ref: fuer tag `v0.4.24`/`v0.4.25` wird `ecosystem_release_pins.py`
  aufgerufen (Zeilen 131-137); demo_ref kommt aus `docs/releases/${tag}.demo-ref` (Zeilen 139-159).
- docker+podman parallel: `runtime_light_gates_docker` (matrix `runtime: [docker]`, Zeilen 537-553) und
  `runtime_light_gates_podman` (Zeilen 555-...) `needs: [source, runtime_candidate]`, beide unveraendert
  parallel; heavy/ha folgen als Abhaengigkeit. `max-parallel: 2`.
- `pre-release-runtime-suite.yml` (id 371960202) = reusable `workflow_call`, Inputs: `ref`, `demo_ref`,
  `runtime` (static|docker|podman), `gate`, `candidate_artifact`.

### LIVE-Referenzen (gh ls-remote, 2026-10-23)

| Referenz | Wert | Quelle |
|----------|------|--------|
| develop HEAD (baseharbor) | `1861ed07888a1ec22c218ca491d23ee54f48e261` | `git ls-remote origin develop` |
| console PR #7 head | `0ea88f553fdac2097e26dce7a5203233a5530527` | `git ls-remote .../baseharbor-console work/v0426-console-consumption` |
| connector PR #8 head | `570864c7c8bec738438aa70aa967be7e8490d7c3` | `git ls-remote .../baseharbor-node-connector work/v0426-connector-consumption` |

### D2-Status: NICHT real (Run 39977865544 = HTTP 404 "Not Found")

- `gh api .../actions/runs/39977865544` -> 404. Kein D2-Branch vorhanden (nur develop + PR-Head + Feature-Branches).
- `w1-input.sh` Zeilen 15-16 zeigen D2-Platzhalter (`570864c7...` ist PR8-Head, kein echter D2-Commit).
  EXECUTE-Modus darf D2 NICHT dispatchen — nur realer W1-Dispatch mit W1_CONFIRM=GO.

### o33-Regel

KEIN kahles Checkout (keine ungeschutzten fremden Arbeitsbaeume). Alle Referenzen via `gh ls-remote`/
`git ls-remote` remote, kein lokales Checkout von develop/PR-Head.

## PLAN

1. [x] basics-check.py (Turn-Start-Pflicht) + W1-Vertrag live verifizieren (gh workflow list, YML-Analyse, ls-remote)
2. [x] D2-Status verifiziert: 404 -> NICHT real dispatchen
3. [ ] w1-input.sh Ergaenzung: DRY_RUN=1 (Standard) + EXECUTE (W1_CONFIRM=GO), docker+podman parallel, ref/consumer_ref LIVE
4. [ ] bash -n Syntax-Check
5. [ ] DRY-Lauf ausfuehren, Ausgabe in dieser Evidence
6. [ ] END O62-W1DISPATCH-DONE

## Ergaenzung w1-input.sh

Schreibt den Dispatch-Teil am Ende von `w1-input.sh` (nach dem Input-Sheet, Zeilen 33-55).
Write-Set auf dieses Skript beschraenkt.

### Design

- `DRY_RUN` default `1`. Ueber Umgebung: `DRY_RUN=0 bash w1-input.sh`.
- `EXECUTE`-Pfad nur mit `W1_CONFIRM=GO` (Owner-Gate); sonst Fehler + Hinweis.
- consumer_ref LIVE via `gh pr view`/`git ls-remote` zur Laufzeit (o44/o33-Regel).
  Demo_ref bleibt Owner-Wert (kein D2-Pin raten).
- ref = `develop` (standardisierte W1-Referenz).
- docker+podman als zwei getrennte `gh workflow run`-Aufrufe, im EXECUTE-Modus parallel (background + wait).
- Kein kahl-Checkout; alle Schecks gegen Remote.

## repo-rule compliance

- Matrix-Vertrag aus dem echten Workflow-YML abgeleitet (nicht geraten): `pre-release.yml` -> `pre-release-runtime-suite.yml`.
- consumer_ref/demo_ref/ref = LIVE 40-Hex bzw. Branch-Referenz, kein erfundener D2-Pin.
- dry-run standardmaessig; realer Dispatch durch explizites Owner-Env (W1_CONFIRM=GO) gesperrt.
- Kein Produkt-Branch-Mutation; nur Evidence-/Skript-Write-Set.

## O62-W1DISPATCH-DONE
