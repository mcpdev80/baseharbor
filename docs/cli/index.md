# Command-line interface

The `baha` CLI is BaseHarbor's primary human interface.

This documentation follows the actual command surface and groups commands by responsibility instead of presenting one undifferentiated command dump.

## Start here

For the normal application workflow, begin with:

```text
baha app new
baha app init
baha up
baha status
baha doctor
baha down
```

## CLI sections

- [Core workflow](core.md) — start, stop, inspect, plan, diagnose and destroy.
- [Applications](applications.md) — create, adopt and operate applications.
- [Targets](targets.md) — deployment targets and runtime selection.
- [Providers](providers.md) — bundled, external/BYO and provider authoring.
- [Development](development.md) — Stack Profiles, workspace and developer access.
- [Organization and policy](organization-policy.md) — organization defaults, configuration and policy.
- [Security and trust](security-trust.md) — trust, login and connection security.
- [Automation and agents](automation-agents.md) — JSON, MCP and agent-facing surfaces.
- [Shell and UX](shell-ux.md) — completion, shell integration, prompt and TUI.
- [Global options](global-options.md) — options accepted across commands.

## Documentation contract

Each command section should answer the same questions:

1. What does this command do?
2. When should I use it?
3. What is the syntax?
4. Which options materially change behavior?
5. Is it read-only, mutating or destructive?
6. How does non-interactive use differ from interactive use?
7. Is there equivalent JSON/MCP behavior?
8. What are the common failure modes?
9. What is the safest next action?
