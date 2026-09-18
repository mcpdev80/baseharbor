# v0.3.0 pre-release documentation audit

This document is the current documentation and release-scope gate for BaseHarbor `v0.3.0`. The historical v0.2.0 audit is preserved under `docs/release-audits/v0.2.0.md`.

## Evidence basis

The v0.3.0 scope was reviewed against the actual repository history, not only the latest pull requests. `release/v0.3.0` is 159 commits ahead of `v0.2.0` at the start of this audit.

The implemented v0.3 Compose scope includes, at minimum:

- trusted-local developer access (`psql`, Redis/Valkey clients, masked credentials, logs, shell and exec);
- health-aware service-level repository workload truth shared by `show`, `status` and `doctor`;
- active application-owned HTTP/HTTPS exposure readiness;
- application overview plus protected backup/recovery metadata;
- guided encrypted backup and restore with verified recovery and fail-closed READY semantics;
- strict fast-forward Git-backed application update with explicit recovery policy for durable state;
- guarded BaseHarbor self-update with release-artifact verification, atomic replacement and rollback;
- explicit workload-only Compose applications without invented managed backend dependencies;
- configurable workload host-port fallback, including IPv4 and IPv6 Docker bind-conflict forms;
- protected repository deployment initialization for Public FQDN and TLS mode;
- existing/BYOC certificate validation, update checking, downgrade protection, protected installation, reload and readiness verification;
- Linux terminal directory completion for certificate source selection without a new readline dependency.

The detailed release summary is maintained in `CHANGELOG.md`.

## Architecture alignment

`app.name` remains stable logical application identity. `app.environment` remains deployment context.

`baseharbor.yaml` remains the portable, repository-owned application desired-state contract. Compose-specific realization remains provider/operator state and must not be promoted into the common manifest merely because the current provider needs it.

For v0.3, protected deployment/runtime state may contain:

- Public FQDN;
- deployment TLS mode;
- normalized existing/BYOC certificate and key material;
- automatically selected published-port fallbacks;
- generated Compose overrides and runtime identity material.

This is consistent with ADR 0005: application contracts describe capabilities/portable requirements, while concrete product/runtime realization belongs behind provider/environment boundaries.

Compose is the complete v0.3 runtime provider and remains first-class. The following are explicitly **future work**, not v0.3 claims:

- Kubernetes and OpenShift runtime providers;
- HA/topology profiles;
- generic runtime/capability provider schema in the application manifest;
- provider-neutral `ingress.http` or `tls.certificate` capabilities;
- BaseHarbor-managed ACME issuance/renewal;
- OpenBao PKI issuance/rotation for application ingress certificates;
- object-storage provider implementation;
- managed-production OIDC/RBAC/JIT access policy.

v0.3 does verify application-owned HTTP/TLS exposure and does manage existing/BYOC certificate state for the current Compose deployment. That must not be described as a BaseHarbor-managed ingress or generic certificate provider.

## Development-guideline alignment

### Product interface and lifecycle

- `baha` remains the primary operator/developer product interface.
- read-only checks are distinct from mutation (`app update --check`, `update --check`, `app tls update --check`).
- mutating paths keep preflight/validation before mutation and capability/readiness verification afterward.
- success is not inferred from a started container alone.

### Fail-closed behavior

- missing/unusable required secrets block workload startup;
- ambiguous repository/Compose selection does not become an implicit guess;
- dirty, ahead or diverged Git application updates do not mutate source;
- TLS mismatch/downgrade or failed recovery does not become a successful update;
- malformed/wrong-identity/wrong-password recovery archives stop before destructive restore work where applicable;
- restore still fails when the verified application boundary does not become READY inside the bounded readiness window.

### Secrets and sensitive state

- application manifests store secret names, not values;
- guided backup/restore does not accept passwords as normal argv values;
- status/show/doctor and update metadata do not persist or print secret values or credential-bearing URLs;
- generated runtime, certificate and credential material remains protected/owner-only.

### Backup and restore

Backup is paired with exercised restore. The real MailFlow recovery path has been used to drive release fixes, including IPv6 published-port conflict handling and the bounded post-restore readiness window. The final release still requires the repository's automated real-product backup/restore acceptance on the exact release-preparation head.

### Scope and maintainability

The v0.3 changes remain incremental around the existing Compose provider. Future provider-neutral abstractions, Kubernetes/OpenShift, HA and managed ingress/PKI are not pulled into this release merely to anticipate later roadmap phases.

## Documentation gate

Before the release-preparation branch is considered ready:

1. README and canonical English documentation must describe v0.3 current behavior consistently.
2. Maintained German documentation must not contradict the canonical English contract.
3. `CHANGELOG.md` must contain a dated `0.3.0` section covering the actual release scope.
4. The CLI reference must include the v0.3 public command surface, including developer access, application update, BaseHarbor update and deployment TLS update.
5. Architecture/application-contract docs must separate portable application requirements from Compose deployment/runtime state.
6. Documentation must distinguish application-owned HTTP/TLS readiness and existing/BYOC lifecycle from future managed ingress/ACME/PKI capabilities.
7. Historical ADR/release statements must remain historical rather than being rewritten as if they were authored for v0.3.
8. The release-documentation change itself must contain no unrelated runtime or feature implementation.

## Final release gate

Documentation alignment does not by itself make the release green.

Before tagging `v0.3.0`:

1. run all required CI and real-product acceptance gates on the exact final release-preparation head;
2. inspect failures before rerunning and fix deterministic defects rather than retrying them blindly;
3. merge the final release-preparation PR to `main` only after the required gates are green;
4. tag `v0.3.0` on that exact green `main` commit;
5. let the release workflow validate source/tag/changelog consistency and publish artifacts/provenance;
6. verify released amd64/arm64 archives, checksums, build provenance and `ghcr.io/mcpdev80/baseharbor-runtime:0.3.0` version coupling before declaring the release usable.

A tag must never be moved after publication. A bad published release is corrected with a new patch version.
