# Changelog

All notable changes to BaseHarbor are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Continuous repository-to-contract reconciliation for evolving applications, including typed capability direction and runtime-operation evidence.
- Repository inspection detection for S3-compatible usage/runtime bucket creation, OpenMetrics `/metrics` endpoints and OTLP export.
- Real `object-storage.s3/v1` application-time create/get/delete execution through a shared mTLS Runtime Provider Executor.
- Persistent asynchronous runtime-resource operations with idempotent mutation keys and restart/resume reconciliation.
- Real runtime S3 bindings with bucket-scoped IAM credentials and authenticated native S3 Put/Get consumption.
- Versioned `metrics/v1` capability specification for application-provided OpenMetrics-compatible HTTP sources.
- Provider-neutral Manifest v1 metrics source declarations with logical source name, workload service, target port and path.
- Prometheus 3.14.0 as the first lazy shared Compose metrics provider with automatic file-based target discovery.
- Real scrape/ingestion verification and manual-only two-application shared-provider acceptance coverage.

### Changed

- Canonical Manifest v1 output is sparse and omits disabled optional capabilities while remaining backward compatible with existing explicit `enabled: false` input.
- Repeated repository inspection reports satisfied/new/ambiguous/stale state without destructively rewriting explicit application intent.
- Repository-first `baha up` now reports new/ambiguous capability drift and runtime-operation hints before convergence while leaving the contract unchanged.
- Added the versioned OpenAPI 3.1 Runtime Resource API v1 contract for provider-neutral application-time resources, including idempotency, asynchronous operation state and secure binding boundaries.
- Interactive API documentation policy is now explicit: development on by default; test/staging and production off by default unless platform/operator policy enables it.
- The per-application runtime component is generalized as the **Application Runtime Broker**: managed OpenBao secrets remain a runtime module, canonical application-bound secret routes move under `/runtime/v1/secrets`, and existing app-qualified routes remain compatibility aliases.
- Development brokers now serve embedded Swagger/OpenAPI documentation on a stable automatically allocated host-loopback port; no public CDN or public bind is required.
- The canonical broker DNS endpoint is `baseharbor-runtime`; the legacy `baseharbor-secrets` alias remains available for compatibility.
- `baha app apply` and `baha app up` now lazily start/reuse the shared Runtime Provider Executor whenever an explicit runtime resource permission requires it.
- Runtime-only applications receive a deterministic broker backend network without requiring artificial PostgreSQL/Valkey services; S3 provider-network access is attached only to workload services explicitly authorized for `object-storage.s3/v1`.
- Global `baha destroy --yes` removes the BaseHarbor-owned shared Runtime Provider Executor before the shared object-storage provider.
- Metrics collection is deployment policy rather than application product intent: development defaults on; test/staging/production require explicit opt-in unless overridden with `BASEHARBOR_METRICS_ENABLED`.
- Each participating application gets an isolated metrics network; only declared/authorized metrics-source services join it, while the selected Prometheus instance is attached only to explicitly registered application networks.
- Prometheus placement now uses the generic provider-placement model: safe shared default, optional named sharing boundaries and application-scoped placement, with unsupported placement failing before mutation.
- Application destroy removes only its metrics target/trust-edge state or dedicated provider according to placement; global destroy removes every BaseHarbor-owned shared Prometheus default/sharing-boundary instance and data volume.

### Security

- Missing repository evidence never authorizes capability removal.
- Detected runtime operations such as S3 bucket creation are evidence only and never grant runtime authorization or provision infrastructure.
- Applications and per-app brokers never receive provider-global S3 administrator credentials; those remain at the Runtime Provider Executor boundary.
- Broker and executor run without Docker/Podman sockets; the executor has no host-published port and accepts only BaseHarbor SPIFFE/mTLS workload identities.
- Runtime resource ownership is application/environment scoped; resource IDs cannot be used to read, bind or delete another application's resource.
- Runtime S3 credentials are excluded from asynchronous operation state, normal resource metadata, logs and manifests and are returned only by the authenticated binding endpoint.
- Prometheus target state contains endpoint identity and attribution labels only; it does not contain application credentials, provider-global credentials or portable product configuration.
- Metrics collection does not implicitly provision Grafana, Loki or Tempo.

