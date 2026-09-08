# BaseHarbor

Secure, modular, self-hosted application backend runtime managed through the `baha` CLI.

BaseHarbor provides reusable backend infrastructure for independent applications without forcing those applications into one monolith or a proprietary data-access SDK.

## Status

Early development. Identity, authorization, tenancy, secrets, PostgreSQL migrations, database-enforced tenant isolation, the single-node control-plane runtime, and the declarative application resource model are in place.

The first per-application convergence slice is PostgreSQL: `baha app apply NAME` creates an isolated Compose project with a dedicated PostgreSQL volume and network, does not publish a host port by default, preserves generated credentials across repeated apply operations, and reports ready only after an authenticated `SELECT 1` succeeds.

The runtime can be inspected with `baha app status NAME`, diagnosed with `baha app doctor NAME`, stopped without deleting persistent data with `baha app down NAME`, and permanently removed through the ownership-verified `baha app destroy NAME --yes` path.

## CLI

Build the single operator binary:

```bash
go build -o baha ./cmd/baha
```

Discover commands at every level:

```bash
./baha --help
./baha app --help
./baha app apply --help
./baha app status --help
./baha app doctor --help
./baha app down --help
./baha app destroy --help
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

Application commands:

```bash
./baha app create demo
./baha app list
./baha app show demo
./baha app plan demo
./baha app preflight demo
./baha app apply demo
./baha app status demo
./baha app doctor demo
./baha app down demo
./baha app destroy demo
./baha app destroy demo --yes
```

`baha app down` removes the managed container and transient network but preserves the PostgreSQL volume, runtime state and credentials. A later `baha app apply` converges the same application again using the preserved data.

`baha app destroy` is destructive by design. Without `--yes` it performs the safety preflight and prints the exact managed resources that would be removed, but makes no changes. With `--yes`, BaseHarbor first verifies the generated runtime definition and exact Compose ownership labels. Ambiguous or mismatched ownership fails closed before deletion. Persistent volumes and local application state are then removed and absence is verified.

`baha doctor`, `baha status`, `baha app status`, and `baha app doctor` verify actual service readiness rather than only process/container state.

Application convergence follows the stable contract:

```text
plan -> preflight -> apply -> verify
```

Destructive lifecycle operations add explicit ownership verification before mutation and post-verification after mutation.

The current `app apply` milestone intentionally supports PostgreSQL-only desired state. Manifests that also enable Redis/Valkey or managed secrets fail closed until those convergence modules are implemented.

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
