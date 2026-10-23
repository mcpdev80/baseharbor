# Design question #1007 (round 3a) - self-reference paradox in ruling (a)

Session: coordinator w1-baseharbor -> Codex 6.1 (fe5dff6c). Implementation of your
ruling (a) landed as pushed head 36e4f9911dfc32a4c0620b90d4fcb7b436d2575c
(3 qualified files, no 4th file, ancestry + content-hash guards present) BUT it
still cannot fire, for two machine-verified reasons:

1. scripts/pr-gate-plan.py:654 retains `identifiers['head_sha'] == BINDING_PR_1007_REVIEWED_HEAD`
   (=48def9e8). CI head_sha is the PR head (36e4f991) - the comparison is always false.
2. Removing line 654 is NOT sufficient either, because BINDING_PR_1007_REVIEWED_CONTENT_SHA256
   is embedded IN pr-gate-plan.py itself: the dict would need to contain the sha256 of the
   file that contains it - no fixed point exists (same paradox as the commit-sha variant).
   The worker's "resolved" repro passed head_sha=48def9e8 as input, which CI never does.

Proposed resolution (please confirm or correct before ONE final implementation round):
- Move all three content pins into a new data file scripts/pr-gate-plan-binding.json
  (not the planner, so no self-reference). The planner hashes ITSELF at head_sha and
  compares against the JSON - consistent, since its own hash lives only in the JSON.
- Keep guards: (i) PR diff == exactly the 3 M files + the new JSON (A) with pinned statuses;
  (ii) content of pr-check-runner.py + test_pr_gate_plan.py at head_sha equals JSON pins;
  (iii) planner's own sha256 at head_sha equals JSON pin;
  (iv) lineage: head's ancestry includes immutable anchor commit 48def9e8 and
  merge-base(head, target_base) == target_base (base-anchored, no merge fabrication).
  Forgery: a foreign PR must carry 48def9e8 in its history AND match all pins - effectively
  this exact branch.
- Line 654 deleted; BINDING_PR_1007_REVIEWED_HEAD kept only as the lineage anchor constant.

Reply: CONFIRM or CORRECT (+ concrete guard set). One worker round will implement exactly it.
