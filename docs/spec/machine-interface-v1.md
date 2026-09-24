# Machine Interface v1

## Scope

CLI JSON and MCP expose the same underlying BaseHarbor semantic state.

The machine interface is an adapter over the shared BaseHarbor lifecycle. It is not a second orchestration path.

## Lifecycle surface

The v1 semantic operation set covers the currently supported application lifecycle:

```text
inspect
plan
apply
status
doctor
observe
update
repair
backup
restore
destroy
policy.check
policy.explain
```

`apply` represents the semantic converge operation; MCP does not mirror every human CLI alias or presentation command.

## Safety classes

Every machine operation declares one of:

- `read_only`;
- `mutating`;
- `destructive`.

Destructive operations declare whether explicit approval is required. `destroy` requires explicit approval before any mutation.

Unknown or unresolved safety requirements fail closed.

## Non-interactive behavior

Machine operations never depend on an interactive terminal.

When a lifecycle action requires unresolved developer/operator input, the operation returns a structured actionable error instead of prompting or guessing. Examples include:

- missing required application secrets;
- an explicit recovery choice before updating durable state;
- destructive approval;
- unsupported or ambiguous ownership state;
- policy denial.

## Secret handling

Passwords, tokens, private keys and secret values MUST NOT be accepted or returned through normal machine result fields.

Backup and restore accept an owner-only local password-file reference rather than the plaintext password.

Credential-bearing URLs MUST NOT be returned as normal machine output.

## Requirements

- Machine results MUST be versioned.
- Errors MUST be structured and secret-safe.
- CLI, JSON and MCP MUST reuse the same lifecycle semantics.
- Interfaces MUST NOT bypass policy, ownership, preflight, reconciliation or verification.
- Mutations MUST preserve the `plan -> preflight -> apply -> verify` lifecycle.
- Destructive operations MUST preserve explicit safety/approval semantics.
- MCP MUST NOT expose generic shell, Docker, Compose, Podman or provider-native execution as a substitute for BaseHarbor lifecycle operations.
- Runtime-provider implementation details MUST NOT become MCP semantics.
- Unsupported lifecycle operations MUST return an explicit typed result.
- Docker/Compose and Podman/Quadlet MUST expose the same agent-facing semantic operations where the lifecycle operation is supported.

The typed machine operation registry in `internal/machine` is the authoritative operation/safety definition.
