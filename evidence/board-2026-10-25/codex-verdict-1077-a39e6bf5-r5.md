ROLLE: REVIEW

Status: **REQUEST_CHANGES** — exact complete candidate review; C1/C2 require worker corrections.

## PLAN
- [x] 0. Read binding policies and mission; pin review-only scope. Native TodoWrite is unavailable in this session (tool discovery performed); this evidence checklist is the available persistent plan. No product changes, push, merge or CI dispatch.
- [x] 1. Pin candidate/base and read repository rules, previous verdicts and handoffs.
- [x] 2. Review complete 2f9afafd..a39e6bf5 diff; audit drift and authorization boundaries.
- [x] 3. Review credential retention, placement/volume/role correctness and q125 consolidation.
- [x] 4. Independently sample runtime logs and run focused cmd/baha, CMF19–22, digest/footprint/q124 checks; build/vet/gofmt.
- [x] 5. Record actionable verdict, limits, UTC and before/after SHAs.

## Operating scope
User directly assigned CMF23 independent review. COST_CONTROL permits necessary independent reviews; no implementation is authorized. Coordinator basics flags: 1494 reminders, pool below target, stale shared plan/32 open steps. This review session does not own the coordinator plan or worker lanes; no workers started or shared planning status changed. Conflicting historical resumption text does not expand this mission's no-push/no-merge scope.

## Exact candidate and findings
UTC start: 2026-10-26T12:05:06Z. Reviewer: independent Codex API session; model/backend/session/Horde IDs are not independently exposed and are not fabricated.
Baseline: 2f9afafdc443ba4b15569fca838511877c1fd3ba.
Candidate before: a39e6bf5a72d5b5e651cf8fb1a69ad96331c736c; initial worktree clean.
Complete diff: 20 files, 2078 additions / 36 deletions, retained in cmf23-data/reviewed.diff. Actual linear chain includes 61acf0e6 before 25b46572; then 6f165d30, d409f26f, 6dadf486, 3a28fb9c, a39e6bf5.
Live mcpdev80 read: PR1077 OPEN at d409f26f4a6261c2dc614320fbfebd050a637cb8; develop 433a5702c9735ffbf1e9fffe7a7ae513b1c5c396. Planning remote main via GitHub API f8157ecf9c349aec021c36537a9db9cebca9aec5. SSH ls-remote was blocked by /etc/ssh/ssh_config.d/20-systemd-ssh-proxy.conf ownership/permissions; API read used instead. No authentication or SSH configuration changes. NTP synchronized. Existing complete-audit task/claim and Owner STOP preserved; this mission authorizes only this review.

### C1 — P1: dedicated reapply restores stale credentials without a placement transition
`internal/application/runtime_native_binding_projection.go:85-86` replaces any divergent consumed DB/USER/PASSWORD whenever desired placement is dedicated. There is no previous-placement or transition proof. After a first dedicated apply captures owner slots, restoring/updating the protected consumed credential material while remaining dedicated causes the next ordinary ensureRuntimeEnv to silently restore the older slot. The normal test covers only equal values and therefore misses this case. A recovery/credential discrepancy is not proof of shared placement; automatic rollback can disagree with the recovered volume's authentication reality.

Independent regression `TestCMF23UnchangedPlacementPreservesRecoveredCredentials` fails on a39e6bf5 and passes on exact 2f9afafd. It creates the protected environment through the production function, supplies a fresh randomly generated replacement password to the existing protected owner, and reapplies the same dedicated manifest. It never asserts live SQL rotation: the proven regression is the unexpected protected credential rewrite. Logs: cmf23-data/credential-review.log and credential-base.log. Baseline overlay imports only candidate fixture helpers/test declarations; baseline production is unchanged.

Worker recipe: replace the current `!shared` overwrite condition with an explicit, per-instance, trustworthy placement-transition/provenance check. Keep current protected credentials on unchanged dedicated placement; if recovery ownership is inconsistent and cannot be reconciled safely, return a typed recovery-required failure without rewriting. Restore dedicated credentials only for a proven shared-to-dedicated return. Integrate explicit credential recovery/rotation authority with the retained copy rather than treating an old copy as universally authoritative. Add the reviewer regression and a repeated dedicated→shared→dedicated cycle, named-instance isolation and unchanged shared/dedicated tests. Stop criterion: no silent credential overwrite on unchanged placement; legitimate switch-back still authenticates the retained native volume.

