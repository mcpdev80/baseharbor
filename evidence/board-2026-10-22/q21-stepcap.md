# Q21 — Step-Cap Fix: data-podman `Run runtime gate` 8-minute timeout

## Problem

F7-native-dispatch.md §Runs row 6: podman/data-capabilities run 39836743807
hit the 8-minute step cap at `19:12:07Z` on the `Run runtime gate` step
(pre-release-runtime-suite.yml:201, else-branch `8`), before a RESULT line
could be emitted. The actual gate took ~9 min (dispatch 19:03Z → error
19:10:42Z → timeout 19:12:07Z → terminal 19:13Z).

Expected duration from canonical enumeration (o21-matrix-inputs.md:52,70):
**20–40 minutes** per heavy data-capabilities gate. 8 minutes is
**insufficient by 6×** for the actual workload profile.

## Root cause

The step-level `timeout-minutes` at `pre-release-runtime-suite.yml:201`
uses a gate-specific ternary chain:

```yaml
timeout-minutes: ${{ inputs.gate == 'backup-restore' && 40 || steps.resource_profile.outputs.profile == 'recovery' && 20 || inputs.gate == 'security' && 12 || 8 }}
```

For `data-capabilities`, the profile is `"data"` (gate-resources.json:78–90),
which does NOT match `'recovery'`. The else-branch `8` applies — but the
actual data profile takes 9+ min even on failure, and 20–40 min when
proceeding through all 5 capability rows.

## Core C references

| Item | File:Line | Detail |
|------|-----------|--------|
| Step cap definition | `baseharbor/.github/workflows/pre-release-runtime-suite.yml:201` | `timeout-minutes` ternary, else-branch `8` |
| Step name | `pre-release-runtime-suite.yml:196-201` | `- name: Run runtime gate / id: runtime_gate` |
| Job cap | `pre-release-runtime-suite.yml:35` | `timeout-minutes: 60` — not the blocker |
| Data profile definition | `baseharbor-demo/tests/gate-resources.json:78-90` | `"profile": "data"`, 5 capabilities, `"class": "runtime"` |
| Evidence: run | `evidence/board-2026-10-22/f7-native-dispatch.md:63` | podman/data timeout at 19:12:07Z |
| Evidence: expected | `evidence/board-2026-10-22/o21-matrix-inputs.md:52,70` | 20–40 min expected |
| Evidence: cost | `evidence/board-2026-10-22/f4-demo-trigger-plan.md:76` | 15–30 min per run |
| Evidence: timing | `evidence/board-2026-10-22/f5-demo-gates.md:71-74` | new_edit.py 111s on podman |

## Proposed fix: split + extend

### Option A — Profile-based timeout extend (minimal change)

**File:Line**: `pre-release-runtime-suite.yml:201`

**Change**: Add `data` to the profile check, extend its budget to match
the existing recovery profile budget of 20 minutes (empirically insufficient
for a full pass but sufficient to avoid the cap on failures):

```yaml
timeout-minutes: ${{ inputs.gate == 'backup-restore' && 40 || steps.resource_profile.outputs.profile == 'recovery' && 20 || steps.resource_profile.outputs.profile == 'data' && 20 || inputs.gate == 'security' && 12 || 8 }}
```

**Expected duration per step**: 20 min cap. Actual run ~9 min on failure.
Sufficient headroom.

**Risk**: If a full data-capabilities pass takes 20–40 min (cold build),
this cap would still fire. However, the data-capabilities gate is invoked
through `scripts/acceptance.sh` which either fails fast on wizard interaction
(3–9 min as observed) or proceeds through all 5 capability rows. The 20-min
cap covers the observed failure path with headroom.

**Test requirement**: Verify that data-capabilities on podman no longer hits
the step cap. Run the gate against C/D or the fixed candidate. Expected:
terminal within 20 min.

---

### Option B — Split podman/data into a dedicated job with extended budget

**File:Line**: `pre-release-runtime-suite.yml:31-335`

**Change**: Extract a second job `heavy_gate` that shares the pre-step
infrastructure (checkout, runtime prepare, image load) but runs the
gate step with extended timeout. This preserves the single-job evidence
structure while giving data-capabilities its own budget.

**Rationale**: The 8-min cap is a hard GitHub Actions limit on a single
step. Splitting doesn't change the step cap but allows reorganizing
the heavy gate into a job with a higher step budget by moving the
infrastructure steps to a pre-setup job and the gate execution to a
dedicated step with the extended cap.

