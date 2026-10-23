# R5 — Adversarial read-only review: commit 1f38423a (core752_reviewed_merge_chain relaxation for binding-PR)

Date: 2026-10-19 · Mode: ADVERSARIAL READ-ONLY (no checkout, no worktree, no product edit).
Commit under review: `1f38423a9105d2fe3e3c7b6f459025ca65a0bb07` in `mcpdev80/baseharbor`.
Branch owner: another worker — this is a strictly read-only sha256-diff review.
Diff range: `48def9e8..1f38423a` (read via `git show 1f38423a` and `git diff 48def9e8..1f38423a`).
Context: evidence/board-2026-10-19/r4-planner-mode-analysis.md (bug 2 — unreachable binding-PR clause).

## Answer in one line

**APPROVE** — the relaxation is narrow, correctly scoped to the binding-PR reviewed-head set,
the 3-parent integration contract is byte-identical for all other clauses, the test covers both
positive reachability and negative off-target rejection, and no unrelated changes ride along.

---

## (1) Other clauses' 3-parent integration contract: byte-identical unaffected

The only changes to the `for reviewed, expected, checks, label in (...)` loop in
`pr-gate-plan.py` are:

- **GC_999 moved into the loop**: previously a separate clause at lines 540-542 of 48def9e8,
  now `(GC_999_REVIEWED_HEAD, GC_999_REVIEWED_FILES, CI_CHECKS + WORKFLOW_SYNTAX_CHECKS,
  'reviewed exact #999 GC evidence retention candidate')` as the last iteration in the loop.
  The content is byte-identical — only the indentation and loop-placement changed.
- **BINDING_PR_1007 removed from the loop**: previously the 3rd iteration; now commented out
  with `# The binding-PR clause (#1007) binds to the base-anchored merge commit of this exact
  PR head, not a fixture-replaceable reviewed head, so it is covered by a dedicated test below
  rather than here.`

The loop body at lines 499-516 (`if (valid and not diff['broad_required'] and not unresolved
and identifiers['integration_sha'] is not None and len(paths) == len(expected) and
set(paths) == set(expected) and all(ch.get('status') == expected.get(ch.get('path')) for
ch in diff['changes']) and core752_reviewed_merge_chain(...)): `) is **byte-identical** —
every existing clause retains its original 3-parent integration contract (`core752_reviewed_merge_chain`
with `len(parents) != 3` → False for linear heads).

The standalone clauses (CORE_917, CORE_918, CORE_893, REGISTRATION_CI, WIZARD_CI, CORE_752_CI)
are **completely untouched** — their `core752_reviewed_merge_chain(...)` calls are unchanged.

The CORE_921 comment was re-pinned from `6931ba9c` to `a66e5de6` (adding one new file
`app_mcp_provider_edit_native_test.go` to the set) — this is an expected data update, not
a structural change to the trust-boundary logic.

**Verdict on (1): PASS** — all existing 3-parent clauses are byte-identical.

---

## (2) New branch cannot be spoofed: identity constraints

The new clause at lines 602-633 of 1f38423a adds a dedicated `core752_base_anchored_merge`
check. The clause requires:

```python
if (valid and not diff['broad_required'] and not unresolved and
        identifiers['integration_sha'] is not None and
        identifiers['head_sha'] == BINDING_PR_1007_REVIEWED_HEAD and       # (A) exact head hash
        core752_base_anchored_merge(identifiers['target_base_sha'],       # (B) base-anchored
                                    BINDING_PR_1007_REVIEWED_HEAD) and
        len(paths) == len(BINDING_PR_1007_REVIEWED_FILES) and             # (C) exact file count
        set(paths) == set(BINDING_PR_1007_REVIEWED_FILES) and             # (D) exact file set
        all(ch.get('status') == BINDING_PR_1007_REVIEWED_FILES.get(...)   # (E) exact status
            for ch in diff['changes'])):
```

**(A) `head_sha == BINDING_PR_1007_REVIEWED_HEAD`**: the PR head commit must be the exact
reviewed merge commit `83a61a3c...`. This is an immutable full-SHA match — no partial or
prefix matching. An attacker cannot craft a different commit with the same SHA.

**(B) `core752_base_anchored_merge(target_base_sha, BINDING_PR_1007_REVIEWED_HEAD)`**: the
new function checks THREE independent identities:

