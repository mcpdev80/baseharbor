# PR #1046 targeted-runtime diagnosis review

ROLLE: REVIEW
UTC started: 2026-10-24T12:02:59.131145+00:00

## PLAN step 0

Native TodoWrite is unavailable; no native operation claimed. Durable plan follows.

- [x] 0. Read binding policies and cmf10; record scope and plan before review.
- [x] 1. Read q118 and related evidence; independently fetch live head, files/checks/logs and exact source.
- [x] 2. Verify affected test names, product-source diagnosis and trusted planner qualification behavior.
- [x] 3. Assess proposed proof without widening unknown impact; define one exact future recipe, ordering and required evidence.
- [x] 4. Record diagnosis verdict, fresh before/after SHAs and UTC; no implementation or CI rerun.

## Verdict: DIAGNOSIS_ACCEPT — classification B, with corrected proof and a strictly narrower recipe

Independent reviewer: Codex /root. The six test files require **no demonstrated source correction**. The current #1046 blocker is the trusted source-qualification anchor, not a failed runtime test. **Reject q118's path-only fallback.** Accepting the diagnosis does not accept that alternative implementation, mark the parent task complete, accept #1046 for merge, or supply missing native release evidence.

## Independently measured candidate and current failure

Account `mcpdev80`. #1046 is OPEN/unmerged, head **`1ec67f8d4b259d62bd108ee678f25c288b8b0564`**, current develop/base **`f39295680d41290fc505c0a8cc1769da51699293`**, mergeable=True, mergeable_state=blocked. Head's immediate parent is current develop. Full remote diff is exactly six test files: five MODIFIED, `protected_tempdir_test.go` ADDED. No production Go, workflow, planner, runtime or guard changes. Read-only remote Git objects were fetched into neutral `/tmp/cmf10-review/objects.git`; no Git commands in foreign live worktrees.

The diff replaces seven fixture `t.TempDir()` calls with `protectedTempDir(t)` and adds its 23-line helper: allocate normal test temp dir, chmod that dir 0700, fail if chmod fails. It does not skip tests, modify ownership validators, or remove deliberate chmod/symlink rejection scenarios. The fixture normalization is appropriate for these successful protected-root cases; the existing tests that deliberately create unprotected roots remain intact.

