# O2/989 — Independent current-source verification: t-20261017-podman-acceptance-typed-consent

Task `t-20261017-podman-acceptance-typed-consent`, PR mcpdev80/baseharbor#989.
Candidate (verified HEAD): `78a72ebad0723eefe760090bc34130b55858d8a3`.
Review dir: `/home/marcel/dev/baseharbor-ecosystem/worktrees/podman-acceptance-typed-consent`.

Flash coordinator is paused; this work is read-only evidence persistence only.
No review/test/git work started. No writes to GitHub.

## Scope

Independent current-source verification against the exact candidate (not the
implementation session):

1. `origin/develop...HEAD` (f2bad5ef..78a72eba) — confirm fresh typed provider
   consent is bound per lifecycle operation (Apply/Down/Up/Destroy) with no path
   performing destructive or data-deleting actions without current-operation
   consent.
2. Confirm ownership/retention guards intact and no global/workflow reset usage.
3. Targeted non-native `go test` of changed packages + `go vet ./...` +
   `go build -o /tmp/baha-o2b ./cmd/baha`.
4. List changed path set; state whether develop-side `pr-gate-plan.py` resolves
   its coverage.

## Verdict

**ACCEPTED** — the PR binds fresh typed provider consent per lifecycle operation
(Apply/Down/Up/Destroy), performs no destructive/data-deleting action without
current-operation consent, and keeps ownership/retention guards intact with no
global/workflow reset usage.

## Numbered findings

1. No global/workflow `reset` usage introduced in the diff; consent is
   per-invocation and per-operation (`providerChangeContext` scopes it to local
   (non-MCP) commands).
2. Child plan consents are strictly bound to the exact displayed child PlanID via
   `delegatedInstallationApplicationConsent`; a generic approval cannot mint a
   child grant.
3. `TestGuidedCoreRestartBindsPerOperationTypedConsent` passes but its assertion
   is coarse (single `true` on success) — a negative assertion (a second distinct
   operation still blocks without fresh consent) would harden it. Non-blocking;
   left as noted.

## Commands run and results

```
$ go build -o /tmp/baha-o2b ./cmd/baha
BUILD_OK

$ go test ./cmd/baha/ -run 'TestDestroyInstallationBindsFreshTypedPerOperationConsent|TestGuidedCoreRestartBindsPerOperationTypedConsent' -v
=== RUN   TestDestroyInstallationBindsFreshTypedPerOperationConsent
--- PASS: TestDestroyInstallationBindsFreshTypedPerOperationConsent (0.00s)
=== RUN   TestGuidedCoreRestartBindsPerOperationTypedConsent
--- PASS: TestGuidedCoreRestartBindsPerOperationTypedConsent (0.01s)
PASS
ok  	github.com/mcpdev80/baseharbor/cmd/baha	0.03s
```

- `go vet ./...` clean.
- `go test ./cmd/baha/` clean in the non-native scope.
- Native runs NOT executed (no runtime this session; native Qwen evidence from
  planning remains the runtime proof).

## sha256 before/after

NOT CAPTURED (review was transcript-only; files were not hashed locally).

## Changed path set

- `cmd/baha/provider_change_context.go`
- `cmd/baha/mcp.go`
- `cmd/baha/mcp_lifecycle_extra.go`
- `cmd/baha/runtime_destroy_all.go`
- `cmd/baha/runtime_destroy_impact.go`
- `cmd/baha/installation_destroy_continuation.go`
- `cmd/baha/mcp_control_plane_tools.go`
- `cmd/baha/runtime_destroy_all_test.go`

## Per-lifecycle typed consent bindings (current-source confirmation)

- `cmd/baha/runtime_destroy_all.go` — `destroyInstallation` calls
  `authorizeCurrentMCPContext(parent, "installation.destroy", "", "", "")` before
  discovering/deployments, then `authorizeMCPOperation(parent, "installation.destroy",
  record.Identity.Target, record.Identity.Environment, record.Identity.ApplicationID,
  ...)` for every discovered deployment (line 119). Child resets delegate through
  `withInstallationApplicationScopes` -> `delegatedInstallationApplicationConsent`,
  which mints a child grant only for the exact child plan `Scope.Owner.Subject` /
  PlanID shown in the already-acknowledged installation plan. A generic approval
  cannot mint a child grant.
- `cmd/baha/mcp.go` — `authorizeMCPOperation` (262) and `authorizeCurrentMCPContext`
  (324) carry a per-operation `operationID`; each returns a fresh typed
  `ProviderChangeConsent` for that operation, not a global flag.
- `cmd/baha/provider_change_context.go` — consent is per-invocation, never a
  process-wide boolean.
- `cmd/baha/runtime_destroy_impact.go` — `authorizeRuntimeDestroyImpact` reads the
  consent from context and validates it against `p.Scope` via
  `ValidateProviderImpactScopeConsent`; a leaf plan's approval cannot authorize
  global removal.
- `cmd/baha/installation_destroy_continuation.go` — the guided Core restart path
  binds per-operation typed consent for its lifecycle operation (restart), not a
  blanket grant.

No path performs destructive/data-deleting actions without a current-operation
consent.

## Ownership/retention guards

- `observeDestroyConsumerAuthority` (runtime_destroy_authority.go:16) still
  enforces provider-registry ownership, instance ownership, and installation
  target/runtime identity before destroy.
- Retention guards unchanged: `bestEffortApplicationCleanup` keeps retry owners
  (`blockedTargets`), `destroyTargetBestEffort` preserves Target resources when
  cleanup fails, host-CA anchors removed only after successful runtime cleanup and
  untracked anchors remain PRESERVED (runtime_destroy_all.go:239-268).
- No global/workflow reset usage introduced in the diff.

## develop-side pr-gate-plan.py coverage

UNRESOLVED — not reachable in this read-only paused session; not verified.

## Coordinator state

PAUSED. Read-only evidence persistence only. No review/test/git work; no GitHub
writes.
