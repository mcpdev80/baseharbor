## Review PR #1007 (binding) — O13, independent read-only

- **PR head (verified, detached):** `706b524489f8410091902e595df50427f80a4eab`
  fetched via `git fetch origin`, `git checkout --detach 706b5244` in
  `worktrees/ci-core921-coverage-binding`, confirmed `git log --oneline` =
  "ci(core-921): require cli-gofmt and strengthen negative regressions (Codex round 2)".
- **Reviewer:** O13 (independent; did NOT implement).
- **Round-2 findings reviewed** (`codex-verdict-core921-round2.md`, session
  fe5dff6c, 2026-10-19T13:46:36Z): #1 `cli-gofmt` omitted from the core-921
  clause; #2 negative coverage incomplete (ancestry test runs against `.`;
  stubbed variant remains; no wrong-status-only rejection proof).
- **Delta vs origin/develop:** exactly 2 files, 67 insertions / 44 deletions.
  `scripts/pr-gate-plan.py` (+7/−7 net single semantic line: adds
  `require(WIZARD_BASE_CHECKS)` to the core-921 clause); `scripts/test_pr_gate_plan.py`
  (stubbed variant removed, `expected_checks` gains `WIZARD_BASE_CHECKS`,
  `test_wrong_status_only_stays_unresolved` added, `test_real_unreviewed_commit`
  reworked to the real verifier against the temp repo).

### Corrected working-tree state (read-only integrity)

My first verification pass imported `pr-gate-plan.py` via `importlib` in a
heredoc, which left the two scripts dirty (`3addc8f0…`). Detected via
`git status`, restored with `git checkout --`, re-confirmed clean before
re-running. All results below were produced against the restored, unmodified
committed blobs. No product files touched.

### sha256 of the two scripts (before == after my checks)

```
0b012fbc17c321aa8b92630f8b575393ef26b739f10ca3519e475a91c001cdd2  scripts/pr-gate-plan.py
44ac245e613ab670135be6c77d208cb704d43425e6fa5022badbcd69d2c16318  scripts/test_pr_gate_plan.py
```
Equal before/after; matches `git cat-file -p 706b5244:<path> | sha256sum`.

### Check (1): core-921 clause now requires cli-gofmt + four coverage families

Round-2 finding #1 addressed. `pr-gate-plan.py:686` — the core-921 clause now
calls `require(WIZARD_BASE_CHECKS)`, whose definition
(`pr-gate-plan.py:67-72`) includes `cli-gofmt`
(`test -z "$(gofmt -l cmd/baha)"`) plus `cli-vet`/`cli-build`. The four named
coverage lanes are also required (`pr-gate-plan.py:687-690`):
`wizard-registration-tests`, `wizard-machine-adapter-tests`,
`wizard-provider-choice-tests`, `wizard-core921-tests`.

**Emitted required-checks for the exact 16-file reviewed set**
(`CORE_921_PRODUCTION_REVIEWED_FILES`, bound to head `6931ba9c…`, clean
trusted-target merge chain stubbed True; the positive test
`test_exact_reviewed_bundle_resolves_with_full_check_set` builds the identical
temp repo and asserts this set exactly — it passes):

```
cli-guided-tests        | cmd/baha guided read-only adapter
cli-vet                 | cmd/baha package
cli-build               | cmd/baha package
source-contracts        | CI source / contracts
cli-gofmt               | cmd/baha formatting        <-- round-2 finding #1
documentation-audit     | docs (assumed from release-requirements)
mkdocs-de/en-strict     | docs (assumed from release-requirements)
pages-candidate-build   | docs (assumed from release-requirements)
wizard-registration-tests   <-- four coverage families
wizard-machine-adapter-tests
wizard-provider-choice-tests
wizard-core921-tests
```

(`documentation-audit`/`mkdocs-de/en-strict`/`pages-candidate-build` come from
`release-requirements.json` `required_validation_checks`, enforced in every
clause; the four wizard-* lanes are the newly added core-921 coverage families
and are present.) cli-gofmt is emitted. ✅

