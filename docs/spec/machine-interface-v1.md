# Machine Interface v1

## Scope

CLI JSON and MCP expose the same underlying BaseHarbor semantic state.

## Requirements

- Machine results MUST be versioned.
- Errors MUST be structured and secret-safe.
- Interfaces MUST NOT bypass policy, ownership, preflight, reconciliation or verification.
- Destructive operations MUST preserve explicit safety/approval semantics.
- MCP MUST NOT expose generic shell or unrestricted runtime execution as a substitute for BaseHarbor lifecycle operations.
- Unsupported lifecycle operations MUST return an explicit typed result.
