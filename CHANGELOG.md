# Changelog

All notable changes to BaseHarbor are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/mcpdev80/baseharbor/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/mcpdev80/baseharbor/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/mcpdev80/baseharbor/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/mcpdev80/baseharbor/compare/v0.1.0-rc.1...v0.2.0
[0.1.0-rc.1]: https://github.com/mcpdev80/baseharbor/releases/tag/v0.1.0-rc.1