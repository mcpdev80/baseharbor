# ROLLE: REVIEW — S39b / S42b

## PLAN

- [x] 0. Read the assignment and binding policies; record the review plan before review work.
- [x] 1. Refresh remote states and verify frozen sources, scope, prior findings and evidence.
- [x] 2. Review complete source diffs, workflow semantics and focused verification evidence.
- [x] 3. Record one exact-SHA verdict per branch with executable recipe and stop criteria; recheck heads.

Native TodoWrite is not exposed in this session's tool inventory. This checklist records the plan; no native TodoWrite update is claimed. The prompt itself contains no numbered PLAN step 0; step 0 above implements the user's explicit request.

Review only. No product edits, worker dispatch, merge or Planning acceptance is authorized by this assignment. The bootstrap's Maintainer pause takes precedence over historical Planning resumption prose.

## Verdict

| Candidate | Verdict | Blocking finding |
| --- | --- | --- |
| A / #1039 `fe17e6e58da16fb7eb0912e28825dd622c079d16` | **CHANGES-REQUIRED** | One residual blank line in `scripts/pr-gate-plan.py:245`; actual source inventory has two paths instead of one. |
| B / #1042 `c34740fa5fee5c5c7b07af03c3eb929386840e93` | **CHANGES-REQUIRED** | One residual blank line in `scripts/pr-gate-plan.py:748`; actual source inventory has six paths instead of five. |

Neither frozen SHA is accepted for the qualification predecessor Q. These are exact-inventory blockers, not formatting preferences. The accepted planner's guard at `scripts/pr-gate-plan.py:774–780` requires complete path/status equality. Do not add the leftover planner path to either qualification map to evade the source-only contract.

## Immutable inputs and review method

Independent reviewer: this directly assigned Codex REVIEW session, separate from the implementing workers. No Horde task was created; session UUID/model/backend metadata are not exposed to this tool interface and are not invented. Observed 2026-10-23 approximately 11:15–11:20 UTC. Host reports `NTPSynchronized=yes`. Product and Planning remotes were fetched; authenticated GitHub account is `mcpdev80`. Planning HEAD versus origin/main divergence is `0 0`.

Both live PR heads were checked before inspection and again after complete-diff inspection; both remain the requested frozen SHAs and OPEN. Their API base and Git merge-base are D0 `2f005813dc6d7fa94de164388b2531d025af31bc`. Fetched develop is `9f4916be333d0f99d7e40001441d9dbb87e993e2`. No checkout, index or product file was changed. Existing dirty/staged product changes were preserved; all source inspection used immutable `git show` / `git diff` blobs rather than that working tree.

Read the assignment, selector recipe, preceding S39/S42 verdict, complete q36 cleanup evidence, ecosystem RUNBOOK and product/Planning rules. Complete D0-to-source diffs and immediate correction diffs were inspected. The current develop planner and test blobs still equal D0, so q36's statement that S42 has different accepted base content does not explain its remaining planner difference.

| Artifact | Value |
| --- | --- |
| Accepted planner blob D0/develop | `98a9229c9126d8a6d242d44c4de523c69a6f843e` |
| S39b planner blob | `51ff40cace45c7e5ef056717a20288c07d36b8f5` |
| S42b planner blob | `70659b9806773d652187e53d3491dfb537bfbda4` |
| Accepted test blob, both candidates/D0/develop | `1a1eb3de903363677c95edac416c614551de9b57` |
| SHA256 of `git diff --binary D0...S39b` | `c49f6a187f45a4e8ab347141f38ad44035686e6fdeb612710b53f07927b24b46` |
| SHA256 of `git diff --binary D0...S42b` | `6dab232e31f1e220f15fd553eb8ced84762c9fe1f068d93281da40bc59a4cc81` |

## A: source review and executable worker recipe

The complete real workflow diff changes only the runtime-step timeout expression at `.github/workflows/pre-release-runtime-suite.yml:201`: data-capabilities gets 25 minutes, other data gates get 20. It preserves backup/recovery/security/default ordering and the existing job cap and gate execution. No separate acceptance phase was added by this actual diff; the branch's “split” name is not evidence of one.

