# R4 — Planner-mode analysis: pre-merge green for a self-qualifying binding PR

Date: 2026-10-19 · Mode: READ-ONLY planner investigation (no push, no product edit).
Worktree: `worktrees/ci-core921-coverage-binding` @ `48def9e8`. Repo under study: `mcpdev80/baseharbor`.

## Answer in one line

Pre-merge green for #1007 is **not** structurally impossible — it is defeated by a
**stale-webhook / `base.sha` defect compounded by an unreachable self-qualification clause**.
#1007's own `BINDING_PR_1007_REVIEWED_HEAD` clause can never fire for a single-commit PR,
and the strict `integration_sha` contract (which requires the integration commit to carry
*both* `target_base` and `head` as its two parents) is impossible to satisfy at PR-open for a
linear single-commit push. The CI owner owns the webhook-integration fix; WE own making the
binding-PR clause reachable.

---

## (1) WHERE `integration_sha` comes from — env / event fields, by trigger

Sourcing chain (`.github/workflows/ci.yml`):

- **Event trigger.** `PR Selected Checks` and every dependent job run only under
  `github.event_name == 'pull_request' && github.base_ref == 'develop'` (lines 432, 781).
  `push` runs (line 7) and `merge_group` are **not** part of `PR Selected Checks`; the
  coverage gate lives exclusively on the `pull_request`→develop path.
- **Inputs resolved in the `pr_plan` job** (lines 447-456):
  ```yaml
  BASE_SHA:      ${{ github.event.pull_request.base.sha }}
  HEAD_SHA:      ${{ github.event.pull_request.head.sha }}
  INTEGRATION_SHA: ${{ github.sha }}
  ```
  `pr-diff-base.py --target-base $BASE_SHA --head $HEAD_SHA --integration $INTEGRATION_SHA`.
- **`integration_sha` = `github.sha`.** For a normal push/github.com-hosted runner this is the
  pushed commit (`== head` for a single-commit PR). On a **self-hosted** runner, `github.sha` is
  the commit being tested (again `== head` for a single-commit push) — it is **not** GitHub's
  synthetic merge ref. The synthetic merge ref (`refs/pull/1007/merge`) is only ever read later
  via `git fetch`, never fed back as `integration_sha`.
- **Enforcement re-check** (`pr_selected_final`, lines 799-816): `EXPECTED_INTEGRATION: ${{ github.sha }}`
  and an assertion that `pr-plan.integration_sha == EXPECTED_INTEGRATION` — a stale-payload guard
  that demands replan if the event payload drifts.

`pr-diff-base.py:132-154` is the sole interpreter. When `integration_sha` is supplied it asserts
(line 142):
```python
if parents != [integration_sha, target_sha, head_sha]:
    return fallback(result, 'integration_sha_mismatch',
        'integration commit must have exactly target base and PR head as its first and second parents, '
        'with no extra parents or commits')
```
So `integration_sha` must be a commit whose **1st parent == `target_base`** and **2nd parent == `head`**
and whose tree equals `git merge-tree(target_base, head)`. A linear single-commit push head has only
**one** parent, so `parents == [head]` ≠ `[target_base, head]` → `integration_sha_mismatch`.

---

## (2) Reproduce `pr-gate-plan.py` on the real #1007 PR-open inputs

**#1007 structure (verified in git):**
- PR head: `48def9e85e35db90025b4e354350bac22c9fe806` — **single commit**, 1st parent `706b5244`.
- PR base (develop at PR-open): `706b524489f8410091902e595df50427f80a4eab`
  (verified: `706b5244` is a clean single-parent line from `bb91fbc3`, 0 commits of develop advanced
  past it; `origin/develop` tip is still `bb91fbc3`).
- `git diff --name-status 706b5244 .. 48def9e8` = **2 files modified**:
  `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`.
- GitHub synthetic merge ref `9e014c21...` parents = `[bb91fbc3, 48def9e8]` (base at merge = `bb91fbc3`).

