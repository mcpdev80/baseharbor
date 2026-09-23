# Changelog

All notable changes to BaseHarbor are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.14]

### Added

- Shared runtime-neutral reconciliation domain with typed desired, observed, diff and ownership state.
- Typed reconciliation states for `missing`, `in_sync`, `drift`, `conflict`, `foreign_ownership`, `unsupported` and `degraded`.
- Typed reconciliation actions for `create`, `noop`, `repair`, `destroy`, `observe` and `blocked`.
- Optional provider-native reconciliation observation integrated into the existing capability lifecycle without creating a second lifecycle engine.

### Changed

- Semantic providers are observed after preflight and before mutation; blocked ownership/conflict/unsupported/degraded states fail closed before `Provision`.
- Stable resources can resolve to a core-level NOOP without calling provider mutation hooks.
- BaseHarbor-owned drift resolves to minimal repair and is observed again after provider verification.
- Provider conformance now validates typed reconciliation results and verified convergence in addition to existing lifecycle/fault-injection coverage.

### Security

- Foreign ownership and competing reconciliation ownership block mutation.
- External ownership remains observe-only.
- Post-verification observation prevents successful lifecycle completion when BaseHarbor-owned desired state did not actually converge.
- No Kubernetes/OpenShift implementation, product-specific delivery mechanism or portable-contract expansion is introduced.

## [0.4.13] - 2026-09-21

### Added

- Deterministic repository environment resolution for root `baseharbor.yaml` and complete `envs/<environment>/baseharbor.yaml` contracts without hidden overlay inheritance.
- Environment-scoped protected deployment state under `.baseharbor/environments/<environment>/` for multi-environment deployments while preserving the existing single-environment state path.
- Typed policy result contract with `allow`, `warn` and `deny` decisions.
- `baha policy check` and `baha policy explain` with human and JSON output.
- Read-only MCP tools `baseharbor.policy.check` and `baseharbor.policy.explain`.

### Changed

- Repository lifecycle resolution now separates repository root, selected manifest path and environment-scoped deployment state.
- `-e/--environment` selection is handled by the common application resolver for repository-aware lifecycle/read operations.
- Backup, restore, update and destroy preserve the selected environment identity and state boundary.
- Existing Compose workload-isolation findings feed the shared policy model instead of forming a separate policy surface.
- Agent discovery now advertises six read-only semantic operations/tools.

### Security

- Managed environments cannot be downgraded to the development workload-security profile through `BASEHARBOR_WORKLOAD_SECURITY_MODE`.
- Operator policy exceptions are bounded: the existing development host-device acknowledgement remains explicit, while privileged containers and the other isolation-bypass classes are not overridable.
- Environment-specific state prevents deployment inputs, TLS material and generated runtime state from colliding across environments.
- Environment remains separate from runtime, topology, availability and future Kubernetes namespace details.


## [0.4.12] - 2026-09-21

### Added

- Versioned BaseHarbor machine contract `v1` for inspect, plan, status and doctor structured results.
- `baha agent describe` / `baha agent describe -o json` semantic discovery with operation safety metadata and supported capability specifications.
- Local stdio-only `baha mcp serve` built on the official Model Context Protocol Go SDK v1.8.0, targeting MCP `2026-07-28` with negotiated `2025-11-25` compatibility.
- Four read-only MCP tools: `baseharbor.inspect`, `baseharbor.plan`, `baseharbor.status` and `baseharbor.doctor`.
- Stable structured machine error categories and JSON error envelopes.
- Generic MCP-client acceptance covering discovery, read-only annotations, inspect/plan/status/doctor execution and secret-leak protection.
- English/German agent-machine-interface documentation.

### Changed

- Status+TLS and Doctor now expose shared typed result paths consumed directly by CLI, TUI and MCP instead of routing machine adapters through rendered CLI JSON.
- Inspect/plan/status/doctor machine results include additive `contract_version: "v1"` metadata.
- Bounded `AGENTS.md` guidance now directs coding agents toward structured BaseHarbor/MCP interfaces and away from shell/Docker/Compose bypasses.
- Repository development now integrates normal feature/fix/chore/dependency work through `develop`; `main` remains the released source line.
- Pre-release validation defaults to candidates contained in `develop`, while the release workflow continues to require tags contained in `main`.

