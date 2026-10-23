# core-921 / PR #998 — Read-only Local Review (2026-10-19)

- Task: core-921 — Central CLI/MCP registration for `app edit` and `app footprint show`
- PR: mcpdev80/baseharbor#998
- Candidate: `756b22dc8967d0daa7393065cd5bc15f262c7050` (verified exact at HEAD)
- Reviewer: o1-core921 (Ornith), read-only, no commits / no runtime
- Session: c3b6820f-5562-41a2-ae14-b92611dff14f (coordinator w1-baseharbor), mapping.json
- Worktree: `baseharbor/worktrees/core-921-central-cli-mcp-registration`

## Scope applied

Read `baseharbor/AGENTS.md`, `docs/DEVELOPMENT_GUIDELINES.md`, and the full
`origin/develop...HEAD` diff (13 files, +283/-2). Verified locally: `go vet ./cmd/baha/`,
`go build ./cmd/baha`, exact PR tests `TestCore921ActualCommandTreeHelpAndReadOnlyEntry`
and `TestCore921MCPRealDiscoveryAndRejectIncompleteEdit` (both PASS), and
`scripts/pr-gate-plan.py` coverage-binding logic (read-only). No product code touched.

## Findings

1. No duplicated registration of `app.new`. `app.new` MCP tool is registered
   exactly once (`cmd/baha/mcp_tools.go:353`, single `machineMCPTool("app.new"`).
   New `app edit` / `app footprint` are new, not re-registration of existing ops.
   (review of `cmd/baha/mcp_tools.go`, `cmd/baha/commands.go:240`)

2. Edit/footprint are registered through the accepted lifecycle owners. The two
   new CLI children are appended to the authoritative `appCmd` via
   `configuredApplicationCommand` (`cmd/baha/commands.go:240-242`) and the two
   MCP tools via `registerMCPApplicationMutationTools` /
   `registerMCPApplicationReadTools` (`cmd/baha/mcp_tools.go:139,469`). `app edit`
   CLI delegates to the existing `runAppEdit` handler (`cmd/baha/app_edit.go:229`);
   the MCP edit apply path reuses `appMachineChangeDesired` +
   `appEditRuntimeExecuteAuthority` with an exact fresh `Proposal` +
   `ProviderChangeConsent`. No new lifecycle or domain semantics are introduced
   (new file `cmd/baha/app_registration_921.go`, +167 lines, under the 800-line
   source limit).

3. CLI/JSON/MCP parity holds. `internal/machine/contract.go:228-229` and
   `contracts/machine/v1/control.golden.json:661-682` declare both operations;
   `machineMCPTool` derives `ReadOnlyHint` from `SafetyReadOnly`, so annotations
   are driven by the same registry the golden file and docs consume.
   EN/DE reference docs (`docs/reference/mcp.md`, `docs/de/reference/mcp.md`,
   `docs/reference/cli-machine-coverage.md`, `docs/de/reference/cli-machine-coverage.md`)
   and command indexes (`docs/cli/command-index.md`, `docs/de/cli/command-index.md`)
   add identical EN/DE rows.

4. EN/DE reference generation is consistent. `app.edit` maps to
   `baseharbor.app.edit` (mutating, confirmation `true`, policy `true`) and
   `app.footprint.show` to `baseharbor.app.footprint.show` (read_only) in both EN
   and DE golden/tool tables, matching `cliMachineOperations`
   (`cmd/baha/machine_coverage.go:73-96`) and `classifyCommandCoverage`
   (`cmd/baha/machine_coverage.go:71`).

5. Secret protection preserved. The edit path reuses existing guarded handlers;
   no password/token/key material is introduced. The `app.footprint.show` MCP
   handler is read-only: it resolves, authorizes via `authorizeResolvedMCPOperation`
   (`cmd/baha/mcp.go:313`), reads via the guided read context, and returns only
   the `application.Footprint` struct — no secret or credential path. No
   logging/print statements in the new file.

6. No lifecycle/semantic change. The change is registration wiring only; the new
   file contains no new ownership, consent, preflight, verify, or mutation logic
   beyond calling the accepted, reviewed handlers and adapters.

