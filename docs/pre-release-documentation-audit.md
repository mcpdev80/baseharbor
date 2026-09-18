# v0.4.0 pre-release documentation audit

This document is the current documentation and release-scope gate for BaseHarbor `v0.4.0`. The historical v0.3.0 audit is preserved under `docs/release-audits/v0.3.0.md`.

## Evidence basis

The v0.4 scope was reviewed against the implementation, architecture decisions, public documentation, issue acceptance criteria and exact-head CI evidence.

The release-preparation head `7cefb4b581ee250e62f90a03fc9393b3fa5e8d60` passed all 10 release-preparation workflows before merge, including:

- main CI;
- release snapshot;
- developer journey;
- real MailFlow acceptance including backup -> destroy -> restore;
- PostgreSQL backup acceptance;
- OpenBao backup acceptance;
- application backup/restore acceptance;
- runtime broker acceptance;
- runtime broker performance;
- documentation/pages build.

Release preparation was merged to `main` as `993c5c0102efd50935946f35c8b96aec816f763b`.

A published `v0.4.0` release must still be created from that exact release-ready source or from a later documentation-only correction that itself passes the complete release gate. Until the immutable tag and release exist, `v0.3.0` remains the latest published stable release.

## Implemented v0.4 scope

v0.4 adds the architecture and DX seams needed to evolve beyond the current Compose runtime without changing the logical application contract:

- provider-neutral `PortableContract` compatibility view for Manifest v1 application intent;
- explicit separation between runtime-provider selection and capability-provider selection;
- deployment-owned runtime provider and profile state;
- runtime capability negotiation and fail-closed unsupported-provider behavior;
- centralized guards preventing application runtime commands from falling through into Compose-specific code when an unsupported provider is selected;
- reusable declarative input resolution for default, generated, external and conditional values;
- resolver-driven repository deployment initialization shared by `baha app init` and repository-aware `baha up`;
- explicit non-secret automation input through `--input NAME=VALUE`;
- contract-evolution/versioning rules that preserve Manifest v1 compatibility while requiring new versions for incompatible required semantics.

Compose remains the only complete runtime implementation in v0.4. Kubernetes and OpenShift are future providers, not release claims.

## Development-guideline alignment

### Scope and architecture

- Changes were incremental rather than a runtime rewrite.
- Application requirements, deployment state, runtime providers and capability providers remain separate responsibilities.
- Manifest v1 remains a compatibility surface; provider-specific runtime objects are not added to the portable application contract.
- No speculative Kubernetes/OpenShift implementation or generic plugin framework was introduced.
- Stable concepts use typed domain models for provider kind, runtime profile, runtime capabilities and portable contract requirements.

### Product interface and lifecycle

- `baha` remains the stable product interface.
- Existing v0.3 Compose workflows remain compatible.
- Application runtime mutation paths continue to use preflight/verification semantics.
- Provider selection is resolved before guarded runtime operations.
- Unsupported providers or capabilities fail clearly before mutation instead of silently degrading to Compose.

### Security and fail-closed behavior

- Runtime provider/profile state is protected deployment state, not committed application configuration.
- Unknown provider/profile values fail closed.
- Secret input values are explicitly marked, render redacted and are excluded from generic persistable values.
- `--input NAME=VALUE` is intentionally limited to declared non-secret deployment inputs.
- Existing secret storage, runtime identity, backup/restore and TLS security boundaries remain separate from generic input resolution.
- Backup remains supported only with exercised restore and post-restore verification.

### Documentation

Canonical documentation must distinguish:

- Manifest v1 compatibility from the internal provider-neutral `PortableContract`;
- current Compose implementation from future Kubernetes/OpenShift providers;
- runtime-provider selection from SQL/cache/secrets capability-provider selection;
- deployment/operator inputs from portable application requirements;
- implemented existing/BYOC TLS lifecycle from future managed ACME/PKI/provider-neutral TLS capabilities;
- release-ready source from an actually published stable GitHub release.

Historical ADRs and release audits remain historical and are not rewritten to pretend they were authored for v0.4.

## Maintained documentation surface

The release gate covers at minimum:

- `README.md`;
- `CHANGELOG.md`;
- `docs/cli.md`;
- `docs/application-contract.md`;
- `docs/architecture.md`;
- `docs/capability-provider-model.md`;
- `docs/input-resolution.md`;
- `docs/roadmap.md`;
- `docs/releases.md`;
- maintained German documentation under `docs/de/`;
- ADRs 0005 through 0008;
- executable CLI help and acceptance workflows.

## Final release gate

Before publishing `v0.4.0`:

1. documentation and code must agree on current behavior;
2. all required workflows must succeed on the exact final release-preparation head;
3. merge the release-preparation/fix PR only after that exact head is green;
4. create immutable tag `v0.4.0` on that exact green main commit;
5. let the release workflow validate tag/source/changelog consistency and publish artifacts;
6. verify Linux amd64/arm64 archives, checksums, build provenance and matching runtime image;
7. only then describe `v0.4.0` as the current published stable release.

A published tag must never be moved. A bad release is corrected with a new patch version.
