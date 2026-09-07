# BaseHarbor architecture

BaseHarbor is a secure, modular, self-hosted backend foundation for independent applications.

## Principles

- One-command lifecycle through the `baha` CLI.
- Apps stay independent from BaseHarbor and contain only application-specific logic.
- Secure defaults, least privilege, deny by default.
- Prefer mature open-source components over reimplementing commodity infrastructure.
- Kubernetes is optional; single-node deployments are a first-class target.
- AI, MCP and RAG are platform capabilities, not application-specific add-ons.

## Initial platform boundaries

```text
BaseHarbor
├── baha CLI
├── control plane
├── auth / authorization
├── database
├── secrets
├── storage
├── jobs
├── realtime
├── audit
├── observability
├── AI integration
├── MCP
├── RAG
└── module system
```

## Application boundary

BaseHarbor provides reusable platform capabilities. Business logic stays in the consuming application.

Examples:

- MailFlow keeps IMAP/SMTP, mailbox sync, classification, review and routing logic.
- AWC keeps runner isolation, worktrees, build/test/commit/push and execution scheduling.
- AI Coding System keeps repository, coding-agent and Git lifecycle logic.

## Current milestone

The first milestone is deliberately small: a reliable `baha` binary with `version`, `init` and `doctor`. Platform services are added only after the lifecycle foundation is testable and predictable.
