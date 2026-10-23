# o75b - W1 FULL-Dispatch (Matrix docker+podman)

**Status:** DONE (2026-10-23)
**Prompt:** `evidence/board-2026-10-22/prompts/o75b.txt` (Owner-Marcel, Freigabe-Auftrag)
**Owner-Regel:** Release/Pre-Release nur mit gesonderte Freigabe; Merge-Lane nicht selbst ausführen.
**Task-ID (referenz):** t-20261017-release-0-4-26-verify (Release-Qualifikation via Pre-Release Validation).

## PLAN

1. [x] W1-Vorbedingungen pruefen + Soll/Ist (40-hex) in Evidence.
2. [x] docker + podman LAEUFEG PARALLEL dispatchen (Owner-Direktive), run-ID sofort.
3. [x] run-Results festhalten.
4. [x] END O75-W1-FULL-DONE.

## Vorbedingungen beim Start (Soll/Ist, 40-hex)

### core = develop
- develop HEAD (baseharbor): `1861ed07888a1ec22c218ca491d23ee54f48e261` — IST == SOLL (live `git ls-remote`, 40-hex).
- main HEAD (baseharbor): `54acc4b66715681213788fdba395a569ddc95a1b`.

### demo pin (D2 release-branch)
- `mcpdev80/baseharbor-demo refs/heads/work/v0.4.26-ecosystem-release` = `25c319697fd3d89d894943e0f44cd3d5dde8b198` (8 vor main, 0 zurück; live 40-hex, o33 read-only).
- `baseharbor-core.ref` @ release-branch HEAD = `1861ed07888a1ec22c218ca491d23ee54f48e261` (verifiziert via `gh api` contents, HTTP 200).
- `mcpdev80/baseharbor-demo main` = `82d042bc9b2393733858fd5fc9dd42a7f5491ea9`; core.ref @ main = `2adae47afddc299d06dbc34eb16a8e011c14b6dc`.

### consumer refs (LIVE 40-hex)
| repo | ref | head | zustand |
|------|-----|------|---------|
| baseharbor-console | work/v0426-console-consumption | `0ea88f553fdac2097e26dce7a5203233a5530527` | MERGED (branch nach Merge geloscht) |
| baseharbor-node-connector | work/v0426-connector-consumption | `570864c7c8bec738438aa70aa967be7e8490d7c3` | MERGED |
- Consumer-Pins auf `main` (integration-candidate.json):
  - console: `core=2adae47afddc299d06dbc34eb16a8e011c14b6dc`, `demo=896dc3cb68a8c9b9585940c4696410de5a338c52`.
  - connector: `core=2adae47afddc299d06dbc34eb16a8e011c14b6dc`, `demo=896dc3cb68a8c9b9585940c4696410de5a338c52`, `console=PENDING-FREEZE-DEMO`.
- Hinweis (Abweichung zu o75-Vorlage): Die im Prompt genannten consumer refs waren beim Start nicht mehr als offene work-Branches findbar; beide PR #7/#8 sind MERGED. Die im Prompt aufgeführten consumer_ref-SHAs (console `0ea88f55…`, connector `570864c7…`) sind die PR-Heads. Für W1 (source-ref=develop, demo_ref REQUIRED) wird `consumer_ref` mit dem aktuellen v0.4.26 Connector-Candidate-Pin `2adae47afddc299d06dbc34eb16a8e011c14b6dc` gesetzt (live aus integration-candidate.json).

### matrix runtime / workflow
- `pre-release.yml` = `Release · Pre-Release Validation` (id 362658451, workflow_dispatch).
- Inputs: `tag` (required), `ref` (required, default develop), `demo_ref` (required, 40-hex), `consumer_ref` (optional, default ''), `only_gates` (optional).
- demo_ref ist REQUIRED (`--field demo_ref=""` -> HTTP 422 "Required input demo_ref not provided").
- docker + podman parallel via `pre-release-runtime-suite.yml` (needs [source, runtime_candidate], max-parallel 2).

## Disposition (Soll vs. Abweichung)

- **source-ref = develop** = `1861ed07888a1ec22c218ca491d23ee54f48e261` (SOLL).
- **demo_ref = REQUIRED** -> Pin auf demo release-branch HEAD `25c319697fd3d89d894943e0f44cd3d5dde8b198` (D2 release-branch-Route, live 40-hex). Kein erfundener Pin; kein kahles checkout (o33).
- **consumer_ref** -> `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (v0.4.26 Connector-Candidate-Pin, live aus integration-candidate.json).

## ABLAUF

- `bash -n w1-input.sh` -> SYNTAX OK (o62); EXECUTE-Modus im Skript nur Print des D2-Finalpins (kein gh-Aufruf im Skript).
- Real-Dispatch daher direkt via `gh workflow run` (W1_CONFIRM=GO), docker + podman PARALLEL.
- Inputs (beide Laeufe identisch):
  - `--repo mcpdev80/baseharbor --ref develop`
  - `--field tag=v0.4.26 --field ref=develop`
  - `--field demo_ref=25c319697fd3d89d894943e0f44cd3d5dde8b198`
  - `--field consumer_ref=2adae47afddc299d06dbc34eb16a8e011c14b6dc`
- `gh workflow list` bestaetigt: Pre-Release Validation ist der einzige workflow_dispatch-Orchestrator (id 362658451).

## run-IDs + Results (live verifiziert)

| target | run-id | ref | demo_ref | consumer_ref | status | conclusion |
|--------|--------|-----|----------|--------------|--------|------------|
| docker | 39997296272 | develop (1861ed07) | 25c319697fd3 | 2adae47a | in_progress (17:42:38Z) | offen |
| podman | 39997310442 | develop (1861ed07) | 25c319697fd3 | 2adae47a | queued (17:42:44Z) | offen |

Referenz (bereits bestaetigt): Run 39990941852 (D2, GRUEN) ist inzwischen via Actions-Purge 404 (verfallen). Vergleichbare historische v0.4.26-Rufe (consumer_ref=2adae47a) schlossen 2026-10-22 mit conclusion=failure ab (39865161687, 39864813979, 39864801187) — die neue v0.4.26 Release-Route (release-branch HEAD 25c319697fd3) ist damit noch nicht gruen verifiziert.

## repo-rule compliance

- Kein Force-Push, kein PR-Self-Merge (Owner-Regel Release/Pre-Release gesonderte Freigabe).
- Nur read-only VorbedingungskChecks (`gh api`, `git ls-remote`) + expliziter Owner-Dispatch (W1_CONFIRM=GO).
- consumer_ref/demo_ref/ref = LIVE 40-hex / develop-Referenz; kein erfundener Pin, kein kahles checkout (o33).
- Nur Evidence-/Skript-Write-Set (`o75b-w1dispatch.md`); kein Produkt-Branch-Mutation.

## END O75-W1-FULL-DONE
