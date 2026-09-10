# Changelog

All notable changes to BaseHarbor are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/mcpdev80/baseharbor/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/mcpdev80/baseharbor/compare/v0.1.0-rc.1...v0.2.0
[0.1.0-rc.1]: https://github.com/mcpdev80/baseharbor/releases/tag/v0.1.0-rc.1
