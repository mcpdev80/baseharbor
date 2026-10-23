# Independent final review: Podman recovery pair

- Reviewer session: `01a15816-c04a-7582-9355-bdf99453156f` (CODEX_THREAD_ID).
- Model: GPT-6 / Codex, per this session's supplied identity. An exact deployment/model suffix is not exposed; this report does not assert an independently verified GPT-6.1 deployment.
- Timestamp: 2026-10-20T10:53:37Z.
- GitHub read identity verified through the connector: `mcpdev80`, user `22537373`.
- PR #995 reviewed HEAD: `0af3982bdacd2a4120a3109d8585bc2dcd2b39d1`.
- PR #990 reviewed HEAD: `50ad8d72c72a368bf865e42cc0c43d9fd8285a12`.
- Authenticated live base for both: `develop`, `bb91fbc327414952b36a88271153db43970280cc`.
- Live PR integration SHAs reported by GitHub: #995 `677c938bc1f231f7a48cdbe641b74e916d1173d2`; #990 `8d7f8b8467aff0c11e41bf35f41119d8818735ed`.

## Per-PR verdicts — verbatim

**PR #995 at 0af3982bdacd2a4120a3109d8585bc2dcd2b39d1: CHANGES_REQUESTED.** The fixture preserves substantial recovery, consent and redaction assertions, but this candidate moves CI qualification into PR-controlled source, introduces reusable workflow coverage exemptions, breaks existing focused workflow gates and contains malformed action pins. The cited native atomic jobs do not execute the required Go recovery fixture or its automatic cleanup. This candidate is not ACCEPTED.

**PR #990 at 50ad8d72c72a368bf865e42cc0c43d9fd8285a12: CHANGES_REQUESTED.** This stacked candidate includes the same eight-file product/CI change set and inherits every blocking finding below. Merging #995 into this branch supplies workflow definitions, not evidence that the required opt-in Go disaster-recovery test executed. This candidate is not ACCEPTED.

## Blocking findings, applicable to both PRs

1. **P1 — Restore the trusted CI policy boundary.** `.github/workflows/ci.yml:444` changes the `pr_plan` checkout from authenticated `base.sha` to candidate `head.sha`. Lines 456–460 then execute the candidate's diff resolver, planner and check-runner matrix implementation. This lets the candidate define its own coverage and required validations. `persist-credentials: false` does not establish policy trust. The old base checkout is deliberate protection, not a fixture bug. Keep qualification/executor policy on authenticated trusted source; any prerequisite policy update needs its own reviewed integration rather than executing unqualified PR policy to obtain green checks.

2. **P1 — The +96-line planner change is not immutable qualification.** `scripts/pr-gate-plan.py:282`, `:784` and `:844` resolve fixture paths and either of two powerful workflow files by reusable path membership. There is no reviewed-head/content binding for this exemption. A change to `ci.yml` becomes resolved whenever any listed fixture accompanies it, irrespective of the workflow's actual changes. I reproduced this without editing files: a synthetic diff containing only modified `.github/workflows/ci.yml` and modified `cmd/baha/app_backup_runtime_fixture_test.go` returns `coverage_status=resolved`, no unresolved coverage and the two atomic backup/restore gates. Those gates neither qualify arbitrary CI trust changes nor execute the changed Go fixture. Replace the exemption with independently reviewed, content-bound coverage and the actual required fixture executor. Adding tests for this boundary belongs with that correction. The exact-set branch at approximately line 608 is also a path-set mechanism, not an immutable source qualification.

