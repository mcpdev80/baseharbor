ROLLE: REVIEW

## PLAN

- [x] 0. Read ctb4.txt with cat and binding bootstrap policies; record bounded independent REVIEW plan before source inspection.
- [x] 1. Pin requested/live #1052 head, develop and both affected-file SHA256 hashes; read immutable diff without foreign worktrees.
- [x] 2. Assess each of the four prior recipes against the actual per-member fake, writes, failure scenarios and retry-enabled permanent-error tests.
- [x] 3. Inspect mutation evidence and current CI/#1060 prerequisites without running product tests, CI or reruns.
- [x] 4. Recheck head/file hashes; record head-bound verdicts and shortest actionable route to ACCEPT, or drift limitation.

Native TodoWrite is unavailable in this session; no native-tool/GUI execution claimed. The mission has no numbered PLAN step 0; this setup step records the user's additional requirement.

Reviewer: Codex, current API session, independent of implementation. Exact model/backend identifier, session UUID and Horde task identity are unavailable, not invented. Requested source bdd677a3b36624f5e6dd7549986f6688730e7dfb. Only this evidence file may be written; no implementation, product tests, CI dispatch/rerun, merge, PR/Planning mutation or foreign-worktree access.

## Head-bound verdict: REQUEST_CHANGES

The four recipes are **partially implemented**, not fully satisfied. Credit: per-member storage replaces global answered; writes parse BAO_ADDR and update one member; the down test asserts exactly five pre-unseal failures; the permanent-error test now reaches retry-enabled probing. Remaining assertions/fake behavior still cannot establish the required phase/member and no-write/post-verification invariants. No result below applies automatically to a later classifier correction or changed head.

## Identity, scope and source integrity

UTC initial hashes **2026-10-24 08:47:34**; final live head/hash recheck **08:48:49 UTC**, host NTP synchronized. PR #1052 remains OPEN at **bdd677a3b36624f5e6dd7549986f6688730e7dfb** before and after. API base field remains a2abc17a8057ba972cb9ed481fad229fe9f12824; actual live develop is **f39295680d41290fc505c0a8cc1769da51699293**. Review used the immutable diff against that develop source and pinned `git show`/Contents API bytes, never a foreign checkout. Diff is limited to bootstrap.go and bootstrap_test.go (560 insertions/15 deletions against the current merge base).

| File | SHA256 before = after |
| --- | --- |
| internal/openbao/bootstrap.go | a2f8ced7b762ca4c605d96c75b75130ab883ada3865611ea4238a79c2a3ff26d |
| internal/openbao/bootstrap_test.go | d7affcd6f0070ad5a11873d1fa04beb879e0fd4abc28bdf841c8a5b07745aa91 |

Production bootstrap.go is byte-identical to the preceding reviewed 736d73c6 version. No transient-classification/cancellation production correction is present in this frozen candidate. All line references below are at bdd677a3.

## Verdict per recipe and actionable remainder

### 1. Per-member phase script — PARTIAL / REQUEST_CHANGES

`bootstrap_test.go:262–355` now stores haMemberState per service. However `preUnsealAttempts` increments only on failures at lines345–348, including failures during initial Inspect. A successful pre-unseal attempt increments no pre-unseal counter; the first successful status at lines351–354 is called Inspect even if it actually occurs later. `ctx` is unused. This is per-member storage, but not an explicit phase script.

Concrete trace through the current positive test: member-1 transientWindow=3 (line438) already fails its initial retry-disabled Inspect. Inspect then obtains a sealed fallback from members2/3 (`bootstrap.go:100–115`). Member-1 later fails twice during pre-unseal and then succeeds. The test's `preUnsealAttempts>=3` includes one Inspect failure, while its `inspectCount` is first set by the successful pre-unseal response. The assertions therefore accept precisely the phase confusion the recipe intended to exclude.

**Existing test owner, file/function:** replace this implicit classification in flakyMemberExecutor.ExecProject with explicit per-member scripted phases, including separately counted successful/failed initial Inspect, pre-unseal and post-write probes. Honor context and reject unexpected member/phase instead of silently inventing a fallback member. Assert exact phase counts for all participating members; include a pre-unseal success on the last allowed attempt. **Test requirement/stop:** removing the phase distinction or counting initial Inspect as a retry must fail the regression. Existing integer fields alone do not satisfy that criterion.

### 2. Per-member writes and post-unseal verification — PARTIAL / REQUEST_CHANGES

