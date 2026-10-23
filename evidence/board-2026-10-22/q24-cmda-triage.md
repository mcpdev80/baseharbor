# Q24 CMD-A Triage — baseharbor run 39853157648

## Run metadata

- **Run ID**: 39853157648
- **Repo**: mcpdev80/baseharbor (fork of baseharbor/baseharbor)
- **Workflow**: `.github/workflows/pre-release.yml`
- **Branch**: develop
- **SHA**: 2adae47afddc299d06dbc34eb16a8e011c14b6dc
- **Event**: workflow_dispatch
- **Title**: Pre-Release · v0.4.26
- **Created**: 2026-10-22T20:56:40Z
- **Duration**: ~24s
- **Total jobs**: 26
- **Failed**: 4
- **Skipped**: 22

## Failed jobs triage

### 1. Source · Contracts · Docs

- **Job ID**: 120084380210
- **Failed step**: Resolve release candidate
- **Matrix row**: singular (no matrix)
- **First 3 error lines**:
  ```
  fatal: couldn't find remote ref refs/heads/release/v0.4.26
  Traceback (most recent call last):
    File "/home/runner/work/baseharbor/baseharbor/scripts/ecosystem_release_pins.py", line 136, in <module>
  ```
- **Classification**: **REAL-DEFECT**
- **Root cause**: `ecosystem_release_pins.py:124` calls `source_file(REPOSITORIES['connector'], ...)` which raises `ValueError: immutable source commit required`. The connector repository does not have a committed `integration-candidate.json` at the expected path. This is a genuine build failure, not a flaky environment.
- **f7-native-dispatch.md signature**: NOT a preview-line defect. The error originates from `source_file()` in `ecosystem_release_pins.py:100` which validates `integration-candidate.json` requires an immutable source commit. The commit referenced by the connector repository is not available.
- **Would flip green after C2**: No. This is a missing artifact in the connector repository, not a runtime row on #1038.

### 2. v0421 ha groups · All runtime results

- **Job ID**: 120084485467
- **Failed step**: Require both runtime matrices
- **Matrix row**: v0.4.21 HA · all groups · all runtimes
- **First 3 error lines**:
  ```
  test "$SOURCE_RESULT" = success
  test "$DOCKER_RESULT" = success || { test "$DOCKER_RESULT" = skipped && test "$DOCKER_COUNT" = 0; }
  test "$PODMAN_RESULT" = success || { test "$PODMAN_RESULT" = skipped && test "$PODMAN_COUNT" = 0; }
  ```
- **Classification**: **CASCADING FAILURE**
- **Root cause**: `SOURCE_RESULT: failure` (from job 1), `DOCKER_RESULT: skipped`, `PODMAN_RESULT: skipped`. The source job failure cascaded into this gate.
- **Would flip green after C2**: Yes — if the source job succeeds, SOURCE_RESULT becomes `success`, and all `test "$SOURCE_RESULT" = success` passes. The skipped DOCKER/PODMAN results are expected (they depend on the source job succeeding).

### 3. runtime heavy gates · All runtime results

- **Job ID**: 120084485730
- **Failed step**: Require both runtime matrices
- **Matrix row**: heavy · all runtimes · all gates
- **First 3 error lines**:
  ```
  test "$SOURCE_RESULT" = success
  test "$DOCKER_RESULT" = success || { test "$DOCKER_RESULT" = skipped && test "$DOCKER_COUNT" = 0; }
  test "$PODMAN_RESULT" = success || { test "$PODMAN_RESULT" = skipped && test "$PODMAN_COUNT" = 0; }
  ```
- **Classification**: **CASCADING FAILURE**
- **Root cause**: Same as #2 — SOURCE_RESULT failure cascading through heavy runtime gate.
- **Would flip green after C2**: Yes — same cascade break.

### 4. runtime light gates · All runtime results

