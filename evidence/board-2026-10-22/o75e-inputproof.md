# o75e - TECHNISCHER BEWEIS: Inputs wurden gesetzt (Prompt-Klage widerlegt)

**Status:** DONE (2026-10-23)
**Prompt:** `evidence/board-2026-10-22/prompts/o75e.txt`
**Klage im Prompt:** "API zeigt fuer alle drei Runs 'inputs: {}' - deine gh workflow run Kommandos setzen die Inputs NICHT."

## PLAN (o75e)

1. [x] Dry-Run mit `gh api --method POST .../dispatches --field inputs[...]=...` (prompt-Methode).
2. [x] Nach 20s per `gh api .../runs?per_page=1 --jq .workflow_runs[0].inputs` pruefen.
3. [x] runner-interne Input-Werte aus dem Job-Log gegenlesen (ground truth).
4. [x] conclusions in Evidence, END O75-W1REDO3-DONE.

## BEWEIS 1: runner-interne Input-Werte (ground truth, aus Job-Log)

`gh api .../jobs/<sid>/logs` zeigt die tatsaechlich im Runner gesetzten env-Variablen:

| Run | INPUT_TAG | INPUT_DEMO_REF | INPUT_CONSUMER_REF | Ergebnis |
|-----|-----------|----------------|--------------------|----------|
| 39998705831 | v0.4.26 | 25c319697fd3… | 570864c7c8bec7… | failure (PENDING-FREEZE) |
| 39998719700 | v0.4.26 | 25c319697fd3… | 570864c7c8bec7… | failure (PENDING-FREEZE) |
| 39999967958 | v0.4.26 | 25c319697fd3… | 570864c7c8bec7… | failure (PENDING-FREEZE) |
| 40000941427 | v0.4.26 | 25c319697fd3… | 570864c7c8bec7… | failure (PENDING-FREEZE) |

Beispiel-Logzeilen (Run 39999967958 / 40000941427):
```
INPUT_TAG: v0.4.26
INPUT_DEMO_REF: 25c319697fd3d89d894943e0f44cd3d5dde8b198
INPUT_CONSUMER_REF: 570864c7c8bec738438aa70aa967be7e8490d7c3
...
raise ValueError('immutable source commit required')
ValueError: immutable source commit required
```

Die Inputs sind also **definitiv gesetzt und im Runner angekommen**. Der runner-interne Kontext (env vars) ist der maessgebliche Beweis; `.workflow_runs[0].inputs` resp. `.inputs` auf dem run-Objekt ist fuer `workflow_dispatch` schlicht **nicht befuellt** (GitHub-API-Verhalten: Inputs des Dispatches leben nur im `dispatches`-Endpunkt).

## BEWEIS 2: Dry-Run per prompt-Empfehlung (`gh api --field inputs[...]`)

```
gh api --method POST repos/mcpdev80/baseharbor/actions/workflows/pre-release.yml/dispatches \
  --field ref=develop \
  --field 'inputs[tag]=v0.4.26' \
  --field 'inputs[ref]=develop' \
  --field 'inputs[demo_ref]=25c319697fd3d89d894943e0f44cd3d5dde8b198' \
  --field 'inputs[consumer_ref]=570864c7c8bec738438aa70aa967be7e8490d7c3'
```
- Exit 0, erzeugt real Run 40000941427.
- Nach ~20s: Run-Log bestaetigt INPUT_* = oben. Also: Inputs WERDEN gesetzt — beide Dispatch-Pfade (`gh workflow run` UND `gh api --field inputs[...]`) sind korrekt.

## BEWEIS 3: `sed` auf die Inputs-Definition im Workflow (Key-Namen)

`.github/workflows/pre-release.yml` (ref=develop, blob 2995fdb16c45) `workflow_dispatch.inputs`:
- `tag` (required, string)
- `ref` (required, string, default `develop`)
- `demo_ref` (required, string)
- `consumer_ref` (optional, string, default '')
- `only_gates` (optional, string)

Key-Namen exakt wie im Prompt vermutet (tag/ref/demo_ref/consumer_ref/only_gates). `demo_ref` REQUIRED — `--field demo_ref=""` → HTTP 422.

## WICHTIG: Der Fail ist NICHT die Input-Aufloesung

Der Prompt vermutet `--field`-/Syntax-/Variable-Aufloesungsfehler. **Falsch.** Die Inputs kamen sauber an (Beweis 1+2). Der eigentliche, reproduzierbare Fail ist der Produkt-Content:

> connector `integration-candidate.json` (570864c7 und Auto-Resolve-Branch `work/v0.4.26-integrations-docs`) hat `console: "PENDING-FREEZE-DEMO: …"` resp. `console/demo: "PENDING-FREEZE"`.
> `source_file()` (ecosystem_release_pins.py:100) wirft `ValueError: immutable source commit required`, weil `PENDING-FREEZE…` kein 40-hex-SHA ist.
> `source`-Job failure → alle Runtimes `skipped`.

Release **v0.4.26 ist noch nicht releasbar** (Console v0.4.26 noch nicht aus Demo gebaut). Dies aendert auch ein korrektes Input-Setze NICHT.

## run-IDs + conclusions

| run | dispatch-Methode | inputs gesetzt | conclusion |
|-----|------------------|----------------|------------|
| 39998705831 | `gh workflow run` | ja (log) | failure (PENDING-FREEZE) |
| 39998719700 | `gh workflow run` | ja (log) | failure (PENDING-FREEZE) |
| 39999967958 | `gh workflow run` | ja (log) | failure (PENDING-FREEZE) |
| 40000941427 | `gh api --field inputs[...]` | ja (log) | failure (PENDING-FREEZE) |

## END O75-W1REDO3-DONE
