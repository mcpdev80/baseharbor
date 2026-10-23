# O3-Lane Handoff — core-754 integration onto PR #985

**Coordinator session:** O3-lane (Ornith implementation worker)
**Task:** core-754 — verify typed CLI/MCP precondition classifications
**PR:** #985 `mcpdev80/baseharbor` (draft, OPEN)
**Branch:** `fix/core-754-mcp-precondition-classification`
**New head SHA:** `e1c797bb5f45dfac598b5795556c733962b6d621`
**Date:** 2026-10-19

## Task

Integrate `origin/develop` into `fix/core-754-mcp-precondition-classification`,
preserve the core-754 read-precondition test intent, execute the exact MCP
precondition test families, and push so PR #985 updates (kept as draft).

## Branch analysis

- Local branch tip was `5a381338` (test-only commit on base `d54ac358`).
- `origin/develop` head: `f2bad5ef` (feat app: safely apply confirmed edits).
- `merge-base HEAD origin/develop` = `d54ac358` (my own base), develop branched
  there with 8 forward commits → **true divergence, not fast-forwardable**.
- `git merge --no-ff origin/develop` applied automatically; **no file conflicts**.
- Divergence summary:
  - Branch-only: `5a381338` (core-754 read-precondition test).
  - Develop-only (7 new): `f2bad5ef`, `9f622e6e`, `8e77b704`, `58efa1a3`,
    `85dfcfa3`, `29cb0920`, `980e8db8`.

## Merge

- Performed with `git merge --no-commit --no-ff origin/develop`; merge staged,
  test file `cmd/baha/mcp_preconditions_test.go` untouched, no conflict markers.
- Committed as `e1c797bb` (31 files changed, +6147/-38) — staged explicitly,
  **not** `git add -A`.
- Pushed: `5a381338..e1c797bb` → `origin/fix/core-754-mcp-precondition-classification`.
- PR #985 status after push: `state: OPEN`, `isDraft: true`,
  `mergeStateStatus: BEHIND`, updated 2026-10-19T00:15:58Z. Left as draft; not merged;
  no GitHub comments posted.

## Toolchain

- Go not installed in environment; installed via
  `sudo apt-get install -y golang-go` → **go1.26.8 linux/amd64**.
- go.mod: `go 1.25.0` / `toolchain go1.26.9`. Local toolchain 1.26.8 satisfies
  the directive; runs used `GOTOOLCHAIN=local`.

## Fix applied

- `TestAppEditCommandChecksGoSourceFormat` (a develop test) failed because the
  core-754 test file had a `gofmt` alignment defect (`err error` → `err  error`).
- Applied `gofmt -w cmd/baha/mcp_preconditions_test.go`; only the struct-field
  alignment changed, test intent preserved. Re-verified: gofmt clean,
  `TestAppEditCommandChecksGoSourceFormat` passes.

## Command outputs

### go build -o /tmp/baha-o3 ./cmd/baha

```
go: downloading charm.land/lipgloss/v2 v2.0.6
go: downloading charm.land/bubbletea/v2 v2.0.9
... (module downloads) ...
=== BUILD EXIT: 0 ===
```

Build OK.

### go vet ./...

```
=== VET EXIT: 0 ===
```

Vet clean.

### go test -run TestExpectedPreconditionsShareTypedCLIMCPErrors

```
=== RUN   TestExpectedPreconditionsShareTypedCLIMCPErrors
=== RUN   TestExpectedPreconditionsShareTypedCLIMCPErrors/workspace_model
=== RUN   TestExpectedPreconditionsShareTypedCLIMCPErrors/workspace_source
=== RUN   TestExpectedPreconditionsShareTypedCLIMCPErrors/repository_manifest
=== RUN   TestExpectedPreconditionsShareTypedCLIMCPErrors/organization_not_configured
=== RUN   TestExpectedPreconditionsShareTypedCLIMCPErrors/unsupported_organization_source
=== RUN   TestExpectedPreconditionsShareTypedCLIMCPErrors/registered_deployment_absent
=== RUN   TestExpectedPreconditionsShareTypedCLIMCPErrs/deadline
=== RUN   TestExpectedPreconditionsShareTypedCLIMCPErrs/cancellation
=== RUN   TestExpectedPreconditionsShareTypedCLIMCPErrs/unknown_failure
=== RUN   TestExpectedPreconditionsShareTypedCLIMCPErrs/unknown_matching_words
--- PASS: TestExpectedPreconditionsShareTypedCLIMCPErrors (0.00s)
    --- PASS: .../workspace_model (0.00s)
    --- PASS: .../workspace_source (0.00s)
    --- PASS: .../repository_manifest (0.00s)
    --- PASS: .../organization_not_configured (0.00s)
    --- PASS: .../unsupported_organization_source (0.00s)
    --- PASS: .../registered_deployment_absent (0.00s)
    --- PASS: .../deadline (0.00s)
    --- PASS: .../cancellation (0.00s)
    --- PASS: .../unknown_failure (0.00s)
    --- PASS: .../unknown_matching_words (0.00s)
PASS
ok  	github.com/mcpdev80/baseharbor/cmd/baha	0.006s
=== EXIT: 0 ===
```