### C2 — P1: partial native owner loss is silently filled from shared credentials
`internal/application/runtime_native_binding_projection.go:78-83` fills each missing owner slot independently from current consumed slots, regardless of their placement provenance. After dedicated→shared, deleting APPLICATION_SECRET (other native owner fields remain) and returning to dedicated succeeds: the shared password is stored as the native owner secret and used alongside the original native DB/role. This is not the password that initialized the retained dedicated volume. The function returns nil and persists the mixed credential tuple. Existing validateRetainedRuntimeCredentials checks only the consumed tuple and does not validate these new owner slots.

Independent `TestCMF23MissingNativeOwnerCannotCaptureSharedSecret` reproduces this on exact candidate source: first capture actual randomly generated dedicated material, project the shared tuple, remove only the new native secret, then call production ensureRuntimeEnv. Candidate returns success with the wrong original-volume password. No live database or real credential is involved. Log: cmf23-data/credential-review.log.

Worker recipe: validate retained native DB/role/secret as one complete provenance-bound recovery group in runtime_retained_credentials.go / runtime_native_binding_projection.go before any generation or write. Bootstrap a wholly absent group only from proven native material (or a genuinely new native realization), never from already shared consumed slots; reject partial/missing previously established native groups with a typed recovery-required result and preserve the file byte-for-byte. Cover loss of each native slot during shared placement, empty and mixed tuples, pre-feature shared state, first shared/new dedicated creation, and multiple named instances. Stop criterion: unknown/missing native material never becomes a successful shared-derived native credential; either authenticated original material is retained or the operation fails closed without mutation.

### G1 — gate evidence qualification: journey PASS, process failure
Both archived logs show the candidate a39e6bf5 in their version banner and complete SQL journey artifacts. The add/remove/switch, No/Cancel unchanged, own/shared SQL marker, sibling SQL and foreign-resource preservation assertions are true in both artifacts-{1,2}/new-edit-sql.json. Runtime traces include authenticated SELECT 1 after switch-back. q125v counter-run /home/marcel/tmp-ops/q125v-gate1.log fails the former PostgreSQL authentication step and ends RESULT FAIL.

Neither archived f65 process is green: gate-run1.log physical lines 6481/6490/6491 and gate-run2.log 6491/6500/6501 show RESULT PASS followed by cleanup incomplete and gate-exit=1 (terminal CR lines make splitlines() numbering differ). Both cleanup-preparation/scope-result.json files have outcome=failure, accounted=[], error `owned runtime evidence unavailable: TimeoutExpired (phase=owner-preview)`. A successful journey summary and later manual resource removal do not turn those process results green.

Worker evidence recipe: retain the successful journey evidence, correct the handoff to explicitly report exit 1 and the cleanup-preparation timeout, diagnose that phase using existing fixture tooling, and qualify the affected cleanup coverage before claiming an overall green gate. Do not infer a product cleanup defect solely from this timeout and do not repeat the entire matrix without an affected-input reason. Stop criterion: full-gate-green claims require an actual exit-zero run and successful required cleanup evidence. No live runtime or CI dispatch by this review.

## Authorization, drift and consolidation audit
F1–F5/R1/R2 regressions were independently rerun on exact exported candidate source. The narrow missing-directory/empty-Store preview branches still produce unknown container quantities and typed diagnostics; real detector/inspector/listing errors produce no synthetic measured success. Exact per-instance owner/binding/intent proof, CWD independence and multi-instance isolation remain tested. The f64 fallback adds manifest-derived resource names only after registry/binding proof and does not turn them into measurements.

