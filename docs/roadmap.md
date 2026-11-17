# Roadmap

Detailed planning lives in [baseharbor-plan/ROADMAP.md](https://github.com/mcpdev80/baseharbor-plan/blob/main/ROADMAP.md). This page shows product direction; historical issue links retain discussion and context.


## Planning transition

Planning lives in [baseharbor-plan](https://github.com/mcpdev80/baseharbor-plan), activated at `47474109ea69475e60be4f065ad17476c71665bb`. Read its `AGENTS.md` and the assigned task note for scope, dependencies, ownership and handoffs. Product engineering rules, PR reviews and authenticated release evidence stay in their existing repositories. Migration-closed issues are historical references, not evidence of implementation.


## Release scope — v0.4.27

[v0.4.27](releases/v0.4.27.md) focuses on connecting a developer workstation to a Core, user and workstation lifecycle, application delivery, per-stage versions, Target deployment policies and the reference Demo. See [GitHub releases](https://github.com/mcpdev80/baseharbor/releases) for published versions.

## Before the v0.5 freeze

The current sequence is:

- v0.4.28: consistent CLI command names, options and interaction.
- v0.4.29: WSL2 and macOS support, after the CLI work.
- v0.4.30: native TLS, justified proxies and attack-protection boundaries.
- v0.4.31: final semantic acceptance.

Console and the standalone Node Connector are paused and outside the public release scope.

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

