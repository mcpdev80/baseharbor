# BaseHarbor documentation

This directory is the canonical documentation source for BaseHarbor.

The README is a product overview and quick entry point. Detailed behavior, operational contracts and security guarantees belong here. The GitHub Pages site is generated from these documents rather than maintaining a second independent copy.

## Start here

- [Application contract](application-contract.md)
- [Repository application workflow](repository-application-workflow.md)
- [`baha` CLI](cli.md)
- [Local control-plane runtime](runtime-compose.md)
- [Secrets and OpenBao](secrets-and-openbao.md)
- [Backup and restore](backup-and-restore.md)
- [Releases and versioning](releases.md)

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

Documentation describes behavior that exists on the default branch unless explicitly marked as planned. Code and executable acceptance tests are the immediate source of truth when documentation drifts.

Public documentation distinguishes between implemented behavior, published release contracts and planned work.

English is canonical/default. GitHub Pages publishes English at `/` and maintained German documentation at `/de/`.