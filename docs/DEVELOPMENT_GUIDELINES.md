# BaseHarbor Development Guidelines

These rules apply to code, tests, documentation and provider/runtime work.

## 1. Smallest correct design

Implement only what the current requirement needs. Avoid speculative frameworks, unrelated refactors and product-specific shortcuts.

## 2. Secure and fail closed

Ambiguous ownership, missing authorization, invalid state, unsupported semantics and unverifiable security assumptions are not success.

## 3. Contracts, not products

Applications describe capabilities. Provider/runtime product choices stay behind BaseHarbor boundaries.

## 4. Keep provider axes separate

```text
runtime != capability != delivery
```

Do not hide one provider axis inside another.

## 5. Secrets never enter normal data paths

Do not expose passwords, tokens, private keys, secret values or credential-bearing URLs through logs, errors, metrics labels, audit records, machine output or committed manifests.

## 6. One authoritative source of truth

Do not maintain the same detailed contract manually in several places.

Prefer schema/OpenAPI/Protobuf/typed definitions where appropriate.

## 7. Mutations follow one lifecycle

```text
plan -> preflight -> apply -> verify
```

Planning and preflight do not mutate. Do not report success before required verification succeeds.

## 8. CLI, JSON and MCP share one semantic core

Presentation differs. Domain behavior does not.

No interface may bypass policy, ownership, reconciliation, verification or evidence.

## 9. Tests prove invariants

Test security, ownership, compatibility, failure and recovery behavior, not only happy-path function output.

Use local/repository validation first. Use GitHub Actions where required by the release gate.

## 10. Keep changes scoped

Do not refactor unrelated code in the same change. Preserve current architecture unless the task demonstrates a missing primitive.

## Documentation

Follow [Documentation style](STYLE_GUIDE.md).

Human docs explain. Reference enumerates. Specs define. ADRs preserve rationale. GitHub Issues plan future work. Releases preserve history.

Every release runs the [pre-release documentation audit](pre-release-documentation-audit.md).