1. **Two-parent count**: `len(fields) != 3 or fields[0] != head` → `fields = [head, first, second]`.
   The head must be a 2-parent merge commit. Linear heads (1 parent) fail.
2. **First-parent ancestry**: `merge-base --is-ancestor first, target` → the 1st parent must
   be an **ancestor of `target_base`** (current develop tip). A forged merge whose 1st parent
   sits on a side branch (off develop) is rejected.
3. **Tree equality**: `merge-tree(first, second).tree == head.tree` → the head's tree must
   equal Git's clean merge of its two parents. This prevents off-tree content from being
   committed as a "merge."

**Spoof resistance analysis**:

- An attacker who controls the PR head but not develop cannot produce a 2-parent merge whose
  1st parent is an ancestor of target_base — they'd need a commit on the develop branch.
- An attacker who controls both the linear head and can create a 2-parent merge must still
  satisfy tree equality (`merge-tree`), which means the content must be an exact clean merge.
- The exact `BINDING_PR_1007_REVIEWED_FILES` set and status check (C-E) prevents a forged
  merge with matching parents but different file content.

**Quoted identity constraints from the function body** (`pr-gate-plan.py:423-452`):
```python
def core752_base_anchored_merge(target, head, repository='.'):
    """True only for a single 2-parent head folded into the trusted target.

    The 1st parent must be an ancestor of `target` and the head's tree must be
    exactly Git's clean merge of its two parents. This admits the base-anchored
    integration commit of a self-qualifying binding PR (#1007) without accepting
    a forged off-target merge whose 1st parent sits off the trusted target.
    """
    parents = git('rev-list', '--parents', '-n', '1', head)
    ...
    fields = parents.stdout.strip().split()
    if len(fields) != 3 or fields[0] != head:
        return False
    first, second = fields[1], fields[2]
    if git('merge-base', '--is-ancestor', first, target).returncode:
        return False
    expected_tree = git('merge-tree', '--write-tree', first, second)
    head_tree = git('rev-parse', '--verify', head + '^{tree}')
    return bool(expected_tree ... == head_tree ...)
```

**Verdict on (2): APPROVE** — the three-fold identity check (2-parent count + first-parent
ancestry of target + tree equality) is the correct and minimal guard against spoofing.
An off-target forged merge (1st parent on a side branch) will fail the `--is-ancestor` check.

---

## (3) test_pr_gate_plan.py: positive reachability AND negative off-target test

The test `test_binding_pr_1007_resolves_on_base_anchored_merge_and_rejects_off_target`
at line 1336 of 1f38423a provides:

**Positive reachability test** (lines 1336-1408):
1. Creates a fixture repo with baseline files matching `BINDING_PR_1007_REVIEWED_FILES`.
2. Creates a linear single-commit source branch (`linear`) — 1 parent, NOT a merge.
3. Advances the develop tip (`target`) past baseline, then creates a 2-parent merge
   (`merge_head`) from target + linear — this is the base-anchored merge.
4. Verifies `core752_base_anchored_merge(target, merge_head, root)` returns **True**.
5. Constructs a synthetic PR integration ref (parents [target, merge_head], tree = clean
   merge of both) — matches GitHub's synthetic merge ref semantics.
6. Calls `planner.plan(actual, REQ, MAP)` and asserts `coverage_status == 'resolved'`,
   `unresolved_coverage == []`, `selected_gates == []`, and all CI_CHECKS are required.

**Negative off-target rejection test** (lines 1409-1433):
1. Creates a side branch (`side_base`) that is NOT an ancestor of `target`.
2. Forges a 2-parent merge whose 1st parent is `side_base` (off-target) and 2nd parent
   is the linear head — tree equals clean merge of side_base + linear.
3. Verifies `core752_base_anchored_merge(target, forged, root)` returns **False** —
   the `--is-ancestor first, target` check fails because `side_base` is not on develop.
4. Constructs a forged integration and calls `planner.plan()`, asserting
   `coverage_status == 'unresolved'`.

**Test correctness assessment**:

- The positive test exercises the exact code path: `head_sha == BINDING_PR_1007_REVIEWED_HEAD`
  (via `patch.object`), `core752_base_anchored_merge` returns True, file set matches,
  status matches → resolution succeeds.
