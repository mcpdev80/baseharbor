# f45 — PR #1056 rebase onto develop (o90-planner-pr)

Mission: ops/missions/f45.txt | Lane: [wip:f45] | Repo: baseharbor (absolute-path git only)
Date: 2026-10-24 | Coordinator basics-check ran at turn start (flags are coordinator-host level; no owner-stop found; events acked).

## PLAN

- [ ] 0. Reality sync: fetch origin o90-planner-pr develop; record branch/base SHAs, PR 1056 check state + failed-log cause
- [ ] 1. New worktree /home/marcel/dev/baseharbor-ecosystem/worktrees/f45-rb from origin/o90-planner-pr
- [ ] 2. git rebase origin/develop; if conflicts: resolve hook so Akte paths reference the MERGED 24c33afd version (Akte check against real develop file)
- [ ] 3. Run focused tests in worktree
- [ ] 4. ONE push --force-with-lease origin HEAD:o90-planner-pr
- [ ] 5. Observe gh pr checks; record check table + remote head in evidence
- [ ] 6. END F45-1056REBASE-DONE with remote-head + check-tabelle

## Findings (pre-work)

- origin/develop = 24c33afd; origin/o90-planner-pr = 38ba06bb; merge-base = 24c33afd
  -> the requested linear rebase onto develop is a NO-OP: the branch is already rebased
  (01132e8c, 72431414, 38ba06bb sit directly on 24c33afd).
- PR 1056 fails only "PR Selected Checks":
  `BLOCKED: coverage-unresolved: CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py`
- develop's Akte (docs/internal/ci-binding-qualifications.md, merged via #1058 at 24c33afd) holds ONE entry anchored at
  reviewed_head=4e737e26 (the PRE-rebase branch tip; 4e737e26 is NOT an ancestor of 38ba06bb).
- core752_reviewed_merge_chain accepts ONLY: head==reviewed_head, or first-parent chains of CLEAN
  merge-tree commits whose second parents are in target history. Linear rebase commits (non-merges)
  can never reach 4e737e26; merge(4e737e26, 24c33afd) is conflicted (both sides edited pr-gate-plan.py /
  test_pr_gate_plan.py since 1861ed07) so merge-tree fails -> chain false. This is why "the rebase that
  o106/o107 did not achieve" cannot qualify via the existing register entry, conflict-resolution or not.
- pr-check-runner.py blob is identical at 4e737e26 and head; pr-gate-plan.py and test_pr_gate_plan.py differ
  (develop-side register code from #1058 + branch-side 38ba06bb multi-record hardening).

## Execution log

(to be filled during run)

## repo-rule compliance

- git invocations use absolute repo path /home/marcel/dev/baseharbor-ecosystem/baseharbor exclusively (pwd-PFLICHT).
- Isolated worktree only; no touched foreign working trees; single push --force-with-lease, no protection bypass.
- Truthful check reporting: no gate claimed green that is not.
