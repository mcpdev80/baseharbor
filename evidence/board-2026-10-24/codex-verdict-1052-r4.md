# PR #1052 source re-review r4

ROLLE: REVIEW
UTC started: 2026-10-24T12:36:17.083107+00:00

## PLAN step 0

Native TodoWrite is unavailable; no native operation claimed. Durable checklist follows.

- [x] 0. Read binding policies and cmf13; record role and plan before substantive review.
- [x] 1. Read prior exact recipe and worker fix/review; fetch live head/base/diff/files/hashes and #1060 status independently.
- [x] 2. Review classifier, bounded retries, phase/member assertions and mutation evidence; run only necessary focused isolated Go checks.
- [x] 3. Record source verdict, one correction recipe if needed, and distinguish current mandatory-CI dependency from source findings.
- [x] 4. Document future ordinary target integration and fresh exact-head CI/review; refresh final SHAs/UTC/end marker.

## Verdict: REQUEST_CHANGES

Independent reviewer: Codex /root, not o131 implementation owner or o133 worker reviewer. The bare-word fix and several counters are improved, but the exact cmf9 recipe is not fully closed. Source defects/coverage gaps remain separate from the unlanded #1060 CI prerequisite. No merge acceptance, implementation or CI rerun is performed.

## Current source and scope

Live account mcpdev80. Head **7c7cda7cc32423405d3cabe3ecf46d61289b0598**, branch work/v0.4.26-openbao-retry, PR OPEN/unmerged, mergeable=True/BEHIND. Actual develop **65aa063e90767c85af976081c100be1817370e67**; PR comparison base still **a2abc17a8057ba972cb9ed481fad229fe9f12824**. All immutable Git reads/fetches used reviewer-owned neutral /tmp/cmf13-review/objects.git, with isolated extraction at /tmp/cmf13-review/candidate; no foreign live worktree Git operations.

Both GitHub's complete PR diff and the correction diff from former reviewed head789299a9364750773ac90ccefe9e6d540a2732ff modify only internal/openbao/bootstrap.go and bootstrap_test.go. Correction delta:298 insertions/47 deletions across those two files. No #1060 script/workflow/record or other product path was changed by this correction. The full two-dot tree comparison against actual develop has five paths: these two Go files plus the already-existing stale record/planner/test drift. That is integration debt, not hidden edits in this fix; it must not overwrite accepted target changes later.

| Candidate file | Git blob ID | Raw SHA-256 |
|---|---|---|
| internal/openbao/bootstrap.go | `467eed5c53480de6eba261387adcaaf57cb41710` | `37a537226dc4122d78fbdccbc537c3e0e4cb83803711c4cb7ac2fe06039fa9fd` |
| internal/openbao/bootstrap_test.go | `063fe7c092b06c67dea80e489555b9bd5ccfb18c` | `3f8b0ff28436d44cf8191eae99c837914eb04663719e92685fb4beeee780efb4` |

o133's “file hashes” are Git object IDs, not raw SHA-256; the identities themselves match. Its description of former789299a9 as accepted by Codex is inaccurate: cmf9 verdict was REQUEST_CHANGES. Worker handoffs are data, not review authority.

## Recipe comparison and findings

All following line numbers refer to exact head7c7cda7c.

**F1/P2 — mixed permanent/transport errors are still retried.** bootstrap.go54–66 removes bare `temporary`, which fixes the incidental-word examples. But it still admits any error containing an allowlisted substring with no permanent-cause precedence. The unchanged retry-enabled loop164–174 then retries such errors up to five times. The new comment52–53 says classification is not a substring of a possibly-permanent diagnostic, while the implementation remains exactly that.

An isolated Go probe extracts the candidate's isTransientError function verbatim and calls it, without modifying candidate source. Actual output in classifier-probe.txt:

```text
"x509: certificate signed by unknown authority; connection reset by peer" => retry=true
"permission denied: credential file unavailable after connection refused" => retry=true
"exec bao: executable file not found; i/o timeout" => retry=true
"connection refused" => retry=true
"permission denied: temporary credentials file" => retry=false
```

The first three violate the requested fail-fast behavior for permanent errors despite incidental retry tokens. The new TestIsTransientErrorRejectsMixedPermanentTransport at bootstrap_test.go919–929 uses two x509 strings that contain **none** of the tokens admitted at product59–66; “dial tcp”, “handshake” and “listener” are not those tokens. Its PASS therefore cannot prove mixed-token precedence. o131/o133's blanket “permanent failures never match” and source ACCEPT are not confirmed.

