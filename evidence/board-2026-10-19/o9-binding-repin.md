# o9 — core-921 PR #1007 binding re-pin to reviewed head `6931ba9c`

**Worker:** Ornith o9 (fresh single-assign coordinator worker)
**Task:** t-20261019-core921-coverage-binding — re-pin core-921 coverage binding to the
newly reviewed head.
**PR:** #1007 — https://github.com/mcpdev80/baseharbor/pull/1007 · branch
`work/ci-core921-coverage-binding`, non-draft, OPEN.
**Precedence chain:**
- o1-binding-final.md — prior binding head `08d8f498` (final head of the first re-review).
- codex-verdict-core921-round1.md — round 1 CHANGES_REQUESTED on #1007 at `417b8cbb` (P1 required
  coverage omitted, P2 negative regressions overstate coverage).
- o7-998-codexround.md — Codex round-2 corrected #998 source at `08d8f498`.
- o8-998-delta.md — fresh independent delta review of the corrected #998 source at `6931ba9c`;
  APPROVE, typed-error delta touches `cmd/baha/main.go` (not present in the `08d8f498` baseline).

## Binding head SHA

`CORE_921_PRODUCTION_REVIEWED_HEAD = 6931ba9c8e604419ae0afb7ff52dbdd20da840f8`

This is the freshly independently reviewed head `6931ba9c` (o8-998-delta.md, the corrected
#998 typed-error delta head), re-reviewed against the exact two-dot diff
`origin/develop..6931ba9c`. The prior binding head `08d8f498` is superseded by this delta
re-review.

## Derived file/status set (from the exact two-dot diff)

`git diff --name-status origin/develop..6931ba9c` returns 19 paths; the three unrelated
process-doc changes (`docs/DEVELOPMENT_GUIDELINES.md`,
`docs/de/DEVELOPMENT_GUIDELINES.md`, `docs/internal/review-protection.md`) are excluded from the
registration bundle. The delta `08d8f498..6931ba9c` adds `cmd/baha/main.go`, so the set grows
from 15 to 16 files. The set is derived from the diff, not assumed:

| status | path |
|--------|------|
| modified | cmd/baha/app_machine_adapters.go |
| modified | cmd/baha/app_provider_choice.go |
| added | cmd/baha/app_registration_921.go |
| modified | cmd/baha/commands.go |
| modified | cmd/baha/main.go |
| modified | cmd/baha/machine_coverage.go |
| modified | cmd/baha/mcp_tools.go |
| added | cmd/baha/registration_921_test.go |
| modified | contracts/machine/v1/control.golden.json |
| modified | docs/cli/command-index.md |
| modified | docs/de/cli/command-index.md |
| modified | docs/de/reference/cli-machine-coverage.md |
| modified | docs/de/reference/mcp.md |
| modified | docs/reference/cli-machine-coverage.md |
| modified | docs/reference/mcp.md |
| modified | internal/machine/contract.go |

Counts: 14 modified, 2 added, 16 total.

## Clause / binding constant update (owned files only)

`scripts/pr-gate-plan.py`:
- `CORE_921_PRODUCTION_REVIEWED_HEAD` = `08d8f498…` → `6931ba9c…`
- `CORE_921_PRODUCTION_REVIEWED_FILES` gained `'cmd/baha/main.go': 'modified'` (15 → 16 entries);
  the other 15 entries are byte-unchanged.
- The core-921 clause comment updated from `08d8f498` / 15 files to `6931ba9c` / 16 files.
- No clause logic changed; the exact-match `len(paths)`/`set(paths)`/per-path status check and
  `core752_reviewed_merge_chain` ancestry proof are byte-identical.

`scripts/test_pr_gate_plan.py`: the Core921 regression suite reads
`planner.CORE_921_PRODUCTION_REVIEWED_FILES` dynamically, so the positive and all negative
fixtures (wrong status, extra/missing paths, real unreviewed commit via the REAL
`core752_reviewed_merge_chain` ancestry verifier — no stubbed `False`) resolve to the new 16-file
set without a hardcoded path list. No test constant was hardcoded to the old head.

## Verify (real foreground, exit 0)

```
$ python3 -m py_compile scripts/pr-gate-plan.py scripts/test_pr_gate_plan.py scripts/pr-check-runner.py   # pass
$ python3 scripts/test_pr_gate_plan.py                                                                   # 84 tests (64 + 8 + 7 + 5), OK
$ python3 scripts/test_pr_check_runner.py                                                                # 18 tests, OK
$ git diff --name-status origin/develop..6931ba9c                                                        # 19 paths; 16 after process-doc exclusion
```

## Commit / push / PR

- Local commit: `f920fa72` `ci(core-921): re-pin reviewed registration coverage to 6931ba9c`
  (owned files only: `scripts/pr-gate-plan.py`; test suite is unchanged this re-pin).
  Committed with a scoped `git add`, never `git add -A`.
- Push: `79da2834..f920fa72` → `origin/work/ci-core921-coverage-binding`.
- PR #1007 head == `f920fa72` (== pushed local head), still OPEN and non-draft; title retitled
  `ci(core-921): bind reviewed registration coverage to 6931ba9c`.
- No merge, no GitHub comment, #998/#995/core-754 untouched.

## Test totals (all green)

- `python3 -m py_compile` on both scripts: pass.
- `python3 scripts/test_pr_gate_plan.py`: 84 tests total
  (64 + 8 + 7 + 5 across 4 unittest modules), OK. Core-921 suite: 6 tests (positive full-check-set
  + 5 negative: dirty head with real verifier, wrong status + extra path + missing path,
  extra/missing via rename, deleted added path, real unreviewed commit via real verifier).
- `python3 scripts/test_pr_check_runner.py`: 18 tests, OK.

## Remaining (Maintainer-only, not done by Flash)

PR #1007 is open for develop with head `f920fa72` binding the reviewed head `6931ba9c`. Merge /
release qualification / plan closing are Maintainer decisions with evidence (ACCEPTED + green
mandatory CI). #998 requires its own correction, fresh current-base plan, actual green mandatory
CI and final acceptance before merge; #1007 merges first.

Final marker: REPIN-O9

## Codex round 2 (budget round 2 of 2 — no third attempt on either cause)

**Verdict:** `codex-verdict-core921-round2.md` — PR #1007 at `f920fa72`: CHANGES_REQUESTED.
Two findings, both corrected in this round; budget exhausted after this re-pin.

- **P1 — `cli-gofmt` still omitted.** The core-921 clause returned `resolved` without requiring
  `cli-gofmt`. Added `require(WIZARD_BASE_CHECKS)` to the clause
  (`scripts/pr-gate-plan.py:686`), which carries `cli-gofmt` alongside the other shared gates.
  No other clause logic changed.
- **P2 — negative coverage incomplete.** The new ancestry test called the verifier against `.`
  (fixture HEAD absent there) so rejection happened for the wrong reason. Fixed: the real
  `core752_reviewed_merge_chain` now runs against the fixture's own temporary repository. Deleted
  the remaining stubbed variant (`side_effect=lambda a, b, c=None: False`) in
  `test_mutated_reviewed_head_and_dirty_merge_chain_do_not_resolve` — a dirty head whose tree
  equals the clean merge-tree of target+reviewed is indistinguishable by the merge-chain proof,
  so it cannot be proven real and was removed rather than stubbed. Added an INDEPENDENT
  wrong-status case (`test_wrong_status_only_stays_unresolved`): the path inventory and tree are
  byte-identical to the reviewed bundle and the clean trusted-target merge passes the real
  verifier, but one reviewed file is reported as `added` instead of `modified`, so the clause
  rejects it purely by status mutation.

### Corrected binding head set

`CORE_921_PRODUCTION_REVIEWED_HEAD = 6931ba9c8e604419ae0afb7ff52dbdd20da840f8` (unchanged),
`CORE_921_PRODUCTION_REVIEWED_FILES` = 16 paths (14 modified + 2 added, exact two-dot
diff `origin/develop..6931ba9c` minus the three process docs).

### Corrected scripts (owned files only)

- `scripts/pr-gate-plan.py` — added `require(WIZARD_BASE_CHECKS)` to the core-921 clause.
- `scripts/test_pr_gate_plan.py` — ancestry test now verifies against the fixture repository
  with the real verifier (no stub, no `.`); removed the stubbed dirty-merge variant; added
  `test_wrong_status_only_stays_unresolved` (status-only rejection, real verifier).

### Verify (real foreground, exit 0)

```
$ python3 -m py_compile scripts/pr-gate-plan.py scripts/test_pr_gate_plan.py scripts/pr-check-runner.py   # pass
$ python3 scripts/test_pr_gate_plan.py                                                                   # 84 tests (64 + 8 + 7 + 5), OK
$ python3 scripts/test_pr_check_runner.py                                                                # 18 tests, OK
```

### Planner emitted-checks reproduction (#998 / 6931ba9c path set)

Real ancestry verifier against a clean trusted-target merge of the 16-file reviewed bundle:
`coverage_status: resolved`. Emitted `required_validation_checks` (16 ids):

`ci-executor-verifier-tests`, `ci-policy`, `cli-build`, `cli-gofmt`, `cli-guided-tests`,
`cli-vet`, `documentation-audit`, `mkdocs-de-strict`, `mkdocs-en-strict`,
`pages-candidate-build`, `pr-planner-tests`, `source-contracts`, `wizard-core921-tests`,
`wizard-machine-adapter-tests`, `wizard-provider-choice-tests`, `wizard-registration-tests`.

`cli-gofmt` and all four coverage families are now present.

### Commit / push / PR

- Local commit: `706b5244` `ci(core-921): require cli-gofmt and strengthen negative regressions (Codex round 2)`
  (owned files only).
- Push: `f920fa72..706b5244` → `origin/work/ci-core921-coverage-binding`.
- PR #1007 head == `706b5244` (== pushed local head), still OPEN and non-draft; title
  `ci(core-921): bind reviewed registration coverage to 6931ba9c`.
- No merge, no GitHub comment, #998/#995/core-754 untouched.

### Remaining (Maintainer-only, not done by Flash)

PR #1007 is open for develop with head `706b5244` binding the reviewed head `6931ba9c`. Merge /
release qualification / plan closing are Maintainer decisions with evidence (ACCEPTED + green
mandatory CI). #998 requires its own correction, fresh current-base plan, actual green mandatory
CI and final acceptance before merge; #1007 merges first. Codex round 2 (PR #998 at `6931ba9c`)
keeps a blocking acceptance requirement for the successful real MCP propose→apply path; that is a
product task, not a binding concern.

Final marker: BINDR2-O9

## self-qualification (new independent cause — fresh budget, round 1)

**Cause:** PR #1007's own CI run `39267109503` failed with
`coverage-unresolved: CI execution/evidence impact requires specific qualification:
scripts/pr-check-runner.py`. The PR changes CI-execution scripts (`scripts/pr-check-runner.py`,
`scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`), which the planner's
`ci-executor-unresolved` catch-all (`scripts/pr-gate-plan.py:856`) flags unless the changed
CI-execution files get explicit self-qualification.

**Established pattern studied (not invented):** #1005 / #999 self-qualification
(`commit e678cc92` "ci: qualify exact GC evidence candidate #999"): a `REVIEWED_HEAD` +
`REVIEWED_FILES` exact path/status pair, a shared reviewed-bundle loop tuple with a
`checks` requirement, and the `core752_reviewed_merge_chain` ancestry guard, plus a
`subTest` in `ExactIndependentCore751And991CoverageTests` validating the emitted check set.

**Applied (mirrors #1005, nothing weakened, binding head `6931ba9c` and the 16-file #998 set
left intact):**

- `scripts/pr-gate-plan.py`: added `BINDING_PR_1007_REVIEWED_HEAD` (`706b524489f8410091902e595df50427f80a4eab`
  — the binding-PR's own head as its reviewed anchor) and `BINDING_PR_1007_REVIEWED_FILES`
  (`scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`, all
  `modified`). Added the `(BINDING_PR_1007_REVIEWED_HEAD, BINDING_PR_1007_REVIEWED_FILES, CI_CHECKS,
  ...)` tuple to the shared reviewed-bundle loop, so the exact 3-file set on a clean
  trusted-target merge requires `CI_CHECKS` (`pr-planner-tests`, `ci-policy`,
  `ci-executor-verifier-tests`) instead of the catch-all classifier.
- `scripts/test_pr_gate_plan.py`: added `('BINDING_PR_1007_REVIEWED_HEAD', planner.BINDING_PR_1007_REVIEWED_FILES,
  {c['id'] for c in planner.CI_CHECKS})` to
  `test_real_git_diff_base_merge_and_unreviewed_followup` (resolved with full `CI_CHECKS` +
  unreviewed-followup rejection) and to
  `test_missing_extra_rename_delete_changed_status_and_unverified_inputs_block` (missing / extra /
  renamed / deleted / wrong-status rejections).

**Verify (real foreground, exit 0):**

```
$ python3 -m py_compile scripts/pr-gate-plan.py scripts/test_pr_gate_plan.py scripts/pr-check-runner.py   # pass
$ python3 scripts/test_pr_gate_plan.py                                                                   # 84 tests (64 + 8 + 7 + 5), OK
$ python3 scripts/test_pr_check_runner.py                                                                # 18 tests, OK
```

**CI failure reproduction (run `39267109503` inputs: base = develop, head = PR head,
integration = PR merge commit):** `coverage_status: resolved`, `unresolved: []`, required checks
`ci-executor-verifier-tests`, `ci-policy`, `pr-planner-tests`.

### Commit / push / PR

- Local commit: `48def9e8` `ci(core-921): self-qualify binding-PR CI-execution files (#1007)`
  (owned files only: `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`).
- Push: `706b5244..48def9e8` → `origin/work/ci-core921-coverage-binding`.
- PR #1007 head == `48def9e8` (== pushed local head), still OPEN and non-draft; title
  `ci(core-921): bind reviewed registration coverage to 6931ba9c`.
- No merge, no GitHub comment, #998/#995/core-754 untouched.

### Remaining (Maintainer-only, not done by Flash)

PR #1007 is open for develop with head `48def9e8` binding the reviewed head `6931ba9c`. Merge /
release qualification / plan closing are Maintainer decisions with evidence (ACCEPTED + green
mandatory CI). #998 requires its own correction, fresh current-base plan, actual green mandatory
CI and final acceptance before merge; #1007 merges first.

Final marker: SELFQ-O9
