# Contributing to BaseHarbor

Thanks for helping improve BaseHarbor.

This page is intentionally short. It gives you the fastest path into the project and links to the authoritative documentation instead of duplicating it.

## Start here

Before changing code, read:

- [Project overview](README.md)
- [Getting started](docs/tutorials/getting-started.md)
- [Architecture](docs/explanation/architecture.md)
- [Development guidelines](docs/DEVELOPMENT_GUIDELINES.md)
- [Documentation style](docs/STYLE_GUIDE.md)
- [Architecture decisions](docs/decisions/)
- [Normative specifications](docs/spec/README.md)
- [Roadmap](docs/roadmap.md)

Looking for something to work on?

- [Open issues](https://github.com/mcpdev80/baseharbor/issues)
- [Pull requests](https://github.com/mcpdev80/baseharbor/pulls)

## Development setup

BaseHarbor currently uses Go 1.25.

```bash
git clone https://github.com/mcpdev80/baseharbor.git
cd baseharbor
git switch develop
go test ./...
```

For normal development, prefer targeted local tests for the area you changed. Use the full repository test suite and GitHub Actions where required by the change or release gate.

## Repository map

```text
cmd/        CLI entry points
internal/   Core implementation
spec/       Machine-readable contracts and schemas
deploy/     Runtime and deployment assets
docs/       Human documentation, specs and ADRs
scripts/    Project automation
.github/    CI and repository workflows
```

## Contribution workflow

1. Start from `develop`.
2. Create a focused branch.
3. Understand the existing architecture and implementation before adding new concepts.
4. Keep the change small and scoped.
5. Add or update tests that prove the relevant behavior.
6. Update documentation when behavior, contracts or workflows change.
7. Open a pull request against `develop`.

Normal feature, fix, chore and dependency pull requests target `develop`. The `main` branch represents released source.

### Production hotfixes

Production hotfixes are the exception to the normal `develop` flow.

- Start from the released `main` line, not from in-progress `develop`.
- Use a focused `hotfix/<version-or-issue>-<slug>` branch.
- Keep the change defect-only and link it to the hotfix release parent issue.
- Run targeted tests while implementing; do not repeatedly run the full release matrix.
- Update affected docs plus changelog/release notes.
- Run the complete pre-release validation once against the final hotfix candidate.
- Merge the proven hotfix to `main`, publish the immutable hotfix tag/runtime image, verify artifacts, then forward-port the same fix to `develop`.
- If the proposed correction changes runtime identity, migration semantics or architecture beyond the released defect, move it to normal roadmap work instead.

The authoritative release/hotfix procedure is [Releases and versioning](docs/reference/releases.md).

## Core rules

- Prefer the smallest correct design.
- Keep runtime, capability and delivery provider axes separate.
- Applications declare capabilities, not infrastructure products.
- Security-sensitive behavior must fail closed.
- Secrets must never enter logs, errors, metrics, machine output or committed manifests.
- Planning and preflight must not mutate state.
- CLI, JSON and MCP must share the same semantic core.
- Do not duplicate authoritative contracts across multiple documents.
- Avoid unrelated refactors and speculative abstractions.
- Remove obsolete compatibility or legacy code instead of extending it without a current requirement.

The detailed rules live in [Development guidelines](docs/DEVELOPMENT_GUIDELINES.md).

## Where to look before changing something

Changing architecture or lifecycle behavior?

- Read [Architecture](docs/explanation/architecture.md)
- Check [Architecture decisions](docs/decisions/)

Changing application contracts or provider semantics?

- Read [Application contract](docs/explanation/application-contract.md)
- Check [Normative specifications](docs/spec/README.md)

Changing deployment targets or runtime behavior?

- Read [Targets and deployment destinations](docs/explanation/targets.md)
- Check the relevant runtime/provider documentation under [docs](docs/)

Changing CLI behavior?

- Read [CLI reference](docs/reference/cli.md)
- Keep machine-readable output and MCP semantics aligned

Changing documentation?

- Follow [Documentation style](docs/STYLE_GUIDE.md)
- Keep one authoritative source of truth

## Pull requests

A good pull request should explain:

- what changed;
- why it changed;
- how it was verified;
- whether contracts, compatibility, security or user-facing behavior changed.

Keep PRs reviewable. If a change requires a new architectural primitive, make that explicit instead of hiding it inside an unrelated implementation.

## License

By contributing, you agree that your contributions are licensed under the repository's [Apache License 2.0](LICENSE).
