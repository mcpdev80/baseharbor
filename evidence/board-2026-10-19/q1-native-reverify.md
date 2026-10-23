# Q1 — NATIVE RE-VERIFICATION (Flash, no product edits)

Worker run (Flash), 2026-10-19. Independent re-verification of o10's native
MCP proof. No product code changes; read-only evidence collection on the
bound SHA.

## Command

```
ssh baha-docker 'cd /home/marcel/baha-reverify-1792443444 && \
  BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE=true ./baha.test \
  -test.run TestAppEditMCPApply -test.v -test.timeout 15m; \
  echo "EXIT_CODE=$?"'
```

## SHA

`a66e5de664b22c295a8b5ae918a9c2dfe4c6e327` (verified via `git rev-parse HEAD`
after `git checkout --detach a66e5de664b22c295a8b5ae918a9c2dfe4c6e327`).

## Full output tail

```
=== RUN   TestAppEditMCPApplyRunsRealProviderEditLifecycle
    app_mcp_provider_edit_native_test.go:64: live runtime inventory: Core containers=0 total containers=20
    app_mcp_provider_edit_native_test.go:272: native MCP provider edit: real MCP apply converged a supported SQL add and verified it with a complete receipt; stale proposal rejected as conflict with unchanged binding; missing second stage rejected as approval-required with unchanged binding
--- PASS: TestAppEditMCPApplyRunsRealProviderEditLifecycle (234.22s)
PASS
EXIT_CODE=0
```

## Timing

- `TestAppEditMCPApplyRunsRealProviderEditLifecycle`: **234.22s** (~3m54s)
- Overall exit code: **0**

## Container cleanup verification

Post-run inspection of all containers on `baha-docker` shows zero new containers
created during the re-verify run. All containers visible in `docker ps -a`
belong to the pre-existing shared test infrastructure (`bh-docker-test-core-*`,
`bh-docker-test-shared-*`) dating from 2026-10-08 (11 days old). No containers
with labels `project=bh-docker-dev-shared`, `baseharbor`, or `io.komop` appear.
The test's isolated `bh-docker-dev-shared` project reuses the shared backend
infrastructure without creating disposable containers.

## Verdict

**REVERIFIED**

The native MCP apply path converges a supported SQL add and returns a verified
complete receipt. Stale proposals are correctly rejected with structured
`ErrorConflict`. Missing second-stage consent is correctly rejected with
`ErrorApprovalRequired`. Exit code 0. Timing consistent with o10's original
~202s (within normal variance of ~16% for infrastructure differences).

End: Q1-REVERIFY-DONE
