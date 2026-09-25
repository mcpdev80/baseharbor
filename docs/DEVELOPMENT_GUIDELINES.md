# BaseHarbor Development Guidelines

These rules apply to code, tests, documentation and provider/runtime work.

## Project engineering principle

> **AI-generated, human-specified, machine-verified.**  
> **KI-generiert, menschlich spezifiziert, maschinell verifiziert.**

This is a concrete engineering rule:

- **Human-specified:** architecture, intent, constraints, safety boundaries and acceptance criteria are deliberate human decisions.
- **AI-generated:** AI may implement code, tests, documentation and repetitive integration work from those specifications.
- **Machine-verified:** correctness is established by deterministic validation, tests, static checks, runtime verification and release evidence rather than trust in generated output alone.

AI-generated changes do not bypass reviewable contracts, ownership rules, verification or release gates.

## 1. Smallest correct design

Implement only what the current requirement needs. Avoid speculative frameworks, unrelated refactors and product-specific shortcuts.

## 2. Secure and fail closed

Ambiguous ownership, missing authorization, invalid state, unsupported semantics and unverifiable security assumptions are not success.

## 3. Contracts, not products

Applications describe capabilities. Provider/runtime product choices stay behind BaseHarbor boundaries.

## 4. Standards first

BaseHarbor MUST prefer established open standards over proprietary contracts.

- De-facto standards and established ecosystem conventions SHOULD be used where no suitable formal standard exists.
- Baha-specific contracts MUST define only semantics that cannot reasonably be represented by an existing standard.
- Baha extensions MUST be clearly separated, versioned and provider-neutral.
- Provider products MUST NOT pull product-specific contracts into the BaseHarbor core.
- Every new service kind or capability provider MUST document: Existing Standards, Adopted Standards, Deviations, Baha Extensions and Compatibility Impact before implementation.

## 5. Keep provider axes separate

```text
runtime != capability != delivery
```

Do not hide one provider axis inside another.

## 6. Secrets never enter normal data paths

Do not expose passwords, tokens, private keys, secret values or credential-bearing URLs through logs, errors, metrics labels, audit records, machine output or committed manifests.

## 7. One authoritative source of truth

Do not maintain the same detailed contract manually in several places.

Prefer schema/OpenAPI/Protobuf/typed definitions where appropriate.

## 8. Mutations follow one lifecycle

```text
plan -> preflight -> apply -> verify
```

Planning and preflight do not mutate. Do not report success before required verification succeeds.

## 9. CLI, JSON and MCP share one semantic core

Presentation differs. Domain behavior does not.

No interface may bypass policy, ownership, reconciliation, verification or evidence.

## 10. Tests prove invariants

Test security, ownership, compatibility, failure and recovery behavior, not only happy-path function output.

Use local/repository validation first. Use GitHub Actions where required by the release gate.

## 11. Keep changes scoped

Do not refactor unrelated code in the same change. Preserve current architecture unless the task demonstrates a missing primitive.

## 12. Keep source human-readable

Source layout is part of the contributor contract.

- A non-test Go source file MUST stay at or below 800 lines. Split by responsibility before adding more behavior.
- A Go function or method MUST stay at or below 200 lines. Long orchestration should delegate to named lifecycle phases or domain helpers.
- Do not split code only to satisfy a number: file names and helper boundaries MUST describe real responsibilities.
- Tests, generated artifacts, reference documentation and historical release material are not governed by these source-code limits.
- The normal source-quality workflow and Pre-Release gate enforce these limits with `scripts/source-readability-audit.sh`.

## 13. Delete merged branches

Branches are temporary work surfaces, not long-lived history.

- A feature, fix, release-preparation or validation branch MUST be deleted after its changes have been merged or otherwise fully incorporated into the target branch.
- Temporary `runtime-validation/*` branches MUST be removed as soon as the validation result is no longer needed.
- Long-lived branches are limited to explicitly designated integration or maintained work tracks such as `main`, `develop` and intentionally retained active feature/test branches.
- Before deleting a diverged branch, verify that any required changes are already present in the target branch or are intentionally obsolete.

## Documentation

Follow [Documentation style](STYLE_GUIDE.md).

Human docs explain. Reference enumerates. Specs define. ADRs preserve rationale. GitHub Issues plan future work. Releases preserve history.

Every release runs the complete [pre-release audit](internal/pre-release-documentation-audit.md). It covers release-scoped issues, BaseHarbor implementation, contracts, EN/DE documentation, roadmap/staleness, changelog/release notes, the external `baseharbor-demo`, GitHub Pages, exact-candidate pre-release evidence, promotion, publication and post-release verification.
