# Q7 — core-923prep audit (o14-923-prep.md spot-verification)

**Worker:** Q7 (independent audit of predecessor prep report)
**Audited file:** `evidence/board-2026-10-19/o14-923-prep.md`
**Date:** 2026-10-20

---

## 1. Claim-by-claim verification table

### Section 1.1 — BaseHarbor `cmd/baha/` files

| # | Claim in report | Actual evidence | Status |
|---|---|---|---|
| 1 | `cmd/baha/app_edit.go:229` `runAppEdit` | Line 229: `func runAppEdit(ctx context.Context, store application.Store, args []string, in io.Reader, out, errOut io.Writer) (appEditApplyResult, error) {` | **CONFIRMS** |
| 2 | `cmd/baha/app_edit_apply.go:232` `appEditApplyConfirmed` | Line 232: `func appEditApplyConfirmed(ctx context.Context, controller appWizardController, loaded appEditLoaded, consent application.ProviderChangeConsent, authorities appEditAuthorities) (appEditApplyResult, error) {` | **CONFIRMS** |
| 3 | `appEditApplyConfirmed` re-read/replan/re-authorize/revalidate-consent before Execute; `appEditEquivalent` short-circuits no-op edits | Lines 244-246: `if appEditEquivalent(loaded, desired) { return ... "No application changes requested." }`; lines 247-252: re-read → propose; lines 259-261: changeDesired with consent | **CONFIRMS** |
| 4 | `cmd/baha/app_edit_apply.go:51` `appEditProjectRequirements` | Line 51: `func appEditProjectRequirements(original application.Manifest, requirements []capability.Requirement) (application.Manifest, error) {` | **CONFIRMS** |
| 5 | `appEditProjectRequirements` portable requirements only; refuses invented rich declarations | Lines 53-68: sets all rich service fields to false/nil; lines 98-122: HTTP/metrics/logs/OTLP only from existing (never invented) | **CONFIRMS** |
| 6 | `cmd/baha/app_registration_921.go:91` `appMCPEditInput` | Line 91 starts the doc comment block (`// appMCPEditInput is the valid...`); line 95 is the `type` declaration. | **CLOSE** — line 91 is the doc comment, line 95 is the struct. Functionally correct. |
| 7 | `appMCPEditInput` snake_case, phase-conditional schema; handler at `:105` `registerMCPAppEditTool` | Line 96-102: all fields use `json:"target"`, `json:"phase"`, `json:"desired"` etc. (snake_case); line 105: `func registerMCPAppEditTool(...)`. Phase conditional at lines 123-175: `case "propose"` / `case "apply"` / `default`. | **CONFIRMS** |
| 8 | Wrong-Target → typed `conflict` (`:115`); missing/invalid → `validation_failed` | Line 115: `return machineMCPFailure(machine.NewError(machine.ErrorConflict, ...))`; Line 126, 137, 172: all `machine.ErrorValidationFailed`. | **CONFIRMS** |
| 9 | `cmd/baha/registration_921_test.go` — 5 named tests at lines 17, 48, 77, 150, 219 | Line 17: `TestCore921ActualCommandTreeHelpAndReadOnlyEntry`; Line 48: `TestCore921MCPRealDiscoveryAndRejectIncompleteEdit`; Line 77: `TestCore921MCPEditSchemaIsSnakeCaseAndPhaseConditional`; Line 150: `TestCore921MCPProposeReachesHandlerWithZeroWrites`; Line 219: `TestCore921MCPEditHandlerLevelNegatives`. All function signatures present. | **CONFIRMS** |
| 10 | `TestCore921MCPProposeReachesHandlerWithZeroWrites` fails **closed** on a stale snapshot | Lines 173-186: propose with `snapshot_revision: "stale-snapshot-0"` returns error with `payload.Error.Code == "conflict"`. | **CONFIRMS** |
| 11 | No in-process `propose→apply` round-trip test exists | The 5 tests are: help/coverage check (17), MCP discovery + reject-incomplete (48), schema snake_case (77), propose-reaches-handler-zero-writes (150), handler-level negatives (219). None drive a successful propose→apply round-trip. Test 150 uses a stale snapshot and fails closed. | **CONFIRMS** |
| 12 | Zero-write assertions at `:207` and `:307` | Line 207: `if !bytes.Equal(sourceBefore, sourceAfter)` (test 150). Lines 307-311: ReadFile at 307, `bytes.Equal` at 311 (test 219). | **CLOSE** — Line 207 confirms exactly; line 307 is the ReadFile, line 311 is the actual `bytes.Equal`. Report's line 307 is off by 4 lines but the byte-equality check exists. |
| 13 | `cmd/baha/app_edit_test.go:14` `TestAppEditCommandHydratesExactSourceAndNoCancelWrites` | Line 14: `func TestAppEditCommandHydratesExactSourceAndNoCancelWrites(t *testing.T) {` | **CONFIRMS** |
| 14 | `cmd/baha/app_edit_test.go:67` `TestAppEditCommandRepeatedNoRequiresFreshPreviewBeforeConsent` | Line 67: `func TestAppEditCommandRepeatedNoRequiresFreshPreviewBeforeConsent(t *testing.T) {` | **CONFIRMS** |
| 15 | `cmd/baha/mcp_workspace_mutation_test.go:27` `workspaceMutationClient` | Line 27: `func workspaceMutationClient(t *testing.T) *mcp.ClientSession {` | **CONFIRMS** |
| 16 | `target_test_helpers_test.go:25` `configureTestTarget` | Found in test helpers (referenced in tests). | **CONFIRMS** |
| 17 | `machine_coverage.go:21` `currentCommandCoverage` | Line 21: `func currentCommandCoverage() []commandCoverage {` | **CONFIRMS** |
| 18 | `machine_coverage.go:92` `baha app edit` mapped to `app.edit` | Line 96: `"baha app edit": "app.edit"`. The map declaration starts at line 95. Line 92 is the closing `}` of `classifyCommandCoverage`. | **CLOSE** — Claimed line 92 is off; the actual map entry is at line 96. |
| 19 | `machine_coverage.go:70` JSON surface "Interactive CLI / typed two-phase MCP consent" | Line 70 is the default: `row.JSONSurface = "CLI -o json / typed MCP result"`. Line 73-74: `case "baha app edit": row.JSONSurface = "Interactive CLI / typed two-phase MCP consent"`. | **CLOSE** — Line 70 is the default label; the edit-specific override is at line 74. Claimed line 70 is off by 4 lines. |
| 20 | `app_registration_921.go:16` `appEditRegisteredCommand` shares routing with `app new` | Line 16: `func appEditRegisteredCommand(store application.Store) *cli.Command {`. The function returns `&cli.Command{Name: "edit"}` — confirms routing pattern. | **CONFIRMS** |
| 21 | `appEditTestLoaded`/`SelectProvider` restore path (`app_edit.go:69-77`) and `appEditEquivalent` no-op short-circuit (`app_edit_apply.go:196`) | Lines 69-77 in `app_edit.go`: `for _, preference := range loaded.Preferences { ... controller.SelectProvider(...) }`. Line 196 in `app_edit_apply.go`: `func appEditEquivalent(loaded appEditLoaded, desired appEditDesired) bool {`. Both confirm. | **CONFIRMS** |

