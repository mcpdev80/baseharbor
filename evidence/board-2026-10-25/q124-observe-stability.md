# Q124 Evidence: Observe-Stability Patch Push to PR #1077

## PLAN
1. Audit git status + diff in worktrees/m1077
2. Run gofmt, go vet, CGO_ENABLED=0 go build, targeted tests
3. git fetch origin + branch setup (checkout -B fix/v0.4.26-edit-preview-error)
4. Create single ordinary commit
5. git push origin HEAD:refs/heads/fix/v0.4.26-edit-preview-error
6. gh pr view 1077 + observe CI
7. Capture evidence

## Evidence

### Step 0: pwd-Pflicht
pwd -P: /home/marcel/dev/baseharbor-ecosystem (NOT root - OK)

### Step 1: git status --porcelain
```
M cmd/baha/application_broker_observation.go
 M cmd/baha/application_identity_observation.go
 M cmd/baha/application_native_backend_observation.go
 M cmd/baha/application_provider_container_observation.go
 M cmd/baha/application_provider_container_observation_test.go
 M cmd/baha/application_shared_backend_observation.go
 M cmd/baha/application_standalone_observation.go
```
**Result: 7 files changed (6 observation + 1 test) - exactly as expected. No debug artifacts (zz_debug*, debugDumpPlan, debugLogConsent, dbg(, BAHA_DEBUG_PLAN_DIR) found.**

### Step 2: gofmt / go vet / build / tests

**gofmt -l (empty = pass):**
```
(no output - all files formatted)
```

**go vet ./cmd/baha/ ./internal/application/ (empty = pass):**
```
(no output - all checks passed)
```

**CGO_ENABLED=0 go build ./... (empty = pass):**
```
(no output - build successful)
```

**go test ./cmd/baha/ -run 'TestZeroContainerListing|TestProviderContinuation' -count=1:**
```
ok 	github.com/mcpdev80/baseharbor/cmd/baha	0.013s
```

**go test ./internal/application/ -run 'Consent|Digest|Provider' -count=1:**
```
ok 	github.com/mcpdev80/baseharbor/internal/application	4.445s
```

**All local checks PASSED.**

### Step 3: git fetch origin + branch setup
```
git fetch origin fix/v0.4.26-edit-preview-error
-> 87736a36 (matches expected Remote-HEAD)

git checkout -B fix/v0.4.26-edit-preview-error 87736a364735ffc303d1c5f940d1046767ff003c
-> Switched to a new branch 'fix/v0.4.26-edit-preview-error'
```
**Branch setup successful. Working tree preserved.**

### Step 4: Single ordinary commit
```
Commit SHA: 2f9afafdc443ba4b15569fca838511877c1fd3ba
Message: fix(obs): stabilize provider fact observation against racy listings and unmaterialized inventory dirs

Refs: stale-impact-consent #1077 Journey new-edit-new-sql-add
```
**Signed-off trailer: NOT invented (as per mission directive)**

### Step 5: git push
```
git push origin HEAD:refs/heads/fix/v0.4.26-edit-preview-error
-> To github.com:mcpdev80/baseharbor.git
   87736a36..2f9afafd  HEAD -> fix/v0.4.26-edit-preview-error
```
**Push successful (ordinary fast-forward, not non-fast-forward).**

### Step 6: gh pr view 1077
```
PR #1077 state: OPEN
headRefOid: 2f9afafdc443ba4b15569fca838511877c1fd3ba
mergeStateStatus: BLOCKED (expected - CI still running)
reviewDecision: REVIEW_REQUIRED

CI Status Check Rollup:
- Workflow policy: IN_PROGRESS (run/40206411432/job/121208192776)
- Release Source Preflight: SKIPPED (run/40206411432/job/121208193728)
- Targeted HA · docker: SKIPPED (run/40206411432/job/121208217512)
- Targeted HA · podman: SKIPPED (run/40206411432/job/121208193709)
- validate: SKIPPED (run/40206411432/job/121208193669)
- PR · Trusted Plan: IN_PROGRESS (run/40206411432/job/121208192812)
```

### Step 7: Diffstat
```
 7 files changed, 91 insertions(+), 7 deletions(-)

cmd/baha/application_broker_observation.go         |  2 +-
cmd/baha/application_identity_observation.go       |  4 +--
cmd/baha/application_native_backend_observation.go | 36 ++++++++++++++++++++--
cmd/baha/application_provider_container_observation.go | 18 +++++++++++
cmd/baha/application_provider_container_observation_test.go | 34 ++++++++++++++++++++
cmd/baha/application_shared_backend_observation.go |  2 +-
cmd/baha/application_standalone_observation.go |  2 +-
```

### Files Modified
1. **application_broker_observation.go** - 2 +- (minor fix)
2. **application_identity_observation.go** - 4 +-- (minor fix)
3. **application_native_backend_observation.go** - 36 ++++-- (major: ErrNotExist handling for inventory dirs)
4. **application_provider_container_observation.go** - 18 ++++-- (major: observeCanonicalProviderContainersConfirmed fresh listing)
5. **application_provider_container_observation_test.go** - 34 ++++-- (new test: TestZeroContainerListingIsConfirmedByFreshListing)
6. **application_shared_backend_observation.go** - 2 +- (minor fix)
7. **application_standalone_observation.go** - 2 +- (minor fix)

### CI-Run-IDs
- Workflow Run: 40206411432
- Workflow: CI · Source & Quality

## STOP-KRITERIEN CHECK
- Debug-Reste: UNAUFFINDBAR - PASS
- Testfailure: NONE - PASS
- Remote-Head-Merge Konflikt: NONE (ordinary fast-forward) - PASS
- Push non-fast-forward: NO (ordinary fast-forward 87736a36->2f9afafd) - PASS

## END Q124-OBSERVE-PUSHED
