# o1-binding-pause-note (2026-10-19)

Task: t-20261019-core921-coverage-binding
Branch: `work/ci-core921-coverage-binding` in `worktrees/ci-core921-coverage-binding`
(develop base `f2bad5ef`)

## Coordinator hold (2026-10-19, controlled pause)

All further work STOPPED. No commit, no push, no PR, no CI. Both modified files
left **UNCOMMITTED** exactly as they are.

### git status --short
```
 M scripts/pr-gate-plan.py
 M scripts/test_pr_gate_plan.py
```

### git diff --stat
```
 scripts/pr-gate-plan.py      |  51 +++++++++++++++
 scripts/test_pr_gate_plan.py | 146 +++++++++++++++++++++++++++++++++++++++++++
 2 files changed, 197 insertions(+)
```

### Complete
- Read baseharbor/AGENTS.md (CI discipline) and baseharbor/AGENTS.md (worktree).
- Confirmed branch `work/ci-core921-coverage-binding` based on develop f2bad5ef, clean tree.
- Read exact #974 clause structure (constant at pr-gate-plan.py:263, exact-match clause at :550-569) and the existing `RegistrationBootstrapPolicyTests` test at test_pr_gate_plan.py:260-327.
- Added `CORE_921_PRODUCTION_REVIEWED_HEAD`/`CORE_921_PRODUCTION_REVIEWED_FILES` constant to scripts/pr-gate-plan.py (13-file path/status set: app_registration_921.go+added, commands.go+M, machine_coverage.go+M, mcp_tools.go+M, registration_921_test.go+added, contracts/machine/v1/control.golden.json+M, 6 EN/DE docs+M, internal/machine/contract.go+M).
- Added one exact-match clause after the #974 clause (before the WIZARD clause), mirroring #974: `len(paths)`/`set(paths)`/per-path status match + `core752_reviewed_merge_chain(target, head, CORE_921_PRODUCTION_REVIEWED_HEAD)`, requiring `CI_CHECKS` + `CODE_CHECKS` + `CLI_CHECKS` + `DOC_CHECKS`.
- Added `Core921ProductionRegistrationCoverageTests` class to test_pr_gate_plan.py with 3 tests: exact reviewed bundle resolves with full check set; mutated/unreviewed head + forced-fail merge chain stays unresolved; changed status + deleted added path stays unresolved.
- `python3 -m py_compile` passes on BOTH scripts.
- `python3 scripts/test_pr_gate_plan.py`: 56 tests run; 3 NEW core-921 tests — 1 PASS, 2 FAIL (see pending).

### Pending / in progress
- `test_exact_reviewed_bundle_resolves_with_full_check_set`: **PASS**.
- `test_mutated_reviewed_head_and_dirty_merge_chain_do_not_resolve`: **FAIL** — the git fixture builds an integration commit whose parents are `[target, head]` but `resolve_pr_diff` rejects with `integration_sha_mismatch`. Root cause diagnosed: in the two-line-from-common-baseline fixture, `head = merge(source, target)` makes `head`'s second parent the *advanced* target line, not a clean source-head; the integration-parent validation (pr-diff-base.py:142 requires parents == [target_sha, head_sha]) and the merge-base check do not line up. The correct fixture construction (matching the proven WIZARD pattern where `head` = merge of `source` and a *linear* target advance) still needs to be pinned down and the test rewritten.
- `test_changed_status_and_extra_path_do_not_resolve`: pending (depends on the fixture pattern from the mutated test once confirmed working).
- Remaining task steps after tests pass: run `python3 scripts/test_pr_check_runner.py`; commit coherently (no `git add -A`); push branch; open non-draft PR `gh pr create --repo mcpdev80/baseharbor --base develop --head work/ci-core921-coverage-binding` titled "ci(core-921): bind reviewed registration coverage"; write PR number, head SHA, test outputs to evidence/board-2026-10-19/o1-921-binding.md.

### Exact resume point
Resume at: fix `test_mutated_reviewed_head_and_dirty_merge_chain_do_not_resolve` — construct the git fixture so `head` is a clean two-parent merge `[source_head, target_linear_advance]` and `integration` is `[target, head]`, then force `core752_reviewed_merge_chain` to return False and assert `coverage_status == 'unresolved'`. Files already compiled clean; do NOT re-run without the fixture fixed. Then proceed to the pending steps above. Do NOT commit/push/PR until all three core-921 tests pass and test_pr_check_runner.py is green.
