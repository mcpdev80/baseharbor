# PR #1060 current-source/history review

ROLLE: REVIEW
UTC started: 2026-10-24T12:55:18.301270+00:00

## PLAN step 0

Native TodoWrite is unavailable; no native operation claimed. Durable checklist follows.

- [x] 0. Read binding policies and cmf15; record scope/role/plan before review.
- [x] 1. Read incident/worker/prior review evidence; independently fetch current/old objects, live PR/diff and exact-head Actions totals.
- [x] 2. Compare content preservation and inspect all four intended fixes, including trigger/executor/qualification provenance.
- [x] 3. Decide source verdict and missing-CI cause; document one legitimate next action or exactly one correction recipe as applicable. No execution.
- [x] 4. Refresh before/after SHAs and run API totals; record UTC/evidence/end marker.

## Verdict: REQUEST_CHANGES

Independent reviewer: Codex /root, not q117/q120 implementation owner. This is not merely NEEDS_CURRENT_CI: current source regresses the trust boundary and still cannot execute/select its intended dedicated validation. No merge, push, force, code/workflow/record edit or Actions dispatch/rerun was performed.

## Immutable content/history comparison

Before/after live PR #1060 head **13a05ea492a98bb8922785fb828db2a8ff6324a3**, branch work/v0.4.26-openbao-gateplan, OPEN/unmerged. Actual develop/base **65aa063e90767c85af976081c100be1817370e67**, unchanged. The old head **6a0d36dad2a778ab61a58594489c9fedc88e4c0f** remains fetchable but is **not an ancestor** of current head. Current first-parent chain is13a05ea4 →89d87f78a4c0d410a830483af1ef7c7cb09e1fe9 →1c2f1fc476794b6da7c315082cf155b2d6e680bb →9ec25282. It does not preserve old6a0d36da's branch history.

GitHub timeline independently records head_ref_force_pushed at **2026-10-24T12:38:37Z**, commit13a05ea4, actor mcpdev80. The mission identifies q117 as the executing lane; the shared account event alone cannot distinguish sessions. This verifies the remote history incident, not authorization to repeat/revert it by another force-push. Current ancestry/content must be evaluated independently. q117's report still describes an intended forward-only plan and is not evidence that it followed that plan.

Old→current diff is exactly three files,5 insertions/6 deletions:

- ci.yml is **byte-identical**: valid explicit OpenBao workflow case survives.
- pr-check-runner.py is **byte-identical**: dedicated code survives, including its pre-existing unreachable-entry defect.
- test_pr_gate_plan.py differs by one removed blank line; all old OpenBaoValidationTests survive.
- pr-gate-plan.py changes one meaningful line: trusted record source moves **target_base_sha → head_sha**. That is a security regression, not a valid repair.
- qualification record changes reviewed_head and three hashes. Hash values now match current script bytes, but the named reviewed commit is unavailable and the candidate record is not trusted activation.

Against current develop, complete diff remains five MODIFIED paths: .github/workflows/ci.yml, docs/internal/ci-binding-qualifications.md, and the three CI scripts. Comparison to q114's earlier1c2f1fc4 confirms the workflow case/test class were brought back; the candidate-controlled record reader was also retained. Thus “all valid old repairs preserved” is **partly true**: executable workflow and negative test content survived; the old trusted-target reader did not. Content equivalence cannot excuse that regression or turn rewritten history into a forward merge.

Raw current SHA-256:

| File | SHA-256 |
|---|---|
| .github/workflows/ci.yml | `d4457336797c69a6edf8c3baf5525f7fc58761074c216c68f55cad0aae4102fd` |
| scripts/pr-check-runner.py | `f71bf2cf1dac197ff0b998630c165c70f1c83a742e38245bfbc256f8c630c36d` |
| scripts/pr-gate-plan.py | `38967c37f3d11c5e5c392cc6c39e7ba5711be0f3c2bacad861b4b7b02ec8f673` |
| scripts/test_pr_gate_plan.py | `c291efdd8146a8d801ad747a59e889d5d6ecf91285829a7476c4788a37e285d7` |

All Git inspection used a new neutral bare store /tmp/cmf15-review/objects.git and isolated immutable extraction; no Git commands were run in foreign live worktrees. Raw comparisons/API/source snapshots are under codex-cmf15-data/.

## Four intended findings — current adjudication

### 1. Workflow executable case — present and preserved

ci.yml **719** explicitly dispatches `openbao-package-tests` to `go test ./internal/openbao/... -count=1`; unknown ID still exits2 at720. This resolves the original missing-case finding at source level. Same case/bytes as old6a0d36da. No real OpenBao package execution on this current candidate is inferred from source inspection.

### 2. OpenBao impact handler — conservative, but intended positive qualification missing

