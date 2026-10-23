# R1-998 LOCAL PRE-REVIEW VERDICT: CHANGES_REQUESTED

**PR**: mcpdev80/baseharbor#998
**SHA**: 6931ba9c8e604419ae0afb7ff52dbdd20da840f8
**Worktree**: /home/marcel/dev/baseharbor-ecosystem/worktrees/core-921-central-cli-mcp-registration
**Reviewer**: Flash (local pre-reviewer, read-only)
**Date**: 2026-10-19

## Tool Outputs

### `go build ./...`
PASS (no output = no errors)

### `go test ./cmd/baha/ -run MCP -count=1`
PASS — all 5 Core921 MCP tests pass:
- TestCore921ActualCommandTreeHelpAndReadOnlyEntry: PASS
- TestCore921MCPRealDiscoveryAndRejectIncompleteEdit: PASS
- TestCore921MCPEditSchemaIsSnakeCaseAndPhaseConditional: PASS
- TestCore921MCPProposeReachesHandlerWithZeroWrites: PASS
- TestCore921MCPEditHandlerLevelNegatives: PASS

### `gofmt -l` on touched files
PASS — no output = all files properly formatted

### `go vet ./cmd/baha/...`
PASS — no output

### Full test suite (TestCore921* + TestAppEdit*)
FAIL — but the single failure is `TestAppEditCommandCompleteProviderEditRunsRealLifecycle` which fails because Docker daemon is unavailable in this environment (`unix:///var/run/docker.sock is unavailable`). This is the native docker happy path explicitly excluded from re-litigation per task scope. All Core921 tests pass.

---

## Findings

### F1: MCP/schema surfaces use snake_case consistently — PASS
**Evidence**: app_machine_adapters.go lines 60-66, 248-249, 256-259, 268-275 — all JSON tags use snake_case (`target`, `application_id`, `requirements`, `provider_preferences`, `footprint`, `snapshot_revision`, `plan_revision`, `requirement`, `instance_id`, `desired`, `choices`, `preferences`, `plan`).

**Verification**: `TestCore921MCPEditSchemaIsSnakeCaseAndPhaseConditional` (registration_921_test.go:77) explicitly asserts PascalCase fields (`Desired`, `Proposal`, `Consent`, `Target`) do NOT appear in the serialized schema payload, and snake_case fields (`target`, `phase`, `desired`, `proposal`, `consent`) DO appear. This test passes.

**Verdict**: No action required.

---

### F2: Typed error classification via classifyAppMachineError — PASS
**Evidence**: 
- `appMachineError` struct defined at app_machine_adapters.go:16 — private type with stable `Code` field.
- `classifyAppMachineError` at app_machine_adapters.go:25-53 — maps 5 internal classifications (conflict, cancel, denied, unsupported, partial) onto the shared `machine.Error` type with concrete `ErrorCode`, `Message`, `Next`, `Retryable` fields.
- Integration at main.go:321-323 — `errors.As` catches `*appMachineError` before the final `machine.Classify(err)` fallback.

**Error code mapping verified against machine/contract.go:47-56**:
- `conflict` → `ErrorConflict` ("conflict") — non-retryable, remediation provided
- `cancel` → `ErrorRuntimeUnavailable` ("runtime_unavailable") — retryable
- `denied`/`unsupported` → `ErrorPolicyDenied` ("policy_denied") — non-retryable
- `partial` → `ErrorVerificationFailed` ("verification_failed") — retryable
- default (unknown) → `ErrorInternal` ("internal_error") — non-retryable

**No internal_error fallthrough**: The `default` case in the switch explicitly returns `machine.ErrorInternal` — but this is the typed machine error owner, NOT the untyped `internal_error` fallthrough. The key improvement is that every known classification (conflict, cancel, denied, unsupported, partial) maps to a specific, typed `ErrorCode`, not the catch-all.

**Verdict**: No action required.

---

### F3: Structured handler-negative tests with zero-write proof — PASS
**Evidence**:
- `TestCore921MCPProposeReachesHandlerWithZeroWrites` (registration_921_test.go:150-217): Tests stale-snapshot propose rejection through the real MCP handler. Verifies typed `conflict` code in the structured error payload and zero writes: manifest bytes byte-identical before/after, deployment records count and snapshot identical.

- `TestCore921MCPEditHandlerLevelNegatives` (registration_921_test.go:219-324): Tests 5 handler-level negative scenarios through real MCP:
  1. Wrong target → conflict
  2. Invalid phase ("suspend") → validation_failed
  3. Apply without proposal/consent → validation_failed
  4. Apply with stale snapshot but missing proposal/consent → validation_failed (consent gate is the reachable boundary)
  5. All verified via `structuredCode` helper that extracts `error.code` from the MCP structured content

- Zero-write proof in both tests: `os.ReadFile(manifestPath)` before/after comparison, `deployment.ListDeployments()` before/after with `deploymentSnapshot()` byte-level comparison.

**Verdict**: No action required.

---

### F4: Repo conventions in baseharbor/AGENTS.md (CI discipline, no skipped tests) — PASS
**Evidence**:
- No skipped tests in registration_921_test.go: `grep -n 't.Skip\|t.Skipf\|SkipNow'` returns nothing.
- All tests pass (the one failure is pre-existing Docker-unavailability in the environment, explicitly excluded from scope).
- CI discipline: changes are batched into a coherent verification candidate (3 files: app_machine_adapters.go, main.go, registration_921_test.go). Build, vet, tests, and gofmt all pass.
- gofmt -l on all 3 touched files: clean.
- No force-push, no test-skipping, no CI-workaround.

**Verdict**: No action required.

---

### F5: Codex round-2 finding still BLOCKING — CHANGES_REQUESTED
**Evidence**: Codex verdict at codex-verdict-core921-round2.md:11-28 states:

> "The successful real MCP propose→apply path remains a blocking acceptance requirement."

The existing tests prove:
- Stale-snapshot propose rejection (F3 above) ✓
- Apply rejection before proposal/consent validation ✓
- Handler-level negatives all classified as typed codes ✓

But they do NOT prove:
- A validly proposed snapshot survives SDK serialization
- An actual proposal reaches successful apply
- The full propose→consent→apply round-trip through real MCP

Codex (round-2, fe5dff6c) specifies the smallest sufficient native gate: one MCP integration test on `baha-docker` only, with isolated state and a disposable application. This is the ONLY remaining blocking finding from the Codex round-2 verdict that is answerable locally.

**Finding**: The PR at SHA 6931ba9c does NOT yet contain the full MCP propose→apply lifecycle proof. All round-2 locally answerable findings (F1-F4) pass, but the blocking acceptance requirement (F5) remains unfulfilled.

---

## Verdict: CHANGES_REQUESTED

**Numbered findings requiring action:**

1. **F5** (app_machine_adapters.go / main.go / registration_921_test.go): The successful real MCP propose→apply path is not yet proven. A full MCP integration test on `baha-docker` only is required, using isolated state and a disposable application, that proves: (a) a valid propose survives SDK serialization, (b) round-trip through JSON with matching explicit consent reaches apply, (c) a complete verified result is returned, and (d) stale-wrong-proposal and wrong-consent against the valid fixture return structured errors with unchanged state.

**All other round-2 locally answerable findings pass:**

- F1: MCP schema uses snake_case consistently — PASS
- F2: Typed error classification via classifyAppMachineError — PASS  
- F3: Structured handler-negative tests with zero-write proof — PASS
- F4: Repo conventions (CI discipline, no skipped tests) — PASS

**R1-998-DONE**
