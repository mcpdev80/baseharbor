# Platform support

Support applies to a specific runtime, operating system and architecture.
A published binary, container image or cross-build is not a runtime acceptance
result. This table records the released v0.4.22 baseline; v0.4.23 qualification
is pending its exact-candidate evidence.

| Host / execution context | Runtime | amd64 | arm64 | Evidence / limitation |
| --- | --- | --- | --- | --- |
| Native Linux | Docker / Compose | Release-qualified | Build/published artifact; runtime unproven | [v0.4.22 pre-release](https://github.com/mcpdev80/baseharbor/actions/runs/37385682137): atomic + HA + full Journey |
| Native Linux, user systemd | Rootless Podman / Quadlet | Release-qualified | Build/published artifact; runtime unproven | Same exact-input pre-release, separate Podman proofs; user systemd is required |
| WSL2 Linux guest | Docker / Podman | Unproven host combination | Unproven | Linux binary does not prove WSL service/network/terminal behavior |
| Native Windows | Any | Unsupported release binary/runtime | Unsupported | No native Windows Core artifact or live lifecycle qualification |
| Native macOS | Any | Unsupported release binary/runtime | Unsupported | Linux VM/remote target is a different host context |
| Linux | Kubernetes / K3s | Architecture/adoption proof only | Unproven | Kubernetes source/adoption tests do not deliver a supported Runtime Provider |
| Linux | OpenShift / OKD | Planned | Unproven | Later runtime implementation; no support from namespace design alone |
| Linux remote node | [Node Connector](https://github.com/mcpdev80/baseharbor-node-connector) -> Docker / Podman | Integration pending | Unproven | Public package tests/CI prove Connector-local boundaries; full pinned Core -> Connector -> remote runtime lifecycle evidence is still required |
| Browser | [Console](https://github.com/mcpdev80/baseharbor-console) -> HTTPS Core | Partial live qualification | Host-independent UI; architecture-specific browser evidence | Native evidence proves authenticated admission, runtime list/inspect/metrics, terminal and log streaming; complete application mutation/rotation/final release pinning remains pending |

Linux amd64/arm64 release artifacts are configured by `.goreleaser.yaml`;
[v0.4.22 publication](https://github.com/mcpdev80/baseharbor/actions/runs/37394723884)
proves publication. Runtime acceptance must retain its original runner/host
identity and input references. New architectures and hosts become supported
only after their own lifecycle, security, ownership, recovery and cleanup
evidence passes. Browser versions require separate UI/authentication coverage.


Companion repositories are public, but they remain optional implementations behind Core-owned contracts. The Console never talks directly to a runtime or Node Connector. The Node Connector transports bounded operations selected and authorized by Core; it does not own desired state, authorization, placement or reconciliation. Their own CI is necessary but not sufficient for BaseHarbor release support: v0.4.23 requires exact-ref cross-repository evidence through the release requirements.