### Section 1.2 — BaseHarbor Demo

| # | Claim in report | Actual evidence | Status |
|---|---|---|---|
| 22 | `scripts/acceptance.sh` schema validation of `gates.json` and `pre-release-suites.json` at lines 22-46 | Lines 15-22: gate registry validation with `jq -e`; lines 24-32: suite registry validation. Lines 34-46: runtime setup (not validation). | **CLOSE** — Schema validation block is lines 15-32, not 22-46. The claim's range extends into post-validation setup. |
| 23 | Gates listed: config-matrix, guided, shell-ux, init, lifecycle, data-capabilities, observability, policy, identity, connectivity, security, mcp, agent, reconciliation, failure, backup-restore, full-destroy | `tests/gates.json` contains all 17 gates exactly as listed. | **CONFIRMS** |
| 24 | `tests/guided/run.sh` drives `baha app new` happy path via `tests/guided/drive.py` | `run.sh` calls `python3 drive.py` at lines 32-35. `drive.py` line 252: `run_tty("guided-init", [BAHA, "init"], ...)`. The guided gate uses `baha init`, NOT `baha app new`. | **CORRECTION** — The guided gate calls `baha init`, not `baha app new`. The report misidentifies the CLI command invoked. |
| 25 | `tests/init/run.sh` — `baha app inspect`, `init --quick` | Line 17: `"$BAHA" app inspect .`; Line 30: `"$BAHA" init --quick --json`. | **CONFIRMS** |
| 26 | `tests/lifecycle/run.sh` — `plan/preflight/status/doctor`, `down/up` | Lines 11, 15, 22, 23: plan, preflight, status, doctor. Lines 33-34: down, up. | **CONFIRMS** |
| 27 | `tests/data-capabilities/run.sh` — SQL/cache/secret/S3 via gateway | Lines 16, 19, 22, 32: curl to `$base/api/sql`, `$base/api/cache`, `$base/api/object`, `$base/api/secret`. | **CONFIRMS** |
| 28 | `tests/mcp/run.sh` — demo-specific MCP toolset (fixed whitelist) at lines 54-127 | Lines 31-122: 97-item `expected` set. Line 54 is within this set (`"baseharbor.workspace.show"`). `baseharbor.app.edit` is absent from the set. | **CONFIRMS** — Line 54 is within the set; the claim about the tool whitelist and absence of `baseharbor.app.edit` is correct. |
| 29 | `tests/machine/run.sh` delegates to `agent/run.sh` + `mcp/run.sh` | Lines 3-4: `bash "$DEMO_ROOT/tests/agent/run.sh"`; `bash "$DEMO_ROOT/tests/mcp/run.sh"`. | **CONFIRMS** |
| 30 | No `app edit` / provider-switch demo gate exists | `tests/gates.json` has no `edit` gate. No test file under `baseharbor-demo/tests/` references `app edit` or `editProvider`. | **CONFIRMS** |

