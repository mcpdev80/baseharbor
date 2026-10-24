---
id: t-20261020-develop-appid-manifest-test-regression
type: bug
areas: [ci, contract]
release: v0.4.26
version: ""
parent: v0.4.26
owner: "Flash (codeman w1-baseharbor)"
priority: high
depends_on: []
repos: [mcpdev80/baseharbor]
prs: ["mcpdev80/baseharbor#1008", "mcpdev80/baseharbor#1029", "mcpdev80/baseharbor#1046", "mcpdev80/baseharbor#1047", "mcpdev80/baseharbor#1050", "mcpdev80/baseharbor#1056"]
sources: []
blocker: ""
resolution: ""
evidence: ["https://github.com/mcpdev80/baseharbor/pull/1029", "https://github.com/mcpdev80/baseharbor/commit/30e112431b04a7e24e7e7e9ea76a3600e439a988", "https://github.com/mcpdev80/baseharbor/actions/runs/39627285799/attempts/2", "https://github.com/mcpdev80/baseharbor/actions/runs/39619782116", "https://github.com/mcpdev80/baseharbor/pull/1046", "https://github.com/mcpdev80/baseharbor/pull/1047", "https://github.com/mcpdev80/baseharbor/actions/runs/40076426268", "https://github.com/mcpdev80/baseharbor/actions/runs/39995102290", "https://github.com/mcpdev80/baseharbor/commit/f39295680d41290fc505c0a8cc1769da51699293"]
---