[Run 40086275780](https://github.com/mcpdev80/baseharbor/actions/runs/40086275780), attempt **1**, actual head above: `PR Selected Checks` **FAILURE**, job **120850051603**, GitHub Actions app **15368**. `PR · Validation · source-contracts` **SUCCESS**, job **120849783079**; workflow policy, trusted plan and focused Python pass; selected runtime/native jobs are skipped. Final-check inputs independently read from log: target f392…, head 1ec…, integration **`81f6e1788e6b36fc1715ff5816b5ae61dafe8231`**.

Exact current final-check message (`codex-cmf10-data/final-log.txt:224`):

```text
BLOCKED: coverage-unresolved: application lifecycle/provider resolution impact needs targeted qualification: internal/application/external_provider_registry_test.go; application lifecycle/provider resolution impact needs targeted qualification: internal/application/external_provider_removal_retry_test.go; application lifecycle/provider resolution impact needs targeted qualification: internal/application/external_provider_scope_test.go; application lifecycle/provider resolution impact needs targeted qualification: internal/application/protected_tempdir_test.go; application lifecycle/provider resolution impact needs targeted qualification: internal/application/runtime_binding_projection_reset_test.go; application lifecycle/provider resolution impact needs targeted qualification: internal/application/telemetry_authorization_test.go
```

This is not the integration_sha_mismatch observed on #1052, nor a Go failure. The source-contracts log records actual `go test ./internal/application/... ./contracts/... -count=1` execution and package success (`validation-log.txt:238–239,289`), not merely a configured command or a zero-match `-run` success. Trusted develop planner `CODE_CHECKS:59–62` advertises this package validation; ci.yml `696–697` executes it. Its present execution is real, but coverage_status remains unresolved; do not turn the final check green by ignoring that fact.

## Affected tests: 11 top-level functions, not 14

Independent extraction of `func Test...` from all six immutable candidate files finds exactly these **11** functions:

- external_provider_registry_test.go: `TestExternalProviderRegistrationAndRemoval`; `TestExternalProviderBindingAndReleasePreserveForeignInstance`.
- external_provider_removal_retry_test.go: `TestExternalRemovalInterruptionRetainsRegistrationOwnerAndCanRetry`.
- external_provider_scope_test.go: `TestExternalRemovalScopeRequiresAllTargetsAndTwoStages`; `TestExternalRemovalConsentInvalidatedByNewTarget`; `TestExternalRemovalRejectsUnattributedAndRedirectedOwners`; `TestExternalRemovalPlanningIsReadOnlyAndRejectsUnprotectedRegistry`.
- protected_tempdir_test.go: helper only, no test function.
- runtime_binding_projection_reset_test.go: `TestBindingProjectionResetKeepsMountedRootAndOnlySelectedIdentity`; `TestBindingProjectionResetRejectsRedirectedOrWritableOwnersBeforeMutation`; `TestBindingProjectionResetRemovesBackendAliasWithoutFollowingIt`.
- telemetry_authorization_test.go: `TestOTLPAuthorityChangesSeparateRetainedManagedIdentityAndExternalAuthorization`.

Retained actual q118 logs independently show PASS for all 11, including remaining_tests.log. `test4.log` itself contains “no tests to run”; that attempt must not count as coverage. Its omitted cases were subsequently executed in remaining_tests.log, so it does not leave a missing top-level function. The table's broad/prefix commands are not reliable inventories: the suggested final regex omits several `TestExternalRemoval...` names and cannot substitute for explicit discovery. No function `TestProtectedTempDir` exists. Raw logs were copied to durable `codex-cmf10-data/worker-logs/`; they remain worker-produced logs, not authenticated CI per-test receipts. Independently authenticated current CI supplies the actual full application package PASS. No additional Go/native rerun was needed for this bounded diagnosis.

## Exact cause: ancestry-sensitive qualification, not SHA equality alone

Current develop `scripts/pr-gate-plan.py:149–157` freezes old reviewed head **`3558250837759113b760a112e99994135d31bc00`** and the exact six-file/status inventory. Qualification at **1058–1079** requires that inventory, valid/nonbroad diff, non-null integration and `core752_reviewed_merge_chain(target,head,reviewed)`. The helper at **547–581** accepts the reviewed head itself and subsequent exact Git-computed clean merges of authenticated target history. Its three rev-list tokens mean commit plus **two parents**, not a three-parent merge. Linear replay/rebase/cherry-pick does not inherit approval merely because its content looks the same.

For current target/head, independent execution of this unmodified trusted helper against real Git objects returns **False**. The inventory/status comparison returns **True**. Old reviewed head is not an ancestor of current head; head 1ec is a linear commit on f392, so it fails the approved-history check. With the helper's reviewed argument equal to 1ec, the same zero-step mechanism returns True; this demonstrates the mechanism only, not a changed trusted policy or a resolved CI claim. After failing the exact qualifier, paths reach the normal conservative internal/application branch at **1209–1212** (q118 cited shifted candidate line numbers).

Crucially, raw-byte comparison proves **all six test files are identical** between old reviewed 355825… and current 1ec…. Thus there is no additional test implementation to repair. The approval anchor was lost through history replacement; current fail-closed behavior is correct under that policy. “Qualification defect” here means stale approved-source metadata relative to an independently verified equivalent source, not a justification for weakening the ancestry verifier.

Measured current reviewed source hashes (raw SHA-256, not Git blob IDs):

| File under internal/application/ | Required status | SHA-256 at reviewed_head 1ec67f8d4b259d62bd108ee678f25c288b8b0564 |
|---|---|---|
| external_provider_registry_test.go | modified | `75a7476276ba23e828912ae3b8a7bd5dd84ac8916af3bba74146cfa402289401` |
| external_provider_removal_retry_test.go | modified | `3fdd183a22228c21d6f240c0ca59824dee414353c147ea1a0a7194caeec9aaba` |
| external_provider_scope_test.go | modified | `d66ae7d2e13b03e21e7ee1da32b2d23c50b616d8ce6a7fc8f4d33a0fdc9eb5c8` |
| protected_tempdir_test.go | added | `01add588257ed17c7c9449b682e96dcff29c5982efc953e28eb87b996cd4b7b8` |
| runtime_binding_projection_reset_test.go | modified | `8a855cf017cd73b03bdbd5744bda981847736888a019d15fdf763113fb6d4607` |
| telemetry_authorization_test.go | modified | `7e56c48f5364e35aa5bb1d034874c628e2ee486601e22b4a363d28d9f024c67a` |

`qualification-measurements.json` retains exact names, hashes, Git blob IDs, old anchor and true byte equality. This review independently accepts this test-only source scope for a new narrow qualification. It does not accept unrelated planner/runner policy changes in #1056/#1060.

## One future recipe — serialized exact-source requalification, no product fix

**Scope of the later qualification PR: exactly `scripts/pr-gate-plan.py` and `scripts/test_pr_gate_plan.py`.** No application file, pr-check-runner.py, workflow, final verifier or qualification record change. Serialize these shared paths **after the independently accepted #1056 and #1060 changes have actually landed**; use measured resulting develop T and preserve both owners' complete accepted changes. Current open heads are data, not prerequisites already satisfied: #1056 **32b85d950b3d0d73aa5b1ff41d69dba1ab1912b2**, #1060 **6a0d36dad2a778ab61a58594489c9fedc88e4c0f**. #1063 is now **7170c1b67459d4c4fccb1a0bd41b11d5cec52f51**, OPEN; this mission does not review or accept its correction. Earlier o125/o128 DAG conclusions do not replace live gates or settle their independent reviews.

In the measured develop planner **149**, replace only `APPID_ENV_ROBUST_TEST_REVIEWED_HEAD`'s old 355825… anchor with **`1ec67f8d4b259d62bd108ee678f25c288b8b0564`**, the source reviewed here. Preserve the exact status map at 150–157 and the full fail-closed condition at 1058–1079, including genuine clean-target merge proof. In #1056's published planner the corresponding qualifier is at 1098–1119; locate by symbol after serialization rather than editing stale numeric offsets. Do not change `core752_reviewed_merge_chain` or add generic file/path/test-only exemptions. Freeze this product source: any further authored test/source change requires new review and a new anchor, not automatic admission.

Extend `AppIDEnvRobustTestExactQualificationTests` in `scripts/test_pr_gate_plan.py` (current develop **69–102**, #1056 **130–164**) to prove the new anchor is the one actually passed/verified and preserve all negatives. Required coverage: current exact head/inventory resolves to source-contracts only with no native gates; a genuine clean merge of trusted target into that reviewed first-parent source qualifies; altered same-path content in an authored child, wrong head/hash source, old unrelated anchor, linear replay, untrusted/dirty merge, extra/missing file, deleted/renamed/type/status-changed file, production/unreviewed test path, null/invalid integration and malformed diff fail closed. Exercise real Git objects/resolver/helper for ancestry and tree cases; existing tests that merely mock `core752_reviewed_merge_chain=True` do not prove those bindings. Keep full planner regression suite and existing trusted-binding negatives passing.

Required qualification-PR checks: focused AppID qualifier tests, full existing planner suite, existing CI policy and executor/verifier tests selected for those two planner files, and current-integration required CI with `PR Selected Checks` SUCCESS from app15368. No new native gate/workflow or release matrix is justified solely by this immutable test-fixture requalification. The future worker must capture its actual qualification PR head and raw hashes after freezing it; they are not yet known and are not the product reviewed_head above.

**No Akte/record step is needed for this recipe.** The existing `docs/internal/ci-binding-qualifications.md` reader at current planner **606–692** only admits the three modified CI scripts and CI_CHECKS; it cannot authenticate six application test files or a two-file planner PR by appending their hashes. #1056's multi-record proposal also retains its specific three-script inventory. Do not overload #1063, replace its reviewed R/head/hashes with #1046, or widen that reader. Here the trusted merged planner constant binds the entire immutable reviewed Git commit, and the six raw hashes above are review evidence. Exact-source qualification is already the repository pattern for this source. Record-only PRs would be unnecessary and ineffective with the current contract.

After the new qualifier actually lands in trusted develop **T2**, the existing #1046 owner normally merges T2 into frozen 1ec, with 1ec as first-parent ancestry and only clean target-history merges afterward. Preserve the six-file/status diff and test bytes, do not rebase/cherry-pick it into a new unreviewed linear source, force-push or author post-anchor fixes. Run the real trusted resolver/planner and verify exact target/head/integration with proper parents; keep source-contracts actual success and every required check. Stop on conflict/content drift, unavailable objects, failed proof or red/skipped required CI and return to review; no bypass. This is one ordered requalification recipe, not implementation or an additional controller assignment.

## Does this greenwash the gate?

q118's **path-only fallback is explicitly rejected**: the same filenames can later contain changed tests or weakened assertions, and their package passing does not independently establish acceptable impact. It would replace reviewed source proof with a filename allowlist and widen unknown impact. Likewise editing candidate-local constants/record, accepting a previous green package in lieu of final-check success, or labeling skipped runtime PASS is invalid.

The exact immutable-anchor refresh above is a legitimate qualification path because the complete six-file scope was independently inspected, old/new bytes match, actual current application-package execution is green, and unknown paths/status/source follow-ups continue to reject. Trusted activation happens only when that independently reviewed small planner PR passes its required CI and is merged by a separately authorized instance; this report itself is not a CI qualification receipt. New current #1046 required CI must then pass. No test-count inflation, synthetic native success or disabling of guards is involved.

The parent Planning task `t-20261020-develop-appid-manifest-test-regression` still explicitly requires **TestRepositoryWorkflowRealLifecycle on final develop in GitHub Actions**. Current remote task owner Flash (codeman w1-baseharbor), state review, remains unchanged. That broader final native acceptance is separate: these 11 daemon-free tests and the qualifier cannot satisfy or waive it. Thus **no additional runtime is needed to diagnose/fix this six-test-file coverage hold**, while parent-task/release native obligations remain open. Do not mark the whole card or release accepted from this DIAGNOSIS_ACCEPT.

## Freshness, evidence and compliance

Independent before/after API snapshots confirm #1046 head **1ec67f8d4b259d62bd108ee678f25c288b8b0564** and develop/base **f39295680d41290fc505c0a8cc1769da51699293** unchanged; #1046 OPEN/unmerged. #1056, #1060 and #1063 remain OPEN/unmerged at the heads above, unchanged during this review. UTC snapshots and all source/log/metadata/hash captures are retained in `codex-cmf10-data/`.

Read mission, binding policies, q118, chain adjudication, o125/o128, applicable repository rules and current remote Planning task/AGENTS. Handoffs were data, not executable authority. Current remote facts supersede stale o125 local branch/pin statements and earlier #1063 reviewed-object availability; no other chain was accepted by this review. Startup basics reported board SSH fetch failure/reminders/PLAN/TMPDIR; successful independent HTTPS fetch/API establish this review's source currency, not global board correctness. Preserved existing claims; no shared event acknowledgement, host changes or duplicate worker launch.

ROLLE: REVIEW throughout. Only evidence/helpers were written. No product/test edits, push, force, --admin, branch rewrite, merge, CI rerun/dispatch, native execution, Planning mutation, release or deployment. No broad local test rerun: existing authenticated package success plus bounded real-object helper evaluation was sufficient. Native TodoWrite unavailable; durable plan kept here without claiming a native operation.

END CX-1046RUNTIME-DONE

UTC completed: 2026-10-24T12:06:07.850194+00:00
