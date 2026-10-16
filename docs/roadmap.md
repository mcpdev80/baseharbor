# Roadmap

Detailed planning lives in [baseharbor-plan/ROADMAP.md](https://github.com/mcpdev80/baseharbor-plan/blob/main/ROADMAP.md). This page shows product direction; historical issue links retain discussion and context.


## Planning transition

Planning lives in [baseharbor-plan](https://github.com/mcpdev80/baseharbor-plan), activated at `47474109ea69475e60be4f065ad17476c71665bb`. Read its `AGENTS.md` and the assigned task note for scope, dependencies, ownership and handoffs. Product engineering rules, PR reviews and authenticated release evidence stay in their existing repositories. Migration-closed issues are historical references, not evidence of implementation.


## Current — v0.4.26 development

[v0.4.25 is published](releases/v0.4.25.md). Development now focuses on the needs-first app wizard, capability editing and bounded development/CI improvements (#873/#888). Development progress does not imply pre-release readiness.

## Before the v0.5 freeze

The binding split is recorded in [#427](https://github.com/mcpdev80/baseharbor/issues/427):

- v0.4.26: TUI and developer journeys (#873).
- v0.4.27: onboarding, profiles and Node authority (#874).
- v0.4.28: Console API and UX (#875).
- v0.4.29: host platforms including WSL2/macOS (#876).
- v0.4.30: native TLS, justified proxies and attack-protection boundaries (#904).
- v0.4.31: final semantic acceptance (#174).

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