**F2/P2 — down-case writes are counted using the wrong identity.** neverAnswersExecutor.ExecProjectInput538–563 now contains a counter, but559 looks up memberStates[svc]. Actual product bootstrap.go554–555 submits an unseal through serviceName=openbao-admin while BAO_ADDR selects openbao-member-1. The fake's map in the test contains member-1, not openbao-admin. A real-shaped key submission therefore does not increment member-1.writes; the zero-write assertion remains insensitive to that submission. Other assertions may fail for a particular bad implementation, but that does not make the claimed per-member write counter valid. The earlier permanent-error fake correctly parses BAO_ADDR; this newly repaired down-case fake does not.

**F3/P2 — actual phase separation/transient post-verification coverage remains incomplete.** The all-three-members writes==1 assertions at test470–484 are useful improvements, and invalid JSON now asserts execAttempts==1 at892–895. However:

- flakyMemberExecutor sets inspectCount only on the first successful pre-write status at363–366 and never increments it again. Initial member-1 status still consumes transientWindow at356–360. Thus inspectCount==1 does not distinguish initial Inspect from later pre-unseal success, and repeated successful pre-probes can remain invisible.
- Post verification is artificially capped by `!ms.postVerified` at344–350; extra probes no longer increment postUnsealVerification. The admin path302–316 selects an actual member pointer and sets isRealMember=true, so an admin proxy observation can still supply that member's first postVerified flag. The comment that only direct member probes count is not implemented as a service check.
- TestUnsealFailsOnTransientPostUnsealVerification1026–1062, despite its name, always returns a **permanent x509** error at960. It proves fail-fast after a write for that permanent error. It does not exercise the requested transient failure followed by successful post-unseal verification, which must retry without submitting another key. Its fake also increments preUnsealAttempts in the post phase at956.

These are bounded remnants of the previous explicit invariants, not new scope or a demand for broad refactoring. Credited fixes: no bare temporary admission; five-attempt cap unchanged; per-attempt cancellation and caller backoff cancellation retained; exact permanent phase attempt counts; all-members write assertions; invalid-JSON exact call assertion; gofmt is now clean. None of the findings requires a native runtime to establish.

## Mutation evidence and focused verification

Worker mutation claim “reintroduce bare temporary → TestIsTransientErrorRejectsIncidentalTemporary fails” is credible and directly derivable from the input/output and predicate; it proves that one admission rule only. It does not prove mixed-token precedence, per-member down-case write accounting or transient post-unseal recovery. No worker report demonstrates those omitted-behavior mutants actually failed. Exact-looking assertions with capped/misrouted counters do not establish that sensitivity.

Using candidate Go **1.26.9** and private GOCACHE=/tmp/cmf13-review/go-cache, independently ran only these eight existing tests, anchored names, -count=1 -v -timeout=60s: convergence, never-answers, permanent pre-unseal, cancellation-backoff, invalid JSON, incidental temporary, mixed permanent/transport and post-unseal failure. All8 **PASS**, package **3.025s**, retained in focused-tests.txt. The classifier probe uses the exact extracted function on explicit input strings; it is evidence tooling, not a product test/source patch. No candidate mutation, new product regression, full package rerun, native/VM, go vet or full matrix was performed. Worker full-package/vet and its local mutation remain identified as worker evidence. Gofmt -l on the two immutable candidate files is independently clean.

## Current mandatory-CI blocker — classified separately