## [0.4.7] - 2026-09-19

### Added

- Versioned `telemetry.otlp/v1` capability specification for explicit OTLP export semantics independent of any observability backend product.
- Typed provider-neutral OTLP workload binding in the shared capability lifecycle and Provider Protocol v1.
- OpenTelemetry Collector 0.161.0 as the first lazy shared Compose reference provider.
- External OTLP endpoint binding through deployment-owned state without BaseHarbor taking provider lifecycle ownership.
- Standard OpenTelemetry workload configuration through `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_SERVICE_NAME` and `OTEL_RESOURCE_ATTRIBUTES`.
- Real OTLP HTTP/protobuf verification using an exported trace accepted by the selected OTLP endpoint.
- Manual-only Docker/Compose acceptance coverage for the managed Collector path.

### Changed

- Canonical telemetry terminology is now `telemetry.otlp/v1`; OpenTelemetry is treated as the ecosystem/instrumentation model and the Collector as one replaceable provider implementation.
- Shared provider registry records managed Collector placement as BaseHarbor-owned/shared and external OTLP destinations as externally owned.
- Repository workloads using managed OTLP attach to a dedicated BaseHarbor telemetry integration network; external OTLP bindings do not create that network.
- Global `baha destroy --yes` removes the shared BaseHarbor-owned Collector after application bindings are released.
- Common telemetry resource identity uses OpenTelemetry service/environment semantic attributes plus BaseHarbor application/resource/provider attribution.
- Shared provider lifecycle operations expose secret-safe metadata-only instrumentation hooks for preflight/apply/bind/verify so later observability providers can instrument the same core lifecycle.

### Security

- OTLP endpoints and provider topology remain deployment/provider state rather than portable application identity.
- Optional external OTLP authorization headers are accepted only from deployment/runtime state and injected at the trusted workload/provider boundary.
- Authorization headers and other credential material are not written to `baseharbor.yaml`, provider-registry state or normal diagnostics.
- Invalid, missing or ambiguous OTLP bindings fail before provider mutation.

### Compatibility

- Manifest version remains `1`; `telemetry.otlp` is additive and opt-in.
- Existing PostgreSQL, Valkey, OpenBao, secure-binding, managed exposure and S3 behavior remains compatible.
- Requesting OTLP transport does not provision Prometheus, Loki, Tempo or Grafana.
- Kubernetes/OpenShift collector realization, metrics/logs/traces storage providers and broader observability policy remain later roadmap work.


## [0.4.6] - 2026-09-19

### Added

- Versioned `object-storage.s3/v1` capability specification with logical bucket identity, S3-compatible application semantics and provider-neutral secure bindings.
- SeaweedFS 4.47 as the first lazy shared Compose reference provider for S3-compatible object storage.
- Manifest v1 `services.object_storage` bucket declarations plus deterministic `--s3` and `--s3-bucket` CLI paths.
- Standard host/workload S3 bindings and AWS-compatible environment variables without requiring a BaseHarbor SDK.
- Authenticated SigV4 Put/Get readiness verification in apply, up, status and doctor.
- Real Docker/Compose acceptance coverage for S3 provisioning, binding, restart, masking, isolation and destroy behavior.

### Changed

- Shared provider registry now records SeaweedFS as a BaseHarbor-owned shared provider while logical buckets remain application-owned resources.
- Repository workloads requesting S3 attach to a dedicated BaseHarbor object-storage integration network while provider-native topology remains deployment state.
- Global `baha destroy --yes` removes the shared BaseHarbor-owned SeaweedFS provider only after application bindings have been released.

### Security

- Every managed logical bucket receives separate access credentials and bucket-scoped SeaweedFS IAM authorization.
- SeaweedFS is explicitly started with IAM enabled; BaseHarbor does not persist a global S3 superuser credential.
- S3 access-key and secret-key values are owner-only and masked by default in `baha app env`.
- SeaweedFS IAM administration receives per-bucket credential commands through stdin rather than process arguments, keeping credentials out of runtime command lines and command-error rendering.
- Capability metadata and provider-registry state contain references/identity only, never plaintext S3 credentials.

### Recovery

- Application backup and restore fail closed for manifests containing managed object storage until object contents are part of the BaseHarbor recovery unit. BaseHarbor does not claim an incomplete S3 recovery as successful backup/restore.

