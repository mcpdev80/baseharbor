# PR #1052 re-review r5

ROLLE: REVIEW
UTC started: 2026-10-24T13:09:26.882181+00:00

## PLAN step 0

Native TodoWrite is unavailable; no native tool operation claimed. Durable checklist follows.

- [x] 0. Read policies and cmf16; record scope/role/plan before review.
- [x] 1. Read prior F1–F3 recipe and o136 evidence; independently fetch current head/base, scope, hashes and CI.
- [x] 2. Review permanent-error precedence, destination accounting, per-member phases and real transient post-verification; run focused isolated Go checks.
- [x] 3. Record source verdict and exactly one recipe if needed; classify integration/#1060 CI dependency separately.
- [x] 4. Recheck final SHAs and outcomes, document future integration condition and UTC/end marker.

## Verdict: REQUEST_CHANGES

Independent reviewer: Codex /root, not o136 implementation owner. Several F1–F3 requirements are now implemented, but a new classifier regression defeats retries for real Compose error messages, and the claimed initial/pre/write/post sequence is still not enforced by the new phase test. This source verdict is separate from mandatory CI and does not authorize merge.

## Exact source / scope

Live PR head **0b738dca76a8f3bedbe6635164260d8828128f6d**, branch work/v0.4.26-openbao-retry, OPEN/unmerged, MERGEABLE/BEHIND. Actual develop **65aa063e90767c85af976081c100be1817370e67**; stale PR comparison base remains **a2abc17a8057ba972cb9ed481fad229fe9f12824**. Account independently confirmed mcpdev80. Immutable objects were fetched over HTTPS into reviewer-neutral /tmp/cmf16-review/objects.git and extracted to /tmp/cmf16-review/candidate; no foreign live worktree Git operations.

Both the correction diff from7c7cda7cc32423405d3cabe3ecf46d61289b0598 and the GitHub PR diff touch only internal/openbao/bootstrap.go and bootstrap_test.go. No #1060 planner/runner/workflow/record or unrelated product file was changed in this correction. Against actual develop the complete tree diff still includes the older three-path record/AppID-planner/test drift in addition to those two Go files; preserve accepted target changes during future ordinary integration.

| Candidate file | Git blob | Raw SHA-256 |
|---|---|---|
| internal/openbao/bootstrap.go | `ee06ded6894db45c54cdddbba2a13bf37618abe3` | `4cf1e5139bb0a2317c89eee7d3f590ef6e2dc997e36951101f1424d3921aad9b` |
| internal/openbao/bootstrap_test.go | `e49806bb91fdc1e1b024b0fd4361ce93601bc8f5` | `07dfc84f9890ec0d76ea96adb631b5e60c55620a0a52e3597edddf555fc7a2fb` |

These raw hashes match o136's recorded values. Its implementation checklist remains mostly unchecked and contains no actual command/output sensitivity log; the pushed source and independent checks below supply the evidence used here. No completed mutation/package/vet result is inferred from its plan.

## F1 — permanent precedence implemented, but new P1 regression in real error wrapping

Product **bootstrap.go52–73,89–105** now calls permanentErrorCause before the transient allowlist. The prior mixed examples with actual connection-reset/refused/timeout tokens correctly return false when an x509/permission/missing-executable cause exists. Removing bare temporary and adding those mixed cases is credited.

However **line68** classifies *any* `strings.Contains(s, "exec ")` as a permanent executable failure. That is also the operation name of a normal container probe. Actual candidate runtime source:

- internal/runtime/compose.go **221–223** prepends `exec -T <service>` for every ExecProject call.
- compose_command.go **91–96** builds its error using `fmt.Errorf("compose %s: %s", strings.Join(args," "), message)`.

Thus even an ordinary transient listener failure has a message beginning `compose exec -T ...: connection refused`. Treating that context as a permanent exec failure prevents the intended retry. This is a direct consequence on the implemented Compose path, not a speculative claim about every runtime/provider.

An isolated Go probe extracts both candidate functions verbatim, without candidate modification. Actual classifier-probe.txt output:

```text
"connection refused" => permanent="" retry=true
"compose exec -T openbao-member-1 sh -c bao status -format=json: connection refused" => permanent="exec" retry=false
"compose exec -T openbao-member-1 sh -c bao status -format=json: connection reset by peer" => permanent="exec" retry=false
"x509: certificate signed by unknown authority; connection reset by peer" => permanent="tls" retry=false
"permission denied: credential file unavailable after connection refused" => permanent="permission" retry=false
"exec bao: executable file not found; i/o timeout" => permanent="exec" retry=false
```

