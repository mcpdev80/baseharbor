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

## Semantic tools

The current surface is intentionally small:

```text
baseharbor.target
baseharbor.inspect
baseharbor.plan
baseharbor.apply
baseharbor.status
baseharbor.doctor
baseharbor.observe
baseharbor.update
baseharbor.repair
baseharbor.backup
baseharbor.restore
baseharbor.destroy
baseharbor.policy.check
baseharbor.policy.explain
```

`baseharbor.target` reports the effective Target plus repository-resolved application/environment context. It accepts an optional target selector and returns the same target semantics as `baha target -o json`.

These are BaseHarbor lifecycle operations, not wrappers around CLI commands.

`baseharbor.apply` is the semantic converge operation rather than a 1:1 mirror of every human CLI alias.

`baseharbor.destroy` is destructive and requires explicit approval.

Backup and restore accept an owner-only local password-file reference. Plaintext backup passwords are not accepted as MCP arguments.

## Rules

- MCP uses the same semantic lifecycle as CLI/JSON.
- MCP is local stdio in this release.
- MCP does not expose a generic shell.
- MCP does not grant unrestricted Docker, Compose, Podman or provider-native execution.
- Operations carry read-only, mutating or destructive safety semantics.
- Policy, ownership, preflight, reconciliation, verification and secure bindings cannot be bypassed.
- Operations do not prompt interactively; unresolved choices return typed actionable results.
- Outputs are secret-safe.
- Runtime-specific realization details do not become MCP semantics.

The authoritative operation and safety schema is the versioned machine contract returned by `baha agent describe -o json` and implemented by the typed machine operation registry.
