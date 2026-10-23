# R3 delta review — PR mcpdev80/baseharbor#998, native MCP propose→apply gate

**SHA:** `a66e5de664b22c295a8b5ae918a9c2dfe4c6e327` (parent `6931ba9c`) — verified detached HEAD
**File under review:** `cmd/baha/app_mcp_provider_edit_native_test.go` (371 lines, new)
**Reviewer:** Flash (read-only delta review)
**Scope:** only the delta `6931ba9c..a66e5de6` — one new file. No product edits performed.

## Reference basis

- Codex 6.1 round 2 (session fe5dff6c, 2026-10-19): the successful real MCP propose→apply path was the single blocking acceptance requirement. Smallest sufficient gate = one focused MCP integration test on `baha-docker` only, isolated state, disposable application, driving the real `baseharbor.apply` entry point (verdict: 4 numbered steps).
- `o10-native-mcp-proof.md`: worker evidence — `TestAppEditMCPApplyRunsRealProviderEditLifecycle` passed on `baha-docker` (~202 s), compiled clean, `go vet` clean, native PASS.

## (1) Coverage of Codex's 4 requirements

The test implements exactly the four required assertions against the **real** `baseharbor.apply` MCP tool (not the CLI):

1. **Valid propose survives serialization + apply round-trip with consent reaches verified complete result.**
   - Baseline resolved from production read authorities (`baseharbor.plan`, line 157) → current snapshot.
   - Two-stage plan required (`impact.ConfirmationsRequired < 2` → fail, lines 183-185) with matching explicit consent `First:true/Second:true` (lines 186-191).
   - Real MCP apply with consent (lines 231-237); JSON decode of structured content requiring `Status == "complete"` or `Verified && (running|complete|applied)` (lines 242-256). Exact SQL binding preserved (lines 258-265). ✅
2. **Stale-wrong-proposal → structured error + unchanged state.** Apply with a non-matching `PlanID` (lines 196-200), asserted `IsError`, classified `ErrorConflict` (lines 204-207), `bindingUnaffected` true (lines 208-210). ✅
3. **Missing-second-stage → structured error + unchanged state.** Matching `PlanID`, only `First:true` (lines 214-217), asserted `IsError`, classified `ErrorApprovalRequired` (lines 221-224), `bindingUnaffected` true (lines 225-227). ✅
4. Negative cases run **against the valid fixture** and must not disturb it; final `runtimeInventoryPresent` guard (lines 269-271) confirms the fixture's runtime is still intact after the negative apply. ✅

Ordering note: the negative cases (4a/4b) run before the positive apply (2). They are independent because the stale case uses a mismatched plan id and the missing-second-stage uses an un-acknowledged plan id; neither mutates state, and `bindingUnaffected` is asserted on both. The final inventory check covers "fixture untouched by the negative cases" as the o10 note states. All four Codex steps are covered with no extra negative scenarios beyond the spec.

Type-checked against source: `machine.Classify` returns `*machine.Error` with `.Code` (contract.go:60,99); `ErrorConflict`/`ErrorApprovalRequired` are `ErrorCode` (contract.go:41,48); `ProviderChangeConsent` has `PlanID/First/Second/DeleteData` (provider_change_types.go:182-187); `ProviderChangeResult.Status` is `string` (provider_change_types.go:219,228). Compiles via `go vet`.

## (2) CI gating / skip semantics

- **Opt-in via env guard at the top of the test** (lines 47-49): `t.Skip` when `BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE != "true"`. This is the first statement, before any runtime/docker access.
- **Skip-not-fake is safe here.** Without the env var set, CI's standard unit step (`ci.yml:121` — `go test ./cmd/baha`) skips the test cleanly and never runs it. There is no CI job that sets `BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE` and runs this test by a broad pattern — analogous to the established `mcp-lifecycle-acceptance.yml` job which sets `BASEHARBOR_MCP_LIFECYCLE_ACCEPTANCE` and selects the sibling test via explicit `-run '^TestMCPGenericClientRealApplicationLifecycle$'`. No CI job currently exists for `TestAppEditMCPApplyRunsRealProviderEditLifecycle`; the opt-in guard correctly prevents it from ever running (or failing) on unconfigured runners.
- **No build tag** — matches the sibling MCP-native test (`app_mcp_lifecycle_acceptance_test.go`) which also uses an env skip rather than a `//go:build` tag. Both native tests skip before touching docker; the skip is genuine (not faked), and CI never claims it ran because CI never sets the gate.
- `DOCKER_HOST` is set to the hosted rootful socket *after* the opt-in guard (lines 50-52) with an explicit "never fall back after a failed engine selection" comment — consistent with the o10 run environment and the sibling's own `testruntime.Resolve` runtime-gate approach.

## (3) gofmt / vet

- `gofmt -l cmd/baha/app_mcp_provider_edit_native_test.go` → clean (no output).
- `go vet ./cmd/baha/` → rc=0, clean.

Local `go test -run TestAppEditMCPApply -count=1` is expected to **skip** without `BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE=true` (env guard) and would otherwise require a rootful docker daemon on `unix:///var/run/docker.sock` — a docker-less local run is an environment limitation only, not a product finding. No local test execution performed; native pass is evidenced by o10 (exit 0, ~202 s).

## Verdict

**APPROVE**

The delta is a single, correctly-gated native acceptance test that implements exactly Codex's four blocking requirements against the real MCP `baseharbor.apply` entry point: valid apply with staged consent reaches a complete verified result; stale proposal yields structured `conflict` with unchanged binding; missing second stage yields structured `approval_required` with unchanged binding. gofmt/vet clean. Skip semantics are genuine and CI-safe (opt-in env gate; never runs or fails on unconfigured runners). No product edits performed by this review.

R3-998-DONE
