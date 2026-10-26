ROLLE: REVIEW

Status: **REQUEST_CHANGES** (CMF21). Blocking R2: retirement preparation still authorizes registry-only preview facts. No FF-push recommendation for this SHA. No push, merge or CI dispatch.
UTC start: 2026-10-25T19:45:37.528293+00:00
Candidate before: 6f165d30df4f3c4b7f08623feff2eee58249e688; working tree clean.

## PLAN
- [x] 0. Read owner policies and CMF21, identify exact candidate, record numbered plan before review changes. Native TodoWrite is not exposed by this session; tool discovery found no native todo/plan tool. CMF21 itself has no PLAN step 0. This evidence checklist is the available durable substitute, not a claim of native TodoWrite execution.
- [x] 1. Read repository rules, R1/F1–F5 verdicts and source diff; verify live references and scope.
- [x] 2. Independently exercise both R1 halves: verified retirement/ownership fail closed; materialization diagnostic appears in serialized machine JSON.
- [x] 3. Run CMF19, CMF20 and q124 regression checks against exact candidate.
- [x] 4. Run one baseline cmd/baha test suite at 2f9afafd and candidate suite, independently compare failures (NEW=0 required).
- [x] 5. Verify unchanged consent digest, footprint.go and q124 confirmation; build, vet and formatting.
- [x] 6. Recheck SHAs and clean source; publish ACCEPT with exact FF recommendation or REQUEST_CHANGES with actionable recipes.

## Scope and operating flags
Independent bounded review explicitly authorized by CMF21; no implementation or controller activation. Global pool/old coordinator plan flags do not authorize coordinator takeover, spawning workers or modifying another session's plan. Existing global events/reminders will be inspected without taking ownership of unrelated work. Historical conflicting pause/resumption policy text does not change CMF21's explicit no-push/no-merge/no-CI-dispatch scope.

## Live reference check
GitHub account mcpdev80; PR1077 OPEN, head 2f9afafdc443ba4b15569fca838511877c1fd3ba; develop 433a5702c9735ffbf1e9fffe7a7ae513b1c5c396; Planning main f8157ecf9c349aec021c36537a9db9cebca9aec5. SSH ls-remote failed on system SSH config permissions; read-only gh API supplied live references. No configuration changes. Host reports NTP synchronized. Initial global events/reminders inspected and acknowledged as data; no spawn/merge instructions from them executed. Task core-1077 does not exist; PR1077 belongs to the active t-20261019-v0426-complete-defect-audit, whose ownership is preserved.

## Independently executed focused checks

All runs use exact source exports under `cmf21-data/{base,head}`, generated via `git archive` from the specified Git objects. No candidate files are edited; reviewer tests use overlays in `scripts/cmf21*`. Go cache is the writable `/tmp/cmf20-go-cache`, consistently reused from the prior review. Build/test versions follow go.mod (Go 1.25.0, toolchain go1.26.9).

- `GOCACHE=/tmp/cmf20-go-cache go test ./cmd/baha/ -run 'TestCMF19|TestCMF20|TestWizard|TestO141|TestZeroContainerListing|TestProviderContinuation' -count=1 -v`: exit 0 (`cmf21-data/focused.log`). 12 CMF19, 5 CMF20, 5 Wizard, 1 fresh-list and 2 continuation top-level tests pass. No test named TestO141 exists; its fixture regressions are named TestWizard.
- Independent unchanged neutral retirement regression from CMF20, renamed only to avoid duplicate test declarations: baseline exit 0 (`neutral-base.log`), candidate exit 0 (`neutral-head.log`). Both reject runtime-retirement verification from registry-only observation with unsupported runtime. This closes the final verification defect only; the separately tested preparation/authorization consumer below remains blocking.
- Eight independently overlaid CMF20 owner-file negatives pass, exit 0 (`owner-file-negatives.log`): missing compose/env, symlink directory/compose, broad permissions directory/compose, noncanonical compose, unreadable directory. No success facts or materialization diagnostics for these genuine failures.
- Independent structured output regression uses real `classifyMachineCLIError` -> `machine.ResultError` -> `writeJSON`, then decodes and checks outer failure plus every materialization cause/code/resource/application/target entry. Ownership and retirement both pass, exit 0 (`structured-json.log`). It invokes real observation, retained ownership, verification and output functions; it does not claim a full live destructive lifecycle was executed.
- First structured-output test failed solely because the review fixture asserted exactly one diagnostic, whereas existing owner aggregation emits duplicate identical per-instance entries. All entries contain the required cause/context. The corrected reviewer assertion validates a nonempty array and every entry. Original log retained as `structured-json-first.log`; no production fix made or required by the scoped R1 contract. Duplicate entries are a nonblocking presentation observation.
- `CGO_ENABLED=0 GOCACHE=/tmp/cmf20-go-cache go build ./...`: exit 0 (`build.log`).
- `GOCACHE=/tmp/cmf20-go-cache go vet ./cmd/baha/`: exit 0 (`vet.log`).
- `gofmt -l cmd/baha internal`: exit 0, no output (`gofmt.log`).
- `bash scripts/source-readability-audit.sh`: exit 0, all ratchets pass (`readability.log`).
- `git diff --check 2f9afafd..6f165d30`: exit 0.