### Security

- MCP exposes no generic shell, exec, Docker or Compose execution primitive.
- The initial MCP surface is read-only and local-only over stdio; no remote listener/auth surface is introduced.
- Machine output and MCP responses remain secret-safe and are covered by marker-secret acceptance.
- MCP annotations are descriptive hints only; BaseHarbor safety remains enforced by explicit semantic operations and the existing ownership/isolation/verification paths.

## [0.4.11] - 2026-09-21

### Final acceptance and lifecycle hardening

- `baha app destroy` now recovers safely from incomplete generated runtime state instead of refusing managed-secret applications. It derives canonical runtime paths, verifies exact Compose ownership, removes only ownership-verified expected runtime resources when generated Compose files are missing, and verifies OpenBao policy/AppRole ownership directly without depending on the application credential file.
- Repository workload/log-registration cleanup and full-reset state removal remain available during partial-runtime recovery; ambiguous resource ownership still fails closed.

- Ctrl-C handling is now reliable across long-running lifecycle operations and raw terminal prompts: the first interrupt cancels work immediately, a second interrupt exits immediately, and a single interrupt is force-bounded to two seconds if a child/cleanup path ignores cancellation.
- Linux path completion and hidden backup-password prompts now handle Ctrl-C locally while restoring terminal settings, and user cancellation exits with code 130 instead of being presented as a normal runtime failure.
- Best-effort workload cleanup after cancellation is capped at two seconds instead of waiting up to thirty seconds.

- Provider preflight/status visibility now follows explicit application capability intent: undeclared object storage, traces, telemetry, metrics, logs and exposure providers are absent from normal output instead of appearing as meaningless OK checks. Obsolete logs/metrics state can still be discovered internally for cleanup.
- Fixed the Manifest v1 logs validation insertion regression that broke compilation after the explicit-logs change.

- Capability-intent audit completed: metrics, telemetry, traces, exposure, object storage, secrets and runtime broker already gate provider lifecycle behind explicit application intent. Logs now also participate in the central PortableContract/capability-binding model, and regression tests enforce that deployment/environment policy cannot invent undeclared provider capabilities.

- Application log collection is now explicit Manifest v1 intent via `logs.collect`; `environment: dev` no longer silently provisions Loki/Alloy. Without declared logs intent, BaseHarbor removes stale workload log overrides/registrations and skips log-ingestion verification.
- `BASEHARBOR_LOGS_ENABLED` may disable declared logging but can no longer create undeclared application capability intent.

- Repeated `baha up` is now a true bounded no-op when both the existing control plane and repository application are already READY and the successfully applied desired-state fingerprint still matches; STOPPED applications use the existing start path, while drift/degraded state falls back to full reconciliation.
- Repository convergence records a protected desired-state SHA-256 fingerprint only after successful verification. The fingerprint covers repository source inputs plus deployment realization state, so READY alone can never suppress a real source/Compose/manifest/deployment change.
- Apply output now labels static plan entries as `desired actions` rather than incorrectly calling them detected `changes`.

- Shell-style interactive path prompts now keep the displayed current working directory static while editing; relative/absolute path input and completion only affect the text to the right of `$`.

- Interactive Linux path prompts now use literal shell-style `label:path$` syntax, with `~` under the user's home directory; OpenBao bootstrap/unseal prompts use `OpenBao-recovery-key` / `OpenBao-decrypt-key` labels.

- Interactive Linux path prompts now use a shell-like current-directory prefix (`~/.../` or `/.../`) directly on the input line, replacing the separate path-base/browsing blocks.

- Interactive Linux path prompts now show a live `Browsing` directory that follows the currently typed relative, absolute or `~/...` path while preserving the original process working directory.

