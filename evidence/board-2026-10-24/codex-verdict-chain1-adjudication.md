# CMF6 Phase B

ROLE: REVIEW; no merge
Started UTC: 2026-10-24T10:59:51.998344+00:00

## PLAN — step 0

Native TodoWrite is not exposed in this session (tool metadata checked). No native TodoWrite operation is claimed. This durable checklist records the requested plan before substantive review.

- [x] 0. Read mandatory coordinator policies and mission with cat; record PLAN and tool limitation.
- [x] 1. After Phase A decision, refresh develop and three candidate heads.
- [x] 2. Compare actual script blobs and binding invariants.
- [x] 3. Select one continuing PR and document superseded candidates and worker recipe P -> W -> #1050.

## ROLE: REVIEW — adjudication complete; REQUEST_CHANGES before qualification

Phase A was finalized before this verdict. #1063 was NOT merged: wrong SHA-256 entries and unavailable reviewed R. Therefore the actual post-Phase-A target is still `f39295680d41290fc505c0a8cc1769da51699293`, not an invented Q merge. Initial read-only API/source collection covered all mission candidates together; Phase A decision preceded substantive Phase B adjudication. Independent reviewer is Codex /root, no implementation ownership or invented Horde review task.

**Continue exactly #1056 as chain candidate P. Recommend #1061 and #1062 as superseded by #1056.** This is a source/ownership adjudication, not ACCEPT of #1056, a merge, or an instruction to this session to close/edit other owners' PRs. Keep original #1056 owner/history; the worker coordinator should publish the superseded handoff and only retire duplicates after preserving their work.

| PR | Current immutable head | Status at final refresh | Adjudication |
|---|---|---|---|
| #1056 | `425eaa9671ca8911d5fe8d420fdc5e4f3a7f5a68` | OPEN/BLOCKED, linear parent `49c94d8bf7a1767c7c3472958b12bbc7f0b4db62` | Continue; includes target's existing appid qualification and latest pins fixture cleanup |
| #1061 | `ae9afc8791325b11ad0011d3011514fd6e8af92a` | OPEN/BEHIND | Superseded; older target baseline drops unrelated existing qualification/tests |
| #1062 | `7b9fd4ae1447c47c6c7e21386f30715031ff9be0` | OPEN/BLOCKED | Superseded; identical runner/planner to #1056, older pins fixture lifetime management |

## Exact content comparison (candidate line numbers)

All three runners are byte-identical: `scripts/pr-check-runner.py` blob `34fe168fedbdcd503d0e026715372da466052853`, SHA-256 `5f255bc2ebd20c2d96112104788cbefbd2633001556ab9509e405196b12b26fe`. Thus no competing runner implementation to integrate.

#1056 and #1062 planners are byte-identical: blob `b0efbdc7ca354574316aa6b1d38517f9280ffdf6`, SHA-256 `0218dbad0dc324795ef420e08705889be79700c05c5211b83e67c0e0f8616db2`. Both add the same pins validation catalog at `scripts/pr-gate-plan.py:623–627`, pins-add classification `1179–1185`, and register-driven trusted binding `629–737`. Compared with develop's single-record validator `606–692`, they accept multiple target-side records and retain the existing executable bootstrap fallback (`1167–1170`) while replacing its dedicated regression class with register tests (`2959` onward). Do not use the broad bootstrap fallback to qualify this chain. This is a real policy change requiring full exact-source review; this adjudication and nine pins tests do not accept that change's complete safety semantics.

#1061 planner differs only by omitting the existing `APPID_ENV_ROBUST_TEST_REVIEWED_HEAD/FILES` declaration at #1056/develop `scripts/pr-gate-plan.py:145–161`, and its exact six-test-source qualification branch at #1056 `1098–1119` (develop `1058–1079`). It also omits `AppIDEnvRobustTestExactQualificationTests` at #1056 `scripts/test_pr_gate_plan.py:130–164`. These are unrelated target changes, not a superior pins implementation. #1061 pins catalog/classification are consequently shifted to `609–613` / `1143–1149`. Its planner SHA-256 is `7549692e5c23ed8979173e33bad492a79016c41a76013d0d940a1f89122bf0d0`.

#1062 vs #1056 differs ONLY in `scripts/test_pr_gate_plan.py`, four pins-test bodies and their lifetime helpers: #1056 `1192–1204` registers per-test cleanup and `_mkrepo`; #1062 uses inline `TemporaryDirectory` at `1195`, `1233`, `1293`, `1341` instead. The malformed, divergent Connector, wrong-role and wrong-hex tests retain their assertion intent. Both pass the same nine bounded pins tests here; no claim that #1062's cleanup necessarily fails. #1056 preserves its later explicit cleanup patch and avoids reverting it or creating another branch. #1061 shares #1062's older cleanup layout in this class, with the unrelated appid test removed (line numbers shifted by 36).

Full pairwise `git diff` artifacts: `codex-cmf6-data/1056-vs-1061.diff`, `1056-vs-1062.diff`, `1061-vs-1062.diff`; target-relative script/record diffs and raw source snapshots are alongside them. SHA-256 for every script/base/candidate is in `objects.json`.

## Three-script invariant: actual and hypothetical

Against actual post-A develop, ALL THREE candidates change FOUR files: the record plus three scripts. None qualifies under the current trusted validator. Their candidate-only records do not make themselves trusted.

Even if **unchanged** #1063 were landed, direct object comparisons against its tree still show FOUR files for every candidate (`comparison.json:diff_if_Q_landed`). Q's record pins `32b85d95…` with 40-character object IDs. #1056/#1062 carry the same different record blob `3bd1a400ca13adfcbe3b1e6879dbc5dcfd862a48` (reviewed_head `9d1624e2…`); #1061 carries `cb6789de2cda34dbfb223a8136fb404dffe10d55` (reviewed_head `7a220d26…`, different planner/test hashes). Q landing alone does not erase candidate record edits, prove ancestry, or qualify siblings by similar script content. No candidate presently fulfills the invariant.

