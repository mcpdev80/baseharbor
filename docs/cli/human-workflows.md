# Human CLI — v0.4.24

The default human CLI is **task-first**. Most developer operations work in the current application repository; advanced operators may use the explicit `app`, `target`, `provider`, `stack`, `config` and `mcp` command groups.

## Daily workflow

| Goal | Command |
| --- | --- |
| Create an application or another object | `baha new` |
| Generate a new application non-interactively | `baha new application NAME --stack go` |
| Adopt a repository | `baha init` |
| Start an application and required Core | `baha up` |
| Stop the current application, preserving persistent data | `baha down` |
| Show current state | `baha status` |
| Inspect contract and detected resources | `baha inspect` |
| Preview desired changes | `baha plan` |
| Diagnose problems | `baha doctor` |
| List managed applications | `baha list` |
| Back up / restore | `baha backup` / `baha restore` |
| Remove owned application state | `baha destroy --yes` |

Outside an application repository, `down` and `destroy` operate at the selected installation/target boundary. **`baha destroy --all` is installation-wide** and must be reviewed separately. `baha down` does not delete application persistent data.

Use `baha --help` or `baha COMMAND --help`; developer tasks appear first, and specialist operator namespaces remain discoverable.

## Creating targets

`baha new target` guides a local Docker/Podman target through name, runtime, scope and default selection. It supports `back` and `cancel`, then requires an explicit final confirmation. For automation or remote targets, use `baha target create` with explicit access provider and reference.

## Updates

`baha update --check` inspects published BaseHarbor releases and compares the installed owned Core providers with the selected immutable release catalog. JSON reports `core_plan` and any `core_inspection_error`; unknown inventory is not assumed supported.

`baha update --yes` updates BaseHarbor itself and reconciles only admitted Core provider changes after verified backup, ownership and recovery checks. PostgreSQL major changes and unsupported HA transitions remain excluded.

Use `baha app update` for application source/Git updates. It is a different operation from BaseHarbor/Core updates. See the [Core provider update specification](../spec/core-provider-update-v1.md) for exact admission and recovery boundaries.

## Automation and machine clients

Do not parse the human-oriented terminal display. Use `-o json` where supported, or the same typed operations through MCP/HTTP. The [CLI/machine coverage inventory](../reference/cli-machine-coverage.md) distinguishes semantic commands, human presentation and explicitly excluded host operations.
