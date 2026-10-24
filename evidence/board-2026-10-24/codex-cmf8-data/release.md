---
id: v0.4.26
kind: release
title: "App wizard, capability editing and development/CI V1"
status: active
order: 10
sources: ["mcpdev80/baseharbor#873"]
evidence: []
---
# v0.4.26 · App wizard, capability editing and development/CI V1

Source: [baseharbor#873](https://github.com/mcpdev80/baseharbor/issues/873) (release umbrella). The workflow/CI part is coordinated by epic [core-888](../epics/core-888-development-and-ci.md). Task list: [views/releases/v0.4.26.md](../views/releases/v0.4.26.md).

## Goal

Make the v0.4.25 capability-first semantics easy to create and change, without new domain or provider contracts. In parallel, switch development to small independent task PRs, conservative targeted CI and separately frozen release candidates (epic core-888).

## Binding scope corrections

- **Foundation (2026-10-11):** v0.4.25 (#872) is the corrected foundation: BaseHarbor-owned IDs, automatic or explicit provider realization, no fixed Core bundle, and complete ownership-safe add/remove/re-add convergence. Those were v0.4.25 acceptance requirements, not deferred v0.4.26 work. This release adds the developer interaction surface only. It must not reintroduce manual IDs, a mandatory bundle, legacy migration or a second resolver.
- **Needs-first wizard (2026-10-11):** the wizard asks what the application needs, not which infrastructure product to install. No default-flow questions such as "Do you need OpenBao?" or "Install Keycloak?". Use a compact, editable needs checklist in plain developer language, for example: securely managed secrets/passwords, TLS/HTTPS, users signing in to the application, persistent relational data, a cache. These examples illustrate the wording; they are neither an exhaustive catalog nor a mandatory sequential questionnaire. Further needs are disclosed progressively.
- Application user authentication, BaseHarbor operator/team access and provider service accounts are distinct; "users" must not silently enable all three. TLS must not implicitly request Identity, OpenBao or a full Core; managed secrets do not implicitly request PKI or dynamic credentials. Genuine dependencies come from the accepted #872 resolver and are explained in the preview.
- **Zero selected needs is valid.**
- **Optional provider preferences in the same wizard (2026-10-10):** the developer authors a minimal contract (`version: 1`, `app.name`, `app.environment`) and is never asked for, nor allowed to supply, an application UUID. Each selected need can be expanded in place: `Automatic (recommended)` is the default; an explicit choice lists the actually compatible providers/instances of the active Target with the proposed realization and reason. No mandatory provider questionnaire or advanced page.
- **One flow, two existing authorities:** capability intent goes only to the Application Contract; an optional explicit provider preference goes only to the existing Target/deployment/operator binding owner. No new configuration file; never write `provider: postgresql` into portable application intent.
- **Consequences and consent (2026-10-11):** consume #872's universal typed consequence plan for all supported providers: affected apps, functions and dependencies, before/after realization and access, retained/transferred/deleted data and actual locations, operational/security consequences, prerequisites and blockers. Show actual final dispositions after execution and after partial failure. Stop, removal, replacement and capability disable have distinct effects. No generic warning list, secret disclosure, invented destination or UI-local impact engine. Provider removal shows affected apps, lost secret-management/PKI/login functions, real certificate/token renewal consequences, secure continuation or replacement, retained data and blocked prerequisites; no generic confirmation and no silent unauthenticated/plaintext fallback.
- Preview: selected provider (automatic/explicit), reused vs. new managed dependencies/containers, incremental RAM with a measured/estimated/unknown label. **No** returns to the checklist with both levels of selection retained, indefinitely; **Yes** uses the existing diff/preflight/confirm/apply/verify path; cancel, EOF, non-TTY and `--no-input` never write silently.
- Editing hydrates current capability selections and resolved/explicit provider preferences without overriding another Target's choice. CLI/JSON/MCP and Console reuse #872 resolution semantics; explicit provider preference is discoverable to machine clients with typed errors and equivalent authorization/confirmation.
- Reuse supported rich capability manifests and binding owners: no parser rewrite, parallel state registry, migration project or unconsented data destruction. Obsolete developer-authored IDs are not compatibility scope.

## Product surfaces

- `baha app new`: compact single-screen multi-select; detected preselection visible and editable.
- `baha app edit`: exact intent diff, effects and explicit confirmation; manual canonical/shorthand edits use the same reconcile.
- `baha app footprint show`; plan, CLI, JSON and MCP share the v0.4.25 resolver with typed machine confirmation/denial and effective evidence.
- Console consumes the same domain operations where applicable (no second resolver); full Console management journeys stay in v0.4.28.
- EN/DE documentation, discovery and automated CLI/JSON/MCP parity.

## Acceptance

- Checklists, back-loop and preview verified with real and unknown capacity, reuse and zero-core.
- Changes at any point show diffs, are idempotent and preserve data and shared services.
- Non-interactive and EOF safety, no password/credential leakage, CLI/JSON/MCP parity.
- Independent developer path and exact-SHA runtime evidence.
- No new domain semantics without explicit v0.4.25 contract review.
- No app ID prompt; minimal authored YAML.
- A SQL need can stay Automatic or select a compatible available provider in one wizard.
- The same app intent deploys against two Targets with different explicit provider instances without altering portable YAML.
- The back-loop retains both levels of selection; no hidden writes.
- Missing, ambiguous or incompatible providers are handled safely with a concrete next action.
- A developer can create/edit an app with protected secrets, TLS and/or user login without knowing any provider product names.
- User login, operator access and service credentials are not conflated; TLS/secrets do not silently enable unrelated capabilities.
- The preview explains the actual selected providers and genuine dependencies; explicit preference stays accessible in place.

Final semantic acceptance remains with v0.4.31 (#174). Qualify the actual integration/candidate SHA.

## Ordering (no shared development branch)

### Assignment-readiness correction — 2026-10-16

The qualified foundation and core-910 are accepted, including the Maintainer-authorized deferred tests in core-895. This does not yet qualify CI for newly named wizard files. First finish [t-20261016-wizard-task-ci-readiness](../tasks/t-20261016-wizard-task-ci-readiness.md): trusted file/profile coverage, executor and exact component-test enforcement as one result. core-894 follows this CI writer rather than editing the same workflow concurrently.

Parallel work available immediately: the CI-readiness task, core-901's analysis of existing pilot/cost evidence (its output uses the existing documentation CI lane), and cross-release maintenance core-909 with local gh/mirror access. core-901 does not depend on core-894 display cosmetics; core-909 has no product-task dependency. These are separate outcomes and write sets, not early starts on unfinished wizard interfaces.

After this prerequisite is accepted, core-911 (hydration), core-912 (selection), core-914 (footprint presentation) and core-915 (consequence presentation) can run independently on the merged baseline with disjoint files. Assign up to the actually available ChatGPT sessions; Qwen is reserved for tasks that benefit from local authenticated gh/checkout capabilities. Do not assign all waiting cards at once.

Later waves consume accepted APIs: core-913 after core-912; core-916 after selection/provider/presenters; core-917 and core-918 after the controller (Edit also needs hydration); core-919 after footprint presentation; core-920 after its read adapters; core-921 integrates registrations once handlers are accepted; core-922 then documents and core-923 verifies the integrated surface. The existing dependency graph is authoritative, not session-to-session message passing. Every task includes its full implementation/verification/handoff scope. CI/native/consumer readiness is checked before assignment; a missing lane is an explicit prerequisite, not a reason to make Marcel coordinate a second implementation owner mid-task.

1. Qualify the PR/review/runner foundation (#890–#893, #895, #896) and the product interface core-910. Small separate PRs; no new resolver or state architecture. **Partial CI is not product-start clearance.** The CI order is fixed by the 2026-10-15 correction in [core-888](../epics/core-888-development-and-ci.md).
2. Then core-911, core-912, core-914 and core-915 in parallel (different files). core-913 after core-912; core-919 after core-914. Pilot bug core-752 runs in its own lane.
3. core-916 connects the finished UI components. Then core-917 (new) and core-918 (edit) in parallel; core-920 follows its read interfaces independently.
4. **core-921 is the single integration task** for the command tree, help, machine/MCP registry, discovery/coverage goldens and generated references. Predecessors deliver compiling handlers/adapters and do not edit these shared files.
5. core-922 writes the public EN/DE guide once the surface is stable. core-923 delivers targeted real product/consumer acceptance; candidate/evidence/GC work (#897–#900) must be complete before it. core-901 owns the cost/parallelism comparison.

## Standard rules for v0.4.26 tasks

These apply to every task of this release and epic core-888 and are not repeated in each note.

- One small branch and PR against `develop` per task; split further when an independently reviewable part emerges. Record the owner before editing; read current rules and remote state first; never overwrite other sessions' commits.
- Develop against the actually integrated interfaces of dependencies. Do not edit other tasks' shared files on the side; land a needed interface delta first as a small agreed PR. Integrate before pushing or after base changes and re-plan affected checks. No permanent file locks.
- Re-read current rules, SHAs and the existing implementation before implementing; the cut was made against develop `33b7ca94b42cc45e908c0d964cfbe0cc8349f231`. Preserve all #873 requirements, reuse the #872 resolver/lifecycle and add no new domain or state authority. Keep relevant EN/DE terms consistent.
- No complete pre-release, merge, tag or publication without separate authorization.
- The v0.4.25 procedure (#872/#881) and its workflows and evidence stay unchanged. (The precondition "start only after qualified v0.4.25 completion" is satisfied: #872 closed as completed on 2026-10-15.)

## Recurring release operations

The [four release completion cards](../RELEASE_WORKFLOW.md) apply to this already-running release: preparation, pre-release approval, publication/tagging and post-release verification. No retroactive cleanup or Renovate-start cards apply to v0.4.26. Their task links and actual states are in the generated release view. Card creation is planning, not readiness, acceptance or authorization.