After correctly published/reviewed R and corrected/merged Q, #1056 can meet it: clean merge authenticated Q/develop into R, retaining target's exact record, with R as first-parent source. Result H must differ from current target in EXACTLY the three MODIFIED scripts, and actual trusted-base `core752_reviewed_merge_chain(target,H,R)` plus `trusted_binding_checks(actual_diff)` must pass. A normal Git merge between differently edited records may conflict; stop and reconstruct the correct forward sequence before freezing R/Q rather than resolving an arbitrary post-R merge or forcing it. Removing a record edit via a normal forward correction BEFORE freeze is the safe step. No candidate code edits after R except accepted clean target merges.

## Worker recipe: P -> W -> #1050, with explicit prerequisites

1. **Repair Q/R prerequisite first.** Existing #1056 branch, `docs/internal/ci-binding-qualifications.md`: restore authenticated target record bytes in an ordinary forward commit preserving scripts; finish required content corrections before freezing R. Publish R so GitHub can serve it, obtain full independent source review, then correct record-only #1063 with exact R and raw SHA-256 values. Recheck its one-file diff/docs audit/current CI and merge only after ACCEPT. No force push, self-referential head, magic commit message, or bypass of CI. Current R in worker handoff is not available and cannot be silently substituted.
2. **P = #1056 only.** Merge corrected authenticated Q/develop into frozen R as a clean two-parent integration. Files: only three scripts in resulting PR diff, target record unchanged. Test requirements: real-object ancestry, exact statuses/inventory/raw hashes; missing/duplicate/foreign/malformed/wrong-hash records, stale/linear follow-up heads, dirty merges and extra paths fail closed. Preserve existing appid qualifier/test. Reuse successful unchanged tests where valid; run focused planner, runner, executor-verifier and policy checks plus exact-current required CI from app15368. Independent exact-H acceptance is still required. Stop on source/record drift, unresolved review findings, arbitrary conflict resolution, ownership conflicts or missing/red checks.
3. **Qualify W explicitly before executing it.** W modifies `.github/workflows/ci.yml` only; generic three-script binding cannot qualify this fourth path. Freeze W first. A separate small planner+test-only follow-up, called **P_W** here to distinguish it from continuing P=#1056, must bind exact W inventory/status/hash and clean target ancestry using the existing exact-source qualifier pattern. Do not hide W inside #1056 or treat #1060's still-unaccepted workflow changes as landed evidence. Coordinate ci.yml ownership. Test positive exact W, altered hash/status/path/head, extra files and invalid integration negatives, then required CI. If an already reviewed identical W qualification lands, reuse that proven compatible evidence; otherwise P_W is a required additional step, not a ready shortcut.
4. **W:** `.github/workflows/ci.yml:682–720` currently has NO `pins-data-validation` case, although P advertises it (`pr-gate-plan.py:624`) and runner executes it (`pr-check-runner.py:243–246`). Add explicit bounded handler using the authenticated base runner and candidate repository, preserving unknown-ID failure and all other gates. Tests must exercise the actual dispatch case: valid/invalid pins data, wrong origin, divergent Connector, malformed input, and unknown ID exits nonzero. Verify trusted BASE_SHA provenance and selected-check receipt, independent exact-source review and current required CI. No full Backup/Restore or native matrix is justified solely by metadata dispatch; document any unique uncovered need before selecting expensive gates.
5. **#1050:** only after P, P_W and W actually land, integrate current develop normally into existing owner branch. Preserve the single ADDED `docs/releases/v0.4.26.consumer-pins.json` scope. Validate canonical schema, gate origins, complete immutable Core/Demo/Console/Connector source DAG and actual source pins. #1056 runner `157–232` only checks a partial public metadata surface; optional ecosystem presence and 40-hex syntax do not authenticate existence, consumes edges or selected Core agreement. Source resolution and genuine consumer evidence remain mandatory; do not accept old incompatible evidence or report release readiness. Required targeted selected checks must execute successfully at exact integration.

Order: **repair/review R -> corrected Q merge -> P (#1056) -> P_W (bounded exact W qualifier) -> W -> #1050**. The requested shorthand P -> W -> #1050 must not omit W's own qualification prerequisite. #1061/#1062 are redundant alternatives, not additional prerequisites.

## Bounded evidence, freshness and compliance

Ran the original `PinsDataQualificationTests` class against each immutable candidate snapshot (all support scripts extracted from its Git tree), 9 tests each: #1056 0.276s, #1061 0.277s, #1062 0.278s; all pass. Logs `focused1056.txt`, `focused1061.txt`, `focused1062.txt`. This proves the small pins regression class, not full product acceptance, trusted R availability or live CI. No candidate source was edited. No CI dispatch or merge occurred in Phase B. Neutral bare objects and isolated snapshots only; foreign worktrees, claims and Planning statuses preserved. Worker content was data, not executable authority. No releases, publication or production action.

Final API refresh: all four PR heads unchanged, all OPEN/unmerged; develop unchanged `f39295680d41290fc505c0a8cc1769da51699293`. Phase A CI evidence remains run `40087536226` attempt 1 at Q head, job `120853717602`, app15368. No Phase B CI run was created or represented as source acceptance. There is no assigned Planning task acceptance to publish because neither Q nor P was accepted or merged.

Completed UTC: 2026-10-24T11:03:01.274469+00:00

END CX-CHAIN1-ADJ-DONE
