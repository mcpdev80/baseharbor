# Runtime Standards Audit

Status: v0.4.20 pre-freeze review  
Verified against upstream specifications: 2026-10-02

This audit records which external standards define BaseHarbor runtime behavior, which are delegated to an underlying runtime or cluster, and which BaseHarbor extensions remain necessary.

The central rule is:

> Adopt an existing interoperable standard where it defines the boundary. Add BaseHarbor semantics only where no suitable standard describes portable application intent, provider selection, ownership or verification.

## OCI Image Specification

Upstream: https://specs.opencontainers.org/image-spec/

### Existing Standards

The OCI Image Specification defines interoperable image manifests, indexes, configuration and filesystem layers.

### Adopted Standards

BaseHarbor uses OCI-compatible container images as runtime artifacts. Docker and Podman consume those images through their normal OCI-compatible image stacks.

### Deviations

BaseHarbor does not define a competing image format and does not directly implement the OCI image loader.

### BaseHarbor Extensions

BaseHarbor records desired image references and verifies runtime image identity where lifecycle/evidence requires it.

### Compatibility Impact

A future runtime provider must preserve portable image identity or explicitly declare a different supported workload source. Portable application intent must not depend on Docker- or Podman-specific image metadata.

## OCI Distribution Specification

Upstream: https://specs.opencontainers.org/distribution-spec/

### Existing Standards

The OCI Distribution Specification defines the registry protocol for distributing content such as OCI images.

### Adopted Standards

BaseHarbor delegates image pull/distribution behavior to the selected runtime and compatible registries instead of inventing a BaseHarbor registry protocol.

### Deviations

BaseHarbor does not currently expose its own OCI Distribution client or registry implementation.

### BaseHarbor Extensions

Runtime evidence may record the resolved image identity/digest, but registry transport remains provider/runtime owned.

### Compatibility Impact

Runtime providers may use different registry clients while preserving the same portable image reference semantics.

## OCI Runtime Specification

Upstream: https://github.com/opencontainers/runtime-spec/blob/main/spec.md

### Existing Standards

The OCI Runtime Specification defines container configuration, execution environment and lifecycle at the low-level runtime boundary.

### Adopted Standards

Docker and Podman ultimately execute OCI-compatible containers. BaseHarbor relies on their conforming runtime stacks.

### Deviations

BaseHarbor intentionally does not emit or manage OCI `config.json` directly.

### BaseHarbor Extensions

The BaseHarbor `RuntimeProvider` contract operates at a higher orchestration level: converge, observe, exec, logs, ownership, backup/restore primitives and semantic verification.

### Compatibility Impact

The provider contract must not assume a specific low-level OCI runtime implementation such as runc or crun.

## Compose Specification

Upstream: https://github.com/compose-spec/compose-spec/blob/main/spec.md  
Schema: https://github.com/compose-spec/compose-spec/blob/main/schema/compose-spec.json

### Existing Standards

The Compose Specification defines a platform-agnostic multi-container application model with services, networks, volumes, configs and secrets.

### Adopted Standards

Compose is the current repository workload-source standard.

DockerProvider realizes that source through Docker Compose.

PodmanProvider consumes the same source semantics and realizes them as native Quadlet units managed by `systemd --user`.

### Deviations

BaseHarbor is not itself a complete Compose implementation. Docker delegates realization to Docker Compose. The Podman renderer supports the subset required by the portable workload contract and must fail closed for unsupported semantics.

### BaseHarbor Extensions

BaseHarbor adds deployment-owned provider selection, ownership labels/state, security policy, Service Binding projection, managed capability replacement, workload routing and semantic verification.

### Compatibility Impact

`compose` is a repository workload-source identity, not a Runtime Provider identity. In v0.4.20 Compose is one built-in Workload Source Adapter beside repository-authored Podman Quadlet and raw Kubernetes YAML.

All three built-ins normalize into the same source-neutral Workload Evidence model and logical workload-component identity. Source kind/path remain repository provenance rather than portable Application Intent.

Helm and Kustomize are intentionally deferred to later Workload Source Adapters. Their later addition must not change portable Runtime Provider identity or Application Intent.

## Kubernetes API

Upstream: https://kubernetes.io/docs/reference/using-api/api-concepts/

### Existing Standards

The Kubernetes API is a resource-oriented HTTP API for declarative cluster state, including watch and status semantics.

### Adopted Standards

Raw Kubernetes YAML is statically inspectable as a repository workload source in v0.4.20. Inspection recognizes standard workload/supporting resources without requiring a live cluster and keeps unknown CRDs as opaque evidence where relevant.

Kubernetes runtime execution is still not implemented in v0.4.20. A future Kubernetes Runtime Provider should use the Kubernetes API as its primary lifecycle boundary rather than shelling out to container runtimes on cluster nodes.

### Deviations

BaseHarbor does not currently require CRDs or an operator for runtime operation.

### BaseHarbor Extensions

BaseHarbor keeps portable application intent, provider capability negotiation, ownership and semantic verification above Kubernetes object realization.

### Compatibility Impact

Kubernetes resource kinds remain provider implementation details. Portable `baseharbor.yaml` must not become a Kubernetes manifest.

## Container Runtime Interface (CRI)

Upstream: https://kubernetes.io/docs/concepts/containers/cri/

### Existing Standards

CRI defines the gRPC boundary between kubelet and container runtimes. Kubernetes documents CRI v1 as the supported stable interface.

### Adopted Standards

Delegated, not directly consumed.