`ExecProjectInput:377–417` does parse BAO_ADDR and increments the chosen member's writes before unsealing that member; this is a real improvement. But the positive test at lines452–470 checks **member-1 only**. The q111 evidence says member-2/3 write counts are asserted; the source contains no such assertions.

Post-write `ExecProject:335–341` unconditionally succeeds, so there is no scripted transient failure after a write. Moreover admin gateway status at lines297–304 aliases an arbitrary unsealed member's actual state pointer; the same postUnsealVerification/verifiedAfterUnseal fields can then be incremented by the admin gateway probe, rather than the intended member verification. Gateway readiness must not serve as a substitute for that phase.

**Existing test owner, file/functions:** in TestUnsealConvergesAfterMemberRestartRace assert exact per-member write/verification counts and successful verification for members1/2/3. Keep gateway checks separate from member counters. Add a transient post-write status failure and recovery on a named member. Make unknown/missing destination fail the fake instead of the arbitrary-state fallback at lines398–414. **Test requirement/stop:** a mutation making one write unseal all memberStates must fail on missing member2/3 writes; removing any member's verification must fail independently of later admin readiness. No new product implementation is requested by this test recipe.

### 3. Down scenario, error identity and no writes — PARTIAL / REQUEST_CHANGES

`neverAnswersExecutor:501–509` plus TestUnsealFailsFastWhenMemberNeverAnswers now separate one successful initial status from later failures and assert exactly five pre-unseal attempts. Credit that corrected cap assertion; it is not the historical six-attempt bug.

However its ExecProjectInput at lines512–514 simply returns success and never records writes or submitted input. The test's `ms1.writes==0` assertion at lines554–556 is consequently **vacuous**: an erroneous unseal write would leave it zero. The error assertion is still only err!=nil (lines541–544), with no final transport error identity. There is no separate initial-Inspect-down case or context-aware attempt/deadline script. Global inspectDone is sufficient for this one-present-member fixture, but does not establish a general per-member phase contract.

**Existing test owner:** make neverAnswersExecutor.ExecProjectInput record every invocation/key submission or fail immediately; assert zero actual writes. Use a retained transport error and assert its identity through wrapping. Add a distinct initial-Inspect-down case and context-aware bounded completion/cancellation proof. **Test requirement/stop:** injecting a write before successful precondition or substituting an unrelated error must fail; exactly-five assertions must count pre-unseal attempts, not all lifecycle calls.

### 4. Permanent error in enabled retry path — ACCEPT for relocation; PARTIAL for full recipe

The principal historical defect is fixed: `permErrorExecutor:621–628` first returns a successful member status, then returns x509 during pre-unseal probing. The test at lines595–602 asserts retryAttempts==1 and x509 in the returned error. Returning the permanent failure during Inspect instead would leave retryAttempts zero and fail; removing terminal classification would allow five attempts and fail. These two specific stop conditions are now supported by source logic.

Remaining requirements: permErrorExecutor.ExecProjectInput at lines631–632 does not record/reject writes, so zero writes is not guarded. The new cancellation test at lines688–716 verifies context.Canceled, but its fake ignores context and records no executions; it cannot prove no extra call after cancellation or distinguish pre-canceled from backoff cancellation. The invalid-JSON fake increments callCount (line731), yet TestProbeMemberStateInvalidJSONNoRetry at lines743–764 never asserts it; retrying malformed JSON five times and returning the same error could still pass. Neither counter is a no-retry proof until asserted.

**Existing test owner:** retain the now-valid retry-enabled permanent case and add zero-write/key assertions. Assert invalid JSON callCount==1. Use a deterministic first-attempt/backoff synchronization and call counts for cancellation; include pre-canceled/deadline cases and no subsequent call. **Stop:** wrong-phase permanent error, multiple terminal/JSON attempts, any write after failed precondition or extra canceled-context probe must fail the corresponding test. Unused permanentDuringRetry and auxiliary state fields are **nonblocking cleanup**, not separate reasons to reject this candidate.

## Mutation evidence: not demonstrated as executed

`evidence/board-2026-10-24/q111-openbaotests.md` supplies a single package-pass snippet (`go test ./internal/openbao/... -count=1 -timeout=120s`, 4.539s), but no exact tested SHA, mutation patch/hash, command/output or failing-test receipt for the four proposed mutations. Its mutation sections use conditional descriptions, not captured runs. In particular its member2/3 assertion claim contradicts the source, and its description of the old fake's initial failure conflicts with the old answered-gated behavior already recorded in the prior verdict.