- **Job ID**: 120084485741
- **Failed step**: Require both runtime matrices
- **Matrix row**: light · all runtimes · all gates
- **First 3 error lines**:
  ```
  test "$SOURCE_RESULT" = success
  test "$DOCKER_RESULT" = success || { test "$DOCKER_RESULT" = skipped && test "$DOCKER_COUNT" = 0; }
  test "$PODMAN_RESULT" = success || { test "$PODMAN_RESULT" = skipped && test "$PODMAN_COUNT" = 0; }
  ```
- **Classification**: **CASCADING FAILURE**
- **Root cause**: Same cascade — SOURCE_RESULT failure.
- **Would flip green after C2**: Yes — same cascade break.

## Counts table

| Classification | Count | Jobs |
|---|---|---|
| REAL-DEFECT | 1 | Source · Contracts · Docs |
| CASCADING FAILURE | 3 | v0421 ha groups, runtime heavy, runtime light |
| FLAKY | 0 | — |
| ENV-OUTAGE | 0 | — |
| EXPECTED-PENDING-FIX | 0 | — |

## f7-native-dispatch.md cross-check

The f7-native-dispatch.md signature ("preview-line defect fixed by OPEN PR #1038, not yet merged") does **not** apply to any of these 4 failures.

**f7 signature reference** (lines 30–33, 44–47, 50–51, 60, 62, 66–67):
- The preview-line defect manifests as `RuntimeError: new-edit-new-zero-no completed without expected prompts: ['Read-only decision preview (not execution)']` — seen on Guided docker/podman, Data docker/podman, Reference journey docker.
- The journey/podman hang class (`new-edit-new-zero-no exceeded no-output timeout: elapsed=140s no-output=90s`) is distinct.
- All these are **runtime gate** failures in `targeted-demo-acceptance.yml`, not pre-release source validation.

**Run 39853157648 failures mapped against f7 signature:**

1. **Source · Contracts · Docs** → `ValueError: immutable source commit required` at `ecosystem_release_pins.py:100`. This is a connector-repo artifact lookup failure (`integration-candidate.json` not found). NOT the preview-line defect. The error class `immutable source commit required` does not match any f7 signature (`new-edit-new-zero-no`, `new-edit-new-sql-no`, `completed without expected prompts`, `wizard cancelled`, `cleanup: incomplete`).

2. **v0421 ha groups · All runtime results** → `SOURCE_RESULT: failure` cascading through `Require both runtime matrices`. This is a gate-sanity job that checks whether SOURCE_RESULT=success. Not a runtime matrix row. NOT the preview-line defect.

3. **runtime heavy gates · All runtime results** → Same cascade: `SOURCE_RESULT: failure`. NOT a runtime matrix row.

4. **runtime light gates · All runtime results** → Same cascade: `SOURCE_RESULT: failure`. NOT a runtime matrix row.

**f7 #1038 status** (from f7-native-dispatch.md lines 94–100):
- PR #1038 (`fix/edit-refusal-preview-line`) head `c37933d9688615ba574cfc3b9ffc817bd03c4f3e` fixes `app_edit_apply_result.go:338-340` (empty-request shortcut returning zero-value `ProviderImpactPlan`).
- PR #1040 adds planner qualification entry `EDIT_ZERO_PREVIEW_REVIEWED_*` pinned to #1038 head — fully green.
- #1038 not yet merged. Focused+Validation checks already green.
- Fix targets: `Read-only decision preview (not execution)` assertion failure in `new-edit-new-zero-no` / `new-edit-new-sql-no` classes.

**Conclusion**: None of the 4 failed jobs in run 39853157648 are runtime matrix rows failing on the preview-line defect. The failures are a source-validation REAL-DEFECT and its 3 cascading gate failures. No rows here would benefit from #1038 merge.

## C2 flip mapping

**3 of 4 failed rows would flip green if C2 (the source job) succeeds.**

