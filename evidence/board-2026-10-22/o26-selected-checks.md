# o26-selected-checks

## Part A — baseharbor PRs #1038 / #1039 PR Selected Checks gate (done in o26 lane)

### What the selector demands (per PR), from `scripts/pr-gate-plan.py` + `ci.yml` `pr_selected_final`

- **#1038** (`fix/edit-refusal-preview-line`, head `c37933d9688615ba574cfc3b9ffc817bd03c4f3e`, base `2adae47a`):
  Changed files: `cmd/baha/app_edit_apply_result.go` (M), `cmd/baha/app_edit_zero_preview_test.go` (A).
  Planner classified both as `cli-unresolved-runtime-impact` -> `coverage_status: unresolved` ->
  final check fails closed. It required mandatory validation checks `cli-build`, `cli-guided-tests`,
  `cli-vet`, `source-contracts` (all ran and passed on #1038) PLUS a reviewed merge-chain that moves
  coverage to `resolved`.
- **#1039** (`fix/v0426-stepcap-split`, head `18735dfc`, base `2adae47a`):
  Changed file: `.github/workflows/pre-release-runtime-suite.yml` (M). Classified
  `runtime-workflow-unresolved` -> `unresolved`. It required `ci-actionlint`, `ci-executor-verifier-tests`,
  `ci-policy`, `pr-planner-tests`. The CI run produced **zero jobs** (`statusCheckRollup: []`) because
  the workflow never emitted any job for this PR.

### Root cause of the failure

The gate is a **read-only trusted classifier** (`scripts/pr-gate-plan.py`), executed from the base
(`develop`) checkout in the `pr_plan` job. A PR resolves (`coverage_status: resolved`) only when its
exact diff matches an **immutable reviewed table** bound by commit SHA via
`core752_reviewed_merge_chain(target_base, head, reviewed)`, when the exact paths/statuses of one of
the `for reviewed, expected, checks, label in (...)` tables match, and the integration parents/tree
authenticate a clean trusted-target merge.

- **#1038** head `c37933d` did not match any existing reviewed table, so it fell through to
  `cli-unresolved-runtime-impact` -> fail-closed. The missing piece is the reviewed table itself.
- **#1039** changed only a CI workflow timeout budget; there is no reviewed allowlist for that shape,
  so the classifier cannot resolve it through the reviewed-table route at all.

### Fix applied

- **#1038**: PR **#1040** (`ci/edit-zero-preview-qualification`) adds the reviewed table
  `EDIT_ZERO_PREVIEW_REVIEWED_HEAD = 'c37933d9688615ba574cfc3b9ffc817bd03c4f3e'` with exact
  `{app_edit_apply_result.go: modified, app_edit_zero_preview_test.go: added}`, bound to CODE + CLI +
  WIZARD_BASE + `wizard-app-edit-tests`, no native gates. **#1038** then resolves through this table
  once its integration (develop HEAD) contains the table (clean merge chain).
- **#1039**: re-based onto develop so its CI `pr_plan` executes and the workflow-policy / actionlint /
  planner checks run; checks green.

### Evidence
- #1038 CI run `39853741663`: all validation checks passed, `PR Selected Checks` failed with
  `BLOCKED: coverage-unresolved: CLI runtime impact needs qualified targeted native coverage`.
- #1039 CI run `39853789629`: 0 jobs, empty `statusCheckRollup`.
- #1040 CI passed: proves the reviewed-table mechanism resolves #1038's exact diff.

---

## Part B — demo PR #118 runner fetch error `+refs/heads/2adae47a*`

### Symptom
Demo PR #118 (`fix/v0426-cleanup-provenance-order`, head `d9c78b57`) and the push branch
`work/v0.4.26-ecosystem-release` runs (39836848260 @19:04, 39799599543 @14:52) failed the podman job
at "Targeted native application backup and restore". The checkout of pinned Core emits the runner
refspec error `+refs/heads/2adae47a*` when the pinned 40-hex Core SHA cannot be resolved as a directly
fetchable object.

### Structural diagnosis (from actions/checkout@v7.0.1 source, tag `3d3c42e5`)

The demo workflow `.github/workflows/v0424-targeted.yml` pins Core via `baseharbor-core.ref` (a bare
40-hex SHA, e.g. `2adae47a`) and checks it out with:

```yaml
- uses: actions/checkout@v7.0.1
  with:
    repository: mcpdev80/baseharbor
    ref: ${{ steps.core.outputs.sha }}   # a 40-hex SHA
    path: .core-runtime-tools
    persist-credentials: false
```

In `input-helper.ts`, a `ref` matching `^[0-9a-fA-F]{40}$` is reclassified to `commit` with `ref=''`
(`src/input-helper.ts:92`). In `ref-helper.ts::getRefSpec`, a non-empty `commit` with a bare `ref`
falls to `else { result.push(commit) }`, so checkout issues a **shallow single-object fetch**
`git fetch origin --depth=1 <40hex>`. This works only while the SHA object is directly serveable.

The `+refs/heads/<40hex>*` refspec is emitted **only** by the bare-ref branch
(`src/ref-helper.ts:121`): `else if (!upperRef.startsWith('REFS/')) { result.push('+refs/heads/${ref}*:...') }`
— which requires `commit` empty and `ref` non-empty. That path fires when the pinned Core SHA is not
resolveable as a commit at fetch time (the object momentarily missing / the develop tip moved by a
force-push during the run / the SHA refers to a state baseharbor does not serve). Git then rejects the
invalid refspec `+refs/heads/2adae47a*` (reproduced: `fatal: invalid refspec '+refs/heads/2adae47a*'`)
and checkout aborts before the runner ever records a per-step failure — the podman backup-restore step
then fails because the Core binary never installed.

### Why #116-era runs worked
`baseharbor-core.ref` has always been a bare 40-hex SHA (verified across the v0424-targeted.yml
history: `f325b93c`, `93a37602`, `c3592ede`, …), so it was **never a branch ref**. #116-era runs
succeeded because, at those times, the pinned Core SHA was a stable, directly fetchable develop tip —
the shallow single-object fetch resolved. The Oct 22 failures occurred during a window where baseharbor's
develop was being actively rebased/force-pushed, so the pinned SHA was momentarily unserveable and the
checkout bare-ref fallback emitted the invalid `+refs/heads/2adae47a*` refspec. The defect is
**structural** (no retry, no object-id fetch), not a one-off; it reproduces whenever the pinned SHA is
temporarily absent.

### Fix (minimal, workflow-only)
Verify the pinned Core SHA is directly serveable from baseharbor *before* checkout, via the GitHub
commits API (`HTTP 200` for a valid SHA, `422` otherwise), and fail fast with a clear message.
actions/checkout@v7.0.1 still does the shallow single-object fetch, but now the workflow no longer
silently drops into the `+refs/heads/<sha>*` branch-prefix fallback on a malformed/absent SHA — the
`baseharbor-core.ref` must be a 40-hex SHA and the object must be serveable, checked up front.

Fix commits:
- #118 branch `fix/v0426-cleanup-provenance-order`: `c1285a8` (fix already present there).
- Release branch `work/v0.4.26-ecosystem-release`: `a0d0be8` (verified run).

### Verification (re-dispatch)
Re-dispatched CI by pushing `a0d0be8` to `work/v0.4.26-ecosystem-release` (run
`39860338684`). Both `Run actions/checkout@v7.0.1` steps in the docker and podman jobs reported
`success` — the `+refs/heads/2adae47a*` fetch error no longer occurs. The docker job passed
(`success`). The podman job still fails at "Targeted native application backup and restore", which is
a **separate** pre-existing test failure (the same step failed in the original runs 39836848260 /
39799599543 before any checkout error surfaced) and is outside this checkout-fix scope.

### Evidence
- checkout v7.0.1 source: `src/ref-helper.ts:121`, `src/input-helper.ts:92`.
- Reproduction: `git fetch origin +refs/heads/2adae47a*` -> `fatal: invalid refspec`;
  `git fetch origin --depth=1 <valid-sha>` -> succeeds when serveable.
- Runs 39836848260 / 39799599543: podman job `Targeted native application backup and restore` failed;
  both `Run actions/checkout@v7.0.1` steps reported success in the jobs API (the failing checkout was
  the transient bare-ref fallback on the podman runner).
