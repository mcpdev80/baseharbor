# Q81 — Source-Job-Fehler in W1 trotz validen Inputs: Root-Cause-Analyse

Lane: q81-sourcelog — Typ: READ-ONLY-Analyse (kein Produkt, kein Dispatch; W1 = f29 dispatcht)
Tag: [wip:q81]
Produkt-Repo: `mcpdev80/baseharbor` — Workflow: `.github/workflows/pre-release.yml`
Betreff: Warum scheitert der Source-Job in W1 trotz korrekter (40-hex) `$INPUT_*`-Inputs im Source-Job-Log?

---

## (a) Exakte Resolver-Zeile mit den TATSAECHLICH angekommenen `$INPUT_*`-Werten

Beide Runs: die `$INPUT_*`-Werte kamen im Source-Job als korrekte 40-hex-SHAs an. Der Auslöser des Absturzes
war NICHT die Input-Syntax, sondern der **Inhalt** des Connector-Candidates auf diesem SHA.

**Run 40001750271 — Pre-Release · v0.4.26 (run 225), develop @ `1861ed07`**
Source-Job ID `120579336953` (`.github/workflows/pre-release.yml`, workflow_dispatch, 18:15:03Z, head `1861ed07`).
Angekommene Inputs (Source-job-Log, Zeilen 284–286, `env:`-Block):

```
INPUT_TAG:     v0.4.26
INPUT_DEMO_REF:  25c319697fd3d89d894943e0f44cd3d5dde8b198
INPUT_CONSUMER_REF: 570864c7c8bec738438aa70aa967be7e8490d7c3
```

Absturz (Log-Zeilen 294–303):

```
fatal: couldn't find remote ref refs/heads/release/v0.4.26
Candidate follows the current develop release line.
Traceback (most recent call last):
  File ".../scripts/ecosystem_release_pins.py", line 136, in <module>
    main()
  File ".../scripts/ecosystem_release_pins.py", line 125, in main
    console_candidate = json.loads(source_file(REPOSITORIES['console'], candidate['console'], 'integration-candidate.json'))
  File ".../scripts/ecosystem_release_pins.py", line 100, in source_file
    raise ValueError('immutable source commit required')
ValueError: immutable source commit required
##[error]Process completed with exit code 1.
```

**Run 40001628646 — Pre-Release · v0.4.26 (run 224, ohne `--ref`, head `54acc4b`)**
Source-Job ID `120578909399`. Gleich angekommene Inputs (Log-Zeilen 284–286):

```
INPUT_TAG:          v0.4.26
INPUT_DEMO_REF:     25c319697fd3d89d894943e0f44cd3d5dde8b198
INPUT_CONSUMER_REF: 570864c7c8bec738438aa70aa967be7e8490d7c3
```

Dieser Run stürzte an einer **früheren** Stelle ab — nicht am Console-Pin, sondern an der Demo-Ref-Datei
(Log-Zeilen 292–295):

```
fatal: couldn't find remote ref refs/heads/release/v0.4.26
Candidate follows the current develop release line.
Exact demo revision file is missing: docs/releases/v0.4.26.demo-ref
##[error]Process completed with exit code 1.
```

Beide Runs: `INPUT_CONSUMER_REF=570864c7c8bec738438aa70aa967be7e8490d7c3` (40-hex, `git rev-parse`-tauglich),
`INPUT_DEMO_REF=25c319697fd3d89d894943e0f44cd3d5dde8b198` (40-hex), `INPUT_TAG=v0.4.26`. **Die Inputs waren
syntaxkorrekt — das ist die Prämisse aus q81, hier live bestätigt.**

---

## (b) Die im Skript gelesene Stelle — welche Variable „leer"/ungültig war

Fundstelle: `scripts/ecosystem_release_pins.py` (`main()`, Quelle live on develop @ `1861ed07`, Zeilen 124–126):

```python
candidate = json.loads(source_file(REPOSITORIES['connector'], connector, 'integration-candidate.json'))           # L124
console_candidate = json.loads(source_file(REPOSITORIES['console'], candidate['console'], 'integration-candidate.json'))  # L125
demo_core = source_file('mcpdev80/baseharbor-demo', candidate['demo'], 'baseharbor-core.ref')                     # L126
```