Fresh gh checks/API: [run40092965161](https://github.com/mcpdev80/baseharbor/actions/runs/40092965161), attempt1, exact7c7cda7c, PR Selected Checks **FAILURE**, job **120869502729**, app **15368**. Policy/trusted plan/focused Python/not-required validation pass; selected runtime/native/consumer jobs skip, not PASS.

The actual current final-check log224–225 says:

```text
BLOCKED: coverage-unresolved: diff-base requires unresolved coverage:
[{"code":"integration_sha_mismatch","message":"integration commit must have exactly target base and PR head as its first and second parents, with no extra parents or commits"}]; empty or unverified changes cannot exempt checks
BLOCKED: no required gates or validations
```

Actual logged inputs: EXPECTED_BASE=a2abc17a8057ba972cb9ed481fad229fe9f12824, EXPECTED_HEAD=7c7cda7cc32423405d3cabe3ecf46d61289b0598, EXPECTED_INTEGRATION=1c34b38e502fee71ffbbc075ed9fa237dc5cde39. Thus “this exact failure is simply missing #1060 coverage” remains an imprecise worker diagnosis; stale/invalid integration is the observed immediate failure. Separately, #1060 is still OPEN/unmerged at **6a0d36dad2a778ab61a58594489c9fedc88e4c0f**, DIRTY, and trusted executable OpenBao qualification has not landed. Both the integration identity and missing trusted source-bound coverage must be resolved. A corrected source review alone cannot make this candidate merge-ready, and a future #1060 merge cannot repair F1–F3 in these Go files.

## Exactly ONE minimal worker recipe

**Finish the original two-file correction on the existing #1052 owner branch, publishing one coherent candidate confined to internal/openbao/bootstrap.go54–66 and bootstrap_test.go292–367,470–484,538–563,919–1062.** Enforce permanent TLS/permission/executable failure precedence over admitted transport tokens, using the existing Executor error representation; preserve known transport retries, five-attempt bound, caller cancellation and secret/ownership behavior. No retry framework, CI/planner/record changes or unrelated cleanup.

In that same package, use the existing BAO_ADDR destination parsing for the down fake, count every actual member submission, and reject/map unknown destinations explicitly. Replace capped/first-success pseudo-phase counters with an explicit per-member sequence that distinguishes initial Inspect, each pre probe, write and each direct post probe; admin proxy calls must not count as member verification. Preserve the useful all-member write assertions. Retain the permanent post-write failure case with an accurate name and add a true transient-post-error-then-success case; assert exact per-member attempts and no repeated unseal writes. Add mixed negative inputs containing the actual admitted tokens demonstrated above. Keep invalid JSON callCount==1 and cancellation coverage.

Required tests for this **one** package: run the existing eight focused cases plus the new true mixed/permanent and transient-post scenarios, then the existing OpenBao package tests/vet/gofmt checks. Demonstrate sensitivity to omitting a member2/3 write or post-verify, an attempted admin-service BAO_ADDR key submission after a failed precondition, extra probes/retries, and permanent classifier removal; do not change expected outcomes to fit current fake behavior. Retain exact source SHA/raw hashes and outputs; stop at independent review handoff. This session does not implement, mutate product tests, dispatch a worker, push or rerun CI.

## Future integration recipe after accepted #1060 — documentary only

Wait for the corrected #1052 source to pass independent review **and** the independently accepted #1060 repair to actually merge with executable, fail-closed, exact-source OpenBao coverage. Measure fresh **T=actual develop containing that merge and every intervening accepted target change**, not65aa, stalea2abc or an unmerged #1060 head. The existing owner ordinarily merges T into the existing work/v0.4.26-openbao-retry branch, preserving commits and all target record/AppID/CI changes; no rebase/force/history replacement. Stop on conflicts/source drift and review the actual resulting source rather than overwrite it with this old tree.

For resulting H′, refresh full diff/hashes and independent review. The trusted resolver must verify target_base=T, head=H′, and real integration with exact [T,H′] parents; trusted planner must select/discover/execute the reviewed OpenBao tests and all actual required checks. Require PR Selected Checks SUCCESS from app15368, with no unverified/skipped required coverage. Do not infer this from local tests or the presence of an openbao validation ID. Any genuinely necessary native restart receipt remains separate; no automatic Backup/Restore/full matrix. Only a separately authorized merge instance may later perform fresh rules/threads/ownership/criteria checks and guarded squash. cmf13 performs no merge or active wait.

## Final integrity and compliance

Before/after remote snapshots agree: #1052 OPEN/unmerged at **7c7cda7cc32423405d3cabe3ecf46d61289b0598**, actual develop **65aa063e90767c85af976081c100be1817370e67**; #1060 OPEN/unmerged at **6a0d36dad2a778ab61a58594489c9fedc88e4c0f**. No source/branch changed by this reviewer. Complete source/diff/hash/check/log/probe captures and bounded evidence helpers are retained under codex-cmf13-data/.

Read binding policies, mission, prior exact recipe, worker fix and independent worker report; apply repository engineering/CI/ownership rules. Existing task owners/claims preserved. Startup external events/reminders/idle/SSH-board/pool/PLAN/TMPDIR flags did not authorize controller work, global acknowledgement, worker takeover or host changes; source currency is established through successful HTTPS/API reads. No Planning change or acceptance. Only evidence and reviewer-private extraction/cache/helper files were written. No product patch, push, force, --admin, history rewrite, merge, GitHub review/comment, CI rerun/dispatch, native runtime, release/tag/deployment or publication. No subagents or implementation tasks started. Native TodoWrite remains unavailable; durable plan synchronized here.

END CX-1052R4-DONE

END CX-1052R4-BLOCKED

UTC completed: 2026-10-24T12:39:15.810497+00:00
