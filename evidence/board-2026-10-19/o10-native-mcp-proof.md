# o10 — native MCP provider-edit proof (core-921)

Worker run (Ornith), 2026-10-19. Closes the last blocking acceptance
requirement of core-921 PR mcpdev80/baseharbor#998 left by Codex 6.1 round 2
(session fe5dff6c): *the successful real MCP propose→apply path remains a
blocking acceptance requirement.*

Per the verdict this is the smallest sufficient native gate: one focused MCP
integration test on `baha-docker` only, using isolated state and a disposable
application. Marcel permitted only the `baha-docker` ssh alias for this run;
the podman/k3s VMs and #995 were not touched, and only the worker's own
resources were created and removed.

## What the test proves

`cmd/baha/app_mcp_provider_edit_native_test.go`
(`TestAppEditMCPApplyRunsRealProviderEditLifecycle`, opt-in via
`BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE=true`) reuses the fixture of
`TestAppEditCommandCompleteProviderEditRunsRealLifecycle` — a workload-only
baseline, then one supported SQL backend — and drives that edit through the
**real `baseharbor.apply` MCP entry point** rather than the CLI:

1. obtain the current workload-only snapshot from production read authorities;
2. real MCP `apply` with matching explicit staged consent for one supported
   provider change; require a complete verified receipt and the intended
   persisted SQL binding;
3. round-trip the returned proposal through JSON and require a complete
   verified apply result;
4. exercise a stale proposal and a missing-second-stage consent against the
   valid fixture, asserting structured error codes (conflict / approval
   required) and unchanged persisted state.

The worker installed its own isolated management Core in a dedicated target
project (`bh-docker-dev-shared`) so no shared Core/provider instance was
referenced or mutated. The foreign shared Core
(`bh-docker-test-core-shared`) on the VM was left untouched.

## VM run (redacted transcript)

Environment on `baha-docker`: Go 1.26.8, Docker server 29.8.1, rootful system
Docker (`unix:///var/run/docker.sock`), isolated `HOME`/`XDG_*`/`BASEHARBOR_STATE_DIR`.

```
=== RUN   TestAppEditMCPApplyRunsRealProviderEditLifecycle
    app_mcp_provider_edit_native_test.go:64: live runtime inventory: Core containers=0 total containers=20
    app_mcp_provider_edit_native_test.go:272: native MCP provider edit: real MCP apply converged a supported SQL add and verified it with a complete receipt; stale proposal rejected as conflict with unchanged binding; missing second stage rejected as approval-required with unchanged binding
--- PASS: TestAppEditMCPApplyRunsRealProviderEditLifecycle (202.53s)
PASS
FINAL_TEST_EXIT=0
```

The apply converged the supported SQL add and returned a verified complete
receipt (`State:running Verified:true`). The stale-proposal apply was rejected
with a structured `conflict` (`ErrorConflict`) and left the binding unchanged;
the missing-second-stage apply was rejected with a structured
`approval-required` (`ErrorApprovalRequired`) and left the binding unchanged.

## Verification

- `gofmt`: clean.
- `go vet ./cmd/baha/`: clean.
- `go build` (linux): clean.
- Native VM run: PASS, exit 0, ~202 s.

## Artifacts

- Committed + pushed on `work/core-921-central-cli-mcp-registration`.
- New head SHA: **`a66e5de664b22c295a8b5ae918a9c2dfe4c6e327`** (parent
  `6931ba9c`).

Note for the binding/Coordinator: the #998 PR binding needs a **re-pin to the
new head SHA `a66e5de6`** because this run added a commit on top of the
previously bound `6931ba9c`.

End: NATMCP-O10
