# Evidence: ci1 repair (r27) — HOST-CI-5 actionlint parse error cascade

## PLAN

1. [x] Gather HOST-LOG error lines for all 5 failing jobs from run 40500234432
2. [x] Identify root cause: YAML parse error in receipt-post-run.yml:107
3. [x] Fix: replace multi-line `python3 -c "..."` with single-line `python3 -c '...'`
4. [x] Verify actionlint passes on all workflows
5. [x] Verify ci-workflow-policy passes
6. [x] Verify test_pr_gate_plan and test_pr_diff_base pass
7. [x] Commit and push work/r27-ci-repair
8. [x] Create DRAFT-PR #1115 against develop
9. [x] Write this evidence file

## Log-Auszuege

### 1. Workflow policy — Validate workflow syntax
```
.github/workflows/receipt-post-run.yml:107:0: could not parse as YAML: could not find expected ':' [syntax-check]
    |
107 | import json, zipfile, sys
    | 
```

### 2. PR · Focused Python — Focused diff/planner/verifier/pins regressions
```
ERROR: test_receipt_execution_check_actually_runs_receipt_suites
...
RuntimeError: receipt discovery/execution failed: test_approval_identity_and_strict_json
```
Root cause: actionlint YAML parse error blocks receipt workflow discovery.

### 3. PR · Validation · ci-actionlint — Execute existing validation owner
```
actionlint exits with error on receipt-post-run.yml:107 (same YAML parse error)
```

### 4. PR · Validation · pr-planner-tests — Execute existing validation owner
```
Planner validation depends on actionlint-passing workflows; parse error cascades.
```

### 5. PR Selected Checks — Enforce every required check
```
BLOCKED: coverage-unresolved: workflow execution/cleanup/evidence impact requires native qualification
BLOCKED: required authenticated job not successful: PR · Focused Python
BLOCKED: required authenticated job not successful: Workflow policy
BLOCKED: required validation not successful: ci-actionlint (blocked)
BLOCKED: required validation not successful: pr-planner-tests (blocked)
```
Downstream of #1–4.

## Lokale Wiederholungen

```
$ actionlint -shellcheck= .github/workflows/*.yml
# exit 0, no output → PASS

$ bash scripts/ci-workflow-policy.sh
Workflow policy PASS: 22 approved workflow files.

$ python3 scripts/test_pr_gate_plan.py
Ran 5 tests in 5.075s — OK

$ python3 scripts/test_pr_diff_base.py
Ran 13 tests in 1.10s — OK
```

## repo-rule compliance

- **Actionlint (v1.7.12)**: All 22 workflow files pass syntax validation with zero errors.
- **ci-workflow-policy.sh**: Confirmed 22 approved workflow files, no additions/removals.
- **pr-gate-plan.py / test_pr_gate_plan.py**: All planner tests pass.
- **pr-check-runner.py / test_pr_diff_base.py**: All diff/coverage tests pass.
- **Receipt workflow syntax**: `receipt-post-run.yml` now parses cleanly; single-line python3 call avoids YAML key collision.
- **Cross-compatibility with #1112 / #1114**: Fix is purely syntax-level (no import/runtime changes). Compatible with writer push-propagation and tasklink autoresolve.
- **No content changes**: Only changed `python3 -c "..."` multi-line block to single-line; no logic, no allowlist extensions, no new tests.
- **No changes to receipt_writer.py / receipt_verifier.py / test_receipt_chain.py**: As confirmed by Owner-Review e4fb183.

## Tip-SHA

`6c8aeef4e4cb816943d89988c3bb8b216d7e3e0a`

## DRAFT-PR

https://github.com/mcpdev80/baseharbor/pull/1115
