# Coordinator Status

**ACTIVE – resumed 2026-10-19 (Marcel release) with test-VM supplement**

## Test-VM supplement (clarification to the release, 2026-10-19)

- The earlier native-test restriction targeted needless GitHub-Actions runs, not
  Marcel's provided local VMs. Aliases verified BatchMode-only: `baha-docker`,
  `baha-podman`, `baha-k3s` (overview: `~/.config/codeman/test-targets.yaml`,
  local convention, must be explicitly read when needed).
- Delegation of targeted native tests to workers is permitted where the task
  genuinely requires a runtime. #995 native Podman recovery remains SOLE Qwen
  responsibility — no competing runs or resource touches.
- The supplement text received by the coordinator was cut off after "Dabei
  gelten"; standard protections (ownership-safe cleanup, no secrets in
  evidence, smallest gates) remain in force.

## Active assignment (post-resume)

- o5-998fix (74144db2): correcting #998 source per review 5569055023 on the
  existing branch; uncommitted corrections in worktree; scratch artifacts must
  not be committed.
- o1 (b70198fc): coverage binding rebased onto develop 19548bf0, committed
  `be82d726` locally, NOT pushed; placeholder head guarded until the corrected
  #998 SHA passes fresh independent review.
- o2 (ef07916b), o4 (19d72a4b): idle, retained.
- Prior state under the pause note below remains historical record.

## PAUSED – BaseHarbor CI stabilization (historical, 2026-10-19, ended by release above)

- No new claims, no new workers, no new implementation delegation, no CI reruns,
  no correction loops, no merges, no evidence deletion.
- Sessions kept alive idle at safe points: o1-core921 (b70198fc), o2-995fix
  (ef07916b), o4-993review (19d72a4b); coordinator w1-baseharbor (c3b6820f).
  Worker o3-985core754 completed and was removed after evidence (its branch/PR
  work persists on origin).
- Full lane state, SHAs, tests and blockers: `board-2026-10-19/mapping.json` +
  per-lane reports in this directory; authoritative records in baseharbor-plan
  task handoffs (planning main >= eaf0e04).
- Uncommitted work preserved intentionally: `worktrees/ci-core921-coverage-binding`
  (2 files, see o1-binding-pause-note.md) — do not clean, reset or stash.
