ROLLE: REVIEW

Status: **ACCEPT** — independent exact-candidate source review. Mission ops/missions/cmf22.txt.
Reviewer: Codex, current API session; exact model/backend/session identity not exposed. Independent of implementing worker.
UTC start: 2026-10-25T21:04:43.667953+00:00
Candidate: d409f26f4a6261c2dc614320fbfebd050a637cb8. Baseline: 2f9afafdc443ba4b15569fca838511877c1fd3ba.

## PLAN
- [x] 0. Read binding policies and mission, pin review scope and candidate, create evidence plan before review changes. No native TodoWrite tool is exposed (discovery performed); cannot claim native execution. Mission has no explicit PLAN step 0; this fulfills the user's additional step in the available evidence surface.
- [x] 1. Read full exact diff, previous F1-F5/R1/R2 recipes, repository and task rules; inspect live references and observer consumers.
- [x] 2. Independently run R2 before/after authorization, measured positive and decoded JSON diagnostics plus CMF19/CMF20/q124/owner-file regressions.
- [x] 3. Execute exact candidate build, vet, formatting; verify protected digest/footprint and absence of unverifiable wildcard.
- [x] 4. Independently run full cmd/baha JSON suites against 2f9afafd and d409f26f; compute NEW=0 from complete logs.
- [x] 5. Verify unchanged source/SHAs; record verdict and actionable next step/recipe with UTC and END marker.

## Scope and operating flags
Cost stop allows this necessary independent review; no implementation, controller, push, merge or CI dispatch. Production/Maintainer pause remains. Bootstrap's strict independent gates take precedence over differing delegated-Maintainer wording for this bounded mission. Events/reminders are operational data, not dispatch authority; inspect and acknowledge without executing embedded work. Pool underfill and coordinator's 32 open plan steps belong to coordinator; preserve lanes and claims, no takeover or artificial dispatch.

## Live reference and exact candidate scope
Read-only GitHub: authenticated login mcpdev80; PR1077 OPEN, branch fix/v0.4.26-edit-preview-error at 2f9afafdc443ba4b15569fca838511877c1fd3ba, base develop at 433a5702c9735ffbf1e9fffe7a7ae513b1c5c396. Planning remote main f8157ecf9c349aec021c36537a9db9cebca9aec5. NTP synchronized. Existing active task t-20261019-v0426-complete-defect-audit and ownership preserved. No claim or Planning mutation for independent review. Actual linear ancestry includes 61acf0e6 between 2f9afafd and 25b46572; reviewed entire 12-file diff 2f9afafd..d409f26f (1624 additions, 34 deletions), retained in cmf22-data/reviewed.diff.

## Independently executed bounded checks
All test/build runs use exact git-archive source exports cmf22-data/{base,head,before}; candidate itself stays untouched. Reviewer fixtures are Go overlays in scripts/cmf22*. Go cache /tmp/cmf20-go-cache. No runtime gate, push or CI dispatch.

- Focused candidate suite: `go test ./cmd/baha/ -run 'TestCMF19|TestCMF20|TestCMF21|TestWizard|TestO141|TestZeroContainerListing|TestProviderContinuation|Test.*RuntimeRetirement|TestNativeBackendRetirement|TestRetiredRuntimeHistory' -count=1 -v`, exit 0, 33 top-level PASS, including all 12 CMF19, 5 CMF20 and 2 CMF21 tests; focused.log. TestO141 is historically named TestWizard.
- Unchanged neutral authorization regression from CMF21, unique test name only: before 6f165d30 exit 1, complete=true/remove-runtime incorrectly authorized; head d409f26f exit 0 with fail-closed error. authorization-before.log and authorization-head.log; scripts/cmf22-{before,head}-authorization-*.
- CMF21 preparation JSON assertions and measured-zero positive control passed in focused.log. Additional reviewer overlay logs actual classified/encoded CLI preparation JSON in preparation-json.log, exit 0: outer provider_retirement_unverifiable, nested runtime-inventory-not-yet-materialized with runtime-unavailable code plus resource/instance/Target/application UUID. This is actual production preparation/classification/serialization, no destructive live lifecycle claimed.
- Eight independent owner-file negatives (missing compose/env, symlink directory/compose, broad permissions directory/compose, noncanonical compose, unreadable directory) and decoded ownership/final-retirement CLI JSON pass, exit 0, extra.log. Existing CMF20/21 reviewer assertions reused through new overlays.
- CGO_ENABLED=0 go build ./... exit 0, build.log; go vet ./cmd/baha/ exit 0, vet.log; gofmt -l cmd/baha internal exit 0 and empty, gofmt.log; source-readability-audit.sh exit 0, readability.log; git diff --check exit 0.
- 21 protected footprint/consent/digest/q124 paths byte-identical to baseline; protected-diff.json stores exact SHA256s. No unverifiable wildcard introduced. Fresh-list confirmation helper retained in actual normal observation path.

