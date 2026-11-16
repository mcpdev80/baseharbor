# Reusable foundations audit

This document records reusable implementation patterns identified across existing projects without coupling BaseHarbor to any application-specific codebase or private repository history.

## Decision classes

- **Already in BaseHarbor** — a generic equivalent already exists and should be evolved in place.
- **Adopt into BaseHarbor** — a proven pattern should be reimplemented behind BaseHarbor-owned interfaces.
- **Keep application-specific** — the behavior belongs to a consuming application, not the platform.

## Audit matrix

| Capability / pattern | Decision | BaseHarbor target | Notes |
| --- | --- | --- | --- |
| Provider-neutral principal / identity context | Already in BaseHarbor | `internal/identity` | Keep framework- and provider-neutral. |
| Fail-closed RBAC / CRUD authorization | Already in BaseHarbor | `internal/authorization` | Unknown roles/actions stay denied. |
| Explicit tenant context resolution | Already in BaseHarbor | `internal/tenancy` | Continue toward DB-enforced isolation and RLS. |
| Credential broker / opaque secret references | Already in BaseHarbor | `internal/credential` | Secret payloads must not leak into logs/errors. |
| OIDC configuration and bearer boundary | Already in BaseHarbor | auth/API layer | Keep provider-neutral; concrete JWKS verifier remains a follow-up. |
| PostgreSQL migrations and control-plane persistence | Already in BaseHarbor | `internal/postgres`, migrations | App business schemas remain outside BaseHarbor. |
| Compose runtime abstraction and truthful service readiness | Already in BaseHarbor | `internal/runtime` | Preserve distinction between container-running and service-ready. |
| Real provider/service preflight checks before mutation | Adopt into BaseHarbor | `internal/preflight` | Resolve dependencies first, perform real connectivity/health checks, then mutate. |
| Unified health/readiness report with degraded state | Adopt into BaseHarbor | `baha doctor`, app status | Aggregate dependency health without turning unknown/stale state into healthy. |
| Redis/Valkey connectivity probe | Adopt into BaseHarbor | service template / doctor | Needed once Redis/Valkey becomes an app service. |
| Runtime convergence: config -> compose -> migrations -> verify | Adopt into BaseHarbor | app apply/lifecycle | Desired-state application should end in verification, not just container creation. |
| Safe configuration generation with persistent secret preservation | Adopt into BaseHarbor | app state/runtime | Regeneration must not silently rotate existing credentials. |
| Port discovery and conflict handling | Adopt into BaseHarbor | runtime / edge | Prefer no host exposure for app services; where ports are required, detect conflicts explicitly. |
| Guided install / resumable setup flow | Adopt into BaseHarbor | `baha bootstrap` | Minimal questions, path completion where appropriate, resumable after interruption. |
| Certificate folder auto-discovery | Adopt into BaseHarbor | `internal/certificate` | Detect PEM/CER/CRT/KEY/ZIP/P7B/P7C/P12/PFX. |
| Leaf certificate selection by private/public key match | Adopt into BaseHarbor | `internal/certificate` | Never rely only on filenames. |
| Intermediate-chain construction | Adopt into BaseHarbor | `internal/certificate` | Build ordered chain and omit unnecessary self-signed root from served fullchain. |
| Hostname/SAN/wildcard and validity verification | Adopt into BaseHarbor | `internal/certificate` | Add EKU/serverAuth and trust-root verification beyond the proven baseline. |
| ACME automatic TLS plus imported/BYOC certificates | Adopt into BaseHarbor | certificate manager | Equal first-class sources together with internal PKI. |
| Custom outbound CA trust | Adopt into BaseHarbor | trust manager | Needed for enterprise/internal environments. |
| Secret key rotation with key-ring fallback | Adopt pattern, not implementation | OpenBao/bootstrap secret lifecycle | OpenBao is the target secret system; retain the operational rule that old keys remain usable during controlled rotation. |
| Central log/exception redaction | Adopt into BaseHarbor | logging/observability | Redact passwords, tokens, API keys, Authorization headers and credential-bearing URLs before persistence/export. |
| Restore preflight: schema revision + secret validation + ownership/invariant checks | Adopt into BaseHarbor | backup/restore | Restore is incomplete until invariants are verified before workloads resume. |
| Fail-closed maintenance/restore pause | Adopt into BaseHarbor | lifecycle | Mutating workers/services must be quiesceable during restore or dangerous migrations. |
| Backup-before-schema-migration | Adopt where stateful migration warrants it | lifecycle/backup | Especially useful for local control-plane state; do not substitute this for normal backups. |
| Meaningful lifecycle audit events with bounded details | Adopt into BaseHarbor | audit | Record state-changing lifecycle events, not noisy per-request/per-record events. Sensitive detail keys are excluded. |
| Retention classes and bounded cleanup | Adopt into BaseHarbor | audit/operational state | Operational state and audit/history need distinct retention policies. |
| App/service isolation using dedicated containers, networks and volumes | Adopt into BaseHarbor | application runtime | Core product capability. Default app services must not publish host ports. |
| Per-job cancellation via context/cancel handles | Keep application-specific for now | future jobs module | Generic concept is useful, but current implementations are tightly coupled to execution workflows. Revisit only when BaseHarbor ships a jobs capability. |
| Leases, heartbeats and hard timeouts | Keep application-specific for now | future jobs module | Good patterns, but not needed for the first application-runtime MVP. |
| Live SSE job timelines/log streams | Keep application-specific for now | future events/jobs | BaseHarbor should expose standards-based logs/events, not inherit a workflow UI contract. |
| Disposable isolated workspaces / Docker execution sandboxes | Keep application-specific | consuming applications | BaseHarbor manages backend services, not arbitrary coding/agent execution sandboxes. |
| Git/GitHub repository lifecycle and merge gates | Keep application-specific | consuming applications | Outside BaseHarbor scope. |
| Mailbox/IMAP/SMTP/OAuth behavior | Keep application-specific | consuming applications | Domain behavior, not backend-runtime infrastructure. |
| Email classification, DecisionMemory and mailbox action policy | Keep application-specific | consuming applications | AI/domain semantics remain in the application. |
| Attachment classification/extraction and mail-safe rendering | Keep application-specific | consuming applications | Not a generic platform concern. |
| Model-role routing, inference fallback and AI workload scheduling | Keep application-specific initially | AI integration later | BaseHarbor may expose AI connectivity/health in the future but should not import application-specific scheduling policy. |

