# BaseHarbor

Secure, modular, self-hosted application backend runtime managed through the `baha` CLI.

BaseHarbor provides reusable backend infrastructure for independent applications without forcing those applications into one monolith or a proprietary data-access SDK.

> BaseHarbor should hide operational complexity without hiding standard interfaces.

## Status

Early development. Identity, authorization, tenancy, secrets foundations, PostgreSQL migrations, database-enforced tenant isolation, the single-node control-plane runtime, and the declarative application resource model are in place.

Per-application runtime convergence supports dedicated PostgreSQL and Valkey services. Applications can also request an isolated managed OpenBao secret scope alongside PostgreSQL and/or Valkey. `baha app apply NAME` creates the isolated Compose runtime, provisions the application OpenBao policy/AppRole when requested, and reports success only after protocol/authentication verification.

PostgreSQL readiness requires an authenticated `SELECT 1`. Valkey readiness requires an authenticated `PING` returning `PONG`. Valkey uses the official `valkey/valkey:9.1.2-alpine` image with AOF persistence enabled.

Managed secrets use isolated per-application/environment KV documents plus an application-specific `baseharbor-app-<app>-<environment>` policy/AppRole. Application RoleID/SecretID bootstrap state is owner-only; application secret payloads are not stored in the manifest or local runtime environment files.

Applications can declare required secret names as part of their manifest contract. `baha app apply` and `baha app up` fail closed before workload start when a required secret is missing or unreadable. `status` and `doctor` report only `present`/`usable` metadata and never reveal values.

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

Create an application with required managed secrets:

```bash
./baha app create secure-demo \
  --postgres \
  --require-secret OPENAI_API_KEY \
  --require-secret SMTP_PASSWORD

./baha app plan secure-demo
./baha app preflight secure-demo
./baha app apply secure-demo
```

The first `apply` may materialize the runtime definition and isolated OpenBao scope, but it will not start the workload while required secret values are missing. Configure them without exposing values on the command line:

```bash
printf '%s' "$OPENAI_API_KEY" | ./baha app secret set secure-demo OPENAI_API_KEY --stdin
printf '%s' "$SMTP_PASSWORD" | ./baha app secret set secure-demo SMTP_PASSWORD --stdin
./baha app apply secure-demo
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

`baha app up` resumes only an already-materialized runtime. It validates ownership and the managed runtime definition, requires every expected persistent volume instead of silently recreating missing state, verifies any managed OpenBao identity, and refuses to start when a required secret is missing or unusable.

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

Managed application scopes are provisioned server-side. Runtime delivery of secret values remains a separate provider concern: BaseHarbor may later use in-memory files, explicit environment injection, workload identity/OpenBao, or Kubernetes-native secret projection without changing the `secrets.required` application contract.

See [docs/application-contract.md](docs/application-contract.md), [docs/cli.md](docs/cli.md), [docs/runtime-compose.md](docs/runtime-compose.md), [docs/secrets-and-openbao.md](docs/secrets-and-openbao.md), [docs/architecture.md](docs/architecture.md), [docs/roadmap.md](docs/roadmap.md), and the mandatory [development guidelines](docs/DEVELOPMENT_GUIDELINES.md).

## Design goals

- one dependable binary for setup and lifecycle management
- secure defaults, least privilege and fail-closed behavior
- isolated backend service stacks for independent applications
- native protocols and standard interfaces for application consumption
- applications remain runnable without BaseHarbor when equivalent standard interfaces are supplied elsewhere
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