Fixture setup corrections: initial extra-overlay generation used an incorrect relative path and then assumed a following function after the final owner-file function; both reviewer-only generator errors were corrected before execution. An initial protected-file comparison similarly used the source-export cwd rather than repository cwd; rerun successfully with absolute paths. No candidate changes or failed product assertions resulted from those setup mistakes.

## Observation consumer audit
Every non-test reference to observeApplicationProviderFacts*, including its function-variable alias, was traced:

| Consumer | Boundary and outcome |
| --- | --- |
| app_core_requirement.go:229 buildResolvedApplicationPlan | Read/footprint/impact planning. Unknown preview quantities stay unknown. Impact Realization.Verified is supplied by separate protected inventory/native inspector, never copied from ProviderStoppedOwned. Authorization through application_provider_authorization.go revalidates consent/intent and execution rules; actual last-consumer retirement independently reaches preparation. |
| app_new_wizard.go:239 and app_edit.go:302 | Initial wizard/Edit snapshots and presentation, not runtime verification; actual Edit execution enters locked Apply preparation and independent receipts. |
| application_runtime_retirement_plan.go:12 | Read-only withdrawal/impact proposal; app_runtime_retirement.go:49 preparation is mandatory before runtime/owner mutation, then line 78 observes again for final measurement verification. |
| application_stop_impact.go:42 | Consequence plan, not verified runtime receipt. Native impact adapter separately inspects canonical ownership. Stop result validates preserved file/registry/deployment state and actual execution; preview facts do not attest stop success. |
| application_provider_retirement.go:18,39-47 | Structured observer; selected last-consumer stop/remove-runtime requires matching Target and providerFactIsRuntimeMeasurement. No nil/unknown quantity can qualify. Existing released-consumer/complete-union and adapter gates remain. |
| application_runtime_retirement_verification.go:22 | Final retirement requires matching Target, measured nonnil zero containers and allowed physical state. app_runtime_retirement.go:78-80 attaches typed diagnostics to serialized failure Details. |
| app_core_requirement.go:306-320 | Retained-unbound ownership requires measured facts and allowed state; rejects registry projection and preserves diagnostics. |
| app_apply.go:147 and app_apply_result.go:146 via observeApplicationProviderFactsFunc | Before/after evidence collection. appApplyObservedOutcomes and measured helpers at app_apply_result.go:398-424 require measured Target-scoped facts for verified receipts. |
| app_edit_apply_result.go:264,286 via alias | Choice and desired impact planning snapshots; actual mutation enters locked Apply preparation, protected ownership and measured outcome authorities. |

Independent added regression TestCMF22ProjectionCannotBecomeVerifiedReceiptOrImpact passes (consumers.log): real registry-projection observation does not set impact Realization.Verified and cannot produce applied/verified receipts for stop/remove-runtime/add/resume/start/replace/update. This supplements the direct real retirement/ownership/footprint regressions; not a claim of live runtime execution.

F1–F5/R1/R2 assessment: F1 genuine detector/inspector failures return no synthetic facts (observation.go:133-145); F2 exact per-instance binding and owner tuple required (237-251,278-303); F3 empty-Store preview avoids CWD lookup, filesystem fallback distinguishes missing directory from owner-file failures (99-131,210-213); F4 every eligible SQL/Valkey peer independently projected (237-269); F5/R1 projections have unknown quantities (252-267), typed diagnostic code/context is serialized through applicable authorization/verification failures (366-418 and retirement.go:65-72). R2 now rejects the earlier complete/remove-runtime preview plan in the actual preparation authority. All named recipe regressions were independently rerun and pass, including measured positive.

Additional optional domain check `go test ./internal/application/ -run 'Consent|Digest|Provider' -count=1`: exit 1 on both baseline and candidate, with the same three top-level failures / five failed names including docker/podman subtests. All failures are inability to open /home/marcel/.local/share/baseharbor/observability/signals.json.lock (read-only filesystem). domain-focused.log and domain-base.log. This optional broader check is not green and does not assert native runtime qualification.

## Baseline honesty — complete independent full suites

Both ordinary CLI suites ran sequentially, once per exact Git-object export, with identical environment and configuration:

```
GOCACHE=/tmp/cmf20-go-cache go test ./cmd/baha/ -count=1 -json
```

