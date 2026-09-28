<p align="center">
  <img src="docs/brand/github_banner.png" alt="BaseHarbor" width="100%">
</p>

<h2 align="center">One application contract. Replaceable infrastructure.</h2>


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

BaseHarbor is a **portable application infrastructure control plane**.

It inspects an application repository, turns infrastructure requirements into one explicit application contract, and realizes that contract through replaceable providers. The application keeps using standard protocols and clients; BaseHarbor owns provider selection, policy, lifecycle, credentials, trust, verification and evidence.

The result is one developer workflow from local containers to later Kubernetes/OpenShift targets without rewriting application infrastructure intent around a specific product.

## The idea

**Inspect what the application already says**  
BaseHarbor deterministically derives repository evidence from code, dependencies, Compose files, ports and configuration. No LLM or external AI service is required for inspection, planning or verification.

**Declare needs, not products**  
The application asks for SQL, cache, S3, secrets, identity, exposure or observability. PostgreSQL, Valkey, SeaweedFS, OpenBao, Keycloak and other products are provider realizations behind that intent.

**Keep runtime and workload source separate**  
Compose is currently a repository workload-source standard. Docker realizes it through Docker Compose; Podman realizes the same portable semantics through native Quadlet + `systemd --user`. Runtime Provider identity is `docker` or `podman`, not `compose`.

**Use standard interfaces at the application boundary**  
PostgreSQL · Redis/Valkey · S3 · HTTP(S) · OIDC/OAuth2 · OTLP · Service Binding · environment variables · files

**Secure the lifecycle, not just the container**  
Least privilege · scoped credentials · explicit trust boundaries · ownership verification · fail-closed behavior · secret-safe evidence

**Make the same control plane usable by humans and agents**  
Human-readable CLI output and structured JSON/MCP expose the same bounded operations. MCP does not provide a generic shell, Docker socket or unrestricted runtime execution.

```bash
baha app inspect .
baha app init
baha target
baha up
baha status
baha doctor
```

> **Runtime status:** Docker and Podman are implemented Runtime Providers behind the same portable contract. Docker uses Docker Compose. Podman uses generated Quadlet units managed by rootless `systemd --user`; there is no `podman compose` fallback. Kubernetes and OpenShift are planned providers and are not implemented yet.

## Why BaseHarbor?

Infrastructure usually becomes application-specific glue: Compose fragments, Helm values, cloud resources, credentials, local setup scripts and environment-specific conventions all describe the same application in different ways.

BaseHarbor puts the stable part in one place:

```text
Application repository
        |
        v
repository evidence
        |
        v
portable baseharbor.yaml intent
        |
        v
BaseHarbor control plane
        |
        +--> Runtime Providers
        +--> Capability Providers
        +--> policy + security
        +--> lifecycle + recovery
        +--> verification + evidence
        |
        v
Docker / Podman today
Kubernetes / OpenShift later
```

The portable contract stays application-facing. Provider- and runtime-specific objects remain realization details.

## What you get

- Deterministic repository inspection with **Detected / Suggested / Possible** evidence.
- Portable application intent with provider-neutral capability and runtime boundaries.
- Explicit Runtime Providers with versioned descriptors, capability negotiation and fail-closed selection.
- PostgreSQL, Valkey/Redis, S3, secrets, managed/external OIDC identity, HTTP exposure, metrics, logs, traces and OTLP.
- Optional provider management surfaces for pgAdmin, Redis Commander, SeaweedFS Admin, OpenBao, Keycloak and Prometheus.
- Canonical local development URLs through one Target-scoped domain (default `baha.localhost`) instead of random loopback ports as normal developer UX.
- Environment-aware operator access: trusted local operation in dev; authenticated OIDC operator access in test/prod.
- A canonical developer path: `baha app init` -> select/inspect Target -> `baha up` -> verified READY.
- First-class deployment Targets with target-scoped runtime/deployment state.
- Plan, preflight, policy and explicit apply for automation and troubleshooting.
- Backup/restore, updates, runtime-created resources and explicit app-to-app connectivity.
- Provider placement for application-scoped, shared or externally managed infrastructure.
- Shared provider lifecycle with application-isolated databases, cache resources, credentials, bindings and destroy ownership.
- Machine-readable results and a versioned local MCP interface for agent workflows.

## Local development access

For `dev`, BaseHarbor keeps local access predictable without changing the portable application contract.

The effective Target owns one development domain, defaulting to `baha.localhost`. Browser-facing application and provider surfaces receive deterministic HTTPS names such as:

```text
my-app.baha.localhost
pgadmin.baha.localhost
auth.baha.localhost
secrets.baha.localhost
metrics.baha.localhost
```

Docker serves these canonical development hosts on HTTPS port 443. Rootless Podman uses the fixed unprivileged HTTPS port 8443 and reports that port in canonical URLs; no host sysctl change is required.

The default reference placement is shared wherever the bundled provider can safely isolate applications. PostgreSQL uses one Target-owned provider with per-application databases and roles. Valkey uses one Target-owned provider lifecycle with isolated per-application cache resources. Dedicated `application` placement remains available when an isolated provider instance is explicitly required.

A Target-scoped development account defaults to username `developer` with a generated strong password. BaseHarbor reuses that identity across selected development management surfaces. When managed Identity is present, the same developer identity is reconciled through OIDC; providers that require native authentication receive an adapter using the same development credentials.

Use the explicit commands when you need to inspect or change local development access:

```bash
baha dev domain
baha dev domain dev.example.internal
baha dev credentials
baha dev credentials --reset
```

The password is never printed by normal `status`, `doctor`, plan or evidence output. Test and prod do not use this shared development credential; they keep the authenticated operator-OIDC boundary.

## Security is behavior

BaseHarbor does not treat security as a label.

Current behavior includes:

- deny-by-default and fail-closed decisions;
- scoped application/runtime credentials;
- bucket-scoped S3 credentials;
- application-scoped runtime identities;
- standard OIDC application identity with file-based trust material for managed/private issuers;
- authenticated BaseHarbor operator boundaries for test/prod, separate from application-user identity;
- explicit directional cross-application connectivity;
- protected deployment state;
- real protocol/data-flow readiness checks;
- workload preflight checks for risky Compose settings such as privileged mode, runtime sockets, host networking, dangerous capabilities, devices and critical host mounts.

## Application contract

The application describes **what it needs**, not how infrastructure must be implemented.

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

Your application keeps using native ecosystem clients and protocols.

## Agent-native, without giving the agent a shell

```bash
baha agent describe -o json
baha mcp serve
```

BaseHarbor itself does not require AI. Commands such as `inspect`, `init`, `plan`, `status`, `doctor` and policy checks are deterministic BaseHarbor functionality implemented in code. The local MCP interface is only an optional integration surface that exposes those existing operations to agents; it does not change how BaseHarbor detects, plans or verifies infrastructure. It does not expose a generic shell, Docker socket or unrestricted runtime execution.

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

Docker and Podman are the implemented Runtime Providers today. Docker realizes Compose workload input through Docker Compose; Podman realizes it through native Quadlet + rootless `systemd --user`. Kubernetes and OpenShift remain future runtime tracks and must preserve the same portable application contract when implemented.

Normal feature, fix, chore and dependency pull requests target `develop`. The `main` branch represents released source.

## License

Apache License 2.0. See [LICENSE](LICENSE).

---

<sub>Human-specified, machine-verified. AI can integrate through the bounded MCP surface, but BaseHarbor's core inspection, planning and verification are deterministic.</sub>
