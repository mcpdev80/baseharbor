# o75c - W1 REDO + ROOT-CAUSE (Release v0.4.26)

**Status:** DONE (2026-10-23)
**Prompt:** `evidence/board-2026-10-22/prompts/o75c.txt` (Korrektur o75/o75b: blindes Dispatch vermeiden)
**Owner-Regel:** Release/Pre-Release nur mit gesonderte Freigabe; Merge-Lane nicht selbst ausführen.

## PLAN (o75c)

1. [x] Exakte Input-Namen aus pre-release.yml (`workflow_dispatch`-Block) ermitteln — `gh` git-blob (kein curl).
2. [x] BEIDE Runtimes dispatchen mit tag v0.4.26 / source-ref develop (1861ed07) / demo 25c319697fd3 / connector 570864c7.
3. [x] Runs beobachten, Job-für-Job in Evidence.
4. [x] END O75-W1REDO-DONE.

## Input-Schema (live aus pre-release.yml, ref=develop, blob 2995fdb16c45)

`workflow_dispatch.inputs` (Names aus dem Schema):
- `tag` (required, string) — Release-Tag, z.B. v0.4.26.
- `ref` (required, string, default `develop`) — Source-Ref.
- `demo_ref` (required, string) — exact baseharbor-demo 40-hex SHA.
- `consumer_ref` (optional, string, default '') — exact Connector candidate SHA.
- `only_gates` (optional, string) — comma-sep gate-IDs ohne Release-Freigabe.

`demo_ref` ist REQUIRED (`--field demo_ref=""` → HTTP 422 "Required input demo_ref not provided").

## Vorbedingungen beim Start (40-hex, live verifiziert, gh read-only)

| Ref | Wert | Quelle |
|-----|------|--------|
| core develop | `1861ed07888a1ec22c218ca491d23ee54f48e261` | `git ls-remote develop` |
| core main | `54acc4b66715681213788fdba395a569ddc95a1b` | `git ls-remote main` |
| demo release-branch | `25c319697fd3d89d894943e0f44cd3d5dde8b198` | `git ls-remote work/v0.4.26-ecosystem-release` |
| demo main | `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` | `git ls-remote main` |
| demo main core.ref | `2adae47afddc299d06dbc34eb16a8e011c14b6dc` | `gh api contents/baseharbor-core.ref?ref=main` |
| connector 570864c7 | `570864c7c8bec738438aa70aa967be7e8490d7c3` | `git ls-remote work/v0426-connector-consumption` |
| console main core | `2adae47afddc299d06dbc34eb16a8e011c14b6dc` | integration-candidate.json @ main |
| connector core (570864c7) | `2adae47afddc299d06dbc34eb16a8e011c14b6dc` | integration-candidate.json @ 570864c7 |
| connector console field | `PENDING-FREEZE-DEMO: ...` | integration-candidate.json @ 570864c7 |

## W1-Vertrag LIVE verifiziert

- `pre-release.yml` = `Release · Pre-Release Validation` (id 362658451, workflow_dispatch).
- `source`-Job: `if: ${{ !startsWith(github.ref_name, 'validation/evidence/') }}` — läuft bei `workflow_dispatch` immer.
- v0.4.24/25/26 special-case (pre-release.yml:132): `ecosystem_release_pins.py --tag $tag --core $(git rev-parse HEAD) --connector $INPUT_CONSUMER_REF --demo $INPUT_DEMO_REF`.
- `source_file(repo, commit, path)` (ecosystem_release_pins.py:96): verlangt `exact(commit)` (40-hex), sonst `raise ValueError('immutable source commit required')`.
- `resolve_pins` (ecosystem_release_pins.py:17): verlangt konsistenten 4-Repositorien-DAG; `candidate['core'] == core` und `console_candidate` als 40-hex.

## ROOT-CAUSE des Fails (beide Runs)

Beide Runs schlugen im `source`-Job (`Source · Contracts · Docs`) fehl → alle anderen Jobs `skipped`.

**Kette:**
1. Runner checkoutet `ref=develop` (1861ed07), `git rev-parse HEAD` = 1861ed07 (40-hex, ok).
2. `source_file(connector_repo, '570864c7...', 'integration-candidate.json')` → ok (40-hex), liefert `{core:2adae47a, demo:896dc3cb, console:'PENDING-FREEZE-DEMO: ...'}`.
3. `source_file(console_repo, candidate['console']='PENDING-FREEZE-DEMO: ...', 'integration-candidate.json')` → **'PENDING-FREEZE-DEMO...' ist KEIN 40-hex-SHA' → `raise ValueError('immutable source commit required')'` (ecosystem_release_pins.py:100/125).
4. `Resolve release candidate`-Step → exit 1 → `steps.release.outcome=failure`.
5. `Generate source evidence` / `ci-evidence.sh` → TSV-Zeile `Candidate resolution  failure` → `source`-Job `failure`.
6. `runtime_candidate`/`integration_*`/`runtime_*`/`light_*`/`heavy_*`/`reference_journey` → `skipped` (depends_on source).

**Befund:** Release **v0.4.26 ist noch NICHT releasbar**. Die Console v0.4.26 wurde noch NICHT aus Demo gebaut (`console = PENDING-FREEZE-DEMO` im Connector `integration-candidate.json` @ 570864c7). Der Ecosystem-Pin-Graph (`resolve_pins`) verlangt zwingend den frozen Console-SHA als 40-hex — den es noch nicht gibt. Dies ist kein Konfigurations-/Dispatch-Fehler, sondern ein echter Release-Blocker auf der Produktseite.

**Zusatz:** `docs/releases/v0.4.26.demo-ref` existiert auf develop NICHT (nur `v0.4.26.md`); der v0.4.26-special-case umgeht die demo-ref-file-Logik, ist aber durch den PENDING-FREEZE-Blocker gesperrt. Der im Prompt genannte `consumer_ref=570864c7` ist der alte verbrauchte Pin; er ist für source-ref develop inkonsistent (connector pinnt core=2adae47a, nicht 1861ed07), der eigentliche Fail ist aber der console PENDING-FREEZE-Wert.

## ABLAUF

- `bash -n w1-input.sh` ok (o62). Real-Dispatch via `gh workflow run` (W1_CONFIRM=GO), BEIDE Runtimes.
- Inputs (beide identisch): `--repo mcpdev80/baseharbor --ref develop`,
  `--field tag=v0.4.26 --field ref=develop --field demo_ref=25c319697fd3d89d894943e0f44cd3d5dde8b198 --field consumer_ref=570864c7c8bec738438aa70aa967be7e8490d7c3`.

## run-IDs + Results (job-für-job)

| run | ref | demo_ref | consumer_ref | status | conclusion |
|-----|-----|----------|--------------|--------|------------|
| 39998705831 | develop (1861ed07) | 25c319697fd3 | 570864c7 | completed | failure |
| 39998719700 | develop (1861ed07) | 25c319697fd3 | 570864c7 | completed | failure |

Beide: `source` = failure (PENDING-FREEZE), `runtime_candidate`+alle `skipped`.
Referenz (verfallen, 404): Run 39990941852 (D2, GRUEN); v0.4.26-Rufe (consumer_ref=2adae47a) 2026-10-22 = failure.

## END O75-W1REDO-DONE