## Source-level assessment

F1 retains fail-closed real detector/inspector/listing failures (`application_native_backend_observation.go:133-145`). F2 checks instance owner and exact binding (`237-251,278-303`). F3 separates no-store preview from filesystem observation and qualifies directory absence only (`99-131,210-213`). F4 records every eligible peer independently (`237-269`). F5/R1 now emits preview quantities without measured values (`252-267`), requires measurements for retirement (`application_runtime_retirement_verification.go:18-25`) and retained-unbound ownership (`app_core_requirement.go:310-320`), and serializes typed context through Details (`application_native_backend_observation.go:366-418`, `app_runtime_retirement.go:78-80`). Actual regression results above substantiate these boundaries.

`cmf21-data/protected-diff.json` records the complete 10-file changed-path list and SHA256 comparison of footprint.go, consent/digest files, and q124 confirmation helper/tests: all identical to baseline. The ordinary native observation still calls `observeCanonicalProviderContainersConfirmed` at line 192. Preview projection is now explicitly unknown and cannot pass its measurement consumers; no wildcard was added. The exact R1 diff is `cmf21-data/r1.diff`; the full baseline-to-candidate diff is `cmf21-data/reviewed.diff`.

Full cmd/baha runs completed; exact results and final verdict follow below.

## Blocking finding R2 — P1: retirement preparation still authorizes the preview-only projection

Locations: `cmd/baha/application_provider_retirement.go:17-20,31-50`; affected resolver `internal/application/provider_union.go:146-163`; producer `cmd/baha/application_native_backend_observation.go:252-267` and diagnostic-discarding wrapper `cmd/baha/app_core_requirement.go:338-340`.

The correction protects final `verifyApplicationRuntimeRetirement` and retained-unbound ownership, but `prepareApplicationProviderRetirement` still consumes the wrapper's map, ignores the typed diagnostic and never checks `providerFactIsRuntimeMeasurement`. The resolver treats `ProviderStoppedOwned` with nonnil dependencies as verified even when containers are unknown. The preparation gate therefore returns a complete `remove-runtime` plan for a last-consumer native backend with no canonical runtime inventory and an explicitly unsupported runtime. The lifecycle explicitly uses this preparation function as an authority before mutation (`app_runtime_retirement.go:49-53`). Later final verification rejecting the projection does not restore this earlier fail-closed gate.

Independent neutral before/after regression: `TestCMF21ProjectionCannotAuthorizeProviderRetirement`. Identical test source on both exact SHAs; real exact registry binding, registered deployment owner, missing runtime inventory, unsupported runtime and withdrawn desired capabilities. Baseline `2f9afafd` exits 0 and returns `provider_retirement_unverifiable`. Candidate `6f165d30` exits 1 because preparation returns nil error, `Complete=true`, `Action=remove-runtime`. Evidence: `cmf21-data/authorization-base.log`, `authorization-head.log`; standalone fixture `scripts/cmf21-authorization-review.go` and baseline/head overlay JSONs. No runtime removal was executed. This is an actual production preparation/authorization regression, not a claim that every later physical ownership guard can be bypassed.

### Exact worker recipe

1. **File/change:** in `cmd/baha/application_provider_retirement.go`, consume `observeApplicationProviderFactsWithDiagnostics`; before authorizing any selected last-consumer `stop`/`remove-runtime`, require actual Target-scoped runtime measurement/ownership evidence. Registry-only preview projections must remain usable on preview surfaces but must not qualify this verification/authorization gate. Return the existing fail-closed retirement-unverifiable error for diagnostic-only facts and preserve materialization code/cause plus instance/Target in serialized Details alongside existing plan/observation evidence. Avoid weakening the common resolver, consent or native physical inspection. If the worker instead introduces a distinct verification observer, audit all authorization consumers of the wrapper and document the chosen boundary.
2. **Tests:** port the unchanged neutral authorization regression from `scripts/cmf21-authorization-review.go` into the product tests, using the real preparation caller; require failure-closed on absent canonical inventory/unsupported runtime and assert actual serialized JSON carries `provider_retirement_unverifiable` and nested `runtime-inventory-not-yet-materialized` with code/instance/Target. Add a measured-runtime positive control so valid preparation remains supported. Preserve both already-correct R1 final-verification/retained-owner paths, all CMF19/CMF20/owner-file/q124 regressions. Independently compare the full cmd/baha suite against `2f9afafd` with NEW=0 on the final candidate.
3. **Stop criterion:** no preparation/authorization or final verification consumer accepts an unmeasured preview projection as verified runtime/ownership. Applicable JSON failures retain typed materialization context; genuine runtime failures remain unverifiable; consent digest, footprint.go and q124 remain unchanged. Worker implements; Codex re-reviews the exact new SHA. No FF-push recommendation for the current SHA.

