# Automation and agents

BaseHarbor keeps human CLI, JSON and MCP as separate interfaces over the same domain lifecycle.

## Structured output

Commands that support machine-readable results use:

```text
-o json
--output json
```

Global non-interactive behavior uses:

```text
--no-input
--non-interactive
```

Automation must never depend on parsing decorative human output.

## Agent interface

```text
baha agent describe
```

Use agent description/discovery to inspect the supported semantic machine surface.

## MCP

```text
baha mcp serve
```

The MCP server uses stdio and exposes BaseHarbor semantic operations. It does not expose a generic shell or generic Docker/Podman execution.

## Standalone serve

The root command tree also contains `baha serve` for the supported local service surface.

See [MCP / agent interface](../reference/mcp.md) and [Machine Interface v1](../spec/machine-interface-v1.md).

## Example: inspect an application without terminal prompts

Inside an adopted or generated application repository:

```bash
baha --no-input app inspect . -o json > inspection.json
baha --no-input app plan -o json > plan.json
baha --no-input agent describe -o json > agent-tools.json
```

Parse these JSON documents in your automation, and check each command's exit status before using its output. `--no-input` fails with an actionable error when required decisions are missing; it does not silently approve mutation.

## Example: launch the MCP server from a client

Use the standard local stdio server configuration in a client that supports this format, adapting the executable path to your installation:

```json
{
  "mcpServers": {
    "baseharbor": {
      "command": "baha",
      "args": ["mcp", "serve"]
    }
  }
}
```

The client launches `baha` and performs MCP initialization and `tools/list`. Discover the actual registered semantic tools before calling them. A CLI command name is not automatically an MCP tool name; complete CLI/JSON/MCP coverage is tracked for v0.4.22 in [issue #787](https://github.com/mcpdev80/baseharbor/issues/787).
