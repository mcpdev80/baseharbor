# CMF19 independent candidate review

ROLLE: REVIEW

Candidate before: `cee2ef5b084983c47e85b041562fd476f395555c`; base `2f9afafd`.
Reviewer: Codex, current API session; model/backend/session identifier not exposed by the session tools.
Scope: user-authorized mission `ops/missions/cmf19.txt`; no product edits, push, merge, approve or CI dispatch.

## PLAN

- [x] 0. Read binding policies and mission; establish SHA, scope and review plan. Native TodoWrite is unavailable (tool discovery returned no Todo tool); this checklist records the plan without claiming a native TodoWrite action. The mission itself contains no numbered PLAN step 0; this step fulfills the user's additional instruction.
- [x] 1. Read complete diff, repository rules and o141/o142 evidence; inspect observation, ownership, error propagation and q124 interaction.
- [x] 2. Execute required build, vet, format and focused tests, retaining outputs outside the candidate.
- [x] 3. Assess fail-closed behavior, canonical paths and regression coverage; document actionable recipes for findings.
- [x] 4. Verify unchanged candidate SHA and tracked content; record UTC verdict and completion marker.

## Initial state and scope decisions

Worktree has an existing untracked `baha` binary; preserve it. Tracked worktree initially clean.
Basics flags: watcher events, reminders, idle o142, pool underfill and stale coordinator plan. This bounded review does not authorize pool/controller work or removal/re-tasking of existing sessions. Preserve all sessions and the coordinator's plan; record unrelated flags rather than expanding the mission. Existing Maintainer/production pause stays in force. Repository Maintainer delegation language differs from the bootstrap's strict independent review gates; this review follows the bootstrap and mission, and performs no Maintainer actions.

## Verdict

**REQUEST_CHANGES** — UTC `2026-10-25T17:16:51Z`.

The required focused suite is green, but the candidate fails the required ownership regression and converts genuine runtime failures to verified success facts. The o141 acceptance recipe is not fulfilled. No implementation corrections were made by this reviewer.

## Executed checks and evidence

| Check | Actual result | Evidence |
| --- | --- | --- |
| `git diff 2f9afafd cee2ef5b --stat` and complete three-file diff read | 418 additions, 1 deletion; observation, tests and exported instance-ID helper only | candidate Git objects |
| `CGO_ENABLED=0 go build ./...` | exit 0 | `cmf19-build.log` (empty output) |
| `go vet ./cmd/baha/` | exit 0 | `cmf19-vet.log` (empty output) |
| `gofmt -l cmd/baha internal/application` | exit 0, no files listed | `cmf19-gofmt.log` |
| `go test ./cmd/baha/ -run 'TestWizard\|TestO141\|TestZeroContainerListing\|TestProviderContinuation' -count=1 -v` | exit 0 | `cmf19-tests.log` |
| Additional bounded review regressions, candidate source | exit 1; four expected-invariant failures | `cmf19-review-tests.log` |
| Same three fail-closed regressions with only observation file overlaid from parent `2f9afafd` | exit 0; all three pass | `cmf19-base-review-tests.log` |

All logs are adjacent to this verdict. Review fixtures and overlay files are under `scripts/cmf19*`. Reproduce the candidate failures from the candidate worktree with:

```sh
go test -overlay /home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-25/scripts/cmf19-overlay.json ./cmd/baha/ -run '^TestCMF19' -count=1 -v
```

The overlay replaces only the existing test file with its original content plus review tests. It does not edit the worktree. Baseline overlay additionally replaces the observation file with its parent source; this is a controlled function-level comparison, not a claim that the entire parent checkout was tested. Final negative fixtures use an explicitly unsupported runtime to rule out an environmental runtime successfully verifying absence. Absolute temporary Store roots exercise the ordinary local observation/ownership path, not a sandbox-relative path accident. No Docker/Podman gate or full suite was dispatched. Full-suite success asserted in o142 evidence is not independently established here; its own listed selected-suite failures also cannot establish o141's full-suite stop criterion.

## Findings and exact worker recipes

### F1 — P1: genuine runtime failures become measured stopped-owned facts

File: `cmd/baha/application_native_backend_observation.go:105-119,245-254`.