### Compatibility

- Manifest version remains `1`; `services.object_storage` is an additive optional capability.
- Existing PostgreSQL, Valkey, OpenBao, secure-binding and managed-exposure behavior remains compatible.
- SeaweedFS is a reference provider, not application identity. Ceph RGW, AWS S3 and other conforming S3 providers can implement the same `object-storage.s3/v1` contract; dynamic external-provider loading/provider-selection UI remains future work.


## [0.4.5] - 2026-09-19

### Added

- Shared `secure-binding/v1` model for provider-neutral workload identity, credential references, trust material, authorization metadata, secret references, security lifecycle declarations and machine-readable diagnostics.
- Secure binding support in the shared capability lifecycle and `baseharbor.provider/v1` workload binding protocol.
- SPIFFE-based identity mapping for the existing managed-secrets/runtime-broker path.

### Changed

- Managed `secrets/v1` bindings now expose existing security semantics through provider-neutral references while preserving OpenBao as the current reference provider.
- Secure binding metadata is validated during plan construction before provider preflight or mutation.
- Secure binding references are restricted to opaque `baseharbor://` references.

### Security

- Plaintext credentials, credential-bearing URLs, tokens, private keys and secret values are rejected from the shared secure-binding reference boundary.
- Application/environment identity and credential references remain isolated across bindings.
- Provider-specific OpenBao AppRole, policy, KV and PKI internals remain protected provider state.

### Compatibility

- Manifest v1 and existing Compose/OpenBao/runtime-broker behavior remain compatible.
- No new application-facing security configuration is required.
- Human OIDC/RBAC/MFA/JIT/breakglass, full cross-provider rotation completion and managed public certificate issuance remain intentionally deferred.

## [0.4.4] - 2026-09-19

### Added

- Shared machine-readable endpoint/exposure core with stable logical workload-service identity and HTTP/HTTPS readiness semantics reusable outside the CLI.
- Versioned `exposure.http/v1` Capability Specification with provider-neutral logical route, target service/port, transport and `public|internal` visibility intent.
- Caddy as the first application-scoped Compose reference provider for explicit managed HTTP/HTTPS exposure behind the existing Provider Integration Contract and provider registry.
- Stable BaseHarbor exposure network attached only to explicitly exposed workload services through generated Compose override state, without rewriting application Compose source.
- End-to-end managed exposure lifecycle across apply/up, status/doctor, down/destroy, backup/restore and existing/BYOC TLS update.

### Changed

- Existing application-owned HTTP/HTTPS publishers now use the shared endpoint probing core while remaining application-owned observation/readiness state rather than managed exposure.
- The shared capability lifecycle supports staged prepare/preflight, provision/bind and verify phases so traffic providers can be coordinated around workload convergence without introducing a second lifecycle engine.
- Managed public exposure binds through the host-facing provider port; managed internal exposure is loopback-only in the Compose reference implementation.
- Managed HTTPS reuses existing/BYOC deployment TLS state and restarts/reconciles the Caddy provider when protected certificate material changes.

### Security

- Managed exposure preflight completes before traffic-provider mutation and fails closed on invalid provider/TLS prerequisites.
- Caddy provider convergence snapshots and rolls back BaseHarbor-owned state on failed new/changed realization.
- Destroy removes only BaseHarbor-owned routing/provider state; application Compose source and application-owned volumes remain untouched.
- FQDNs, host-published ports, certificate source paths, generated network names and Caddy configuration remain deployment/provider state rather than portable application intent.

### Compatibility

- Manifest version remains `1`; `exposure.http` is an additive optional contract extension. Existing manifests and app-owned publishers continue to work unchanged.
- Compose remains the complete current runtime provider.
- Managed ACME, OpenBao PKI issuance, automatic certificate renewal, Traefik, Kubernetes Gateway API/OpenShift Routes, cloud load balancers, service mesh and air-gap/private-registry work remain intentionally deferred.

### Fixed