**PASS.** Both exact preconditions (CLI `machine.Classify` and MCP failure
payload) match the expected `machine.ErrorCode` for all 9 sub-cases.

### go test -run TestMCPReadEntryExpectedPreconditions

```
=== RUN   TestMCPReadEntryExpectedPreconditions
=== RUN   TestMCPReadEntryExpectedPreconditions/organization.inspect
=== RUN   TestMCPReadEntryExpectedPreconditions/status
=== RUN   TestMCPReadEntryExpectedPreconditions/policy.explain
=== RUN   TestMCPReadEntryExpectedPreconditions/workspace.status
--- PASS: TestMCPReadEntryExpectedPreconditions (0.02s)
    --- PASS: TestMCPReadEntryExpectedPreconditions/organization.inspect (0.00s)
    --- PASS: TestMCPReadEntryExpectedPreconditions/status (0.00s)
    --- PASS: TestMCPReadEntryExpectedPreconditions/policy.explain (0.00s)
    --- PASS: TestMCPReadEntryExpectedPreconditions/workspace.status (0.00s)
PASS
ok  	github.com/mcpdev80/baseharbor/cmd/baha	0.041s
=== EXIT: 0 ===
```

**PASS.** All 4 MCP read-entry preconditions return the expected typed error
codes: `organization.inspect`→`ErrorValidationFailed`, `status`→`ErrorNotFound`,
`policy.explain`→`ErrorNotFound`, `workspace.status`→`ErrorSourceMissing`.

### go test ./cmd/baha (full suite) — residual failures

```
--- FAIL: TestAppEditCommandCompleteProviderEditRunsRealLifecycle (0.41s)
    app_edit_apply_test.go:740: application preflight failed: selected Docker
       endpoint unix:///var/run/docker.sock is unavailable; ... no fallback to System Docker
--- FAIL: TestExternalProviderRemovalTransportsShareScopeAndExplicitStages (cli/http/mcp)
    provider_external_removal_test.go:53: provider change: provider-registry-ownership-unverifiable
--- FAIL: TestExternalProviderRemovalChecksEveryManagedConsumerBeforeMutation (0.00s)
    provider_external_removal_test.go:122: provider change: provider-registry-ownership-unverifiable
--- FAIL: TestExternalProviderRemovalCannotInferGlobalOwnershipFromTenantRole (0.00s)
    provider_external_removal_test.go:151: provider change: provider-registry-ownership-unverifiable
FAIL
FAIL	github.com/mcpdev80/baseharbor/cmd/baha	139.164s
=== EXIT: 1 ===
```

These 4 failures are **not introduced by this integration**:

1. `TestAppEditCommandCompleteProviderEditRunsRealLifecycle` — Docker daemon
   unavailable in environment. Verified fails identically on clean `origin/develop`
   (`docker info` → UNAVAILABLE). Environment-gated; requires isolated verified
   runtime per AGENTS.md, not a product defect.
2. `TestExternalProviderRemoval*` ×3 — `provider-registry-ownership-unverifiable`.
   Originates from develop commit `8e77b704` (fix inspect: derive import evidence
   from source syntax). Verified fails identically on clean `origin/develop`.
   Environment/config-gated ownership check, not mine.
3. `TestAppEditCommandChecksGoSourceFormat` — was failing pre-merge on my file's
   gofmt defect; **fixed** above, now passes.

## Evidence

- New head: `e1c797bb5f45dfac598b5795556c733962b6d621`
- PR #985 draft, OPEN, updated.
- Both exact MCP precondition families: PASS.
- `go build -o /tmp/baha-o3 ./cmd/baha`: OK. `go vet ./...`: clean.
