# CMF20 independent correction review

ROLLE: REVIEW

Mission: `ops/missions/cmf20.txt`. Reviewer: Codex, current API session; exact model/backend/session identifier unavailable through session tools.

## PLAN

- [x] 0. Read authority files and mission; establish read-only REVIEW scope. Mission contains no numbered PLAN step 0; this step records the user's additional requirement. Native TodoWrite is unavailable: tool discovery found no TodoWrite/todowrite/update_plan tool. This evidence checklist does not claim a native tool action.
- [x] 1. Read repository rules, complete exact diff and F1–F5 recipes; pin candidate and baseline identities and tracked state.
- [x] 2. Run each ported CMF19 regression and complete relevant fail-closed suite; inspect consent/footprint/q124 semantic preservation.
- [x] 3. Independently run full cmd/baha suites on 2f9afafd and 25b46572 in isolated checkouts; compare actual failure sets and explain environmental failures.
- [x] 4. Execute build, vet and gofmt checks; finish F1–F5 file:line assessment and actionable finding.
- [x] 5. Verify candidate SHA/content unchanged; record UTC, verdict and required END marker.

## Scope and operational flags

Binding cost stop permits necessary independent review; mission prohibits push, merge and CI dispatch. Production/Maintainer pause remains active. Basics flags report watcher events/reminders, idle f61, pool underfill and 32 coordinator PLAN steps. This bounded reviewer will inspect operational notifications as data and preserve existing lanes, ownership and coordinator plan. Pool dispatch, lane retirement and coordinator takeover are outside this mission; no implementation work will be started.

## Candidate identity and exact scope

Before review: `25b465723b3f0bd2ac8439d77ecebcb21b2f7628`, tracked worktree clean. Actual ancestry is linear: candidate parent `61acf0e6e768a4446c02f3c4d6331a9ab846f3bb`, whose parent is baseline `2f9afafdc443ba4b15569fca838511877c1fd3ba`; candidate is not a two-parent merge. Complete reviewed diff is retained in `cmf20-data/reviewed.diff`: 5 files, 1122 insertions and 23 deletions against the requested baseline.

Independent source checkouts were exported directly from those exact Git objects into separate `cmf20-data/base/` and `cmf20-data/head/` directories using `git archive` (no Git metadata, no product source edits). All runs below use those isolated source checkouts; they do not substitute `61acf0e6` for the baseline. Added reviewer tests are overlays, retained in the prescribed `scripts/` evidence path.

Live read-only GitHub check: authenticated account is `mcpdev80`; PR1077 is OPEN at baseline `2f9afafdc443ba4b15569fca838511877c1fd3ba`, targeting `develop`; develop was `433a5702c9735ffbf1e9fffe7a7ae513b1c5c396`, Planning main `f8157ecf9c349aec021c36537a9db9cebca9aec5`. SSH `ls-remote` failed due host SSH configuration permissions; `gh` API reads supplied the remote identities without changing SSH configuration. Local Planning task `t-20261019-v0426-complete-defect-audit` is active; its owner/claim was preserved. No Planning writes or status transitions are authorized by this review. Repository delegated-Maintainer language does not change this mission's strict REVIEW scope or bootstrap gates.

## F1–F5 assessment

Every named test in this table was independently executed in `cmf20-data/focused.log` with `GOCACHE=/tmp/cmf20-go-cache go test ./cmd/baha/ -run 'TestCMF19|TestWizard|TestO141|TestZeroContainerListing|TestProviderContinuation' -count=1 -v`; exit 0. All 12 ported top-level CMF19 tests pass. The port retains the four original CMF19 regression names and extends their fixtures/assertions.