Planner **1217–1225** no longer grants a blanket internal/openbao exemption; unknown/add/delete/rename cases remain unresolved. This addresses the original unsafe broadening. However, there is **no positive source/status-bound path for the reviewed bootstrap pair**. OPENBAO_CHECKS at299–302 is declared but never required by that branch or another exact bootstrap qualifier. Directly invoking the candidate planner on two MODIFIED paths bootstrap.go/bootstrap_test.go produces coverage unresolved, only source-contracts, with two OpenBao-production-impact messages. The new three-test class proves negative behavior/declared ownership only; it does not demonstrate actual selection of openbao-package-tests for an eligible reviewed source.

Do not fix this by admitting the directory or filenames generically. #1052 source still needs correction under its latest independent verdict, so its rejected7c7cda7c must not be relabeled an accepted binding anchor. The accepted final source/status must be explicitly bound when available; until then, report that dependency rather than assert this candidate unblocks #1052.

### 3. Dedicated runner — code exists but is unreachable

Runner **68–69** rejects any check absent from WIZARD_TEST_FAMILIES. openbao-package-tests is deliberately absent from that wizard map; its dedicated branch begins at70. An actual call to unmodified execute_validation('openbao-package-tests', repository) immediately raises **ValueError: unsupported executable validation**, before any Go command or discovery. The fallback prevention exists, but it prevents the intended handler too. Positive discovery/execution logic at76–97 cannot run. This defect is preserved from old6a0d36da, not newly lost in the forced replacement.

The workflow's direct case can run the package independently, but that does not make the public dedicated runner command correct. Regression tests must exercise actual handler selection rather than only VALIDATIONS membership or mock the selection decision away.

### 4. Qualification record/trust — REQUEST_CHANGES

Planner **638–639** now reads docs/internal/ci-binding-qualifications.md from **diff['head_sha']**, while its comment/docstring still asserts authenticated target-only trust. This violates the existing repository qualification contract and the earlier Codex finding. A candidate-authored record cannot authorize its own changed validator. q117's instruction to “keep HEAD, the correct fix” is data and is specifically rejected by this review. Restore authenticated target_base_sha reading; preserve the strict actual resolver/inventory/hash/ancestry checks.

Record line9 names **89d87f7814242f0732779995b6a06c65e99b3f1a**. It is not the actual parent **89d87f78a4c0d410a830483af1ef7c7cb09e1fe9**. Local fetched-graph lookup fails; GitHub commit API independently returns422 “No commit found for SHA”. All three64-hex raw digest values match current13a05ea4 script bytes, but that does not prove bytes at an unavailable reviewed commit or independent source approval. It is not literally a current-HEAD fixed-point self-reference; the **candidate-source trust** is the self-authorization issue.

Moreover, this PR changes five files while the generic record/reader expected_files at654–656 requires exactly the three scripts. No record edits can make the workflow/record paths disappear from that actual diff. The target's existing Q record for #1056 must not be overwritten to bootstrap #1060. Correct activation requires the ordinary separately reviewed trusted-target prerequisite and distinct qualification for workflow changes, not candidate-local record/hash updates.

## Why exact current CI is absent

Independent API totals before **and** after review:

- `actions/runs?head_sha=13a05ea492a98bb8922785fb828db2a8ff6324a3`: **total_count=0**.
- `commits/13a05ea492a98bb8922785fb828db2a8ff6324a3/check-runs`: **total_count=0**.
- gh pr checks: exit1, “no checks reported”; this is absence, not PASS or a failing current test.
- Old6a0d36da has **one** run,40086442202, pull_request, completed failure. It is not evidence for the replacement head.