A Kubernetes provider talks to the Kubernetes API; kubelet/runtime communication remains the cluster's CRI responsibility.

### Deviations

BaseHarbor does not implement a CRI client and does not bypass kubelet to manage node containers.

### BaseHarbor Extensions

None at the CRI layer.

### Compatibility Impact

The Kubernetes provider remains independent of whether cluster nodes use containerd, CRI-O or another conforming runtime.

## Container Network Interface (CNI)

Upstream: https://github.com/containernetworking/cni/blob/main/SPEC.md

### Existing Standards

CNI defines network configuration and the protocol between runtimes and networking plugins.

### Adopted Standards

Delegated.

Docker/Podman local networking is handled by their runtime stacks. Future Kubernetes networking is expressed through Kubernetes resources and delegated to the cluster networking implementation.

### Deviations

BaseHarbor does not execute CNI plugins directly.

### BaseHarbor Extensions

BaseHarbor defines portable connectivity intent, ownership and semantic reachability verification above the network implementation.

### Compatibility Impact

Portable application connectivity must not depend on a particular CNI implementation.

## Container Storage Interface (CSI)

Upstream: https://github.com/container-storage-interface/spec/blob/master/README.md

### Existing Standards

CSI defines a vendor-neutral storage plugin interface for container orchestrators.

### Adopted Standards

Delegated for future Kubernetes/OpenShift providers.

BaseHarbor does not talk directly to CSI drivers.

### Deviations

Local Docker/Podman volume lifecycle uses their native volume mechanisms rather than CSI.

### BaseHarbor Extensions

BaseHarbor defines portable ownership, backup/restore contributors and verification for BaseHarbor-owned application state.

### Compatibility Impact

A future Kubernetes provider should realize persistent storage through Kubernetes storage APIs/PVCs and allow the cluster to select CSI implementation details.

## Kubernetes Gateway API

Upstream: https://gateway-api.sigs.k8s.io/reference/api-spec/main/spec/

### Existing Standards

Gateway API defines role-oriented, extensible Kubernetes APIs for traffic exposure and routing.

### Adopted Standards

Not executable in v0.4.17.

Gateway API is the preferred standards-first candidate for future Kubernetes HTTP/TLS exposure when the target cluster supports the required resources and capabilities.

### Deviations

Current local development access uses BaseHarbor's local gateway realization because Docker/Podman are not Kubernetes clusters.

### BaseHarbor Extensions

Portable exposure intent, canonical developer URLs, ownership and TLS policy remain BaseHarbor semantics. Provider implementations translate those semantics to Gateway API or another explicitly supported target mechanism.

### Compatibility Impact

Portable exposure intent must remain stable whether realized by the local gateway, Kubernetes Gateway API or an OpenShift-specific realization.

## Service Binding Specification

Upstream: https://github.com/servicebinding/spec  
Project: https://servicebinding.io/

### Existing Standards

Service Binding defines a consistent filesystem-oriented contract for exposing service connection material to workloads. The project publishes Core 1.1.0.

### Adopted Standards

BaseHarbor uses Service Binding 1.1-compatible workload projections for managed service connection data and trust material.

### Deviations

BaseHarbor also maintains protected operator/runtime metadata that is intentionally not projected into workload bindings.

### BaseHarbor Extensions

BaseHarbor determines provider placement, ownership, generated credentials, TLS material and portable capability mapping before producing the consumer-facing binding.

### Compatibility Impact

Applications consume stable binding semantics independent of whether the backing provider is application-scoped, shared, external, Docker-backed, Podman-backed or later Kubernetes-backed.

## Crossplane Composition Patterns

Upstream: https://docs.crossplane.io/latest/composition/

### Existing Standards

Crossplane Composition provides a Kubernetes-native pattern for building custom APIs that compose multiple resources.

### Adopted Standards

Reviewed as an architectural pattern, not adopted as a mandatory Runtime Provider dependency.

### Deviations

BaseHarbor does not require Crossplane, XRDs, Compositions or a controller to run Docker/Podman workloads, and the planned Kubernetes provider is not operator-first.

### BaseHarbor Extensions

The BaseHarbor provider registry and descriptor contract keep portable intent separate from realization in a similar architectural spirit, but remain runtime-neutral and usable outside Kubernetes.

### Compatibility Impact

Crossplane may later be an optional integration or realization technique. Portable BaseHarbor applications must not require Crossplane-specific resources.

## Result

For v0.4.20:

- OCI image/distribution/runtime standards are delegated to conforming runtime stacks.
- Compose, repository-authored Podman Quadlet and raw Kubernetes YAML are built-in repository Workload Source Adapters behind one versioned source/evidence boundary.
- Helm and Kustomize are deferred source adapters, not v0.4.20 support.
- Service Binding 1.1 is the application-facing managed-service binding boundary.
- Docker and Podman are explicit Runtime Providers behind `baseharbor.runtime/v1`.
- Podman runtime realization remains generated Quadlet + `systemd --user`, separate from repository-authored Quadlet as an input source.
- Kubernetes API and Gateway API remain future provider realization standards even though raw Kubernetes YAML can already be inspected/adopted statically.
- CRI, CNI and CSI remain cluster/runtime implementation boundaries and are not reimplemented by BaseHarbor.
- Crossplane is an optional pattern/integration, not a runtime prerequisite.
- BaseHarbor-specific extensions are limited to portable intent, provider negotiation, ownership, policy, lifecycle semantics and verification where the reviewed standards do not define those concerns.
