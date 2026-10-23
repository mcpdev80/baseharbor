# O90 — pr-gate-plan.py: qualify docs/releases/*.consumer-pins.json as pins-data path

## Status: IMPLEMENTATION COMPLETE; BLOCKED on owner merge authority (bootstrap lock)

Implementation is done and verified. CI blocks on a self-qualification lock that
requires the owner (Markus/Codeman) to merge the trusted-planner fix to develop.
See `## Blocker` below. No merge performed, no CI bypass attempted.

## PLAN

- [x] 0. Read binding policies, dispatched prompt and codex-verdict-1050 Folgebefund; establish worker-lane scope; record PLAN.
- [x] 1. Verify worktree `work/v0.4.26-gateplan-pins` (base `1861ed07`); no other lane owns my branch.
- [x] 2. Inspect `pr-gate-plan.py` classifier (`scripts/pr-gate-plan.py:1039` permissive branch; JSON fall-through to `unresolved` at `:1153`); `pr-check-runner.py` `VALIDATIONS` whitelist.
- [x] 3. Add pins-data validation profile: `docs/releases/<tag>.consumer-pins.json` adds route to one `pins-data-validation`, no runtime gates.
- [x] 4. Register `pins-data-validation` in `pr-check-runner.py` + real fail-closed validator (public origin surface).
- [x] 5. Fail-first regressions in `test_pr_gate_plan.py`.
- [x] 6. Existing suite: 101 tests OK, `test_pr_check_runner` 18 tests OK.
- [x] 7. Opened PRs #1055 (integrated) and #1056 (planner-only against develop).
- [x] 8. Diagnosed CI bootstrap lock; documented owner-merge prerequisite.

## Implementation (committed to `o90-planner-pr`, also in `work/v0.4.26-gateplan-pins`)

- `scripts/pr-gate-plan.py`: permissive branch `elif path.endswith('.consumer-pins.json') and path.startswith('docs/releases/')` -> `pins-data` area, `require(PINNED_PINS_VALIDATION)`, `selected_gates=[]`. `path_status != 'added'` stays unresolved.
- `scripts/pr-check-runner.py`: `pins-data-validation` added to `VALIDATIONS`; `validate_pins_data()` reads the candidate file, enforces correct consumer repositories, 40-hex commits, and the single joint Connector commit shared across every gate. Malformed/missing/wrong-repo/non-hex/divergent/non-added/out-of-directory data fails.
- `scripts/test_pr_gate_plan.py`: `PinsDataQualificationTests` (6 tests) — pins-only add resolves with exactly `pins-data-validation` and zero runtime matrix; fail-closed for malformed/missing/wrong-repo/non-hex/divergent/non-added/out-of-directory.

## Blocker (definitive, CI-proven)

The trusted `pr_plan` job in `.github/workflows/ci.yml` checks out
`${{ github.event.pull_request.base.sha }}` (pinned develop `1861ed07`,
`allow-unsafe-pr-checkout: false`), **never the PR branch** (ci.yml:445). The
comment there is explicit: "The base is pinned to base.sha". Therefore the
pinned-develop trusted planner classifies the candidate; a planner change in an
untrusted candidate cannot qualify itself.

Evidence from CI run 40035332774 (PR #1056):

- With `pr-gate-plan.py` **and** `pr-check-runner.py` modified (my full fix):
  `PR · Trusted Plan` SUCCESS, but `PR Selected Checks` FAILURE with
  `BLOCKED: coverage-unresolved: CI execution/evidence impact requires specific
  qualification: scripts/pr-check-runner.py`.
- With **only** `pr-gate-plan.py` modified (no `pr-check-runner.py`): the pins
  add resolves cleanly (`resolved`, empty `unresolved_coverage`) — verified both
  locally and against the committed HEAD.

So the only remaining blocker is the self-qualification of my
`pr-check-runner.py` edit (registering the validator + `validate_pins_data`).
The pinned-develop classifier routes any modified `scripts/pr-check-runner.py`
to `ci-executor-unresolved`; my change does not match an existing reviewed-SHA
allowlist (`BINDING_PR_1007_REVIEWED_FILES`/`CORE_991_REVIEWED_FILES` name the
trio but require a reviewed SHA I do not hold).

## Required owner action (not a worker action)

1. Review + merge `o90-planner-pr` (#1056) to develop through the normal
   trusted-source path (owner/Codeman merge authority; this is exactly the
   CI-readiness prerequisite from the codex-verdict-1050 Folgebefund recipe
   step 3).
2. Then #1050 (consumerpins) — or a fresh pins-only PR — will pass: `PR ·
   Trusted Plan` resolved, `pins-data-validation` SUCCESS, Focused Python /
   Workflow policy SUCCESS, and `PR Selected Checks` SUCCESS from app 15368.
   `Consumer Pins` / `Selected Runtime` remain correctly SKIPPED (core mode for
   a metadata-only add).

## END O90-GATEPLAN-PINS-DONE (implementation complete; owner-merge prerequisite)

- **pr**: #1056 `o90-planner-pr` against develop (planner-only, no pins payload); integrated candidate on `work/v0.4.26-gateplan-pins` (#1055).
- **head**: `o90-planner-pr` = `3c02f48c` (branch HEAD); `work/v0.4.26-gateplan-pins` = `4f294811`.
- **test**: `python3 -m unittest scripts.test_pr_gate_plan` → 101 OK; `scripts.test_pr_check_runner` → 18 OK. CI #1056: Trusted Plan SUCCESS, all validations PASS, only blocker is pinned-develop self-qualification of `pr-check-runner.py` (run 40035332774). Requires owner merge to develop.

## repo-rule compliance

Changes stay inside `scripts/pr-gate-plan.py`, `scripts/pr-check-runner.py`,
`scripts/test_pr_gate_plan.py` on isolated worktrees. Root git never used. No
`ci.yml` `paths` filter, label, or dispatch-input shortcut. No pins-payload
change in #1056. No merge, no CI bypass, no force-push of protected develop.
Verified `git diff --check` clean; no secrets logged.
