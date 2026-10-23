# F9 — POST-C2 RERUN DISPATCH TABLE (READ-ONLY prep)

Mission: `prompts/f9-rerun-prep.txt` (delegation-inbox line RERUN-PREP). Assemble the complete
post-C2 native rerun dispatch table from `o21-matrix-inputs.md` (vehicles+inputs) +
`q24-cmda-triage.md` (consumer_ref correction) + `f7-native-dispatch.md` (f7 gate set) +
`o24-matrix-gonogo.md` (GO buckets). READ-ONLY lane: sole write = this file; no dispatches issued.
Placeholders: `{C2}` = rekeyed baseharbor develop SHA (C + #1038/#1040 lineage), `{D2}` = demo ref
at that freeze point, per o21 §0 rekey rule ("guided-fix changes inputs/C → re-derive").

## PLAN (todowrite mirror)
- [x] 1. Turn-start basics-check + FLAGS; process events (permission dialog lane q25) + ack
- [x] 2. Read o21 + q24 + f7 + o24 (inputs)
- [x] 3. Reality check (read-only gh): #1038/#1040/#7/#8 state, develop HEAD, connector work-branch tip
- [x] 4. Derive rerunnable row set = f7 9-row gate set + 56 CMD-A gap rows; apply rekey/reuse rule
- [x] 5. Emit per-row table: repo | workflow | event+inputs (runtime, gate, ref={C2}, demo_ref={D2}, corrected consumer_ref) | concurrency group | flip rationale
- [x] 6. Batch grouping for parallel dispatch (no serial waits) + concurrency notes (F-1/F-2)
- [x] 7. ## repo-rule compliance + END F9-PREP-DONE

## 1) Reality check @ prep time (read-only, 2026-10-22)
- baseharbor `develop` HEAD = `2adae47afddc299d06dbc34eb16a8e011c14b6dc` = C → **C2 not yet produced**.
- PR #1038 head `c37933d9688615ba574cfc3b9ffc817bd03c4f3e` **OPEN**; PR #1040 head `b5ae3684e74f300ff9026e7c7626be2415cb8f68` **OPEN** (f7 T1: #1038's Selected Checks resolves only once #1040 merges → merge order #1040 → #1038).
- Console PR #7 `0ea88f553fdac2097e26dce7a5203233a5530527` **OPEN**; Connector PR #8 `570864c7c8bec738438aa70aa967be7e8490d7c3` **OPEN**.
- Connector `work/v0.4.26-integrations-docs` tip = `8e6bb897f7dcb9ee77f0c40a4600980439e2eac6` (40-hex ✓) BUT its `integration-candidate.json` still carries `core=d5417bde…` (stale) + `demo/console="PENDING-FREEZE"` → q24 Option B (auto-resolve) reaches a *valid SHA shape* but **invalid content** until P0-3 below is done.
- demo `baseharbor-core.ref` @ D = C ✓ (will require new demo commit → {D2} on rekey).
- Demo PR #116 (q15 cleanup-accounting fix) MERGED 17:57Z into D — f7's podman cleanup-provenance failure occurred ON tree(D) ⇒ that lineage is **not fixed by C2** (AT-RISK rows below).

## 2) consumer_ref correction (q24, binding for every row using pre-release.yml)
- INVALID: `570864c7` (8-char prefix) — `ecosystem_release_pins.py:99,117` requires 40-hex or empty.
- VALID: `consumer_ref = ""` (auto-resolve to tip of `work/v0.4.26-integrations-docs`) OR the **full 40-char tip SHA at dispatch time** (`{N40}`; currently `8e6bb897…` but advances with the P0-3 candidate-content commit → re-read `git ls-remote` immediately before dispatch).

## 3) Preconditions (P0 — NOT dispatches; complete before any W1 row is expected green)
- **P0-1** Merge #1040 then #1038 into develop ⇒ new develop SHA = **{C2}** (placeholder rule per o21 §0).
- **P0-2** Freeze rekey: push demo commit `baseharbor-core.ref = {C2}` ⇒ **{D2}**; update `docs/releases/v0.4.26.consumer-pins.json` with U (pins resolution `pre-release.yml:133-137,304`).
- **P0-3** Connector branch candidate update (q24 §7b.2): `integration-candidate.json` on `work/v0.4.26-integrations-docs` = `{core: {C2}, demo: {D2}, console: {U40}}` all 40-hex; console candidate commit per q24 §7b.3 (`release-integration/v0.4.26/core/<SHA>`).
- **P0-4** Merge U (#7) + N (#8) — required by rows 46-53 (HA pins), 59 (live-console), 60/63 (remote-target).
- **Gate-reuse rule**: C2 ≠ C ⇒ **every old-C receipt is digest-void** (o17 §3d reuse discipline) — including f7's GREEN mcp (39831982468) and backup-docker job; ALL 65 rows rerun. No row keeps an old receipt.

## 4) Vehicle design (parallel-safe; OWNER DIRECTIVE 2026-10-22 honored)
- **W1 = ONE dispatch**: `pre-release.yml` `workflow_dispatch`, `--ref develop`, inputs `tag=v0.4.26`, `ref={C2}`, `demo_ref={D2}`, `consumer_ref={N40|""}`, `only_gates=""` (CMD-B shape ⇒ complete run ⇒ authorizing + emits `release-approved.json`/coverage provenance; `RELEASE_WORKFLOW.md:46`). All 65 rows execute inside this single run: docker∥podman matrices (`:537-626`, MP2), static/adoption/HA/integration jobs in parallel per `needs` graph. Occupies the shared `baseharbor-release-qualification` slot exactly once (F-1); no sibling run can queue-evict it (f7 §6a hazard avoided by not double-dispatching).
- **W2 = diagnostic reruns** (per-gate `targeted-demo-acceptance.yml`, explicit `runtime`+`gate`) ONLY for W1 rows that fail; dispatched ONE AT A TIME with terminal+authentication before next — this serialization is forced by the same-group cancel hazard (f7 §6a), documented per rule, not a silent docker/podman serialization. Docker+podman diagnostics of the same class are still cross-pollable (demo `v0424-targeted.yml` / per-gate groups if the recommended group split lands).
- CMD-A (partial gap-fill) is SUPERSEDED post-C2: with rekey voiding all receipts, the union of CMD-A ∪ f7-rerun set = all 65 = W1 itself. CMD-C (adoption vehicle) unused — adoption ⊂ W1.
- Wall: W1 ≈ 5-8h (o21 §4 CMD-B estimate); runner fit: hosted `ubuntu-latest`/`ubuntu-26.04`, no self-hosted path on this sheet.

## 5) RERUN DISPATCH TABLE — all 65 rows (per-row explicit)
Common columns apply to every row: **repo** `mcpdev80/baseharbor` · **workflow** `.github/workflows/pre-release.yml` · **event** `workflow_dispatch --ref develop` · **inputs** `tag=v0.4.26, ref={C2}, demo_ref={D2}, consumer_ref={N40|""} (per §2), only_gates=""` (single W1 dispatch covers all rows; rows listed for coverage+rationale audit) · **concurrency group** `baseharbor-release-qualification` (`pre-release.yml:41-43`, queue, cancel:false) · **batch** W1.

Legend FLIP: ✓#1038 = flips via merged edit-preview fix in C2; ✓P0 = flips once P0 source-job blockers (§2/P0-3) fixed (q24 cascade/first-execution); ⚠ = AT-RISK, C2 alone not proven sufficient.

| # | Gate | route job (pre-release.yml @ C) | runtime | FLIP | Rationale (expected-green / residual risk) |
|---|---|---|---|---|---|
| 1 | atomic/static/config-matrix | static_gates `:520` | static | ✓P0 | never executed (q24 run: skipped behind source-job fail); static class, no C2-lineage defect |
| 2 | atomic/static/init | static_gates `:520` | static | ✓P0 | as row 1 |
| 3 | atomic/static/mcp | static_gates `:520` | static | ✓P0 | f7 GREEN 39831982468 was @C → digest-void at C2 → rerun; no known C2 regression |
| 4 | atomic/static/agent | static_gates `:520` | static | ✓P0 | as row 1 |
| 5 | atomic/static/shell-ux | static_gates `:520` | static | ✓P0 | as row 1 |
| 6 | atomic/docker/guided | runtime_light_gates_docker `:537` | docker | ✓#1038 | f7 RED 39832388802 class `new-edit-new-zero-no` missing preview line = exactly the `app_edit_apply_result.go:338-340` defect removed in #1038 |
| 7-12 | atomic/docker/{lifecycle,policy,connectivity,reconciliation,failure,full-destroy} | runtime_light_gates_docker `:537` | docker | ✓P0 | f7-owned? no — never executed (skipped); first real C2 execution |
| 13 | atomic/docker/data-capabilities | runtime_heavy_gates_docker `:591` | docker | ✓#1038 | f7 RED 39834803429: 5 capability rows PASS, fail = same preview-assert class → fixed in C2 |
| 14-16 | atomic/docker/{observability,identity,security} | runtime_heavy_gates_docker `:591` | docker | ✓P0 | never executed on C2 lineage; no f7 RED |
| 17 | atomic/docker/backup-restore | runtime_heavy_gates_docker `:591` | docker | ✓P0 | f7 demo-push docker job SUCCESS @tree(D); route now native heavy matrix; C2 core change neutral (fixture path unchanged) |
| 18-23 | atomic/docker/{rabbitmq,mongodb}-runtime, durable-key-value, control-plane-recovery, loki-runtime, tempo-runtime | runtime_heavy_gates_docker `:591` | docker | ✓P0 | never executed; first real run |
| 24 | atomic/podman/guided | runtime_light_gates_podman `:555` | podman | ✓#1038 | f7 RED 39833309541 identical class → fixed in C2 (podman cleanup was already complete) |
| 25-30 | atomic/podman/{lifecycle,policy,connectivity,reconciliation,failure,full-destroy} | runtime_light_gates_podman `:555` | podman | ✓P0 | never executed; first real run |
| 31 | atomic/podman/data-capabilities | runtime_heavy_gates_podman `:609` | podman | ✓#1038 (watch cap) | same sql-no preview class fixed; residual: f7 hit 8-min `Run runtime gate` step cap AFTER the assertion — if fix removes the cancel-loop the run should fit, else step-cap regression (monitor) |
| 32-34 | atomic/podman/{observability,identity,security} | runtime_heavy_gates_podman `:609` | podman | ✓P0 | never executed |
| 35 | atomic/podman/backup-restore | runtime_heavy_gates_podman `:609` | podman | ⚠ | f7 GATE PASS / CLEANUP-PROVENANCE FAIL (`destroy=0 preparation=1`) occurred WITH q15 fix #116 already merged into D ⇒ q15-lineage owner must land a further fix; C2 does not address it |
| 36-41 | atomic/podman/{rabbitmq,mongodb}-runtime, durable-key-value, control-plane-recovery, loki-runtime, tempo-runtime | runtime_heavy_gates_podman `:609` | podman | ✓P0 | never executed |
| 42-45 | adoption/{conformance,realworld-compose,realworld-quadlet,realworld-kubernetes} | adoption `:427` | ubuntu-latest | ✓P0 | f7 RED-adjacent? no — never executed (skipped); included in W1 ⇒ F-2 one-at-a-time constraint moot (single run) |
| 46-49 | ha/docker/{data,identity,observability,routing} | v0421_ha_groups_docker `:645` | docker | ✓P0 + needs P0-4 | native route (legacy `work/` push dead, o21 §1-3); HA gates consume pins ⇒ require U merged + pins file updated |
| 50-53 | ha/podman/{data,identity,observability,routing} | v0421_ha_groups_podman `:740` | podman | ✓P0 + needs P0-4 | as 46-49, ubuntu-26.04 |
| 54 | journey/docker | reference_journey `:853` | docker | ⚠ (partial #1038) | f7 attempt-2 39845349366 = preview-assert class (fixed by C2) **plus** `cleanup: incomplete (destroy=0 preparation=1)` (q15 lineage, unfixed) ⇒ prompt-assert flips, cleanup-provenance may still fail |
| 55 | journey/podman | reference_journey `:853` | podman | ⚠ | f7 39836753432 = F5 HANG class `no-output timeout` (`tests/guided/drive.py:172`) — f7 attributes to B-owner terminal Name/cancellable-reader coverage gap, NOT the #1038 shortcut ⇒ no proven C2 flip |
| 56-58 | integration/static/{public-contracts,configuration-policy,browser-terminal} | `:1056/:1096/:1136` | static | ✓P0 | never executed; first real run |
| 59 | integration/static/live-console | integration_live_console `:1458` | static | ✓P0 + needs P0-4 | deps core+console+connector+demo pins (o21 §3 precondition) |
| 60 | integration/docker/remote-target | integration_remote_target_docker `:1366` | docker | ✓P0 + needs P0-4 | needs connector=N merged + pins |
| 61-62 | integration/docker/{core-bootstrap,coreless-lifecycle} | `:1176/:1264` | docker | ✓P0 | never executed |
| 63 | integration/podman/remote-target | integration_remote_target_podman `:1412` | podman | ✓P0 + needs P0-4 | needs N + pins |
| 64-65 | integration/podman/{core-bootstrap,coreless-lifecycle} | `:1220/:1315` | podman | ✓P0 | never executed |

W1 job-level cascade rows (q24): `v0421 ha groups · All runtime results`, `runtime heavy gates · All runtime results`,
`runtime light gates · All runtime results` — flip green iff source job green (q24 C2-flip table); corrected
consumer_ref (§2) + P0-3 candidate content fix are the actual root blockers.

### W2 diagnostic vehicle (explicit, only for W1 failures)
- repo `mcpdev80/baseharbor` · workflow `targeted-demo-acceptance.yml` · event `workflow_dispatch --ref develop`
- inputs: `ref={C2}`, `demo_ref={D2}`, `runtime=docker|podman`, `gate=guided|data-capabilities|mcp|reference-journey`
- concurrency: same `baseharbor-release-qualification` group (`targeted-demo-acceptance.yml:38-40`, blob `78c31a78`) → one-at-a-time + terminal-authenticated before next (f7 §6a queue-cancel receipt 39836748059); recommendation to split group per gate stands (f7 §6a) — until then this is a STRUCTURAL, documented serialization.
- backup-restore demo-push vehicle (`baseharbor-demo`, branch `work/v0.4.26-ecosystem-release [demo-runtime]`, cross-repo group `demo-v024-${{ github.ref }}` → parallel-safe vs core) is DIAGNOSTIC-ONLY post-C2: native route row 17/35 owns the release receipt; if used: normally-created empty commit, tree(T2)=tree(D2), never force-push (f7 recipe §6).

## 6) Batch summary
| Batch | Action | Parallelism |
|---|---|---|
| P0 | merges (#1040→#1038, U #7, N #8), rekey {C2}/{D2}, connector+console candidate content fix, pins update | prerequisite, not dispatch |
| W1 | 1 dispatch: full CMD-B-shape pre-release run (all 65 rows above) | internal docker∥podman MP2 + independent jobs ∥; single slot occupation; ≈5-8h |
| W2 | per-failed-gate targeted reruns (only rows 6/13/24/31/35/54/55 candidates + any new-class failure) | structurally serialized in-group (documented F-1 finding); cross-repo demo vehicle may overlap |
| W3 | post-W1: verifier `pre-release-resume.py` → `release-approved.json` + coverage provenance (o24 §2: only complete run populates) | after W1 terminal |

## repo-rule compliance
READ-ONLY prep lane: sole write = this file; no dispatches issued, no product-tree writes, no merges.
Reads: 4 evidence inputs + `git`/`gh` read-only (`gh pr view`, `gh api .../contents`, commit refs) — outputs
above verbatim-verified. consumer_ref correction applied per q24 (§2). Per-row inputs explicit with
placeholders {C2}/{D2}/{N40}/{U40} (C2 not yet produced — reality check §1: develop still = C, #1038/#1040
OPEN). Concurrency facts cited file:line at C (o21 §4); docker+podman parallel directive honored via single
run internal matrices; group-collapse serialization findings referenced, not silently adopted. No secrets
logged. Turn-start basics-check FLAGS processed; q25 permission event inspected (read-only home-dir pattern,
allowed once) + acked.

END F9-PREP-DONE
