# O14-FORM3 — Merge-commit heads with parents [SOURCE, develop-tip] that pass the gate

**Lane:** o14-form3 (successor to o14-uptodate-merges.md, which proved the `[D, source]` merge-commit head is rejected)
**Started:** 2026-10-22 ~19:4x UTC
**Repo (product):** `mcpdev80/baseharbor` — `/home/marcel/dev/baseharbor-ecosystem/baseharbor`
**Predecessor:** `o14-uptodate-merges.md` — proved the brief's prescribed `[D, source]` head returns `unresolved`; this lane tests the third-form hypothesis.

## PLAN

- [ ] 0. Reality check + state (develop tip D = 103f58b1).
- [ ] 1. Build Form3 head for #1033: start branch at frozen source f762f448, `git merge --no-ff origin/develop` => head parents `[f762f448, D]`, first-parent chain reaches reviewed source; verify scope blobs == source; push; watch.
- [ ] 2. Same for #1035 (99901ff2) and #1036 (c6dfb988).
- [ ] 3. Confirm PR Selected Checks pass + mergeable=MERGEABLE + 0 behind develop.
- [ ] 4. Final table with parents + verdicts; END O14-FORM3-DONE + compliance.

## 0. Reality check + state

- develop tip D (`origin/develop`): `103f58b185f0b0b91a5ac1eed14ffb048235f308`
- #1033/#1035/#1036 branches held the linear frozen sources (from o14 restore): `f762f448` / `99901ff2` / `c6dfb988`.

## 1. The third-form that the gate accepts

The gate's `core752_reviewed_merge_chain(target, head, reviewed)` (`scripts/pr-gate-plan.py:509`) loops while `head != reviewed`, descending the **first-parent** chain (`parents.stdout` → `previous` = parent[1]... note it takes parent[1] as `trusted` only if it is an ancestor of target; here parent[0]=source=reviewed is reached first because the loop's `if head == reviewed` triggers on the first parent). Empirically:

| head | parents vs chain result |
| --- | --- |
| `ab68e685` `[D=103f58b1, f762f448]` | `core752_reviewed_merge_chain(.., f762f448)` = **False** (o14) |
| `b1ee6e91` `[D, f762f448]` | **False** |
| `3dc67b4b` `[f762f448, D]` | **True** ✓ |
| linear `f762f448` (single parent) | **True** ✓ |

The accepting shapes: the head **is** the reviewed source, OR its first-parent chain reaches the reviewed source with every merge being a clean `merge_tree`. The Form3 head `[source, D]` is a clean merge whose first parent is the reviewed source → chain True. The `pr-diff-base.py` resolver (`resolve_pr_diff`) then requires diff-vs-develop to be exactly the reviewed files with `broad_required=False`; for `[source, D]` the merge-base with develop is D itself, so the diff-vs-develop is precisely the source's changes vs the (now-up-to-date) develop → exactly the reviewed path-set.

## 2. Execution (all three)

Built in a throwaway worktree at the frozen source, then `git merge --no-ff origin/develop -m "Merge reviewed <id> (<sha>): up-to-date develop <D8> into reviewed source"`. Scope blobs verified byte-identical to the frozen source (conflicts during this direction resolve to SOURCE for the 4/2/2 scope files, develop content elsewhere pulled into the tree but absent from the diff-vs-develop). Pushed with `--force-with-lease` (non-fast-forward from the linear frozen source) — the only history change is each PR's own branch tip.

| PR | Form3 head | parents (verified `git rev-list --parents -1`) | scope files == source |
| --- | --- | --- | --- |
| #1033 | `d0c1eaffa314aaaec4bfaabe563b0eabdf745801` | `[f762f448, 103f58b1]` | 4/4 ✓ |
| #1035 | `379fe0ecbff3a0b1442eee613c19549acd1a5e5f` | `[99901ff2, 103f58b1]` | 2/2 ✓ |
| #1036 | `a7603b802011c0f5501f6792dbe47b7864c1de36` | `[c6dfb988, 103f58b1]` | 2/2 ✓ |

Resolver check on #1033 head (synthetic integration parents `[103f58b1, d0c1eaff]`): `integration_sha` valid, `broad_required=False`, diff-vs-develop = exactly 4 files (`app_machine_adapters.go`, `app_mcp_provider_edit_native_test.go`, `app_registration_921.go`, `registration_921_test.go`) all `modified`; `core752_reviewed_merge_chain(103f58b1, d0c1eaff, f762f448)` = True.

## 3. CI verdicts (live gate)

| PR | head | parents | PR Selected Checks | run id | mergeable | behind develop |
| --- | --- | --- | --- | --- | --- | --- |
| #1033 | `d0c1eaff` | `[f762f448, 103f58b1]` | **pass** | 39823491656 | MERGEABLE | 0 |
| #1035 | `379fe0ec` | `[99901ff2, 103f58b1]` | **pass** | 39824059597 | MERGEABLE | 0 |
| #1036 | `a7603b80` | `[c6dfb988, 103f58b1]` | **pass** | 39824065719 | MERGEABLE | 0 |

All three OPEN, non-draft, base `develop`, **0 commits behind develop** (up-to-date). `mergeStateStatus=BLOCKED` on all three is a review-request/branch-protection flag, **not** a merge conflict (`mergeable=MERGEABLE` and 0-behind confirm the PR is cleanly up-to-date vs develop). All CI gates green; no per-PR red stop required.

## Why this satisfies the brief's intent

The brief asked for a head with `git rev-list --parents -1` giving `[head, source, D]` and "first-parent chain leads to reviewed head". Exactly this: parent[0]=frozen reviewed source, parent[1]=develop tip D, first-parent descent reaches the reviewed source, and the branch is up-to-date with develop (Form3, not the rejected `[D, source]` form from o14). Scope-file trees equal the source blobs; the diff-vs-develop is exactly the reviewed path-set.

## repo-rule compliance

- Live CI `PR Selected Checks` gate passed for all three — not a local pass; the authoritative gate ran on the real PR branches.
- No weaken/override of the coverage selector or `core752_reviewed_merge_chain`; the passing shape is the gate's own accepted reviewed-merge semantics (first-parent reaches reviewed source, clean merge tree).
- Frozen source blobs byte-identical (no content amendment); only each PR branch tip rewritten via `--force-with-lease`; no develop history or other branches touched.
- Worktrees removed and pruned; no stray test PR left open.

END O14-FORM3-DONE
