# Pre-release documentation audit

This file is the active documentation/release gate for the **next unreleased BaseHarbor version**.

Target release: `v0.4.5`.

The latest published stable release before this release is `v0.4.4`. Completed release audits are preserved under `docs/release-audits/`.

Before preparing the next release, update this document with the concrete target version and audit the actual implementation rather than copying claims from the previous release.

## Required audit areas

### Scope and architecture

- Compare the target release issues/roadmap with the actual code.
- Confirm current behavior and future architecture are clearly separated.
- Confirm application contract, deployment state, runtime providers and capability providers remain separate responsibilities.
- Confirm new shared lifecycle behavior belongs in the common core rather than only in CLI/API/UI/Operator presentation layers.
- Reject speculative abstractions that are not required by the target release.

### Product behavior

- Verify the exact CLI/API/operator behavior claimed by documentation.
- Confirm mutation paths retain plan/preflight/apply/verify semantics where applicable.
- Confirm readiness reflects actual protocol/application readiness.
- Confirm unsupported providers/capabilities fail clearly instead of silently degrading guarantees.
- Confirm compatibility and migration behavior are explicitly documented.

### Security and recovery

- Confirm secret values are not exposed through logs, errors, API responses, audit output, metrics, tests or committed manifests.
- Confirm protected runtime/deployment state remains outside the portable application contract.
- Confirm backup claims include exercised restore and post-restore verification.
- Confirm risky updates have preflight and post-change verification.

### Documentation

Audit at minimum:

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
- relevant ADRs;
- executable CLI help and acceptance workflows.

Historical release audits and ADRs should remain historical. Do not rewrite them merely to make old text look current.

## Validation policy

Documentation-only corrections use the lightweight documentation/Pages gate.

Run expensive runtime, MailFlow, broker, backup/restore or full product acceptance only when executable code, runtime/deployment configuration, dependencies, CI/release mechanics or other behavior-affecting files change.

Before a product release tag is created, the exact final release-preparation head must have the validation required by `docs/DEVELOPMENT_GUIDELINES.md`.

## Release completion

After the next release is successfully published:

1. archive the completed target-specific audit under `docs/release-audits/vX.Y.Z.md`;
2. update README/current-version installation examples;
3. return this file to an unreleased/next-cycle state;
4. do not move or recreate the published tag.

## v0.4.5 audit findings

Scope reviewed against #160 and the Provider Integration Contract v1.

- The change extracts security semantics from the existing OpenBao/runtime-broker/mTLS path; it does not replace that implementation.
- Secure-binding logic lives in the shared capability/application core, not CLI-only code.
- Manifest v1 remains unchanged.
- Provider-specific OpenBao paths, AppRoles, policies, RoleIDs/SecretIDs, certificate paths, private keys and secret values remain outside portable application intent.
- Secure-binding references are opaque `baseharbor://` references and credential-bearing URLs are rejected.
- SPIFFE workload subjects reuse the existing runtime client identity scheme.
- Invalid secure bindings fail during plan construction before provider preflight/mutation.
- Cross-application and cross-environment logical identity/credential separation has explicit negative coverage.
- Renewal/rotation/revocation are capability declarations only; broader cross-provider rotation remains deferred.
- Human OIDC/RBAC/MFA/JIT/breakglass remains v0.6 work.

Canonical documentation reviewed/updated for v0.4.5:

- `README.md`
- `CHANGELOG.md`
- `docs/application-contract.md` + DE
- `docs/architecture.md` + DE
- `docs/capability-provider-model.md` + DE
- `docs/provider-integration-contract.md` + DE
- `docs/secrets-and-openbao.md` + DE
- `docs/runtime-secret-broker-security.md`
- `docs/roadmap.md` + DE
- `docs/releases.md`
- `spec/capabilities/secrets/v1.md`
- `spec/bindings/secure/v1.md`
- `spec/provider/v1/provider.proto`
- `docs/releases/v0.4.5.md`

Pre-final-head external validation completed through Hugging Face:

- full `gofmt` check: PASS
- `go test ./...`: PASS
- `go vet ./...`: PASS
- `go build ./cmd/baha`: PASS
- `mkdocs build --strict -f mkdocs.yml`: PASS
- `mkdocs build --strict -f mkdocs.de.yml`: PASS
- stale current-version/status search: PASS
- `protoc` descriptor compilation for `spec/provider/v1/provider.proto`: PASS

The exact final PR head must repeat these gates after this audit commit.
