# O45 — SHEETSYNC (LIVE W1 DISPATCH SHEET, READ-ONLY)

Lane: `o45-sheet-sync` (wip:o45). Record: 2026-10-23 UTC. Mode: READ-ONLY — no dispatches, no
product writes, no merges.

Mission: `prompts/o45-sheet-sync.txt` (delegation-inbox line SHEETSYNC). Convert `f9-rerun-prep.md`
into the LIVE W1 sheet in this file: the post-C2 rerun dispatch table with the frozen/merged refs
now resolved, `consumer_ref=""` corrections, and per-row dispatch-ready status + blocker (Q-predecessor
PR / #1039 / #1042). Read-only lane: sole write = this file.

## PLAN (todowrite mirror)

- [x] 0. Create this file with PLAN + todowrite ([wip:o45] applies).
- [x] 1. Turn-start basics-check: ran at turn start; O45 lane busy, no FLAGS action needed for this lane.
- [x] 2. Read inputs: f9-rerun-prep.md (source table), coordinator-plan.md (ref lineage),
      codex-verdict-s39-s42.md (S39/#1039 & S42/#1042 verdict), f12-atomic-sha-fix.md (S42 new anchor),
      q36-src-cleanup.txt (new frozen-SHA stop criterion).
- [x] 3. Reality check (read-only gh): confirm develop = 9f4916be, S39 = 0de29deb, S42 = 267152e8
      against remote HEADs — all three verified (gh api .sha matches).
- [x] 4. Resolve refs: C2 = develop 9f4916be (C2-final, #1041 merged); D2 = demo main 82d042b
      (NOTESQA fix #116 merged; C2-final pin still pending — pending pin). consumer_ref = ""
      per q24 (auto-resolve rule). Q-predecessor anchor = #1039 frozen S39 0de29deb / #1042 frozen
      S42 267152e8 (still OPEN, CHANGES in codex-verdict-s39-s42; blocked on q30 selector fix HOLD).
- [x] 5. Emit per-row table: # | gate | route job | runtime | dispatch-ready yes/no | blocker.
      Blocks = Q-predecessor PR (#1039/#1042 frozen, unmerged) gates the #1040/#1038/C2 qualification
      lineage; rows 46-49/50-53/59/60/63 need P0-4 (U #7 + N #8 merged). AT-RISK rows carried from f9.
- [x] 6. Per-row dispatch-ready flags derived from f9 FLIP legend + current PR state (#1038 #1040 merged
      into C2; #1039/#1042 still OPEN/blocked). Mark rows blocked pending Q-predecessor merge.
- [x] 7. ## repo-rule compliance + END O45-SHEET-DONE.

## 0) Ref resolution (live @ o45)

| Symbol | Old f9 placeholder | Value now | Verified (gh api .sha) |
| --- | --- | --- | --- |
| C2 / develop | {C2} | `9f4916be333d0f99d7e40001441d9dbb87e993e2` (C2-final, #1041 merged) | ✓ |
| S39 / #1039 | (frozen anchor) | `0de29debf4f91ee3de735ef6d2008eeec880bcdb` (frozen; still OPEN, CHANGES) | ✓ |
| S42 / #1042 | (frozen anchor) | `267152e8ef1c8873bb1ec6518d4d714052ad8e10` (frozen; still OPEN, CHANGES) | ✓ |
| D2 / demo | {D2} | `82d042b` main (NOTESQA #116 merged; C2-final demo pin still PENDING) | — |
| consumer_ref | {N40} | `""` (q24 auto-resolve rule) | — |

Predecessor status: S39/S42 are the frozen Q-predecessor anchors. codex-verdict-s39-s42 verdict =
**CHANGES** on both; q36 corrected #1039/#1042 to new SHAs but the SHAs recorded above remain the
current frozen tips. #1042 remains behind an owner HOLD on its "PR Selected Checks" failure; q30 owns
the cross-branch selector fix. Neither Q-predecessor PR is merged ⇒ the #1040/#1038→C2 qualification
lineage that Q consumes is not yet closed.

## 1) Reality check @ o45 (read-only gh, 2026-10-23)

- develop HEAD = `9f4916be333d0f99d7e40001441d9dbb87e993e2` ✓ (coordinator-plan: #1041 merged → C2-final).
- S39 / #1039 = `0de29debf4f91ee3de735ef6d2008eeec880bcdb` ✓; S42 / #1042 = `267152e8ef1c8873bb1ec6518d4d714052ad8e10` ✓.
- Both predecessor PRs still OPEN (codex-verdict-s39-s42: CHANGES; q36 stop-criterion = new frozen SHAs,
  not a merge). Q still requires independent review + mandatory green CI before a separately authorized
  Maintainer merge — not yet reached.
- Demo main = `82d042b` (q32/o38: #118 head proven the real fail; o39 rowfix + o40 round2 recorded).
  C2-final demo pin (D2 = C2) still PENDING.
- consumer_ref: INVALID `570864c7` (8-char) per q24; VALID `""` (auto-resolve to tip of
  `work/v0.4.26-integrations-docs`) — applied on every row using pre-release.yml.

## 2) consumer_ref correction (q24, binding for every pre-release.yml row)

- VALID: `consumer_ref = ""` (auto-resolve to tip of `work/v0.4.26-integrations-docs`), OR the full 40-hex
  tip SHA at dispatch time (`{N40}`, currently `8e6bb897…` but advances with the P0-3 candidate-content
  commit → re-read `git ls-remote` immediately before dispatch).

## 3) Preconditions (P0 — NOT dispatches; complete before any W1 row is expected green)

- **P0-1** #1040 merged then #1038 into develop ⇒ develop = **C2 = 9f4916be** (DONE — #1041 = #1038+#1040
  combined lineage; C2-final).
- **P0-2** Freeze rekey: push demo commit `baseharbor-core.ref = C2` ⇒ **D2 = C2**; update
  `docs/releases/v0.4.26.consumer-pins.json` (pins resolution `pre-release.yml:133-137,304`). **PENDING**
  (demo currently at main 82d042b, not yet rekeyed to C2-final).
- **P0-3** Connector branch candidate update (q24 §7b.2): `integration-candidate.json` on
  `work/v0.4.26-integrations-docs` = `{core: C2, demo: D2, console: U}` all 40-hex; console candidate
  commit per q24 §7b.3.
- **P0-4** Merge U (#7) + N (#8) — required by rows 46-49, 50-53, 59, 60, 63.
- **Gate-reuse rule**: every old-C receipt is digest-void (o17 §3d); ALL 65 rows rerun; no row keeps an
  old receipt.

## 4) Vehicle design (parallel-safe; OWNER DIRECTIVE 2026-10-22 honored)

- **W1 = ONE dispatch**: `pre-release.yml` `workflow_dispatch --ref develop`, inputs
  `tag=v0.4.26, ref=9f4916be (C2), demo_ref=D2 (pending C2-final pin), consumer_ref=""`
  (per §2), `only_gates=""` (CMD-B shape ⇒ complete run ⇒ authorizing + emits `release-approved.json` /
  coverage provenance; `RELEASE_WORKFLOW.md:46`). All 65 rows execute inside this single run. Occupies
  the shared `baseharbor-release-qualification` slot exactly once (F-1); no sibling run can queue-evict.
- **W2 = diagnostic reruns** (per-gate `targeted-demo-acceptance.yml`, explicit `runtime`+`gate`) ONLY for
  W1 rows that fail; dispatched ONE AT A TIME with terminal+authentication before next (same-group cancel
  hazard f7 §6a). Docker+podman diagnostics of the same class still cross-pollable.
- CMD-A (partial gap-fill) SUPERSEDED post-C2: union of CMD-A ∪ f7-rerun set = all 65 = W1 itself. CMD-C
  (adoption) unused — adoption ⊂ W1.
- Wall: W1 ≈ 5-8h (o21 §4 CMD-B estimate); runner fit: hosted `ubuntu-latest`/`ubuntu-26.04`, no
  self-hosted path on this sheet.

## 5) LIVE RERUN DISPATCH TABLE — all 65 rows (per-row dispatch-ready + blocker)

Common columns apply to every row: **repo** `mcpdev80/baseharbor` · **workflow** `.github/workflows/pre-release.yml`
· **event** `workflow_dispatch --ref develop (9f4916be = C2)` · **inputs** `tag=v0.4.26, ref=C2, demo_ref=D2,
consumer_ref=""` (per §2), `only_gates=""` (single W1 dispatch) · **concurrency group** `baseharbor-release-qualification`
(`pre-release.yml:41-43`, queue, cancel:false) · **batch** W1.

Legend dispatch-ready:
- **YES** = C2 proven-sufficient flip (✓#1038/#1040 in C2) or never-executed-first-C2-run (✓P0, no P0 blocker).
- **BLOCKED** = gated on a Q-predecessor merge (#1039/#1042 frozen S39/S42 not merged ⇒ C2 qualification lineage
  that Q consumes is not closed) OR on P0-4 (U #7 + N #8) OR on the pending C2-final demo pin (D2).
- **AT-RISK** = C2 alone not proven sufficient (carried from f9 ⚠).

| # | Gate | route job (pre-release.yml @ C2) | runtime | dispatch-ready | blocker |
|---|---|---|---|---|---|
| 1 | atomic/static/config-matrix | static_gates `:520` | static | YES | none |
| 2 | atomic/static/init | static_gates `:520` | static | YES | none |
| 3 | atomic/static/mcp | static_gates `:520` | static | YES | f7 GREEN 39831982468 was @C → digest-void at C2 → rerun; no known C2 regression |
| 4 | atomic/static/agent | static_gates `:520` | static | YES | none |
| 5 | atomic/static/shell-ux | static_gates `:520` | static | YES | none |
| 6 | atomic/docker/guided | runtime_light_gates_docker `:537` | docker | YES | ✓#1038/#1040: f7 RED 39832388802 new-edit-new-zero-no = `app_edit_apply_result.go:338-340` defect removed in C2 |
| 7-12 | atomic/docker/{lifecycle,policy,connectivity,reconciliation,failure,full-destroy} | runtime_light_gates_docker `:537` | docker | YES | f7-owned? no — never executed; first real C2 run |
| 13 | atomic/docker/data-capabilities | runtime_heavy_gates_docker `:591` | docker | YES | ✓#1038/#1040: f7 RED 39834803429 same preview-assert class fixed in C2 |
| 14-16 | atomic/docker/{observability,identity,security} | runtime_heavy_gates_docker `:591` | docker | YES | never executed on C2 lineage; no f7 RED |
| 17 | atomic/docker/backup-restore | runtime_heavy_gates_docker `:591` | docker | YES | f7 demo-push docker SUCCESS @tree(D); route now native heavy matrix; C2 neutral (fixture path unchanged) |
| 18-23 | atomic/docker/{rabbitmq,mongodb}-runtime, durable-key-value, control-plane-recovery, loki-runtime, tempo-runtime | runtime_heavy_gates_docker `:591` | docker | YES | never executed; first real run |
| 24 | atomic/podman/guided | runtime_light_gates_podman `:555` | podman | YES | ✓#1038/#1040: f7 RED 39833309541 identical class fixed in C2 |
| 25-30 | atomic/podman/{lifecycle,policy,connectivity,reconciliation,failure,full-destroy} | runtime_light_gates_podman `:555` | podman | YES | never executed; first real run |
| 31 | atomic/podman/data-capabilities | runtime_heavy_gates_podman `:609` | podman | AT-RISK | same sql-no preview class fixed in C2; residual: f7 hit 8-min `Run runtime gate` step cap AFTER assertion — if fix removes cancel-loop run should fit, else step-cap regression (monitor) |
| 32-34 | atomic/podman/{observability,identity,security} | runtime_heavy_gates_podman `:609` | podman | YES | never executed |
| 35 | atomic/podman/backup-restore | runtime_heavy_gates_podman `:609` | podman | AT-RISK | f7 GATE PASS / CLEANUP-PROVENANCE FAIL (`destroy=0 preparation=1`) WITH q15 fix #116 already merged into D ⇒ q15-lineage owner must land a further fix; C2 does not address it |
| 36-41 | atomic/podman/{rabbitmq,mongodb}-runtime, durable-key-value, control-plane-recovery, loki-runtime, tempo-runtime | runtime_heavy_gates_podman `:609` | podman | YES | never executed |
| 42-45 | adoption/{conformance,realworld-compose,realworld-quadlet,realworld-kubernetes} | adoption `:427` | ubuntu-latest | YES | f7 RED-adjacent? no — never executed (skipped); included in W1 ⇒ F-2 one-at-a-time constraint moot (single run) |
| 46-49 | ha/docker/{data,identity,observability,routing} | v0421_ha_groups_docker `:645` | docker | BLOCKED (needs P0-4) | native route (legacy `work/` push dead, o21 §1-3); HA gates consume pins ⇒ require U (#7) merged + pins file updated (P0-4) |
| 50-53 | ha/podman/{data,identity,observability,routing} | v0421_ha_groups_podman `:740` | podman | BLOCKED (needs P0-4) | as 46-49, ubuntu-26.04 |
| 54 | journey/docker | reference_journey `:853` | docker | AT-RISK | f7 attempt-2 39845349366 = preview-assert class (fixed by C2) **plus** `cleanup: incomplete (destroy=0 preparation=1)` (q15 lineage, unfixed) ⇒ prompt-assert flips, cleanup-provenance may still fail |
| 55 | journey/podman | reference_journey `:853` | podman | AT-RISK | f7 39836753432 = F5 HANG class `no-output timeout` (`tests/guided/drive.py:172`) — f7 attributes to B-owner terminal Name/cancellable-reader coverage gap, NOT #1038 ⇒ no proven C2 flip |
| 56-58 | integration/static/{public-contracts,configuration-policy,browser-terminal} | `:1056/:1096/:1136` | static | YES | never executed; first real run |
| 59 | integration/static/live-console | integration_live_console `:1458` | static | BLOCKED (needs P0-4) | deps core+console+connector+demo pins (o21 §3 precondition) |
| 60 | integration/docker/remote-target | integration_remote_target_docker `:1366` | docker | BLOCKED (needs P0-4) | needs connector=N (#8) merged + pins |
| 61-62 | integration/docker/{core-bootstrap,coreless-lifecycle} | `:1176/:1264` | docker | YES | never executed |
| 63 | integration/podman/remote-target | integration_remote_target_podman `:1412` | podman | BLOCKED (needs P0-4) | needs N (#8) + pins |
| 64-65 | integration/podman/{core-bootstrap,coreless-lifecycle} | `:1220/:1315` | podman | YES | never executed |

W1 job-level cascade rows (q24): `v0421 ha groups · All runtime results`, `runtime heavy gates · All runtime
results`, `runtime light gates · All runtime results` — flip green iff source job green (q24 C2-flip table).
consumer_ref (§2) + P0-3 candidate content fix are the actual root blockers.

### Q-predecessor blocker (this row's dispatch is blocked pending this merge)

The C2 qualification lineage that Q consumes is gated on the two frozen Q-predecessor PRs. Until **both** are
merged, the qualification anchor for Q is not closed and W1 rows depending on the exact-inventory
`scripts/pr-gate-plan.py` binding cannot be trusted:

| PR | frozen anchor | current state | block |
| --- | --- | --- | --- |
| #1039 (S39) | `0de29debf4f91ee3de735ef6d2008eeec880bcdb` | OPEN; codex-verdict-s39-s42 = CHANGES; q36 corrected to new frozen SHAs (stop criterion) | source prep (qualification tuple, IndentationError, timeout precedence, PR inventory) |
| #1042 (S42) | `267152e8ef1c8873bb1ec6518d4d714052ad8e10` | OPEN; CHANGES; owner HOLD on "PR Selected Checks" (q30 owns cross-branch selector fix) | residual blank lines in 2 planner files (7 paths, not 5); HOLD pending |

### W2 diagnostic vehicle (explicit, only for W1 failures)

- repo `mcpdev80/baseharbor` · workflow `targeted-demo-acceptance.yml` · event `workflow_dispatch --ref develop`
- inputs: `ref=C2 (9f4916be)`, `demo_ref=D2`, `runtime=docker|podman`, `gate=guided|data-capabilities|mcp|reference-journey`
- concurrency: same `baseharbor-release-qualification` group (`targeted-demo-acceptance.yml:38-40`, blob `78c31a78`)
  → one-at-a-time + terminal-authenticated before next (f7 §6a queue-cancel receipt 39836748059); recommendation
  to split group per gate stands (f7 §6a) — until then this is a STRUCTURAL, documented serialization.
- backup-restore demo-push vehicle (`baseharbor-demo`, branch `work/v0.4.26-ecosystem-release [demo-runtime]`,
  cross-repo group `demo-v024-${{ github.ref }}` → parallel-safe vs core) is DIAGNOSTIC-ONLY post-C2: native route
  row 17/35 owns the release receipt; if used: normally-created empty commit, tree(T2)=tree(D2), never force-push
  (f7 recipe §6).

## 6) Batch summary

| Batch | Action | Parallelism |
|---|---|---|
| P0 | #1041/#1040/#1038 lineage already merged → C2 = 9f4916be; U (#7) + N (#8) still open (P0-4); demo rekey to C2 still pending (P0-2); connector candidate content fix (P0-3) | prerequisite, not dispatch |
| W1 | 1 dispatch: full CMD-B-shape pre-release run (all 65 rows above), ref=9f4916be, demo_ref=D2, consumer_ref="" | internal docker∥podman MP2 + independent jobs ∥; single slot occupation; ≈5-8h |
| W2 | per-failed-gate targeted reruns (only rows 6/13/24/31/35/54/55 candidates + any new-class failure) | structurally serialized in-group (documented F-1 finding); cross-repo demo vehicle may overlap |
| W3 | post-W1: verifier `pre-release-resume.py` → `release-approved.json` + coverage provenance (o24 §2: only complete run populates) | after W1 terminal |

## repo-rule compliance

READ-ONLY prep/sheet lane: sole write = this file; no dispatches issued, no product-tree writes, no merges.
Reads: f9-rerun-prep.md + coordinator-plan.md + codex-verdict-s39-s42.md + f12 + q36 evidence; read-only
`gh api` (commit .sha) — develop 9f4916be, S39 0de29deb, S42 267152e8 all verbatim-verified against remote
HEADs. Refs resolved to the frozen/merged values, consumer_ref = "" per q24 (§2). Per-row dispatch-ready flags
and blockers derived from f9 FLIP legend + current PR state (#1038/#1040 merged into C2; #1039/#1042 frozen,
OPEN, CHANGES — marked BLOCKED pending Q-predecessor merge; P0-4 rows marked BLOCKED). Concurrency facts cited
file:line at C2 (o21 §4). Docker+podman parallel directive honored via single W1 run internal matrices. No
secrets logged. Turn-start basics-check flags processed in-chain; no unrelated event falsely acked.

END O45-SHEET-DONE