Independent in-memory parsing of the immutable workflow succeeds. Evaluation of the actual timeout expression with the six requested inputs confirms backup=40, recovery=20, data-capabilities/data=25, other data=20, security=12, default=8. Both planner Python files compile in memory; neither contains `RUNTIME_STEP_CAP` or `DISPATCH_GATE_REF`. The dangling tuple/case and test IndentationError are repaired. The test file equals accepted D0 byte-for-byte.

The remaining full diff nevertheless contains modified `scripts/pr-gate-plan.py` in addition to the modified runtime-suite workflow. Its only difference is the extra blank line after `EDIT_ZERO_PREVIEW_REVIEWED_FILES` at candidate line 245.

**Existing A worker executes:** in its assigned isolated worktree, verify HEAD and preserve concurrent changes; restore only `scripts/pr-gate-plan.py` from D0, then make one normal follow-up commit. Concrete restoration command, after that ownership/HEAD check:

```bash
git restore --source=2f005813dc6d7fa94de164388b2531d025af31bc -- scripts/pr-gate-plan.py
```

**Tests and stop criterion:** require both planner blobs to equal the accepted hashes above; `git diff --name-status D0...NEW_SHA` must contain only modified `.github/workflows/pre-release-runtime-suite.yml`. Preserve the accepted timeout expression. Run/reuse verifier-valid focused planner, YAML/actionlint and workflow-policy evidence as required by selector step 2; retain the six-case timeout proof. Publish the new full source SHA and accurate evidence, then return for independent review. Stop if any product blob changes unexpectedly or another owner has advanced HEAD. Do not pin S39b in Q, amend/rebase/force-push, or merge.

## B: source review and executable worker recipe

Reviewed the entire five-file implementation as well as the leftover planner change: `ci.yml` adds the resolver tests to ordinary and PR focused Python; `targeted-demo-acceptance.yml` checks out event-SHA tooling for dispatch as well as push, invokes the helper under `set -euo pipefail` with `GH_TOKEN`, and passes its resolved output onward. The helper expands short hashes, distinguishes hex-named branches/tags, validates full commit objects and fails on unresolved input. The reusable runtime workflow guards the candidate before checkout and tightens diagnostic uploads after successful checkout. Its new demo preservation marker ensures a diagnostic file exists. Tests cover resolver classification, mocked API endpoints, workflow wiring and execution of the inline shell guard with a fake gh.

All three affected workflows parse in memory. All five product blobs remain byte-for-byte identical to prior `7bdd46fd7cba2a78c56eebe0da7076dc6a252321`; the cleanup did not introduce a new product implementation. Both planner files compile, branch-local qualification identifiers are absent and the test blob equals D0. No additional product defect was established in this bounded review. This is not source acceptance while the exact-inventory blocker remains.

At candidate `scripts/pr-gate-plan.py:748`, the blank line between the EDIT_ZERO_PREVIEW tuple and ECOSYSTEM_RESOLVER tuple remains. This is the fourth residual blank line from the preceding verdict; q36 removed the other three. The evidence's claim that the PR diff contains only two planner files conflates a correction-commit diff with the complete PR diff.

**Existing B worker executes:** verify ownership/HEAD and restore only `scripts/pr-gate-plan.py` with the same D0 command shown for A, then commit normally. Preserve all five product blobs. Correct `q36-src-cleanup.md` to distinguish correction-commit changes from full PR inventory and record real tests tied to the new SHA.

**Tests and stop criterion:** both planner hashes must equal accepted D0; full new PR inventory must be exactly three modified workflows (`ci.yml`, `pre-release-runtime-suite.yml`, `targeted-demo-acceptance.yml`) and two added files (`scripts/targeted-gate-ref.py`, `scripts/test_targeted_gate_ref.py`). Run/reuse valid `python3 scripts/test_targeted_gate_ref.py`, planner, YAML/actionlint and policy checks under the preceding recipe. Publish actual command/output and the new frozen SHA; request independent review before Q. Stop on unexpected product changes, changed ownership or failed required checks. No qualification-map expansion, rebase, force-push or merge.