7. Coverage binding (proposal, no change). The CI coverage-unresolved state is
   driven by `scripts/pr-gate-plan.py`: a PR lands in `coverage_status: unresolved`
   when the changed path set is not bound to a named reviewed bundle. The merged
   #991 (9f622e6, reviewed head `CORE_991_REVIEWED_HEAD = eeda633071714a0b43551ab947ad036ffaac1b1b`)
   bound reviewed coverage by an exact-match clause in `plan()`:
   `len(paths)==len(expected)`, `set(paths)==set(expected)`, every
   `change.status==expected[path]`, plus `core752_reviewed_merge_chain(target, head, reviewed)`
   (strict clean first-parent/merge ancestry). The exact diff of #991 is exactly
   four files: `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`,
   `scripts/test_pr_check_runner.py`, `scripts/test_pr_gate_plan.py` — all
   `modified` — and that path/status set + reviewed-head ancestry is what selects
   `CI_CHECKS` and resolves coverage.

   **Minimal analogous binding for #998:** add one exact-match clause before the
   broad fallthrough, mirroring the #991 pattern:
   `CORE_921_REVIEWED_FILES = frozenset({`
   `  'cmd/baha/app_registration_921.go', 'cmd/baha/commands.go',`
   `  'cmd/baha/mcp_tools.go', 'cmd/baha/machine_coverage.go',`
   `  'contracts/machine/v1/control.golden.json', 'internal/machine/contract.go',`
   `  'docs/reference/cli-machine-coverage.md', 'docs/reference/mcp.md',`
   `  'docs/de/reference/cli-machine-coverage.md', 'docs/de/reference/mcp.md',`
   `  'docs/cli/command-index.md', 'docs/de/cli/command-index.md',`
   `  'cmd/baha/registration_921_test.go'`  (add/remove only if the landed set differs)
   `}` with `CORE_921_REVIEWED_HEAD = '756b22dc8967d0daa7393065cd5bc15f262c7050'`,
   guarded by
   `len(paths)==len(CORE_921_REVIEWED_FILES) and set(paths)==set(CORE_921_REVIEWED_FILES) and
   all(ch.status==expected[path]) and core752_reviewed_merge_chain(...)`, then
   `require(CI_CHECKS)`. This binds exactly the reviewed candidate (exact hash +
   complete changed status set + clean trusted-target merge) to `CI_CHECKS`
   (which includes `go test ./cmd/baha/` covering the new `registration_921_test.go`),
   never a directory-wide or prefix exemption. The landed candidate differs from
   this proposed set only if any doc path is dropped; adjust the frozenset to the
   precise 13-file status set of the merged commit.

## sha256 of the four key changed files (read-only, before = origin/develop, after = HEAD)

- `cmd/baha/app_registration_921.go`
  - before (develop): `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` (empty — file added)
  - after (756b22dc): `1ad1c67cd53137ae39f2e289d28ae5d796e02f95367029b1eaaa4c6990adcd98`
- `contracts/machine/v1/control.golden.json`
  - before: `769d4dd1f2dc09e4c0507b57e00b306123c84398da39342fdad7ea76933753f9`
  - after:  `f0f2cf4411921054bd18fb8dcfe9c1f9bc6136fed014abe54e4a201e26debbed`
- `internal/machine/contract.go`
  - before: `62ddf4d2da2aaf084cbc9b2ed522c293057e5e9ffda0ae119370ae3657ced101`
  - after:  `ec9f5c589ca616dcfef1b090457d88e424581f0306b2bce4a1811839892c9597`
- `cmd/baha/mcp_tools.go`
  - before: `891a45a2fcc1f2cf188bcc7261fd4f28ae0398057f576d9353f2701fdd9802ee`
  - after:  `3400a5738f7eff45a4de73bfbc183fb868f277e4220f2f978299c11d1e3d8176`

## Round 2