`connector` kommt aus `--connector "$INPUT_CONSUMER_REF"`; `candidate['console']` wird aus dem **gepatchten**
Connector-`integration-candidate.json` gelesen (L124 decoded → dict). Der Absturz liegt auf **L125**: `source_file()`
wird mit `REPOSITORIES['console']=mcpdev80/baseharbor-console` und dem Wert `candidate['console']` aufgerufen.

`source_file()` (Zeilen 96–104) hat zwei Guards:

```python
def source_file(repository, commit, path):
    if repository not in REPOSITORIES.values() and repository != 'mcpdev80/baseharbor-demo':
        raise ValueError('unreviewed source repository')
    if not exact(commit):                       # <-- L100, hier Absturz run 225
        raise ValueError('immutable source commit required')
    ...
```

`exact(commit)` (Zeilen 13–20) verlangt ein exaktes 40-hex-SHA:

```python
def exact(value):
    return isinstance(value, str) and re.fullmatch(r'[0-9a-f]{40}', value) is not None
```

**Was „leeric"/ungültig war:** `candidate['console']` ist **kein leerer String, aber auch kein 40-hex-SHA** — es
ist der Text `"PENDING-FREEZE-DEMO: replace with the v0.4.26 frozen Console SHA once Console U is built from D
(stays PENDING until Console U exists)"`. Damit schlägt `exact(candidate['console'])` auf L100 fehl →
`ValueError('immutable source commit required')` (Log-Zeile 303). Die „leere" Stelle ist also nicht `connector`
(der ist ein valides 40-hex-SHA), sondern das **Console-Feld des Connector-Candidates**, das noch nicht eingefroren
ist.

Bestätigung live durch `gh api .../integration-candidate.json?ref=570864c7...`:

```json
{
  "schema": "baseharbor.ecosystem-candidate/v1",
  "release": "0.4.26",
  "core": "2adae47afddc299d06dbc34eb16a8e011c14b6dc",
  "demo": "896dc3cb68a8c9b9585940c4696410de5a338c52",
  "console": "PENDING-FREEZE-DEMO: replace with the v0.4.26 frozen Console SHA once Console U is built from D (stays PENDING until Console U exists)"
}
```

`re.fullmatch(r'[0-9a-f]{40}', connector)` = **True** (`len=40`); `re.fullmatch(..., candidate['console'])` = **False**.
Identisch auf allen live Refs: main-TIP `7c7093d`, `work/v0426-connector-consumption` `570864c7`,
`work/v0.4.26-integrations-docs` `8e6bb897` (demo+console = `PENDING-FREEZE`).

---

## Warum der Source-Job trotz validen Inputs scheitert — Zusammenfassung

Die 40-hex `INPUT_CONSUMER_REF` macht `connector` gültig, aber der Job liest L125
`source_file(REPOSITORIES['console'], candidate['console'], ...)` und `candidate['console']` ist noch der
`PENDING-FREEZE-DEMO`-Platzhalter statt eines gefrorenen 40-hex Console-SHA. Der Console-Ref ist noch nicht aus
Demo U gebaut (Console v0.4.26 nicht releasbar, vgl. o75e/o78). Der fehlende `refs/heads/release/v0.4.26` und die
fehlende `docs/releases/v0.4.26.demo-ref` sind **sekundäre** Release-Metadata-Lücken (Run 224), kein Auslöser von
Run 225 — dort ist der harte Auslöser der `PENDING-FREEZE`-Console-Wert auf L100.

Dies ist dasselbe produktseitige PENDING-FREEZE-Hindernis wie in `f29-w1final.md` (F3): ein korrektes Input-Set
ändert nichts daran (o75e-inputproof:61-69).

---

## (c) Fix-Vorschlag auf Datei/Zeile — OHNE Änderung am Produkt

**Primärer Fix (Run 225, `ValueError('immutable source commit required')` auf L100):**

Datei: `.github/workflows/pre-release.yml` — **Source-Job** (`Run set -euo pipefail`, Zeilen 284–280 des Logs;
`python3 scripts/ecosystem_release_pins.py --tag ... --connector "$INPUT_CONSUMER_REF" --demo "$INPUT_DEMO_REF"`).

Problem: Der Workflow leitet den raw `INPUT_CONSUMER_REF` direkt an `--connector` weiter. Solange der Connector kein
40-hex Console-SHA einfrieren darf (PENDING-FREEZE), ist der Aufruf von vornherein zum Scheitern verurteilt.

