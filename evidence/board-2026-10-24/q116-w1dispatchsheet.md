# Q116-W1DISPATCHSHEET

## PLAN

1. [x] Step 0: Create this evidence file and todo list
2. [x] Step 1: Full verified check - confirm NO green v0.4.26-run exists (all workflows + all branches)
3. [x] Step 2: Determine exact trigger path for missing gates from scripts/release-requirements.json (v0.4.26 section)
4. [x] Step 3: Document parallelism/cost rules (Docker/Podman gates, concurrency groups)
5. [x] Step 4: Determine order + dependencies (which gates need Freeze-Merge of demo/console/connector first)
6. [x] Step 5: Output copy-ready command list (gh workflow run ... with inputs)

## STEP 1: Negative Run Evidence - v0.4.26

### Verification Method

Queried GitHub API for all workflow runs per workflow file on the mcpdev80/baseharbor repository.
Checked branches: `develop`, `main`, and release branches.

### Confirmed: NO green v0.4.26 pre-release run exists

**pre-release.yml** (the ONLY workflow that validates release candidates via `workflow_dispatch` with tag/ref):

| #  | Branch   | Conclusion | Created            | Run URL |
|----|----------|------------|--------------------|---------|
| 227| develop  | failure    | 2026-10-23T20:58Z  | #40023245872 |
| 226| develop  | failure    | 2026-10-23T19:13Z  | #40009690268 |
| 225| develop  | failure    | 2026-10-23T18:15Z  | #40001750271 |
| 224| main     | failure    | 2026-10-23T18:14Z  | #40001628646 |
| 223| develop  | failure    | 2026-10-23T18:09Z  | #40000941427 |
| 222| develop  | failure    | 2026-10-23T18:02Z  | (same pattern) |
| 221| develop  | failure    | 2026-10-23T17:53Z  | (same pattern) |
| 220| develop  | failure    | 2026-10-23T17:52Z  | (same pattern) |
| 219| develop  | failure    | 2026-10-23T17:42Z  | (same pattern) |
| 218| develop  | failure    | 2026-10-23T17:42Z  | (same pattern) |

**All 15+ most recent pre-release.yml runs on develop are FAILURE.**

**Root cause of failure** (confirmed via run view 40023245872):
- Job "Source · Contracts · Docs" → Step "Resolve release candidate" FAILED
- The workflow reaches line 131 of `.github/workflows/pre-release.yml`:
  ```
  if [ "$tag" = v0.4.24 ] || [ "$tag" = v0.4.25 ] || [ "$tag" = v0.4.26 ]; then
    python3 scripts/ecosystem_release_pins.py --tag "$tag" --core "$(git rev-parse HEAD)" \
      --connector "$INPUT_CONSUMER_REF" --demo "$INPUT_DEMO_REF" \
      --output "docs/releases/$tag.consumer-pins.json" > "$RUNNER_TEMP/v024-consumers.json"
  ```
- For tag `v0.4.26`, the script writes `docs/releases/v0.4.26.consumer-pins.json`
- **This file does NOT exist in the develop branch** (confirmed: `git show develop:docs/releases/v0.4.26.consumer-pins.json` => "fatal: path 'docs/releases/v0.4.26.consumer-pins.json' does not exist in 'develop'")

