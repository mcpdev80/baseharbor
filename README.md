# BaseHarbor

[![CI](https://github.com/mcpdev80/baseharbor/actions/workflows/ci.yml/badge.svg)](https://github.com/mcpdev80/baseharbor/actions/workflows/ci.yml)
[![GitHub Release](https://img.shields.io/github/v/release/mcpdev80/baseharbor?display_name=tag&sort=semver)](https://github.com/mcpdev80/baseharbor/releases)
[![License](https://img.shields.io/github/license/mcpdev80/baseharbor)](LICENSE)

## Your application declares what infrastructure it needs. BaseHarbor handles the infrastructure underneath.

BaseHarbor inspects existing applications, turns infrastructure requirements into a portable contract, and provisions those capabilities through interchangeable providers — without coupling application code to the underlying infrastructure.

> Describe requirements once. Keep the infrastructure underneath replaceable.

```text
                         YOUR APPLICATION
                                |
                                v
                        baha app inspect
                                |
                                v
                         baseharbor.yaml
                                |
                                v
                       +----------------+
                       |   BaseHarbor   |
                       |                |
                       | inspect        |
                       | plan           |
                       | preflight      |
                       | provision      |
                       | bind           |
                       | verify         |
                       | reconcile      |
                       +-------+--------+
                               |
                  +------------+------------+
                  |            |            |
                  v            v            v
               Compose     Kubernetes    OpenShift
              available      planned       planned
                  |
                  v
        PostgreSQL · Valkey · S3 · Secrets
          HTTP · OTLP · Metrics · Logs · Traces
```

Compose is the complete runtime implementation today. Kubernetes and OpenShift are future runtime providers that must preserve the same application contract.

## Five-minute adoption path

For an existing repository, the normal v0.4.11 developer path is:

```bash
baha app inspect .
baha app init
baha plan
baha up -e dev
baha status
baha doctor
```

Use `-o json` on inspect/plan/status/doctor for secret-safe structured read-only results. `baha app init --agents` can add or update only BaseHarbor's bounded section in `AGENTS.md`.

See [Five-minute onboarding](docs/five-minute-onboarding.md).

## 30-second demo

Start with an existing application:

```bash
cd my-app

# Inspect the repository without changing it
baha app inspect .

# Create/review the portable application contract
baha app init

# See what BaseHarbor intends to do
baha app plan

# Converge infrastructure and workload
baha app apply
```

Repository inspection reports evidence as:

```text
Detected
  PostgreSQL usage
  Redis/Valkey usage
  HTTP workload port

Suggested
  S3-compatible object storage
  OpenTelemetry export

Possible
  secret names
  additional runtime operations
```

Detection is evidence-based:

- **Detected** means strong evidence.
- **Suggested** requires developer confirmation.
- **Possible** is weak evidence and is never silently adopted.
- Inspection is read-only.
- Repository evidence never silently grants permissions or creates infrastructure.

That distinction is intentional: BaseHarbor should discover aggressively, but mutate conservatively.

## The idea

Applications know **what** they need. Infrastructure knows **how** to provide it. BaseHarbor connects the two.

```text
Application
    |
    v
"I need SQL, cache, S3, secrets and metrics."
    |
    v
Portable application contract
    |
    v
Capability resolution
    |
    v
Provider + placement
    |
    v
Actual infrastructure
```

The application should not need to know whether a capability is provided by a local Compose service, a shared platform service, an external system, or a future Kubernetes/OpenShift implementation.

### One contract. Different infrastructure.

```text
                         baseharbor.yaml
                                |
                   +------------+------------+
                   |            |            |
                   v            v            v
                Compose     Kubernetes    OpenShift
               available      planned       planned
                   |
                   v
               providers
```

**The application intent stays provider-neutral. The provider/runtime realization can change underneath it.**

## A real application contract

```yaml
version: 1

app:
  name: my-app
  environment: production

services:
  postgres:
    enabled: true

  redis:
    enabled: true

  object_storage:
    buckets:
      uploads: {}

secrets:
  required:
    - name: APP_SECRET
```

BaseHarbor materializes standard application-facing bindings such as:

```text
DATABASE_URL
REDIS_URL
VALKEY_URL
S3_ENDPOINT
S3_BUCKET
APP_SECRET
```

Applications keep using native ecosystem clients and protocols. They do not need a BaseHarbor SDK in their business code.

Manifest v1 is the current v0.4 compatibility surface. Provider-specific topology, credentials and runtime state stay outside the portable application intent.

## Bring your existing application

BaseHarbor does not require a rewrite.

```bash
baha app inspect .
baha app inspect . --json
```

The shared repository-inspection core understands evidence from sources such as:

- Compose files and Dockerfiles;
- dependency manifests;
- environment-variable **names**;
- source imports and configuration;
- published ports and health checks;
- PostgreSQL and Redis/Valkey usage;
- S3-compatible usage and likely runtime bucket creation;
- OpenMetrics endpoints;
- OTLP export.

`baha app init` consumes the same inspection model to help create the initial contract.

Weak evidence remains a suggestion. Existing contract state is not deleted simply because current repository evidence becomes stale.

## What BaseHarbor provides today

| Need | Portable boundary | Current reference implementation |
| --- | --- | --- |
| SQL | capability lifecycle | PostgreSQL |
| Cache / key-value | capability lifecycle | Valkey / Redis protocol |
| Secrets / trust | secure bindings | OpenBao |
| S3-compatible storage | `object-storage.s3/v1` | SeaweedFS |
| HTTP exposure | `exposure.http/v1` | Caddy |
| Telemetry export | `telemetry.otlp/v1` | OpenTelemetry Collector or external OTLP |
| Metrics | `metrics/v1` | Prometheus 3.14.0 |
| Centralized logs | `logs/v1` | Loki 3.7.8 + Grafana Alloy 1.19.2 |
| Trace storage | `traces/v1` platform facility over `telemetry.otlp/v1` | Tempo 3.0.2 |
| Runtime-created resources | Runtime Resource API | Application Runtime Broker + Provider Executor |
| Cross-app access | explicit directed policy | hardened Compose relay |

Built-in components are reference providers behind versioned semantic boundaries rather than application-facing product requirements.

## Provider placement

Provider placement is platform/operator state, not application intent.

```text
application
    dedicated provider instance for one application/environment

shared
    platform provider instance
    + optional named sharing boundary

external
    lifecycle owned outside BaseHarbor
```

A shared provider does **not** mean every application can access it.

Access stays explicit and deny-by-default. Logical resources remain application-owned even when the backing provider is shared.

Unsupported requested placement fails closed before mutation; BaseHarbor does not silently downgrade to another placement.

## Built for interchangeable providers

BaseHarbor owns the capability semantics and lifecycle. Providers implement them.

```text
Capability Specification
        |
        v
Provider Integration Contract
        |
        +--> built-in/reference provider
        |
        +--> future external provider
```

Provider Integration Contract v1 defines the common lifecycle model:

```text
plan
  -> preflight
  -> provision
  -> bind
  -> verify
  -> reconcile
```

v0.4.9 also includes executable provider conformance and deterministic fault-injection coverage for:

- CREATE;
- repeated NOOP convergence;
- DRIFT -> REPAIR;
- provider unavailable;
- malformed binding;
- verify failure;
- retry convergence;
- ownership-safe destroy;
- secret-safe diagnostics.

The repository already contains the future language-neutral provider schema in `spec/provider/v1/provider.proto`.

gRPC/Protocol Buffers and OCI are the planned open transport and packaging boundary for future external providers. A dynamic external-provider loader is **not implemented yet**.

See [Provider Integration Contract](docs/provider-integration-contract.md).

## Security is behavior, not a marketing label

BaseHarbor aims for least privilege, deny by default and fail closed.

Concrete properties in the current v0.4 line include:

- secrets are not stored in portable application manifests;
- application/runtime credentials are scoped rather than provider-global;
- S3 credentials are bucket-scoped;
- runtime identities and brokers are application-scoped;
- cross-application connectivity is explicit and directional;
- shared providers do not imply shared application access;
- unsupported provider placement fails before mutation;
- readiness verifies real protocols/data flow instead of only checking container state;
- ambiguous repository findings require confirmation;
- protected deployment/provider state uses owner-only permissions.

### Compose workload security preflight

Before BaseHarbor starts a repository-provided Compose workload, it evaluates the **fully rendered Compose configuration**.

Managed environments fail closed for isolation-breaking settings such as:

- `privileged: true`;
- Docker/Podman runtime socket mounts;
- `network_mode: host`;
- host PID/IPC namespaces;
- dangerous `cap_add` values;
- host device mappings;
- critical host mounts such as `/`, `/etc`, `/run`, `/var/run` and `/dev`.

Development can explicitly acknowledge selected exceptions where policy permits.

The v0.4.9 Loki/Alloy provider itself runs without Docker/Podman socket access, uses read-only roots, drops Linux capabilities, sets `no-new-privileges` and binds host-facing listeners to loopback.

## Metrics without Prometheus in application intent

An application can expose OpenMetrics-compatible endpoints while platform policy decides whether and where to collect them.

The current reference implementation provides:

- `metrics/v1`;
- shared or application-scoped Prometheus;
- optional named shared boundaries;
- isolated per-application metrics networks;
- automatic target registration;
- real scrape/ingestion verification.

Prometheus is a provider implementation, not part of the portable application identity. In v0.4.10, BaseHarbor-managed providers may also advertise safe OpenMetrics endpoints through the generic provider contract; when metrics collection is enabled, the selected Prometheus instance registers only policy-authorized provider signals for its placement/sharing boundary.

## Centralized logs without Loki in application intent

Applications already produce logs. Platform policy decides whether those logs are collected centrally.

v0.4.9 adds:

- `logs/v1`;
- Loki 3.7.8 as the reference log store/query provider;
- Grafana Alloy 1.19.2 as collector/forwarder;
- shared or application-scoped placement;
- optional named shared boundaries;
- RFC5424 forwarding from selected repository workload services;
- real Loki query verification before the log path is considered ready.

Loki/Alloy do not get the Docker/Podman socket.

`baha app logs` remains an independent trusted-local developer workflow; it does not couple the application to Loki.

## Traces without Tempo in application intent

Applications export traces through `telemetry.otlp/v1`. Trace retention is deployment policy, not a Tempo field in `baseharbor.yaml`. v0.4.10 adds `traces/v1` as the provider-neutral trace-platform contract and Tempo 3.0.2 as the first shared Compose reference provider. When `BASEHARBOR_TRACES_ENABLED=true`, BaseHarbor routes the managed OpenTelemetry Collector to Tempo and verifies a real trace by querying it back. Grafana is not started implicitly.

## Runtime Resource API

Some infrastructure is needed only after an application is running.

The Runtime Resource API allows authorized application-time operations while keeping provider credentials and provider-specific APIs outside application code.

Current examples include runtime S3 bucket lifecycle.

The Application Runtime Broker:

- authenticates the application with BaseHarbor-managed identity;
- authorizes only declared runtime permissions;
- forwards provider-neutral operations;
- persists asynchronous operation state;
- returns scoped bindings rather than provider-global credentials.

Development can expose embedded OpenAPI/Swagger documentation on loopback. Test/staging/production keep interactive docs disabled by default unless operator policy enables them.

## Explicit cross-application connectivity

Provider sharing and application connectivity are separate concerns.

BaseHarbor connectivity is explicit and directional:

```bash
baha connect app-a/api app-b/sql
baha connections
baha disconnect app-a/api app-b/sql
```

The current Compose implementation uses a hardened relay rather than placing both workloads onto one broad shared network.

## BaseHarbor is

- a portable application infrastructure contract;
- a capability/provider abstraction;
- a lifecycle and reconciliation engine;
- a bridge between applications and infrastructure;
- a secure way to keep product/runtime choices outside application intent.

## BaseHarbor is not

- a replacement for your application framework;
- a proprietary database or secret store;
- a Kubernetes distribution;
- a promise that Kubernetes/OpenShift are already implemented;
- a cloud lock-in layer;
- a reason to replace standard infrastructure tools that already solve their layer well.

BaseHarbor can sit **above** technologies such as Compose today and future Kubernetes/OpenShift runtimes. It can also work with externally managed providers rather than requiring ownership of every dependency.

## Install `baha`

Released Linux binaries are the normal installation path.

```bash
curl -fsSL https://raw.githubusercontent.com/mcpdev80/baseharbor/main/scripts/install.sh | bash
```

Verify the installation:

```bash
baha version
```

For production automation, consume an immutable published release rather than following `main`.

Building from source is intended for development/contribution:

```bash
go build -o baha ./cmd/baha
```

## Normal application workflow

```bash
baha app inspect .
baha app init
baha app plan
baha app preflight
baha app apply
baha app status
baha app doctor
```

Repository-aware startup is also available:

```bash
baha up
```

Inspect standard application bindings:

```bash
baha app env
baha app env --format json
```

Trusted-local developer access includes:

```bash
baha app psql
baha app valkey
baha app logs
baha app shell SERVICE
baha app exec SERVICE COMMAND
```

## Current status

BaseHarbor is **pre-v1**. Manifest v1 is the current v0.4 compatibility surface.

### Available now

- Compose runtime;
- repository inspection with Detected / Suggested / Possible evidence;
- application contract and capability lifecycle;
- PostgreSQL and Valkey;
- OpenBao-backed secrets/trust;
- S3-compatible object storage;
- managed HTTP/HTTPS exposure;
- OTLP telemetry;
- Prometheus metrics;
- Loki/Alloy centralized logs;
- Runtime Resource API;
- explicit directional cross-application connectivity;
- backup/restore and update lifecycle;
- provider placement/ownership registry;
- executable provider conformance;
- Compose workload security preflight.

### Planned / future runtime tracks

- dynamic external-provider loading over the defined gRPC/Protobuf + OCI boundary;
- Kubernetes runtime realization;
- OpenShift runtime realization;
- API/Web UI control surfaces over the shared core;
- broader provider ecosystem.

Planned items are architecture targets, not claims of current availability.

## Documentation

Start here:

- [Architecture](docs/architecture.md)
- [Application contract](docs/application-contract.md)
- [Repository workflow](docs/repository-application-workflow.md)
- [Capability / provider model](docs/capability-provider-model.md)
- [Provider Integration Contract](docs/provider-integration-contract.md)
- [CLI reference](docs/cli.md)
- [Roadmap](docs/roadmap.md)
- [Release policy](docs/releases.md)

## Design principles

- application intent describes requirements, not infrastructure products;
- so little as possible, as much as necessary;
- least privilege and deny by default;
- fail closed before unsafe mutation;
- standard protocols over proprietary application APIs;
- one shared core for CLI and future control surfaces;
- real readiness/data-flow verification;
- idempotent reconciliation;
- provider/runtime details remain deployment state;
- Compose remains first-class even as additional runtimes arrive later.

## Contributing

Provider and architecture contributions should preserve the portable application boundary and reuse the existing capability/provider lifecycle rather than adding product-specific paths.

Start with:

- [Development Guidelines](docs/DEVELOPMENT_GUIDELINES.md)
- [Provider Integration Contract](docs/provider-integration-contract.md)
- [Capability Specifications](spec/capabilities/)

## License

Apache License 2.0. See [LICENSE](LICENSE).
