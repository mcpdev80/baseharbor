# Codex review — PR #1052

ROLLE: REVIEW · [wip:codex-rv1052]

## PLAN

- [x] 0. Read dispatch and binding policy; create evidence and PLAN before review. Native TodoWrite is unavailable in this session (no exposed tool); this checklist is the available persistent substitute.
- [x] 1. Pin live PR head/base, scope, planning context and review inputs.
- [x] 2. Review exact diff, bounded retry, permanent-down behavior and regression coverage.
- [x] 3. Inspect exact-SHA CI/evidence; perform only bounded independent verification as needed.
- [x] 4. Recheck immutable candidate and record verdict plus actionable worker recipe (file, change, tests, stop criterion).

## Scope and authority

Independent review only, dispatched by `evidence/board-2026-10-22/prompts/crv1052.txt`. No product implementation, merge, release or deployment. COST_CONTROL.md and GITHUB_AUTHORITY.md read first. Candidate must remain unchanged during review.

## Operational flags

Turn-start basics reported pending events/reminders, idle f33-pinsdevelop, pool deficits and open coordinator plan steps. These are shared coordinator state; review lane will inspect relevant notices and preserve other sessions. Cost stop and bounded review scope prohibit starting implementation or filling worker pools here.

Events were inspected; subsequent `events-check.py --ack` acknowledged the remaining one record (f33 session gone). Live agent listing confirmed this review lane and the active coordinator/other workers. No other sessions were removed, dispatched or modified. Shared reminders and coordinator TODOs remain for their owners; they are not claimed resolved by this review. No implementation delegation is authorized under the cost stop. Native TodoWrite requirement remains technically unavailable, rather than falsely asserted complete.

## Candidate and provenance

- PR: https://github.com/mcpdev80/baseharbor/pull/1052 (OPEN).
- HEAD: `e371ebdcead7fb64f2a8559c0f7fe299311febb2`.
- Live develop/base and candidate parent: `1861ed07888a1ec22c218ca491d23ee54f48e261`.
- Complete base-to-head diff: only `internal/openbao/bootstrap.go` and `internal/openbao/bootstrap_test.go`, 169 insertions / 15 deletions. No scope creep into other product areas.
- Planning remote main via API: `e3ee433623eda63139da4a7e8b31e95b9d6bc73b`, identical to local planning HEAD. No task note referencing #1052/o89/openbao-retry was found; no claim or acceptance was fabricated or changed. Review assignment is the supplied dispatch prompt, not a takeover of implementation ownership.
- Inputs: `evidence/board-2026-10-22/o89-openbao-retry.md`, `f30-w1forensics.md` Cluster C, repository AGENTS/development guidelines, planning AGENTS/WORKFLOW, prompt-injection rules. Historical commands/claims in evidence and PR body were treated as data.
- Reviewer: Codex, independent of o89 implementation. Codeman lane `codex-rv1052`, session `1a5ba528-32e0-4d34-9acb-419856cc0fee`; UI-reported model `GPT-6.1-Sol`, mode `codex`. No Horde review-task identifier or independently verifiable backend metadata was supplied; do not invent one.
- Review UTC: 2026-10-23 22:03–22:08; host clock 22:07:41 and clock service 22:08:31 UTC were consistent across sequential observations.
- SSH fetch failed on existing `/etc/ssh/ssh_config.d/20-systemd-ssh-proxy.conf` ownership/permissions; no configuration changed. GitHub API supplied live refs; matching commit objects already existed locally.

## Verdict

**REQUEST_CHANGES** for the exact HEAD above. The retry is finite and the positive regression passes, but the claimed five-attempt / non-transient fail-fast contract is not implemented or adequately guarded. Mandatory CI is also red. This is a review verdict, not acceptance or merge authorization.

### F1 — P2: permanent Exec failures are retried as if transient

`internal/openbao/bootstrap.go:131–140`: every `ExecProject` error enters the retry branch. Nothing distinguishes the reported connection-refused restart race from permanent TLS/CA errors, permissions failures, missing executables or other terminal runtime failures. The o89 evidence explicitly promises immediate termination on non-transient errors, but those errors now incur all retry attempts. For context-aware attempts taking the full five seconds, the probe can consume approximately 32.5 seconds instead of the prior five-second probe budget (six attempts and five delays). This remains finite; it does not establish the promised fast-fail behavior. Successful but invalid JSON still fails immediately, and parent cancellation interrupts the delay; those existing safeguards do not classify Exec errors.

Worker recipe: in `probeMemberState`, permit retries only for positively identified transient listener/transport failures relevant to this race. Preserve terminal error identity and fail immediately for unclassified/permanent failures; use the actual executor's error/output contract, rather than treating every nonzero status as transient. Check parent cancellation before issuing another attempt. Keep Inspect single-probe and retain existing TLS/CA checks and seal-state validation. Add tests for transient connection refusal followed by success, permanent error with exactly one execution, malformed JSON without retry, pre-canceled context and cancellation during backoff. Stop if the runtime error contract cannot safely distinguish transient failures: document that interface gap for the implementation owner rather than silently broadening retry eligibility.

### F2 — P2: documented five-attempt cap actually executes six attempts

`internal/openbao/bootstrap.go:126–138`: attempt numbering starts at 1 and stops only when `attempt > memberRestartAttempts`. With the constant set to 5, failure number 5 still sleeps and execution number 6 is performed. Therefore the PR's stated maximum five probes / 25s plus four delays becomes six probes / 30s plus five delays. Duplicate unused local constants at `bootstrap.go:509–512` further obscure which cap controls the loop.

