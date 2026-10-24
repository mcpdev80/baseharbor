---
id: core-923
areas: [acceptance]
type: task
release: v0.4.26
version: ""
parent: v0.4.26
owner: Flash-Coordinator (74e10b98) via lanes f7/f9/q24-o28 - C2/D2/W1 evidence chain
depends_on: [core-921, core-922, core-900]
repos: [mcpdev80/baseharbor]
prs: []
sources: ["mcpdev80/baseharbor#923"]
source_updated: "2026-10-15T16:45:40Z"
labels: [area/evidence]
blocker: ""
resolution: ""
evidence: []
---

- [ ] core-923 · Acceptance: verify New/Edit journeys and ecosystem consumers #task #release/v0-4-26 #status/active #view/active #area/acceptance #kind/task
    <!-- generated:card -->
    **Owner:** Flash-Coordinator (74e10b98) via lanes f7/f9/q24-o28 - C2/D2/W1 evidence chain  
    <!-- /generated:card -->

<!-- generated:links -->
**State:** active  
**Release:** [v0.4.26](../releases/v0.4.26.md)  
**Depends on:** [core-921](core-921-cli-mcp-register-new-interfaces.md), [core-922](core-922-docs-new-edit-provider-selection.md), [core-900](core-900-gc-preserve-active-prs-candidates-evidence.md)  
**Required by:** [t-20261017-release-0-4-26-prepare](t-20261017-release-0-4-26-prepare.md)  
**Owner:** Flash-Coordinator (74e10b98) via lanes f7/f9/q24-o28 - C2/D2/W1 evidence chain  
**Priority:** not set  
**PRs:** none  
**Source:** [baseharbor#923](https://github.com/mcpdev80/baseharbor/issues/923)
<!-- /generated:links -->

## Goal

The integrated v0.4.26 product surface works for developers and existing consumers on the real candidate. Lane: targeted product acceptance.

## Scope

- In scope: existing Demo/consumer fixtures and release-specific gate/evidence inventories.
- Out of scope: product source changes, except a concrete found bug through its own fix PR. Preparation is not approval for a complete pre-release or promotion.

## Acceptance

- Native Docker and rootless Podman: fresh app without managed capabilities, create with real dependencies, edit/add/remove/provider switch, No/Cancel safety and data/shared preservation covered in a targeted way.
- Qualifies the actual current immutable BaseHarbor/Demo/Console/Connector revisions; CLI/JSON/MCP semantics through the real entry point; no old consumer PASS as a substitute.
- Uses existing gates and the core-888 plan/verifier; reuses green gates, also from failed overall runs, only with proven unchanged inputs.
- Isolates found bugs small; no full journey per fix. Final complete necessary coverage and an independent documented developer path.

## Required checks

- First the predecessors' component/contract evidence, then the necessary real consumer/runtime gates. Source/docs are not run serially before every diagnostic run. Coordinate with #897–#901 for candidate/evidence/pilot.

## Dependencies and file ownership

- Depends on core-921 and core-922; candidate/evidence/GC (#897–#900, already accepted) must be complete before.

## Autonomous execution contract — 2026-10-16

- **Owned files:** Existing release-specific consumer fixtures and evidence inventories in the affected product/consumer repositories; record exact paths and immutable candidate refs before executing.
- **Delivered interface/result:** Own candidate-pin resolution, gate execution, evidence validation and independent developer-path report as one acceptance task; no separate operator session is needed to run its checks.
- **Task-specific proof:** Predecessor component/contract evidence, then only required real New/Edit/runtime/consumer gates. Preserve original evidence and input equality rules; no automatic full release matrix.
- **Boundary:** Source bugs get their own explicit fix assignment. Do not start against missing consumer candidates or unsupported executors: detect those at assignment readiness, before long runs. Pre-release/promotion still requires separate authorization.
- **Assignment gate:** use only accepted dependencies and APIs already present on develop. Confirm that the trusted plan/executor runs these exact test families on this task's file set and checks their outcomes. A plan that reports unresolved is not execution-ready. The CI-readiness inventory is maintained by [t-20261016-wizard-task-ci-readiness](t-20261016-wizard-task-ci-readiness.md); later gaps become an explicit prerequisite before assignment, never an instruction to wait for another running session.
- **End-to-end owner:** the assigned session handles integration, implementation, focused tests, hosted CI observation, demonstrated fixes and its planning handoff. It reads other task states itself from planning main and preserves concurrent updates. No shared CI edits, dependency work-in-progress or manual message relay through Marcel. Normal independent review/Maintainer acceptance remains a boundary, not an implementation dependency.

## Handoff

### 2026-10-24 — W1/journeys prerequisite sync, round 3 (live gh, mission o127; develop still f3929568)

- W1 journey execution still cannot start, for current chain reasons rather than missing preparation. Appid test-source chain: #1047's gate-plan qualification is MERGED on develop `f3929568`, while #1046 (now head `1ec67f8d`, BEHIND, PR Selected Checks FAIL in [40086275780](https://github.com/mcpdev80/baseharbor/actions/runs/40086275780) on workflow policy/focused-python/source-contracts SUCCESS, PR Selected Checks FAILURE) still needs its develop-fresh rebase/requalification. Pins-data documentation chain: #1056 (now head `32b85d95`) structurally blocked — run [40088680501](https://github.com/mcpdev80/baseharbor/actions/runs/40088680501) shows pr-planner-tests PASS but PR Selected Checks FAIL `coverage-unresolved: scripts/pr-check-runner.py` (PR · Selected Runtime job FAILURE, o118: linear branch vs core752_reviewed_merge_chain; owner decision pending), and #1050 is ordered behind it. OpenBao gate chain: #1060 (now head `6a0d36da`) green [40086442202](https://github.com/mcpdev80/baseharbor/actions/runs/40086442202) (all mkdocs/documentation-audit/pr-planner-tests PASS) but PR Selected Checks FAILURE in the same run, plus Codex-REVIEW REQUEST_CHANGES (4 findings) with its repair running; #1052 (now head `789299a9`, failing [40084607362](https://github.com/mcpdev80/baseharbor/actions/runs/40084607362)) waits for that corrected landing.
- Consumer freeze chain still mid-lane in dependency order Demo → Console → Connector → #1050 reconcile: demo#121 (head `f57c8b6e`, mergeState CLEAN) in review, console#9 (head `6017de28`, CLEAN) and connector#10 (head `e832d565`, CLEAN) follow; the immutable four-consumer pin matrix (C/D/U/K) is therefore not cut. #1050 (head `38896762`, BEHIND) PR Selected Checks FAIL in [40073196114](https://github.com/mcpdev80/baseharbor/actions/runs/40073196114) (coverage-unresolved consumer-pins.json). Final candidate/native receipts for this card remain gated by these merges. No new journey execution is implied or started by this sync; card stays ACTIVE.

### 2026-10-24 — W1 prerequisite sync from live gh (mission o116)

- The W1/journeys CI path still waits on the open PR chain: #1046 (BEHIND, failing [39995102290](https://github.com/mcpdev80/baseharbor/actions/runs/39995102290)) and #1047 (green [40076426268](https://github.com/mcpdev80/baseharbor/actions/runs/40076426268), awaiting Maintainer-lane approval/merge) gate the appid test sources; #1056/#1050 gate the pins-data documentation path consumed by journey validation. No new native journey execution is implied or started by this sync; card stays ACTIVE.

### 2026-10-21 — Public MCP proof and concrete New/Edit fixture preparation

- Fresh public client bootstrap defect is corrected in candidate `f762f4480a2eb6b29ceebb35ae4032cb2ce7288c`, not yet accepted. Actual [39626844187](https://github.com/mcpdev80/baseharbor/actions/runs/39626844187), job119314164799, PASS71.07s proves omitted revision through public propose, exact returned desired intent through Apply, real SQL add, coreless execution, wrong/stale consent refusal and zero owned residuals. Test/package PASS with zero skip/fail; downloaded artifact12357012289 SHA256 `a1c4270ffa109484a94064614cf4f62cffe493497c33db261fd7082f58c9ed12` independently verified.
- Two further bounded corrections remain unaccepted: terminal reader Name delegation restores single No/Cancel/EOF without source/state changes (reviewed source99901ff2); Edit pre-execution refusals retain truthful unchanged state and original error (sourcec6dfb988, including its directly executed existing Guided regression wrapper). No runtime guard or lifecycle engine is relaxed.
- Demo preparation checkpoint `56661ac9372ce0c5aa296f247e5ebb2f48ce54e2` is local-only and reuses existing guided/data-capabilities/MCP gates, adding actual New/Edit SQL/provider/data/shared journeys and a foreign paused sentinel. Nine helper cases and real daemon-free New/Edit No/Cancel/EOF + MCP/CLI Footprint parity PASS; Docker/Podman executions remain unproven. Cleanup source checkpoints bb9eb892/e6ba5b45 are local-only preparation; fourteen ownership/privacy regressions plus actual trap/error-path checks pass, but final native cleanup acceptance is still open. Sentinel verification must run after actual acceptance teardown and before its separate exact-owner removal.
- No claimed max11 resource limit is established by current catalogs; do not carry that unverified figure as a blocker. Additional workload/SQL/sentinel resources must fit the real selected runner. Immutable four-consumer pins, actual Docker/rootless Podman journeys and final candidate evidence remain outstanding.


### 2026-10-21 — Concrete acceptance gaps under the authorized complete-release audit

- core-921 is accepted, while core-922 final documentation integration remains open; this card stays PLANNED/WAITING until that real prerequisite is accepted. Bounded fixture preparation is proceeding under the existing complete-release audit, not an accepted native journey.
- Existing 65 gates do not yet prove real CLI New/Edit/remove/provider-switch/No/Cancel across Docker and rootless Podman. The successful Core MCP native proof covers a real coreless SQL add and conflict safety, not all these journeys.
- Assigned disjoint preparation: Demo guided PTY/no-capability safety plus heavy data-capabilities real SQL/provider-edit journey; strict MCP tool-list/Edit/Footprint parity; ownership-bounded cleanup and redacted diagnostics with independently owned foreign-resource sentinel. Reuse existing gates and production authorities; no fake registry, relaxed guards, or historical consumer PASS.
- Current consumer SHA freeze, actual targeted native executions and complete candidate evidence are still outstanding. No release acceptance is implied.

The [standard v0.4.26 task rules](../releases/v0.4.26.md#standard-rules-for-v0426-tasks) apply.\n\n> 2026-10-23 progress (coordinator): develop now 49d396c4 (Q+stepcap merged, C2-final pending #1042); consumer PRs #7/#8 validated against C2 line (q41); W1 matrix sheet ready with live consumer_ref tips (o49).\n

### 2026-10-23 — Owner-assigned preparatory consumer merges (ROLLE: MERGE)

Console [#7](https://github.com/mcpdev80/baseharbor-console/pull/7) and Connector [#8](https://github.com/mcpdev80/baseharbor-node-connector/pull/8) were independently reviewed as preparatory source diffs and merged under the bounded cmx2 Owner assignment. Exact source/merge SHAs, live gates, reused source CI and post-merge verification are recorded in the [merge receipt](../evidence/board-2026-10-23/codex-merge-c7n8-post-review.md). Console main is `e7d1e4a1d603158dece19bbc25a8e267d07cde5e`; Connector main is `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e`. Connector post-merge Test/Vet/Build run 39996344778 passed.

This is source preparation only. The immutable final four-consumer matrix, pending Console pin in Connector, actual native/browser receipts and independent developer path remain unaccepted. core-923 stays ACTIVE under its existing Flash-Coordinator owner; no claim, dependency acceptance, card completion, release qualification or production activation changes.
