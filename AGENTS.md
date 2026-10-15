# BaseHarbor: start here

BaseHarbor is a Go CLI (`baha`) that provisions, connects and verifies application
infrastructure on the developer's own Targets. Applications declare capabilities;
Target bindings select replaceable providers. Docker/Compose and rootless
Podman/Quadlet are implemented; consult [platform support](docs/reference/platform-support.md)
for current limits. Applications need only selected services and genuine
dependencies, not a compulsory SQL/secrets/identity stack. Optional management
services have their own security requirements.

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
| Planned work and release evidence | [Roadmap](docs/roadmap.md), the task's issue/PR and latest comments, [internal audits](docs/internal/) |

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

Several sessions often run in parallel. Follow
[Development guidelines → Development process](docs/DEVELOPMENT_GUIDELINES.md#development-process):
one assigned issue at a time, one dedicated branch and PR per task, record
ownership before editing, respect task file boundaries and integrated
dependencies, preserve other sessions' changes, and stop after implementation,
verification and PR handoff. The **Maintainer** (currently `@mcpdev80`) accepts
the result and explicitly assigns the next issue. Report a blocking dependency
rather than switching tasks autonomously.

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

## Release tasks

For any readiness request, apply the [canonical release audit](docs/internal/pre-release-documentation-audit.md):
readiness covers the complete ecosystem, contracts, EN/DE docs, Pages candidate,
release notes, immutable pins and authenticated evidence. Missing, failed,
skipped or unverified required checks remain open.
Pre-release-ready excludes final complete pre-release approval and promotion.
Preparation alone does not authorize that full run, merge, tag, publication or
Pages deployment. Reuse green evidence only when the verifier proves unchanged
inputs; rerun affected gates.