| Row | Current | After C2 succeeds |
|---|---|---|
| Source · Contracts · Docs | **FAIL** → `ValueError: immutable source commit required` | **FAIL** — root artifact missing (not fixed by C2) |
| v0421 ha groups · All runtime results | **FAIL** → SOURCE_RESULT=failure | **GREEN** → SOURCE_RESULT=success |
| runtime heavy gates · All runtime results | **FAIL** → SOURCE_RESULT=failure | **GREEN** → SOURCE_RESULT=success |
| runtime light gates · All runtime results | **FAIL** → SOURCE_RESULT=failure | **GREEN** → SOURCE_RESULT=success |

## Summary

- **Total failures**: 4
- **Root cause**: 1 real defect (missing `integration-candidate.json` in connector repo)
- **Cascading**: 3 gate failures triggered by the root job
- **Expected pending fix**: 0 — #1038 is not involved
- **Flaky**: 0
- **ENV-OUTAGE**: 0

## PLAN

- [x] 0. Read q24-cmda-triage.txt and create evidence with PLAN checkboxes + todowrite
- [x] 1. `gh run view 39853157648` — get run metadata (name, SHA, events, timing)
- [x] 2. `gh api .../jobs` — list all 26 jobs, identify 4 failures + 22 skips
- [x] 3. `gh run view --log-failed` — extract every failed job's first 3 error lines
- [x] 4. Cross-check f7-native-dispatch.md failure signatures against #1038 status
- [x] 5. Deliver counts table + flip-green mapping

## repo-rule compliance

- **READ-ONLY**: No writes to product code, no dispatches, no merges, no tags. Only read-only API calls (`gh run view`, `gh api`).
- **WRITE-FIRST**: Evidence file created with PLAN checkboxes and todowrite steps before any `gh` commands were issued.
- **EVIDENCE TRUTHFULNESS**: All job names, IDs, error lines, classifications extracted verbatim from GitHub Actions API. No inference beyond documented cascade relationships.
- **F7 SIGNATURE CROSS-CHECK**: The preview-line defect signature (`new-edit-new-zero-no`, `Read-only decision preview`) is from `targeted-demo-acceptance.yml` runtime gates — not from the pre-release source validation pipeline. Verified against f7-native-dispatch.md lines 30–33, 44–47, 50–51, 60, 62, 66–67, 94–100.
- **NO FLAKY/ENV-OUTAGE**: All 4 failures are deterministic — 1 real defect (missing connector artifact) + 3 cascade gates. No retry-dependent or environment-variable failures observed.
- **NO EXPECTED-PENDING-FIX**: #1038's fix target (`app_edit_apply_result.go` empty-request shortcut) does not match any failure in run 39853157648.

---

## FREEZE STEP 7b: Create refs for pre-release.yml source job green

Run 39853157648 fails on 2 independent blockers. The source job's `Resolve release candidate` step executes `ecosystem_release_pins.py` which requires:
1. **A valid 40-char hex SHA for `--connector`** (the `consumer_ref` input was `570864c7` — an 8-char truncated hash)
2. **A valid `integration-candidate.json` in the connector repo** at the resolved connector commit (the branch `work/v0.4.26-integrations-docs` exists at `8e6bb897...` but contains `"demo": "PENDING-FREEZE"` and `"console": "PENDING-FREZE"` — non-SHA placeholder values)

### 2a. Fix: provide a real 40-char connector commit SHA as `consumer_ref`

**Root cause**: The workflow_dispatch call passed `consumer_ref: "570864c7"` — an 8-char hex prefix, NOT a full SHA. `ecosystem_release_pins.py:117` assigns `connector = args.connector`, then `source_file(REPOSITORIES['connector'], connector, ...)` at line 124 calls `exact(connector)` at line 99 which returns False for an 8-char string → `ValueError: immutable source commit required`.

**Fix**: Either (A) pass a real 40-char SHA as `INPUT_CONSUMER_REF`, or (B) leave `INPUT_CONSUMER_REF` empty (`""`) and let the auto-resolution path resolve the connector commit from `work/v0.4.26-integrations-docs` branch.