## Required extraction work before broadening scope

### Priority A — application-runtime MVP

1. Application resource + manifest.
2. `baha app create/list/show`.
3. Dedicated PostgreSQL app service with its own network, volume and credentials.
4. `baha app up/down/status/doctor/env`.
5. Generic preflight/convergence model: discover -> validate -> mutate -> verify.
6. Platform secret ownership through OpenBao rather than long-lived plaintext runtime files.
7. Redis/Valkey app service after PostgreSQL is proven.

### Priority B — security and lifecycle

1. Certificate manager with ACME, internal PKI and BYOC/import.
2. Certificate package discovery and cryptographic validation.
3. Log/exception redaction shared by CLI/control-plane services.
4. Backup/restore with schema/invariant verification before resume.
5. Quiesce/pause semantics for destructive lifecycle operations.
6. Meaningful lifecycle audit trail and bounded retention.

### Priority C — operate the platform

1. OpenTelemetry-first telemetry pipeline.
2. Metrics, logs and traces labeled by app/environment/service.
3. Central health/degraded model used by `status` and `doctor`.
4. Upgrade/convergence workflow with rollback-safe state handling.
5. Alert integration after observability state is trustworthy.

## Explicit non-extractions

BaseHarbor will not absorb application domain logic merely because an existing project already implements it. In particular, repository automation, coding-agent execution, mail processing, mailbox workflows, AI classification semantics and application-specific job state machines remain in their applications.

## Architectural rule

A pattern belongs in BaseHarbor only when all of the following are true:

1. it is useful to multiple unrelated applications;
2. it can be expressed without importing a domain model from a consuming app;
3. it strengthens provisioning, security, isolation, observability or lifecycle management;
4. developers can still consume the resulting service through standard protocols or stable BaseHarbor control-plane APIs;
5. adopting it does not turn BaseHarbor into a general-purpose application-hosting PaaS.

The audit therefore favors proven operational patterns over source-code copying. BaseHarbor owns its interfaces and implementations even where the underlying behavior was validated elsewhere.