- Interactive path prompts now always show their current path base even when the input reader cannot provide a raw terminal file descriptor; tab completion remains optional while path orientation is guaranteed.

- Repository-aware first `baha up` once again resolves configurable Compose host ports before the first workload start, persists BaseHarbor-selected values in protected deployment state and reuses the existing post-start bind-conflict retry only as a race-condition fallback.

- Compose-backed lifecycle activities now stream secret-safe live detail such as image pulls, builds, container creation/start and readiness phases into the current CLI activity instead of hiding runtime progress until completion.

- Long-running CLI lifecycle activities now expose elapsed time while running and include actual duration on completion, so slow fresh-start phases can be identified without verbose/debug mode or guessed ETAs.

- Interactive file and directory prompts now show the current path base and explain where relative paths resolve, so recovery, TLS/certificate and future path selection remain orienting and predictable.

- Interactive `baha up` now reprompts in-place when a fresh OpenBao recovery output path already exists or an unseal recovery path is invalid, instead of aborting the guided setup; explicit/non-interactive paths remain deterministic and fail closed.

- Repository applications that were intentionally destroyed or have never been applied now report `NOT APPLIED` in status/doctor instead of synthetic downstream service failures and `DEGRADED`; the next action is normal `baha up` / `baha app apply`.

- Repository workload stop/down/destroy no longer resolve required secret payloads merely to render and stop existing Compose services; start/apply paths continue to require real secret resolution.

- `baha app destroy` now remains ownership-safe but recovery-capable: managed-runtime template drift no longer blocks deletion, and an uninitialized OpenBao control plane is treated as having no application scope to delete; sealed or ambiguous OpenBao still fails closed.

- `baha app doctor --fix` now fails closed when the existing OpenBao control plane is uninitialized or sealed instead of continuing into application reconciliation.

- Real MailFlow acceptance verified clean repository adoption, fresh startup, no-op `baha up`, STOPPED -> READY recovery, encrypted backup/restore, strict fast-forward application update, TLS readiness, required/generated secrets and ownership-safe destroy behavior.
- OpenBao required-secret reads are batched per observation instead of repeating full scope/login/list/read work per key, substantially reducing normal `status` and `doctor` latency.
- Fresh OpenBao recovery prompts now render their question and path input reliably before blocking for input.
- `baha app update --check` now reports the exact dirty working-tree paths and change classes that block automatic update while preserving the fail-closed no-reset/no-stash policy.
- Provider-registry helper call regressions found during fresh acceptance were corrected before release.

### Added

- Repository adoption workflow with remote HTTPS/SSH Git inspection through normal Git authentication.
- Repository-aware root shortcuts for `plan`, `status` and `doctor`.
- Secret-safe `-o json` / `--output json` result paths for inspect, plan, status and doctor.
- Optional idempotent bounded BaseHarbor guidance in `AGENTS.md` through `baha app init --agents`.
- Five-minute onboarding and first-class local Compose Playground documentation.
- Bash, Zsh and Fish shell completion generated by `baha completion`, including fixed-value environment completion.
- Shared terminal UX renderer with semantic result states, delayed progress feedback and stable non-TTY output.
- Read-only `baha tui` dashboard with Overview, Status and Doctor views backed by the same structured health models as the CLI.
- Global `--plain`, `--no-input`/ `--non-interactive` and root `--version` controls.
- Read-only configured-application completion and typo suggestions for commands/high-frequency flags.
- Bash completion now honors the active cursor word through `COMP_CWORD` and correctly splits value/description records on tabs, including completion immediately after a subcommand space.

### Changed

