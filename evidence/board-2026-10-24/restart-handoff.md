# Coordinator Restart Handoff – 2026-10-24 13:48 UTC

## Warum Restart sinnvoll ist
- tmpfs/RAM war kritisch: earlier `MemAvailable=1077MB`, `/tmp` ~4.5Gi used.
- current `tmp-sweep.sh 3` reduced `/tmp` to ~2.3Gi and `MemAvailable` is ~6.2Gi; a restart is cleaner for leaked sockets/processes and the buggy systemd/tmp pressure.
- no active product implementation is mid-write after q122 push; only CI is running and can survive a reboot independently.

## Persisted authoritative state
### Remote branches / PRs
- baseharbor develop: `65aa063e90767c85af976081c100be1817370e67`
- #1052: `9f715184`, OPEN, BEHIND; local worktree clean at same head.
- #1056 P: `b0abe105`, OPEN/BLOCKED after CI success; Codex REQUEST_CHANGES (F1/F2) – correction not started.
- #1060: fast-forwarded at 13:45:26Z from old `13a05ea4` to tested repair `0f3bd169d4a02c5f019da822b4f45f66b972423e`.
  - CI run: `40099514587`; observed Workflow policy pass, selected checks still in progress before this handoff.
  - q122 may have been killed mid evidence; remote state is durable.
- #1063/Q: MERGED, develop `65aa...`.
- #1046: `1ec67f8d`, OPEN; exact-anchor qualifier serialized after #1056/#1060.

### Local git/worktrees to preserve
- `/home/marcel/dev/baseharbor-ecosystem/q114-fix1060`
  - branch `q114-fix1060`, HEAD `0f3bd169`; q120 WIP in `stash@{0}: On q114-fix1060: q120-wip`.
  - DO NOT drop stash or force-push.
- `/home/marcel/dev/baseharbor-ecosystem/worktrees/q113-1052`
  - branch `work/v0.4.26-openbao-retry`, HEAD `9f715184`, clean vs remote #1052.
- `/home/marcel/dev/baseharbor-ecosystem/baseharbor`
  - root product checkout intentionally left dirty: branch `o124-Q-record`, HEAD `7170c1b6`, modified go.mod/go.sum.
  - Codex cmf14 recommends preserve Option1 until owner decides; do not reset/delete.

## Resume procedure after reboot
1. tmux/codeman services (RUNBOOK paths).
2. `python3 evidence/board-2026-10-19/scripts/basics-check.py`
3. `timeout 30 gh pr view 1060 --repo mcpdev80/baseharbor --json headRefOid,mergeStateStatus,statusCheckRollup`
   - expect head `0f3bd169...`.
4. `timeout 30 gh pr view 1052 --repo mcpdev80/baseharbor --json headRefOid,mergeStateStatus,statusCheckRollup`
   - expect head `9f715184...`.
5. Read:
   - `evidence/board-2026-10-24/codex-verdict-1060-history.md`
   - `evidence/board-2026-10-24/codex-verdict-1052-r5.md`
   - `evidence/board-2026-10-24/q121-1060repair.md`
   - `evidence/board-2026-10-24/q122-1060ff.md`
   - `evidence/board-2026-10-22/coordinator-plan.md` PLAN section.
6. Start coordinator/worker pool per RUNBOOK.
7. Next lane after #1060 CI:
   - if #1060 exact-H CI green: Codex independent review/merge #1060, then #1052 integration review, then serialized #1056 correction and #1046 qualifier.
   - if red: one worker correction per failing check, no force/rerun spam.

## Active at handoff
- Codex lane fc55 idle (cmf17 advice done, no running action).
- q122 was only verifying PR checks after successful push.
- Worker sessions were rotated after completion; their evidence/local worktrees remain.
END RESTART-HANDOFF-PREPARED
