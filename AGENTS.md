# BaseHarbor: start here

BaseHarbor is a Go CLI (`baha`) that provisions, connects and verifies application
infrastructure on the developer's own Targets. Applications declare capabilities;
Target bindings select replaceable providers. Docker/Compose and rootless
Podman/Quadlet are implemented; consult [platform support](docs/reference/platform-support.md)
for current limits. Applications need only selected services and genuine
dependencies, not a compulsory SQL/secrets/identity stack. Optional management
services have their own security requirements.


## Planning transition

Planning lives in [baseharbor-plan](https://github.com/mcpdev80/baseharbor-plan), activated at `47474109ea69475e60be4f065ad17476c71665bb`. Read its `AGENTS.md` and the assigned task note for scope, dependencies, ownership and handoffs. Product engineering rules, PR reviews and release test results stay in their existing repositories. Migration-closed issues are historical references, not evidence of implementation.


## Read according to your task

Before implementation, read [development guidelines](docs/DEVELOPMENT_GUIDELINES.md).
Use these existing sources instead of duplicating their contents:

| Need | Source |
| --- | --- |
| Product and developer journey | [README](README.md), [getting started](docs/tutorials/getting-started.md) |
| Architecture and rationale | [Architecture](docs/explanation/architecture.md), [decisions](docs/decisions/) |
| Application intent and provider boundaries | [Application contract](docs/explanation/application-contract.md), [providers](docs/explanation/providers.md), [Targets](docs/explanation/targets.md) |
| Normative and machine contracts | [Specifications](docs/spec/README.md), [schemas](spec/), [CLI](docs/reference/cli.md), [MCP](docs/reference/mcp.md) |
| Contribution and documentation rules | [Contributing](CONTRIBUTING.md), [documentation style](docs/STYLE_GUIDE.md) |
| Planned work and release checks | [baseharbor-plan](https://github.com/mcpdev80/baseharbor-plan) task/release notes; the task's PR and [internal audits](docs/internal/) for results |

## Find the implementation

- `cmd/baha/`: CLI commands and lifecycle orchestration.
- `internal/application*/`: application intent, state and lifecycle.
- `internal/providers/`, `internal/runtime/`, `internal/deployment/`: providers, runtime realization and Targets.
- `contracts/`, `spec/`: shared contracts and schemas.
- `deploy/`: runtime assets; `scripts/` and `.github/workflows/`: validation and automation.

The ecosystem also includes [Demo](https://github.com/mcpdev80/baseharbor-demo),
[Console](https://github.com/mcpdev80/baseharbor-console) and
[Node Connector](https://github.com/mcpdev80/baseharbor-node-connector).
Check affected consumers against actual source pins when changing shared contracts.

## Work and verify

Check the actual branch, remote HEAD, task scope and existing PR before editing.
For an existing shared task, continue its branch/PR; preserve other sessions'
commits and integrate remote changes before pushing. Checkpoints are leads;
confirm their claims against current source, comments and results.

Several sessions often run in parallel; follow the
[development process](docs/DEVELOPMENT_GUIDELINES.md#development-process) for
ownership, Maintainer approval and multi-session rules.

Keep runtime, capability and delivery separate. Mutations follow
plan → preflight → apply → verify; CLI, JSON and MCP share domain semantics.
Preserve ownership, concrete consequence/consent checks and secret protection.

Start with targeted local tests. Broaden checks as the change requires:

```bash
go test ./...
go vet ./...
go build -o /tmp/baha ./cmd/baha
```

Use the toolchain from [go.mod](go.mod) and the relevant CI workflow.
For docs, follow [style](docs/STYLE_GUIDE.md) and run
`bash scripts/documentation-audit.sh`; for Go layout,
`bash scripts/source-readability-audit.sh`.
Native integration tests require isolated, verified runtimes; inspect their
workflow and explicit prerequisites before enabling them.

## CI checks and commit receipts

CI, Pre-Release and Release run ordinary required jobs. Their dependency results determine success; missing, failing or cancelled required work blocks. There is no CI gate-manifest collection, completed-run/job re-verification, evidence reuse, receipt approval or shadow chain. Prior successful CI jobs are never acceptance evidence or merge receipts; an ephemeral same-PR scheduling optimization may omit unchanged successful checks only within 24 hours after authenticated ancestry, workflow/toolchain/test-definition identity, and dependency-aware invalidation checks. Missing or ambiguous proof always schedules the normal full selection. After an authorized develop merge the sole CI record contains merge SHA and PR number; see [minimal CI receipts](docs/development/minimal-ci-receipts.md).

Rulesets remain disabled during the current CI repair. Do not claim live Required Check enforcement until separately verified and enabled. This change authorizes no release or tag. Product audit/recovery records, source pinning, artifact checksums and build provenance retain their own semantics.

## W143 lane identity
worker: w143-worker (w143@baseharbor.local). NOT w135-worker.

## Second basis note (2026-11-04, coordinator f49a3303): merge develop into this lane
develop HEAD = e444ae117173dcf84e7aa8b17f73c020d3e2923c (kodierung verbunden).
This lane's branch (work/w96-production-wiring-f8ef4255) is OLDER than develop.
Action in the SAME run: `git fetch origin develop` then `git merge origin/develop`
into this lane's own branch (no force-transfer). CONFLICT RESOLUTION: in any
conflict under contracts/managementaccess/v1 or the three reviewed database
integration tests, take the develop version (develop wins). A conflict that is
provably non-resolvable per this rule => STOP + MELDEN with file:line.
The base mission stays unchanged; this note only extends it.
Prove ancestry with `git merge-base --is-ancestor origin/develop HEAD`.

## CI execution discipline

Ordinary branch pushes do not start CI. Draft PRs run workflow policy, source
formatting and readability checks. Marking a PR ready and subsequent pushes run
its selected checks. Bundle local commits into coherent pushes. Merging into
`develop` writes the small receipt without a duplicate Source & Quality run.
Explicit validation branches and manual validation remain available.

- Batch related implementation, test and formatting changes into one coherent
  verification candidate. Run available focused local or isolated prechecks
  before pushing; do not use one-file pushes as a CI debugging loop.
- For the same PR, inspect the current candidate's CI result before pushing
  another candidate. An urgent correctness fix or an intentionally superseded
  candidate is an exception: record the reason and preserve the required
  cleanup and test results. Independent tasks may continue in parallel.
- Start another run only for relevant changed inputs, a diagnosed failure, or
  an explicitly justified qualification step. Resolve queue/concurrency
  blockers before launching replacement runs.
- **As little as possible, as much as necessary.** Always choose the
  smallest, fastest, least expensive set of tests that fully verifies the
  actual changed risks. Prefer focused unit/contract/regression checks and
  bounded smoke tests over heavyweight runtime journeys. Before choosing an
  expensive gate (especially Backup/Restore), state what it uniquely proves
  and why cheaper checks are insufficient. Do not trade away required coverage,
  safety, runner isolation, actual test execution or fail-closed checks.
- Select checks from actual impact. Run targeted native runtime gates only when
  needed to prove uncovered behavior. Valid paths without a narrow mapping select
  broad Go unit/vet/build checks and applicable Docker/Podman lifecycles. Invalid
  source/diff/catalog data still fails; never automatically launch the complete
  release matrix for a PR.
- Run affected checks directly. Label deliberate negative pilots and their expected failure.
- Repeated pushes that unnecessarily cancel running checks are a process
  defect. Do not hide the churn by disabling required checks or cancellation
  globally. Record the tested SHA, run/attempt, scope and outcomes in the
  authoritative task handoff.

## Release tasks

For any readiness request, apply the [canonical release audit](docs/internal/pre-release-documentation-audit.md):
readiness covers the complete ecosystem, contracts, EN/DE docs, Pages candidate,
release notes, immutable pins and actual test results. Missing, failed,
skipped or unverified required checks remain open.
Pre-release-ready excludes complete pre-release validation and promotion.
Preparation alone does not authorize that full run, merge, tag, publication or
Pages deployment. Run the required checks for the selected immutable inputs.