## CI and evidence disposition

- A [run 39919131686](https://github.com/mcpdev80/baseharbor/actions/runs/39919131686): focused Python, workflow policy and trusted plan SUCCESS; mandatory `PR Selected Checks` FAILURE.
- B [run 39919305498](https://github.com/mcpdev80/baseharbor/actions/runs/39919305498): focused Python, workflow policy and trusted plan SUCCESS; mandatory `PR Selected Checks` FAILURE.

These are live check conclusions, not a claim that all required validations executed. Both runs report `PR · Validation · not-required`; that is not actionlint/planner/executor-verifier qualification evidence. The log commands returned no text in this session, so no exact failed-check diagnostic or individual test-count attribution is asserted. q36 lists successful test counts without command-to-SHA mappings; those snippets alone do not prove all requested prechecks. No expensive/native tests or new CI runs were launched. The independent reproductions were bounded parsing, compilation, expression evaluation and blob/diff comparisons only.

After corrected sources pass independent review, follow selector steps 4 onward: one two-file predecessor Q with actual newly accepted anchors, independent Q review and required CI, then separately authorized Maintainer actions. The bootstrap pause remains; this review authorizes no merge or release.

## repo-rule compliance

Only this requested evidence file was written. No product fixes, claims, Planning states, worker sessions or GitHub state were changed. Turn-start flags were triaged: pool deficits are conditional scheduling notices and do not authorize assignments in this REVIEW scope; existing coordinator PLAN work remains open. The events queue contains repeated unrelated idle/gone/permission notices requiring their coordinator's session inspection; they are not task-success evidence and were left unacknowledged rather than falsely marked processed. Native TodoWrite could not be updated because it is not available; the evidence PLAN is complete.

END CODEX-S39B-S42B-DONE

## ROLLE: REVIEW — correction round 2 PLAN

- [x] 0. Read the re-review assignment and binding policies; append the plan before candidate inspection.
- [x] 1. Refresh remotes and resolve live exact candidate identities; inspect correction deltas against the reviewed freezes.
- [x] 2. Verify accepted planner blobs, source-only inventories and preservation of reviewed product blobs.
- [x] 3. Append per-branch verdicts, verification limits and final HEAD guards. No merge.

Native TodoWrite remains unavailable in this session's exposed tools; no native update is claimed. The prompt contains no numbered PLAN step 0; the explicit user requirement is recorded above.

## Correction round 2 verdict — 2026-10-23 11:31–11:33 UTC

| Candidate | Exact reviewed SHA | Verdict |
| --- | --- | --- |
| A / #1039 | `75efeef89b61d848d5ea351877ba6423a31e58a6` | **ACCEPT** |
| B / #1042 | `0267dbdb71f63a2c77e193618e6fba4106e5cd5b` | **ACCEPT** |

These verdicts accept the corrected source anchors for the separate qualification predecessor described in selector step 4. They supersede the source-inventory rejection above for these new SHAs only. They do not constitute green CI, Planning acceptance, merge authorization or release readiness. No further source correction recipe is required.

The B identifier supplied in the prompt, `0267dbdb71f63a2c77e193618e6fba410`, is an abbreviated prefix. The fetched branch, live PR API and F19 evidence agree on the full 40-character SHA recorded above. A matches its supplied full SHA exactly. Both PRs remain OPEN, and repeated API queries after inspection confirmed both heads unchanged.

### Verified corrections and retained review

Independent reviewer: the same directly assigned Codex REVIEW session as the preceding review; not the F19 implementation worker. No Horde task was created. Product and Planning remotes were refreshed; authenticated account remains `mcpdev80`, host NTP is synchronized and Planning local/origin divergence remains `0 0`. Current develop remains `9f4916be333d0f99d7e40001441d9dbb87e993e2`; PR API bases and source merge-bases remain D0 `2f005813dc6d7fa94de164388b2531d025af31bc`.

Read `f19-gate-blank.md` and inspected both complete correction diffs. Each new commit has exactly the previous reviewed freeze as its sole parent and changes only `scripts/pr-gate-plan.py`, deleting one blank line:

- A removes the extra line after `EDIT_ZERO_PREVIEW_REVIEWED_FILES` (old line 245).
- B removes the extra line between EDIT_ZERO_PREVIEW and ECOSYSTEM_RESOLVER tuples (old line 748).

For both candidates, immutable blob comparison confirms planner `98a9229c9126d8a6d242d44c4de523c69a6f843e` and planner-test `1a1eb3de903363677c95edac416c614551de9b57`, identical to accepted D0. Both files compile in memory and contain neither branch-local qualification identifier. There are no other changed blobs relative to the previous reviewed freezes. Consequently the complete product-source review, YAML parsing and six-case timeout proof recorded above remain applicable without repeating unchanged checks. No new product defect or unresolved source finding remains from that review.

Actual full D0-to-candidate inventories now match the required maps exactly:

```text
A:
M .github/workflows/pre-release-runtime-suite.yml

B:
M .github/workflows/ci.yml
M .github/workflows/pre-release-runtime-suite.yml
M .github/workflows/targeted-demo-acceptance.yml
A scripts/targeted-gate-ref.py
A scripts/test_targeted_gate_ref.py
```

| Immutable diff (`git diff --binary`) | SHA256 |
| --- | --- |
| A prior freeze → new source | `f210bebb799d06dbf1f3cb6a473c00440be934e3e6dbc9fbff97961ce6ce6ab5` |
| B prior freeze → new source | `455fa692fadb98d4546087247da0d0c1b22570bce440557003942cee44f29f7e` |
| D0...A | `ccaa2f8bb4b7db27797b574b197474b77041d606acf3cb557c342119971dad83` |
| D0...B | `28498dda0dae272f9b26c47cdadb0fb12be3325b687c19609001648fa85fbccb` |

### Tests, CI and next-step boundary

F19 reports compile success and `python3 scripts/test_pr_gate_plan.py` (5 tests) on both corrected worktrees, plus `python3 scripts/test_targeted_gate_ref.py` (14 tests) for B. These are worker-reported results, not tests independently rerun by this reviewer. The five-test direct invocation must not be described as the full planner qualification suite; the predecessor still requires `python3 -m unittest scripts/test_pr_gate_plan.py` and the real-Git/negative qualification cases specified in the selector recipe.

Current exact-head CI, queried live:

- A [run 39946225153](https://github.com/mcpdev80/baseharbor/actions/runs/39946225153): workflow policy SUCCESS, focused Python SUCCESS, `PR Selected Checks` FAILURE.
- B [run 39946290135](https://github.com/mcpdev80/baseharbor/actions/runs/39946290135): workflow policy SUCCESS, focused Python SUCCESS, `PR Selected Checks` FAILURE.

Source acceptance does not override those mandatory failures or certify actionlint/full qualification coverage. The next separately assigned qualification worker may use exactly these accepted full SHAs and source-only inventories in Q. Q needs its own complete independent review and required green CI. Subsequent source branches must import the trusted predecessor through the clean-merge chain in selector step 6 and obtain valid current-input mandatory checks. Stop on source changes after these anchors, inventory mismatch, conflicting/manual integration or any failed required gate. No CI rerun, source edit or merge was performed here.

### Correction round 2 repo-rule compliance

Only this evidence file was appended. Existing worktrees, index changes, claims, Planning state, sessions and GitHub state were preserved. Pool deficits, coordinator PLAN age/open steps and idle F19 lane are coordinator-owned operational follow-ups; this bounded re-review neither reassigns nor removes sessions. Watcher events concern other lanes' idle/gone/permission states and remain unacknowledged pending actual coordinator inspection. The reviewed F19 deliverable is persisted and verified here; its idle status alone was not used as proof of success. The bootstrap pause and no-merge instruction remain in force.

END CODEX-REREVIEW-SHA-DONE