Reproduce from each `cmf21-data/{base,head}` source export:

```sh
GOCACHE=/tmp/cmf20-go-cache go test -overlay /home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-25/scripts/cmf21-authorization-base-overlay.json ./cmd/baha/ -run '^TestCMF21ProjectionCannotAuthorize' -count=1 -v
# In head/, choose cmf21-authorization-head-overlay.json.
```

## Baseline honesty — complete independent full runs

Executed once per exact source export with the same environment:

```sh
# cmf21-data/base/: 2f9afafdc443ba4b15569fca838511877c1fd3ba
GOCACHE=/tmp/cmf20-go-cache go test ./cmd/baha/ -count=1 -json > ../base-full.jsonl 2> ../base-full.stderr
# cmf21-data/head/: 6f165d30df4f3c4b7f08623feff2eee58249e688
GOCACHE=/tmp/cmf20-go-cache go test ./cmd/baha/ -count=1 -json > ../head-full.jsonl 2> ../head-full.stderr
```

Both commands completed normally, exit 1. Baseline elapsed 550.290 seconds, candidate 553.272 seconds. Baseline has 1633 pass events; candidate 1657. Each has 44 skipped test/subtest events. Each has the same 9 failed top-level tests / 12 failed names including three transport subtests. **NEW=0, REMOVED=0**, independently computed from complete run logs by `scripts/cmf21-compare.py`, which refuses incomplete results. Machine-readable comparison: `cmf21-data/failure-comparison.json`. Neither ordinary full suite is green; skipped native tests are not passes. This does not erase the separate new R2 reviewer regression.

| Same existing failures on both SHAs | Actual matching reason |
| --- | --- |
| TestNonTTYBackupDefaultUsesProtectedExternalArchive | Source exports are inside enclosing Git workspace; default backup destination fails that boundary check. |
| TestCorelessRuntimeFilesHaveNoBackendResources; TestBug866WorkloadAccessEnvironmentIncludesNativeBindings | Global observability signals.json.lock is read-only in this sandbox. |
| TestAppEditCommandCompleteProviderEditRunsRealLifecycle | Selected Docker endpoint unix:///var/run/docker.sock unavailable. |
| TestExternalProviderRemovalTransportsShareScopeAndExplicitStages (cli/http/mcp); TestExternalProviderRemovalChecksEveryManagedConsumerBeforeMutation; TestExternalProviderRemovalCannotInferGlobalOwnershipFromTenantRole | Same provider-registry-ownership-unverifiable failure. |
| TestWritePathPromptContextShowsCurrentDirectory | Actual display abbreviates /home/marcel/tmp-ops via ~; assertion expects full path. |
| TestRecoveryFileForRepositoryUpUsesTargetDefaultNonInteractive | Actual probe-x recovery default versus expected local default. |

## Final verdict and integrity

**REQUEST_CHANGES** — UTC 2026-10-25T19:56:39.134292+00:00. R1 half (a) is only partially fulfilled: final retirement verification and retained-unbound ownership reject preview facts, but the real retirement-preparation authority still accepts them (R2). R1 half (b) is fulfilled on the corrected ownership/final-retirement machine-output paths, including decoded cause/code and instance/Target context. F1–F4 boundary regressions remain green; F5's end-to-end distinction remains incomplete because of R2. Apply the exact worker recipe above and independently review the resulting SHA. **Do not FF-push 6f165d30 to fix/v0.4.26-edit-preview-error on the basis of this review.**

Candidate after: `6f165d30df4f3c4b7f08623feff2eee58249e688`; parent `25b465723b3f0bd2ac8439d77ecebcb21b2f7628`, unchanged from before. Final `git diff --exit-code HEAD` and `git diff --cached --exit-code HEAD` both exit 0; `git status --short` is empty. PR1077 remains OPEN at remote `2f9afafdc443ba4b15569fca838511877c1fd3ba`. Post-test source-integrity comparison against the exact Git archive contents: baseline 2551/2551 and candidate 2554/2554 identical (`cmf21-data/source-integrity.json`). Host clock reports NTP synchronized; host and clock-tool UTC agree.

## repo-rule compliance

ROLLE remained REVIEW. Reviewer: Codex, current API session; exact reviewer model/backend/session ID is not independently exposed by available tools, so no fabricated identifier or Horde task is recorded. Review is separately performed from the implementing worker. No production edits, implementation corrections, commits, push, merge, approvals, PR comments, CI dispatch, release, tag, deployment, controller activation, agent spawn, lane deletion, task takeover or Planning publishing. All fixtures, source exports, logs and recipes are under the prescribed evidence tree; Go build cache is disposable under /tmp. Existing claims, sessions and coordinator plan were preserved. Initial watcher notifications were inspected and acknowledged as operational data; unrelated pool/stale-plan/session flags remain for their coordinator and do not authorize work outside CMF21. Native TodoWrite was unavailable throughout; the evidence PLAN is complete, and no native tool action is claimed.

END CMF21-REVIEW-DONE