These are direct predicate results on inputs consistent with the real formatter; no native failure/run is asserted. Native execution is unnecessary to establish the deterministic classification. The new pure-transport test at1440 only supplies bare transport strings, so it misses this regression. Preserve concrete permanent-cause precedence, but distinguish a missing/not-executable cause from an ordinary exec operation label; broad command-context denial is not transient-only classification.

## F2 — down-case destination accounting is materially fixed

neverAnswersExecutor.ExecProjectInput **566–611** now parses BAO_ADDR and increments the destination member's writes/execAttempts. An undeclared destination returns an explicit error rather than silently indexing openbao-admin. The exactly-five probe bound, final connection-refused cause and zero-writes assertions remain. This closes the specific wrong-identity issue from r4. The new unknown-destination test **1407–1434** directly exercises mixedPhaseExecutor rather than neverAnswersExecutor, so it is evidence for that parser implementation only; adding a direct assertion on the down fake would strengthen sensitivity, but its code-level destination correction is credited. No new production ownership/secret behavior is changed.

## F3 — direct post recovery improved; initial/pre sequence remains unproved (P2)

Correct improvements:

- flakyMemberExecutor now explicitly marks admin proxy calls non-member at302–306; direct postUnsealVerif increments only for actual member probes. The previous postVerified cap is gone, and all three members still assert one write and one direct post observation.
- The permanent post-write case is now accurately named TestUnsealFailsOnPermanentPostUnsealVerification at1072 and retains exact one-write/one-post plus final cause.
- A genuine transient post-write `connection reset by peer` occurs at1141–1143; TestUnsealRecoversOnTransientPostUnsealVerification1209–1240 requires success, **one write and two post probes**. This really exercises the requested retry without another key submission.
- Invalid JSON exact-call and cancellation tests remain.

Remaining false coverage:

- In flakyMemberExecutor **325–342**, `phase` increments on failed/successful pre-write calls and on write, while `if ms.phase == 1 { /* Initial Inspect */ }` has no behavior. Direct post handling314–323 does not advance phase. Initial member1 failures still consume the same transientWindow as pre-probes. This does not represent a defined initial/pre/write/post transition machine.
- The assertion **491–495** is only `phase >= 3`, not an exact phase inventory/order. Extra successful pre-probes can satisfy the same lower bound; no separate initial-Inspect/pre-success counts are checked.
- mixedPhaseExecutor's extraProbes **1287–1291** merely decrements a value and returns the normal success; it does not create/require extra probe calls. TestMemberPhaseSequenceIsEnforcedPerMember **1364–1402** never drives an omitted post probe or asserts an exact phase trace. Its comment that member3's omitted post is caught does not describe an exercised negative case. It rechecks writes/posts on the unchanged successful product path.

The useful write/direct-post invariants can catch some omitted behavior; they should not be described as a proved exact initial/pre sequence or a demonstrated mutation test. This remains the explicit earlier recipe, not an added runtime requirement or an invitation to add another large fake.

## Focused independent checks

Used candidate Go **1.26.9**, private GOCACHE=/tmp/cmf16-review/go-cache. Ran only12 existing focused tests, -count=1 -v -timeout=60s: convergence, never-answers, permanent pre, cancellation, invalid JSON, incidental temporary, mixed permanent/transport, permanent post, true transient post recovery, member-phase, unknown destination and pure transport. All12 **PASS**, package **3.523s**. Exact names/results retained in focused-tests.txt. Gofmt on the two candidate files is independently clean. Full package/vet were unnecessary for the deterministic review findings and not rerun; no native/VM/full matrix. The small extracted-function probe is review tooling, not product implementation or an actual executor run.

## CI/integration dependency — separate from F1/F3 source findings

