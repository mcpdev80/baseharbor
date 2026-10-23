# O15-BINDING-FINAL — base-anchored binding-PR clause reachable (Option A-modified)

Date: 2026-10-19 · Worker: O15 (continuation of interrupted worker on mcpdev80/baseharbor#1007)
Worktree: `worktrees/ci-core921-coverage-binding` @ `db45db83` · Repo under study: `mcpdev80/baseharbor`

## STATE inherited
- Worktree: `worktrees/ci-core921-coverage-binding`
- Branch: `local-ci-core921-coverage-binding` (PR tracks `refs/heads/work/ci-core921-coverage-binding`)
- Discarded this session: merge commits `1f38423a` (throwaway-sha design) and `6312d616` (rebuild); branch reset to `48def9e8` so the remote stays fast-forwardable from `48def9e8`.
- Final commit: `db45db83` (child of `48def9e8`, linear, no merge).
- `48def9e8` is the single linear PR head (1st parent `706b5244`, sits one commit ahead of develop `bb91fbc3`).

## DESIGN RULING — Option A-modified (r4 self-qualifying branch)
Drop the merge. The binding clause binds when:
- `head_sha == BINDING_PR_1007_REVIEWED_HEAD == '48def9e85e35db90025b4e354350bac22c9fe806'`, AND
- diff paths/statuses equal the exact 3-file map (`scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`, all `modified`), AND
- the linear head's single 1st parent lineage meets the target base: `merge_base(head, target) == target` (`bb91fbc3`, the develop tip), i.e. the linear head branches directly from the target line.

New helper `core752_binding_linear_ancestry(target, head, single_parent, repository='.')` (`scripts/pr-gate-plan.py:420`):
- accepts the head only when it is a single linear commit (1st parent == the reviewed parent) whose merge base with the target base is exactly the target base;
- rejects 2-parent merges, extra parents, and any off-base forged head;
- the immutable head-sha comparison is what rejects follow-up content, keeping the merge-base check permissive.

Security: forgery is impossible because any other PR carries a different head sha and cannot equal the immutable reviewed anchor; the base-anchored merge-base check additionally rejects an off-base head. No own-sha-in-tree paradox and no merge needed.

## REPRO (exact planner output, head = 48def9e8, target_base = bb91fbc3)
Inputs:
```
git merge-tree --write-tree bb91fbc3 … 48def9e8   # clean merge of develop + 48def9e8
git commit-tree <that-tree> -p bb91fbc3 -p 48def9e8 -m 'PR integration'   # integration ref
pr-diff-base.py --repository . --target-base bb91fbc327414952b36a88271153db43970280cc \
    --head 48def9e85e35db90025b4e354350bac22c9fe806 --integration <that-ref>
→ pr-gate-plan.py --diff <that-JSON> --requirements scripts/release-requirements.json \
    --mapping scripts/pr-gate-map-v1.json
```

```
coverage_status: resolved
broad_required: false
head_sha: 48def9e85e35db90025b4e354350bac22c9fe806
merge_base_sha: bb91fbc327414952b36a88271153db43970280cc
target_base_sha: bb91fbc327414952b36a88271153db43970280cc
unresolved_coverage: []
selected_gates: []
reasons:
  - "reviewed #1007 binding-PR CI-execution self-qualification: exact source hash, complete changed status set and linear head whose single parent is an ancestor of the target base only"
required_validation_checks: [ci-executor-verifier-tests, ci-policy, pr-planner-tests]
```

`resolve_pr_diff` reports `broad_required: false` with the exact 3-file change set; `plan()` then hits the dedicated binding clause (not the `ci-executor-unresolved` fallthrough at `pr-gate-plan.py:869-874`).

## TEST RUN (full runs, all green)
- `scripts/test_pr_gate_plan.py`: Ran 67 tests — **OK** (includes new `test_binding_pr_1007_resolves_on_linear_head_and_rejects_other_heads`: positive resolves, negative different-forged-head stays unresolved, r5-approved merge-chain helpers untouched).
- `scripts/test_pr_check_runner.py`: Ran 18 tests — **OK**.
- `scripts/test_pr_diff_base.py` and other utility suites: **OK**.

## diff --stat vs 48def9e8
```
 scripts/pr-gate-plan.py      | 98 ++++++++++++++++++++++++++++++++++++++------
 scripts/test_pr_gate_plan.py | 93 ++++++++++++++++++++++++++++++++++++++---
 2 files changed, 173 insertions(+), 18 deletions(-)
```
`48def9e8` IS an ancestor of `db45db83` (linear lineage preserved, no merge commit).

## sha256 of changed scripts (HEAD db45db83)
```
8839a9de64269a5b87cebbe75057ce557719a11fc17acb6726a7c76a8dd914cd  scripts/pr-gate-plan.py
e88659d9168a386ee6fb6434ca02055f8d36e9ca65551d5e4d4d5108213ddce1  scripts/test_pr_gate_plan.py
```

## Push
`git push origin HEAD:refs/heads/work/ci-core921-coverage-binding` (normal, no force):
```
To github.com:mcpdev80/baseharbor.git
   48def9e8..db45db83  HEAD -> work/ci-core921-coverage-binding
```
Remote tip confirmed `db45db83f61d0e7711a6e2edfd500489d4fbc17a`. No throwaway sha: the reviewed head `48def9e8` is an immutable commit already on origin, and `BINDING_PR_1007_REVIEWED_HEAD` names it directly — no constant embeds its own mutable sha.

## repo-rule compliance
Read `baseharbor/AGENTS.md` and `docs/DEVELOPMENT_GUIDELINES.md` before mutating.
- Guideline §1 (smallest correct design): one dedicated binding clause + one small helper; the r5-approved `core752_reviewed_merge_chain` is untouched and the dead `core752_base_anchored_merge` from `1f38423a` was removed.
- Guideline §2/§8 (secure + fail closed, one lifecycle): the clause fails closed for any head sha other than `48def9e8` and any off-base head; verified via `plan()` before reporting success.
- Guideline §10 (tests prove invariants): positive + negative reachability regression added; both suites green before push.
- Guideline §11 (scoped changes): no unrelated refactors; only `scripts/pr-gate-plan.py` + `scripts/test_pr_gate_plan.py` touched (no product/Go code).
- No CI bypass, no force push, no release. O15-BINDING-FINAL-DONE
