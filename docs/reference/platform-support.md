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
| Linux remote node | Connector -> Docker / Podman | Integration pending | Unproven | Descriptor and private connector unit tests do not prove end-to-end Core lifecycle |
| Browser | Console -> HTTPS Core | Live integration pending | Host-independent UI; live integration pending | Preview fixtures are not a live browser workflow |

Linux amd64/arm64 release artifacts are configured by `.goreleaser.yaml`;
[v0.4.22 publication](https://github.com/mcpdev80/baseharbor/actions/runs/37394723884)
proves publication. Runtime acceptance must retain its original runner/host
identity and input references. New architectures and hosts become supported
only after their own lifecycle, security, ownership, recovery and cleanup
evidence passes. Browser versions require separate UI/authentication coverage.