| Recipe | Exact candidate implementation | Independent regression result and assessment |
| --- | --- | --- |
| F1 | `cmd/baha/application_native_backend_observation.go:133-140` restores fail-closed detection/inspection errors. The registry fallback occurs separately at `100-119`. | `TestCMF19RuntimeDetectionFailureMustRemainUnverifiable` and `TestCMF19InspectorFailureMustRemainUnverifiable` pass (`cmf19_recipes_test.go:105,127`). q124 fresh-list and continuation tests also pass. **Fulfilled for genuine detection/inspection failures with valid inventory.** |
| F2 | `application_native_backend_observation.go:237-250,272-297` checks owned scope and the exact instance/application/environment/provider/resource/offered-capability binding. Positive wizard fixture now creates a real binding (`o141_wizard_apply_factgap_test.go:198-203`). | Unbound, foreign binding, missing-instance/missing-intent tests pass (`cmf19_recipes_test.go:160,181,215`); retained unbound ownership is rejected. **Fulfilled.** |
| F3 | `application_native_backend_observation.go:100-130,210-213` separates empty-store preview from owner filesystem inspection and checks absence of the directory itself for the exception. Existing-directory ENOENT does not itself trigger the registry exception. Inverted fixture is corrected at `o141_wizard_apply_factgap_test.go:80-94`. | Missing-env and two-CWD tests pass (`cmf19_recipes_test.go:260,292`); independently added eight-case owner-file suite also passes: missing compose/env, symlink directory/compose, broad-permission directory/compose, noncanonical compose and unreadable directory. These negatives use an unsupported runtime to ensure no runtime can accidentally verify absence. Missing-directory positives are exercised by the per-instance F4 tests. **Fulfilled at the exception/path boundary; this does not qualify projected facts as runtime measurements.** |
| F4 | `application_native_backend_observation.go:237-263` evaluates all instances without a first-match return; each instance requires its own binding/resource proof. | Two SQL, SQL+Valkey, and mixed eligible/ineligible tests pass (`cmf19_recipes_test.go:330,359,402`). **Fulfilled.** |
| F5 | `application_native_backend_observation.go:324-347` creates a typed stable machine error; `app_core_requirement.go:334-350` aggregates diagnostics. However `323-325` discards them, and `314` chains them only through a nonserialized `Cause`. | Existing errors.As/direct JSON test and retained-ownership internal-chain test pass (`cmf19_recipes_test.go:434,475`). Independently added production-consumer and machine-result regressions fail. **NOT fulfilled end to end.** |

File references in the first column are within the candidate checkout. The original wrapper's existence does not excuse discarding the newly required diagnostic: its facts now include the newly generated registry-only measured zero.

## Blocking finding R1 — P1: F5 diagnostics disappear while preview facts authorize runtime verification

Locations: `cmd/baha/app_core_requirement.go:323-325,303-314`; `cmd/baha/application_native_backend_observation.go:252-260`.

`observeApplicationProviderFacts` discards the error from the new diagnostic-aware observer and returns `ProviderStoppedOwned` with `Containers=0`, `Classification=measured`. Existing callers include `buildResolvedApplicationPlan` (`app_core_requirement.go:226`), New (`app_new_wizard.go:237`), Edit (`app_edit.go:300`), stop/retirement planning and runtime verification (`app_runtime_retirement.go:78`). Those callers have no signal that the value came from an unmeasured registry projection.

The independent `TestCMF20RegistryProjectionCannotVerifyRuntimeRetirement` uses the same standalone fixture and test source against both SHAs: a real exact application-scoped PostgreSQL binding, absent runtime directory and explicitly unsupported runtime. The baseline correctly rejects runtime-retirement verification; the candidate incorrectly returns nil from `verifyApplicationRuntimeRetirement` using only projected facts. This establishes a new verification defect in the actual observation/consumer path, without requiring a live runtime or changing production source. It does not claim that an entire destructive lifecycle was executed.

The second independent test, `TestCMF20RetainedOwnershipMachineResultMustPreserveDiagnostic`, serializes `machine.ResultError(err)` from the real ownership caller. It loses `runtime-inventory-not-yet-materialized`: `machine.Error.Cause` is tagged `json:"-"` (`internal/machine/contract.go:68`), and the emitted JSON contains only `retained_provider_ownership_unverified`. Marshaling the inner helper error directly, as the candidate test does, cannot establish caller/CLI/JSON propagation.

Exact worker recipe:

