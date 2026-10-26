# O140 POLICYPR - prerequisite exact-source qualification PR for #1060

ROLE: WORKER. Separate prerequisite policy PR on develop, confined to
`scripts/pr-gate-plan.py` and `scripts/test_pr_gate_plan.py`. No product edits,
no ci.yml / pr-check-runner changes, no force/amend/merge, no PR merge, no CI
rerun/dispatch, no protection/Ruleset touch.

## PLAN

- [x] 0. basics-check.py turn-start, run FLAGS, record scope and checklist before
  investigation.
- [x] 1. Read binding mission (o140.txt), authoritative recipe (cmf18) and
  independent ACCEPT (o139-review1060.md).
- [x] 2. Fetch R read-only, create dedicated own worktree, derive exact blob
  hashes and MODIFIED inventory purely from Git objects.
- [x] 3. Implement exact-#1060 binding class in pr-gate-plan.py (before ordinary
  path classification); leave 3-file reader, #1056 record, ci.yml, runner and
  ordinary classification unchanged.
- [x] 4. Add tests in test_pr_gate_plan.py with real Git-object fixtures
  (positive + all negative + #1056 retention + record-only refresh).
- [x] 5. Run focused suites, capture exact outputs + exit codes into evidence.
- [x] 6. `git diff --check`, ordinary commit, push branch, open PR vs develop.
- [x] 7. Observe CI up to 25min, record exact logs into evidence.
- [x] 8. Finalize evidence with PR number + head SHA; STOP marker.

Native TodoWrite step 0 recorded; durable checklist above.

## Startup FLAGS

basics-check.py: 2 un-acked EVENTS, 4 unread REMINDERS, flash/ornith/qwen pool
below target if real work exists, 32 open PLAN steps. EVENTS/REMINDERS handled
locally; POOL/PLAN remain coordinator concerns, not for this narrow worker lane.
No unrelated Planning/claim/status changes.

## Verdict and immutable inputs

- R (final independently accepted #1060 candidate):
  `0f3bd169d4a02c5f019da822b4f45f66b972423e`
- R base develop / target-base: `65aa063e90767c85af976081c100be1817370e67`
- R has exactly ONE parent: `cb3a2bf7d73e21fae2ac80996cf3d5287ec0179f` (NOT an
  ancestor of develop; ordinary merge-chain proof cannot qualify R).
- Integration / merge-base of R vs develop: `65aa063e90767c85af976081c100be1817370e67`

### Exact blob hashes at R (derived from Git objects, not reports)

| File | Git object hash (blob) |
|---|---|
| .github/workflows/ci.yml | `cf84de8ecf4ec89cd8b93c75a4e4bfe8b9e25c85` |
| scripts/pr-check-runner.py | `4ebe9499ebdda1194e829a9b2f36bfce93548213` |
| scripts/pr-gate-plan.py | `838fb3d68f730693d0617e930a545279a0f33be9` |
| scripts/test_pr_gate_plan.py | `25554d0a2231d236c6f3bb2774018db39ceeb5fc` |

Raw git objects confirmed present: blob types and R tree `fa584ba329a05aa9ba6e62e430e34493a3ef4ea5`.
MODIFIED inventory via `git diff --name-status 65aa063e..FETCH_HEAD`:
`.github/workflows/ci.yml`, `scripts/pr-check-runner.py`,
`scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py` (4 modified).

## Immutable bindings to add (derived from Git objects; never invented)

```python
EXACT_1060_REVIEWED_HEAD = '0f3bd169d4a02c5f019da822b4f45f66b972423e'
EXACT_1060_REVIEWED_FILES = {
    '.github/workflows/ci.yml': 'modified',
    'scripts/pr-check-runner.py': 'modified',
    'scripts/pr-gate-plan.py': 'modified',
    'scripts/test_pr_gate_plan.py': 'modified',
}
EXACT_1060_REVIEWED_HASHES = {
    '.github/workflows/ci.yml': 'cf84de8ecf4ec89cd8b93c75a4e4bfe8b9e25c85',
    'scripts/pr-check-runner.py': '4ebe9499ebdda1194e829a9b2f36bfce93548213',
    'scripts/pr-gate-plan.py': '838fb3d68f730693d0617e930a545279a0f33be9',
    'scripts/test_pr_gate_plan.py': '25554d0a2231d236c6f3bb2774018db39ceeb5fc',
}
EXACT_1060_TARGET_BASE = '65aa063e90767c85af976081c100be1817370e67'
EXACT_1060_REQUIRED_GATES = {'CI_CHECKS', 'WORKFLOW_SYNTAX_CHECKS'}  # both required
```

## Stop criteria

any Inventory-/Hash-Ableitung aus Reports statt Git-Objekten; Bedarf an
ci.yml/pr-check-runner-Aenderung; Test-Luecke bei einem der Negativfaelle;
Push non-fast-forward; PR-Conflict mit develop.

## Secured state (o140-stop-preserve.txt, owner Marcel 2026-10-24)

Owner directive: no new work, secure the current coherent state. Finished the
coherent state (no half-edit), ran the focused suites once, ordinary commit +
ordinary push, opened PR #1064 (diff + tests green).

- Worktree: `/home/marcel/dev/baseharbor-ecosystem/worktrees/o140-policy`
- Branch: `work/v0.4.26-ci-qualifier-1060`
- Commit: `54dfa458528e958be0ea3d8157a46bd501e76eb1`
  `wip(o140): policy qualifier state 2026-10-24T15:30Z`
- Diff vs develop `65aa063e`: ONLY `scripts/pr-gate-plan.py` +
  `scripts/test_pr_gate_plan.py` (git diff --check clean)
- PR: #1064, OPEN, MERGEABLE, base develop
  https://github.com/mcpdev80/baseharbor/pull/1064
- CI run `40108385755` (PR #1064): completed, **success**, head `54dfa458`

### Focused suites (run once, exact outputs + exit codes)

```
$ python3 -m py_compile scripts/pr-gate-plan.py
EXIT=0
```

```
$ PYTHONPATH=scripts python3 -m pytest -q scripts/test_pr_gate_plan.py scripts/test_pr_check_runner.py
134 passed, 402 subtests passed in 47.13s
GATE_AND_RUNNER_EXIT=0
```

```
$ git diff --check
(no output)
DIFF_CHECK_EXIT=0
```

All 100 pre-existing test_pr_gate_plan tests still pass; 16 new Exact1060Binding/
RecordRetention tests pass (positive R + clean target-forward chain; negatives:
extra/renamed/deleted path, wrong status, wrong repo/branch, missing/altered
integration, changed blob, non-clean merge tree; #1056 three-file record
retention; record-only four-file bundle fails the exact binding without a clean
merge).

## END

END O140-POLICYPR-STOPPRESERVED
