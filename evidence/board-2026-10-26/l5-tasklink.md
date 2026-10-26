# L5 — Automatic Task Link for Receipt Approval (Dormant)

## PLAN

0. WORKTREE: git worktree add -b l5/ustrack /home/marcel/dev/baseharbor-ecosystem/worktrees/l5-ustrack 084b3abc — DONE
1. Read FREEZE-PR1.md + p2-shadow-runbook.md §3 "Automatic task link — missing building block" — DONE
2. Implement `scripts/receipt_tasklink.py` exactly as described in the runbook — DONE
3. Integrate as dormant job call in existing `receipt-post-run.yml` — DONE
4. Focused test `scripts/test_receipt_tasklink.py` — 10 tests OK — DONE
5. Existing 6 `test_receipt_chain.py` suites remain green — DONE
6. Commit, evidence — DONE

## Runbook Reference

Source: `evidence/board-2026-10-26/p2-shadow-runbook.md` section 3 "Automatic task link — missing building block"

The runbook states:

> Missing script/interface: **there is no code in PR 1 that regenerates the plan
> fixture with the real task reference.** The task link is produced by the
> **existing trusted planner** (the PR-2 post-run producer), which must write
> `evidence/receipt-plans/public-contracts.json` with the correct `"task":
> "tasks/<id>.md"`.

Proposed interface from the runbook (exact):

```
struct PlanFixture {
  gate: "public-contracts",
  repository: "mcpdev80/baseharbor",
  workflow_id: <int>,
  head_sha: <40>, candidate_sha: <40>, base_sha: <40>,
  task: "tasks/<task-id>.md"   // the automatic link
}
// written to evidence/receipt-plans/public-contracts.json in baseharbor-plan
```

Contract enforced by `load_plan(receipt_verifier.py:101-112)`:
- Exactly 7 fields: gate, repository, workflow_id, head_sha, candidate_sha, base_sha, task
- 40-char full SHAs for head/candidate/base
- task matches `tasks/[a-z0-9-]+\.md`
- gate == GATE ("public-contracts"), repository == REPOSITORY

## Implementation

File: `scripts/receipt_tasklink.py`

The script `produce_plan_fixture()` implements the exact interface from the runbook:

1. **Input**: `planning_root`, `workflow_id`, `head_sha`, `candidate_sha`, `base_sha`, `task`
2. **Validation**: sha40() checks all 40-char SHAs, TASK_RE validates task format
3. **PlanFixture struct**: dict with exactly the 7 fields from the runbook
4. **Idempotency**: if existing fixture matches canonical, returns early
5. **Commit**: writes `evidence/receipt-plans/public-contracts.json` and commits
6. **Dry-run**: can emit to stdout without committing
7. **Fail-closed**: all validation errors raise `TaskLinkError`

The script is called by the trusted planner in the post-run workflow with:
```bash
python3 scripts/receipt_tasklink.py \
  --planning <planning-repo> \
  --workflow-id <run_id> \
  --head <head_sha> \
  --candidate <candidate_sha> \
  --base <base_sha> \
  --task "tasks/<task-id>.md"
```

## Integration — Dormant

File: `.github/workflows/receipt-post-run.yml`

The tasklink step is integrated as a DORMANT job call within the existing
dormant guard (`github.repository == 'mcpdev80/receipt-post-run-dormant'`).
Activation requires the separate PR2 commit that replaces this guard.

## Tests

File: `scripts/test_receipt_tasklink.py` — 10 focused tests:

| Test | What it verifies |
|------|-------------------|
| `test_produce_plan_fixture_writes_correct_fixture` | Plan fixture is written with correct 7 fields |
| `test_load_plan_consumes_fixture` | `load_plan()` can read the fixture (link shows on right Task-Karte) |
| `test_fail_closed_missing_task_reference` | Rejects invalid task reference |
| `test_fail_closed_wrong_sha_format` | Rejects non-40-char SHAs |
| `test_fail_closed_wrong_workflow_id` | Rejects invalid workflow_id |
| `test_idempotent_same_fixture_returns_existing_commit` | Same inputs → same commit (no duplicate) |
| `test_fail_closed_different_receipt` | Detects mismatched existing fixture |
| `test_dry_run_emits_to_stdout` | Dry-run writes to stdout, not filesystem |
| `test_cli_produces_plan` | CLI writes fixture and prints commit |
| `test_cli_fails_on_invalid_task` | CLI exits nonzero on invalid task |

All 10 tests pass.

## Existing suites — still green

`scripts/test_receipt_chain.py` — 6 original suites:

| Test | Status |
|------|--------|
| `test_real_git_roundtrip_and_offline_approval_binding` | OK |
| `test_negative_wrong_candidate_sha` | OK |
| `test_negative_dead_run_id` | OK |
| `test_negative_edited_receipt_including_edit_and_revert` | OK |
| `test_failed_skipped_stale_or_foreign_live_evidence_blocks` | OK |
| `test_approval_identity_and_strict_json` | OK |

All 6 pass.

## Commit

```
6e170894 feat(ci): bounded automatic task link for receipt approval (dormant)
```

## END L5-DONE