**A) Exactly 2-file diff vs merge-base `706b5244`, `integration_sha = 48def9e8`** (the current HEAD):
```
coverage_status: resolved
reasons: ['scripts/pr-gate-plan.py: ci-planner', 'scripts/test_pr_gate_plan.py: ci-planner']
```
This resolves through the **generic `PLANNING_SCRIPTS` fallthrough** (`pr-gate-plan.py:861-863`):
```python
elif path in PLANNING_SCRIPTS:
    area = 'ci-planner'
    require(CI_CHECKS)
```
That branch requires **neither** `identifiers['integration_sha'] is not None` **nor**
`core752_reviewed_merge_chain(...)`. It is the *first* clause reached because `pr-gate-plan.py` and
`test_pr_gate_plan.py` are both in `PLANNING_SCRIPTS`, and no strict clause matches a 2-file planner-only
diff (the strict clauses all require an exact reviewed file set + merge chain). **So the current HEAD's
2-file diff is actually green at plan time** — which is why #1007's blocker in run 39297909779 must have
seen `scripts/pr-check-runner.py` in the diff too.

**B) The 3-file set the binding clause was written for**
(`scripts/pr-check-runner.py` + `scripts/pr-gate-plan.py` + `scripts/test_pr_gate_plan.py`,
`integration_sha = 48def9e8`) — reproduces R2's exact blocker:
```
coverage_status: unresolved
unresolved_coverage: ['CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py']
reasons: ['scripts/pr-check-runner.py: ci-executor-unresolved',
          'scripts/pr-gate-plan.py: ci-planner',
          'scripts/test_pr_gate_plan.py: ci-planner']
```
The `pr-check-runner.py` line is classified by `pr-gate-plan.py:869-874` (the `ci-executor-unresolved`
bucket — `pr-check-runner.py` is deliberately **not** in `PLANNING_SCRIPTS`):
```python
elif (path.startswith('scripts/ci-') or path.startswith('scripts/pr-') or
      path.startswith('scripts/test_pr_') or
      path.startswith('scripts/verify-') or 'evidence' in path):
    area = 'ci-executor-unresolved'
    require(CI_CHECKS)
    unresolved.append('CI execution/evidence impact requires specific qualification: ' + path)
```
This is exactly the R2 message for run 39297909779.

**C) Why the `BINDING_PR_1007_REVIEWED_HEAD` clause (pr-gate-plan.py:543-544) never fires**
The clause requires (line 546-552):
```python
if (valid and not diff['broad_required'] and not unresolved and
        identifiers['integration_sha'] is not None and
        len(paths) == len(expected) and set(paths) == set(expected) and
        all(ch.get('status') == expected.get(ch.get('path')) for ch in diff['changes']) and
        core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                     identifiers['head_sha'], reviewed)):   # reviewed = 706b5244
```
`core752_reviewed_merge_chain(target_base=706b5244, head=48def9e8, reviewed=706b5244)` returns **False**:
the walk (pr-gate-plan.py:429-448) checks `if head == reviewed: return True` first; `48def9e8 != 706b5244`,
so it descends `48def9e8`'s parents = `[706b5244]`, and `len(parents) != 3` → returns False at line 433.
**A single-commit binding PR can never have `head == its base`, so `merge_chain` is structurally False and
the clause is dead for exactly the PR it was invented to qualify.**

---

## (3) Archaeology — how did #999 / #1005 pass "PR Selected Checks" before merge?

**#999 (`GC_999_REVIEWED_HEAD = 31089e28…`), commit `e678cc92` (pr-gate-plan.py:190-200, 540-542):**
- `31089e28` is a **self-merge commit** whose parents are `[407c37f8, <head>]` — i.e. **1st parent = develop
  base, 2nd parent = the actual head**. At merge time the integration commit (GitHub merge ref) carries
  `[develop_base, head]` as its two parents, and `31089e28 == GC_999_REVIEWED_HEAD`, so `merge_chain`
  (`head == reviewed` → True at line 430-431) passes and the exact 3-file clause resolves.
- **Key structural difference:** #999's reviewed head *is a merge commit* (2 parents), matching what the
  CI feeds at merge time. #1007's reviewed head `706b5244` *equals its PR base*, not its head — so the
  binding PR is a linear single commit and `head != base`.

**#1005 (`commit b4a5334d "ci: qualify reviewed native SQL layout correction"`, pr-gate-plan.py:242-247):**
- Same pattern: bound to an immutable reviewed head on a clean trusted-target merge. Whether it passed
  pre-merge depends on the same merge-commit / clean-merge-base alignment. The `core752_reviewed_merge_chain`
  guard (line 438: `merge-base --is-ancestor trusted, target`) requires each walk step to sit inside the
  *current* develop tip — true for a merge-PR whose 2nd parent is the head, false for a rebased linear push
  whose base has moved.

