# Roadmap

Detailed planning lives in GitHub Issues. This page shows product direction; [issue #155](https://github.com/mcpdev80/baseharbor/issues/155) is the overarching roadmap.

## Current — v0.4.24

Implementation and pre-release validation are complete.

- Task-oriented CLI and repository-aware application lifecycle.
- Explicit Core/Target selection and rootless Docker by default.
- Shared providers without duplicate servers, with opt-in HA.
- Guarded provider updates, PostgreSQL/etcd recovery and bounded upgrade paths.
- Joint Console/Node Connector workflows and complete EN/DE documentation.

The [release notes](releases/v0.4.24.md) describe the highlights and compatibility notes. Detailed scope and evidence are recorded in [PR #837](https://github.com/mcpdev80/baseharbor/pull/837).

## Before the v0.5 freeze

### v0.4.25

Semantic Acceptance.

- prove CLI/JSON/MCP semantic parity;
- prove runtime/provider boundaries and ownership;
- prove provider compatibility is richer than capability-name matching;
- perform the final architecture and lifecycle acceptance before freeze.

## v0.5

Freeze, version and harden the public contracts.

No new major platform primitive belongs in the v0.5 line.

## Later

### v0.6

Post-freeze portability/guarantee hardening and preparatory work that does not
require changing frozen portable Application semantics.

The original portable-availability contract work was pulled forward into v0.4.21
so v0.6 must not introduce a second availability model.

### v0.7

Kubernetes Runtime implementation behind the frozen Runtime Provider boundary.

Namespace-only operation remains the primary authorization profile.

### v0.8

Kubernetes Complete: full lifecycle, capability/provider, recovery,
observability, update/migration, agent/DX and production-hardening parity.

OpenShift, enterprise and cloud/runtime expansion build on the frozen portable
contracts rather than changing them.
