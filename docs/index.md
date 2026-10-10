---
hide:
  - toc
---

<img class="bh-home-mark" src="brand/baseharbor-master-lockup.png" alt="BaseHarbor — Build · Run · Control · Everywhere">

# Your applications. Your infrastructure.

BaseHarbor brings applications and their backends together on verified Linux Targets. Use one CLI to run native workloads, connect secure provider bindings and inspect the result.

[Get started](tutorials/getting-started.md){ .md-button .md-button--primary }
[GitHub](https://github.com/mcpdev80/baseharbor){ .md-button }

## From repository to running application

Install the [CLI and a rootless Docker or Podman runtime](tutorials/getting-started.md), then open your application's repository:

```bash
baha init
baha up
baha status
```

`baha init` adopts the repository and reuses an existing Core, or guides you through the required setup. Core means **SQL + Secrets + Identity**; the Console is optional. `baha up` converges the application. Status distinguishes verified readiness from a workload that is merely running.

## Build with your normal tools

Keep your application code and libraries. BaseHarbor detects supported Compose, Quadlet and Kubernetes workload sources, resolves provider-neutral requirements and supplies protected bindings. Ambiguous or [unsupported sources](explanation/workload-sources.md) require an explicit next action before deployment.

## Run with a shared or isolated Core

Choose supported provider placement and a managed local or enrolled remote Target. Ownership, TLS, permissions and retained data remain explicit throughout the lifecycle. [Core resource observations](explanation/core-resources.md) distinguish measured reference topologies from planning estimates and the cost of additional isolation.

## Control and verify

Inspect a plan before mutation, diagnose failures with Doctor, and review backups before recovery. CLI, JSON and MCP use the same domain operations. [EN and DE documentation](https://mcpdev80.github.io/baseharbor/de/) have equivalent page and command coverage; shipped technical identifiers and verified CLI output stay identical.

## Choose your next task

| Task | Start here |
| --- | --- |
| Create, adopt or operate an application | [Applications](cli/applications.md) |
| Select a local or remote deployment destination | [Targets](cli/targets.md) |
| Connect managed or external backends | [Providers](explanation/providers.md) |
| Manage identity, TLS and trust | [Security](explanation/security.md) |
| Automate through JSON and MCP | [Automation and agents](cli/automation-agents.md) |
| Find exact shipped commands and contracts | [Command index](cli/command-index.md) · [Reference](reference/cli.md) |

Delivered changes and support boundaries are recorded in the [release notes](releases/index.md).