- Repository inspection now treats an existing `baseharbor.yaml` as authoritative for application identity, declared capabilities, required secrets and workload selection while keeping heuristic evidence visible as supplemental signals.
- `app init --quick` no longer promotes heuristic credential-like names from env examples into mandatory managed secrets.
- Global and application status now distinguish an intentionally stopped runtime from a running-but-unready failure state.
- Provider-registry validation now happens during preflight before workload/runtime mutation.
- Failed repository workload starts/readiness attempts clean up resources created by that failed attempt without deleting application-owned persistent data or pre-existing workload state.
- Repeated reconciliation reuses valid runtime mTLS identities; actual identity rotation recreates the broker so bind mounts cannot retain stale certificate/key inodes.
- `app destroy` now shows preserved repository deployment/TLS state and supports explicit `--full-reset` for BaseHarbor-owned repository deployment state without touching external certificate sources.
- Added ownership-aware `baha destroy --yes` for explicit global control-plane/runtime-state removal after application bindings have been released.
- Fresh OpenBao bootstrap clearly asks for a new recovery output file, reuses filesystem completion, refuses an existing target and preserves the non-interactive `--recovery-file` path.


## [0.4.3] - 2026-09-18

### Added

- Read-only repository inspection core with extensible detectors and structured Detected/Suggested/Possible evidence.
- `baha app inspect [PATH]` with human-readable and `--json` machine-readable output.
- Detection evidence for Compose/Dockerfile, dependency manifests, env variable names, source imports, configuration endpoint patterns, ports and health checks.
- Secret-safe repository snapshots that discard env values and skip symlinked/generated/vendor trees.

- Provider Integration Contract v1 with versioned capability specifications and a shared semantic boundary for built-in and future external providers.
- Initial capability specifications for `database.sql/v1`, `cache.key-value/v1` and `secrets/v1`.
- Versioned Protocol Buffers schema for the future language-neutral external provider API.
- Static provider contract conformance foundation and reference integration descriptors for PostgreSQL, Valkey and OpenBao.

### Changed

- Guided `app init` now consumes the shared repository inspection engine instead of owning a separate CLI-only detector.
- Workload-only repositories no longer receive an invented PostgreSQL default when the workload itself is sufficient application intent.
- Compose capability detection is service/image scoped to reduce false positives from application environment configuration.
- `app init --quick` now fails closed when only Suggested/Possible evidence exists and no explicit workload is detected, instead of inventing a backend requirement.

### Architecture

- Provider protocol hardening now defines asynchronous operations, explicit unbind, idempotent mutations, deadline/cancellation rules, gRPC health/security expectations and safe protobuf evolution.
- Provider configuration uses JSON Schema 2020-12; GraphQL is explicitly reserved for possible future control-plane/query use rather than provider lifecycle.
- Future external provider transport is based on gRPC/Protocol Buffers and future package distribution on OCI standards, without introducing a dynamic plugin loader yet.
- Future provider distribution is digest-first, multi-platform through OCI Image Index, and uses OCI subject/referrers plus standard signature/SBOM/provenance mechanisms instead of proprietary BaseHarbor formats.
- All subsequent capability/provider integrations must use the shared lifecycle/registry contract and add capability-specific conformance rather than product-specific lifecycle paths.

## [0.4.2] - 2026-09-18

### Added

- Persistent provider registry for shared, application-scoped and external/BYO provider instances.
- Explicit provider ownership and lifecycle semantics for update, backup and destroy planning.
- Deterministic shared-provider reuse and fail-closed duplicate shared-provider detection.
- External provider bindings without BaseHarbor taking ownership of provider lifecycle.

### Changed

- PostgreSQL and Valkey are registered as BaseHarbor-owned application-scoped providers.
- Control-plane OpenBao is registered once as a BaseHarbor-owned shared provider reusable by multiple applications.
- Successful `app apply` and `app up` reconcile provider registry state; successful `app destroy` releases application bindings and application-owned provider records.

### Security

- Application-scoped providers cannot be bound across application ownership boundaries.
- Corrupt, duplicate or ambiguous provider registry state fails closed.
- Registry updates are serialized and persisted atomically with owner-only permissions.
- External provider records contain non-secret references only.

### Compatibility

- Manifest v1 is unchanged; provider placement remains deployment/operator state.
- Existing v0.4.1 applications are adopted on their next successful `baha app apply` or `baha app up`.

## [0.4.1] - 2026-09-18

### Added

