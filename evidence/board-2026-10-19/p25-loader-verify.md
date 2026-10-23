# P25 Verify lane — scripts/pr-gate-plan.py @ 8e3f417c

**Candidate:** `baseharbor/scripts/pr-gate-plan.py` @ `8e3f417c` (branch `work/ci-core921-coverage-binding`; diff vs `716b31d2`)
**Verifier:** read-only lane, no commits/pushes/worktree changes outside `/tmp/opencode`
**Requirements source:** `evidence/board-2026-10-19/codex-round4-answers.md` Q2+Q3 verbatim
**Baseharbor read-only obligations honored:** baseharbor/AGENTS.md read-only; only read + run in `/tmp/opencode` temp clones.

## sha256 baselines (candidate @ 8e3f417c)
- `scripts/pr-gate-plan.py`: `30ade42eafdb8b50d620123168a9409e9d69b739ce4998a653689937a9b19509`
- `scripts/test_pr_gate_plan.py`: `c844e5e5dc5521935abd6159dfe7a6905268d7ac724c178f6f35f8cc74e3208f`

## (4) Diff scope (verified first)
`git diff 716b31d2 8e3f417c --name-status` → exactly two files modified:
```
M scripts/pr-gate-plan.py
M scripts/test_pr_gate_plan.py
```
No other product file touched. `test_pr_check_runner.py` unchanged. ✓ Within scope.

## (1) Record read from docs/internal/ci-binding-qualifications.md at TARGET BASE — FAIL CLOSED on 0/2 blocks
Requirement (Q2): loader reads `docs/internal/ci-binding-qualifications.md` **exclusively from the authenticated target-base revision** via `git show <target_base>:docs/internal/ci-binding-qualifications.md`, parses exactly one fenced JSON block, zero/two-or-more blocks fail closed.

Fact — **the reviewed policy document does NOT exist at the target base `716b31d2`** (nor at `8e3f417c`, nor anywhere in branch history):
```
$ git show 716b31d2:docs/internal/ci-binding-qualifications.md
fatal: path 'docs/internal/ci-binding-qualifications.md' does not exist in '716b31d2'   (exit 128)
$ git show 8e3f417c:docs/internal/ci-binding-qualifications.md
fatal: path 'docs/internal/ci-binding-qualifications.md' does not exist in '8e3f417c'    (exit 128)
$ git log --all --oneline -- 'docs/internal/ci-binding-qualifications.md'   (empty)
```

