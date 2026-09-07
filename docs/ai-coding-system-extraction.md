# AI Coding System extraction matrix

BaseHarbor reuses proven generic foundations from `mcpdev80/ai-coding-system` while keeping coding-specific behavior in the application.

## Extract now

| AI Coding System area | BaseHarbor target | Decision |
| --- | --- | --- |
| `internal/auth/principal.go` | `internal/identity` | Extract provider-neutral principal/context contract |
| `internal/authorization/authorization.go` | `internal/authorization` | Extract fail-closed CRUD RBAC core |
| tenant context/role aggregation from `internal/tenant/middleware.go` | `internal/tenancy` | Extract framework-neutral resolution logic only |
| `internal/credential/broker.go` + safe no-op behavior | `internal/credential` | Extract provider-neutral secret boundary |

## Extract later after interface cleanup

| Area | Reason |
| --- | --- |
| OIDC/JWT verifier | Generic in principle, but should be composed behind BaseHarbor config and provider interfaces first |
| OpenBao adapter | Valuable, but depends on final BaseHarbor secret configuration and lifecycle model |
| PostgreSQL persistence/migrations | Reuse patterns, not application-specific schemas; BaseHarbor needs its own tenancy/control-plane schema |
| HTTP API error model and middleware | Reuse semantics after choosing BaseHarbor's framework-neutral HTTP boundary |
| observability/tracing | Extract after common service lifecycle exists |
| deployment/runtime provider abstractions | Extract only the generic container/runtime contracts, not coding-job semantics |

## Keep in AI Coding System

- agent runtime and coding workflows
- Git/GitHub adapters and repository lifecycle
- coding-specific projects, tasks, plans, workspaces and runs
- code execution orchestration and worker-agent behavior
- coding-learning domain logic
- run cancellation rules tied to coding execution

## Security invariants carried into BaseHarbor

1. Unknown identities, roles, permissions and tenant ambiguity fail closed.
2. Tenant scope is explicit and context-bound.
3. Secret references stay opaque outside the credential broker.
4. Secret payloads are not JSON-serializable and must never appear in errors or logs.
5. Provider-specific concerns remain behind interfaces.
6. BaseHarbor must not depend on application-specific domain models.

This document is intentionally conservative: extracting fewer stable contracts is preferred over copying tightly coupled application code.
