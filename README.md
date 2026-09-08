# BaseHarbor

Secure, modular, self-hosted application backend runtime managed through the `baha` CLI.

BaseHarbor provides reusable backend infrastructure for independent applications without forcing those applications into one monolith or a proprietary data-access SDK.

## Status

Early development. Identity, authorization, tenancy, secrets foundations, PostgreSQL migrations, database-enforced tenant isolation, the single-node control-plane runtime, and the declarative application resource model are in place.

Per-application runtime convergence supports dedicated PostgreSQL and Valkey services. Applications can also request an isolated managed OpenBao secret scope alongside PostgreSQL and/or Valkey. `baha app apply NAME` creates the isolated Compose runtime, provisions the application OpenBao policy/AppRole when requested, and reports success only after protocol/authentication verification.

PostgreSQL readiness requires an authenticated `SELECT 1`. Valkey readiness requires an authenticated `PING` returning `PONG`. Valkey uses the official `valkey/valkey:9.1.2-alpine` image with AOF persistence enabled.

Managed secrets use one exact KV document per application/environment at `baseharbor/apps/<app>/<environment>` plus an application-specific `baseharbor-app-<app>-<environment>` policy/AppRole. Application RoleID/SecretID bootstrap state is owner-only; application secret payloads are not stored in the manifest or local runtime environment files.

The runtime can be inspected with `baha app status NAME`, diagnosed with `baha app doctor NAME`, stopped without deleting persistent data or the OpenBao scope with `baha app down NAME`, resumed from existing materialized state with `baha app up NAME`, and permanently removed through the ownership-verified `baha app destroy NAME --yes` path.

The bundled OpenBao control-plane runtime has an explicit manual bootstrap and unseal workflow. BaseHarbor initializes OpenBao without persisting or printing the initial root token, creates the `baseharbor/` KV v2 mount, establishes a restricted manager AppRole, verifies it, and revokes the initial root token. Shamir unseal material is written only to an operator-selected recovery file outside `.baseharbor` state.

## CLI

Build the single operator binary:

```bash
go build -o baha ./cmd/baha
```

Discover commands at every level:

```bash
./baha --help
./baha app --help
./baha openbao --help
```

Create a PostgreSQL + Valkey application runtime:

```bash
./baha app create demo --postgres --redis
./baha app plan demo
./baha app preflight demo
./baha app apply demo
./baha app status demo
./baha app doctor demo
```

Create an application with a managed OpenBao scope:

```bash
./baha app create secure-demo --postgres --secrets
./baha app apply secure-demo
./baha app status secure-demo
./baha app doctor secure-demo
```

The manifest retains the `redis` service name for compatibility with Redis-protocol consumers, while the managed implementation is Valkey.

Lifecycle operations:

```bash
./baha app down demo
./baha app up demo
./baha app destroy demo
./baha app destroy demo --yes
```

`baha app down` removes managed containers and the transient network while preserving all managed data volumes, runtime state, credentials and optional OpenBao application scope.

`baha app up` resumes only an already-materialized runtime. It validates ownership and the managed runtime definition, requires every expected persistent volume instead of silently recreating missing state, and verifies any managed OpenBao identity before and after start.

`baha app destroy` is destructive by design. Without `--yes` it performs the safety preflight and prints the managed resources that would be removed, but makes no changes. With `--yes`, BaseHarbor verifies the generated runtime definition, exact Compose ownership and optional OpenBao AppRole/policy ownership before permanent deletion.

Application convergence follows the stable contract:

```text
plan -> preflight -> apply -> verify
```

Lifecycle resume and destructive operations add explicit ownership/state verification before mutation and post-verification after mutation.

## OpenBao bootstrap

Start the control-plane runtime first:

```bash
./baha up
```

On a fresh single-node installation, initialize the bundled OpenBao instance with an explicitly selected recovery file:

```bash
./baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
./baha openbao status
```

The recovery destination is mandatory, is created owner-only, and must be outside `.baseharbor`. BaseHarbor does not print the unseal key. The initial root token is used only in-memory during bootstrap, then revoked after the restricted manager AppRole has been verified.

After an OpenBao restart, the Shamir-sealed single-node profile requires explicit unseal:

```bash
./baha openbao unseal --recovery-file /secure/off-host/openbao-recovery.json
./baha openbao status
```

The recovery file should be stored separately from the host/application data it protects. Automatic KMS/HSM/transit unseal remains a later deployment profile; the current implementation deliberately follows the roadmap requirement to support an explicit manual unseal workflow first.

Managed application scopes are now provisioned server-side, but workload credential injection and direct workload connectivity to the bundled loopback-only OpenBao listener are not yet claimed as complete. The existing network credential adapter remains HTTPS-only; local bootstrap/convergence uses the container-runtime boundary and does not weaken that contract.

See [docs/cli.md](docs/cli.md), [docs/runtime-compose.md](docs/runtime-compose.md), [docs/secrets-and-openbao.md](docs/secrets-and-openbao.md), [docs/architecture.md](docs/architecture.md), [docs/roadmap.md](docs/roadmap.md), and the mandatory [development guidelines](docs/DEVELOPMENT_GUIDELINES.md).

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

## Planned platform capabilities

```text
BaseHarbor
├── baha CLI
├── shared control plane
├── isolated application service stacks
├── auth / authorization
├── PostgreSQL
├── Valkey (Redis protocol)
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
