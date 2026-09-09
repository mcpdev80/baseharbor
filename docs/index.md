# BaseHarbor documentation

This directory is the canonical documentation source for BaseHarbor.

The README is a product overview and quick entry point. Detailed behavior, operational contracts and security guarantees belong here. The future GitHub Pages site is generated from these documents rather than maintaining a second independent copy.

## Start here

- [Application contract](application-contract.md) — what an application declares and what BaseHarbor promises.
- [Repository application workflow](repository-application-workflow.md) — the preferred repo-owned `baseharbor.yaml` workflow.
- [`baha` CLI](cli.md) — current command families and lifecycle behavior.
- [Local control-plane runtime](runtime-compose.md) — PostgreSQL/OpenBao control plane, ports, state and lifecycle.
- [Secrets and OpenBao](secrets-and-openbao.md) — trust model, bootstrap, static and dynamic secret delivery.
- [Backup and restore](backup-and-restore.md) — encrypted application recovery units and restore semantics.

## Architecture and security

- [Architecture](architecture.md)
- [Authentication](authentication.md)
- [Authentication and API errors](authentication-and-api-errors.md)
- [Application runtime identity](application-runtime-identity.md)
- [Application secret API](application-secret-api.md)
- [Dynamic application secrets](dynamic-application-secrets.md)
- [PostgreSQL and migrations](postgresql-and-migrations.md)
- [Service-instance and HA decisions](decisions/0001-service-instances-and-ha-intent.md)

## Project operation

- [Roadmap](roadmap.md)
- [Dependency updates](dependency-updates.md)
- [Development guidelines](DEVELOPMENT_GUIDELINES.md)
- [Pre-release documentation audit](pre-release-documentation-audit.md)

## Documentation contract

Documentation must describe behavior that exists on the default branch unless it is explicitly marked as planned or pre-release work.

When code and documentation disagree, code and executable acceptance tests are the immediate source of truth and the documentation must be corrected before the next release.

Public product documentation should distinguish clearly between:

- **implemented** — available on the default branch and covered by tests;
- **release contract** — behavior included in a published BaseHarbor release;
- **planned** — roadmap or architecture intent that consumers must not rely on yet.

The English documentation is the canonical source. The GitHub Pages site will publish English at `/` and a maintained German translation at `/de/`.