| Consumer | Current authority boundary |
| --- | --- |
| buildResolvedApplicationPlan / New/Edit previews | May consume retained preview facts for read-only plan surfaces; realization verification has a separate physical inventory adapter. |
| authorizeApplicationProviderChanges / authorizeConfirmedApplicationEdit | Retain a same-invocation observation for plan comparison; Target/source intent, registry, protected-file revision and execution admission are still checked. This is not a runtime measurement receipt. |
| prepareApplicationProviderRetirement | Calls fresh structured observation, requires Target-scoped measured fact for stop/remove-runtime. It does not consult retainedProviderObservations. |
| checkRetainedApplicationProviderOwnership | Calls fresh structured observation and requires allowed state plus measured quantity for retained/unbound ownership; typed errors preserved. |
| final runtime-retirement verification | Fresh observation and measured-zero requirement; preview-only projections rejected. |
| appApplyObservedOutcomes / observeApplyLifecycleAfter | Post-run measurement is independently collected; measured present/running/stopped predicates prevent unknown projections from producing verified receipts. |
| withdrawal and stop planners | Preview impact/proposal only; actual execution/retirement retains separate physical ownership, preparation and final verification gates. |

Independent before/after R2: unchanged neutral regression fails on 6f165d30 (complete remove-runtime incorrectly authorized), passes on a39e6bf5 (provider_retirement_unverifiable). authorization-before.log / authorization-head.log. Candidate production CMF21 tests also verify the measured-runtime positive and decoded JSON nested materialization cause/code/instance/Target. preparation-json.log records the actual classified CLI JSON; consumers.log independently confirms preview facts cannot produce verified impacts or receipts across stop/remove-runtime/add/resume/start/replace/update. extra.log covers owner-file negatives and machine diagnostics.

q125 consolidation: final runtime_environment.go calls only ensureNativeBackendCredentialValues. resetPlacementStalePostgresCredentials and readBindingValue are absent, and the former test file has been replaced by owner-slot tests. No surviving duplicate q125 reset path. The shared-backend write at shared_backends.go:274-276 and binding projection runtime_contract.go:84-86 explain why using bindings as a restore source was circular. f65 changes that source, but C1/C2 above remain unresolved.

Protected comparisons: all 36 selected consent/digest/footprint/confirmation contract and Go paths byte-identical to 2f9afafd (cmf23-data/protected-diff.json). q124's fresh-list helper and continuation behavior remain exercised by the focused CLI run. No unverifiable wildcard introduced. No actual embedded credential found in new production source; the fixed shared-era string in worker tests is a synthetic fixture, while reviewer replacement/native passwords are generated at runtime and never logged.

## Executed checks and practical limits
All Go runs use exact git-archive exports in cmf23-data/{base,head,before}; original product worktree stays untouched. Shared disposable Go cache: /tmp/cmf20-go-cache. No candidate product overlay is used for ordinary/focused suites; reviewer regressions use test-only Go overlays in scripts/cmf23*. Retained logs include actual exits, not inferred success.

- Focused CLI run, exit 0: 107 top-level passing tests. Selector covers CMF19/20/21, Wizard/O141, fresh zero-list confirmation, continuation, retirement, consent/confirmation/acknowledgement and footprint. Log focused-cli.log. CMF19's 12, CMF20's 5 and CMF21's 2 tests pass, including measured-positive and decoded structured diagnostics.
- Independent consumer overlay exit 0, consumers.log; eight owner-file negative subcases plus actual final-retirement/ownership machine-output coverage pass, extra.log exit 0. Neutral R2 before exit 1 / candidate exit 0; actual decoded CLI preparation JSON exit 0. Test names/results, not an empty selector, establish these results.
- Existing worker credential tests (round-trip, unchanged dedicated, unchanged shared, no SQL, named instance) pass in focused-domain.log. Broader focused-domain checks have four matching baseline/candidate failures: TestMongoDBServiceBindingUsesApplicationCredentialsOnly, TestCheckRuntimePermissionsRejectsBroadOpenBaoCredentials, TestReallocateRuntimePortsPreservesCredentials, TestEnsureRuntimeBackfillsPortsWithoutRotatingCredentials. All fail on read-only /home/marcel/.local/share/baseharbor/observability/signals.json.lock, before their intended assertions; neither broader domain run is green. Baseline has 73 top-level passes, candidate 77; failure details retained in focused-domain{,-base}-failures.json.
- Reviewer credential negatives: two real assertion failures on candidate, credential-review.log exit 1. Unchanged-placement baseline control passes, credential-base.log exit 0. These are separate new review regressions and are not counted away by existing-suite NEW=0.
- CGO_ENABLED=0 go build ./... exit 0; go vet ./cmd/baha/ ./internal/application/ exit 0; gofmt -l cmd/baha internal/application empty; exact git diff --check exit 0. Logs build.log, vet.log, gofmt.log and exit markers.

