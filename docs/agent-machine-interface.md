# Agent-native machine interface

BaseHarbor v0.4.12 exposes a small, versioned machine interface for coding agents and automation without creating a second lifecycle or policy path.

The same semantic operations back human CLI output, structured JSON, the TUI where applicable, and MCP adapters.

## Machine contract

The initial public machine contract is:

```text
baseharbor.machine/v1
```

Machine-facing results include:

```json
{
  "contract_version": "v1"
}
```

The version identifies BaseHarbor's semantic result contract. It is independent from the MCP wire-protocol version.

Structured errors use stable categories such as validation, policy denial, conflict, ownership ambiguity, provider/runtime availability, timeout, verification failure and unsupported operation. Error responses never require callers to parse human terminal prose.

## Discover supported operations

```bash
baha agent describe
baha agent describe -o json
```

The discovery result includes:

- BaseHarbor version;
- machine-contract version;
- supported semantic operations;
- read-only/mutating/destructive safety classification;
- confirmation and policy metadata;
- supported capability specifications;
- MCP transport, protocol and tool names.

The v0.4.12 public operation set is deliberately small:

| Operation | Safety | Purpose |
| --- | --- | --- |
| `inspect` | read-only | inspect repository evidence and capability findings |
| `plan` | read-only | build the deterministic desired-state plan |
| `status` | read-only | observe runtime/readiness state |
| `doctor` | read-only | run diagnostic verification |

## Structured JSON

These commands expose versioned, secret-safe structured results:

```bash
baha app inspect . -o json
baha plan -o json
baha status -o json
baha doctor -o json
```

Human and machine output use the same underlying typed result collection. JSON is a machine contract, not a serialization of terminal tables or ANSI output.

When a JSON-mode command fails before it can produce its normal result, stderr contains a versioned structured error envelope.

## MCP

Run the local MCP server with:

```bash
baha mcp serve
```

v0.4.12 uses the official Model Context Protocol Go SDK v1.8.0 and targets MCP specification `2026-07-28`, while retaining the SDK's negotiated compatibility with `2025-11-25` for clients that still require the previous lifecycle.

The initial transport is **stdio only**. BaseHarbor does not open a listening socket, provide remote MCP authentication or create a background daemon.

The server exposes exactly four semantic tools:

```text
baseharbor.inspect
baseharbor.plan
baseharbor.status
baseharbor.doctor
```

All four are explicitly annotated read-only. `inspect` may read a remote Git source supplied as its path, so it is marked open-world; the other initial tools operate against the selected local BaseHarbor application context.

### No generic execution

The MCP surface intentionally does not expose generic primitives such as:

```text
exec(command)
shell(command)
docker(command)
compose(command)
```

Agents use BaseHarbor semantics. They do not receive an alternate path around BaseHarbor ownership, isolation, bindings, validation or verification.

## Security boundaries

Machine responses are secret-safe by construction:

- no plaintext secret values or credentials;
- no provider-global credentials;
- no runtime-socket access granted by MCP;
- no arbitrary command execution;
- no hidden confirmation or policy bypass;
- no remote listener in v0.4.12;
- provider/runtime implementation details do not become portable application intent.

MCP tool annotations are useful discovery hints, not the security boundary. BaseHarbor enforces safety by exposing only the supported semantic operations and by routing them through the same application/runtime logic as other control surfaces.

## Repository guidance

`baha app init --agents` maintains only BaseHarbor's bounded section in `AGENTS.md`. v0.4.12 guidance tells coding agents to prefer structured BaseHarbor interfaces, discover operations with `baha agent describe -o json`, and use `baha mcp serve` rather than bypassing BaseHarbor through shell/Docker/Compose operations.

Unrelated repository instructions are preserved and malformed managed markers still fail closed.

## Deferred

v0.4.12 does not add:

- remote/HTTP MCP serving;
- MCP OAuth/OIDC/RBAC;
- broad mutating/destructive MCP tools;
- embedded LLM/model logic;
- vendor-specific agent integrations;
- application-provided/consumed MCP as a portable capability;
- Kubernetes/OpenShift runtime behavior.

Those concerns remain separate from the minimal BaseHarbor agent interface.
