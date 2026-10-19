# BaseHarbor: start here

## Owner-delegated Maintainer execution

Marcel's explicit implementation/acceptance/merge instructions authorize Codex acting through @mcpdev80 to use the existing PR-only review exception without another reviewer, self-approval or repeated confirmation. Follow the central [development process](docs/DEVELOPMENT_GUIDELINES.md#development-process), record Owner-authorized acceptance honestly, and preserve actual Required Checks, evidence and the assigned scope.

BaseHarbor is a Go CLI (`baha`) that provisions, connects and verifies application
infrastructure on the developer's own Targets. Applications declare capabilities;
Target bindings select replaceable providers. Docker/Compose and rootless
Podman/Quadlet are implemented; consult [platform support](docs/reference/platform-support.md)
for current limits. Applications need only selected services and genuine
dependencies, not a compulsory SQL/secrets/identity stack. Optional management
services have their own security requirements.


## Planning transition

Planning lives in [baseharbor-plan](https://github.com/mcpdev80/baseharbor-plan), activated at `47474109ea69475e60be4f065ad17476c71665bb`. Read its `AGENTS.md` and the assigned task note for scope, dependencies, ownership and handoffs. Product engineering rules, PR reviews and authenticated release evidence stay in their existing repositories. Migration-closed issues are historical references, not evidence of implementation.


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
| Planned work and release evidence | [baseharbor-plan](https://github.com/mcpdev80/baseharbor-plan) task/release notes; the task's PR and [internal audits](docs/internal/) for evidence |

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

## CI execution discipline

- Batch related implementation, test and formatting changes into one coherent
  verification candidate. Run available focused local or isolated prechecks
  before pushing; do not use one-file pushes as a CI debugging loop.
- For the same PR, inspect the current candidate's CI result before pushing
  another candidate. An urgent correctness fix or an intentionally superseded
  candidate is an exception: record the reason and preserve the required
  cleanup and evidence. Independent tasks may continue in parallel.
- Start another run only for relevant changed inputs, a diagnosed failure, or
  an explicitly justified qualification step. Resolve queue/concurrency
  blockers before launching replacement runs.
- **As little as possible, as much as necessary.** Always choose the
  smallest, fastest, least expensive set of tests that fully verifies the
  actual changed risks. Prefer focused unit/contract/regression checks and
  bounded smoke tests over heavyweight runtime journeys. Before choosing an
  expensive gate (especially Backup/Restore), state what it uniquely proves
  and why cheaper checks are insufficient. Do not trade away required coverage,
  safety, runner isolation, authenticated evidence or fail-closed checks.
- Select checks from actual impact. Run targeted native runtime gates only when
  needed to prove uncovered behavior. Unknown impact must block explicitly;
  never automatically launch the complete release matrix for a PR.
- Reuse successful evidence only when the existing verifier proves compatible
  inputs. Label deliberate negative pilots and their expected failure.
- Repeated pushes that unnecessarily cancel running checks are a process
  defect. Do not hide the churn by disabling required checks or cancellation
  globally. Record the tested SHA, run/attempt, scope and outcomes in the
  authoritative task handoff.

## Release tasks

For any readiness request, apply the [canonical release audit](docs/internal/pre-release-documentation-audit.md):
readiness covers the complete ecosystem, contracts, EN/DE docs, Pages candidate,
release notes, immutable pins and authenticated evidence. Missing, failed,
skipped or unverified required checks remain open.
Pre-release-ready excludes final complete pre-release approval and promotion.
Preparation alone does not authorize that full run, merge, tag, publication or
Pages deployment. Reuse green evidence only when the verifier proves unchanged
inputs; rerun affected gates.