- A reusable capability-provider domain core with typed capability requirements, provider descriptors, logical resources and workload bindings.
- Machine-readable capability lifecycle results for `plan -> preflight -> apply -> bind -> verify`, including structured diagnostics suitable for CLI, future Web UI/API and future Operator consumers.
- Reference capability descriptors for PostgreSQL (`database.sql`), Valkey (`cache.key-value`) and OpenBao (`secrets`).
- Protected runtime binding metadata now records resolved logical capability/provider/workload bindings alongside the existing standard connection bindings.

### Changed

- Existing Manifest v1 capability types now reuse the shared capability domain while remaining source-compatible inside the application package.
- Application planning resolves current PostgreSQL, Valkey and OpenBao requirements through fail-closed capability/provider negotiation before runtime mutation begins.
- Capability preflight completes for all planned resources before the reusable lifecycle permits provisioning mutation.

### Security

- Unsupported capability/provider combinations fail before mutation.
- Capability diagnostics and runtime metadata contain logical identities and provider names only; secret values and credential-bearing provider configuration remain outside the shared domain model.

### Compatibility

- Manifest v1, the current Compose developer workflow and existing v0.4 runtime behavior remain compatible.
- Provider registry/ownership policy, additional capability providers and Kubernetes/OpenShift/cloud implementations remain intentionally deferred to later v0.4.x issues.

## [0.4.0] - 2026-09-18

### Added

- A provider-neutral `PortableContract` seam that translates supported Manifest v1 application intent into logical capabilities without exposing Compose project names, host ports, deployment FQDN/TLS source paths or other provider implementation details.
- Runtime provider identity and capability negotiation with Compose as the current implementation and explicit extension points for future Kubernetes and OpenShift providers.
- Protected deployment-owned runtime-provider state through `BASEHARBOR_RUNTIME_PROVIDER`; legacy v0.3 state without the key safely resolves to Compose.
- A central fail-closed runtime-provider guard for remaining application runtime commands, including preflight, backup/restore, workload logs/shell/exec, update, TLS reload, status/doctor, down and destroy.
- A reusable declarative application input resolver supporting safe defaults, generated values, external/operator values and conditional `required-if` dependencies.
- Automation-safe `baha app init --input NAME=VALUE` injection for declared non-secret deployment inputs while preserving the existing dedicated flags.
- Resolver-driven repository deployment initialization for `hostname`, `tls_mode` and conditionally required `cert_dir`, shared by `baha app init` and the repository-aware `baha up` path.
- Architecture decisions documenting portable contract/provider boundaries, runtime-provider selection and contract evolution/versioning rules.

### Changed

- Manifest v1 is now explicitly treated as the supported compatibility surface and translated one-way into portable application intent rather than being treated as the permanent provider-neutral schema itself.
- Application requirements, runtime-provider selection and capability-provider/product selection are formally separate concerns; an application requests logical capabilities while deployment/platform policy chooses how they are realized.
- Contract evolution is fail-closed for unknown required versions or semantics; additive evolution remains preferred and provider-specific escape hatches, when eventually required, must remain optional and namespaced.
- Repository runtime operations now resolve the deployment-selected runtime provider and required runtime capabilities before entering the current Compose-backed implementation, preventing future providers from silently falling through into Docker/Compose code.
- `baha app init` and `baha up` ask only for unresolved deployment values when interactive. Complete protected state causes no additional questions; non-interactive operation uses only safe defaults/derivations and never invents an external certificate path.
- Compose remains the complete and first-class runtime implementation for v0.4. Kubernetes and OpenShift are intentionally not implemented in this release; the new seams are the compatibility boundary they will consume later.

### Security

- Unsupported or unavailable runtime providers fail explicitly instead of silently degrading to Compose or weakening requested runtime guarantees.
- Secret input values are represented explicitly, render redacted, and are excluded from generic persistable non-secret resolver output by construction.
- Runtime-provider capability negotiation fails closed when an operation requires behavior the selected provider cannot satisfy.
- Existing protected deployment state, certificate validation, secret storage and provider-specific security controls remain separate from the portable application contract and are not copied into committed manifests.

### Deprecated

