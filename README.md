<h1 align="center">BaseHarbor</h1>

<h2 align="center">One application contract. Replaceable infrastructure.</h2>

<p align="center">
  <a href="https://github.com/mcpdev80/baseharbor/releases">
    <img src="https://img.shields.io/github/v/release/mcpdev80/baseharbor?display_name=tag&sort=date" alt="GitHub Release">
  </a>
  <a href="https://mcpdev80.github.io/baseharbor/">
    <img src="https://img.shields.io/badge/docs-GitHub%20Pages-0068E9" alt="GitHub Pages">
  </a>
  <a href="https://github.com/mcpdev80/baseharbor-demo">
    <img src="https://img.shields.io/badge/demo-baseharbor--demo-6f42c1" alt="BaseHarbor Demo App">
  </a>
  <a href="https://github.com/mcpdev80/baseharbor-console">
    <img src="https://img.shields.io/badge/console-Web%20UI-00897b" alt="BaseHarbor Console">
  </a>
  <a href="https://github.com/mcpdev80/baseharbor-node-connector">
    <img src="https://img.shields.io/badge/node--connector-Remote%20hosts-455a64" alt="BaseHarbor Node Connector">
  </a>
  <a href="CONTRIBUTING.md">
    <img src="https://img.shields.io/badge/contributing-welcome-2ea44f" alt="Contributing">
  </a>
  <a href="LICENSE">
    <img src="https://img.shields.io/github/license/mcpdev80/baseharbor" alt="License">
  </a>
</p>

## The problem

You want to build an app. First, you have to configure databases, networking,
secrets, TLS, login, backups and monitoring.

BaseHarbor is a CLI that provisions, connects and verifies the infrastructure
your app needs — on your own machines, using standard protocols and libraries.

## If you know Terraform, you already understand BaseHarbor

Terraform describes infrastructure resources. BaseHarbor describes
**application needs**: SQL, cache, secrets, storage, login or HTTPS.

Declare the needs, review the plan, apply it and verify the result.
Provider choices live in Target bindings, keeping application intent portable.

```yaml
version: 1
app:
  name: my-app
  environment: dev
services:
  sql: true
```

## What you get

- **Start or adopt an app** — use existing repositories and normal ecosystem tooling.
- **Only the services you need** — selected capabilities plus genuine dependencies.
- **Connected services** — databases, cache, messaging, storage, secrets, identity and observability.
- **Protected access** — TLS, scoped credentials and explicit trust boundaries.
- **Verified lifecycle** — plan, apply, inspect, stop, recover and remove with ownership checks and concrete consequences.
- **Your infrastructure** — Docker and rootless Podman (Quadlet); optional Console and Node Connector.
- **Standard application interfaces** — database drivers, S3, OIDC and OTLP; no BaseHarbor SDK.

## A CLI first — agent-ready if you want it

BaseHarbor works deterministically without AI. Use it directly, or let an agent
use the same structured operations through JSON and MCP.

```bash
baha init
baha up
baha status
```

The optional [Console](https://github.com/mcpdev80/baseharbor-console) adds a web
interface. The [Node Connector](https://github.com/mcpdev80/baseharbor-node-connector)
provides protected access to remote hosts.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/mcpdev80/baseharbor/main/scripts/install.sh | bash
baha version
```

## Learn more

[Getting started](https://mcpdev80.github.io/baseharbor/tutorials/getting-started/) ·
[Documentation](https://mcpdev80.github.io/baseharbor/) ·
[Releases](https://github.com/mcpdev80/baseharbor/releases)

## Project status

BaseHarbor is pre-v1. This branch prepares v0.4.25; it is not yet release-ready.
Docker and Podman runtimes are implemented; Kubernetes and OpenShift runtimes
are planned. See the documentation for supported paths and current limits.

## License

Apache License 2.0. See [LICENSE](LICENSE).
