# Contributing to BaseHarbor Console

Thanks for helping improve the BaseHarbor Console.

The Console is a projection of BaseHarbor Core, not a second orchestration system.


## Planning

Planning lives in [baseharbor-plan](https://github.com/mcpdev80/baseharbor-plan). Read its [AGENTS.md](https://github.com/mcpdev80/baseharbor-plan/blob/main/AGENTS.md) and your assigned task before working. Claim ownership first; update and push the task note for status, blockers, PR/source SHA, verification and handoff. Source GitHub issues retain discussion/history only; migrated closure does not mean implementation or acceptance.


## Start here

Before changing code, read:

- [Project overview](README.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Normative UX/design system](docs/UX-DESIGN-SYSTEM.md)
- BaseHarbor Core issues #767, #768, #769 and #770 when touching machine/API/runtime/auth boundaries

## Development setup

Requirements:

- Node.js 24.14+
- npm

```bash
npm install
npm run check
npm run dev
```

For normal development, prefer focused local/Hugging Face checks for the area you changed. Use GitHub Actions as the PR/release proof rather than repeatedly consuming CI while iterating.

## Architecture rules

- CLI, JSON, MCP, HTTP and Console must project the same BaseHarbor Core semantics.
- Do not create Console-owned deployment state.
- Do not implement separate Console RBAC or policy.
- Do not duplicate the secret store.
- Do not call Docker, Podman, Kubernetes, OpenShift or the Node Connector directly from the browser.
- Do not parse CLI prose or shell out to `baha`.
- Do not invent provisional machine API paths before #767 is finalized.
- HTTP and stream locations must come from the finalized Core contract/discovery surface.
- Runtime Provider and Target Access Provider remain separate.
- Destructive actions must preserve SafetyClass, policy, confirmation, ownership and audit semantics.
- Fixture mode is UI-development-only and must never become authoritative state.

Run the architecture guard before submitting changes:

```bash
npm run check:architecture
```

## UX rules

All UI changes must follow `docs/UX-DESIGN-SYSTEM.md`.

In particular:

- task-first and context-first;
- structured lists/tables for comparable resources;
- avoid card-wall dashboards;
- status uses icon/text as well as color;
- complex dependent workflows use the shared wizard patterns;
- final review before complex mutation;
- no clickable controls that pretend a Core action is available when its contract is still pending;
- target WCAG 2.2 AA.

## Contribution workflow

1. Start from `main` while this standalone repository remains pre-release.
2. Create a focused branch.
3. Inspect the existing architecture before adding a concept.
4. Keep the change scoped.
5. Run targeted checks while iterating.
6. Run `npm run check` before opening a pull request.
7. Update documentation when behavior or contracts change.
8. Open a pull request against `main`.

## Pull requests

A good pull request explains:

- what changed;
- why it changed;
- how it was verified;
- whether UX, security, contracts or user-facing behavior changed;
- whether any part is intentionally blocked on a BaseHarbor Core contract.

## License

By contributing, you agree that your contributions are licensed under the repository's [Apache License 2.0](LICENSE).

