# MCP reference

BaseHarbor exposes a bounded MCP surface for coding agents.

Start the local MCP server:

```bash
baha mcp serve
```

Discover machine-facing operations with:

```bash
baha agent describe -o json
```

Rules:

- MCP uses the same semantic core as CLI/JSON.
- MCP does not expose a generic shell.
- MCP does not grant unrestricted Docker/Compose access.
- Operations carry safety semantics.
- Policy, ownership, preflight, verification and evidence cannot be bypassed.
- Outputs are secret-safe.

The authoritative operation schema is the versioned machine contract and implementation, not duplicated prose.