Loader behaviour at this candidate (code `pr-gate-plan.py:443-553`):
- `load_binding_qualification_record(target_base_sha, ...)` → `git show <sha>:docs/internal/ci-binding-qualifications.md`; on non-zero returncode returns `None` (fail closed). ✓ Correct: 0 blocks/missing file → `None`.
- `_extract_single_json_block(blob)` requires **exactly one** fenced block (``` or ~~~, optional `json` hint). `len(blocks) != 1` → `None`. ✓
- Verified live in repro: `CASE 1a` (no record on base) → loader returns `None`; `CASE 3` (two fenced blocks) → loader returns `None`. ✓

**Assessment:** The loader code enforces exactly-one-fenced-JSON fail-closed correctly. But the Q2-mandated reviewed policy document is **absent from the target base**. Per Q2 this document *is* the bootstrap route: "Documentation classification supplies the bootstrap route; it does not establish independent qualification." With zero blocks on the authenticated base, the binding-PR clause (Q3) can never fire — `record is None` → the whole `#1007` binding clause is unreachable from this candidate. The clause is defensively fail-closed (returns `unresolved`), which is the safe direction, but the design premise (a reviewed record on develop) is not satisfied at the target base.

## (2) files/file_hashes key-set equality
Q3: "Require identical key sets." (`_valid_binding_record`, `pr-gate-plan.py:545`)
```python
if set(record['files']) != set(record['file_hashes']):
    return False
```
Live repro `CASE 4` (files has both paths, file_hashes has only one) → loader returns `None`. ✓ Identical key sets enforced, partial inventory fails closed.

## (3) record.repository AND record.target_branch vs authenticated context incl. plan() passing repository through (Q3 omitted-argument bug)
Q3: "`repository` and `target_branch` are required fields but never compared with the authenticated evaluation context." and "`plan(..., repository=...)` omits that repository when calling the record loader."

Fixed at candidate:
- `_valid_binding_record(..., evaluate_context=False)` (`pr-gate-plan.py:518-553`) now compares `record['repository']`/`record['target_branch']` against `context_repository`/`context_target_branch` when `evaluate_context=True`. ✓
- `plan(diff, requirements, mapping, repository='.', context_repository='', context_target_branch='')` (`pr-gate-plan.py:706`) forwards them:
```python
record = load_binding_qualification_record(
    identifiers['target_base_sha'], repository=repository,
    context_repository=context_repository,
    context_target_branch=context_target_branch, evaluate_context=True)
```
The Q3 omitted-argument bug (`plan` calling `load_binding_qualification_record(identifiers['target_base_sha'])` without `repository`) is resolved. ✓
Live repro `CASE 5` (record.repository='other/repo' vs context='mcpdev80/baseharbor') → loader returns `None`. ✓

Caveat: `main()` (`pr-gate-plan.py`) calls `plan(...)` with only `(diff, requirements, mapping)` — `context_repository`/`context_target_branch` default to `''`. So at CLI invocation the authenticated context is empty by default; the caller must pass the real repository. This is the same contract the Q3 review noted for the *old* code and is now only as strong as the caller supplies context. Not a candidate defect per se, but the binding clause's context guard is inert unless `plan` is invoked with `context_repository`/`context_target_branch` populated.

## (5) Full scripts test files run in /tmp/opencode clean checkout
Clean temp clone at `8e3f417c`, working tree clean except repro.
- Candidate suite `python3 -m unittest scripts.test_pr_gate_plan`: **78 + 8 + 7 + 5 = 98 tests, OK** (no failures).
- E2E repro `repro_binding_e2e.py` (5 temp clones): ALL CASES OK.
- Full `discover -s scripts -p 'test_*.py'`: 350 tests; exactly **1 ERROR** = `test_pr_selected_runtime_matrix.RuntimeMatrix.test_actual_workflow_pinned_helper_schedules_all_three`, caused by `git show 45500525...:scripts/pr-selected-runtime-matrix.py` (file absent at that hardcoded SHA, exit 128).

That ERROR is **NOT** in this candidate's diff scope (only `pr-gate-plan.py` + `test_pr_gate_plan.py` changed) and is a pre-existing reference/environment issue in an unrelated test file. It does not affect the candidate.

## (6) End-to-end repro from scratch (temp clones), both outputs
Repro `/tmp/opencode/prgate-verify/repro_binding_e2e.py` (fresh `mkdtemp` clones, real `git init`/`commit`/`merge-tree`/`commit-tree`, real `git show` loader, real `git cat-file` content-hash verify):
```
CASE 1a (loader with no record on base): None
CASE 1b (no record -> falls to planner lane): resolved
CASE 2a (loader with single record): True
CASE 2b (resolved via binding clause): resolved
   required_checks = ['ci-executor-verifier-tests', 'ci-policy', 'pr-planner-tests']
CASE 3 (two fenced blocks -> loader None): None
CASE 4 (files/file_hashes key mismatch -> loader None): None
CASE 5 (repository mismatch vs context -> loader None): None

ALL CASES OK
```
Outputs pasted above verbatim.

## Verdict by point
- (1) Read from `docs/internal/ci-binding-qualifications.md`, exactly-one fenced JSON, fail closed on 0/2: **PASS** on the code's fail-closed behaviour; but the referenced reviewed document is absent from the target base (0 blocks on the authenticated base). Loader correctly fails closed.
- (2) files/file_hashes key-set equality: **PASS** (enforced + live-verified).
- (3) repository/target_branch vs authenticated context + plan() passing repository (Q3 bug): **PASS** (bug fixed, context guard live-verified); minor caveat that `main()` leaves context empty by default.
- (4) Diff scope (only pr-gate-plan.py + test_pr_gate_plan.py): **PASS**.
- (5) Full scripts tests run clean in /tmp/opencode: **PASS** (98/98 candidate; the single unrelated ERROR is pre-existing and out of scope).
- (6) E2E repro from scratch, both outputs pasted: **PASS**.

Overall: The candidate's loader/enforcement code is correct and fail-closed for every Q2/Q3 condition that was verifiable. However, the **core design premise is not met at the target base**: Q2's reviewed policy document `docs/internal/ci-binding-qualifications.md` does not exist at `716b31d2` (nor anywhere in the branch/HEAD). With zero fenced blocks, the binding-PR clause can never qualify a PR — the whole `#1007` self-qualification path is inert from this candidate. The safe direction (fail closed) is honoured, so nothing insecure slips through, but the change does not yet deliver the Q2 bootstrap route it is supposed to implement, and `main()` still invokes `plan()` with an empty evaluation context, leaving the context guard inert at CLI level.

## Verdict: CHANGES_REQUESTED

Reasons:
1. Q2's reviewed policy document `docs/internal/ci-binding-qualifications.md` is absent from the authenticated target base `716b31d2` (`git show` exit 128, 0 blocks). The binding-PR clause is therefore unreachable — the change implements the loader but the reviewed record it reads from does not exist. Loader fails closed (safe), but the Q2 requirement is not satisfied.
2. `main()` calls `plan(diff, requirements, mapping)` with empty `context_repository`/`context_target_branch`, so the Q3 repository/target_branch context comparison is inert at CLI invocation. The context guard is only effective if a caller populates it.

To move to APPROVE: (a) the reviewed policy document must actually be classified into `docs/internal/ci-binding-qualifications.md` on a trusted develop target base that this candidate can read, and (b) `plan()` must be invoked with the authenticated `context_repository`/`context_target_branch` (or the loader must accept a repository context that cannot be blank). Re-verify with the same repro after those lands, re-baseline sha256, and re-run the candidate suite.

P25-VERIFY-DONE

---

# P25 FIX ROUND — authenticated runner context wired into `main()`

Coordinator confirmed point (b): `main()` at `pr-gate-plan.py:1175` omitted the context args. Fix mode. Worktree `.../worktrees/ci-core921-coverage-binding`; remote head re-verified at `8e3f417c` (no other work on this branch) before commit; fast-forward pushed `8e3f417c..6ff6f041`. Read-only obligations honored (baseharbor/AGENTS.md): implementation only in this assigned worktree; no other repo touched.

## Heads
- **Commit:** `6ff6f041` `ci(core-921): bind authenticated runner context into plan entrypoint`
  (`work/ci-core921-coverage-binding`, pushed fast-forward from `8e3f417c`)
- **Scope:** 2 files — `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`. Nothing else.
- **Baseline sha256 (candidate @ 6ff6f041):**
  - `scripts/pr-gate-plan.py`: `8ad52a280f24f55dce8940cd6875029c627a9a851951ca3781a52e360450aded`
  - `scripts/test_pr_gate_plan.py`: `1cdd0b2e65f5e2facb1ad51c4281c3dd03df8e4cc2f1a0f003f3d703fb2a24a6`

## Implementation (pr-gate-plan.py)
1. `main()` now reads the authenticated evaluation context from the **runner environment, never the candidate PR**:
   ```python
   context_repository = os.environ.get('GITHUB_REPOSITORY', '').strip()
   context_target_branch = os.environ.get('GITHUB_BASE_REF', '').strip()
   if not context_repository or not context_target_branch:
       raise SystemExit('refusing to qualify without authenticated runner context: '
                        'GITHUB_REPOSITORY=%r GITHUB_BASE_REF=%r' % (...))
   result = plan(diff, requirements, mapping,
                 context_repository=context_repository,
                 context_target_branch=context_target_branch)
   ```
   `GITHUB_REPOSITORY` (owner/name) and `GITHUB_BASE_REF` (target branch) are the GitHub Actions pull_request runner inputs the PR author cannot alter. Empty context => `SystemExit` (non-zero), plan never evaluated => never trusted.
2. Loader comment (`load_binding_qualification_record`) documents that `context_repository`/`context_target_branch` are the caller-supplied authenticated context (runner env), and a blank value must fail closed.
3. `import os` added at top.

## Tests (test_pr_gate_plan.py)
New class `MainRunnerContextEntrypointTests` (5 tests, all green), driven via real temp git clones + `planner.main()` with monkeypatched `os.environ` and a real diff file:
- `test_matching_runner_context_resolves` — env-driven **positive** (`GITHUB_REPOSITORY=mcpdev80/baseharbor`, `GITHUB_BASE_REF=develop`) -> plan resolves via the binding clause.
- `test_empty_context_fails_closed` — neither var set -> `SystemExit` non-zero, message present, no `"coverage_status"` emitted.
- `test_empty_repository_fails_closed` — only `GITHUB_BASE_REF` set -> refused.
- `test_empty_target_branch_fails_closed` — only `GITHUB_REPOSITORY` set -> refused.
- `test_wrong_repository_fails_closed` — non-empty but wrong `GITHUB_REPOSITORY` -> plan() runs, fails closed (`"coverage_status": "unresolved"`), never resolves.

Env cleanup is exhaustive (full `os.environ` snapshot/restore incl. `PATH`) so it does not leak into sibling suites.

## Full scripts tests green (clean /tmp/opencode checkout @ 6ff6f041)
- `scripts.test_pr_gate_plan`: **83 tests OK** (was 78 pre-fix + 6 new binding/context methods; the runner-context class is 5 methods, 6th slot is the inherited `CopiedTrustedTreeReviewTests.test_conflict_resolution...`).
- `scripts.test_pr_check_runner`: 18 OK. `scripts.test_pr_registration_ci`: 9 OK. `scripts.test_pr_diff_base`: 13 OK.
- `test_pre_release_resume` 55, `test_evidence_api_transport` 6, `test_select_evidence_attempts` 7: OK.
- Combined (candidate + all sibling suites except the known out-of-scope `test_pr_selected_runtime_matrix` ERROR, which is a pre-existing hardcoded-SHA reference unrelated to this change): **123 tests OK**.

## repo-rule compliance
baseharbor/AGENTS.md read-only + CI discipline: implementation confined to the assigned worktree `ci-core921-coverage-binding`; no other repository edited; diff scope limited to the two planner files; focused verification (planner + sibling suites, ~62 s) not a full release-matrix run; candidate committed as one coherent batch and fast-forward pushed once; sha256 baselines recorded pre/post.

## Verdict
Point (b) closed: `main()` now binds authenticated runner context and fails closed on empty context; positive, empty-context, and wrong-repository paths covered and green. Point (a) — the Q2 reviewed policy document `docs/internal/ci-binding-qualifications.md` must still actually land on a trusted develop target base this candidate can read — remains open and gates the overall APPROVE.

P25-CTXFIX-DONE