1. **Change:** update `app_core_requirement.go` observation consumers and the new projection creation in `application_native_backend_observation.go` so a preview-only projection cannot enter the ordinary verified/measured fact path. Carry typed per-instance diagnostics through the existing structured caller/result/output path; consumers must explicitly distinguish preview from verification. Preserve fail-closed runtime verification and all q124 fresh-list behavior. Keep the stable materialization cause/code and instance/target context visible in actual machine output; an ignored return value or Go-only `Cause` chain is insufficient. Do not alter consent digest, `footprint.go` or introduce an unverifiable wildcard.
2. **Tests:** port the neutral before/after regression, the wrapper/retirement regression and the real `machine.ResultError` serialization regression from this evidence; add actual New/Edit/lifecycle output assertions for the same condition. Require no successful authorization/retirement verification from diagnostic-only evidence. Keep all 12 CMF19 regressions, the eight owner-file negatives and q124 continuation/fresh-list cases green. Compare the complete ordinary cmd/baha suite against `2f9afafd`, with identical environment and NEW=0.
3. **Stop criterion:** no consumer silently accepts the preview projection as runtime measurement; all applicable CLI/JSON callers preserve the typed diagnostic; genuine runtime failures remain unverifiable; all required unchanged safety semantics and regression gates pass on the final SHA. Implementation belongs to the worker, not this reviewer.

Reproduce the neutral control from each `cmf20-data/{base,head}` source checkout:

```sh
GOCACHE=/tmp/cmf20-go-cache go test -overlay /home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-25/scripts/cmf20-neutral-base-overlay.json ./cmd/baha/ -run '^TestCMF20RegistryProjectionCannotVerifyRuntimeRetirement$' -count=1 -v
# In head/, select cmf20-neutral-head-overlay.json instead.
```

Run the expanded reviewer cases from `cmf20-data/head/`:

```sh
GOCACHE=/tmp/cmf20-go-cache go test -overlay /home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-25/scripts/cmf20-overlay.json ./cmd/baha/ -run '^TestCMF20' -count=1 -v
```

Expected current result: neutral baseline exit 0, neutral candidate exit 1; expanded candidate exit 1 for retirement verification and machine JSON propagation. Logs: `cmf20-data/neutral-base.log`, `neutral-head.log`, `review-regressions.log`.

## Consent, footprint and q124 semantics

No consent-digest or `footprint.go` file changes occur in the exact diff. q124's normal runtime path still invokes `observeCanonicalProviderContainersConfirmed` (`application_native_backend_observation.go:190`) and its helper is unchanged. The full fresh-list/continuation tests pass, including both provider kinds and all partial/foreign/unhealthy/init negative subcases. However the new registry-only early-return paths bypass real measurement, and R1 proves that the missing diagnostic lets an actual downstream verifier treat their result as success. Therefore the requested global assertion of unchanged fail-closed confirmation semantics cannot be made; this is a blocking finding, not a green check inferred from unchanged helper text.

## Executed checks and limitations

All commands ran locally; no hosted CI or native Docker/Podman gate was dispatched. Candidate production source was not edited.

| Command / check | Actual result | Evidence |
| --- | --- | --- |
| `CGO_ENABLED=0 go build ./...` | exit 0 | `cmf20-data/build.log` |
| `go vet ./cmd/baha/` | exit 0 | `cmf20-data/vet.log` |
| `gofmt -l cmd/baha internal` | exit 0, no listed files | `cmf20-data/gofmt.log` |
| `bash scripts/source-readability-audit.sh` | exit 0, all ratchets pass | `cmf20-data/readability.log` |
| Required focused wizard/CMF19/q124 suite | exit 0 | `cmf20-data/focused.log` |
| Neutral retirement regression on baseline | exit 0 | `cmf20-data/neutral-base.log` |
| Same neutral regression on candidate | exit 1: unmeasured projection incorrectly verifies retirement | `cmf20-data/neutral-head.log` |
| Expanded reviewer suite | exit 1: retirement-verification and actual machine-result propagation regressions fail; all eight owner-file negatives pass | `cmf20-data/review-regressions.log` |
| Extra `go test ./internal/application/ -count=1 -v` | exit 1, 35 top-level failures with read-only `/home/marcel/.local/share/baseharbor/observability/signals.json.lock` errors; relevant native backend inventory tests pass | `cmf20-data/application-tests.log` |
| Exact source integrity against Git blobs | baseline 2551/2551 and candidate 2553/2553 match | `cmf20-data/source-integrity.json` |

The shell's normal Go build cache is read-only in this session. Initial attempts could not start tests; the executed runs consistently use the writable `GOCACHE=/tmp/cmf20-go-cache`. No source or safety-test behavior was modified to accommodate sandbox restrictions. The extra internal/application run is explicitly red in this environment; the worker's green claim is not independently reproduced here. Its failures were not corrected by this reviewer.

