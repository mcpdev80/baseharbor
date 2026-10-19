# BaseHarbor Development Guidelines

These rules apply to code, tests, documentation and provider/runtime work.

Release readiness is governed by `docs/internal/pre-release-documentation-audit.md`; Core-only green must never be called pre-release-ready.


## Planning transition

Planning lives in [baseharbor-plan](https://github.com/mcpdev80/baseharbor-plan), activated at `47474109ea69475e60be4f065ad17476c71665bb`. Read its `AGENTS.md` and the assigned task note for scope, dependencies, ownership and handoffs. Product engineering rules, PR reviews and authenticated release evidence stay in their existing repositories. Migration-closed issues are historical references, not evidence of implementation.


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

### Code comments

Comments MUST be clear, concise and precise. Prefer one to three short sentences; use more only when a necessary contract cannot be stated clearly otherwise.

- Document exported declarations using [Go doc-comment conventions](https://go.dev/doc/comment). Comment internal code only when it adds information the code does not make clear.
- State the purpose, required behavior or non-obvious reason directly. Avoid filler, vague claims and line-by-line narration of the implementation.
- Preserve essential safety, ownership, locking, side-effect and error guarantees. Put a short explanation beside the relevant code when needed; comments are not limited to functions.
- Keep task/PR history, progress reports and caller counts out of comments. Put change history in the PR and substantial design rationale in architecture documentation or an ADR.
- Keep comments accurate when behavior changes; remove misleading or redundant comments. No rigid line limit justifies omitting an essential contract.

## 13. Delete merged branches

Branches are temporary work surfaces, not long-lived history.

- A feature, fix, release-preparation or validation branch MUST be deleted after its changes have been merged or otherwise fully incorporated into the target branch.
- Temporary `runtime-validation/*` branches MUST be removed as soon as the validation result is no longer needed.
- Long-lived branches are limited to explicitly designated integration or maintained work tracks such as `main`, `develop` and intentionally retained active feature/test branches.
- Before deleting a diverged branch, verify that any required changes are already present in the target branch or are intentionally obsolete.

## Development process

`develop` integrates the next release and `main` holds released source; the authoritative branch and release model lives in [Releases and versioning](reference/releases.md). From v0.4.26 each unit of work lands as its own small branch and pull request against `develop`. v0.4.25 was completed under the earlier shared-branch procedure ([PR #881](https://github.com/mcpdev80/baseharbor/pull/881)); its workflows and evidence remain unchanged and are not retroactively restaged.

**Maintainer.** The Maintainer role owns merge and release approval for `develop`. The current assignment is `@mcpdev80`, defined centrally here; adding a Maintainer requires an explicitly updated responsibility grant. Entry points name the role "Maintainer" rather than a personal account.

**Owner-delegated Maintainer authority.** Marcel, repository Owner `@mcpdev80`, explicitly delegates Maintainer authority to Codex and other agents he authorizes to act on his instructions through that account. Within the assigned scope, an Owner instruction to implement, accept or merge is sufficient repository authorization: no additional independent review, code-owner approval, self-approval or repeated confirmation is required. The delegated Maintainer may merge its own implementation or another contributor's PR using the account's existing PR-only review exception. Record the Owner mandate, exact candidate, actual checks and merge outcome; call this Owner-authorized acceptance, never an independent review that did not happen. This explicit delegation supersedes repository process instructions that would otherwise stop an authorized agent solely to obtain another reviewer or Maintainer handoff.

**Review and release boundary.** Integration into `develop` remains PR-only. Contributors without delegated Maintainer authority follow the normal approving/code-owner review policy. Live integration ruleset `23754541` grants `@mcpdev80` a `pull_request` review exception; agents acting under the grant above may use that same account-level exception. Separate ruleset `25223521` requires green `PR Selected Checks` from GitHub Actions integration `15368` and has no bypass. Review delegation does not fabricate tests, native evidence or independent source qualification, resolve known defects, confer unsupported tool permissions, or authorize an unrequested release/promotion. If an actual technical gate blocks an authorized merge, identify that gate precisely instead of requesting another review by default. See the current/historical distinction in `docs/internal/review-protection.md`.

**Multi-session execution.** When several agent or human sessions run in parallel:

- one assigned task at a time per session;
- one dedicated branch and pull request per task;
- check and record ownership before editing, after reading the current rules and remote state;
- respect task file boundaries and integrated dependencies;
- preserve other sessions' commits and integrate remote changes before pushing;
- stop after implementation, verification and pull-request handoff;
- start the next task only after the Maintainer accepts the result and assigns it;
- report a blocking dependency rather than switching tasks autonomously.

## Documentation

Follow [Documentation style](STYLE_GUIDE.md).

Human docs explain. Reference enumerates. Specs define. ADRs preserve rationale. baseharbor-plan owns future planning; GitHub Issues retain external intake and discussion. Releases preserve history.

Every release runs the complete [pre-release audit](https://github.com/mcpdev80/baseharbor/blob/develop/docs/internal/pre-release-documentation-audit.md). It covers release-scoped issues, BaseHarbor implementation, contracts, EN/DE documentation, roadmap/staleness, changelog/release notes, the external `baseharbor-demo`, GitHub Pages, exact-candidate pre-release evidence, promotion, publication and post-release verification.