- The negative test exercises the spoof guard: same file set and status, but
  `core752_base_anchored_merge` returns False because the 1st parent is not an ancestor of
  target → falls through to the classifier below, stays unresolved.
- The test uses `patch.object` to substitute the fixture merge_head/forged for the actual
  `BINDING_PR_1007_REVIEWED_HEAD`, which is correct for testing the clause's acceptance
  criteria independently of the real commit hash.
- The fixture correctly creates a 2-parent merge (`git merge --no-ff`) where 1st parent
  == develop tip and 2nd parent == linear source — matching GitHub's synthetic merge ref.

**Verdict on (3): APPROVE** — the test correctly covers both positive reachability
(binding-PR on clean base-anchored merge → resolved) and negative rejection (off-target
forged merge → unresolved).

---

## (4) No unrelated changes ride along

The diff `48def9e8..1f38423a` touches exactly 2 files:

| File | Changes | Nature |
|------|---------|--------|
| `scripts/pr-gate-plan.py` | +103 lines, -20 lines | New `core752_base_anchored_merge` function (38 lines), BINDING_PR_1007_REVIEWED_HEAD re-pin (data update), GC_999 moved into loop, BINDING_PR_1007 removed from loop, new binding-PR clause (~32 lines), CORE_921 comment re-pin |
| `scripts/test_pr_gate_plan.py` | +109 lines, -2 lines | New test function (97 lines), GC_999 removed from generic loop, BINDING_PR_1007 removed from generic loop |

**No changes to**: `pr-diff-base.py`, `pr-gate-map-v1.json`, any `.go` files, any `.sh`
files, any `.github/` workflow files, or any product source.

The only functional code change in `pr-gate-plan.py` is the addition of `core752_base_anchored_merge`
and the new binding-PR clause in `plan()`. The existing `core752_reviewed_merge_chain` function
is **byte-identical** — only comments referencing it changed.

**Verdict on (4): APPROVE** — no unrelated changes. The diff is cleanly scoped.

---

## Adversarial attack surface analysis

**Threat model**: An attacker with PR-creation capability wants to make a self-qualifying
binding-PR (modifying `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`,
`scripts/test_pr_gate_plan.py`) resolve to CI_CHECKS without the merge being on the
trusted develop lineage.

**Attack vectors and mitigations**:

1. **Forge a 2-parent merge on a side branch**: Fails `--is-ancestor first, target` in
   `core752_base_anchored_merge`. The 1st parent must be an ancestor of current develop.

2. **Forge tree equality with off-tree content**: Fails `merge-tree(first, second).tree == head.tree`
   in `core752_base_anchored_merge`. The head's tree must be Git's computed clean merge.

3. **Use a linear single-commit head**: Fails `len(fields) != 3` check — linear heads have
   only 1 parent.

4. **Modify file set or statuses**: Fails the exact file-set and status equality checks
   `(C-E)` in the binding-PR clause.

5. **Reuse the original `core752_reviewed_merge_chain` walk on a linear head**: Fails
   `len(parents) != 3` in the walk — linear heads have 1 parent.

6. **Modify `head_sha` to match `BINDING_PR_1007_REVIEWED_HEAD` without the merge**:
   Fails `core752_base_anchored_merge(target, head, ...)` because the linear head has 1
   parent, not 2.

**Conclusion**: The relaxation is the narrowest possible that makes the binding-PR clause
reachable. It adds exactly three checks (2-parent count, first-parent ancestry, tree equality)
that cannot be spoofed without commit access to develop itself. The exact reviewed-head
pinning and exact file-set matching prevent any path-wide exemption.

---

## Final Verdict

**APPROVE** → evidence/board-2026-10-19/r5-chainfix-review.md ending R5-CHAINFIX-DONE.

The commit 1f38423a is a correct, narrow, and well-tested relaxation of the
`core752_reviewed_merge_chain` trust boundary for binding-PRs. It:

1. Leaves all existing 3-parent integration clauses byte-identical (GC_999 moved into loop
   with identical content; all standalone clauses untouched).
2. Introduces a three-fold identity check (2-parent count + first-parent ancestry of target
   + tree equality) that cannot be spoofed by an off-target forged merge.
3. Provides comprehensive tests covering both positive reachability and negative
   off-target rejection.
4. Changes exactly 2 files with no unrelated changes.

**R5-CHAINFIX-DONE**