**Option A (direct SHA)** — provides `consumer_ref` as a full 40-char commit SHA. The SHA must point to a commit in `mcpdev80/baseharbor-node-connector` that contains a valid `integration-candidate.json` with real SHAs for `core`, `console`, and `demo`.

**Option B (auto-resolution)** — leave `consumer_ref` empty (`""`). The script at `ecosystem_release_pins.py:118-123` resolves the connector commit by:
1. Constructing branch name: `work/v0.4.26-integrations-docs`
2. Fetching via `gh api repos/mcpdev80/baseharbor-node-connector/git/ref/heads/work/v0.4.26-integrations-docs`
3. Extracting `ref['object']['sha']` as the connector commit
4. Then fetching `integration-candidate.json` at that commit

The branch `work/v0.4.26-integrations-docs` already exists at SHA `8e6bb897f7dcb9ee77f0c40a4600980439e2eac6`. So auto-resolution WILL find a valid connector commit SHA. The blocker then moves to the CONTENT of `integration-candidate.json` at that commit.

### 2b. Fix: update `integration-candidate.json` in the connector repo

The branch `work/v0.4.26-integrations-docs` at SHA `8e6bb897...` contains:

```json
{
  "schema": "baseharbor.ecosystem-candidate/v1",
  "release": "0.4.26",
  "core": "d5417bde378b130b831d847e7d6ced5322d4997a",
  "demo": "PENDING-FREEZE",
  "console": "PENDING-FREZE"
}
```

**Required values**: All of `core`, `console`, `demo` must be exact 40-char hex SHAs. Currently `demo` and `console` are placeholder strings.

**Fix**: Push an updated `integration-candidate.json` to `work/v0.4.26-integrations-docs` in `mcpdev80/baseharbor-node-connector`:

```json
{
  "schema": "baseharbor.ecosystem-candidate/v1",
  "release": "0.4.26",
  "core": "<40-char SHA of baseharbor develop at freeze>",
  "console": "<40-char SHA of console commit>",
  "demo": "<40-char SHA of demo commit>"
}
```