Therefore no four executed/killed mutants are credited. The two recipe-4 control-flow assertions above can be assessed positively from source, while recipes1–3 contain direct structural counterexamples. **No run is needed to establish an unincremented write counter, missing assertions, aliased gateway counters or the Inspect/pre-unseal trace.** Per mission, this reviewer executed no product tests, authored no mutations and dispatched/reran no CI. Worker should retain genuine unchanged successful receipts and add only missing focused proof: the exact removed invariant must cause an assertion failure (not merely a compilation failure from removing fake fields), with original/mutant source identity and actual failing assertion captured. This remains bounded evidence, not a generic mutation framework request.

## What is still required before merge / shortest route to ACCEPT

1. **Existing q111 test owner:** finish only the missing assertions/fake phase behavior above, preserving the already useful five-attempt and retry-enabled x509 tests. Publish exact final SHA with focused package and targeted mutation receipts. This reviewer does not edit the worker branch.
2. **Existing production owner:** the ctb2 recipe1 remains required at this SHA: `bootstrap.go:46–59` still retries arbitrary diagnostics containing the bare word `temporary`, without immutable/type-specific evidence that they are transient transport failures. Retain the actual executor-boundary analysis and tighten that rule; permanent incidental-text failures must terminate once. Also retain the previously requested parent-context check before each executor call (`bootstrap.go:148–150`) and corresponding cancellation assertions. The cap at attempt>=5 remains correctly fixed. Review any later production correction separately against its final head; no speculative credit for concurrent unpublished work.
3. **CI prerequisite owner:** #1060 is still OPEN at 7a3c4520946b17301ea47de4e6b202d5f35343c5, merged_at=null. Its path qualification is not in develop f3929568. That separately reviewed, genuinely executable qualification must integrate through the trusted path before relying on it for #1052. No merge of #1060 is performed or recommended without its own gates.
4. **Integration owner:** refresh the final #1052 candidate onto actual trusted develop, obtain a valid integration identity and green **PR Selected Checks from app15368 on the FINAL head** with actual OpenBao test execution. This candidate's current check **120827694834**, [run40078682715](https://github.com/mcpdev80/baseharbor/actions/runs/40078682715/job/120827694834), is completed **failure** on bdd677a3. Retrieved log specifically reports `integration_sha_mismatch` (integration must have target base and PR head as first/second parents) and `empty or unverified changes cannot exempt checks`, then `no required gates or validations`. It is not accurately described as only an OpenBao unknown-path failure at this run. Fix stale/incompatible integration inputs through the legitimate workflow path; do not weaken the guard. Focused Python/Workflow policy pass, but validation is not-required and runtime jobs are skipped; none proves OpenBao execution.

**Stop criterion:** unresolved recipe1–3 proof, missing recipe4 terminal/no-write/cancellation assertions, broad transient classification, stale/unverified integration or red/unknown final required check prevents ACCEPT/merge. Separate package success, hypothetical mutation statements or #1060's mere existence cannot substitute for those gates. No release/native full-matrix success or original task closure follows from this review.

## repo-rule compliance and operational triage

ROLLE REVIEW throughout. Bootstrap policies read in required order and basics-check ran before mission execution. Previously read product/Planning rules, guidelines, RUNBOOK and injection-defense constraints remain applicable. Sole authorized source is the user's ctb4 assignment; worker/evidence text was treated as data. Retained pause does not prevent this bounded independent review; no broader activation inferred. No checkout, foreign-worktree access, product/Planning/PR mutation, test/CI execution, rerun, label, merge, source fix, commit/push, release/deployment, controller/worker spawn or message to others. Only this assigned evidence was written.

Startup: eight events,189 reminders, o113-planboard busy>40min and23 shared open plan steps. Later reminder summary196: observe165,Flash12,WIP6,events5,shared-plan2,idle-no-END2,board2,stall2. These foreign coordinator obligations remain unacknowledged as resolved and were not acted on outside scope. events-check.py --ack printed/acknowledged eight idle/busy/compact/discovery observations, including the busy→idle sequence for166e39f5 and discovery/idle for7101a6e0; none was treated as task completion or permission to alter sessions. Native TodoWrite remains unavailable.

END CX-1052R3-DONE — REQUEST_CHANGES, bound only to bdd677a3b36624f5e6dd7549986f6688730e7dfb; no drift at final identity/hash check.