Scope (read-only): diagnosed why trusted-base `scripts/pr-gate-plan.py` reports
`coverage_status: unresolved` for PR #998 head `756b22dc8967d0daa7393065cd5bc15f262c7050`
despite accepted t-20261016-registration-ci-readiness (#974). No code touched.
Trusted pin examined at `c6db56c4ac077dbc496134cb0871c8e996a593fe`
("ci: stage full native selection and authenticated public contracts regression coverage"
— a trusted-target regression branch, NOT develop; develop is not its ancestor and
it is not an ancestor of HEAD #998).

### (1) Exact changed path set of `origin/develop...756b22dc` (13 files, all statuses)

| # | Path | Status |
| --- | --- | --- |
| 1 | `cmd/baha/app_registration_921.go` | added |
| 2 | `cmd/baha/commands.go` | modified |
| 3 | `cmd/baha/machine_coverage.go` | modified |
| 4 | `cmd/baha/mcp_tools.go` | modified |
| 5 | `cmd/baha/registration_921_test.go` | added |
| 6 | `contracts/machine/v1/control.golden.json` | modified |
| 7 | `docs/cli/command-index.md` | modified |
| 8 | `docs/de/cli/command-index.md` | modified |
| 9 | `docs/de/reference/cli-machine-coverage.md` | modified |
| 10 | `docs/de/reference/mcp.md` | modified |
| 11 | `docs/reference/cli-machine-coverage.md` | modified |
| 12 | `docs/reference/mcp.md` | modified |
| 13 | `internal/machine/contract.go` | modified |

### (2) Registration-binding clauses

- **#974 CI-review exact-match clause** (trusted develop, `scripts/pr-gate-plan.py:264,552-569`):
  `REGISTRATION_CI_REVIEWED_FILES` is a 6-file `{path: status}` set
  (`cmd/baha/registration_ci_readiness_test.go`=added, `scripts/pr-check-runner.py`/`pr-gate-plan.py`/`test_pr_check_runner.py`/`test_pr_gate_plan.py`=modified, `scripts/test_pr_registration_ci.py`=added), bound by
  `REGISTRATION_CI_REVIEWED_HEAD = bb9c713ab0449b0c8243e7535a51b8e43195bf0c` and
  `core752_reviewed_merge_chain(target, head, reviewed)`. Match requires
  `len(paths)==6`, exact set match, exact status match, clean merge chain → `resolved`.
- **#974 `REGISTRATION_COVERAGE` path-area handler** (`scripts/pr-gate-plan.py:136,616-627`):
  `REGISTRATION_COVERAGE = REGISTRATION_REFERENCE_FILES | REGISTRATION_TEST_FILES`
  = exactly the 6 doc-reference files + `cmd/baha/registration_ci_readiness_test.go`.
  Only these paths reach the coverage-resolving area; docs additionally require
  `DOC_CHECKS`.
- **Mixed/unresolved handler** (`scripts/pr-gate-plan.py:644-650`): `commands.go`,
  `machine_coverage.go`, `mcp_tools.go` (and other `cmd/baha/mcp_*.go`) fall here →
  `unresolved: mixed registration/execution source needs independent diff review`.

### (3) Diff of the 998 path/status set against each clause

- vs **#974 exact-match clause**: 13 paths ≠ 6; the 998 set shares only
  `docs/reference/cli-machine-coverage.md`/`mcp.md` (and DE twins) with
  `REGISTRATION_CI_REVIEWED_FILES`, and contains none of the 4 `scripts/*.py`.
  Fails `len(paths)==6`, `set(paths)==set(...)`, `status` match, merge-chain and
  `integration_sha`/`target_base_sha` guards → clause does not fire.
- vs **#974 `REGISTRATION_COVERAGE` path-area**: only the 4 doc reference paths
  (`docs/{,de}/reference/{cli-machine-coverage,mcp}.md`) qualify as
  `registration-reviewed-path` (and get `DOC_CHECKS`). The other 9 paths are not
  in `REGISTRATION_COVERAGE`; 3 (`commands.go`, `machine_coverage.go`, `mcp_tools.go`)
  land in the mixed-unresolved clause; `app_registration_921.go`,
  `registration_921_test.go`, `contracts/machine/v1/control.golden.json` and
  `internal/machine/contract.go` match no coverage handler at all.
- **Root cause:** #974 ("registration-ci-readiness") is the *test/CI bootstrap* —
  its registered coverage set deliberately enumerates only the CI-test file and the
  6 doc references, never the production registration handlers, the golden file or
  the contract registry. #998 is the *production* registration change; its 13-file
  set matches no registered bundle and no clause binds it, so the planner falls
  through to `coverage_status: unresolved`. There is **no** core-921 production
  registration exact-match clause on trusted develop, and the trusted pin
  `c6db56c4` adds none (zero registration clauses).

### (4) Single minimal corrective option

Mirror exactly how #991 (#974) landed: add **one** exact-match clause to trusted
develop `scripts/pr-gate-plan.py`, placed among the other reviewed bundles, before
the broad fallthrough. It must bind the **complete 13-file 998 status set** to the
trusted reviewed head `756b22dc8967d0daa7393065cd5bc15f262c7050`:

```python
CORE_921_PRODUCTION_REVIEWED_HEAD = '756b22dc8967d0daa7393065cd5bc15f262c7050'
CORE_921_PRODUCTION_REVIEWED_FILES = {
    'cmd/baha/app_registration_921.go': 'added',
    'cmd/baha/commands.go': 'modified',
    'cmd/baha/machine_coverage.go': 'modified',
    'cmd/baha/mcp_tools.go': 'modified',
    'cmd/baha/registration_921_test.go': 'added',
    'contracts/machine/v1/control.golden.json': 'modified',
    'docs/cli/command-index.md': 'modified',
    'docs/de/cli/command-index.md': 'modified',
    'docs/de/reference/cli-machine-coverage.md': 'modified',
    'docs/de/reference/mcp.md': 'modified',
    'docs/reference/cli-machine-coverage.md': 'modified',
    'docs/reference/mcp.md': 'modified',
    'internal/machine/contract.go': 'modified',
}
```
clause guard identical to #974's:
`len(paths)==len(CORE_921_PRODUCTION_REVIEWED_FILES) and set(paths)==set(CORE_921_PRODUCTION_REVIEWED_FILES)
and all(ch['status']==expected[ch['path']]) and core752_reviewed_merge_chain(target, head, CORE_921_PRODUCTION_REVIEWED_HEAD)`,
then `require(CI_CHECKS)` (+ `require(DOC_CHECKS)` for the 6 reference docs, matching
the `REGISTRATION_REFERENCE_FILES` rule) and return `coverage_status: resolved`.
This is the same mechanism #991 uses: exact reviewed head hash + complete
changed-status set + clean first-parent merge chain, no directory/prefix exemption.
The alternative (aligning the #998 branch file set down to the #974 test-only set)
would drop the production handlers and is not viable; the CI owner lane must add the
above develop-side clause.

## Verdict

APPROVE