## Baseline honesty: complete independent runs

Both full commands completed normally (no timeout), each exit 1:

```sh
# cmf20-data/base/, exact 2f9afafdc443ba4b15569fca838511877c1fd3ba
GOCACHE=/tmp/cmf20-go-cache go test ./cmd/baha/ -count=1 -json > ../base-full.jsonl 2> ../base-full.stderr
# cmf20-data/head/, exact 25b465723b3f0bd2ac8439d77ecebcb21b2f7628
GOCACHE=/tmp/cmf20-go-cache go test ./cmd/baha/ -count=1 -json > ../head-full.jsonl 2> ../head-full.stderr
```

Baseline elapsed 564.687 seconds; candidate 565.650 seconds. Baseline has 1633 passing test/subtest events and candidate 1652; each has 44 skipped test/subtest events. Skipped native gates are not claimed as passed. Each run has **9 identical top-level failures**, or **12 identical failed names including the three transport subtests**. Exact complete sorted-set comparison: **NEW=0, REMOVED=0**. Reproduce comparison with `python3 evidence/board-2026-10-25/scripts/cmf20-compare.py`; it refuses incomplete logs. Machine-readable result: `cmf20-data/failure-comparison.json`.

| Existing failure(s) on both SHAs | Observed matching reason |
| --- | --- |
| `TestNonTTYBackupDefaultUsesProtectedExternalArchive` | Default archive destination resolves within the enclosing Git workspace in these source-export checkouts. |
| `TestCorelessRuntimeFilesHaveNoBackendResources`, `TestBug866WorkloadAccessEnvironmentIncludesNativeBindings` | Read-only global observability `signals.json.lock` path. |
| `TestAppEditCommandCompleteProviderEditRunsRealLifecycle` | Selected Docker endpoint `unix:///var/run/docker.sock` unavailable. |
| `TestExternalProviderRemovalTransportsShareScopeAndExplicitStages` (cli/http/mcp), `TestExternalProviderRemovalChecksEveryManagedConsumerBeforeMutation`, `TestExternalProviderRemovalCannotInferGlobalOwnershipFromTenantRole` | Same `provider-registry-ownership-unverifiable` error on both SHAs; these are baseline-existing failures in this environment, not new candidate regressions. |
| `TestWritePathPromptContextShowsCurrentDirectory` | Path display abbreviates `/home/marcel/tmp-ops` through `~`, conflicting with the assertion. |
| `TestRecoveryFileForRepositoryUpUsesTargetDefaultNonInteractive` | Same `probe-x` versus expected `local` target-default path mismatch. |

This is the requested comparison against `2f9afafd`, not the worker's comparison against `61acf0e6`. It establishes equality of the full existing failure set in this test environment; it does not make either full suite green. The separate new reviewer regression is deliberately outside those ordinary suites and exposes the missing F5 invariant.

## Verdict and final integrity

**REQUEST_CHANGES** — UTC `2026-10-25T18:50:46Z`.

F1–F4 satisfy their reviewed boundary checks; F5 fails end-to-end propagation and permits unmeasured preview facts to satisfy runtime verification. Apply R1's exact worker recipe and review the resulting final SHA. An FF push of this candidate is not qualified as the next action by this verdict.

Candidate after: `25b465723b3f0bd2ac8439d77ecebcb21b2f7628`, identical to before. Final `git diff --exit-code HEAD` and `git diff --cached --exit-code HEAD` both exit 0; `git status --short` is empty. Host UTC and clock-tool UTC agreed at the timestamp above.

## repo-rule compliance

ROLLE remained REVIEW. No product implementation, source edits, commits, push, merge, approve, PR writes, CI dispatch, release, tag, deployment, worker spawn, controller startup, lane deletion or Planning acceptance/publishing occurred. Review tests, source snapshots and logs remain in the evidence tree; `/tmp/cmf20-go-cache` is a disposable build cache. Existing claims and sessions were preserved. Initial watcher events/reminders were inspected as data and acknowledged; pool, idle-lane and coordinator PLAN actions remain deferred to the existing coordinator under the bounded mission and pause. Native TodoWrite remained unavailable; the evidence PLAN is complete and no native Todo action is fabricated.

END CMF20-REVIEW-DONE