**No other clause weakened.** `git diff origin/develop -- scripts/pr-gate-plan.py`
is purely additive: 93 added lines, 2 removed — both removed lines are 2→4
space re-indentation of the GC_999 tuple entry (the new
`BINDING_PR_1007_REVIEWED` entry was appended to that loop at `:543`). Every
referenced reviewed-head assertion is intact and byte-present:
`CORE_991`, `CORE_982`, `CORE_917`, `CORE_918`, `CORE_893`
(`CORE_893_REVIEWED_HEAD_SHA`), `CORE_751`, `REGISTRATION_CI`
(#974), `BINDING_PR_1007`, `GC_999`. No body of any existing clause changed. ✅

### Check (2): test suite

`python3 scripts/test_pr_gate_plan.py`: **84 tests, all OK** (64 + 8 + 7 + 5).
`python3 scripts/test_pr_check_runner.py`: **18 tests, all OK.**

Round-2 finding #2 addressed — confirmed against the committed diff
(`git show 706b5244 -- scripts/test_pr_gate_plan.py`):

- **Ancestry negative runs against the temp repo, not `.`.**
  `test_wrong_status_only_stays_unresolved` and
  `test_real_unreviewed_commit...` both call
  `core752_reviewed_merge_chain(target, head, reviewed, root)` with the fixture
  `root`, never `.`. `proof()` delegates to the real function against `root`.
- **Stubbed variant removed.** The old
  `test_mutated_reviewed_head_and_dirty_merge_chain_do_not_resolve`
  (`side_effect=lambda a, b, c=None: False` — forcing rejection) is gone; the
  test no longer stubs the verifier to a fixed value.
- **Wrong-status-only rejection proven.** `test_wrong_status_only_stays_unresolved`
  sanity-checks the real verifier ACCEPTS the clean merge
  (`assertTrue(core752_reviewed_merge_chain(target, head, reviewed, root))`),
  then flips one `modified` path to `added` and asserts the clause still rejects
  — proving the rejection is caused purely by the status flip, not by ancestry
  or a missing commit.
- Positive test now asserts `expected_checks` includes `WIZARD_BASE_CHECKS`
  (cli-gofmt), matching the clause.

### Check (3): no product changes anywhere

`git diff --stat 706b5244` excludes only the two scripts; with them excluded
the diff is empty. Worktree clean. Only the two CI-execution scripts changed in
the entire PR. ✅

### Verdict: APPROVE

Both Codex round-2 findings are resolved and the two test suites pass; no other
clause was weakened; no product changes.

**Findings:**

1. **cli-gofmt now required.** The core-921 clause
   (`pr-gate-plan.py:686`) adds `require(WIZARD_BASE_CHECKS)`, which contains
   `cli-gofmt`. Emitted required-checks above include `cli-gofmt` alongside the
   four coverage families (`wizard-registration-tests`,
   `wizard-machine-adapter-tests`, `wizard-provider-choice-tests`,
   `wizard-core921-tests`). Round-2 finding #1 resolved.
2. **Negative coverage strengthened.** Stubbed
   `side_effect=lambda a,b,c=None: False` variant removed; ancestry negatives
   run against the fixture `root` (not `.`);
   `test_wrong_status_only_stays_unresolved` proves rejection purely via status
   mutation (real verifier still accepts the clean merge). Round-2 finding #2
   resolved.
3. **No clause weakened.** `pr-gate-plan.py` diff is additive only (the two
   removed lines are whitespace re-indentation of the GC_999 tuple entry); all
   of #991/#1005/#974/#982/#917/#918/#893/#751 and the other reviewed-head
   assertions remain byte-present.
4. **Read-only integrity note.** My initial `importlib` heredoc left the two
   scripts dirty; restored via `git checkout`, re-confirmed clean before
   re-running tests. SHA-256 equal before/after.

**Finish line:** REV1007-O13