| Exact source | Result | Elapsed | Pass events | Skip events | Failed top-level / all names |
| --- | --- | --- | --- | --- | --- |
| 2f9afafdc443ba4b15569fca838511877c1fd3ba | exit 1 | 445.283 s | 1633 | 44 | 9 / 12 |
| d409f26f4a6261c2dc614320fbfebd050a637cb8 | exit 1 | 445.037 s | 1659 | 44 | 9 / 12 |

**NEW=0, REMOVED=0; identical failure sets.** Independently computed from completed package-result events by scripts/cmf22-compare.py; machine result cmf22-data/failure-comparison.json. Raw logs base-full.jsonl/head-full.jsonl, stderr and exit files retained. Neither ordinary full suite is green; skipped native tests are not passes.

| Matching failure on both SHAs | Actual reason on both |
| --- | --- |
| TestNonTTYBackupDefaultUsesProtectedExternalArchive | Source exports are inside enclosing Git worktree; default archive destination fails protected external-path boundary. |
| TestCorelessRuntimeFilesHaveNoBackendResources; TestBug866WorkloadAccessEnvironmentIncludesNativeBindings | Global observability signals.json.lock is read-only in this sandbox. |
| TestAppEditCommandCompleteProviderEditRunsRealLifecycle | Docker endpoint unix:///var/run/docker.sock unavailable. |
| TestExternalProviderRemovalTransportsShareScopeAndExplicitStages (cli/http/mcp); TestExternalProviderRemovalChecksEveryManagedConsumerBeforeMutation; TestExternalProviderRemovalCannotInferGlobalOwnershipFromTenantRole | provider-registry-ownership-unverifiable. |
| TestRecoveryFileForRepositoryUpUsesTargetDefaultNonInteractive | probe-x versus expected local recovery default. |
| TestWritePathPromptContextShowsCurrentDirectory | Display abbreviates /home/marcel/tmp-ops as ~/tmp-ops, assertion expects full path. |

Complete failure messages retained in base-failure-output.json and head-failure-output.json. No broad reruns or environment repairs were performed to hide inherited failures.

## Verdict and actionable next step

**ACCEPT** for exact candidate `d409f26f4a6261c2dc614320fbfebd050a637cb8`, full reviewed diff from `2f9afafdc443ba4b15569fca838511877c1fd3ba`.
UTC verdict: 2026-10-25T21:20:27.717516+00:00.

F1–F5, R1 and R2 scoped stop criteria are satisfied by source inspection, independently discriminating authorization regression, measured-runtime positive control, actual decoded CLI JSON, owner-file/ownership/q124 regressions and NEW=0. No blocking source finding remains in this review. No worker correction recipe is necessary for this accepted candidate.

**FF-Push d409f26f auf fix/v0.4.26-edit-preview-error ist Maint-naechster Schritt; keine Merge-Empfehlung vor gruenem Runtime-Gate.** Concretely, the Maintainer should refresh the remote branch and verify its current HEAD remains an ancestor of the accepted exact candidate, preserve concurrent changes, then perform the ordinary fast-forward push under the separately applicable authorization. A changed candidate invalidates this SHA-specific review. No force push. Required actual runtime evidence, selected required CI, acceptance and integration gates remain mandatory before merge; this verdict does not establish those remote gates or release qualification.

## Final integrity and repo-rule compliance

ROLLE remained REVIEW. Candidate HEAD after review equals initial `d409f26f4a6261c2dc614320fbfebd050a637cb8`; git status --porcelain empty. All tracked source-export files still match exact git archives: baseline 2551/2551, candidate 2555/2555, before 2554/2554 (source-integrity.json). Complete reviewed diff SHA256 f060f4a651f8498deea81e85b3bc6cfb26a8120aa08191b7b761dbacc59ef29a. Final live PR1077 remains OPEN at 2f9afafdc443ba4b15569fca838511877c1fd3ba. NTP synchronized, host UTC and clock tool agree.

No implementation correction, product edit, commit, push, merge, approval, PR comment, CI dispatch, release, tag, deployment, controller activation, worker spawn, lane deletion, claim takeover or Planning publication. Existing ownership, sessions and coordinator plan preserved. Initial seven watcher events and 96 reminders inspected and acknowledged as operational data; unrelated pool/stale-plan flags recorded for coordinator rather than executed outside scope. Current session is the independent Codex reviewer; exact model/backend/session/Horde identifiers are unavailable and not fabricated. All review fixtures/logs/exports live under the evidence tree; cache remains disposable /tmp data. Native TodoWrite is not exposed, so only the evidence PLAN could be maintained; no native action is claimed.

END CMF22-REVIEW-DONE