- No public `baha` command or Manifest v1 field is deprecated in v0.4.0. Compose-specific Manifest v1 fields remain supported compatibility inputs, but they are no longer treated as the long-term provider-neutral application model.

### Removed

- Nothing from the supported v0.3 CLI or Manifest v1 workflow is removed in v0.4.0.

## [0.3.0] - 2026-09-17

### Added

- Trusted-local developer access workflows: `baha app psql`, `redis`/`valkey`, masked/reveal-on-demand credentials, workload logs, shell and exec using logical application/resource identities.
- Health-aware, service-level Compose workload truth shared by `baha app status`, `doctor` and `show`, including HTTP/HTTPS exposure readiness for conventional app-owned web publishers.
- `baha app show` as a read-only application overview with backend, workload, secret, backup and recovery readiness metadata without exposing secret values or credential-bearing URLs.
- Guided application backup and restore with secure no-echo password entry, explicit impact previews, verified recovery metadata and `Status: READY` only after successful post-restore verification.
- Safe Git-backed `baha app update --check` and strict fast-forward application updates, with dirty/diverged history protection, optional encrypted pre-update recovery points and protected update metadata.
- Guarded BaseHarbor self-update through `baha update --check` and explicit mutation, including release-asset/checksum verification, atomic replacement, retained recovery binary and rollback on failed post-update verification.
- Workload-only repository applications for explicit Compose workloads that do not require artificial PostgreSQL or Valkey dependencies.
- Automatic published-port fallback for configurable Compose bindings when host ports are already allocated, including IPv4 and IPv6 loopback/wildcard Docker error forms.
- Repository deployment runtime initialization for public FQDN and TLS mode while keeping deployment/runtime details outside the portable `baseharbor.yaml` application contract.
- Existing/BYOC TLS certificate lifecycle with `baha app tls update --check` and `baha app tls update`, including certificate/key/FQDN validation, downgrade protection, protected installation, restart and readiness verification.
- Linux terminal directory completion for interactive existing-certificate source selection without adding a new readline dependency.

### Changed

- Compose readiness now distinguishes running, starting, unhealthy, exited and missing services instead of treating every running container as READY.
- HTTP/TLS exposure failures now make the associated workload and whole application NOT READY; redirects are accepted as reachable exposure while 5xx/unreachable endpoints fail readiness.
- HTTPS readiness continues to probe the local published socket while using the configured public FQDN for HTTP Host and TLS ServerName, allowing hostname-bound application-owned TLS endpoints to be verified locally.
- Backup/restore interactive UX now retries short or mismatched passwords and shows reliable indeterminate progress without inventing percentage estimates.
- Restore workload verification now allows a bounded readiness window for real applications to reach service and HTTP/TLS readiness while remaining fail-closed.
- Application updates reuse the existing plan/preflight/apply/verify lifecycle after source fast-forward; durable applications require either an encrypted recovery point or explicit `--no-backup` acknowledgement before mutation.
- Self-update keeps stable as the default release channel, refuses downgrades, never invokes `sudo` automatically and reconciles the current repository application through the normal lifecycle when applicable.
- Repository workload port fallback preserves explicit operator environment overrides and persists BaseHarbor-selected fallback values in protected runtime state for later lifecycle commands.

### Security

- Guided backup/restore passwords are never accepted as command-line values and are passed to the existing hardened recovery path through owner-only in-memory file descriptors on Linux.
- Update metadata records non-secret state only; raw runtime errors, credentials and secret values are not persisted.
- TLS updates fail closed on invalid key pairs, FQDN mismatch, downgrade attempts or failed workload recovery and restore the previous protected certificate state on failure.
- Workload-only applications do not receive invented backend credentials, backend networks or services that they did not request.

## [0.2.0] - 2026-09-10

### Added

