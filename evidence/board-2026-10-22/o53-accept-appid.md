# Evidence: o53 — Accept appid manifest regression tests on develop C2-final

**Task ID**: t-20261020-develop-appid-manifest-test-regression
**Card status (per prompt)**: review
**Verifier (o53)**: local independent review against develop HEAD `1861ed07` (C2-final)
**Date**: 2026-10-23T15:01Z (UTC)
**Base commit verified**: `1861ed07888a1ec22c218ca491d23ee54f48e261`
**Scope**: acceptance-point verification only. No PR required unless a test gap is found.

---

## PLAN + todowrite ([wip:o53])

### Step 0: Baseline
- Prompt acceptance target = remote develop HEAD `1861ed07` (C2-final), NOT local `7fbc1ec5`.
- Checked out detached worktree at exactly `1861ed07`.
- `go build ./...` → exit 0.

### Step 1: Locate appid manifest regression tests
- `internal/application/manifest_test.go` present in C2 tree, 452 lines.
- Regression functions: `TestDeveloperManifestWithoutAppIDParses` (:126), `TestDeveloperManifestWithAppIDIsRejected` (:144), `TestApplicationIDRejectsNonV4Identity` (:446), plus `TestApplicationIDRoundTripAndValidation`, `TestManifestValidationFailsClosed`.
- PR #1008 diff scope (per q27): exactly `cmd/baha/app_workload_integration_test.go` (authored-ID removals) + `internal/application/manifest_test.go` (+33 regression lines). No parser file changed. Confirmed the regression tests target authored `app.id` rejection (fail-closed).

### Step 2: Run regression tests locally (independent of retired CI)
- See acceptance proofs below.

### Step 3: Receipt reconciliation (q27)
- q27 receipt run IDs `39627285799` and `39619782116` now return HTTP 404 on current Actions API → retired CI, unreproducible from outside.
- Internal local regression + mandatory-native-eight scope verified green in C2 tree (see Step 2).
- Docker-native8 acceptance test (`TestDualDockerEngineRootlessCoreAcceptance`) is opt-in and `SKIP`s without a Docker daemon → not a failure, not runnable in this sandbox.

### Step 4: Verdict

---

## Acceptance points — per-point proof

Target: `1861ed07` (C2-final), detached worktree, independent of the retired CI runs cited in q27.

### Point 1 — appid manifest regression tests run and are GREEN in the develop tree

Command (C2 worktree, no daemon present):
```
go test ./internal/application/ -run 'TestDeveloperManifest|TestApplicationID|TestManifestValidationFailsClosed' -v
```

Result:
```
--- PASS: TestDeveloperManifestWithoutAppIDParses (0.00s)
--- PASS: TestDeveloperManifestWithAppIDIsRejected (0.00s)
--- PASS: TestManifestValidationFailsClosed (0.00s)
--- PASS: TestApplicationIDRoundTripAndValidation (0.00s)
--- PASS: TestApplicationIDRejectsNonV4Identity (0.00s)
ok  	github.com/mcpdev80/baseharbor/internal/application
```
Exit 0. Authored `app.id` in a developer manifest is rejected fail-closed; bare-v4 identity is accepted; manifest round-trips preserve `ApplicationID`.

### Point 2 — Contract unchanged, authored IDs rejected fail-closed

`manifest_test.go:144` `TestDeveloperManifestWithAppIDIsRejected`:
```go
if _, err := ParseYAML(man); err == nil {
    t.Fatalf("authored app.id in a developer manifest must be rejected")
}
```
PASS. Parser contract intact for the appid regression surface.

### Point 3 — Native "mandatory eight" scope green in C2 tree

Command:
```
go test ./cmd/baha/ -run 'TestGuidedRepositoryRuntimeFixtureRegressionSuite/mandatory-native-eight-test-scope'
```
Result:
```
--- PASS: .../mandatory-native-eight-test-scope (0.00s)
ok  	github.com/mcpdev80/baseharbor/cmd/baha
```
Exit 0. (The broader `TestDualDockerEngineRootlessCoreAcceptance` is opt-in and `SKIP`s without a Docker daemon — expected, not a failure.)

### Point 4 — Independent local review + evidence links

- Independent reproduction on a detached C2 worktree (not the CI machine).
- `go build ./...` → exit 0 at `1861ed07`.
- PR #1008 squash `8a40e60a` confirmed ancestor of both local develop `7fbc1ec5` and target `1861ed07`.

---

## Defect found (NON-BLOCKING, per prompt §"nur wenn Testlucke")

`internal/application/external_provider_registry_test.go` (provider-registry ownership) has pre-existing failures on develop `7fbc1ec5`:
```
--- FAIL: TestExternalProviderRegistrationAndRemoval (0.00s)
    external_provider_registry_test.go:42: provider change: provider-registry-ownership-unverifiable
```
Reproduced identically on local develop `7fbc1ec5` (before/without my involvement) → **pre-existing, out of appid scope**. Not a test gap for the appid acceptance point. Left unmodified (read-only review).

---

## Verdict

| Item | Value |
| --- | --- |
| Card | t-20261020-develop-appid-manifest-test-regression |
| Base verified | `1861ed07` (C2-final), detached worktree |
| Verdict | **READY** — acceptance points proven for the appid manifest regression surface |
| Acceptance gap | None in scope. Retired CI run IDs (39627285799/39619782116) are 404/unreproducible; in-tree regression verified green independently. Docker-native8 acceptance test is opt-in/SKIP without daemon. |
| Pre-existing out-of-scope defect | `external_provider_registry_test.go` ownership failures on `7fbc1ec5` (unrelated to appid). |

No PR required for the appid acceptance point. The pre-existing provider-registry ownership defect is a separate track and was NOT widened in this read-only review.

---

END O53-ACCEPT-APPID-DONE.