**Expected duration**: Same gate time (~9 min on failure, 20–40 min on
full pass) but now under a 20–40 min step budget.

**This option is not recommended** — it adds structural complexity without
solving the root cause (missing profile entry in the ternary).

---

### Option C — Gate-specific override + profile extension (recommended)

**File:Line**: `pre-release-runtime-suite.yml:201`

**Change**: Add both `'data'` profile AND `data-capabilities` gate overrides:

```yaml
timeout-minutes: ${{
  inputs.gate == 'backup-restore' && 40 ||
  steps.resource_profile.outputs.profile == 'recovery' && 20 ||
  steps.resource_profile.outputs.profile == 'data' && 20 ||
  inputs.gate == 'data-capabilities' && 25 ||
  inputs.gate == 'security' && 12 ||
  8
}}
```

**Expected duration per step**:
- `backup-restore`: 40 min (unchanged — empirical baseline)
- `recovery`: 20 min (unchanged — empirical baseline)
- `data` profile / `data-capabilities` gate: **25 min** (extends 8→25,
  empirical baseline 9 min on failure, 20–40 min full pass; 25 min
  covers failure path + warm-build full pass with 2 min headroom)
- `security`: 12 min (unchanged — empirical baseline)
- other gates: 8 min (unchanged)

**Test requirement**:
1. Run `data-capabilities` on podman against the candidate — verify terminal
   within 25 min, no step cap timeout.
2. Run `data-capabilities` on docker against the candidate — verify unchanged
   behavior (docker already uses 8 min but completes in ~3 min on failure).
3. Verify evidence semantics preserved: the `manifest.json` at
   `pre-release-runtime-suite.yml:370-397` must still contain the correct
   `profile` field (`data`) and the gate evidence TSV at
   `pre-release-runtime-suite.yml:345-355` must still reference
   `runtime_gate.outcome`.

**Stop criterion**: `data-capabilities` on podman produces a terminal
outcome (PASS or FAIL) within the 25-min step budget. No `The action
'Run runtime gate' has timed out after N minutes` in the step log.

---

## Worker-ready recipe

### File:Line, change

**`baseharbor/.github/workflows/pre-release-runtime-suite.yml:201`**

Replace:
```yaml
timeout-minutes: ${{ inputs.gate == 'backup-restore' && 40 || steps.resource_profile.outputs.profile == 'recovery' && 20 || inputs.gate == 'security' && 12 || 8 }}
```

With (Option C):
```yaml
timeout-minutes: ${{
  inputs.gate == 'backup-restore' && 40 ||
  steps.resource_profile.outputs.profile == 'recovery' && 20 ||
  steps.resource_profile.outputs.profile == 'data' && 20 ||
  inputs.gate == 'data-capabilities' && 25 ||
  inputs.gate == 'security' && 12 ||
  8
}}
```

### Test requirement

1. `git diff` shows only the timeout-minutes expression changed.
2. Run `data-capabilities` on podman via `workflow_dispatch`:
   - Input: `runtime=podman`, `gate=data-capabilities`, `ref=C`, `demo_ref=D`
   - Expected: terminal outcome within 25 min, no timeout error
   - Evidence: `pre-release-gate-podman-data-capabilities-evidence-*` artifact
     contains `manifest.json` with `profile: "data"`, `gate: "data-capabilities"`
3. Run `data-capabilities` on docker — verify unchanged (8 min cap still
   applies but completes in ~3 min on failure).

### Stop criterion

- `[ ]` No `The action 'Run runtime gate' has timed out after` in the step log
- `[ ]` `manifest.json` outcome is terminal (PASS or FAIL, not timeout)
- `[ ]` `profile` field in manifest = `"data"`
- `[ ]` `gate` field in manifest = `"data-capabilities"`
- `[ ]` Runtime gate duration ≤ 25 min

### Gate evidence preservation

The following evidence artifacts and their schemas MUST remain unchanged:
- `manifest.json` at `pre-release-runtime-suite.yml:370-397` — schema
  `baseharbor.pre-release.gate-evidence/v1`
- `atomic-gate-evidence.tsv` at `pre-release-runtime-suite.yml:345-355`
- `pre-release-gate-*-evidence-*` artifact upload at `pre-release-runtime-suite.yml:399-406`

### Prior art

This change is consistent with the existing pattern: `backup-restore` has
40 min, `recovery` has 20 min, `security` has 12 min. Adding `data`
profile (20 min) and `data-capabilities` gate (25 min) extends the same
ternary chain without structural modification.

---

END Q21-STEPCAP-DONE