- End-to-end developer-journey acceptance that validates a clean MailFlow checkout against the exact BaseHarbor CLI/runtime under test.
- Release-facing CI coverage for occupied default control-plane ports, fail-closed missing application secrets, workload startup, health checks and restart behavior.
- Developer-journey documentation defining the product-level acceptance promise for future reference applications.
- Project-aware guided `baha app init` that detects common Compose files, PostgreSQL, Redis/Valkey, workload services and likely required secret names before asking setup questions.
- `baha app init --quick` for non-interactive manifest generation from unambiguous detections and safe defaults.
- Guided PostgreSQL and Redis/Valkey instance selection, including automatic proposals when multiple backend services are visible in the repository.
- Actionable required-secret readiness output with exact safe `baha app secret set <NAME> --stdin` remediation commands.
- Explicit generated application secrets in `secrets.required`, with bounded cryptographically secure `random` and `hex` generators persisted directly to managed OpenBao storage.
- Repository-aware `baha up` happy path that can start/reuse the control plane, prepare OpenBao, converge declared application backends, start the repository workload and verify readiness from one command.
- Explicit `baha up --control-plane-only` advanced mode for operators and CI flows that intentionally want to skip repository application convergence.

### Changed

- Interactive application initialization now follows the rule "detect first, ask only what is unclear" while the existing explicit flags remain the deterministic CI/automation path.
- Application initialization previews the generated manifest and never copies detected secret values into `baseharbor.yaml`.
- `baha app init --quick` now preserves multiple detected logical PostgreSQL and Redis/Valkey instances instead of collapsing them into one default service.
- Required-secret checks now distinguish configured, missing and unusable values and explicitly report when no application secrets have been configured yet.
- `baha app apply` now generates only explicitly declared missing generated secrets before the normal required-secret gate; existing values are never automatically overwritten or rotated, and external secrets remain fail-closed user input.
- A fresh managed-secret repository startup through `baha up` now asks for an operator-held recovery path interactively or requires `--recovery-file PATH` in non-interactive mode instead of requiring a separate OpenBao bootstrap command.
- `baha up` now routes detected application projects without `baseharbor.yaml` into the existing guided app-init flow; `--yes` uses only unambiguous detections and safe `app init --quick` defaults, while ambiguous projects remain fail-closed.
- `app.environment` is explicitly defined as deployment context rather than intrinsic application identity, preserving a path for the same logical application to run in multiple future environments/providers.
- Compose remains the complete v0.x runtime target while provider-specific details stay outside portable application requirements.
- MailFlow acceptance now validates the real MailFlow `main` branch instead of the temporary v0.2 compatibility branch used during pre-release integration.

### Removed

- Temporary MailFlow v0.2 validation-sync marker accidentally merged with the validation-only compatibility PR.

## [0.1.0-rc.1] - 2026-09-09

### Added

- Formal Semantic Versioning and GitHub Release process for the `baha` CLI.
- Reproducible release archives for Linux amd64 and arm64.
- SHA-256 checksums and GitHub build-provenance attestations for release artifacts.
- Versioned runtime container tags aligned with CLI releases.
- Verified release installer for released `baha` binaries.
- Bilingual GitHub Pages documentation with English as the default and German under `/de/`.

### Changed

- The moving development container tag is `edge`; `latest` is reserved for stable releases.
- Control-plane runtime state is user-global by default instead of repository-relative.
- Renovate automation is restricted to low-risk updates; platform, major and security-sensitive changes require review.
- Application runtime startup now retries transient loopback host-port bind races without changing credentials, database names, or persisted volumes.

### Release candidate scope

- This release candidate validates the real GitHub publishing path before `v0.1.0`.
- It is intentionally not marked as the latest stable release.

[Unreleased]: https://github.com/mcpdev80/baseharbor/compare/v0.4.7...HEAD
[0.4.7]: https://github.com/mcpdev80/baseharbor/compare/v0.4.6...v0.4.7
[0.4.6]: https://github.com/mcpdev80/baseharbor/compare/v0.4.5...v0.4.6
[0.4.5]: https://github.com/mcpdev80/baseharbor/compare/v0.4.4...v0.4.5
[0.4.4]: https://github.com/mcpdev80/baseharbor/compare/v0.4.3...v0.4.4
[0.4.3]: https://github.com/mcpdev80/baseharbor/compare/v0.4.2...v0.4.3
[0.4.2]: https://github.com/mcpdev80/baseharbor/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/mcpdev80/baseharbor/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/mcpdev80/baseharbor/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/mcpdev80/baseharbor/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/mcpdev80/baseharbor/compare/v0.1.0-rc.1...v0.2.0
[0.1.0-rc.1]: https://github.com/mcpdev80/baseharbor/releases/tag/v0.1.0-rc.1