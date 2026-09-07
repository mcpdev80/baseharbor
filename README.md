# BaseHarbor

Secure, modular, self-hosted backend foundation with auth, data, storage, jobs, observability, AI and MCP — managed through the `baha` CLI.

BaseHarbor is intended to provide reusable backend infrastructure for independent applications without forcing those applications into one monolith.

## Status

Early development. The first milestone is the lifecycle foundation: a small, testable Go CLI before platform services are added.

## CLI

```bash
go build -o baha ./cmd/baha
./baha version
./baha init
./baha doctor
```

Current commands:

- `baha version` — print build information.
- `baha init` — create a minimal `baseharbor.yaml` with restrictive permissions.
- `baha doctor` — validate the host and detect Docker or Podman.

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
