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
