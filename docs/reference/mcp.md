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

The complete current registry is checked against actual MCP discovery by the source tests. The table is generated from the typed operation metadata; handwritten partial tool lists are not authoritative.

<!-- BEGIN GENERATED MCP REGISTRY -->

| Tool | Safety | Policy required | Approval required |
| --- | --- | --- | --- |
| `baseharbor.installation.destroy` | `destructive` | false | true |
| `baseharbor.release.check` | `read_only` | false | false |
| `baseharbor.control-plane.status` | `read_only` | false | false |
| `baseharbor.control-plane.doctor` | `read_only` | false | false |
| `baseharbor.control-plane.up` | `mutating` | false | false |
| `baseharbor.control-plane.stop` | `mutating` | false | false |
| `baseharbor.control-plane.repair` | `mutating` | false | false |
| `baseharbor.control-plane.destroy` | `destructive` | false | true |
| `baseharbor.openbao.status` | `read_only` | false | false |
| `baseharbor.openbao.bootstrap` | `mutating` | false | false |
| `baseharbor.openbao.unseal` | `mutating` | false | false |
| `baseharbor.openbao.rotate` | `mutating` | false | false |
| `baseharbor.dev.domain` | `mutating` | false | false |
| `baseharbor.dev.credentials` | `mutating` | false | false |
| `baseharbor.app.environment` | `read_only` | false | false |
| `baseharbor.app.connection` | `read_only` | false | false |
| `baseharbor.connectivity.list` | `read_only` | false | false |
| `baseharbor.connectivity.connect` | `mutating` | false | false |
| `baseharbor.connectivity.disconnect` | `mutating` | false | false |
| `baseharbor.operator.identity` | `read_only` | false | false |
| `baseharbor.provider.init` | `mutating` | false | false |
| `baseharbor.provider.test` | `read_only` | false | false |
| `baseharbor.workspace.show` | `read_only` | false | false |
| `baseharbor.app.show` | `read_only` | false | false |
| `baseharbor.app.create` | `mutating` | false | false |
| `baseharbor.app.adopt` | `mutating` | false | false |
| `baseharbor.app.configure` | `mutating` | false | false |
| `baseharbor.runtime-identity.rotate` | `mutating` | false | true |
| `baseharbor.runtime-identity.revoke` | `destructive` | false | true |
| `baseharbor.tls.update` | `mutating` | false | false |
| `baseharbor.trust.status` | `read_only` | false | false |
| `baseharbor.trust.export` | `mutating` | false | false |
| `baseharbor.trust.install` | `mutating` | false | true |
| `baseharbor.app.stop` | `mutating` | false | false |
| `baseharbor.app.preflight` | `read_only` | false | false |
| `baseharbor.secret.list` | `read_only` | false | false |
| `baseharbor.secret.set` | `mutating` | false | false |
| `baseharbor.secret.delete` | `destructive` | false | true |
| `baseharbor.secret.tls-set` | `mutating` | false | false |
| `baseharbor.target.create` | `mutating` | false | false |
| `baseharbor.target.delete` | `mutating` | false | false |
| `baseharbor.stack.list` | `read_only` | false | false |
| `baseharbor.stack.show` | `read_only` | false | false |
| `baseharbor.stack.create` | `mutating` | false | false |
| `baseharbor.workspace.init` | `mutating` | false | false |
| `baseharbor.workspace.map` | `mutating` | false | false |
| `baseharbor.target` | `read_only` | false | false |
| `baseharbor.target.list` | `read_only` | false | false |
| `baseharbor.runtime.capabilities` | `read_only` | true | false |
| `baseharbor.runtime.list` | `read_only` | true | false |
| `baseharbor.runtime.inspect` | `read_only` | true | false |
| `baseharbor.runtime.metrics` | `read_only` | true | false |
| `baseharbor.runtime.start` | `mutating` | true | false |
| `baseharbor.runtime.stop` | `mutating` | true | false |
| `baseharbor.runtime.restart` | `mutating` | true | false |
| `baseharbor.app.list` | `read_only` | false | false |
| `baseharbor.inspect` | `read_only` | false | false |
| `baseharbor.workspace.list` | `read_only` | false | false |
| `baseharbor.workspace.resolve` | `read_only` | false | false |
| `baseharbor.workspace.status` | `read_only` | false | false |
| `baseharbor.workspace.update` | `mutating` | false | false |
| `baseharbor.app.new` | `mutating` | false | false |
| `baseharbor.plan` | `read_only` | false | false |
| `baseharbor.apply` | `mutating` | true | false |
| `baseharbor.status` | `read_only` | false | false |
| `baseharbor.doctor` | `read_only` | false | false |
| `baseharbor.observe` | `read_only` | false | false |
| `baseharbor.evidence` | `read_only` | false | false |
| `baseharbor.update` | `mutating` | true | false |
| `baseharbor.repair` | `mutating` | true | false |
| `baseharbor.backup` | `mutating` | false | false |
| `baseharbor.restore` | `mutating` | true | false |
| `baseharbor.destroy` | `destructive` | true | true |
| `baseharbor.policy.check` | `read_only` | true | false |
| `baseharbor.provider.list` | `read_only` | false | false |
| `baseharbor.provider.inspect` | `read_only` | false | false |
| `baseharbor.provider.verify` | `read_only` | false | false |
| `baseharbor.provider.add` | `mutating` | true | false |
| `baseharbor.provider.remove` | `destructive` | true | true |
| `baseharbor.organization.inspect` | `read_only` | false | false |
| `baseharbor.organization.check` | `read_only` | false | false |
| `baseharbor.organization.set` | `mutating` | true | false |
| `baseharbor.organization.update` | `mutating` | true | true |
| `baseharbor.policy.explain` | `read_only` | false | false |