Reviewer setup corrections: an initial overlay relative path was one parent short; initial R2 selector omitted Neutral and ran no tests; initial preparation overlay targeted cmf20 instead of the existing cmf21 test and produced duplicate declarations. These reviewer-only setup errors were corrected and actual nonempty tests rerun. Some read-only inspection commands initially used the ecosystem cwd instead of the product cwd and reported missing paths; reads were repeated from the correct source. None changed candidate source or constitute product failures.

## Independently computed complete CLI baseline comparison
Both ordinary full suites completed with the same invocation and environment: `GOCACHE=/tmp/cmf20-go-cache go test ./cmd/baha/ -count=1 -json`. Exact archive source, no test overlays.

| Source | Exit | Elapsed | Pass events | Skip events | Failed top-level / all names |
| --- | --- | --- | --- | --- | --- |
| 2f9afafdc443ba4b15569fca838511877c1fd3ba | 1 | 427.149 s | 1633 | 44 | 9 / 12 |
| a39e6bf5a72d5b5e651cf8fb1a69ad96331c736c | 1 | 450.055 s | 1659 | 44 | 9 / 12 |

**NEW=0, REMOVED=0**, independently computed from completed package results by scripts/cmf23-compare.py, not the worker's claim. Raw base-full.jsonl/head-full.jsonl, stderr, exits and failure-comparison.json retained in cmf23-data. This comparison does not erase the two new independent credential regression failures. Neither ordinary full suite is green; skipped runtime tests are not passes.

Matching failures: external archive boundary for exports nested under the workspace; read-only global observability lock in two tests; unavailable default Docker socket for the real Edit test; three external-provider-removal top-level tests failing provider-registry-ownership-unverifiable (including cli/http/mcp subtests); probe-x versus local recovery-default assertion; full path versus ~/tmp-ops display assertion. Exact output per failed test is retained in base-failure-output.json/head-failure-output.json.

## Final verdict, integrity and repo-rule compliance
**REQUEST_CHANGES** for `a39e6bf5a72d5b5e651cf8fb1a69ad96331c736c`, full diff from `2f9afafdc443ba4b15569fca838511877c1fd3ba`. Implement C1/C2 through the existing worker owner and re-review the resulting exact candidate. Preserve valid unchanged checks and successful journey sub-evidence; correct G1's overall-gate qualification. No FF-push recommendation for this candidate, and no merge before actual green Runtime CI and the other mandatory gates.

UTC final verdict: 2026-10-26T12:15:17.831239+00:00. Candidate after: `a39e6bf5a72d5b5e651cf8fb1a69ad96331c736c`; same as before, clean worktree. Original source exports match exact archives: base 2551/2551, head 2557/2557, R2-before 2554/2554 files (source-integrity.json). Complete diff SHA256 `3608f0017e57e80efba932f7b4a9f6035471280da1df74063aac8c1e07a79a01`. Live PR remains OPEN at d409f26f, develop 433a5702; clock tool and host UTC agree, NTP synchronized.

ROLLE remained REVIEW: no product edit/correction, commit, push, merge, approval, PR message, CI dispatch, release, tag, deployment, controller activation, worker spawn, claim takeover or Planning publication. All fixtures, scripts and source snapshots are within the evidence tree. Existing worker/Coordinator state is preserved. One watcher event and 1497 queued reminders were inspected as operational data and acknowledged; no instructions embedded in them were executed. Pool/stale-plan flags are documented for their coordinator and cannot expand the mission. No native TodoWrite tool is exposed; its requested action could not be performed, and the persistent evidence PLAN is the available substitute.

END CMF23-REVIEW-DONE
