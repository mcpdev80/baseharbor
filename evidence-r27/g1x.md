# g1x — F1 post-run ID/revision wiring (Owner correction 2026-10-27)

LANE g1x (ornith). Worktree base `839e5cc9` (v3.4). File scope per
`t-20261022-r27-evidence-publisher.md` "Required correction" and Owner addendum:
only `.github/workflows/receipt-post-run.yml`, `receipt-shadow.yml`,
`scripts/test_receipt_postrun.py`. `receipt_tasklink.py` / `receipt_verifier.py`
left untouched (g2x Owner).

## PLAN

1. [x] Read Owner task contract + Codex F1 review + current head.
2. [x] Read uncommitted foreign diff vs F1 recipe.
3. [x] Target branch `work/r27-fix-f1-postrun` empty (ls-remote), push allowed.
4. [x] Fix decode block: artifact-as-data, validation, no :-unknown, no eval-of-text.
5. [x] Fix controller checkout pin (develop -> reviewed SHA).
6. [x] Fix tasklink step: id + unambiguous plan_revision output.
7. [x] Fix publish step: RUN_ID=run.id, PLAN_REVISION=tasklink plan commit.
8. [x] Qualification: YAML, unittest full count, run-block repro.
9. [x] Evidence + ordinary commit/push (Zwischenstand).

## F1 proof items

### 1. run.id strictly separate from workflow_id; attempt!=1 rejected

- `receipt-post-run.yml:98` sets `RUN_ID=${{ github.event.workflow_run.id }}`;
  decode block exports `run_id` from the same `workflow_run.id`.
- `workflow_id` comes from `workflow_run.workflow_id` (decode validator,
  base64 source `/tmp/v.py`) and is stored only in the plan fixture.
- Publish step `receipt-post-run.yml:204` `RUN_ID: ${{ needs.extract_plan.outputs.run_id }}`
  (live run id, NOT the definition id).
- `receipt_writer.py:241` `require(value["run"]["attempt"] == 1, ...)` fails a
  rerun; `live()` (verifier:245-254) checks `run.id == receipt.run.id` and
  `run.workflow_id == plan.workflow_id` — neither weakened.
- Proof: `F1PositiveRun` asserts `assertNotEqual(int(RUN_ID), workflow_id)`
  (test:316) with distinct fixture IDs; `RUN_ID != workflow_id` in the
  published receipt.

### 2. plan_revision from tasklink step output, NOT parse-text; candidate separate; reachable

- Tasklink step `receipt-post-run.yml:153` has `id: plan_commit`.
- Result extracted as last 40-hex line of stdout
  (`receipt-post-run.yml:186-192`), never parsed from progress text.
- Publish step `receipt-post-run.yml:205` `PLAN_REVISION: ${{ steps.plan_commit.outputs.plan_revision }}`.
- `candidate_sha` stays separate (validator emits it; not substituted for plan).
- Proof: `F1PositiveRun` asserts the returned plan commit equals the Planning
  commit and the published receipt `plan` field equals that commit, which is
  reachable on the bare remote `main` (test:293-318). No local-only commit.

### 3. reject invalid extraction at source; no :-unknown; 40-hex + positive-int

- Validator (base64 `/tmp/v.py`) requires `workflow_id` positive int,
  `head_sha`/`integration_sha`/`target_base_sha` exactly 40 lowercase hex.
- No `:-unknown`, no `unknown`, no candidate-to-head fallback.
- Missing `integration_sha` / missing artifact / malformed -> nonzero exit
  before tasklink. `eval "$decoded"` only reassigns validated vars.
- Proof: `F1NegativeMissingArtifact` (test:342) asserts nonzero extraction with
  no outputs; validator repro rejects missing artifact (exit 1) and missing
  `integration_sha` (exit "missing required integration_sha").

### 4. artifact never source/eval; heredoc redirect on Python call; no undef vars under set -u

- Artifact parsed via `base64 -d | python3 -` (literal source, never `source`/
  `eval` of artifact content); the only `eval` reassigns already-validated
  KEY=VALUE pairs from the validator's own stdout.
- `set -euo pipefail` throughout; every exported var derives from the validator
  or `workflow_run` facts (no undefined var under `-u`).
- Proof: `test_all_run_blocks_exist` checks `bash -n` clean and no `PY > plan.txt`
  glued delimiter; no `ref: develop` remains.

### Repo-rule compliance

- Exclusive writes only: `receipt-post-run.yml`, `test_receipt_postrun.py`.
  `receipt_tasklink.py`/`receipt_verifier.py`/`receipt-shadow.yml` unchanged by
  this lane (shadow read-verified, YAML OK). No tasklink/verifier edits.
- No force push, no dispatch, no develop/main write, no secrets logged.
- `python3 -c yaml.safe_load` passes for both workflows; full receipt suite:
  **129 tests OK** (was failing pre-fix); F1 subset 5/5 pass.

## Repo-rule compliance summary

| Rule | Status |
| --- | --- |
| Exclusive write set | OK (post-run.yml, test_receipt_postrun.py) |
| No verifier/tasklink edits | OK |
| YAML valid (both workflows) | OK |
| 40-hex / positive-int validation | OK |
| run.id != workflow_id | OK |
| plan_revision from tasklink output | OK |
| Missing/malformed artifact blocks | OK |
| Full receipt suite green | OK (129) |

END G1X-DONE