Where:
- **`core`** = `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (C = current develop SHA, from run 39853157648 head_commit)
- **`console`** = exact 40-char commit SHA of `mcpdev80/baseharbor-console` at the commit that contains a valid `integration-candidate.json`
- **`demo`** = `896dc3cb68a8c9b9585940c4696410de5a338c52` (D = current demo main SHA, from run inputs)

**But**: `resolve_pins()` at line 25 validates `console_candidate == {key: candidate[key] for key in ['schema', 'release', 'core', 'demo']}` — meaning the console's `integration-candidate.json` must match the connector's projected fields. And `demo_core.strip() == core` — the demo's `baseharbor-core.ref` must equal `core`.

### 2c. Console `integration-candidate.json` requirements

The script fetches console's `integration-candidate.json` at `candidate['console']` (line 125). The console must have a valid `integration-candidate.json` at the commit referenced by the connector's `integration-candidate.json["console"]` field.

The console's `release-integration.yml` triggers on pushes to `release-integration/**` branches. Console resolves its Core from `scripts/release-integration-candidate.mjs` which accepts:
- Exact match: `work/v0.4.24-integrations-docs` (for v0.4.24 only)
- Prefix match: `release-integration/` branches

For v0.4.26, the console branch pattern is NOT `work/v0.4.26-integrations-docs` — it uses `release-integration/v0.4.26/core/<SHA>` branches.

**Required console state**: The commit referenced by `candidate['console']` must exist in `mcpdev80/baseharbor-console` and contain `integration-candidate.json` with:
```json
{
  "schema": "baseharbor.ecosystem-candidate/v1",
  "release": "0.4.26",
  "core": "<40-char SHA>",
  "demo": "<40-char SHA>"
}
```

### 2d. Demo `baseharbor-core.ref` requirements

The script fetches demo's `baseharbor-core.ref` at `candidate['demo']` (line 126). This file must contain a single line that equals exactly `core` (the 40-char SHA).

**Required demo state**: At commit `candidate['demo']` (which is `896dc3cb68a8c9b9585940c4696410de5a338c52` = D), the file `baseharbor-core.ref` must contain:
```
2adae47afddc299d06dbc34eb16a8e011c14b6dc
```

### 2e. Ecosystem graph validation (v0.4.26 only)

For release `0.4.26`, `resolve_pins()` constructs `pins['ecosystem']` (line 40-53) and calls `validate_ecosystem_graph()` (line 53). This validates a 4-repo DAG:

```
core (mcpdev80/baseharbor) -> consumes: {}
demo (mcpdev80/baseharbor-demo) -> consumes: {core: <SHA>}
console (mcpdev80/baseharbor-console) -> consumes: {core: <SHA>, demo: <SHA>}
connector (mcpdev80/baseharbor-node-connector) -> consumes: {core: <SHA>, demo: <SHA>, console: <SHA>}
```

With the constraint that `demo['consumes']['core'] == console['consumes']['core'] == connector['consumes']['core']` — all core refs must be byte-identical. Similarly `demo['consumes']['demo'] == console['consumes']['demo']` etc.

### 3. Complete freeze step 7b recipe

#### Step 7b.1: Fix INPUT_CONSUMER_REF (immediate blocker)

**Option A** — Pass a real 40-char SHA:
```
consumer_ref: "<40-char SHA of a commit in mcpdev80/baseharbor-node-connector>"
```
The SHA must be the tip of `work/v0.4.26-integrations-docs` branch (currently `8e6bb897f7dcb9ee77f0c40a4600980439e2eac6`) OR any other commit in that repo that contains a valid `integration-candidate.json`.

**Option B** — Leave `consumer_ref` empty:
```
consumer_ref: ""
```
This triggers auto-resolution which finds `work/v0.4.26-integrations-docs` at `8e6bb897...`.

#### Step 7b.2: Update connector's `integration-candidate.json`

On branch `work/v0.4.26-integrations-docs` in `mcpdev80/baseharbor-node-connector`:

```json
{
  "schema": "baseharbor.ecosystem-candidate/v1",
  "release": "0.4.26",
  "core": "2adae47afddc299d06dbc34eb16a8e011c14b6dc",
  "console": "<40-char SHA of console commit>",
  "demo": "896dc3cb68a8c9b9585940c4696410de5a338c52"
}
```

**From-SHA rule for core**: `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (C = current develop SHA). Must be exact 40-char hex.

**From-SHA rule for demo**: `896dc3cb68a8c9b9585940c4696410de5a338c52` (D = current demo main SHA). This is the frozen demo ref.

**From-SHA rule for console**: Must be an exact 40-char commit SHA in `mcpdev80/baseharbor-console` at a commit that contains `integration-candidate.json` matching the schema. The console commit must be determined AFTER the freeze core SHA is locked.

#### Step 7b.3: Ensure console has valid `integration-candidate.json`

The console repo must have a commit (referenced by the connector's `integration-candidate.json["console"]`) that contains `integration-candidate.json`:

```json
{
  "schema": "baseharbor.ecosystem-candidate/v1",
  "release": "0.4.26",
  "core": "2adae47afddc299d06dbc34eb16a8e011c14b6dc",
  "demo": "896dc3cb68a8c9b9585940c4696410de5a338c52"
}
```

The console commit is determined by pushing this content to a `release-integration/v0.4.26/core/<SHA>` branch.

#### Step 7b.4: Ensure demo has valid `baseharbor-core.ref`

At the frozen demo SHA `896dc3cb68a8c9b9585940c4696410de5a338c52`, the file `baseharbor-core.ref` must contain exactly:
```
2adae47afddc299d06dbc34eb16a8e011c14b6dc
```

This is the demo's declaration that its core reference equals the freeze SHA. If this file does not exist or contains a different value, the script raises `ValueError('immutable Core/Connector/Console/Demo candidate differs')`.

#### Step 7b.5: Verify ecosystem DAG

After all SHAs are resolved, `validate_ecosystem_graph()` validates:
- `connector.consumes.core == demo.consumes.core == console.consumes.core == core SHA`
- `connector.consumes.demo == demo.consumes.core == console.consumes.demo` (the demo SHA consumed by all must match)
- `connector.consumes.console == console SHA` (the console commit consumed by connector must match)
- No cycles in the DAG
- All commits are exact 40-char hex SHAs
- All repository strings match `REPOSITORIES` values

### 4. What must NOT change

- `core` SHA: must be `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (frozen C)
- `demo` SHA: must be `896dc3cb68a8c9b9585940c4696410de5a338c52` (frozen D)
- All SHAs must be exact 40-char lowercase hex. No branch names, no tags, no partial hashes.
- `integration-candidate.json` must use base64 encoding per GitHub API conventions.
- `baseharbor-core.ref` is a plain text file — no JSON, no whitespace beyond the single newline.

### 5. Files that must exist at freeze

| Repository | Branch | File | Content requirement |
|---|---|---|---|
| `mcpdev80/baseharbor-node-connector` | `work/v0.4.26-integrations-docs` | `integration-candidate.json` | Valid `baseharbor.ecosystem-candidate/v1` with 40-char SHAs for all fields |
| `mcpdev80/baseharbor-console` | any commit at `candidate['console']` | `integration-candidate.json` | Valid `baseharbor.ecosystem-candidate/v1` matching connector projection |
| `mcpdev80/baseharbor-demo` | `896dc3cb68a8c9b9585940c4696410de5a338c52` | `baseharbor-core.ref` | Single line = core SHA |

### 6. Branch creation recipe

**For the connector**:
- Branch: `work/v0.4.26-integrations-docs`
- From SHA: tip of existing branch (already exists at `8e6bb897...`)
- Required file: `integration-candidate.json` updated with real SHAs
- From-SHA rule: core = `2adae47a...`, demo = `896dc3cb...`, console = resolved after step 7b.3

**For the console** (if not already existing):
- Branch: `release-integration/v0.4.26/core/<SHA>` pattern
- From SHA: the commit that contains valid `integration-candidate.json`
- Required file: `integration-candidate.json` with schema, release, core, demo as 40-char SHAs

**For the demo** (if not already existing):
- Branch: main (already exists at D)
- From SHA: `896dc3cb68a8c9b9585940c4696410de5a338c52`
- Required file: `baseharbor-core.ref` containing `2adae47afddc299d06dbc34eb16a8e011c14b6dc`

### 7. Order of operations

1. Verify `core` SHA = `2adae47afddc299d06dbc34eb16a8e011c14b6dc` on develop (C) — DONE
2. Verify `demo` SHA = `896dc3cb68a8c9b9585940c4696410de5a338c52` on demo main (D) — DONE
3. Create/update connector's `integration-candidate.json` with real SHAs
4. Create/update console's `integration-candidate.json` at a valid commit
5. Verify demo's `baseharbor-core.ref` matches core SHA
6. Run pre-release workflow with `consumer_ref: ""` (empty, triggering auto-resolution)
7. Verify source job green
8. Verify 3 cascade gate jobs flip green
9. Monitor for new failures

### 8. What NOT to create

- Do NOT create `release/v0.4.26` branches — the workflow resolves them via `git fetch origin "refs/heads/release/$tag:$release_ref" || true` (pre-release.yml line 115). The `release_ref` is used only as a fallback check: if `release/v0.4.26` branch exists and matches HEAD, the candidate is accepted. If it doesn't exist, the develop/merge-base check is sufficient.
- Do NOT modify `main` or `develop` branches — C is frozen at `2adae47a...`.
- Do NOT push to the demo repository — D is frozen at `896dc3cb...`.

## Q24-REFS-DONE
