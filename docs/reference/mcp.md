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


## Client permissions

A safe default is to allow inspection/planning tools and keep every mutating lifecycle tool behind explicit client approval.

For clients that normalize the server name `baha` plus MCP tool dots into underscore-separated permission keys, an example is:

```json
{
  "permission": {
    "baha_*": "ask",
    "baha_baseharbor_target": "allow",
    "baha_baseharbor_status": "allow",
    "baha_baseharbor_inspect": "allow",
    "baha_baseharbor_plan": "allow",
    "baha_baseharbor_doctor": "allow",
    "baha_baseharbor_observe": "allow",
    "baha_baseharbor_policy_check": "allow",
    "baha_baseharbor_policy_explain": "allow"
  }
}
```

Permission-key syntax is client-specific. The behavioral rule is not: read-only understanding/planning/verification may be allowed automatically, while `apply`, `update`, `repair`, `backup`, `restore` and `destroy` remain approval-gated.

`baseharbor.inspect` is read-only but can inspect repository paths or Git URLs. Environments with stricter information-boundary requirements may therefore keep it on `ask` even though it does not mutate state.

`baseharbor.destroy` additionally requires BaseHarbor's explicit destructive approval contract; client permission alone is not sufficient.

## Lifecycle timeouts

Mutating lifecycle operations are semantic converge operations and can legitimately take longer than a typical MCP request, especially when images must be pulled or built, providers need readiness time, or backup/restore is involved.

Do not configure a very short client timeout such as 30 seconds. Use at least 180 seconds for realistic application work; 300 seconds is the recommended general-purpose starting point.

Example client setting:

```json
{
  "mcp": {
    "baha": {
      "timeout": 300000
    }
  }
}
```

BaseHarbor v0.4.15.1 detaches an accepted mutating lifecycle convergence from client-request cancellation so a client timeout does not intentionally truncate the mutation half-way through. A larger client timeout is still recommended so the caller receives the verified final result rather than having to query status after its own request timeout.

Safety model:

```text
understand / plan / verify
        ↓
mutation requires approval
        ↓
destruction requires explicit BaseHarbor approval too
```
