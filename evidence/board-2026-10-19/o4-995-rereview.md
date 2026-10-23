# Independent Re-Review — PR mcpdev80/baseharbor#995 (task t-20261018-podman-recovery-bootstrap-cleanup)

- **Task:** t-20261018-podman-recovery-bootstrap-cleanup
- **Candidate head:** `d359af833649a16f6b2b121704d5c2a6613f8e36` (verified via `git rev-parse HEAD` in review dir)
- **Reviewer worker:** O4 (independent Ornith, did not implement this lane)
- **Review mode:** READ-ONLY — no commits, no pushes, no GitHub writes, no native/docker runtime
- **Coordinator status at close:** controlled pause (halt) — read-only-only remaining checks not run
- **STATUS: PARTIAL** (required Go checks + coverage classification done; two read-only source checks deferred by coordinator halt)

## sha256 of changed files (before = after, read-only confirmed)

- `02d552c7cf4071da72a45a1c508542ea532043be0e7ae2dd9815d79ac6fc0783`  cmd/baha/app_backup_restore_integration_test.go
- `eee8acce7fc3fd27ed705d4a238a50ca3fcd4c644e325a2c2f4503551564dbfd`  cmd/baha/app_backup_runtime_fixture_test.go
- `36efc53d3ac020717d67e49fb15a80bffacef3cdf247532c6ecb4c0d1c049c1a`  cmd/baha/native_fixture_cleanup_diagnostics.go
- `b188b7cdb65e17050f78ae3417cf68c543ed0a03513e6dd69c36525318369ab0`  cmd/baha/native_fixture_cleanup_test.go

## Changed path set vs develop (merge-base d54ac358 → head d359af83)

Exactly 4 files:
- M cmd/baha/app_backup_restore_integration_test.go
- A cmd/baha/app_backup_runtime_fixture_test.go
- A cmd/baha/native_fixture_cleanup_diagnostics.go
- M cmd/baha/native_fixture_cleanup_test.go

No production resolver, no `internal/`, no contract/schema change. Only cmd/baha test sources + one new diagnostics helper.

## (1) Parser untouched, no exceptions added

CONFIRMED: zero capability-parser files in the diff (`git diff --name-only | grep -iE 'pars|capability'` → empty). Parser contract confirmed in-tree:
- `internal/application/manifest_parse.go:279` — valid service keys: `sql, cache, key_value, document_database, messaging_queue, messaging_pubsub, messaging_stream, object_storage, secrets, identity, observability`.
- `manifest.go:72-79` — `Manifest.Services` exposes `SQL`, `Cache`, `Secrets` (bool). Legacy `postgres`/`redis` keys are NOT in the accepted set.
- No new `machine.*` error codes added (the four codes the diagnostics map — `ErrorOwnershipAmbiguous`, `ErrorPolicyDenied`, `ErrorAuthenticationFailed`, `ErrorValidationFailed` — all pre-exist in `internal/machine/contract.go:36-49`).

## (2) Fixture uses capability names consistently with internal/application contracts

CONFIRMED. The disaster-recovery fixture manifest (app_backup_restore_integration_test.go) now uses `sql:`/`cache:` (not `postgres`/`redis`) and asserts `application.ParseYAML(manifest)` succeeds through the shipped parser. New `TestBackupFixtureManifestMatchesCapabilityContract` (uses `sql`/`cache`/`secrets`) and `TestBackupFixtureManifestRejectsLegacyCapabilityKeys` (asserts `postgres`/`redis` rejected) lock this to the parser. Matches `Services.SQL`/`Services.Cache`/`Services.Secrets`.

## (3) Diagnostics bounded + redacted; ownership/consent guards byte-identical

- **Diagnostics bounded/redacted:** `native_fixture_cleanup_diagnostics.go` (90 lines) publishes exactly 3 fields — `Caller` (constant "native-fixture-cleanup"), `Owner` (coarse `Scope.Owner.Subject`/`.Owner`), `Classification` (coarse `machine.Error.Code` category). No hostname, path, revision, plan digest, deployment ID, or inventory material. The single `grep` hit for "Impact" is the type switch `case runtimeDestroyImpact:` (type name only), not an echoed field.
- **Ownership/consent guards byte-identical:** `git diff eaf86813 d359af833 -- cmd/baha/runtime_destroy_all.go cmd/baha/mcp_control_plane_tools.go` → no `+`/`-` content lines (byte-identical). These guard files are also absent from the HEAD diff entirely. Verified `ProviderScopeOwner{Owner,Reference,Subject,Revision,Destructive}` (internal/application/provider_change_scope.go:15) — diagnostics read only `.Subject`/`.Owner`.

## (4) Local non-native checks

- `go vet ./...` → exit 0.
- `go build -o /tmp/baha-o4 ./cmd/baha` → exit 0 (50MB binary produced).
- Non-native `go test ./cmd/baha` (BASEHARBOR_TEST_RUNTIME unset, no native runtime; excluded the slow native disaster-recovery integration test which requires docker/podman):
  - `TestBackupFixtureManifestMatchesCapabilityContract` PASS
  - `TestBackupFixtureManifestRejectsLegacyCapabilityKeys` PASS
  - `TestAppBackupGuidedRuntimeFixtureRejectsInvalidRuntime` PASS
  - `TestAppBackupOwnedCoreCleanupContextIsBounded` (+ subtest) PASS
  - batch of `TestBackupFixture|TestAppBackupGuided|TestAppBackupOwnedCore|TestNativeFixture` → `ok` exit 0.
- No BASEHARBOR_APP_BACKUP_ACCEPTANCE flags used.
- Full `go test ./cmd/baha -count=1` NOT completed (native integration tests run long / require runtime; exceeded 120s bash timeout). **OPEN.**

## (5) pr-gate-plan.py (develop) coverage classification

Run `scripts/pr-gate-plan.py` on develop against the 4-file diff (target_base=d54ac358, head=d359af83). Result: **`coverage_status: "unresolved"`**.
- All 4 files → area `cli-unresolved-runtime-impact` (cmd/baha/*.go runtime-impact lane; no cmd/baha path-wide exemption).
- `selected_gates: []` (no native gate selected).
- `required_validation_checks` still enforced: `cli-build`, `cli-guided-tests`, `cli-vet`, `source-contracts`.
- `broad_required: true`.

**Which:** this path/set lands **coverage-unresolved**, NOT bound to a reviewed allowlist (no cmd/baha test file appears in any reviewed bundle). The plan remains valid and still requires the four mandatory checks to pass.

## File-size sanity

All under the 800-line non-test limit: diagnostics 90, fixture_test 228, integration_test 336, cleanup_test 68.

## Deferred by coordinator halt (read-only-only, not yet run)

- (1) Confirm diagnostics byte-identical claim against a broader guard set if any other ownership/consent source is referenced by the new diagnostics (all references confirmed above; only ProviderScopeOwner accessed).
- (2) Re-verify the two deferred read-only grep/source checks if coordinator resumes.

## Final line

HALTO4-SAFE