The new fallback is used on **any** `detectRuntimeForTarget` or `InspectProjectResources` error, even after the canonical inventory has been verified present. The helper does not check absence. A registry entry cannot prove a stopped runtime or a measured container count of zero when runtime detection/inspection failed. This bypasses q124's actual observation and fresh-list confirmation path. `TestCMF19RuntimeDetectionFailureMustRemainUnverifiable` constructs protected canonical inventory and a real binding, proves inventory succeeds and runtime detection fails, yet receives `stopped-but-owned`, `Containers=0`, `Classification=measured`. Parent observation passes this negative regression.

- Change: remove the registry-success branches from runtime detection and inspection errors. Preserve their fail-closed return/structured error behavior. Restrict the exceptional missing-inventory path to F3's validated condition. Never substitute inferred zero for a failed measurement.
- Tests: add deterministic detector-failure and inspector-failure cases with valid inventory and binding; assert no verified stopped fact and no successful ownership/retirement authorization. Keep `TestZeroContainerListingIsConfirmedByFreshListing` and provider continuation tests green.
- Stop criterion: genuine API/detection/ownership failures stay unverifiable, and all successful runtime observations retain q124's fresh-list confirmation semantics.

### F2 — P1: missing binding does not exclude the instance; retained ownership is admitted

File: `cmd/baha/application_native_backend_observation.go:202-215,222-231`; affected consumer `cmd/baha/app_core_requirement.go:298-316`.

`bindingSQLNameForInstance` affects only a source string. An empty binding result still returns success. `applicationNativeBackendInstance` proves scope/owner fields, not a binding. The candidate's own positive fixture registers an instance without creating a binding, so its green test does not prove the advertised bound-instance condition. In `TestCMF19UnboundInstanceMustRemainUnverifiable`, the registry explicitly has zero bindings and the runtime cannot be detected; the candidate manufactures a measured stopped fact, and `checkRetainedApplicationProviderOwnership` returns nil. The parent fails closed in the same controlled case. This is the mission's required ownership regression failing, despite that function's source remaining unchanged.

- Change: require an exact registry binding for the selected instance, application UUID, environment, capability and logical resource before applying the special missing-inventory fallback. Handle PostgreSQL/Valkey through their actual capability bindings; missing/foreign bindings must not reach `recordRegistryBoundStoppedOwned`. Keep normal verified observation of retained unbound instances available when the runtime genuinely proves ownership.
- Tests: seed a real binding in the positive fixture. Independently test missing intent, missing instance, missing binding, foreign application/environment/resource bindings, and retained-unbound ownership with unavailable runtime. Call `checkRetainedApplicationProviderOwnership` and assert `retained_provider_ownership_unverified`; do not replace this regression with a structural claim about external ownership. The current empty-registry negative test is vacuous for these cases.
- Stop criterion: unbound/unverifiable retained instances cannot pass ownership merely because a registry owner tuple matches; the explicitly bound positive case remains supported.

### F3 — P1: ENOENT is not restricted to the canonical runtime directory

File: `cmd/baha/application_native_backend_observation.go:79-103,206`; related test `cmd/baha/o141_wizard_apply_factgap_test.go:76-88`.

`nativeBackendInventory` can return ENOENT for `files.Dir`, `compose.yaml`, `runtime.env`, or an earlier inventory operation. The new branch treats all of them as missing-directory proof. `TestCMF19MaterializedDirectoryMissingEnvMustNotProveAbsence` creates the actual protected directory and canonical Compose file, leaves only `runtime.env` missing, binds the instance, and makes the runtime unavailable: the candidate still records stopped/measured zero. The parent passes this fail-closed check. Moreover, an empty wizard Store root still flows through `RuntimeFilesFor` and `os.Lstat` relative to CWD before fallback; the o141 path cause has not been resolved. Creating an unrelated directory in CWD can alter this branch. `RuntimeFilesFor` itself uses `<Store.Root>/<app>/runtime`; the new comments' literal `state/<app>/runtime` is not guaranteed when root is empty. The inherited test's final `!os.IsNotExist(err)` condition actually requires ENOENT while its message claims otherwise.

