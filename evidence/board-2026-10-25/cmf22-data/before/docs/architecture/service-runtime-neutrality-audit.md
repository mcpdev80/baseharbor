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


## v0.4.19 audit conclusions

The freeze-relevant review now distinguishes portable semantics from runtime/delivery realization explicitly.

| Surface | Freeze-ready conclusion | Evidence |
| --- | --- | --- |
| PostgreSQL / `database.sql` | SQL resource identity, placement and authenticated `SELECT 1` verification are runtime-neutral; Docker/Podman project execution is an adapter implementation. | `BackendProbeExecutor` plus real k3s semantic probe |
| Valkey / `cache.key-value` | Cache identity, Service Binding contract and authenticated PING/PONG semantics are runtime-neutral; RESP transport realization and local TLS gateway remain adapter state. | `BackendProbeExecutor` plus real k3s PING/PONG proof |
| OpenBao / `secrets` | bootstrap, AppRole, KV, application scopes and PKI use the focused executor/provider boundary. | real k3s PostgreSQL-backed OpenBao + PKI proof |
| SeaweedFS / `object-storage.s3` | bucket/credential/binding lifecycle is behind the object-storage realization; runtime addressing is not portable intent. | real k3s S3 lifecycle proof |
| Caddy / `exposure.http` | route intent is independent of Caddy, host ports and runtime network aliases. | real k3s HTTP exposure proof |
| OpenTelemetry / `telemetry.otlp` | endpoint, trust material, client identity and HTTP client form the realization boundary. | real mTLS k3s OTLP proof |
| Prometheus / `metrics` | logical metrics source identity is independent of Docker aliases; target discovery is realization-owned. | real k3s Prometheus scrape proof |
| Loki / `logs` | portable format is `runtime-stream`; syslog/journald/CRI collection is runtime realization detail. | real k3s Loki source/query proof |
| Tempo / `traces` | trace storage lifecycle and trace verification are behind `TempoRealization`; OTLP transport remains a separate capability. | real k3s OTLP protobuf ingest + Tempo query proof |
| Keycloak / `identity.oidc` | OIDC realm/client/callback/policy semantics are separated from provider deployment. | real k3s Keycloak identity lifecycle proof |
| runtime-broker | frozen HTTP/mTLS/workload-identity behavior is runtime-neutral. Compose materialization, networks, mounts and healthcheck deployment remain delivery-adapter state. | runtime-control contract architecture gate |
| runtime-executor | request/resource semantics and SPIFFE workload identity are runtime-neutral; current Compose deployment is implementation-private. | runtime-control contract architecture gate |
| Management UIs | administration endpoints are operator surfaces and consume canonical workload Service Bindings. Compose/Gateway delivery is not application intent. | provider-UI conformance gate |

### Important ownership finding

The Prometheus Kubernetes pressure test exposed an ownership bug in the first proof realization: provider reconciliation used an application-wide destroy path and therefore removed the application metrics source together with provider resources.

The corrected invariant is now explicit:

```text
provider reconcile/destroy
  -> may remove only resources owned by that provider realization
  -> must never remove application workload resources
  -> shared/external resources follow their declared provider ownership
```

This invariant applies to every runtime, not only Kubernetes.

### Runtime-control classification

The runtime broker and runtime executor deliberately keep two layers:

```text
frozen semantic/control contract
  HTTP + mTLS + SPIFFE/workload identity + capability operations
                    |
                    v
deployment realization
  Compose/Quadlet today
  Kubernetes/OpenShift later
```

The architecture conformance suite scans the frozen runtime API and executor client/handler and fails if Docker, Podman, Compose or Kubernetes topology leaks into that contract.

### Management-UI classification

Management UIs remain provider/operator delivery surfaces. They consume canonical Service Binding fields such as host, port, credentials and trust material. They are not capabilities requested by application code, and no Kubernetes/OpenShift-specific UI field is introduced into portable application intent.

### Validation evidence

Focused validation completed before broad runtime regression:

```text
Tempo Hosted neutrality R2                 36762950707  PASS
Tempo k3s proof                            36764079128  PASS
Valkey k3s semantic proof                  36764508145  PASS
Runtime-neutrality architecture/conformance 36775413937  PASS
Core identity/runtime-neutrality after rebase 36775419257 PASS
OTLP Hosted neutrality R2                  36752933918  PASS
OTLP k3s proof                             36757633570  PASS
Prometheus Hosted neutrality R2            36759083558  PASS
Prometheus k3s proof R4                    36760436728  PASS
Loki Hosted neutrality R2                  36761005744  PASS
Loki k3s proof                             36761362966  PASS
Docker Core regression (pre-rebase)        36767491690  PASS
Podman destroy adapter focused gate        36777110907  PASS
```

The Kubernetes runs are architecture pressure tests only. They do not constitute the production Kubernetes provider planned for v0.7/v0.8.

### Baseline synchronization

The v0.4.19 implementation branch was synchronized onto the current `develop` baseline after v0.4.18 stabilization continued in parallel.

At synchronization time:

```text
develop  75d1a14c256ddaa06b7317536060489d8afba4f9
#619     393d9fd929afafd70dbd5ed97312993b0bbe8378
behind   0
```

The Keycloak conflict was resolved semantically:

- runtime-neutral `KeycloakRealization` remains the Core boundary;
- the newer PostgreSQL readiness dependency from `develop` is retained;
- the Docker/Quadlet-safe Keycloak HTTPS healthcheck is retained;
- readiness diagnostics are attached through the realization boundary rather than reintroducing Compose lifecycle into the identity Core;
- current targeted-gate routing and the current v0.4.18 immutable demo revision are retained.

The post-rebase Core identity/runtime-neutrality and architecture/conformance gates are green. Docker/Podman exact-head runtime regression evidence is recorded separately when those runner-bound gates complete.


### Final reference-runtime regression

```text
Docker Core regression (post-rebase)        36777265097  PASS
Podman Core regression (post-rebase)        36777185021  PASS
```

Both reference runtimes passed the same post-boundary-correction semantic lifecycle, including recovery, real log ingestion, real trace ingestion, cleanup and evidence generation.