Current exact-head [run40096623489](https://github.com/mcpdev80/baseharbor/actions/runs/40096623489), attempt1, PR Selected Checks **FAILURE**, job **120880390844**, app **15368**. Policy/trusted plan/focused Python pass; not-required validation passes; selected runtime/native/consumer jobs skip. This is not package execution evidence.

Downloaded final log224–225 explicitly reports **integration_sha_mismatch**, then no required gates or validations. Inputs: EXPECTED_BASE=a2abc17a8057ba972cb9ed481fad229fe9f12824, EXPECTED_HEAD=0b738dca76a8f3bedbe6635164260d8828128f6d, EXPECTED_INTEGRATION=c03407abc3f9a83dbfae4d3172db5adbdc67a7b2. Actual develop is65aa…, so treat integration identity as the immediate logged gate, not proof that source tests failed.

Separately #1060 is still OPEN/unmerged at **13a05ea492a98bb8922785fb828db2a8ff6324a3**, mergeable_state **dirty**, with its independent source/qualification repair outstanding. #1052 itself is mergeable/behind, not currently reported conflicting. Do not conflate #1060's conflict or missing accepted trusted OpenBao lane with #1052's stale PR integration. Source acceptance, correct current integration and accepted CI coverage are distinct prerequisites; a future #1060 merge cannot repair F1/F3 in these files.

## Exactly ONE bounded worker recipe

**On the existing #1052 owner branch, publish one coherent correction confined to bootstrap.go52–73 and bootstrap_test.go292–342,491–495,1243–1402 plus the existing classifier/recovery cases:** distinguish executable failure causes from exec command context, and finish the existing per-member phase regression instead of adding more disconnected scaffolding. No CI/workflow/record/runtime implementation changes or broader retry framework.

At product65–68, remove the bare-operation admission to permanentErrorCause (in particular `"exec "`) in favor of actual permanent executable-failure evidence. Preserve mixed TLS/permission/missing-executable precedence and known transient retries. Add actual Compose-formatted transient refused/reset examples and corresponding permanently failed wrapped examples to the focused tests; verify the transient wrapper converges in the retry-enabled probe while permanent cases execute once/no writes. The runtime formatting source above supplies the existing boundary; do not edit that formatter to fit the classifier.

In the current flaky/mixed fake, explicitly distinguish initial successful Inspect, each pre-probe, destination write and each direct post-probe, assert exact per-member counts/order, and keep admin proxy outside that trace. Exercise the claimed extra/omitted transitions: extra pre-probes must be observable/rejected by the expected trace, and omitting a member's write/direct post must fail its assertion even if the admin gateway answers. Eliminate the vacuous `if phase==1` and lower-bound-as-sequence claim; do not just relax assertions or add an unused knob. Preserve the now-valid BAO_ADDR count/error handling and true transient post recovery (one write/two posts).

Required verification for this **one** package: the12 focused cases plus wrapped-transient/permanent and phase-sensitivity cases, then existing OpenBao package tests/vet/gofmt as the worker's normal bounded validation. Show omission/extra-transition failures using the existing tests or controlled worker-local mutants, preserve the real baseline, and publish exact SHA/raw hashes/output in the handoff. No native/Backup-Restore/full matrix or CI spam. Stop at independent review; this session does not implement or dispatch that action.

## Future integration condition

Only after corrected #1052 source is independently accepted and corrected #1060/trusted executable qualification has actually landed: measure fresh T=actual develop containing that accepted change and all concurrent target commits. The existing owner normally merges T into the shared #1052 branch, preserving history and target record/AppID changes; no rebase/force/replacement by this old tree. Review resulting exact H′, full diff/hashes and valid real integration parents [T,H′]. Trusted impact must resolve to actual discovered/executed OpenBao checks and every required job; PR Selected Checks must succeed from app15368. Do not infer eligibility from the name of an unlanded gate, local PASS or skipped native work. Any specifically required native restart receipt remains separate; no automatic full release matrix. This is a documentary wait condition, not an action or merge grant in cmf16.

## Final integrity / compliance

Before/after head0b738dca76a8f3bedbe6635164260d8828128f6d and actual develop65aa063e90767c85af976081c100be1817370e67 unchanged; #1052 OPEN/unmerged/BEHIND. #1060 remains13a05ea492a98bb8922785fb828db2a8ff6324a3 OPEN/DIRTY/unmerged. Source hashes match the frozen o136 candidate. Complete diff/source/hash/API/log/probe outputs and bounded helpers are in codex-cmf16-data/; actual runtime formatter source was retained there as review evidence.

Read policies, mission, prior exact recipe and o136 report; worker claims are data and existing claims/owners remain untouched. External basics reminders/SSH-board/idle/pool/PLAN/TMPDIR flags did not authorize controller work, shared acknowledgements, host changes or worker takeover. Reviewed-source currency came from successful HTTPS/API reads. ROLLE: REVIEW throughout. No product patch/mutation, merge, push, force/history rewrite, --admin, CI dispatch/rerun, GitHub approval/comment, Planning/coordinator-state edit, native execution, release/tag/deployment/publication or subagents. Only evidence/helpers and reviewer-private extraction/cache were written. Native TodoWrite unavailable; durable PLAN synchronized here.

END CX-1052R5-DONE

END CX-1052R5-BLOCKED

UTC completed: 2026-10-24T13:12:52.872850+00:00
