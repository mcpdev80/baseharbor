# o8 — core-921 PR #998 read-only delta review (CORRECTED HEAD `6931ba9c`)

**Worker:** Ornith o8 (fresh independent reviewer, did NOT implement)
**Task:** core-921 · CLI and MCP: register new interfaces consistently
**PR:** mcpdev80/baseharbor#998 · branch `work/core-921-central-cli-mcp-registration`
**Baseline (prior approved head):** `08d8f498fd82992e762d764967ac892721d9adeb`
**Corrected head (reviewed):** `6931ba9c8e604419ae0afb7ff52dbdd20da840f8`
**Reviewed delta:** `08d8f498..6931ba9c`
**Reference verdicts:** Codex 6.1 round 1 (`codex-verdict-core921-round1.md`, #998 at `08d8f498`: CHANGES_REQUESTED); implementer report `o7-998-codexround.md`.
**Worktree HEAD verified:** `6931ba9c8e604419ae0afb7ff52dbdd20da840f8` (== corrected head, detached, clean).
**No product edits, no GitHub writes, no merges. Read-only delta review.**

## Scope (the binding re-pin depends on this exact table)

`git diff --name-status 08d8f498..6931ba9c` (binding anchor `08d8f498` == prior approved head;
`6931ba9c` == current reviewed head):

| status | path |
|--------|------|
| M | `cmd/baha/app_machine_adapters.go` |
| M | `cmd/baha/main.go` |
| M | `cmd/baha/registration_921_test.go` |

Three files only. `19548bf0-` does not resolve to a local Git ref in this worktree; the
binding anchor is the prior approved head `08d8f498`. No `contracts/`, `docs/`,
`app_registration_921.go`, `pr-gate-plan`, `DEVELOPMENT_GUIDELINES`, or #995/Qwen/core-754
files are touched.

## (1) Typed error origin at the MCP boundary — VERIFIED

`classifyAppMachineError` (`cmd/baha/app_machine_adapters.go:20-54`) maps every private
`appMachineError` code to a concrete shared `*machine.Error` owner:

| private code | typed owner | `Code` | `Retryable` |
|---|---|---|---|
| `conflict` | `ErrorConflict` | `conflict` | false |
| `cancel` | `ErrorRuntimeUnavailable` | `runtime_unavailable` | true |
| `denied`, `unsupported` | `ErrorPolicyDenied` | `policy_denied` | false |
| `partial` | `ErrorVerificationFailed` | `verification_failed` | true |
| default | `ErrorInternal` | `internal_error` | false (with remediation) |

- **No new codes invented.** All five target `ErrorCode` constants (`ErrorConflict`,
  `ErrorRuntimeUnavailable`, `ErrorPolicyDenied`, `ErrorVerificationFailed`, `ErrorInternal`)
  exist in `internal/machine/contract.go:36-56` with the documented string values. `NewError`
  signature (`code, message, next, retryable`) matches the call sites.
- **Complete coverage of production sites.** The private `appMachineError{Code:"..."}` literal
  construction sites produce only `conflict / cancel / denied / unsupported / partial` — the
  exact five handled by the switch plus the `default`. Every one maps to a typed owner; the
  only `internal_error` path is the unclassified default, which is the intended fail-safe.
- **No `internal_error` fallthrough at the MCP boundary.** `main.go:315-324` routes
  `*appMachineError` through `classifyAppMachineError` via `errors.As(err, &machineErr)`
  **before** the generic `machine.Classify`. The boundary always surfaces a concrete
  `Code`/`CauseCode`/`Next`.
- **Reachability through `errors.Join`.** The three `errors.Join(&appMachineError{Code:"partial"}, …)`
  sites (`app_machine_adapters.go:229,435,438`) remain reachable — Go `errors.As` traverses the
  joined-error chain, so `classifyAppMachineError` still resolves the `partial` origin. Build/vet
  and the focused suite exit 0, consistent with this.
- **No guard weakened.** Consent/ownership/ordering guards in `app_registration_921.go` (wrong
  Target short-circuit, propose rejects carried proposal/consent, apply demands both at
  `:136-140`) are byte-unchanged; only the error-classification layer was added.

## (2) New tests in `registration_921_test.go` — VERIFIED

- **`TestCore921MCPProposeReachesHandlerWithZeroWrites`:** drives a **real** MCP `baseharbor.app.edit`
  `propose` through `workspaceMutationClient(t)` → the actual handler. Fails closed on a
  stale-snapshot intent (`snapshot_revision:"stale-snapshot-0"`) with a **typed**
  `payload.Error.Code == "conflict"` (structured code, not the old `"conflict"` substring match).
  Zero writes are proven by **byte/snapshot equality**: `bytes.Equal(sourceBefore, sourceAfter)`
  on the manifest plus `deploymentSnapshotEqual(before, after)` over JSON-marshaled, sorted
  deployment records — not by record count. Satisfies Codex P1-B.
- **`structuredCode` closure** (declared locally at line 243 inside the level-negatives test)
  asserts structured `Code`/`CauseCode`/`Next` from `result.StructuredContent` for
  `wrong-target`→`conflict`, `invalid-phase`→`validation_failed`, `apply-missing`→`validation_failed`,
  and `apply-missing-snapshot`→`validation_failed`. No substring matching anywhere in the delta.
- **`apply-missing-snapshot` reclassification is honest.** The apply phase demands the exact typed
  proposal + fresh consent before any snapshot comparison (`app_registration_921.go:136-140`),
  so an apply without proposal/consent returns `validation_failed`; the stale-snapshot conflict
  only fires after a valid propose→apply round-trip. The reviewer's implementer report (o7)
  documents this precisely. The public boundary still surfaces a concrete `code` either way.
- **Schema-root assertion fixed (Codex P2).** The ineffective `properties["app.edit"].required`
  check is removed; the root `required` block now asserts `desired` is present and that
  `proposal`/`consent` are absent from the root required list. Correct positive assertion.
- **`deploymentSnapshot` / `deploymentSnapshotEqual` helpers** marshal `deployment.DeploymentRecord`
  via `.Identity.DeploymentID` sort key and JSON — fields verified against `internal/deployment/deployment_store.go`
  (`DeploymentIdentity{DeploymentID, ApplicationID, Target, Application, Environment}`,
  `DeploymentRecord{Identity}`). `containsString` already exists in `app_observe.go:492` (no
  duplicate declared). No unused imports (`os`, `sort` both used).

## (3) Consent/ownership guard integrity — VERIFIED

`git diff 08d8f498..6931ba9c -- cmd/baha/app_registration_921.go contracts/ docs/` is empty.
Apply still demands exact typed proposal + fresh provider-consequence consent (`:138-139`);
only the error-classification layer was added.

## Verification (real foreground outputs, exit 0)

```
$ gofmt -l cmd/baha           # (clean — no output)
$ go build -o /tmp/baha-o8 ./cmd/baha        # BUILD_EXIT:0
$ go vet ./cmd/baha                          # VET_EXIT:0
$ go test ./cmd/baha -run 'Registration|Consent|MCP|Core921|AppMachine|AppProviderChoice' -count=1
ok  	github.com/mcpdev80/baseharbor/cmd/baha	31.309s
```

Env-gated native-docker acceptance tests (e.g. `TestAppEditCommandCompleteProviderEditRunsRealLifecycle`)
require a rootful docker daemon and are out of scope for this typed-error delta; they fail
identically on pristine HEAD in this environment.

## SHA-256 of delta files (before `08d8f498` / after `6931ba9c`)

| file | sha256 (old 08d8f498) | sha256 (new 6931ba9c) |
|------|------------------------|------------------------|
| `cmd/baha/app_machine_adapters.go` | `80a92b30…913701` | `2f1ab359…cefe` |
| `cmd/baha/main.go` | `09375e81…52cec` | `260163be…ef50` |
| `cmd/baha/registration_921_test.go` | `cf05b8a6…5f8856` | `912930f7…283` |

Files changed between the two heads; hash values confirm the delta is the implementer's
typed-error correction, not a silent re-pin of other work.

## Verdict

**APPROVE.** The delta exactly addresses the two Codex round-1 P1 findings plus the P2
schema-root assertion on the corrected head. Every private `appMachineError` origin now maps
to a typed `*machine.Error` with `Code`/`CauseCode`/`Next`, no new codes are invented, and the
MCP boundary has no `internal_error` fallthrough on a recognized origin. The new tests prove a
real MCP propose reaching the handler with byte/snapshot zero-write equality and assert
structured codes on the apply-side negatives. Consent/ownership/ordering guards are byte
unchanged. `gofmt -l` clean, `go vet` and the focused `go test` suite exit 0.

Binding re-pin anchor: `08d8f498` (prior approved head) → `6931ba9c` (this reviewed head).
Delta is 3 modified files (`cmd/baha/app_machine_adapters.go`, `cmd/baha/main.go`,
`cmd/baha/registration_921_test.go`).

Remaining honest gap (carried from o7, native-runtime only, **not** in this delta's scope):
a full **successful** propose→apply happy path through the real entry point needs a native
runtime (snapshot revision + provider-choice/impact authorities); the typed-conflict boundary
and zero-write proof are proven, the success path is documented as a native-runtime gap and
not claimed here.

Final marker: DELTA-O8