- `baha up -e ENV` / `--environment ENV` selects deployment context without rewriting the portable repository manifest.
- `baha app init -e ENV` accepts the same short environment alias.
- Human and JSON application status use the same readiness result model.
- Doctor JSON is produced directly from shared preflight results rather than terminal rendering.
- `baha app inspect --json` remains a compatibility alias while `-o json` is the canonical structured-output form.
- Human status and doctor views are grouped into scan-friendly sections with one final overall state and actionable next commands.
- Successful mutations use precise result states such as `CREATED`, `UPDATED`, `DELETED`, `REMOVED`, `STARTED`, `STOPPED`, `READY` and `VERIFIED` instead of generic `OK`.
- `--quiet`/`--silent`, `--verbose` and `--no-color` are handled consistently as global human-output controls.
- Potentially slow lifecycle operations show delayed contextual progress; fast operations do not flash activity indicators and CI/non-TTY output remains line-oriented.
- Human result details and help text wrap to terminal width; long help may use `$PAGER` only on a real TTY.
- `-v` is reserved consistently for `--verbose`; version remains available through `baha version` and `baha --version`.
- Broken-pipe/EPIPE termination is silent for normal Unix pipelines.
- Application status is now a bounded fast snapshot rather than a readiness wait loop; broker and subsystem checks no longer make `baha status` appear hung.
- Normal status hides low-level Compose/curl/OpenBao diagnostics behind `--verbose`, and repository TLS renders inside the same status hierarchy before the final READY/DEGRADED state.
- Application doctor now follows the same concise human-output policy: raw Compose/OpenBao/curl diagnostics stay behind `--verbose`, duplicate workload problems and empty workload-service sections are suppressed, TLS is integrated before the final health state, and repair guidance is presented through the normal Next actions.
- TUI Status and Doctor views use the same concise human-detail mapping as the CLI, so raw Compose/OpenBao/curl diagnostics never leak into the interactive dashboard.
- `baha app doctor --fix` now classifies repairability from structured doctor results instead of rendered terminal text, so human-output changes cannot silently disable safe repair.
- Safe doctor repair can restore an existing stopped BaseHarbor control plane before application convergence and then uses the normal application apply path. BaseHarbor-declared required secrets no longer block read-only Compose security rendering, and every repository workload start is security-checked again immediately before start.

- Cross-command lifecycle audit aligned workload-security behavior across preflight, apply, up, restore, backup restart and TLS reload paths.
- Human, JSON and TUI application health now include the same repository TLS observation.
- Repository manifest discovery now uses a typed not-found sentinel rather than parsing error-message strings.
### Security

- Remote Git URLs containing embedded userinfo/credentials are rejected; authentication is delegated to normal Git mechanisms.
- Structured status/doctor output excludes secret values and reports only readiness metadata for required secrets.
- Structured doctor output is read-only and cannot be combined with `--fix`.
- BaseHarbor edits only its own bounded `AGENTS.md` section and fails closed on malformed or ambiguous markers.
- Weak or ambiguous repository evidence remains non-authoritative and never silently grants runtime permissions or replaces infrastructure.
- `NO_COLOR`, `TERM=dumb` and reduced-motion mode suppress visual effects without changing command semantics.
- Progress output never invents percentages or ETAs, and quiet mode preserves failure diagnostics.
- Structured JSON paths remain isolated from ANSI/color/progress rendering.
- `--no-input` guarantees guided flows never prompt and fail closed with actionable explicit-input instructions.
- The TUI is read-only and refuses non-interactive, plain, piped and CI execution.


## [0.4.10] - 2026-09-20

### Added

- Generic provider observability declarations in Provider Integration Contract v1 for provider-owned metrics/log/trace signals without product-specific collector branches.
- Protected provider metrics source registry with placement, sharing-boundary and application-authorization filtering.
- Versioned `traces/v1` platform contract and Tempo 3.0.2 as the first managed shared Compose trace-storage reference provider.
- Managed OpenTelemetry Collector to Tempo routing with real end-to-end verification: the BaseHarbor verification trace must be queryable from Tempo before trace storage is READY.
- Provider metrics auto-registration for the managed OpenTelemetry Collector, Loki and Tempo when metrics collection policy allows provider signal classes.
- Real Prometheus `up=1` verification for registered provider metrics.

### Changed

