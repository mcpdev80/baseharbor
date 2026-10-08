# Core provider update contract — v0.4.24

Status: **implementation in progress**. This document describes required behavior, not proof of a supported end-to-end provider upgrade.

## Authority and boundaries

A selected BaseHarbor release must own immutable provider image digests, versions, compatibility metadata and the complete SQL / Secrets / Identity reference set. No mutable `latest` resolution is permitted. The current provider pin constants in runtime/identity assets are not yet a versioned release manifest.

The Core update domain is implemented in `internal/coreupdate`. It operates on **owned realizations**, not merely provider types: installation, placement scope, instance and owner form the identity. An external or foreign provider must not be changed automatically.

The planner distinguishes:

- `no_change`: version, image and digest all identical;
- `safe_reconcile`: an explicitly proven compatible stateless path;
- `backup_recovery_required`: a supported data-bearing path with a verified recovery point;
- `migration_required`: a supported schema/data migration with explicit verification and recovery requirements;
- `unsupported`: refuse before mutation and report the actionable reason.

A PostgreSQL major version change is currently unsupported by the generic planner: it needs a separately proven, provider-native migration path. Image replacement alone is never evidence that data was upgraded.

## Transaction semantics

1. Resolve the exact BaseHarbor release and all Core expected digests.
2. Inspect installed owned shared and application-isolated realizations; discover any external providers without taking ownership.
3. Compute the plan and preflight **all** changes before mutation.
4. Require verified recovery-point hooks for every data-bearing upgrade.
5. Apply through the existing provider-native Core lifecycle and persist stage/result evidence.
6. Probe actual SQL, Secrets and Identity usability, not merely container readiness.
7. Resume incomplete owned state idempotently. Never promise rollback for irreversible data migrations.
8. Report success only when every affected realization and Core semantics verify successfully.

## Human and machine parity

`baha update --check`, human update output, JSON, MCP and protected HTTP must be projections of the same plan/result domain. The existing self-update implementation only handles binary/release assets and MUST NOT report a complete Core upgrade until the provider update integration and semantic verification are present.

## Release evidence required

- Immutable provider set and installed-version discovery
- No restart for unchanged realizations
- PostgreSQL compatible upgrade, plus fail-closed unsupported major upgrade
- OpenBao sealed/unsealed and recovery state preservation
- Keycloak post-migration authentication/issuer verification
- Shared updated exactly once; isolated handled independently
- Foreign provider untouched
- Injected partial failure and retry, without duplicated state
- Docker rootless and Podman rootless runtime proof
- Evidence bound to the release candidate SHA and provider digests

The initial planner and hook tests prove only contract behavior; **they do not qualify the real provider mutation paths**.