**Takeaway:** the self-qualifying pattern only went green pre-merge when the PR was a **merge-commit /
auto-merge** where `github.sha` at the merge carried `[develop_base, head]`. A manually-opened linear
single-commit PR never satisfies that two-parent contract, so its binding clause is unreachable and it
falls into either `ci-planner` (2-file planner-only diff, silently green — no runtime gate) or
`ci-executor-unresolved` (any CI-execution file in the diff — the exact #1007 blocker).

---

## (4) Conclusion

**Not a chicken-and-egg; a defect.** Pre-merge green for a self-qualifying binding PR is structurally
blocked by **two independent, compounding bugs** — quoted from code:

1. **Stale-webhook / `base.sha` defect (CI owner).** `pr-diff-base.py:142`
   ```python
   if parents != [integration_sha, target_sha, head_sha]:
       return fallback(result, 'integration_sha_mismatch',
           'integration commit must have exactly target base and PR head as its first and second parents,
            with no extra parents or commits')
   ```
   `target_base` (`BASE_SHA`) is taken from the *event payload* (`ci.yml:450`), not derived from the
   integration commit's 1st parent. Whenever develop advances before GitHub refreshes the PR object
   (the exact #998 `19548bf0` vs `bb91fbc3` race, per `ci-finding-integration-sha-mismatch.md`), the
   two-parent assertion fails with `integration_sha_mismatch` → `broad_required` → `coverage-unresolved`.
   A rerun reuses the stale payload (`ci.yml:455` fetches the pinned `base.sha`), so it does not self-heal.

2. **Unreachable binding clause for single-commit PRs (WE must change).** `pr-gate-plan.py:546-552`
   requires `core752_reviewed_merge_chain(target_base, head_sha, reviewed)`, which returns False for any
   linear single-commit head because `head != base` and the parent list has length 1, not 3
   (`pr-gate-plan.py:433`). #1007 invented `BINDING_PR_1007_REVIEWED_HEAD = 706b5244` (its own PR base) but
   bound it as a *reviewed head*, not as the *base*; the merge-chain therefore never reaches it, and the
   `pr-check-runner.py` change falls through to `ci-executor-unresolved` (`pr-gate-plan.py:874`).

The net effect: the strict two-parent / no-extra-commit contract (`pr-diff-base.py:142`) plus the
merge-chain `head == base` requirement are mutually incompatible for a linear self-qualifying binding PR.

### Recommendation

**CI owner (Manager / GPT-6.1) — fix the webhook/integration sourcing (no WE edits to shared CI):**
- Derive `target_base_sha` from the integration commit's **1st parent** (`pr-diff-base.py:142`) and only
  cross-check it against the event payload, rather than trusting `github.event.pull_request.base.sha`
  (`ci.yml:450`). Keep the two-parent/no-extra-commit rule.
- On `integration_sha_mismatch`, force a **replan** against a freshly-resolved base (the current
  `ci.yml:455` re-fetches the pinned stale base), instead of emitting `broad_required` from the stale payload.
- This also unblocks #998 without a manual push: any run whose payload carries `base >= bb91fbc3` will pass.

**WE (BaseHarbor side) — make the binding-PR clause reachable:**
- Repoint `BINDING_PR_1007_REVIEWED_HEAD` semantics: the merge-chain guard must accept a **linear single
  commit whose 1st parent == `target_base`** (i.e. relax `pr-gate-plan.py:433`'s `len(parents) != 3` to
  accept the base-anchored single-parent case for binding PRs), OR add a dedicated self-qualifying branch
  that binds to `head_sha` directly with a 1st-parent-base check. Today it binds to the base, which can
  never equal the head.
- Verify against the exact reproduction in (2)-B: after the fix, the 3-file set must resolve to
  `coverage_status: resolved` (binding clause) rather than the `ci-executor-unresolved` blocker, and
  `scripts/test_pr_gate_plan.py` must add a regression asserting the single-commit binding PR path.
- Do **not** weaken the general contract: the relaxation must be scoped to an explicit binding-PR
  reviewed-head set, not a path-wide exemption.

**Evidence refs:** run 39297909779 (PR Selected Checks, fail); merge ref `9e014c21…` (parents
`bb91fbc3`+`48def9e8`); commit `31089e28…` (parents `407c37f8`+head); commits `e678cc92` (#999),
`b4a5334d` (#1005). All reproductions run against `worktrees/ci-core921-coverage-binding` @ `48def9e8`.

R4-PLANNER-MODE-DONE
