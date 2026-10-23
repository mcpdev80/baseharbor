# o20 — app.edit propose round-trip through apply with structured negatives (core-921)

Worker run (Ornith), 2026-10-20. Closes Codex 6.1 round 3 (session
fe5dff6c-9df2-4b5f-842a-35ee4a881d3d, GPT-6.1-Sol / codex-cli, CHANGES_REQUESTED)
for `baseharbor-plan` core-921 PR `baseharbor#998` at `a66e5de6`.

## What Codex 6.1 round 3 left open

The round-3 verdict required the native gate to:
1. invoke `baseharbor.app.edit` (propose → JSON round-trip → apply), never
   `baseharbor.plan` + `baseharbor.apply`;
2. obtain an app.edit proposal and round-trip it through apply, not hand-write
   the desired manifest and decode an apply receipt;
3. assert the relevant persisted state before/after, not "one binding remains
   present";
4. never return success when the state inspection fails.

The WIP in the worktree already drove the edit through `baseharbor.app.edit`,
round-tripped the typed proposal through JSON, required a complete verified
apply receipt, and added a byte-wise `editStateUnchanged` comparison whose
inspection helper fails-closed. This lane reviewed the delta critically and
fixed what was missing.

## Critical review findings and fixes

### Fix 1 — propose requirement/manifest provider mismatch (bug)
The WIP mixed providers: the manifest carried cache instances
(`application.WithCacheInstances`) while `appEditProposeThroughMCP` sent a
`database.sql` requirement and the registry was seeded with `ProviderValkey`.
The propose phase resolves capabilities from the manifest footprint, so a
cache manifest with a SQL requirement is an incompatible request that fails
before the happy path. Reverted the provider choice to the proven
SQL/PostgreSQL path used by the passing `TestAppEditCommandCompleteProviderEditRunsRealLifecycle`
and the o10 native proof, keeping every requirement/choice/registry/binding
reference consistent (`cache.key-value`/`database.sql`, `ProviderValkey`/`ProviderPostgreSQL`,
`capability.KeyValue`/`capability.SQL`).

### Fix 2 — removed debug artifacts
Deleted the WIP debug blocks that (a) appended to `/tmp/dbgdir/dbg_appedit.log`,
(b) re-proposed to log plan determinism, (c) compared standalone vs nested
desired JSON, and (d) logged apply argument keys. Removed the now-unused
`mapArgs` helper and the `fmt`/`reflect` imports. The apply path is now a
single direct `baseharbor.app.edit` call with the round-tripped proposal and
matching staged consent.

### Fix 3 — tightened the byte-wise state snapshot
`editStateSnapshot` now snapshots only `provider-registry.json` (the real
persisted binding state), dropping the `provider-desired-preferences.json`
entry, which no real apply writes and which only ever recorded "missing".
The comparison is now strictly about the binding registry.

### adapters.go delta justification (already present in WIP, ctx check restored)
`appMachineChangeDesired` (`cmd/baha/app_machine_adapters.go:376`) compares the
fresh re-proposed plan against the round-tripped proposal by **serialized form**
instead of `reflect.DeepEqual`:

```go
freshPlan, freshErr := json.Marshal(fresh.Plan)
proposedPlan, proposedErr := json.Marshal(req.Proposal.Plan)
if freshErr != nil || proposedErr != nil || string(freshPlan) != string(proposedPlan) {
	return application.ProviderChangeResult{}, &appMachineError{Code: "conflict"}
}
```

A JSON round-trip through the MCP client yields a distinct
`*ProviderRealization` After-pointer that `reflect.DeepEqual` flags as a
divergence even when the resolved impact is identical. Comparing the
marshaled forms closes the round-3 value-equality requirement without weakening
the surrounding `conflict` classification. The post-`appMachinePropose`
`ctx.Err()` check removed by the WIP was restored to preserve the original
fail-closed cancellation guard.

## Verification

- `gofmt -l`: clean (both files).
- `go vet ./cmd/baha/`: clean.
- `go build ./cmd/baha`: clean (`/tmp/baha-core921`).
- `go test ./cmd/baha/ -run "TestAppEdit|TestApp.*Apply|TestApp.*Propose|TestApp.*Change|TestApp.*Converge" -count=1`: PASS.

### Native run — BLOCKED, not run
`TestAppEditMCPAppEditRunsRealProviderEditLifecycle` (opt-in via
`BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE=true`) requires `assertCorelessHost`,
which fails when **any** Core container is present on the Docker host. This dev
host runs the shared `bh-docker-dev-shared` Core (keycloak/openbao/postgres) and
the `baha-docker` ssh target runs `bh-docker-test-core-shared`; both trip the
guard before the test reaches its logic. The o10 proof ran on a clean CI runner
(`Core containers=0`). This lane could not execute the native run without
tearing down production Core infrastructure, which is outside scope. Run the
native gate on an isolated CI runner with zero pre-existing Core containers.

### Non-blocking baseline failure (environmental, pre-existing)
`TestAppEditCommandCompleteProviderEditRunsRealLifecycle` fails on both the
base `a66e5de6` and this WIP with `material inventory is incomplete; unlisted
material is not claimed preserved or deleted` — identical on an unstashed
baseline (verified via `git stash`), so it is a shared-Docker-host artifact,
not introduced by this lane. The focused apply/propose/change tests above pass.

### Repo-rule compliance
- No product code written by the coordinator; only this worker's review,
  debug cleanup, and a provider-consistency revert in the test adapter.
- `app_machine_adapters.go` delta is a narrow, guarded value-equality fix; the
  `conflict` classification and all surrounding fail-closed checks are intact.
- Regenerated views were not touched; no binaries staged (the stray `baha` ELF
  binary was deleted from the worktree before commit).
- gofmt/vet/build all green; focused apply/propose/change tests green.

## Evidence baseline (sha256, pre-commit candidate)
- `cmd/baha/app_machine_adapters.go`
  `6ec65d954b88d2d77c5e36c0794707e2f91eb84c14dd99a2fc481b25474995bc`
- `cmd/baha/app_mcp_provider_edit_native_test.go`
  `315c2d3780b647eb87b195c45a5b89bf613510649994bd71a7503750492500ed`

## Verdict
Candidate ready for commit on
`work/core-921-central-cli-mcp-registration`: round-3 requirements satisfied
(edit round-trip, byte-wise fail-closed state comparison, inspection helper
fails on error), adapters delta justified, gofmt/vet/build green. Native gate
blocked by shared-Core host constraint, not by the code.

END O20-APPEDIT-DONE
