# BaseHarbor

Secure, modular, self-hosted application backend runtime managed through the `baha` CLI.

BaseHarbor provides reusable backend infrastructure for independent applications without forcing those applications into one monolith or a proprietary data-access SDK.

## Status

Early development. Identity, authorization, tenancy, secrets, PostgreSQL migrations, database-enforced tenant isolation, the first single-node control-plane runtime, and the declarative application resource model are in place.

The current application CLI can create and inspect desired state and perform read-only planning/preflight. Per-application service convergence is the next runtime milestone.

## CLI

Build the single operator binary:

```bash
go build -o baha ./cmd/baha
```

Discover commands at every level:

```bash
./baha --help
./baha app --help
./baha app create --help
```

Control-plane commands:

```bash
./baha version
./baha init
./baha doctor
./baha up
./baha status
./baha down
```

Application desired-state commands:

```bash
./baha app create demo
./baha app list
./baha app show demo
./baha app plan demo
./baha app preflight demo
```

`baha doctor` and `baha status` verify actual service readiness. PostgreSQL readiness requires a successful authenticated connection and query; OpenBao must be reachable, initialized, and unsealed.

Application lifecycle follows the stable contract:

```text
plan -> preflight -> apply -> verify
```

See [docs/cli.md](docs/cli.md) and [docs/runtime-compose.md](docs/runtime-compose.md).

## Design goals

- one dependable binary for setup and lifecycle management
- secure defaults, least privilege and fail-closed behavior
- isolated backend service stacks for independent applications
- native protocols for application consumption
- self-hosted first, cloud-native where useful
- Docker/Podman first; Kubernetes optional
- mature open-source components instead of unnecessary reinvention
- observable health, backup/restore, certificates and lifecycle operations
- AI, MCP and RAG as optional first-class platform capabilities

See [docs/architecture.md](docs/architecture.md), [docs/roadmap.md](docs/roadmap.md), and the mandatory [development guidelines](docs/DEVELOPMENT_GUIDELINES.md).

## Planned platform capabilities

```text
BaseHarbor
├── baha CLI
├── shared control plane
├── isolated application service stacks
├── auth / authorization
├── PostgreSQL
├── Redis / Valkey
├── secrets / OpenBao
├── certificates / PKI
├── object storage
├── backup / restore
├── observability
├── jobs / realtime
├── AI integration
├── MCP
└── RAG
```

## License

Apache License 2.0. See [LICENSE](LICENSE).
