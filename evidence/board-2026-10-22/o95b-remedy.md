# o95b — trusted-plan index-reset race fix (o90 prereq)

STATUS: accepted + pushed. o90-planner-pr head `4e737e26af38e7f712f2de1f631633d8d59477b9`.

## PLAN
1. Turn-start basics-check.
2. Read o95b.txt: f36 had parallel pushes — verify remote head before pushing, build on remote, fix regressions from the remote tip.
3. Check remote head: already `de500239` (my own commit), no divergence — nothing to rebase over.
4. Diagnose the 3 CI failures on remote `de500239` (PR #1056): `PR · Focused Python`, `PR · Validation · pr-planner-tests`, `PR Selected Checks` all error/FAIL in `TrustedBindingActivationTests`.
5. Root-cause: `resolve_pr_diff()` runs `git merge-tree --write-tree`, which leaves the temp-repo git index dirty. The next `resolved()` call's `git checkout --detach self.base` then fails with exit 128; the `TemporaryDirectory` teardown races behind it. Systemic, not benign teardown noise.
6. Fix: after `merge-tree`, reset the index (`git reset -q`) so the next `resolved()` detaches cleanly to `self.base`.
7. Push, await CI, record evidence.

## repo-rule compliance
- Read-only investigation of the pre-existing PR #1056 (branch o90-planner-pr). My change is a single-file test fix to `scripts/test_pr_gate_plan.py`, amended onto my own commit `de500239` (identical scripts).
- No merge, no approve, no CI-gate actions. Only pushed to o90-planner-pr.
- Verified CI checks out base `1861ed07`; the `coverage-unresolved` block is a pre-existing planning requirement on that base, unrelated to this test-only fix.

## Fix: `scripts/test_pr_gate_plan.py` `TrustedBindingActivationTests.resolved()`
After `tree = self.git('merge-tree', '--write-tree', target, head).splitlines()[0]`,
add `self.git('reset', '-q')` before building the integration commit. This clears
the index that `merge-tree --write-tree` dirties, so the next `resolved()`
`git checkout --detach self.base` no longer exits 128.

## Evidence
### CI run #40039719214 (SHA `4e737e26`, o90-planner-pr)
- `Workflow policy`: SUCCESS
- `PR · Trusted Plan`: SUCCESS
- `PR · Focused Python`: SUCCESS (was FAILURE on `de500239`)
- `PR · Validation · pr-planner-tests`: SUCCESS (was FAILURE on `de500239`)
- `PR · Validation · ci-executor-verifier-tests`: SUCCESS
- `PR · Validation · ci-policy`: SUCCESS
- `PR Selected Checks`: FAILURE — `BLOCKED: coverage-unresolved: CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py`

### Root cause of `PR Selected Checks` block
This is a pre-existing trusted-plan gate on base `1861ed07`, NOT caused by the
test fix. The pins-data routing that classifies `scripts/pr-check-runner.py`
additions as `pins-data` (PR #1056) is not present in CI base `1861ed07`, so the
planner still flags the shared-script mutation as `coverage-unresolved`. Confirmed:
the block message is byte-identical on the `de500239` run (earlier) and the
`4e737e26` run (this fix). The scripts at `4e737e26` are byte-identical to PR #1056
`3c02f48`; the block is orthogonal to this test-only change and outside this
session's scope (PR #1056 script changes are already on the remote; F36 is the
pins-data qualification, which is already in place).

### Local verification
- 4 previously-failing `TrustedBindingActivationTests` (2 crash + 2 assertion): pass 5/5 runs.
- Full suite `python3 -m unittest scripts.test_pr_gate_plan`: 101 tests OK (was 2 failures + 23 errors on `de500239`).

## Head
HEAD SHA (o90-planner-pr, verified): `4e737e26af38e7f712f2de1f631633d8d59477b9`
PR #1056: OPEN, not merged. CI: test-race fixed (Focused Python + pr-planner-tests green); `PR Selected Checks` remains blocked by the pre-existing `coverage-unresolved` plan gate on base `1861ed07`, which is outside this session's scope.

END O95B-REMEDY-DONE