Vorschlag (Workflow-YAML, Vor-Prüfung vor dem `ecosystem_release_pins.py`-Call):

1. **Validierung/Abort mit klarer Fehlermeldung** — erkenne `PENDING-FREEZE*` als „nicht releasbar" ab, bevor die
   Matrix losläuft:
   ```bash
   console_sha="$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['console'])" "$RUNNER_TEMP/connector-candidate.json")"
   [[ "$console_sha" =~ ^[0-9a-f]{40}$ ]] || { echo "Connector integration-candidate.json console is not a frozen 40-hex SHA (console='$console_sha'); release not ready (PENDING-FREEZE). Abort." >&2; exit 1; }
   ```
   Das schlägt deterministisch undlesbar fehl, statt erst in Python auf L100 mit einer irreführenden
   `immutable source commit required`-Meldung zu landen.

2. **ODER** Workaround: `--connector` erst auf einen Connector-Commit zeigen lassen, der ein echtes 40-hex Console-SHA
   in `integration-candidate.json` trägt (nach der Console-U-Freeze-Kette). Bis dahin: Job als „blocked/not-ready"
   markieren, **nicht** als Failure durchlaufen.

**Sekundärer Fix (Run 224, fehlende `docs/releases/v0.4.26.demo-ref`) — Workflow-YAML / Release-Metadata:**

- `docs/releases/v0.4.26.demo-ref` (das 40-hex Demo-Ref-Lockfile) muss existieren und `25c319697fd3d89d894943e0f44cd3d5dde8b198`
  enthalten; die Resolver-Zeile im Log (Zeilen 255–280) macht `test -s "$demo_ref_file" || { ... exit 1; }` — das
  Lockfile ist auf develop noch nicht vorhanden.

**Produktcode-Notiz (nur als Befund; NICHT als Änderung gemeint):** In `ecosystem_release_pins.py` L125–100 ist die
Prüfung korrekt (defensive Validierung). Die Meldung L100 (`immutable source commit required`) ist für den
PENDING-FREEZE-Fall missführlich — ein präziserer Fehler (z.B. `console candidate is not a frozen 40-hex SHA`) würde
Diagnose beschleunigen. **Aber**: der eigentliche Blocker ist fachlich (Console noch nicht releasbar), kein
Implementierungsfehler; der Fix gehört in den Workflow-Scope (Vor-Validierung / Blocked-Markierung), nicht in das
Produkt.

---

## repo-rule compliance

- READ-ONLY-Analyse; kein Produkt, kein Dispatch, keine Änderung an Workflow-YAML oder `ecosystem_release_pins.py`.
- Alle Werte (Input-SHAs, Candidate-Inhalt) sind aus den offiziellen GitHub-Actions-Logs / `gh api`; SHAs als
  40-hex verifiziert, Console-Feld als nicht-40-hex verifiziert.
- Keine Secrets/Tokens geloggt (GH_TOKEN im Log ist `***`-maskiert; hier nicht wiedergegeben).

---

## END Q58-SOURCELOG-DONE

(a) Exakte Resolver-Zeile mit angekommenen Inputs: Run 225 (Source-Job `120579336953`, Log L284–286) —
`INPUT_TAG=v0.4.26`, `INPUT_DEMO_REF=25c319697fd3d89d894943e0f44cd3d5dde8b198`,
`INPUT_CONSUMER_REF=570864c7c8bec738438aa70aa967be7e8490d7c3` (alle 40-hex); Run 224 (Source-Job `120578909399`,
Log L284–286) identisch.
(b) Im Skript gelesene Stelle: `ecosystem_release_pins.py` L125 `source_file(REPOSITORIES['console'], candidate['console'], 'integration-candidate.json')` → L100 `if not exact(commit)` → Absturz `ValueError('immutable source commit required')`. „Ultrültig" ist `candidate['console']` = `PENDING-FREEZE-DEMO:...` (kein 40-hex), NICHT der 40-hex `connector`.
(c) Fix-Vorschlag: `.github/workflows/pre-release.yml` Source-Job — Vor-Validierung von `console` auf 40-hex mit klarer
PENDING-FREEZE-Abort-Meldung (oder `--connector` erst nach Console-U-Freeze speisen); sekundär die fehlende
`docs/releases/v0.4.26.demo-ref`-Lockfile auf develop ergänzen. **Ohne Änderung am Produkt umgesetzt.**
