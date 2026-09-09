# BaseHarbor

[![CI](https://github.com/mcpdev80/baseharbor/actions/workflows/ci.yml/badge.svg)](https://github.com/mcpdev80/baseharbor/actions/workflows/ci.yml)
[![GitHub Release](https://img.shields.io/github/v/release/mcpdev80/baseharbor?display_name=tag&sort=semver)](https://github.com/mcpdev80/baseharbor/releases)
[![License](https://img.shields.io/github/license/mcpdev80/baseharbor)](LICENSE)

Secure, modular, self-hosted application backend runtime managed through the `baha` CLI.

BaseHarbor provides reusable backend infrastructure for independent applications without forcing those applications into one monolith or a proprietary data-access SDK.

> BaseHarbor should hide operational complexity without hiding standard interfaces.

## Status

BaseHarbor is under active development and currently follows the `0.x` Semantic Versioning policy. Real products should consume published releases, not the moving `main` branch.

Identity, authorization, tenancy, secrets foundations, PostgreSQL migrations, database-enforced tenant isolation, the single-node control-plane runtime, and the declarative application resource model are in place.

Per-application runtime convergence supports dedicated PostgreSQL and Valkey services. Applications can also request an isolated managed OpenBao secret scope alongside PostgreSQL and/or Valkey. `baha app apply NAME` creates the isolated Compose runtime, provisions the application OpenBao policy/AppRole when requested, and reports success only after protocol/authentication verification.

PostgreSQL readiness requires an authenticated `SELECT 1`. Valkey readiness requires an authenticated `PING` returning `PONG`. Valkey uses the official `valkey/valkey:9.1.2-alpine` image with AOF persistence enabled.

Managed secrets use an isolated application/environment namespace plus an application-specific `baseharbor-app-<app>-<environment>` policy/AppRole. Application RoleID/SecretID bootstrap state is owner-only; application secret payloads are not stored in the manifest or local runtime environment files.

Applications can declare required secret names as part of their manifest contract. `baha app apply` and `baha app up` fail closed before workload start when a required secret is missing or unreadable. `preflight`, `status` and `doctor` report only presence/usability metadata and never reveal secret values.

Application secret operations share one internal service boundary across the CLI and HTTP handler contract. The protected HTTP chain now includes bearer parsing, concrete OIDC/JWKS verification, fail-closed tenant resolution, RBAC and PostgreSQL-backed application ownership. Pre-tenant membership lookup remains under a dedicated SELECT-only identity-scoped RLS policy rather than using superuser or `BYPASSRLS` access. The network listener is still intentionally disabled until runtime OIDC/database/listener configuration and startup verification are wired explicitly.

The runtime can be inspected with `baha app status NAME`, diagnosed with `baha app doctor NAME`, stopped without deleting persistent data or the OpenBao scope with `baha app down NAME`, resumed from existing materialized state with `baha app up NAME`, and permanently removed through the ownership-verified `baha app destroy NAME --yes` path.

The bundled OpenBao control-plane runtime has an explicit bootstrap and unseal workflow. BaseHarbor initializes OpenBao without persisting or printing the initial root token, creates the `baseharbor/` KV v2 mount, establishes a restricted manager AppRole, verifies it, and revokes the initial root token. Shamir unseal material is written only to an operator-selected recovery file outside managed runtime state.

## Install `baha`

### Recommended: install a released binary

Stable releases publish Linux amd64 and arm64 archives with SHA-256 checksums and GitHub build-provenance attestations.

Install the latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/mcpdev80/baseharbor/main/scripts/install.sh | bash
```

For production automation, pin the installer and the BaseHarbor version to an immutable release tag:

```bash
curl -fsSL https://raw.githubusercontent.com/mcpdev80/baseharbor/v0.1.0/scripts/install.sh \
  | bash -s -- v0.1.0
```

The installer verifies the release archive against the published SHA-256 checksum before installing `baha` to `~/.local/bin/baha` by default.

Verify the installed binary:

```bash
baha version
```

### Development: build from source

Building from `main` is for contributors and development environments:

```bash
go build -o baha ./cmd/baha
```

A source build without release linker metadata reports itself as a development build and should not be treated as a production release.

## CLI

Discover commands at every level:

```bash
baha --help
baha app --help
baha openbao --help
```

Create a PostgreSQL + Valkey application runtime:

```bash
baha app create demo --postgres --redis
baha app plan demo
baha app preflight demo
baha app apply demo
baha app status demo
baha app doctor demo
```

Create an application with required managed secrets:

```bash
baha app create secure-demo \
  --postgres \
  --require-secret OPENAI_API_KEY \
  --require-secret SMTP_PASSWORD

baha app plan secure-demo
baha app preflight secure-demo
baha app apply secure-demo
```

The first `apply` may materialize the runtime definition and isolated OpenBao scope, but it will not start the workload while required secret values are missing or unusable. Configure them without exposing values on the command line:

```bash
printf '%s' "$OPENAI_API_KEY" | baha app secret set secure-demo OPENAI_API_KEY --stdin
printf '%s' "$SMTP_PASSWORD" | baha app secret set secure-demo SMTP_PASSWORD --stdin
baha app apply secure-demo
```

The manifest retains the `redis` service name for compatibility with Redis-protocol consumers, while the managed implementation is Valkey.

Lifecycle operations:

```bash
baha app down demo
baha app up demo
baha app destroy demo
baha app destroy demo --yes
```

`baha app down` removes managed containers and the transient network while preserving all managed data volumes, runtime state, credentials and optional OpenBao application scope.

`baha app up` resumes only an already-materialized runtime. It validates ownership and the managed runtime definition, requires every expected persistent volume instead of silently recreating missing state, verifies any managed OpenBao identity, and refuses workload start when a required secret is missing or unusable.

`baha app destroy` is destructive by design. Without `--yes` it performs the safety preflight and prints the managed resources that would be removed, but makes no changes. With `--yes`, BaseHarbor verifies the generated runtime definition, exact Compose ownership and optional OpenBao AppRole/policy ownership before permanent deletion.

Application convergence follows the stable contract:

```text
plan -> preflight -> apply -> verify
```

Lifecycle resume and destructive operations add explicit ownership/state verification before mutation and post-verification after mutation.

## OpenBao bootstrap

Start the control-plane runtime first:

```bash
baha up
```

On a fresh single-node installation, initialize the bundled OpenBao instance with an explicitly selected recovery file:

```bash
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

The recovery destination is mandatory, is created owner-only, and must be outside managed runtime state. BaseHarbor does not print the unseal key. The initial root token is used only in-memory during bootstrap, then revoked after the restricted manager AppRole has been verified.

After an OpenBao restart, the Shamir-sealed single-node profile requires explicit unseal:

```bash
baha openbao unseal --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

The recovery file should be stored separately from the host/application data it protects. Automatic KMS/HSM/transit unseal remains a later deployment profile; the current implementation deliberately follows the roadmap requirement to support an explicit manual unseal workflow first.

Managed application scopes and operator secret management are implemented. Runtime delivery remains a separate provider concern: BaseHarbor may later use in-memory files, explicit environment injection, workload identity/OpenBao, or Kubernetes-native secret projection without changing the `secrets.required` application contract.

## Releases and compatibility

BaseHarbor follows Semantic Versioning. While the project is in the `0.x` series, patch releases stay backward-compatible within a minor line while a new minor line may contain explicitly documented breaking changes.

Production consumers should pin a compatible release range and must not silently follow `main`.

See [CHANGELOG.md](CHANGELOG.md) and [docs/releases.md](docs/releases.md) for the complete release, compatibility and verification policy.

See also [docs/application-contract.md](docs/application-contract.md), [docs/application-secret-api.md](docs/application-secret-api.md), [docs/authentication.md](docs/authentication.md), [docs/cli.md](docs/cli.md), [docs/runtime-compose.md](docs/runtime-compose.md), [docs/secrets-and-openbao.md](docs/secrets-and-openbao.md), [docs/architecture.md](docs/architecture.md), [docs/roadmap.md](docs/roadmap.md), and the mandatory [development guidelines](docs/DEVELOPMENT_GUIDELINES.md).

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
