# BaseHarbor

Secure, modular, self-hosted backend foundation with auth, data, storage, jobs, observability, AI and MCP — managed through the `baha` CLI.

BaseHarbor is intended to provide reusable backend infrastructure for independent applications without forcing those applications into one monolith.

## Status

Early development. Identity, authorization, tenancy, secrets, PostgreSQL migrations and database-enforced tenant isolation are in place. The current milestone adds the first single-node operational runtime managed by `baha`.

## CLI

```bash
go build -o baha ./cmd/baha
./baha version
./baha init
./baha doctor
./baha up
./baha status
./baha down
```

Current commands:

- `baha version` — print build information.
- `baha init` — create a minimal `baseharbor.yaml` with restrictive permissions.
- `baha doctor` — validate the host, container runtime and actual service readiness.
- `baha up` — materialize and start the local PostgreSQL/OpenBao Compose runtime.
- `baha status` — show container state and BaseHarbor service readiness.
- `baha down` — stop the local runtime without deleting persistent volumes.

The runtime binds PostgreSQL and OpenBao to loopback by default. OpenBao uses persistent server mode rather than an insecure development root token, so a fresh runtime is intentionally reported as not ready until its initialization/unseal lifecycle is completed.

See [docs/runtime-compose.md](docs/runtime-compose.md) for the runtime model and current limitations.

## Design goals

- one-command setup and lifecycle management
- secure defaults, least privilege and deny by default
- modular architecture with independent consuming applications
- self-hosted first, cloud-native where useful
- Docker/Podman first; Kubernetes optional
- mature open-source components instead of unnecessary reinvention
- AI, MCP and RAG as first-class platform capabilities

See [docs/architecture.md](docs/architecture.md) for the initial boundaries and principles.

## Planned platform capabilities

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

## License

Apache License 2.0. See [LICENSE](LICENSE).
