# Service/runtime neutrality audit

Issue: #618  
Parent release: #516 (v0.4.19)  
Kubernetes architecture proof: #616

## Purpose

This document tracks the pre-freeze audit of shipped BaseHarbor service/provider boundaries.

The goal is not to implement Kubernetes in v0.4.19. The goal is to ensure that the contracts frozen before v0.5 can later be realized by Kubernetes/OpenShift without redesigning portable application, capability, provider, binding, trust or lifecycle semantics.

## Audit matrix

| Capability / service | Reference provider | Current runtime coupling to review | Required freeze-ready boundary |
| --- | --- | --- | --- |
| database.sql | PostgreSQL | RuntimeProvider, ExecProject, Compose runtime generation, networks, host paths, volumes | provider lifecycle owns DB/user/credential/isolation semantics; execution and realization stay behind a narrow runtime/provider seam |
| cache.key-value | Valkey | Compose topology, ExecProject, TLS gateway/network assumptions, volume/runtime state | cache semantics, credentials, isolation and verification independent of runtime topology |
| secrets | OpenBao | Executor is already narrow; remaining bhruntime.Files, project/service identity, host-local state/materialization | bootstrap/KV/AppRole/PKI/application-scope semantics reusable through non-Compose executor/API boundary |
| object-storage.s3 | SeaweedFS | Compose provider lifecycle, endpoint/network addressing, local state paths | bucket/credential/binding/verification semantics independent of runtime |
| exposure.http | Caddy | host ports, Docker networks/aliases, Compose service lifecycle | portable route/exposure contract consumable later by Gateway API/platform ingress |
| telemetry.otlp | OTel Collector / external OTLP | container/network endpoints and managed provider lifecycle | transport endpoint/trust/binding semantics independent of runtime |
| metrics | Prometheus | scrape/discovery identity and Compose provider lifecycle | logical source identity and provider placement independent of container/network identity |
| logs | Loki | Docker/container log collection assumptions | runtime-neutral log source identity and collection contract; see #480 |
| traces | Tempo | network endpoints and managed provider lifecycle | OTLP transport and trace storage remain distinct and runtime-neutral |
| identity.oidc | Keycloak / external OIDC | KeycloakRuntime exposes ConfigProject/UpProject/DestroyProject; endpoint/TLS realization | OIDC realm/client/callback/MFA semantics independent of Compose lifecycle |
| BaseHarbor runtime service | runtime-broker | Compose deployment, network/service discovery, host-projected identity material | application-scoped broker semantics realizable inside a target/platform boundary |
| BaseHarbor runtime service | runtime-executor | Compose deployment, target-local files/networks | shared execution/control semantics realizable in-cluster without portable host-local assumptions |

Management UI surfaces are audited separately as provider/operator surfaces. They must not become portable application capabilities or force runtime-specific application intent.

## Cross-cutting checks

For every shipped provider/service:

- portable semantic remains product/runtime-neutral;
- placement is application/shared/external where applicable;
- lifecycle exposes preflight, provision/reference, bind, observe, verify, reconcile and destroy/release without requiring Compose vocabulary at the frozen boundary;
- binding identity does not depend on container names, Docker networks, host paths or future Kubernetes resource names;
- TLS/PKI/trust does not assume certificate material exists on the machine running baha;
- persistent data ownership is provider/capability semantic rather than Docker-volume semantic;
- status/doctor/evidence report semantic readiness instead of container liveness;
- destroy preserves shared/external/foreign ownership;
- CLI, JSON and MCP use the same domain lifecycle.

## Validation rule

Boundary corrections must retain Docker and Podman behavior.

Use focused tests first, then targeted Docker/Podman validation. The Kubernetes proof branch may implement temporary/adaptor realizations only far enough to prove or falsify these contracts.

Production Kubernetes support remains v0.7/v0.8 scope.
