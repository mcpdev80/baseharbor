# Codex 6.1 verdicts (session fe5dff6c-9df2-4b5f-842a-35ee4a881d3d, GPT-6.1-Sol, codex-cli 0.163.0, 2026-10-19T12:04:02Z)

**PR #1007 at `417b8cbb`: CHANGES_REQUESTED**

- **P1 — Required coverage is omitted.** The [new clause](/home/marcel/dev/baseharbor-ecosystem/worktrees/ci-core921-coverage-binding/scripts/pr-gate-plan.py:675) returns `resolved` without `wizard-registration-tests`, `wizard-machine-adapter-tests`, `wizard-provider-choice-tests` or `cli-gofmt`. I reproduced its selected check list. `cli-guided-tests` runs only `^TestGuided`; the existing registration runner also does not select `^TestCore921`. Explicitly require and execute the affected families, including these new regressions.
- **P2 — Negative regressions overstate coverage.** The dirty-head test stubs the proof to `False`; the purported wrong-status test changes file contents without changing its Git status. Add independent cases for wrong status, extra/missing paths and an actual unreviewed commit using the real ancestry verifier.

The exact SHA/path/status guards are otherwise appropriately narrow, and existing clauses remain unchanged. All four new tests pass, but they do not establish the missing guarantees.

**PR #998 at `08d8f498`: CHANGES_REQUESTED**

- **P1 — Adapter errors still become `internal_error`.** The direct wrong-target return is corrected. However, [adapter conflict/consent errors](/home/marcel/dev/baseharbor-ecosystem/worktrees/core-921-central-cli-mcp-registration/cmd/baha/app_machine_adapters.go:354) remain private `appMachineError` values forwarded through `machineMCPFailure`, without a recognized classification. Assert public structured codes and remediation for stale proposal/snapshot and consent failures.
- **P1 — Successful real MCP propose→apply remains unproven.** The [propose test](/home/marcel/dev/baseharbor-ecosystem/worktrees/core-921-central-cli-mcp-registration/cmd/baha/registration_921_test.go:138) deliberately expects failure with a hardcoded snapshot and checks the substring `"conflict"`. There is no successful MCP apply invocation. The correction report explicitly acknowledges this omission, contradicting the assignment’s evidence summary. Deployment-count equality also cannot establish that existing records or files were unchanged.
- **P2 — Schema assertion is ineffective.** The [top-level required-field check](/home/marcel/dev/baseharbor-ecosystem/worktrees/core-921-central-cli-mcp-registration/cmd/baha/registration_921_test.go:131) reads `properties["app.edit"].required` instead of the schema’s root `required`, so it silently checks nothing.

**CI and merge order:** [#1007’s run](https://github.com/mcpdev80/baseharbor/actions/runs/39247588325) is green. [#998’s failure](https://github.com/mcpdev80/baseharbor/actions/runs/39244355590) confirms the reported stale-base mismatch; I verified the merge commit’s actual parents. That infrastructure failure is separate from the findings above, and #1007 does not fix that race.

Correct and independently review #998, update its binding to the resulting reviewed SHA, then accept and merge the binding first. #998 requires a fresh current-base plan, actual green mandatory CI and final acceptance before merge.

Both candidate file manifests remained SHA-256 identical throughout this review. No product edits, GitHub writes or merges were performed.
