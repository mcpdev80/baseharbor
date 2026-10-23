---
id: console-2
areas: [console]
type: task
release: v0.4.28
version: ""
parent: console-1
owner: ""
depends_on: [t-20261017-release-0-4-28-updates, t-20261018-r27-access-proof, t-20261018-r27-public-surfaces, t-20261018-r27-team-proof]
repos: [mcpdev80/baseharbor-console]
prs: ["mcpdev80/baseharbor-console#8"]
sources: ["mcpdev80/baseharbor-console#2"]
source_updated: "2026-10-08T13:49:16Z"
labels: []
blocker: ""
resolution: ""
evidence: []
---

- [ ] console-2 · Console: replace fixtures with the protected Core adapter and live workflow proofs #task #release/v0-4-28 #status/planned #view/waiting #board/planned #area/console #kind/task
    <!-- generated:card -->
    **Owner:** unassigned  
    **PR:** [baseharbor-console#8](https://github.com/mcpdev80/baseharbor-console/pull/8)  
    **Waiting for prerequisites**  
    **Needs:** [t-20261017-release-0-4-28-updates](t-20261017-release-0-4-28-updates.md), [t-20261018-r27-access-proof](t-20261018-r27-access-proof.md), [t-20261018-r27-public-surfaces](t-20261018-r27-public-surfaces.md), [t-20261018-r27-team-proof](t-20261018-r27-team-proof.md)  
    <!-- /generated:card -->

<!-- generated:links -->
**State:** waiting  
**Release:** [v0.4.28](../releases/v0.4.28.md)  
**Parent:** [console-1](../epics/console-1-console-product-plan.md)  
**Depends on:** [t-20261017-release-0-4-28-updates](t-20261017-release-0-4-28-updates.md), [t-20261018-r27-access-proof](t-20261018-r27-access-proof.md), [t-20261018-r27-public-surfaces](t-20261018-r27-public-surfaces.md), [t-20261018-r27-team-proof](t-20261018-r27-team-proof.md)  
**Required by:** [t-20261017-release-0-4-28-prepare](t-20261017-release-0-4-28-prepare.md)  
**Owner:** unassigned  
**Priority:** not set  
**PRs:** [baseharbor-console#8](https://github.com/mcpdev80/baseharbor-console/pull/8)  
**Source:** [baseharbor-console#2](https://github.com/mcpdev80/baseharbor-console/issues/2)
<!-- /generated:links -->

## Goal

The Console's live mode reads and mutates only through the protected Core adapter, with live workflow and stream proofs. Remaining scope is assigned to v0.4.28; published historical slices are not automatically requalified for current source.

## Scope

- Baseline at the source's cut: released Core `1aa1a503facc1f4cbbd58fab21619fda53ed6d47`, Console main `0d7c8522ef91f0fc5af47cf6123868f73f543ed4`, Node Connector main `572b81dab1e48ef7df18c4e40b038ba33e196827`. Re-fetch heads at start. Pre-freeze: no legacy compatibility, migration layer or obsolete aliases; native standards first; preserve shared Core authorization, policy, ownership and secret safety. Parent product plan: console-1.
- Actual baseline then: `src/lib/baseharbor/data.ts` exported `fixtureAdapter`; `adapter.ts` read-only; mutation controls contract-pending; `client.ts` allowed plaintext/foreign absolute URLs; `streams.ts` assumed cookie EventSource; UI types were view models differing from Core JSON. Preserve the design system and preview mode, explicitly separate preview from live.
- Add an explicit live adapter over discovery/versioned Core operations/results; map canonical wire objects to UI models using validated decoders/golden fixtures; no type assertions treating wire JSON as proven UI data.
- Bind supported application/repository/workspace/Target/provider/organization/policy/recovery/security operations, safety/approval, plan/diff, async execution/progress/problems and actor attribution. Complete task-first wizards against actual Core schemas; unsupported capabilities stay explicit.
- Wire read-only runtime inventory/ownership/relationships, real metrics/logs/events and capability-gated terminal through Core only.
- Implement the agreed HTTPS/browser auth path, destination pinning, safe redirects and session expiry/reauthentication. Never send credentials to arbitrary discovery links; no Console user/deployment/secret database.
- Honor Core config/policy provenance (#609), denied/required/overrideability states and no secret drafts.
- Preview fixtures only in clearly marked preview/testing; live mode never falls back to fixtures on failure.
- Remove stale "waits for #767–#770" claims; update contract readiness/live support docs; review migration/replacement wording (no pre-freeze migration obligation).
- Component/contract tests plus browser E2E for repository → inspect → init → plan → apply → status/doctor/evidence → destroy; workspaces/Targets/providers and recovery/rotation flows; real submissions, denial/destructive approval, expiry, streams and cleanup.
- **Additional scope (comment 2026-10-08), centrally managed Console for one Core:** one Console binds to one Core, not one Target; after OIDC login it lists and manages all Targets registered with that Core and authorized for the principal (local Docker/Podman, remote via Node Connector, Kubernetes/OpenShift via native API where supported); aggregate and per-Target views distinguishing registered/connected/unavailable/unauthorized; the TUI follows the same distinction. Proposed onboarding `baha console enable` (not shipped in 0.4.23) plus `status`/`open`/`disable`; Console stays optional. Identity via Core-managed Keycloak or supported external OIDC; no Console-local directory/RBAC/secrets; secure one-use expiring first-admin activation; roles such as admin/changer/viewer/auditor with explicit deny tests; Core is authoritative for authorization. Core observations to verify (not assumed fixed): `cmd/baha/machine_navigation.go:collectMachineTargetList()` enumerates `deployment.LoadConfig().TargetNames()` without obvious principal/tenant filtering; `machine_http_executor_read.go` exposes `target.list` and runtime list/inspect/metrics; `runtime_explorer_machine.go:runtimeExplorerForTarget()` behavior for remote vs local. Track the CLI-managed install and Core authorization/target-list fixes as separate linked Core slices.

## Acceptance

- Live UI reads and mutations use Core, with no direct runtime/connector calls.
- Canonical operations, exact result/error/events and policy/actor decisions match CLI/JSON/MCP/HTTP.
- HTTPS/origin/auth negative cases, keyboard/accessibility and secret-safe wizard behavior pass.
- Docker and Podman integration has pinned Core/Console/demo/connector references and original evidence.
- Docs/README/readiness matrix and CI reflect actual supported live functionality.
- Acceptance journeys from the 2026-10-08 comment: (1) fresh READY Core → single `baha` wizard enables Console → HTTPS reachable from another PC → secure first-admin activation → OIDC login; (2) admin/changer/viewer/auditor scope, tenant isolation, deny, logout/expiry/restart; (3) all registered authorized local Targets listed with aggregate and per-Target resource/metrics/logs/terminal; (4) remote Node Connector Target enrolled, visible, connected and inspected only through Core, with disconnect/reconnect/fail-closed evidence; (5) Kubernetes/OpenShift native API Target visible with capability-gated operations; (6) complete app plan/apply/status/doctor/evidence/destroy with confirmation and policy, no fixture fallback; (7) security/rotation, token and certificate renewal, recovery, stable UI link and diagnostics.

## Required checks

- Console contract tests, architecture guard, TypeScript, lint, production build; pinned native Core/browser qualification for live slices.

## Dependencies and file ownership

- Console repository; Core slices for install UX and authorization/target-list fixes are separate linked tasks (to be cut). Coordinate with nc-3 and core-777.

## v0.4.27 consumption prerequisite

The accepted shared management API, two-workstation authority proof and access proof are explicit prerequisites. v0.4.28 owns Console UI and browser consumption of the same state/identity/access/profile/offboarding semantics; no duplicate authority or credential database. No Console implementation scope is pulled into v0.4.27.

## Handoff

- Qualified slices recorded in the source (2026-10-06): read/inspection/admission (run 37502764895), native terminal (run 37522006929, artifact 11440218968), live logs (run 37526991138, artifact 11442304902); receipts keep `production_rotation_evidence: false`, `release_eligible: false`. Original failed runs 37516538050, 37519019518, 37525663970 remain failures. 2026-10-07: public readiness docs aligned; `npm test` is the architecture-boundary entry point. Still open: Console-driven bootstrap, complete application mutation/setup/rotation journeys, production rotation, final repinning to the completed Core candidate and private release-origin verification.
- Open draft PR [baseharbor-console#3](https://github.com/mcpdev80/baseharbor-console/pull/3) (v0.4.23 live integration, last updated 2026-10-08) — stale relative to current Core; Maintainer to decide continue/close.
- 2026-10-23 (f25): f24 recipe implemented as preparatory PR [mcpdev80/baseharbor-console#8](https://github.com/mcpdev80/baseharbor-console/pull/8) against `main`; head `502cdadae3dff0b285868aae85bab618f4c1dede`; PR run 39986030426: "Architecture · TypeScript · Lint · Build" pass, browser proof job skipped-by-design on pull_request (runs on main/schedule/runtime-validation after merge). Files: ci.yml (permanent live workflow proof, source-lock Core resolution, remote docker journey + Connector pin 0a02e6d9, receipt rename), CONTRACT-READINESS.md rows/slices, README stale #767-#770 removal, 1-line artifact-name consistency in release-integration.yml. No merge (codex REVIEW/MERGE lane). Evidence: ecosystem `evidence/board-2026-10-22/f25-console2.md`. Preconditions remain NOT accepted; card in review as prepared candidate.
> Pre-research evidence retained (o64/o66/o68, console-2 PR #8, nc-3 PR pending): v0.4.26 ist noch nicht geschlossen (D2/W1/verify pending) => Release-27-Arbeit formal zurueck auf waiting, Outcome-Referenzen bleiben verlinkt.
> 27-PAUSE (Owner 2026-10-23): Arbeit ruht bis 26 geschlossen; laufende Research-Ergebnisse/PRs bleiben als Artefakte dokumentiert, keine weiteren Runden.
