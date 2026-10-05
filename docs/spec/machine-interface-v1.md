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
evidence
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

## Machine operator authorization

Every machine operation is authorized below the presentation layer through one transport-neutral decision model.

The decision binds:

```text
effective actor/principal
        +
operation + safety class
        +
application/environment/target/workspace context
        ↓
allow / deny
```

Development uses explicit `trusted-local` actor semantics. Managed environments fail closed when the required authenticated operator principal is absent or invalid. The decision is secret-safe and exposes only stable actor identity/provenance fields, never tokens or private credential material.

MCP, CLI/JSON and future HTTP/Console adapters must consume the same authorization boundary; no adapter may define a separate RBAC model.

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
- Interfaces MUST NOT bypass authorization, policy, ownership, preflight, reconciliation or verification.
- Managed-environment machine operations MUST fail closed without the required authenticated operator identity.
- Mutations MUST preserve the `plan -> preflight -> apply -> verify` lifecycle.
- Destructive operations MUST preserve explicit safety/approval semantics.
- MCP MUST NOT expose generic shell, Docker, Compose, Podman or provider-native execution as a substitute for BaseHarbor lifecycle operations.
- Runtime-provider implementation details MUST NOT become MCP semantics.
- Unsupported lifecycle operations MUST return an explicit typed result.
- Docker/Compose and Podman/Quadlet MUST expose the same agent-facing semantic operations where the lifecycle operation is supported.

The typed machine operation registry in `internal/machine` is the authoritative operation/safety definition.