- Change: resolve the trusted deployment/store identity before any owner filesystem lookup, or use an explicitly separate preview branch with no CWD filesystem lookup when canonical identity is unavailable. Gate the exception on a missing trusted canonical directory specifically (including error path/type), not a generic downstream ENOENT; other missing/unprotected/noncanonical files remain unverifiable. Correct the inverted inherited assertion and its setup to distinguish directory absence from missing owner files.
- Tests: add separate missing-directory, missing-Compose, missing-env, unreadable/symlink/noncanonical-file cases; repeat wizard observation from two CWDs, one containing unrelated matching relative paths, and assert identical behavior. Compare wizard and apply observations for the same canonical owner.
- Stop criterion: only the authorized canonical-directory materialization gap qualifies; CWD contents and unrelated missing files cannot synthesize success facts.

### F4 — P2: early return drops all but the first eligible instance

File: `cmd/baha/application_native_backend_observation.go:98-101,202-215`.

The helper returns one `inventoryMiss`, and its caller writes one fact and returns before the existing per-instance loop. `TestCMF19TwoInstancesMustBothReceiveFacts` supplies two named SQL instances with actual bindings and derivable resources; only `analytics` gets a fact. The other instance remains absent/unverifiable, so the original preview/revalidation drift persists for multi-instance applications. This is a new-branch completeness defect; no claim is made that the parent supplies missing-directory fallback facts.

- Change: evaluate eligibility and record outcomes per instance, collecting every independently proven instance. Preserve fail-closed outcomes for peers that lack proof; do not return after the first match or reuse one instance's proof for another.
- Tests: two bound named PostgreSQL instances, PostgreSQL plus Valkey, and a mixed eligible/ineligible set; assert complete per-instance results and stable preview/revalidation states.
- Stop criterion: all eligible instances receive the correct fact independently, with no facts for unproven peers.

### F5 — P2: the promised typed machine error is discarded as plain stderr text

File: `cmd/baha/application_native_backend_observation.go:234-264`.

`runtimeInventoryNotYetMaterialized` returns a plain `fmt.Errorf`; it has no stable machine code/type and does not even contain the literal `runtime-inventory-not-yet-materialized` identifier. `auditProviderInventoryMiss` only prints it to process-global stderr. The observer returns no error/status to its caller, which consumes the manufactured success fact. Thus neither o141's typed diagnostic contract nor CMF19's requirement that this condition not be misinterpreted as success is fulfilled.

- Change: represent the condition through the existing typed machine error/diagnostic contract with stable `runtime-inventory-not-yet-materialized` cause/code and instance/target context. Propagate it through the observation caller's structured result/error path and existing output writers; let callers explicitly handle the narrowly qualified preview condition rather than silently consuming it as successful runtime verification. Keep genuine failures distinct and fail closed.
- Tests: assert machine type and stable cause/code with `errors.As`/machine output decoding, caller propagation, JSON/CLI handling, and no successful verification/authorization from the diagnostic alone. A substring assertion on stderr is insufficient.
- Stop criterion: callers and machine clients can reliably distinguish the materialization diagnostic from verified runtime success and from genuine runtime failure.

## q124 consistency and repo-rule compliance

No `footprint.go`, consent digest, consent validation or wildcard changes appear in the complete diff. The existing `expected=nil` fallback remains textually, and q124's confirmation helper is unchanged; however, the new early returns bypass it, so green direct helper tests do not establish equivalent observer behavior. The parent `files.Project` guard was also removed. No blanket acceptance of these branch changes is justified.

ROLLE remained REVIEW throughout. No product edits, Git commits, push, PR comment/approve/merge, CI dispatch, release, tag, deployment, controller or worker spawn. All generated review material is in the prescribed evidence tree. Existing untracked `baha` was preserved. Watcher events/reminders were inspected as operational data and acknowledged; their merge/spawn suggestions were not treated as authorization. Pool/stale-plan/idle-session flags remain coordinator handoff matters under the bounded mission and pause, not completed actions.

Candidate after: `cee2ef5b084983c47e85b041562fd476f395555c`, identical to before. `git diff --exit-code HEAD` and `git diff --cached --exit-code HEAD` both clean; status remains only the pre-existing `?? baha`. Host UTC and clock-tool UTC agreed at `2026-10-25T17:16:02Z`.

END CMF19-REVIEW-DONE