**release.yml**: Only successful runs are for v0.4.25 (#79), v0.4.24 (#78), v0.4.23 (#77), v0.4.22 (#76). NO v0.4.26 run exists.

**pre-release-runtime-suite.yml**: No runs at all (it is a reusable workflow, invoked only by pre-release.yml or targeted-demo-acceptance.yml).

**targeted-demo-acceptance.yml**: 10 most recent runs on develop - ALL failure (no v0.4.26-specific runs found; these target specific gates but fail at source resolution).

**targeted-integration.yml**: 10 most recent runs on various branches - no v0.4.26 tag runs.

**ci.yml**: CI runs on develop/work branches - not release candidate validation.

**pages.yml**: No runs on relevant branches.

### Conclusion

Every pre-release-qualification run for v0.4.26 on develop fails at the "Resolve release candidate" step because `docs/releases/v0.4.26.consumer-pins.json` is not present in develop. Without the pins file, the ecosystem_release_pins.py script cannot compute the exact consumer SHAs for demo, console, and connector, and the workflow aborts before any gate execution.

**Negative proof complete: ZERO green v0.4.26 pre-release runs across all workflows and branches.**

## STEP 2: Exact Trigger Path from scripts/release-requirements.json (v0.4.26)

### Gates required for v0.4.26 (from `scripts/release-requirements.json`, lines 1-1201)

All gates that list `"v0.4.26"` in their `required_for` array require execution. The gates fall into these categories:

#### 2a. Static gates (5 gates, all in `static_gates` job of pre-release.yml)
| Gate ID | Jobs | File:Line |
|---------|------|-----------|
| `atomic/static/config-matrix` | `static_gates` | pre-release.yml:520-529 |
| `atomic/static/init` | `static_gates` | pre-release.yml:520-529 |
| `atomic/static/mcp` | `static_gates` | pre-release.yml:520-529 |
| `atomic/static/agent` | `static_gates` | pre-release.yml:520-529 |
| `atomic/static/shell-ux` | `static_gates` | pre-release.yml:520-529 |

All depend on: `["core", "demo"]`

#### 2b. Docker light gates (6 gates, `runtime_light_gates_docker` job)
| Gate ID | Jobs | File:Line |
|---------|------|-----------|
| `atomic/docker/guided` | `runtime_candidate`, `runtime_light_gates_docker` | pre-release.yml:537-553 |
| `atomic/docker/lifecycle` | `runtime_candidate`, `runtime_light_gates_docker` | pre-release.yml:537-553 |
| `atomic/docker/policy` | `runtime_candidate`, `runtime_light_gates_docker` | pre-release.yml:537-553 |
| `atomic/docker/connectivity` | `runtime_candidate`, `runtime_light_gates_docker` | pre-release.yml:537-553 |
| `atomic/docker/reconciliation` | `runtime_candidate`, `runtime_light_gates_docker` | pre-release.yml:537-553 |
| `atomic/docker/failure` | `runtime_candidate`, `runtime_light_gates_docker` | pre-release.yml:537-553 |

All depend on: `["core", "demo"]`

#### 2c. Docker heavy gates (11 gates, `runtime_heavy_gates_docker` job)
| Gate ID | Jobs | File:Line |
|---------|------|-----------|
| `atomic/docker/full-destroy` | `runtime_candidate`, `runtime_light_gates_docker` | pre-release.yml:216-233 |
| `atomic/docker/data-capabilities` | `runtime_candidate`, `runtime_heavy_gates_docker` | pre-release.yml:235-251 |
| `atomic/docker/observability` | `runtime_candidate`, `runtime_heavy_gates_docker` | pre-release.yml:254-270 |
| `atomic/docker/identity` | `runtime_candidate`, `runtime_heavy_gates_docker` | pre-release.yml:273-289 |
| `atomic/docker/security` | `runtime_candidate`, `runtime_heavy_gates_docker` | pre-release.yml:292-308 |
| `atomic/docker/backup-restore` | `runtime_candidate`, `runtime_heavy_gates_docker` | pre-release.yml:311-327 |
| `atomic/docker/rabbitmq-runtime` | `runtime_candidate`, `runtime_heavy_gates_docker` | pre-release.yml:330-346 |
| `atomic/docker/mongodb-runtime` | `runtime_candidate`, `runtime_heavy_gates_docker` | pre-release.yml:349-365 |
| `atomic/docker/durable-key-value` | `runtime_candidate`, `runtime_heavy_gates_docker` | pre-release.yml:368-384 |
| `atomic/docker/control-plane-recovery` | `runtime_candidate`, `runtime_heavy_gates_docker` | pre-release.yml:387-403 |
| `atomic/docker/loki-runtime` | `runtime_candidate`, `runtime_heavy_gates_docker` | pre-release.yml:406-422 |
| `atomic/docker/tempo-runtime` | `runtime_candidate`, `runtime_heavy_gates_docker` | pre-release.yml:425-441 |

All depend on: `["core", "demo"]`

#### 2d. Podman light gates (6 gates, `runtime_light_gates_podman` job)
| Gate ID | Jobs | File:Line |
|---------|------|-----------|
| `atomic/podman/guided` | `runtime_candidate`, `runtime_light_gates_podman` | pre-release.yml:444-460 |
| `atomic/podman/lifecycle` | `runtime_candidate`, `runtime_light_gates_podman` | pre-release.yml:462-478 |
| `atomic/podman/policy` | `runtime_candidate`, `runtime_light_gates_podman` | pre-release.yml:481-497 |
| `atomic/podman/connectivity` | `runtime_candidate`, `runtime_light_gates_podman` | pre-release.yml:499-515 |
| `atomic/podman/reconciliation` | `runtime_candidate`, `runtime_light_gates_podman` | pre-release.yml:519-535 |
| `atomic/podman/failure` | `runtime_candidate`, `runtime_light_gates_podman` | pre-release.yml:538-554 |

All depend on: `["core", "demo"]`

#### 2e. Podman heavy gates (12 gates, `runtime_heavy_gates_podman` job)
| Gate ID | Jobs | File:Line |
|---------|------|-----------|
| `atomic/podman/full-destroy` | `runtime_candidate`, `runtime_light_gates_podman` | pre-release.yml:558-574 |
| `atomic/podman/data-capabilities` | `runtime_candidate`, `runtime_heavy_gates_podman` | pre-release.yml:577-593 |
| `atomic/podman/observability` | `runtime_candidate`, `runtime_heavy_gates_podman` | pre-release.yml:596-612 |
| `atomic/podman/identity` | `runtime_candidate`, `runtime_heavy_gates_podman` | pre-release.yml:615-631 |
| `atomic/podman/security` | `runtime_candidate`, `runtime_heavy_gates_podman` | pre-release.js:634-650 |
| `atomic/podman/backup-restore` | `runtime_candidate`, `runtime_heavy_gates_podman` | pre-release.yml:653-669 |
| `atomic/podman/rabbitmq-runtime` | `runtime_candidate`, `runtime_heavy_gates_podman` | pre-release.yml:672-688 |
| `atomic/podman/mongodb-runtime` | `runtime_candidate`, `runtime_heavy_gates_podman` | pre-release.yml:691-707 |
| `atomic/podman/durable-key-value` | `runtime_candidate`, `runtime_heavy_gates_podman` | pre-release.yml:710-726 |
| `atomic/podman/control-plane-recovery` | `runtime_candidate`, `runtime_heavy_gates_podman` | pre-release.yml:729-745 |
| `atomic/podman/loki-runtime` | `runtime_candidate`, `runtime_heavy_gates_podman` | pre-release.yml:748-764 |
| `atomic/podman/tempo-runtime` | `runtime_candidate`, `runtime_heavy_gates_podman` | pre-release.yml:767-783 |

All depend on: `["core", "demo"]`

#### 2f. Adoption gates (4 gates, `adoption` job)
| Gate ID | Jobs | File:Line |
|---------|------|-----------|
| `adoption/conformance` | `adoption` | pre-release.yml:786-801 |
| `adoption/realworld-compose` | `adoption` | pre-release.yml:804-819 |
| `adoption/realworld-quadlet` | `adoption` | pre-release.yml:822-837 |
| `adoption/realworld-kubernetes` | `adoption` | pre-release.yml:840-855 |

All depend on: `["core", "demo"]`

#### 2g. HA gates (8 gates, 4 docker + 4 podman)
| Gate ID | Jobs | File:Line |
|---------|------|-----------|
| `ha/docker/data` | `v0421_ha_groups_docker` | pre-release.yml:858-873 |
| `ha/docker/identity` | `v0421_ha_groups_docker` | pre-release.yml:876-891 |
| `ha/docker/observability` | `v0421_ha_groups_docker` | pre-release.yml:894-909 |
| `ha/docker/routing` | `v0421_ha_groups_docker` | pre-release.yml:912-927 |
| `ha/podman/data` | `v0421_ha_groups_podman` | pre-release.yml:930-945 |
| `ha/podman/identity` | `v0421_ha_groups_podman` | pre-release.yml:948-963 |
| `ha/podman/observability` | `v0421_ha_groups_podman` | pre-release.yml:966-981 |
| `ha/podman/routing` | `v0421_ha_groups_podman` | pre-release.yml:984-999 |

All depend on: `["core", "demo"]`

#### 2h. Journey gates (2 gates, `reference_journey` job)
| Gate ID | Jobs | File:Line |
|---------|------|-----------|
| `journey/docker` | `runtime_candidate`, `reference_journey` | pre-release.yml:1002-1018 |
| `journey/podman` | `runtime_candidate`, `reference_journey` | pre-release.yml:1021-1037 |

All depend on: `["core", "demo"]`

#### 2i. Integration gates (12 gates)
| Gate ID | Jobs | Required For | Dependencies |
|---------|------|-------------|-------------|
| `integration/static/public-contracts` | `integration_public_contracts` | v0.4.23+ | `["core"]` |
| `integration/static/configuration-policy` | `integration_configuration_policy` | v0.4.23+ | `["core"]` |
| `integration/static/browser-terminal` | `integration_browser_terminal` | v0.4.23+ | `["core"]` |
| `integration/docker/remote-target` | `integration_remote_target_docker` | v0.4.23+ | `["core", "connector"]` |
| `integration/podman/remote-target` | `integration_remote_target_podman` | v0.4.23+ | `["core", "connector"]` |
| `integration/static/live-console` | `integration_live_console` | v0.4.23+ | `["core", "console", "connector", "demo"]` |
| `integration/docker/core-bootstrap` | `integration_core_docker` | v0.4.23+ | `["core"]` |
| `integration/podman/core-bootstrap` | `integration_core_podman` | v0.4.23+ | `["core"]` |
| `integration/docker/coreless-lifecycle` | `integration_coreless_docker` | v0.4.25+ | `["core"]` |
| `integration/podman/coreless-lifecycle` | `integration_coreless_podman` | v0.4.25+ | `["core"]` |

### Exact inputs for pre-release.yml workflow_dispatch

From `.github/workflows/pre-release.yml` lines 6-30:

| Input | Required | Type | Default | Description |
|-------|----------|------|---------|-------------|
| `tag` | YES | string | (none) | Release tag to validate, for example v0.4.20 |
| `ref` | YES | string | `develop` | Source ref to validate before tagging |
| `demo_ref` | YES | string | (none) | Exact baseharbor-demo commit SHA to validate |
| `consumer_ref` | NO | string | `''` | Optional exact Connector candidate SHA; defaults to the reviewed integration branch |
| `only_gates` | NO | string | `''` | Optional comma-separated exact gate IDs for targeted execution without release approval |

**Pin inputs** (for v0.4.24/v0.4.25/v0.4.26, from pre-release.yml:131-136):
```bash
python3 scripts/ecosystem_release_pins.py --tag "$tag" --core "$(git rev-parse HEAD)" \
  --connector "$INPUT_CONSUMER_REF" --demo "$INPUT_DEMO_REF" \
  --output "docs/releases/$tag.consumer-pins.json"
```

**Pin values** (from mission context - o115 matrix):
- `--core 1861ed07` → core commit short SHA (must use full SHA: `1861ed07...`)
- `--connector df1f4378` → connector commit short SHA (must use full SHA: `df1f4378...`)
- `--demo` + `--console` → demo/console commit SHAs (computed by pins.py)
- `--ref` → develop tip or release/v0.4.26 head

## STEP 3: Parallelism / Cost Rules

### Concurrency Groups

**pre-release.yml lines 41-43**:
```yaml
concurrency:
  group: baseharbor-release-qualification
  cancel-in-progress: false
```

**All pre-release runs share a SINGLE concurrency group `baseharbor-release-qualification`.** This means:
- Only ONE pre-release workflow can run at a time across the entire repository
- Queued runs wait until the current one finishes (or is cancelled, but `cancel-in-progress: false`)
- **Cost implication**: Serializing all 54+ gates within one run already maximizes cost within one run. Additional parallelism must happen via the internal matrix strategy (max-parallel: 4-5 per job) within the single workflow run.

**targeted-demo-acceptance.yml lines 38-40**:
```yaml
concurrency:
  group: baseharbor-release-qualification
  cancel-in-progress: false
```

**Same concurrency group** - targeted-demo-acceptance.yml also competes with pre-release.yml for the same single slot.

**targeted-integration.yml lines 42-46**:
```yaml
concurrency:
  group: targeted-integration-${{ github.ref }}-${{ inputs.ref || github.sha }}
  cancel-in-progress: ${{ github.ref_name != 'work/v0.4.24-core-ha-updates' && github.ref_name != 'work/v0.4.25-contract-minimal-core' }}
```

**Separate concurrency group** per ref/input, so targeted-integration can run in parallel with pre-release.

### RUNBOOK Rules (from mission)

- Docker and Podman gates at need: **parallel**, NEVER serialized waiting
- Concurrency groups that block: document with `file:line` as finding
- PR triggers sharing the same Concurrency-Group: explicitly mark

**Finding**: pre-release.yml and targeted-demo-acceptance.yml share `baseharbor-release-qualification` (pre-release.yml:42, targeted-demo-acceptance.yml:39). Running them serially blocks the other.

### Parallel Execution Within pre-release.yml

The internal job strategy uses `max-parallel` settings:
- `static_gates`: max-parallel: 5 (pre-release.yml:526)
- `runtime_light_gates_docker`: max-parallel: 2 (pre-release.yml:543)
- `runtime_light_gates_podman`: max-parallel: 2 (pre-release.yml:562)
- `runtime_heavy_gates_docker`: max-parallel: 2 (pre-release.yml:597)
- `runtime_heavy_gates_podman`: max-parallel: 2 (pre-release.yml:615)
- `v0421_ha_groups_docker`: max-parallel: 2 (pre-release.yml:653)
- `v0421_ha_groups_podman`: max-parallel: 2 (pre-release.yml:750)
- `reference_journey`: max-parallel: 2 (pre-release.yml:858)

These internal parallelism settings mean gates within a single workflow run CAN execute in parallel (up to GitHub's runner limits).

## STEP 4: Order + Dependencies

### Gate dependency graph from release-requirements.json

Each gate lists its `dependencies` array. For v0.4.26:

**Level 0 (no dependencies / self-sufficient)**:
- None - all gates depend on at least `core`

**Level 1 (depends on `core` only)**:
- `integration/static/public-contracts` (v0.4.23+)
- `integration/static/configuration-policy` (v0.4.23+)
- `integration/static/browser-terminal` (v0.4.23+)
- `integration/docker/core-bootstrap` (v0.4.23+)
- `integration/podman/core-bootstrap` (v0.4.23+)
- `integration/docker/coreless-lifecycle` (v0.4.25+)
- `integration/podman/coreless-lifecycle` (v0.4.25+)

**Level 2 (depends on `core` + `demo`)**:
- ALL `atomic/static/*` (5 gates)
- ALL `atomic/docker/*` (17 gates)
- ALL `atomic/podman/*` (18 gates)
- ALL `adoption/*` (4 gates)
- ALL `ha/docker/*` (4 gates)
- ALL `ha/podman/*` (4 gates)
- `journey/docker`, `journey/podman` (2 gates)

**Level 3 (depends on `core` + `connector`)**:
- `integration/docker/remote-target`
- `integration/podman/remote-target`

**Level 4 (depends on `core` + `console` + `connector` + `demo`)**:
- `integration/static/live-console`

### Freeze-Merge dependency order

Gates that read consumer SHAs from `docs/releases/v0.4.26.consumer-pins.json` can only run AFTER the pins file is written to develop.

**The pins file is written by the `ecosystem_release_pins.py` script** during the "Resolve release candidate" step of pre-release.yml (line 132-134):
```python
python3 scripts/ecosystem_release_pins.py --tag "$tag" --core "$(git rev-parse HEAD)" \
  --connector "$INPUT_CONSUMER_REF" --demo "$INPUT_DEMO_REF" \
  --output "docs/releases/$tag.consumer-pins.json"
```

**Critical dependency chain:**

1. **PR must land `docs/releases/v0.4.26.consumer-pins.json` in develop** - this is the BLOCKING STEP. Without this file, every pre-release.yml run on develop fails at "Resolve release candidate".

2. **Freeze-merge demo/console/connector SHAs into the pins file** - the pins.py script computes exact SHAs from the consumer repos at their freeze commits.

3. **Then trigger pre-release.yml** with `tag: v0.4.26`, `ref: develop`, and the frozen consumer refs.

4. **Gate execution order within pre-release.yml** (already handled by GitHub Actions DAG):
   - `source` job first (resolves pins, builds snapshot)
   - `runtime_candidate` depends on `source`
   - `static_gates`, `adoption`, `integration_*` depend on `source`
   - `runtime_light_*`, `runtime_heavy_*`, `reference_journey` depend on `source` AND `runtime_candidate`
   - HA groups depend on `source`
   - `evidence` job is final gate, depends on ALL above

### Summary of Freeze-Merge dependency

| Gates | Can Run | Reason |
|-------|---------|--------|
| `integration/*` (core-bootstrap, coreless-lifecycle, remote-target, live-console) | AFTER pins file in develop | Need consumer_ref from pins |
| ALL `atomic/*`, `adoption/*`, `ha/*`, `journey/*` | AFTER pins file in develop | Demo_ref from pins, all depend on core+demo |
| `source` job | FIRST | Resolves pins to consumer-shas |

## STEP 5: Copy-Ready Command List

### Prerequisite: Get develop tip SHA

```bash
# Get the develop branch tip SHA
DEV_SHA=$(git -C /home/marcel/dev/baseharbor-ecosystem/baseharor rev-parse develop)
echo "develop tip: $DEV_SHA"
```

### Step A: Freeze-merge demo/console/connector and write pins file to develop

This must be done BEFORE triggering the pre-release workflow. The pins.py script requires the exact consumer SHAs.

```bash
# Get exact consumer SHAs (replace with actual freeze commits)
CORE_SHA="1861ed07..."  # full 40-char SHA of core at freeze
CONNECTOR_SHA="df1f4378..."  # full 40-char SHA of connector at freeze
DEMO_SHA="$(git -C /home/marcel/dev/baseharbor-ecosystem/baseharor-demo rev-parse HEAD)"
CONSOLE_SHA="$(git -C /home/marcel/dev/baseharbor-ecosystem/baseharor-console rev-parse HEAD)"

# Write pins file to a PR branch
cd /home/marcel/dev/baseharbor-ecosystem/baseharor
git checkout -b release/v0.4.26-pins develop
python3 scripts/ecosystem_release_pins.py --tag v0.4.26 --core "$CORE_SHA" \
  --connector "$CONNECTOR_SHA" --demo "$DEMO_SHA" \
  --output docs/releases/v0.4.26.consumer-pins.json

# Commit and PR
git add docs/releases/v0.4.26.consumer-pins.json
git commit -m "chore: add v0.4.26 consumer pins"
git push origin release/v0.4.26-pins
```

### Step B: Trigger the pre-release workflow

**This is NOT executed - provided as copy-ready commands.**

```bash
# PRIMARY: Full pre-release validation for v0.4.26
# NOTE: This will fail until docs/releases/v0.4.26.consumer-pins.json is in develop
gh workflow run pre-release.yml --repo mcpdev80/baseharbor \
  --ref develop \
  -f tag=v0.4.26 \
  -f ref=develop \
  -f demo_ref=<DEMO_FROZEN_SHA> \
  -f consumer_ref=<CONNECTOR_FROZEN_SHA>
```

### Step C: Targeted gate execution (individual gates, copy-ready)

These are NOT executed - copy-paste to run individual gates once the pins file exists.

```bash
# Static gates (via pre-release.yml matrix - triggered automatically by full run)
# No individual commands needed; they run within pre-release.yml static_gates job

# Individual runtime gates via pre-release-runtime-suite.yml (reusable)
# These are triggered by pre-release.yml, not direct dispatch

# For targeted single-gate testing via targeted-demo-acceptance.yml:
gh workflow run targeted-demo-acceptance.yml --repo mcpdev80/baseharbor \
  --ref develop \
  -f ref=<CORE_SHA> \
  -f demo_ref=<DEMO_FROZEN_SHA> \
  -f runtime=docker \
  -f gate=guided

# For targeted integration gates:
gh workflow run targeted-integration.yml --repo mcpdev80/baseharbor \
  --ref develop \
  -f gate=core-bootstrap \
  -f ref=<CORE_SHA>
```

### Step D: Verification after trigger

```bash
# Check if the pre-release run completed successfully
gh run list --repo mcpdev80/baseharbor --workflow pre-release.yml --branch develop --limit 3 --json status,conclusion,number

# Check individual job status
gh run view <RUN_NUMBER> --repo mcpdev80/baseharbor --json jobs
```

### Complete copy-ready command for one-shot full v0.4.26 pre-release run

```bash
# Replace placeholders with actual frozen consumer SHAs
cd /home/marcel/dev/baseharbor-ecosystem/baseharor

# 1. Ensure pins file is in develop
git fetch origin develop
git checkout develop
# (pins.py must have been run and committed)

# 2. Get current develop SHA
DEV_SHA="$(git rev-parse develop)"

# 3. Trigger pre-release
gh workflow run pre-release.yml --repo mcpdev80/baseharbor \
  --ref develop \
  -f tag=v0.4.26 \
  -f ref="$DEV_SHA" \
  -f demo_ref="$(git -C /home/marcel/dev/baseharbor-ecosystem/baseharor-demo rev-parse HEAD)" \
  -f consumer_ref="$(git -C /home/marcel/dev/baseharbor-ecosystem/baseharor-node-connector rev-parse HEAD)"
```

---

## Q116 DISPATCH SUMMARY

**BLOCKING PREREQUISITE**: `docs/releases/v0.4.26.consumer-pins.json` must be merged into develop BEFORE any pre-release.yml run on develop can succeed.

**Current state**: 15+ consecutive failures of pre-release.yml on develop, all at "Resolve release candidate" step, because the pins file is missing.

**Solution path**:
1. Compute consumer SHAs (core 1861ed07..., connector df1f4378..., demo, console)
2. Write pins file via ecosystem_release_pins.py
3. Merge pins file to develop
4. Trigger pre-release.yml with tag=v0.4.26, ref=develop

**Gate count for v0.4.26**: 54+ individual gates across 10+ job families.

**Parallelism**: Gates within pre-release.yml run in parallel via matrix strategies; pre-release.yml itself competes for a single concurrency group slot with targeted-demo-acceptance.yml.

**END Q116-W1DISPATCHSHEET-DONE**