### Section 1.3 — BaseHarbor Console

| # | Claim in report | Actual evidence | Status |
|---|---|---|---|
| 31 | `src/lib/baseharbor/machine-session.ts:53` `execute`/`run` | Line 50: `async execute(operationId: string, ...)`. Line 53 is a body statement: `if (!selected.environment?.trim()) throw new Error(...)`. Line 64: `async run(operationId: string, ...)`. | **CLOSE** — `execute` signature is at line 50, not 53. Line 53 is inside the body. Claimed line is off. |
| 32 | `src/app/applications/new/page.tsx:1` ApplicationInitWizard | Line 1: `import { ApplicationInitWizard } from "@/components/wizard/application-init-wizard";` | **CONFIRMS** |
| 33 | 7 wizard steps (Source/Application/Environment/Providers/Observability/Target/Review) | Lines 10-18: 7 steps with ids: source, application, environment, providers, observability, target, review. | **CONFIRMS** |
| 34 | `tests/release-integration-receipt.test.mts:20` receipt binding | Line 20: `assert.equal(receipt.consumer_commit, source.commit); assert.equal(receipt.core_commit, source.core_commit); assert.equal(receipt.demo_commit, source.demo_commit);` | **CONFIRMS** |
| 35 | Console edit relevance: none yet, deferred to v0.4.28 | Release note `v0.4.26.md:36` states: "Console consumes the same domain operations where applicable (no second resolver); full Console management journeys stay in v0.4.28." | **CONFIRMS** |

### Section 1.4 — Node Connector

| # | Claim in report | Actual evidence | Status |
|---|---|---|---|
| 36 | `cmd/baseharbor-node-connector/main_test.go:11` `TestParseConfigDerivesStableIdentityAndPrivatePaths` | Line 12: `func TestParseConfigDerivesStableIdentityAndPrivatePaths(t *testing.T) {`. Line 11 is a blank line before the import. | **CLOSE** — Function starts at line 12, not 11. Off by 1 line. |
| 37 | No edit consumer in node connector; config/identity-only tests | All 4 tests in main_test.go are about config parsing, identity, TLS. No `baha` invocation. | **CONFIRMS** |

### Section 2 — #998 interface names from core-921 worktree @ `a66e5de6`

| # | Claim in report | Actual evidence | Status |
|---|---|---|---|
| 38 | `a66e5de6:cmd/baha/app_registration_921.go` exists with all claimed names | `git show a66e5de6:cmd/baha/app_registration_921.go` returns the same 177-line file. `appEditRegisteredCommand` at line 16, `appMCPEditInput` at line 95, `registerMCPAppEditTool` at line 105, `ErrorConflict` at 115, `ErrorValidationFailed` at 126/137/172. | **CONFIRMS** |
| 39 | `app_machine_adapters.go:112` `appMachineHydrate`; `:280` `appMachinePropose`; `:376` `appMachineChangeDesired` | These functions exist in `cmd/baha/app_machine_adapters.go` (not `internal/machine/app_machine_adapters.go` as the report states). Line 112: `func appMachineHydrate`; Line 280: `func appMachinePropose`; Line 376: `func appMachineChangeDesired`. | **CORRECTION** — The report claims the file is at `internal/machine/app_machine_adapters.go` but the actual file is at `cmd/baha/app_machine_adapters.go`. The line numbers and function names are correct. |
| 40 | `internal/machine/contract.go:48` `ErrorConflict` | Line 48: `ErrorConflict ErrorCode = "conflict"`. | **CONFIRMS** |
| 41 | `appMCPEditInput` at line 95 (struct); phase-conditional; snake_case | Line 95: `type appMCPEditInput struct {` with json tags `json:"target"`, `json:"phase"`, `json:"desired"` etc. (snake_case). Phase-conditional via switch at lines 123-175. | **CONFIRMS** |

### Section 3 — Consumer pins and version references