- [ ] t-20261020-develop-appid-manifest-test-regression · Docker: prove the accepted app.id fixture on the native develop lifecycle #task #release/v0-4-26 #status/review #view/review #area/ci #area/contract #kind/bug
    <!-- generated:card -->
    **Owner:** Flash (codeman w1-baseharbor) · **Priority:** high  
    **PR:** [baseharbor#1008](https://github.com/mcpdev80/baseharbor/pull/1008), [baseharbor#1029](https://github.com/mcpdev80/baseharbor/pull/1029), [baseharbor#1046](https://github.com/mcpdev80/baseharbor/pull/1046), [baseharbor#1047](https://github.com/mcpdev80/baseharbor/pull/1047), [baseharbor#1050](https://github.com/mcpdev80/baseharbor/pull/1050), [baseharbor#1056](https://github.com/mcpdev80/baseharbor/pull/1056)  
    **Evidence:** [proof 1](https://github.com/mcpdev80/baseharbor/pull/1029), [proof 2](https://github.com/mcpdev80/baseharbor/commit/30e112431b04a7e24e7e7e9ea76a3600e439a988), [proof 3](https://github.com/mcpdev80/baseharbor/actions/runs/39627285799/attempts/2), [proof 4](https://github.com/mcpdev80/baseharbor/actions/runs/39619782116), [proof 5](https://github.com/mcpdev80/baseharbor/pull/1046), [proof 6](https://github.com/mcpdev80/baseharbor/pull/1047), [proof 7](https://github.com/mcpdev80/baseharbor/actions/runs/40076426268), [proof 8](https://github.com/mcpdev80/baseharbor/actions/runs/39995102290), [proof 9](https://github.com/mcpdev80/baseharbor/commit/f39295680d41290fc505c0a8cc1769da51699293)  
    <!-- /generated:card -->

<!-- generated:links -->
**State:** review  
**Release:** [v0.4.26](../releases/v0.4.26.md)  
**Depends on:** none  
**Required by:** [t-20261017-release-0-4-26-prepare](t-20261017-release-0-4-26-prepare.md)  
**Owner:** Flash (codeman w1-baseharbor)  
**Priority:** high  
**PRs:** [baseharbor#1008](https://github.com/mcpdev80/baseharbor/pull/1008), [baseharbor#1029](https://github.com/mcpdev80/baseharbor/pull/1029), [baseharbor#1046](https://github.com/mcpdev80/baseharbor/pull/1046), [baseharbor#1047](https://github.com/mcpdev80/baseharbor/pull/1047), [baseharbor#1050](https://github.com/mcpdev80/baseharbor/pull/1050), [baseharbor#1056](https://github.com/mcpdev80/baseharbor/pull/1056)  
**Source:** 
<!-- /generated:links -->

## Goal

`origin/develop` (bb91fbc3) fails `TestRepositoryWorkflowRealLifecycle` because the test fixture writes `app.id` into a developer manifest while `parseAppField` (manifest_parse.go:260) rejects non-canonical `app.id` (contract: app.id is system-owned per #872). This breaks the mandatory docker-runtime check that #998 needs for its final CI.

## Scope

- Update the outdated developer-manifest fixture(s) to the accepted contract (no manual `app.id`; explicit canonical path only where genuinely required).
- Test-side changes only unless the parser itself contradicts the accepted #872 contract.
- Green docker-runtime integration evidence via CI (durable run, not VM one-off).

## Acceptance

1. `TestRepositoryWorkflowRealLifecycle` PASS in GitHub Actions on develop head.
2. No contract semantics change; parser behavior stays fail-closed for non-canonical app.id.
3. Independent local review + evidence links on this card.

## Handoff

### 2026-10-24 — #1047 merged into develop; #1046 awaits its rebase (live gh, mission f50 round 2)

- [#1047](https://github.com/mcpdev80/baseharbor/pull/1047) MERGED 2026-10-24T08:28:06Z as develop squash [`f3929568`](https://github.com/mcpdev80/baseharbor/commit/f39295680d41290fc505c0a8cc1769da51699293) after full-green run [40076426268](https://github.com/mcpdev80/baseharbor/actions/runs/40076426268) on head `40351fd3`. The gate-plan qualification lane for the exact appid env-robust test-only source is now on develop.
- [#1046](https://github.com/mcpdev80/baseharbor/pull/1046) remains OPEN with mergeStateStatus=BEHIND at head `35582508`; its latest PR Selected Checks is still the failing [39995102290](https://github.com/mcpdev80/baseharbor/actions/runs/39995102290), which predates the merge. Next action: rebase onto `f3929568` and rerun the develop-fresh qualification of the six appid test sources.
- Card state remains REVIEW: the explicit final `TestRepositoryWorkflowRealLifecycle` native develop-head acceptance is still unproven and no accepting instance has acted on this card.

### 2026-10-24 — Live gh state of the appid chain: #1047 green and develop-fresh, #1046 still behind

- [#1047](https://github.com/mcpdev80/baseharbor/pull/1047) head `40351fd30af9d1b12a3ea5b4b450056ea714f475` (branch merge of develop a2abc17a into 79e0aa2b, mission f47, fast-forward push, no force): every required check green including PR Selected Checks in [run 40076426268](https://github.com/mcpdev80/baseharbor/actions/runs/40076426268); mergeable=MERGEABLE, mergeStateStatus=BLOCKED solely for the develop ruleset GitHub-approval requirement — merges/approvals belong to the Codex-Maintainer lane, not this board update.
- [#1046](https://github.com/mcpdev80/baseharbor/pull/1046) head `35582508` remains OPEN with mergeStateStatus=BEHIND; its latest PR Selected Checks is FAIL in [run 39995102290](https://github.com/mcpdev80/baseharbor/actions/runs/39995102290). #1047's gate-plan clause binds exactly this reviewed source via the merge-chain check, so #1046 needs its own develop-fresh qualification before either lands.
- Card state remains REVIEW (candidates exist with PRs; nothing here claims acceptance). Chain status synchronized live per mission o116.

### 2026-10-21 — Integrated and REVIEW; final develop-head native qualification pending

- [#1029](https://github.com/mcpdev80/baseharbor/pull/1029) merged as `30e112431b04a7e24e7e7e9ea76a3600e439a988` after all nine selected validations and final verification PASS in [39627285799 attempt2](https://github.com/mcpdev80/baseharbor/actions/runs/39627285799/attempts/2). Independent trusted qualification #1028 is accepted. Exact five source paths are integrated by a proven clean merge; no parser semantics changed.
- Complete native eight-family source c1337192 passed [39619782116](https://github.com/mcpdev80/baseharbor/actions/runs/39619782116) with no skips; independently verified artifact12355416509 digest remains recorded below. Implementation and integration are finished, so this is REVIEW rather than an active source worker.
- The task explicitly requires the named repository lifecycle on the final develop head. Final integrated Core includes additional bounded New/Edit fixes before freeze; execute the necessary final native qualification once on that actual candidate. Do not accept this card or imply complete release readiness from the earlier source anchor alone.
- Original CI attempt1 correctly blocked a persistently nonterminal API cli-vet receipt despite completed steps/logs. The affected job was legitimately rerun; attempt2 has actual authenticated completed/success receipts. No steps/logs inference or final-verifier weakening was used.

### 2026-10-21 — All eight native Docker families PASS; exact-source integration outstanding

- Actual current [39619782116](https://github.com/mcpdev80/baseharbor/actions/runs/39619782116), job119290584209, exact source `c1337192e06e63f8210ace9e5687d8dc8f70f6d2`, proves Repository109.43s, Backend73.78s, WorkloadOnly60.02s, Build90.45s, Multi4.88s, ControlPlaneRecoveryHA255.63s, Loki49.25s and Tempo20.39s. All eight tests and five packages PASS without skips in this one run; no historical composition.
- Verified rootless UID1001, Go1.26.9, exact source/image/binary equality, Doctor, reset, owned cleanup and mandatory JSON verifier PASS. Artifact12355416509, SHA256 `dbec35c8fcc0101e3b9cfae79030061bb8346ffe77832cf93764887e5015d5a4`, was independently downloaded and verified.
- The five-file Docker correction still needs independent trusted qualification and normal integration with current required CI. Task remains ACTIVE for this actual integration and final develop candidate check; the previous failed whole-Docker run is historical, not its current blocker.

### 2026-10-21 — Native repository lifecycle now PASS; integration and remaining Docker fixtures active

- Earlier app.id/parser, canonical-state and stale-plan blockers are resolved. Actual [39615461749](https://github.com/mcpdev80/baseharbor/actions/runs/39615461749), exact b83a53876bcf6f8649519b545e3cbaa7cc47cf74, proves TestRepositoryWorkflowRealLifecycle PASS103.57s and real backend lifecycle PASS65.14s. Parser contract remains unchanged; bounded consent never refreshes stale plans automatically.
- Whole Docker gate remains FAIL because two other workload fixtures still invoke final app destroy --yes without typed retirement consent; recovery/logs/traces skip correctly. Reviewed two-file correction at ba92973ce1a94de7a983eaabef98655781e22dda reuses the existing actual plan-bound retirement helper and verifies zero owned workload containers including unhealthy/stopped residuals. Local Guided/destruction regressions and vet PASS; next coherent native run follows complete current-run evaluation.
- Task is ACTIVE, clearing the obsolete blocked explanation; not accepted until the corrected fixture/source is qualified and integrated into actual develop with current required checks and relevant native evidence. Preserve prior owner/history. This partial native success does not qualify the complete release.

### 2026-10-21 — Apply advanced; precise current fixture and plan-drift failures

- [39600001437](https://github.com/mcpdev80/baseharbor/actions/runs/39600001437) at `772a76730ce5929d76d960949c02688ad38a1a87`: Repository apply succeeds; blanket .baseharbor absence expectation is obsolete because the accepted repository owner maintains its canonical identity copy. Strong Target/deployment/material isolation assertions are being aligned with that actual contract.
- Backend workload retry rejects stale-impact-consent; the bounded helper correctly refuses automatic stale refresh. The actual first/second plan difference must be diagnosed, not waived. Workload-only/build/multi-instance pass again; recovery/logs/traces skipped; artifact `12346072529`. This task remains blocked for its real native acceptance.


### 2026-10-21 — Hosted runtime established; current blocker is real fixture consent

- Actual [39597320139](https://github.com/mcpdev80/baseharbor/actions/runs/39597320139), exact source `e21d69549444c3cb6c724d1663ce1f28bfa03a4f`: verified hosted rootless Docker, Go 1.26.9, candidate image/binary identity, reset and Doctor PASS. Workload-only, build/rebuild and multi-instance native tests PASS.
- Required TestRepositoryWorkflowRealLifecycle and ComposeUsesBackends FAIL with impact-confirmation-required. Recovery/logs/traces did not execute. Artifact `12345350446`. The earlier persistent-host prerequisite is a separate operational issue, not this current hosted acceptance blocker. Bounded fixture consent/retained-owner correction is in progress; no broad guard bypass or inferred complete PASS.


### 2026-10-21 — Persistent-runner prerequisite isolated; hosted native candidate prepared

- [39591903347](https://github.com/mcpdev80/baseharbor/actions/runs/39591903347) confirmed UID 1001 has no selected rootless socket; runtime tests and reset skipped. Bounded user-session recovery was then tested in [39594370883](https://github.com/mcpdev80/baseharbor/actions/runs/39594370883) at `173a69575cbfc673f6c3a0dd505c99247269e780`; private runtime-directory guard failed before daemon start or reset. No fallback or inferred native success.
- Prepared disposable hosted rootless Docker validation reuses the existing setup helper, preserves exact Go 1.26.9 candidate binary through resets and verifies the same binary inside a local SHA-bound runtime image. Eight native Test+Package PASS receipts with no skips are mandatory. Local executable regressions passed; actual hosted execution remains pending.


### 2026-10-21 — Corrected current blocker after live runtime lookup

- #1008 and #1009 are merged; their former policy-lane blocker is resolved and must not remain the current board explanation.
- Triggered [Docker runtime 39584767281](https://github.com/mcpdev80/baseharbor/actions/runs/39584767281) at `f6b50f8757e97b126ca380ee9402a68fd5150d2b` actually FAILED: selected rootless `unix:///run/user/1001/docker.sock` unavailable. Runtime integration, recovery/log/trace steps were skipped. Explicit native `TestRepositoryWorkflowRealLifecycle` acceptance is therefore still missing.
- A separate JSON-Doctor/preflight correction is prepared; no implicit system-daemon fallback or global cleanup on failed preflight. Task stays blocked for genuine native evidence, not for the already fixed planner policy.


> Coordinator verification 2026-10-23 (sharpened, bounded): `TestExternalProviderRegistrationAndRemoval` fails on develop 1861ed07, 9f4916be, 2adae47a AND origin/main with `provider-registry-ownership-unverifiable`. Failure is reproducible only when the ownership verifier cannot establish file ownership (unverifiable UID on the checkout path or an unwritable/unowned temp root), NOT on a normal owned checkout. Reproduced by o53 on detached develop 1861ed07 (local, no daemon) and by the reviewwave live checks; source `internal/application/external_provider_scope.go:184-205` verifies ancestor directories by Lstat UID/permissions with no CI-runner exemption, so pass/fail tracks filesystem ownership+permissions, not CI-vs-local. This is an environment-dependent test, not a C2 regression and separate from the still-required final `TestRepositoryWorkflowRealLifecycle` native lifecycle.

> NOTE: #1047 (mcpdev80/baseharbor, head 79e0aa2b…, OPEN, base develop) qualifies the exact #1046 test-only source with a narrow planner lane and does not change the parser, ownership guard or the required native lifecycle. #1047 bleibt SKIP (mergeStateStatus=BLOCKED durch Self-approve-Regel, Maintainer nicht lifted); Status ohne Owner-Entscheidung unveraendert.

Product fix landed and pushed: branch `fix/t-20261020-develop-appid-manifest`
(HEAD `0e213d42`), draft PR #1008 (base `develop`):
https://github.com/mcpdev80/baseharbor/pull/1008

- Fixture diff: removed authored `app.id` from the 3 workload test fixtures in
  `cmd/baha/app_workload_integration_test.go` (lines 40/178/311); parser
  unchanged, stays fail-closed. `app apply`/`app init` generate the canonical
  identity.
- Added manifest regression tests (`internal/application/manifest_test.go`)
  proving both contract directions.
- Local: `go vet` + full `cmd/baha` (all pass except the pre-existing
  `TestAppEditCommandCompleteProviderEditRunsRealLifecycle` compose/consent
  timeout, reproduced on clean develop) + `internal/application` pass. The
  compose/build workload integration tests need rootless docker (local socket is
  rootful; `DOCKER_MODE` override was removed in `d2bca4ea`); the underlying
  `app.id` error is proven by the parser unit check.

CI blocked (run `39430265422`, trusted plan `pr-plan.json`):
`coverage_status: unresolved`, `selected_gates: []`. All 4 static validations
(cli-build, cli-guided-tests, cli-vet, source-contracts) pass; `PR Selected
Checks` fails closed. Root cause is a pre-existing policy gap, not the product
fix: `scripts/pr-gate-plan.py` classifies every `cmd/baha/*.go` and
`internal/application/*.go` change (including test files) as needing a qualified
native runtime gate but emits an empty gate list, so `runtime_required` stays
false and the docker-runtime acceptance job never runs. `scripts/pr-required-final.py:24`
fails closed on `coverage_status != 'resolved'`.

Action for Marcel / Codex 6.1: add a lane in `pr-gate-plan.py` for audited
test-only fixture changes mapped to the static package validations already
emitted (`go test ./cmd/baha`, `go test ./internal/application/... ./contracts/...`,
`go vet`, `go build`), so the coverage gate resolves without an atomic runtime
matrix. Evidence: `evidence/board-2026-10-20/o1-appid-develop-fix.md`.

## Notes

Local evidence: `evidence/board-2026-10-19/p22-nativejob.md`. Found by p22 split triage on 2026-10-20; pre-existing on develop, surfaced via #998's mandatory CI.

### 2026-10-20 - lane status

- Fix delivered as draft PR #1008 (test-only, parser untouched). CI shows the coverage-unresolved bootstrap contradiction this card surfaced; o1 investigating legitimate green path vs documented wait-on-#1007.

### 2026-10-21 — CI policy correction in review

- Product fixture remains in draft PR [#1008](https://github.com/mcpdev80/baseharbor/pull/1008), source `0e213d42c74cfdb91964592c17e3914a848207b9`.
- Narrow CI policy correction is now published as draft [#1009](https://github.com/mcpdev80/baseharbor/pull/1009), source `a0dcacf93c9a6922163f2718f374f05e1e5e9e3e`.
- Exact two-file/status match, reviewed-source merge-chain proof and hosted validations; no generic `_test.go` exemption or native runtime bypass. Regression tests added for missing/extra/deleted/unreviewed/stale inputs.
- GitHub Actions run [39582522944](https://github.com/mcpdev80/baseharbor/actions/runs/39582522944) started; result pending when this handoff was written. Do not mark accepted or unblock core-921 until valid current CI and final Docker lifecycle evidence exist.
- Release v0.4.26 remains **active, not pre-release-ready**; next: qualify #1009, integrate policy, requalify #1008, run native Docker proof, then finish core-921, Podman recovery, core-922 and core-923.

### 2026-10-21 — CI policy accepted and merged

- [#1009](https://github.com/mcpdev80/baseharbor/pull/1009) merged into `develop` as `b7f920ab6b7890a1a9ca7fdf5b0631c172b13d34` after successful GitHub Actions [39582522944](https://github.com/mcpdev80/baseharbor/actions/runs/39582522944): Trusted Plan, workflow policy, planner tests, CI policy, focused Python, executor/verifier tests and PR Selected Checks all successful.
- #1008 product fixture remains open and needs fresh CI against updated trusted `develop`; native Docker acceptance and final release proof remain pending. Do not mark this task done based only on the policy merge.

### 2026-10-21 — develop integration refreshed

- CI policy fix #1009 is merged into `develop` at `b7f920ab6b7890a1a9ca7fdf5b0631c172b13d34`.
- #1008 branch `fix/t-20261020-develop-appid-manifest` was advanced from `0e213d42c74cfdb91964592c17e3914a848207b9` to merge commit `652891a3f2b98c1f4a242ce3d569c0ed8559d912` (parents: previous PR head and current develop); two test files preserved, all other paths inherited from develop, no force push.
- GitHub reports PR #1008 mergeable; new head CI evidence not yet available at handoff. Keep task blocked until required hosted checks and docker-runtime acceptance prove the stated acceptance criteria. Previous run `39430265422` is stale and must not be counted as current evidence.

### 2026-10-21 — #1008 merged, release runtime acceptance outstanding

- Current #1008 source `652891a3f2b98c1f4a242ce3d569c0ed8559d912` passed GitHub Actions [39583753055](https://github.com/mcpdev80/baseharbor/actions/runs/39583753055): Trusted Plan, workflow policy, planner tests, CLI guided/build/vet, source contracts, focused Python, CI executor/verifier and PR Selected Checks all green.
- #1008 merged into develop via squash commit `8a40e60a84ebbe2c7eb3ceb80e2788c5f05903b8`.
- The PR's targeted Docker runtime job was **skipped**. The task's explicit `TestRepositoryWorkflowRealLifecycle` on develop-head Docker Actions criterion remains unproven. Do not mark full release acceptance complete until separate docker-runtime-acceptance workflow evidence exists.

### 2026-10-21 — Docker runtime acceptance trigger

- Created `runtime-validation/docker/appid-20261021` from accepted develop `8a40e60a84ebbe2c7eb3ceb80e2788c5f05903b8`, then pushed no-content-change commit `f6b50f8757e97b126ca380ee9402a68fd5150d2b` to trigger `.github/workflows/docker-runtime-acceptance.yml` through its explicit `runtime-validation/docker/**` push filter.
- Runtime evidence pending; never treat branch creation or the trigger commit as test success. This runs the workflow's real Docker workload, recovery, logs and traces checks. Do not merge a runtime-validation branch into develop.

> Coordinator verification 2026-10-23: TestExternalProviderRegistrationAndRemoval FAILs LOCALLY on develop 1861ed07, 9f4916be, 2adae47a AND origin/main (ownership-unverifiable) while CI is green -> environment-dependent test, not a C2 regression. READY accepted WITH this caveat; defect card created.

### 2026-10-23 — appid self-approve-block: owner decision skipped, card stays as-is

- PR #1046 `test(application): env-robust protected-owner test dirs (no skip-if, deterministic 0700)` and PR #1047 `ci(gate-plan): qualify exact appid env-robust test-only source (#1046)` are both OPEN on base develop with `mergeStateStatus=BLOCKED` by the self-approve rule (live `gh pr view` 2026-10-23). #1047 head is `79e0aa2b`.
- Owner-decision (assign / re-touch / escalate) is EXPLICITLY SKIPPED here because the merge path is self-approve-blocked and no Maintainer has lifted it. This note exists so no subsequent session re-opens the ownership question for #1046/#1047; it records a state, it does not accept the release or the card.
- The still-required final `TestRepositoryWorkflowRealLifecycle` native lifecycle on develop head remains the card's real outstanding criterion (see Goal/Acceptance). This does not close it.