<!-- END GENERATED MCP REGISTRY -->

`baseharbor.target` reports the effective Target plus repository-resolved application/environment context. It accepts an optional target selector and returns the same target semantics as `baha target -o json`.

These are BaseHarbor lifecycle operations, not wrappers around CLI commands.

`baseharbor.apply` is the semantic converge operation rather than a 1:1 mirror of every human CLI alias.

`baseharbor.evidence` is read-only and returns the same deterministic evidence bundle as `baha app evidence -o json`, including recovery contributor evidence and bounded audit history.

`baseharbor.destroy` is destructive and requires explicit approval.

Backup and restore accept an owner-only local password-file reference. Plaintext backup passwords are not accepted as MCP arguments.

## Operator authorization

MCP does not define its own RBAC model. Every mutating or destructive tool passes through the shared BaseHarbor Machine Operator Authorization boundary before mutation begins.

- development uses explicit `trusted-local` actor semantics;
- managed environments require the configured authenticated operator identity and fail closed when it is missing or invalid;
- machine safety metadata (`read_only`, `mutating`, `destructive`, policy and confirmation requirements) is preserved by the authorization decision;
- authorization/audit metadata is secret-safe and never returns bearer tokens, refresh tokens, private keys or equivalent credential material.

## Rules

- MCP uses the same semantic lifecycle as CLI/JSON.
- MCP is local stdio in this release.
- MCP does not expose a generic shell.
- MCP does not grant unrestricted Docker, Compose, Podman or provider-native execution.
- Operations carry read-only, mutating or destructive safety semantics.
- Authorization, policy, ownership, preflight, reconciliation, verification and secure bindings cannot be bypassed.
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
    "baha_baseharbor_workspace_resolve": "allow",
    "baha_baseharbor_workspace_status": "allow",
    "baha_baseharbor_inspect": "allow",
    "baha_baseharbor_plan": "allow",
    "baha_baseharbor_doctor": "allow",
    "baha_baseharbor_observe": "allow",
    "baha_baseharbor_evidence": "allow",
    "baha_baseharbor_policy_check": "allow",
    "baha_baseharbor_policy_explain": "allow"
  }
}
```

Permission-key syntax is client-specific. The behavioral rule is not: read-only understanding/planning/verification may be allowed automatically, while `apply`, lifecycle `update`, `workspace.update`, `repair`, `backup`, `restore` and `destroy` remain approval-gated.

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

BaseHarbor v0.4.15.1 detaches an accepted mutating lifecycle convergence from client-request cancellation so a client timeout does not intentionally truncate the mutation half-way through. Detached convergence remains bounded by a BaseHarbor-owned 30-minute lifecycle deadline. A larger client timeout is still recommended so the caller receives the verified final result rather than having to query status after its own request timeout.

Safety model:

```text
understand / plan / verify
        ↓
mutation requires approval
        ↓
destruction requires explicit BaseHarbor approval too
```


## Development and workspace tools

`baseharbor.app.new` is the semantic greenfield creation operation. It uses the same Application Contract, Stack Profile, Development Plan and adapter model as the human CLI and does not expose a generic shell.

The result identifies `validation_scope: repository_capability_evidence` and `build_verified: false`. A satisfied repository inspection does not claim that the generated project has compiled or its runtime is ready.

`baseharbor.workspace.resolve` is read-only. It resolves canonical component/source identity against the developer-local XDG workspace mapping.

`baseharbor.workspace.status` exposes the same per-repository Git state model as the human CLI. Optional fetch refreshes remote-tracking state but never changes checked-out revisions.

`baseharbor.workspace.update` uses the same guarded native-Git core as `baha app workspace update`: only clean, non-diverged branches with a configured upstream may fast-forward. MCP cannot bypass dirty/diverged/detached/missing-upstream safety restrictions. `check=true` performs preview-only evaluation.

## CLI coverage

The [CLI / machine matrix](cli-machine-coverage.md) includes every supported visible command and alias. Unclassified new commands fail the generated inventory check; host, interactive and transport exclusions include reasons and safe alternatives. `workspace.init` and `workspace.map` accept typed inputs and use the same shared operations as the noninteractive CLI/JSON paths.


## Typed operation examples

Follow [machine operation recipes](../how-to/machine-operations.md) for application creation, workspace mapping, protected secrets, trust approval and provider conformance. Inputs and required properties come from the actual MCP `tools/list` schema. CLI JSON and MCP use shared operations; raw credentials and arbitrary runtime execution are excluded from MCP.

## Explicit control-plane availability

`baseharbor.control-plane.up` bootstraps the mandatory SQL/Secrets/Identity Core without an Application repository. It accepts `machine_role` (`development` or `deployment`) to select defaults, plus `ha` (boolean, default `false`) alongside target, ports and the protected recovery-file reference. The same lifecycle selects the standard topology for a fresh target and rejects an incompatible HA request for retained single-server state. `control-plane.status` reports actual running members and availability; single-server mode never claims failover.

```json
{"name":"baseharbor.control-plane.up","arguments":{"target":"local","ha":false}}
```