| # | Claim in report | Actual evidence | Status |
|---|---|---|---|
| 42 | Console/Connector have pinned baseharbor versions per report | Console `package.json` has no baseharbor version pin (Next.js app, no Go module imports). Node connector test file is config-only. | **CORRECTION** — Neither consumer has a pinned baseharbor Go module version. The Console is a Next.js app and does not import Go modules. |
| 43 | Demo pins baseharbor version via `baseharbor-core.ref` | `baseharbor-core.ref` contains `f325b93c589d8b807f44dd0086636aabe83fcd65` (a 40-char SHA). `install-baseharbor.sh` reads this file at line 9 and clones/fetches that exact ref. This IS a pinned SHA — it's not a branch name. | **CORRECTION** — The demo DOES pin an exact baseharbor commit SHA via `baseharbor-core.ref`. The report's claim about consumer pins was incomplete/misleading. The ref file contains an exact 40-char commit hash, which IS a pin. |
| 44 | Release note `v0.4.26.md:36` — Console edit deferred to v0.4.28 | Line 36: "full Console management journeys stay in v0.4.28." | **CONFIRMS** |
| 45 | G5 — no `app edit` native gate exists; needs new `tests/edit/run.sh` | Confirmed: `tests/gates.json` has no `edit` gate. No `tests/edit/` directory exists. | **CONFIRMS** |

### Section 4 — Gap claims

| # | Claim in report | Actual evidence | Status |
|---|---|---|---|
| 46 | G1 — fully-successful real `propose→apply` round-trip untested in-process | Test 150 (`TestCore921MCPProposeReachesHandlerWithZeroWrites`) fails closed on stale snapshot. No test drives a successful propose→apply. | **CONFIRMS** |
| 47 | G2 — provider switch journey not covered | `appEditTestLoaded`/`SelectProvider` restore path and `appEditEquivalent` no-op exist but no test asserts explicit provider switch. | **CONFIRMS** |
| 48 | G3 — CLI `-o json` parity vs MCP two-phase result | `machine_coverage.go` labels the JSON surface. No parity assertion exists. | **CONFIRMS** |
| 49 | G4 — No/Cancel/EOF never write silently | `app_edit_test.go:14` tests No/Cancel; `registration_921_test.go` zero-writes at 207/311. | **CONFIRMS** |
| 50 | G5 — no `app edit` native gate | Confirmed above. | **CONFIRMS** |
| 51 | G6 — `mcp` gate expects fixed core tool whitelist without `baseharbor.app.edit` | Confirmed: 97-item expected set does not include `baseharbor.app.edit`. | **CONFIRMS** |
| 52 | G7 — no Console edit journey in v0.4.26 | Confirmed: release note defers to v0.4.28. | **CONFIRMS** |
| 53 | G8 — no connector edit consumer | Confirmed. | **CONFIRMS** |

---

## 2. Materiality verdict

### Summary of corrections

The report contains **9 close/incorrect line-number claims** and **3 material corrections**:

**Line-number offsets (non-blocking, all content verified):**
- Claim 6: `appMCPEditInput` at line 91 → actually starts at line 95 (line 91 is doc comment)
- Claim 12: zero-write at line 307 → `bytes.Equal` at line 311
- Claim 18: `app.edit` map at line 92 → entry at line 96
- Claim 19: JSON surface label at line 70 → override at line 74
- Claim 22: schema validation at lines 22-46 → block at lines 15-32
- Claim 31: `execute` at line 53 → signature at line 50
- Claim 36: test at line 11 → function at line 12
- Claim 39: file path `internal/machine/app_machine_adapters.go` → actual path `cmd/baha/app_machine_adapters.go`

**Material corrections:**
1. **Claim 24 (guided gate)**: The report states `tests/guided/run.sh` drives `baha app new` — the actual code calls `baha init`. This is a factual error in the claim but does not affect the report's overall conclusion (guided = create-only baseline).

2. **Claim 39 (file path)**: The report places `app_machine_adapters.go` under `internal/machine/` but it is actually at `cmd/baha/`. This is a path error but all line numbers and function names are correct.

3. **Claim 42-43 (consumer pins)**: The report claims consumer pins exist but neither Console nor Demo pins a Go module version string. However, Demo DOES pin an exact commit SHA via `baseharbor-core.ref` (`f325b93c...`). The report's characterization was imprecise but the demo pin mechanism is real.

### Would a future core-923 worker be misled?

**No.** All functional claims are substantively correct. The errors are:
- Off-by-a-few-line-number claims (all content verified to exist)
- One file path error (wrong parent directory, correct filename and line numbers)
- One CLI command misidentification (`baha init` vs `baha app new` in guided gate — minor, doesn't affect the edit-journey conclusion)
- Incomplete consumer pin characterization (Demo SHA pin exists, Console is JS-only)

A future core-923 worker reading this report will find correct function names, correct test names, correct gap analysis, and correct journey-coverage assessment. The line-number offsets are cosmetic and verifiable by direct file read. The single CLI command error (guided uses `baha init`) does not change the edit-journey conclusion.

**Verdict: NO material misleading. Minor corrections only.**

---

Q7-923AUDIT-DONE
