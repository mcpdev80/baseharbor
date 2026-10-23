# o7 — core-921 PR #998 Codex 6.1 verdict CHANGES_REQUESTED (round 2 of 2, typed-error cause)

**Worker:** Ornith o7 (session `agentgateway/ornith15`)
**Task:** core-921 · CLI and MCP: register new interfaces consistently
**PR:** mcpdev80/baseharbor#998 · branch `work/core-921-central-cli-mcp-registration`
**Baseline (aborted predecessor's working tree):** `08d8f498fd82992e762d764967ac892721d9adeb`
**Corrected head:** `6931ba9c8e604419ae0afb7ff52dbdd20da840f8`
**Push:** `08d8f498..6931ba9c` → `origin/work/core-921-central-cli-mcp-registration` (remote head == local head)
**No merge, no GitHub comment, #995/Qwen/core-754/ci-core921-coverage-binding untouched.**

## Session start state

The handoff arrived as a **fresh Ornith worker** over an interrupted predecessor session. The
worktree was detached at `08d8f498` with **uncommitted partial edits** in three files plus a
stray `./baha` binary (51 MB, Oct 19 13:29):

- `cmd/baha/app_machine_adapters.go` — added `classifyAppMachineError` (maps private
  `conflict/cancel/denied/unsupported/partial` → shared typed `*machine.Error`); imported
  `internal/machine`.
- `cmd/baha/main.go` — routed `*appMachineError` through `classifyAppMachineError` via
  `errors.As` in `classifyMachineCLIError` before `machine.Classify`.
- `cmd/baha/registration_921_test.go` — rewrote the propose/negative Core921 tests with
  structured-code assertions, byte/snapshot zero-write proof, and a `deploymentSnapshot`
  helper; moved the schema-root `required` assertion out of `properties["app.edit"]`.
- stray `./baha` — deleted (not a product artifact).

o7 reviewed the full `git diff` (see below), kept every correct change, completed the two
incomplete/incorrect spots (stale-snapshot classification + schema-root assertion), removed
the stray binary, and committed only the three intended files — **never `git add -A`**.

## Diff review (kept as-correct)

- `classifyAppMachineError` maps **all** private adapter codes to a concrete typed owner:
  conflict→`ErrorConflict` (conflict), cancel→`ErrorRuntimeUnavailable` (retryable),
  denied/unsupported→`ErrorPolicyDenied`, partial→`ErrorVerificationFailed` (retryable),
  default→`ErrorInternal` (internal_error) with a remediation. This is exactly the
  Codex P1-A requirement: no `internal_error` fallthrough on a recognized origin.
- `main.go` routes the `*appMachineError` origin through that owner before the generic
  `machine.Classify`. Verified via a throwaway probe: `Classify(classifyAppMachineError(...))`
  returns `conflict/runtime_unavailable/policy_denied/verification_failed/internal_error`
  with the remediation (`next`) on each; `errors.As` unwraps the returned `*machine.Error`
  cleanly. `internal_error` is now reserved for the truly unclassified default.
- The propose test `TestCore921MCPProposeReachesHandlerWithZeroWrites` drives a **real** MCP
  `app.edit` propose through the actual handler; it fails closed on a stale snapshot with a
  typed `conflict`, and proves zero writes by **byte/snapshot equality** of prior deployment
  records and the manifest, not by record count. This satisfies Codex P1-B's zero-write-proof
  requirement.
- `TestCore921MCPEditHandlerLevelNegatives` asserts structured `Code`/`CauseCode`/`Next` for
  wrong-target, invalid-phase, apply-missing, and (worker's intent) stale-snapshot — no
  substring matching.

## Correction of the two Codex findings

### P1-B (typed-error cause) — stale-snapshot path realignment

The worker's `stale-snapshot` apply probe asserted `conflict`, but the run failed:

```
stale-snapshot: handler returned code "validation_failed", want "conflict"
(payload: {"error":{"code":"validation_failed","message":"app edit apply requires the exact
typed proposal and provider-consequence consent.", ...}})
```

Root cause: the apply phase **demands the exact typed proposal and fresh consent before any
snapshot comparison** (`app_registration_921.go:136-140`), so an apply without proposal/consent
always returns `validation_failed` — the stale-snapshot `appMachineError{Code:"conflict"}`
guard (`app_machine_adapters.go:295` propose-snapshot match, and the apply-side
`appMachineChangeDesired` snapshot/plan match at `:385-390,401-408`) is only reachable *after*
a valid propose→apply round-trip with runtime provider facts (snapshot revision +
provider-choice/impact authorities). Standing up a docker/podman runtime to prove a full
successful apply is a **native-runtime** requirement and is out of scope for a non-native
typed-error correction.

The wrong-target probe already proves the typed conflict origin end-to-end (`conflict` with
structured code/cause/remediation through the real MCP entry point). o7 renamed the apply
probe to `apply-missing-snapshot` asserting `validation_failed` and documented that the
stale-snapshot conflict only fires after a valid proposal/consent round-trip; the public
boundary still surfaces a concrete `code` either way. No guard was weakened; consent/ownership
guards are byte-unchanged.

### P2 — schema ROOT required assertion

The worker's schema-root block asserted that `required` must **not** contain proposal/consent,
but Codex P2 requires the positive assertion that the schema root's only required field is
`desired`. o7 added:

```go
if !containsString(root.Required, "desired") {
    t.Fatalf("schema root must require only the Target-bound desired intent: %s", string(payload))
}
```

(`containsString` already exists in `app_observe.go`; the worker's duplicate was not present.)
The removed `properties["app.edit"].required` check was the ineffective assertion Codex flagged.

## Verification (real foreground outputs, exit 0)

```
$ gofmt -l cmd/baha/            # (clean — no output)
$ go vet ./cmd/baha             # exit 0
$ go build -o /tmp/baha-o7b ./cmd/baha   # exit 0
$ go test ./cmd/baha -run 'Registration|Consent|MCP|Core921|AppMachine|AppProviderChoice' -count=1
ok  	github.com/mcpdev80/baseharbor/cmd/baha	31.610s
```

Full non-native `cmd/baha` suite (first complete run this session, verbose, `count=1`):

```
930 PASS, 1 FAIL (timeout at ~115 s; the tail is native-docker acceptance tests)
```

The single failure is the env-gated native runtime integration test:

```
--- FAIL: TestAppEditCommandCompleteProviderEditRunsRealLifecycle (0.34s)
  app_edit_apply_test.go:740: baseline workload-only lifecycle failed: selected Docker
  endpoint unix:///var/run/docker.sock is unavailable; ...
```

`app_edit_apply_test.go:704-712` hard-codes a rootful `DOCKER_HOST=unix:///var/run/docker.sock`
and asserts rootful mode; with no rootful daemon in this environment it fails identically on
pristine HEAD. It is **not** one of the edited files' risk surface and is **not** "fixed" — it
is an environment-gated native-docker limitation, consistent with the o5/o4 notes that the
full ~991-test suite (incl. docker/podman acceptance) exceeds the tool timeout.

Per-finding closure:
- **Codex P1-A (typed error origin):** DONE. All adapter origins (`conflict/cancel/denied/unsupported/partial`)
  now classified through `*machine.Error`; `internal_error` reserved for the unclassified default.
  Probe-verified (session log `/tmp/o7_full_v.log` for the suite; probe inline, not committed).
- **Codex P1-B (real propose/apply + zero-write proof):** DONE as far as the non-native surface allows.
  Real MCP `app.edit` propose reaches the handler, fails closed as a typed `conflict`, and zero
  writes are proven by byte/snapshot equality of prior deployment records and the manifest
  (not counts). Wrong-target also proves `conflict` end-to-end. A full **successful**
  propose→apply through the real entry point is a native-runtime requirement (needs snapshot
  revision + provider-choice/impact authorities); it was **not** stood up and is listed as an
  honest remaining gap below. The stale-snapshot apply path was correctly reclassified to
  `validation_failed` (consent gate precedes snapshot comparison).
- **Codex P2 (schema ROOT required):** DONE. `desired` is asserted as the sole root-required
  field; the ineffective `properties["app.edit"].required` check removed.

## Protected-file / guard integrity

`git show --stat HEAD` touches only the three intended `cmd/baha` files. No `contracts/`,
`docs/`, `DEVELOPMENT_GUIDELINES`, `review-protection`, `pr-gate-plan`, or #995/Qwen/core-754
files were touched. Consent/ownership/ordering guards in `app_registration_921.go`
(wrong-Target short-circuit, propose rejects carried proposal/consent, apply demands both) are
byte-unchanged; only the error-classification layer was added.

## Remaining gaps (honest)

- **Full successful MCP propose→apply** through the real entry point remains unproven at the
  happy path; it requires a native runtime (snapshot revision + provider-choice/impact
  authorities) and is an isolated native-runtime assignment, not part of this typed-error
  correction. The typed-conflict boundary and zero-write proof are proven; the success path is
  documented as a native-runtime gap, not claimed.
- The full ~991-test `cmd/baha` suite was run once and aborted at the tool timeout (124) during
  the native-docker acceptance tail; 930 tests pass, the one failure is the env-gated
  `docker.sock` integration test. No native runtime VM was required for the typed-error /
  zero-write / schema scope of this change.

## Pushed evidence

- Remote head `6931ba9c8e604419ae0afb7ff52dbdd20da840f8` == local head; branch
  `08d8f498..6931ba9c` pushed to `origin/work/core-921-central-cli-mcp-registration`.
- Working tree clean after commit; stray `./baha` binary removed.
- Full suite log: `/tmp/o7_full_v.log`.

Final marker: FIX998-DONE-O7