3. **P1 — Required native fixture execution remains unproven.** I fetched live job lists for runs [39397436026](https://github.com/mcpdev80/baseharbor/actions/runs/39397436026) and [39407200224](https://github.com/mcpdev80/baseharbor/actions/runs/39407200224), and full Podman logs for jobs [118544037959](https://github.com/mcpdev80/baseharbor/actions/runs/39397436026/job/118544037959) and [118576338993](https://github.com/mcpdev80/baseharbor/actions/runs/39407200224/job/118576338993). These jobs run the demo atomic backup/restore journey with a prebuilt candidate binary. Their logs contain `>>> demo-backup-restore`, and contain neither `go test` nor `TestAppBackupRestoreCLIRealDisasterRecovery`. Neither run's job list contains `Application backup · podman`. The new `app_backup_restore_podman` job runs only on a separate `workflow_dispatch` with its particular gate input. Therefore these successful atomic jobs are useful product evidence, but do not prove this changed test fixture, SQL/secret/token/certificate assertions, or its registered automatic owned cleanup. Supply actual exact-source execution of the named Go test with explicit Podman selection and successful automatic cleanup, preserving foreign resources. Keep Docker fixture evidence distinct and inspectable. Q17/Q18/Q19 conflate these executors; their acceptance claims must be corrected.

4. **P1 — New job placement makes existing focused gates unreachable.** In `.github/workflows/targeted-integration.yml:582`, the new job begins before the existing `Metrics ingestion`, `OpenBao backup and restore`, PostgreSQL backup and subsequent focused steps. YAML parsing confirms those steps now belong to `app_backup_restore_podman`, whose job condition requires `inputs.gate == 'app-backup-restore-podman'`. Their own conditions require other gate values, so they cannot execute. The original `focused` job no longer contains them. Put the complete new job outside the existing job's step sequence and preserve existing gate discovery/execution. Also exclude the Podman gate from the broad Docker `focused` job condition to avoid launching an unnecessary Docker preparation job for a Podman-only request.

5. **P1 — Malformed immutable action references break the native execution route.** `.github/workflows/targeted-integration.yml:561` and `:608` use `actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef5f85d4a467287`, a 46-character hex string instead of the existing 40-character commit pin `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e`. Restore the verified immutable action pin in both places. Green checks from another workflow/job do not validate this newly added dispatch route.

## Contract and scope assessment

The canonical planning cards are currently `review`, not accepted. I acknowledge the coordinator's correction of the premature closed/accepted publication and the user-authorized claim transfer. Those history corrections are not acceptance evidence. #995 also lists unfinished planning prerequisites; they must be resolved honestly before final task closure.

| Contract requirement | Evidence and assessment |
| --- | --- |
| Native Target selection and selected state authority | Source explicitly configures the selected runtime Target and includes Docker/Podman regression cases. No new production runtime resolver is introduced. |
| Real restored SQL data and consumer access | The Go fixture retains protected consumer binding/TLS checks and SQL seed/readback. Required native execution of this fixture is missing from the supplied runs. |
| Secrets and regenerated runtime identity | The Go fixture retains secret comparison and fresh token/certificate assertions. Their execution is not established by the cited atomic jobs. |
| Ownership, consent and scoped cleanup | Source uses typed operation consent and registers bounded cleanup before startup. Unit regressions cover cancellation and foreign file sentinels. Real automatic native cleanup still needs the actual fixture run; a runtime reset in an atomic workflow is not that proof. |
| Bounded diagnostic output | The diagnostic helper uses fixed caller/owner/error categories; added tests check private sentinels, typed nils and wrapped failures. No new blocking diagnostic finding identified. |
| No production drift | No production recovery/Core/OpenBao owner implementation is modified. However `native_fixture_cleanup_diagnostics.go` is a regular compiled Go file, not a `_test.go` file, and the CI planner/workflow changes alter enforcement. Calling the entire diff harmless test-only scope is inaccurate. |
| Trusted targeted CI | Both cited runs have 17 successful jobs, including `PR Selected Checks`. The source/evidence defects above remain blocking despite these green results. No full release matrix or repeated native run was launched for this review. |

## Stack and merge-order ruling

`0af3982b` is an ancestor of `50ad8d72`. Direct comparison of the heads differs only in three documentation files already integrated into the current develop base. Both PRs' complete diffs against current develop have the same eight paths, statuses and file contents. Thus neither represents an independently mergeable disjoint slice at these heads.

**After correction, fresh independent acceptance and actual green mandatory CI, integrating the complete #990 superset first is a valid way to land both tasks' implementation once.** Then verify the resulting remote develop content, merge commit and remaining #995 diff, and close #995 as incorporated if it has no residual change. Publish each planning task's own acceptance/evidence only after its criteria pass. There is no reason to manufacture a second empty merge.

**Do not promise that squash-merging #990 automatically marks #995 merged or automatically closes it.** Squash does not preserve the original branch commits as target ancestry. Empty content and GitHub's merged/closed state are separate observations; inspect them and explicitly close the redundant PR if necessary. #995-first would also incorporate the shared implementation, leaving #990 redundant at these particular heads. The task dependency order and explicit content evidence matter more than PR number order. Neither merge is authorized by this negative review.

## Verification and integrity

- Read canonical task contracts, release context, repository rules, Q17/Q18/Q19 and current eight-file diffs. Queried live PR metadata and CI job lists through the connected `mcpdev80` account.
- Used bounded read-only Python planner probes, YAML parsing and `git diff --check`; no Go/native test reruns or corrective implementation.
- Both worktrees were clean, and all eight reviewed working files were checked byte-for-byte against their immutable candidate blobs after inspection. Per-file SHA256 values agree between the two candidates.
- Canonical inventory digest for both: `67cbbdd4b49eee1ed6b0b50bfb8dca67f710a2b8c203f19bd08317e220333c32` (SHA256 of UTF-8 JSON `{path: sha256(file_bytes)}`, sorted keys, compact separators).
- Reviewed workflow SHA256: `ae6993af9a379d70346a0beaa887934078998f4e5a8c8ad7c3e862f7e9a7b902` (`ci.yml`); `5d3576796ebb262ebbbc16f3c3fb979e35dc800a4ada639e74b6428709a5b19d` (`targeted-integration.yml`). Reviewed planner SHA256: `b24b6c3907df8b34779d5030c1f0478aa1d8141d75033e61ecf448dcf37e99ca`.
- Local `gh auth status` reports invalid CLI credentials. Connector reads succeeded; no credential repair or alternate account was attempted. Check-run application ID 15368 and current effective merge rules were not independently verified through the connector's job-list surface; this report grants no merge clearance.
- This review writes only the requested verdict artifact. No product edits, GitHub writes, merges, planning publication, shared runtime changes or production actions were performed.

CODEX-PODMAN-DONE