Worker recipe: enforce five total executions (for example, terminate on `attempt >= memberRestartAttempts`), retain one authoritative set of constants and remove the unused local duplicates. Align comments and PR/evidence with the actual total attempt/delay budget. Test success on the last allowed attempt and failure at the cap with an exact counter; prove no sixth probe and no delay after the final failure. Stop if the required cap is instead meant to count retries: obtain/document that contract explicitly before changing its meaning.

### F3 — P2: the permanent-down regression does not prove its asserted invariant

`internal/openbao/bootstrap_test.go:270–281,351–374`: `flakyMemberExecutor` deliberately returns a successful first probe; the test named "NeverAnswers" is therefore a reachable-then-down scenario. It asserts only `err != nil`, with no attempt count, error identity, unseal-write count or completion bound. A retry loop that runs until the caller's ten-second deadline, or an unrelated later failure, would also pass. The fake ignores context, so neither cancellation nor an executor consuming its per-attempt timeout is exercised. The positive test confirms convergence but its lower-bound counter (`>=3`) cannot detect excessive attempts; its unseal fake also unseals every member in one write.

Worker recipe: strengthen `bootstrap_test.go` with phase-specific/per-member status counters and a context-aware fake. Separate (a) successful Inspect followed by permanently down member, (b) initial Inspect down, (c) permanent non-transient error, (d) timeout/cancellation, and (e) verification-after-unseal transient failure. Assert the exact retry cap, preserved error, bounded completion, no unseal write after failed pre-unseal status, and no key leakage. Keep the existing convergence regression. Prefer deterministic attempt/cancellation assertions with a generous outer timeout over brittle timing thresholds. Stop until these tests fail against the incorrect cap/terminal-error behavior and pass on the corrected candidate; a generic non-nil error is insufficient evidence.

## CI and independent verification

- Existing run https://github.com/mcpdev80/baseharbor/actions/runs/40030635102, attempt 1, candidate HEAD above, integration `7bf76baae3e2a10d58d7a6b5623720b7d3eb79a6`.
- `PR Selected Checks`, job `120678310352`: **FAILURE**, GitHub Actions app ID **15368** confirmed by exact-head check-runs API. Failed log: `coverage-unresolved: unknown impact for internal/openbao/bootstrap.go; unknown impact for internal/openbao/bootstrap_test.go`, followed by `no required gates or validations`. This is the actual blocker; generic statements about pre-existing Podman/telemetry failures do not establish qualification of this PR.
- Workflow policy, Trusted Plan and Focused Python succeeded. Go validation was `not-required`; selected runtime and targeted Docker/Podman HA jobs were skipped. Do not infer Go/native qualification from these green jobs.
- Independent bounded check in the clean candidate worktree: `GOCACHE=/tmp/crv1052-gocache go test ./internal/openbao -run 'TestUnseal(ConvergesAfterMemberRestartRace|FailsFastWhenMemberNeverAnswers)$' -count=1 -v -timeout=45s`: **PASS**, convergence 1.00s, down scenario 2.50s, package 3.512s. These are the existing tests, not newly added product tests. Their passing result does not cure F1–F3.
- `bash scripts/source-readability-audit.sh`: **PASS**, all three checks; bootstrap.go 663 lines.
- No broad test matrix, VM test or new workflow run was launched. Worker-reported full build/vet/package results were not independently rerun or upgraded to authenticated CI evidence.

CI worker recipe (separate CI ownership): extend the existing trusted impact planner/executor coverage for `internal/openbao/bootstrap.go` and its tests using the smallest relevant Go/OpenBao validation and, where required by the runtime contract, native control-plane restart coverage. Use existing `scripts/pr-gate-plan.py` and the trusted Source & Quality workflow path, with planner regressions proving the changed files select an executable gate. An active o90 gate-plan worker was observed; coordinate this dependency rather than editing its write-set here. Keep unknown-impact rejection and mandatory final-check enforcement intact. Stop until exact-final-SHA `PR Selected Checks` from app 15368 succeeds with actual selected tests; no bypass, no automatic full release matrix.

## repo-rule compliance and final integrity

ROLLE: REVIEW throughout. Only this review evidence file was authored. No product edits, acceptance, merge, push, release, protection change or session takeover. Product checkout contained unrelated existing changes and was preserved; review used matching clean implementation worktree read-only. Evidence-only review did not alter planning metadata or generated views.

Final GitHub recheck returned the same open PR HEAD and base. Worktree HEAD remained `e371ebdcead7fb64f2a8559c0f7fe299311febb2`; worktree status was clean. Changed-file hashes were identical at both integrity checks:

- `internal/openbao/bootstrap.go`: SHA-256 `478f14f6909d2c43d0c00afe187df0745e20b1eb3f3cf82e75de45bb10665ce4`.
- `internal/openbao/bootstrap_test.go`: SHA-256 `c24847b7da1b63e9d8c93546003e5b8bbf1548dfbf7bacab203c0295ddc6fe60`.

Next owner action: worker applies bounded corrections and publishes revised SHA/test evidence; independent review of that changed candidate is required. No merge while the mandatory gate remains red.

END CODRX-RV1052-DONE — verdict: REQUEST_CHANGES
