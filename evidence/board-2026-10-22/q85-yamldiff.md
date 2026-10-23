# Q85: YAML-Diff-Analyse — Integration Gates brechen (private_verifier_from_environment → None)

## PLAN [wip:q85]

- [x] 0. Write this evidence file with PLAN + todowrite
- [x] 1. Read-only diff analysis of `baseharbor/.github/workflows`:
  - (a) Compare integration-job call contexts in `pre-release*.yml` (current develop) vs. `static/live-console`
  - (b) Find the commit of the last green 25er-matrix workflow (`git log --follow .github/workflows/pre-release.yml`)
  - (c) Identify the verifier-token variable name and whether the job receives it
  - (d) Build the env-var | source | 25er-Run | 26er-Run | diagnosis table
- [x] 2. END Q85-YAMLDIFF-DONE with the result table

## Constraints

- `git` NUR innerhalb von `baseharbor/`, niemals im Root-Workspace.
- Read-only — KEINE Änderungen an Workflow-Dateien.

## 1. Commit-Historie der Integration-Jobs

**git log --follow .github/workflows/pre-release.yml:**

```
2b8095b9 fix(release): complete v0.4.26 consumer resolution and final evidence dependencies (#1020)
84183b99 feat: v0.4.25 capability contracts and selective lifecycle (#905)
f4cf147a release: prepare qualified v0.4.24 on develop (#878)
```

**Green 25er-Run-Commit:** 84183b99  
**Broken 26er-Run-Commit:** 2b8095b9

## 2. Vergleich der Integration-Job-Aufrufkontexte

### (a) pre-release.yml — Integration Jobs (current develop, 2b8095b9)

Drei Integration-Jobs, die `pre-release-private-integration.py` aufrufen:

| Job | Gate | Zeilen |
|-----|------|--------|
| `integration_remote_target_docker` | `integration/docker/remote-target` | 1366-1411 |
| `integration_remote_target_podman` | `integration/podman/remote-target` | 1412-1456 |
| `integration_live_console` | `integration/static/live-console` | 1458-1502 |

**Job-Envs (alle drei identisch im Auth-Schritt):**

```yaml
- name: Authenticate full pinned private consumer qualification
  env:
    BASEHARBOR_PUBLIC_EVIDENCE_FILE: ${{ format('docs/releases/{0}.consumer-pins.json', needs.source.outputs.tag) }}
    GH_TOKEN: ${{ github.token }}
    CANDIDATE_SHA: ${{ needs.source.outputs.candidate_sha }}
    DEMO_REF: ${{ needs.source.outputs.demo_ref }}
    TAG: ${{ needs.source.outputs.tag }}
    BASEHARBOR_CONSUMER_REF: ${{ needs.source.outputs.consumer_ref }}
  run: >-
    python scripts/pre-release-private-integration.py
    --gate integration/docker/remote-target  # (bzw. podman/live-console)
    --candidate "$CANDIDATE_SHA" --demo "$DEMO_REF" --tag "$TAG"
    --output "$RUNNER_TEMP/integration-docker-remote-target"
```

### (b) Green 25er-Run-Commit (84183b99) — Vergleich

**Consumer-pins-Generation (source job, 25er):**

```yaml
if [ "$tag" = v0.4.24 ] || [ "$tag" = v0.4.25 ]; then
```

**Consumer-pins-Generation (integration jobs, 25er):**

```yaml
if: needs.source.result == 'success' && (needs.source.outputs.tag == 'v0.4.24' || needs.source.outputs.tag == 'v0.4.25')
```

**Job-Envs (25er, identisch zum 26er):**

```yaml
BASEHARBOR_PUBLIC_EVIDENCE_FILE: ${{ format('docs/releases/{0}.consumer-pins.json', needs.source.outputs.tag) }}
```

### (c) Verifier-Token-Variable

**`private_verifier_from_environment()`** (private_consumer_evidence.py:75-101) prüft:

```python
public_path = os.environ.get('BASEHARBOR_PUBLIC_EVIDENCE_FILE')
path = os.environ.get('BASEHARBOR_PRIVATE_EVIDENCE_FILE')
raw = os.environ.get('BASEHARBOR_PRIVATE_EVIDENCE_JSON')
```

