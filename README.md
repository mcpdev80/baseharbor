<h1 align="center">BaseHarbor</h1>

<h2 align="center">One application contract. Replaceable infrastructure.</h2>

<p align="center"><strong>
A command-line tool that turns <em>"I just want to build an app"</em>
into provisioned, secured, verified infrastructure —
on your own machine, no vendor, no lock-in.
</strong></p>

<p align="center">
  <a href="https://github.com/mcpdev80/baseharbor-demo">
    <img src="https://img.shields.io/badge/demo-baseharbor--demo-6f42c1" alt="BaseHarbor Demo App">
  </a>
  <a href="https://github.com/mcpdev80/baseharbor/releases">
    <img src="https://img.shields.io/github/v/release/mcpdev80/baseharbor?display_name=tag&sort=date" alt="GitHub Release">
  </a>
  <a href="https://mcpdev80.github.io/baseharbor/">
    <img src="https://img.shields.io/badge/docs-GitHub%20Pages-0068E9" alt="GitHub Pages">
  </a>
  <a href="CONTRIBUTING.md">
    <img src="https://img.shields.io/badge/contributing-welcome-2ea44f" alt="Contributing">
  </a>
  <a href="LICENSE">
    <img src="https://img.shields.io/github/license/mcpdev80/baseharbor" alt="License">
  </a>
</p>

## BaseHarbor Core

The Core has three mandatory capabilities: **SQL, Secrets and Identity**.
The reference providers are PostgreSQL, OpenBao and Keycloak. The Web Console is optional.
TLS and protected credentials also apply to local development.

Core bootstrap does not require an application or repository. The first application
flow offers setup when needed and continues after verified Core readiness.
Core capabilities are mandatory; provider placement may be shared or
application-isolated. Additional isolation can add provider instances and resource use.
Measured memory figures must distinguish idle/stabilized usage, startup/convergence
peak and the simultaneous Core total; unavailable measurements are reported explicitly.

## Public companion implementations

BaseHarbor keeps optional user-facing and remote-access implementations in separate public repositories while Core remains authoritative:

- [BaseHarbor Console](https://github.com/mcpdev80/baseharbor-console) — optional visual client over the protected Core machine/HTTP contracts. It has no separate deployment state, RBAC, secret store or direct runtime/Connector control path.
- [BaseHarbor Node Connector](https://github.com/mcpdev80/baseharbor-node-connector) — optional Target Access implementation for remote Docker/Podman-style hosts. It is not a control plane or Runtime Provider and exposes no unauthenticated inbound management listener.

Kubernetes/OpenShift normally use their native authenticated APIs and do not require the Node Connector. Support claims for Console or Connector require pinned cross-repository evidence; repository presence or contract shape alone is not support evidence.

## The problem

You know the drill. You want to build an app — and before the first feature
works, you are configuring a database container, networking, ports, secrets,
TLS, identity, backups and a monitoring stack.

BaseHarbor takes that work off your plate. It is a normal CLI —
deterministic, no AI involved:

```bash
baha app init      # adopt your existing repo, or plan a new app
baha up            # provision, connect, secure, verify
baha status        # done — with evidence, not guesswork
```

Your app ends up at a stable, verified HTTPS URL (`demo.baha.localhost`
by default), with identity, secrets and metrics on their own canonical
hostnames — no port hunting, no certificate setup.

Your app receives normal bindings (`DATABASE_URL`, `REDIS_URL`,
`S3_ENDPOINT`, `OIDC_ISSUER`, …) and keeps using its normal libraries.
BaseHarbor is not a framework, not a PaaS, and never touches your code.

## If you know Terraform, you already understand BaseHarbor

Terraform made infrastructure declarative: describe what you need, and it
provisions it and keeps it from drifting — on AWS, Azure or GCP, without
rewriting your code.

BaseHarbor does the same — one layer up, for your **application's needs**
instead of raw infrastructure:

```text
Terraform:   "I need a VM, a network, a DNS record."
BaseHarbor:  "My app needs SQL, a cache, secrets, object storage,
              identity, HTTPS exposure — provision it, wire it up, verify it."
```

Your `baseharbor.yaml` is the HCL of your application. Change the Target,
keep the contract: Docker / Podman today, Kubernetes later — the way the
same Terraform file targets different clouds.

BaseHarbor is **not** Terraform and does not replace it. It answers a
question Terraform deliberately never covered: not *"does the resource
exist?"* but *"does the application actually work?"*

## What you get

- **Create new applications** — `baha app new` turns capability intent into a normal Go, Next.js, Python or Quarkus project, validates it, and keeps ecosystem-native libraries instead of introducing a BaseHarbor app framework.
- **Multi-repo workspaces** — one logical application can span monorepos, multiple existing worktrees and OCI components; guided workspace mapping keeps local checkout paths out of portable intent. `baha app workspace status` and fast-forward-only `workspace update` provide safe Git synchronization without hiding normal Git decisions.
- **Adopt existing repositories** — `baha` inspects your code, dependencies,
  workload sources and ports deterministically (parsers, rules, repository
  evidence — no LLM or external AI service). v0.4.20 understands Compose,
  repository-authored Podman Quadlet and raw Kubernetes YAML through one
  source-neutral evidence model, while keeping source syntax out of portable intent.
- **Declare needs, not products** — SQL, cache, durable key-value, document database,
  queue/pub-sub/stream messaging, secrets, object storage, identity and HTTPS exposure
  are logical capabilities; PostgreSQL, Valkey, MongoDB, RabbitMQ, S3-compatible
  storage and Keycloak are replaceable realizations underneath.
- **Resource-aware local mutation** — Docker/Podman preflight checks host memory headroom before starting providers/workloads; future Kubernetes/OpenShift providers use cluster-native capacity/quota/scheduling evidence instead of CLI-host memory.
- **Everything wired and verified** — provisioning, credentials, TLS, bindings,
  backups, updates, drift detection. READY means the real protocol/data flow
  was verified, not just that a container is up.
- **Shared providers, isolated resources** — one Target-owned provider can
  serve many applications while databases, cache resources, credentials,
  bindings and destroy ownership remain application-isolated.
  `application   shared | external` placement, failing closed when unsupported.
- **Managed identity** — provider-neutral OIDC/OAuth2 with managed Keycloak
  or external providers, exposure-derived redirect/logout URIs, portable
  MFA/passkey policy, Service Binding 1.1 projection.
- **Canonical local development access** — one Target-scoped domain
  (default `baha.localhost`) with deterministic HTTPS names such as
  `my-app.baha.localhost`, `pgadmin.baha.localhost`, `auth.baha.localhost`
  — instead of exposing random loopback ports as developer UX.
- **Optional provider management surfaces** — pgAdmin, Redis Commander,
  Mongo Express, RabbitMQ Management, SeaweedFS Admin, OpenBao, Keycloak and
  Prometheus, selected per capability,
  never duplicated per application, environment-policy driven.
- **Deployment Targets** — Target + Application + Environment, with
  target-scoped state; Docker / Podman today, Kubernetes / OpenShift planned.
- **Environment-aware operator access** — trusted local operation in dev;
  authenticated OIDC operator access in test/prod, separate from
  application-user identity.
- **Standard interfaces only** — PostgreSQL · Redis/Valkey · MongoDB · AMQP 0-9-1 ·
  S3 · HTTP · OIDC/OAuth2 · OTLP · environment variables · files. No BaseHarbor SDK,
  no imports in your app.
- **Zero-trust by default** — least privilege, scoped credentials,
  explicit trust boundaries, fail-closed behavior.

> **Runtime status:** Docker and Podman are implemented Runtime Providers
> behind the same portable contract. Repository understanding is separate from
> runtime realization: v0.4.20 can inspect/adopt Compose, repository-authored
> Quadlet and raw Kubernetes YAML. The current repository-workload realization
> path remains Compose-based for Docker/Podman; Podman realizes that portable
> workload through native Quadlet units managed by rootless `systemd --user`.
> Kubernetes and OpenShift Runtime Providers are planned and are not implemented yet.

## A CLI first — agent-ready if you want it

BaseHarbor is a command-line tool that automates your infrastructure work.
Commands such as `inspect`, `init`, `plan`, `up`, `status`, `doctor`
and policy checks are deterministic BaseHarbor functionality implemented in
code. It has nothing to do with AI — unless you want it to:

```bash
baha agent describe -o json
baha mcp serve
```

The same operations you run by hand are exposed as structured JSON and a
local **MCP interface**. That is the only place AI enters the picture: if you
use an AI agent, it can operate BaseHarbor through these bounded, verified
operations instead of raw shell access. No generic shell, no Docker socket,
no unrestricted runtime execution. The MCP interface is an optional
integration surface — it does not change how BaseHarbor detects, plans or
verifies infrastructure. Without an agent, none of this matters: it is just
a CLI doing work for you.

The [coverage matrix](docs/reference/cli-machine-coverage.md) lists every supported
command, its JSON/MCP equivalent and explicit host or presentation modes.
[Automation examples](docs/how-to/machine-operations.md) cover protected secret
files, workspaces, trust and control-plane operations. Extension publishers can
run the [public conformance profile](docs/spec/provider-conformance-v1.md);
[artifact verification and trust policy](docs/spec/extension-artifact-trust-v1.md)
remain separate checks.

## Application contract

The application describes **what it needs**, not how infrastructure must be
implemented:

```yaml
version: 1

app:
  name: my-app
  environment: production

services:
  sql:
    enabled: true

  cache:
    enabled: true

  object_storage:
    buckets:
      uploads: {}

  identity:
    enabled: true

secrets:
  required:
    - name: APP_SECRET
```

BaseHarbor exposes normal application-facing bindings such as:

```text
DATABASE_URL
REDIS_URL
VALKEY_URL
S3_ENDPOINT
S3_BUCKET
OIDC_ISSUER
OIDC_CLIENT_ID
OIDC_CLIENT_SECRET_FILE
OIDC_CA_FILE
APP_SECRET
```

Your application keeps using native ecosystem clients, protocols and
frameworks — standard OIDC libraries, standard database drivers, no
BaseHarbor SDK.

## Local development access

For `dev`, BaseHarbor keeps local access predictable without changing the
portable application contract.

The effective Target owns one development domain, defaulting to
`baha.localhost`. Browser-facing application and provider surfaces receive
deterministic HTTPS names such as:

```text
my-app.baha.localhost
pgadmin.baha.localhost
auth.baha.localhost
secrets.baha.localhost
metrics.baha.localhost
```

Docker serves these canonical development hosts on HTTPS port 443. Rootless
Podman uses the fixed unprivileged HTTPS port 8443 and reports that port in
canonical URLs; no host sysctl change is required.

A Target- and environment-scoped development account defaults to username
`developer`, but the username is configurable. Its generated strong password
and identity are reused across selected development management surfaces. When managed Identity is present, the same developer identity is
reconciled through OIDC. The password is never printed by normal `status`,
`doctor`, plan or evidence output. Test and prod do not use this shared
development credential; they keep the authenticated operator-OIDC boundary.

Use the explicit commands when you need to inspect or change local
development access:

```bash
baha dev domain
baha dev domain dev.example.internal
baha dev credentials
baha dev credentials --username USER
baha dev credentials --reset
```

## What BaseHarbor is not

- **Not a PaaS** — self-hosted, runs on your Docker or Podman, no vendor.
- **Not a framework** — your code stays normal Go/Python/Node; no SDK,
  no BaseHarbor APIs in your app.
- **Not a Kubernetes distribution** — Docker/Podman today; Kubernetes is a
  future runtime for the *same* contract.
- **Not an AI product** — a deterministic CLI that automates infrastructure;
  agents can optionally drive it via MCP.
- **Not Terraform** — it does not manage cloud resources; it is the
  declarative layer *above* them, for your application.

## Security is behavior

BaseHarbor does not treat security as a label. Current behavior includes:

- deny-by-default and fail-closed decisions;
- scoped application/runtime credentials;
- bucket-scoped S3 credentials;
- application-scoped runtime identities;
- standard OIDC application identity with file-based trust material for
  managed/private issuers;
- authenticated BaseHarbor operator boundaries for test/prod, separate from
  application-user identity;
- explicit directional cross-application connectivity;
- protected deployment state;
- real protocol/data-flow readiness checks;
- workload preflight checks for risky Compose settings such as privileged
  mode, runtime sockets, host networking, dangerous capabilities, devices
  and critical host mounts.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/mcpdev80/baseharbor/main/scripts/install.sh | bash
baha version
```

## Learn more

- [Documentation home](docs/index.md)
- [Getting started](docs/tutorials/getting-started.md)
- [Architecture](docs/explanation/architecture.md)
- [Application contract](docs/explanation/application-contract.md)
- [Providers](docs/explanation/providers.md)
- [Targets and deployment destinations](docs/explanation/targets.md)
- [CLI reference](docs/reference/cli.md)
- [Normative specifications](docs/spec/README.md)
- [Roadmap](docs/roadmap.md)

## Project status

BaseHarbor is **pre-v1**. Manifest v1 is the current v0.4 compatibility surface.

Docker and Podman are the implemented Runtime Providers today. The portable
application contract uses logical workload components rather than source-native
Compose/Quadlet/Kubernetes identity. Compose is currently the realizable
repository-workload source for Docker/Podman; raw Kubernetes and repository-authored
Quadlet can already be inspected/adopted but require future realization support.
Kubernetes and OpenShift remain future runtime tracks and must preserve the same
application contract when implemented.

Normal feature, fix, chore and dependency pull requests target `develop`.
The `main` branch represents released source.

## License

Apache License 2.0. See [LICENSE](LICENSE).