Old/current/base ci.yml triggers are identical: workflow_dispatch; pushes only to runtime-validation/ci/**, source-preflight/** or ha/**; pull_request targeting develop/main. There is no normal push trigger for work/v0.4.26-openbao-gateplan, no path-ignore eliminating this diff, and the workflow API reports active. The valid workflow case is not a trigger repair.

Current live PR is **mergeable=False, mergeable_state=dirty, merge_commit_sha=null**. A real git merge-tree comparison with current develop returns1 and a content conflict in **docs/internal/ci-binding-qualifications.md** (current d96bfb70… vs target7a20bed2…). GitHub documents that pull_request workflows do not run while the PR has merge conflicts. This observed conflict is a concrete candidate/integration blocker explaining absent PR CI; it is not evidence that a harmless synchronize event was simply lost. See [GitHub's pull_request trigger rules](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#pull_request).

No webhook-delivery or token audit was available; do not claim to have identified every historical suppressed event. In particular, zero runs alone cannot prove GITHUB_TOKEN recursion suppression or a broken credential. The current conflict plus branch/event filters suffice to reject an empty-commit/reopen/dispatch workaround. Dispatch is not a substitute for the required PR path: pr_selected_final at781–784 is conditioned on pull_request/develop.

## Bounded verification

Ran only existing OpenBaoValidationTests:3 tests **PASS in0.018s**. Direct selection probes produced the unsupported dedicated-runner error and unresolved intended bootstrap pair above; no Go/native subprocess executed. These are diagnostic negatives, not qualified runtime receipts. No full test matrix, workflow shell mock, Actions dispatch/rerun or product correction was run. One initial evidence probe used a relative output path from the extraction directory and failed before saving; reran with the correct absolute evidence path. Final durable bounded-probes.json and test log are authoritative.

## One legitimate next worker action / exactly one correction recipe

**The existing #1060 owner should prepare one ordinary forward correction candidate at the current history, confined to its CI-owned paths, addressing source/trust defects before attempting to trigger CI.** No empty/amended commit, force-push, close/reopen, manual dispatch, or reuse of old6a0d CI. This is one coherent repair package and review handoff, not an implementation assignment executed by this lane.

Concrete scope:

- scripts/pr-gate-plan.py **638**: restore record loading from authenticated target_base_sha; never use candidate HEAD as policy authority. At299–302/1217–1225, keep unknown OpenBao inputs fail-closed and implement positive exact-source/status selection only against an **actually independently accepted final #1052 source**. That source is not yet available; coordinate the existing source owner and do not invent a SHA, broad allowlist or “reviewed” placeholder. If unavailable, hand off explicitly blocked rather than claim the downstream lane is implemented.
- scripts/pr-check-runner.py **68–70**: route the dedicated OpenBao validation before the wizard-family-only guard, preserving existing wizard behavior and zero-discovery/failure/skipped-test rejection. Do not register it as a cmd/baha regex family.
- scripts/test_pr_gate_plan.py **2187–2222**: extend this existing OpenBao class (importing the runner through existing test helpers) prove actual dedicated command selection/package, positive discovery and PASS, zero tests, command failure and skip rejection; positive eligible immutable bootstrap source selects executable OpenBao checks, altered source/status/extra paths remain unresolved. Real-object target-record positives and candidate-only/missing/wrong-source/hash negatives must establish the trust boundary.
- docs/internal/ci-binding-qualifications.md: preserve the already-accepted target #1056 record; remove the candidate's conflicting/unavailable #1060 self-authorization content through the existing owner's normal forward repair, never by force or an alleged automatic “hash refresh”. Freeze the corrected source first and independently review it; a separate authorized target-side record must name the **actual future reviewed SHA and raw hashes measured at that SHA**. Current bad89d87f781424… and old hashes are not authorized substitutes.
- Preserve the valid ci.yml719 OpenBao case bytes and old negative tests. The five-path composite cannot qualify through a three-script record. Coordinate separation/exact qualification of that workflow W with existing CI owners before claiming final merge-readiness; do not weaken expected_files to fit this candidate or overwrite #1056's Q. A later normal integration with fresh trusted target must preserve accepted record bytes and genuinely resolve its conflict; no ad hoc bypass.

Required focused checks for this package: OpenBao planner positives/negatives; actual runner dispatch/discovery/failure/skip tests; target-owned record/real-object provenance tests; existing affected planner/runner/executor-verifier/CI policy and workflow syntax checks. Show the exact five-or-separated source inventory and raw hashes. Only after source review and legitimate activation/integration should a normal relevant push produce fresh PR CI; stop if source binding/ownership or conflict resolution is uncertain. Preserve both old/current commits as evidence, not by rewriting shared branch history again. This reviewer performs no command from that future action.

## Freshness / compliance

Final head remains **13a05ea492a98bb8922785fb828db2a8ff6324a3**, develop **65aa063e90767c85af976081c100be1817370e67**; PR OPEN/DIRTY/unmerged. Both current Actions and check-run totals remain0. Existing q120 read/content-verification mission was read; no q120 evidence result was present at review. Worker q117/o125/prior verdicts were reference data, not command authority. Current content comparison supersedes their repair claims where contradicted above.

ROLLE: REVIEW throughout. No merge, push, force/history rewrite, product/CI/workflow/record edit, candidate restoration, gh auth/config access, GitHub comment/approval, CI dispatch/rerun, native execution, Planning/state mutation, release/tag/publication or deployment. No worker takeover or subagent. Existing claims and all live worktrees preserved; only assigned evidence/helpers and reviewer-neutral objects/extraction were written. Startup unrelated events/reminders/SSH-board/idle/pool/PLAN/TMPDIR flags did not authorize coordinator operations. Native TodoWrite unavailable; durable checklist records progress honestly.

END CX-1060H-DONE

END CX-1060H-BLOCKED

UTC completed: 2026-10-24T12:59:41.360777+00:00