**Kein `PRIVATE_VERIFIER_TOKEN` oder `VERIFIER_TOKEN` existiert in den Workflows.**  
Das Token wird NICHT als separate Env-Var übergeben — stattdessen wird `BASEHARBOR_PUBLIC_EVIDENCE_FILE` auf einen Dateipfad gesetzt.

### (d) Ergebnis-Tabelle

| env-Var | Quelle | 25er-Run (84183b99) | 26er-Run (2b8095b9) | Diagnose |
|---------|--------|---------------------|---------------------|----------|
| `BASEHARBOR_PUBLIC_EVIDENCE_FILE` | integration_remote_target_docker/podman, integration_live_console, evidence jobs | `$\{format('docs/releases/{0}.consumer-pins.json', tag)\}` → z.B. `docs/releases/v0.4.25.consumer-pins.json` | `$\{format('docs/releases/{0}.consumer-pins.json', tag)\}` → z.B. `docs/releases/v0.4.26.consumer-pins.json` | **Identisch.** Pfad wird korrekt gesetzt. Aber: Datei muss vom vorherigen Schritt (consumer-pins-Gen) erzeugt worden sein. |
| `BASEHARBOR_PRIVATE_EVIDENCE_FILE` | keine | nicht gesetzt | nicht gesetzt | **Kein Einfluss — beide nicht gesetzt.** |
| `BASEHARBOR_PRIVATE_EVIDENCE_JSON` | keine | nicht gesetzt | nicht gesetzt | **Kein Einfluss — beide nicht gesetzt.** |
| GH_TOKEN | alle integration jobs + source | `${{ github.token }}` | `${{ github.token }}` | **Identisch, korrekt.** |
| BASEHARBOR_PUBLIC_EVIDENCE_FILE consumer-pins-Gen bedingung | source job + integration jobs | `tag == 'v0.4.24' \|\| tag == 'v0.4.25'` → für v0.4.25: TRUE → consumer-pins.json erzeugt | `tag == 'v0.4.24' \|\| tag == 'v0.4.25' \|\| tag == 'v0.4.26'` → für v0.4.26: TRUE → consumer-pins.json erzeugt | **BEIDE erzeugen die Datei.** Kein Unterschied in der Bedingung für den jeweiligen Tag. |

## 3. Diagnose

**KEY FINDING: Die Workflow-Envs und ihre Namen sind IDENTISCH zwischen 25er und 26er.**

Die Integration-Jobs in `pre-release.yml` verwenden dieselben `BASEHARBOR_PUBLIC_EVIDENCE_FILE`-Referenzen. Der einzige YAML-Unterschied zwischen 84183b99 (25er green) und 2b8095b9 (26er broken) ist:

1. **Tag-Bedingung erweitert:** `'v0.4.24' || 'v0.4.25'` → `'v0.4.24' || 'v0.4.25' || 'v0.4.26'`
2. **Job-Names leicht verändert:** "Resolve exact v0.4.24 consumer source bindings" → "Resolve exact immutable consumer source bindings"
3. **Additional needs in evidence job:** `integration_coreless_docker`, `integration_coreless_podman` hinzugefügt

**WARUM `private_verifier_from_environment()` None zurückgibt:**

Die `BASEHARBOR_PUBLIC_EVIDENCE_FILE` wird auf `docs/releases/v0.4.26.consumer-pins.json` gesetzt. Dieser Pfad wird durch den consumer-pins-Generation-Schritt (`ecosystem_release_pins.py`) erstellt. Wenn dieser Schritt fehlschlägt (z.B. weil `integration-candidate.json` im connector-Repo oder `baseharbor-core.ref` im demo-Repo fehlt), wird die consumer-pins.json **nicht** oder **inkorrekt** erstellt.

**Aber:** Die Bedingung `'v0.4.24' || 'v0.4.25' || 'v0.4.26'` ist für v0.4.26 TRUE, also wird der Schritt ausgeführt. Das Problem liegt NICHT im YAML, sondern im **Inhalt** der consumer-pins.json oder im Fehlen der Abhängigkeiten im connector/demo-Repo.

**Root Cause ist in q84 dokumentiert:** `verifier is None` weil `BASEHARBOR_PUBLIC_EVIDENCE_FILE` auf eine nicht-existente oder inkorrekte consumer-pins.json verweist, ODER `self.pins.get(key)` returns None weil der Gate-Key nicht im pins-file gefunden wird.