- Metrics collection defaults include safe metrics advertised by managed application/platform providers when the metrics facility is enabled.
- Prometheus provider state now reconciles generic provider targets and attaches only the provider networks required by authorized registrations.
- Loki's internal provider network has a stable BaseHarbor-owned name so an authorized metrics collector can join it without broadening application connectivity.
- OTLP transport and trace retention remain separate: requesting `telemetry.otlp/v1` alone still does not start Tempo.
- Tempo is deliberately limited to the default shared Compose placement in v0.4.10; unsupported application/external/named-boundary placement fails before mutation.

### Security

- Shared Prometheus filters application-scoped provider signals by the exact applications registered in that Prometheus sharing boundary.
- Provider observability reachability is a dedicated BaseHarbor-managed collector path and does not grant applications access to shared provider networks.
- Tempo runs non-root with a read-only root filesystem, all Linux capabilities dropped, `no-new-privileges`, explicit writable storage/tmpfs and a loopback-only host API.
- Provider observability state contains endpoint identity/labels only and no credentials or secret-bearing URLs.


## [0.4.9] - 2026-09-20

### Added

- Versioned `logs/v1` lifecycle semantics and Loki 3.7.8 as the first Compose log platform provider.
- Grafana Alloy 1.19.2 forwarding from selected repository workload services into Loki without a Docker/Podman socket.
- Shared/default, named shared-boundary and application-scoped Loki placement through the existing provider-placement model.
- Real Loki readiness and query-based ingestion verification before the logs path is READY.
- Repository Compose workload security preflight with machine-readable allow/warn/deny findings.
- Reusable executable Provider Integration Contract conformance harness and deterministic fake provider.
- Fault-injection coverage for CREATE/NOOP, drift/repair, provider outages, malformed bindings, verify failure, retry convergence and ownership-safe destroy.

### Changed

- Logical log resources and Loki placement are reconciled through the protected provider registry.
- The application provider registry now consumes the canonical Provider Integration Contract descriptor mapping instead of maintaining a duplicate provider switch.
- Development log collection defaults on; test/staging/production remain opt-in platform policy.
- `app apply`, `app up`, `status`, `doctor`, `down` and `destroy` now reconcile the managed log lifecycle where enabled.

### Security

- Repository workloads that request privileged mode, host network/PID/IPC, runtime sockets, dangerous capabilities or critical host mounts fail closed in managed environments before workload mutation.
- Development-only exceptions require explicit acknowledgement; host devices are warnings by default in development and denies in managed environments.
- Loki and Alloy run read-only, drop Linux capabilities, use `no-new-privileges`, receive no container-runtime socket and expose host-facing ports on loopback only.
- Log registration state and provider files are owner-only and contain no credentials.


## [0.4.8] - 2026-09-20

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
- Minimal platform-level cross-application connectivity commands: `baha connect SOURCE TARGET`, `baha disconnect SOURCE TARGET` and `baha connections`.
- Directed Compose connectivity realized through a hardened BaseHarbor Runtime relay instead of a shared source/target bridge network.

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
- Provider placement semantics are explicit: `application` means a dedicated provider instance for one application/environment; `shared` means lazy BaseHarbor Platform/Core Runtime infrastructure; `external` remains externally lifecycle-owned.
- Cross-application connectivity is independent from provider sharing; `app down` suspends relay runtime while preserving policy and `app up`/`apply` reconcile it.
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
- Cross-application connectivity is deny-by-default and directional; target services never join source link networks, and connectivity relays have no host-published port or Docker/Podman socket and run hardened non-root/read-only.

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

[Unreleased]: https://github.com/mcpdev80/baseharbor/compare/v0.4.11...HEAD
[0.4.11]: https://github.com/mcpdev80/baseharbor/compare/v0.4.10...v0.4.11
[0.4.10]: https://github.com/mcpdev80/baseharbor/compare/v0.4.9...v0.4.10
[0.4.9]: https://github.com/mcpdev80/baseharbor/compare/v0.4.8...v0.4.9
[0.4.8]: https://github.com/mcpdev80/baseharbor/compare/v0.4.7...v0.4.8
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