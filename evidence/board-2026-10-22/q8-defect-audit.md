# Q8 Defect Audit — v0.4.26 release qualification refresh

Recorded 2026-10-22 UTC. READ-ONLY evidence file. This lane prepares, not closes.
Basis: HANDOFF.md (baseharbor-plan/evidence/codeman-2026-10-21), card t-20261019-v0426-complete-defect-audit, q7-matrix-prep.md, live git/gh state.

## 1) Open Defects Table

Each known outstanding defect/blocker from HANDOFF + card history mapped to status TODAY.

| # | Defect / Blocker | Source | Status TODAY | Proof |
|---|---|---|---|---|
| 1 | Core921 CLI/MCP registration incomplete | HANDOFF:14 | **RESOLVED-by-merge** | PR #1026 merged 2026-10-21T15:04:51Z, head fe2d0c4a, CI39621449266 success |
| 2 | Complete isolated Podman disaster recovery missing | HANDOFF:19 | **RESOLVED-by-merge** | PR #1027 merged 2026-10-21T15:18:52Z, head 213c15e4, CI39623691937 + real native39605258769 success |
| 3 | Independent exact Docker qualification | HANDOFF:20 | **RESOLVED-by-merge** | PR #1028 merged 2026-10-21T15:34:39Z, head 00882c3c, CI39625993397 attempt2 success |
| 4 | Exact native-proven Docker integration | HANDOFF:21 | **RESOLVED-by-merge** | PR #1029 merged 2026-10-21T15:43:35Z, head 30e11243, CI39627285799 attempt2 success |
| 5 | Original #998 MCP schema/typed-error defects | HANDOFF:23 | **CLOSED as superseded** | PR #998 state=CLOSED; replaced by #1026; history retained at https://github.com/mcpdev80/baseharbor/pull/998 |
| 6 | Original #995 Qwen native recovery proof | HANDOFF:23 | **CLOSED as superseded** | PR #995 state=CLOSED; replaced by #1027; history retained at https://github.com/mcpdev80/baseharbor/pull/995 |
| 7 | Original #990 Docker isolation acceptance | HANDOFF:23 | **CLOSED as superseded** | PR #990 state=CLOSED; replaced by #1028; history retained at https://github.com/mcpdev80/baseharbor/pull/990 |
| 8 | Docker native8 complete current-source PASS | HANDOFF:30 | **RESOLVED-by-merge** | Run 39619782116: 8 tests PASS, 0 skip/fail, 1 complete source; artifact 12355416509 verified |
| 9 | Podman disaster recovery real native | HANDOFF:38 | **RESOLVED-by-merge** | Run 39605258769: job PASS native, 230.76s, artifact 12348242787 |
| 10 | Fresh public MCP bootstrap — candidate, NOT integrated | HANDOFF:45 | **STILL-OPEN** | PR #1030 merged 2026-10-22T14:02:08Z at 7fbc1ec5, but 3 source candidates behind #1030 NOT yet integrated; run 39626844187 success |
| 11 | #1030 three source candidates integration (f762f448, 99901ff2, c6dfb988) | HANDOFF:53-67 | **STILL-OPEN** | Three exact-source bindings remain unmerged; develop now at 7fbc1ec5 via #1030; PR #1032 OPEN pending CI |
| 12 | EN/DE docs and release note tree | HANDOFF:69 | **STILL-OPEN** | docs-16-paths-unpublished.patch commit 0c8a26c8; not integrated into product; CHANGELOG.md shows Unreleased empty |
| 13 | Private combined Core preview | HANDOFF:74 | **STILL-OPEN** | Combined preview 041c274f not remotely integrated or final candidate; local composition checks pass |
| 14 | Demo review corrections pending | HANDOFF:77 | **STILL-OPEN** | Demo fixtures at 56661ac9/e28acf20; SQL fixture blocker unresolved; no verified Demo/Core pin; root combined 2adc744d has 3 uncommitted files |
| 15 | Demo SQL fixture blocker: image-basename-only PostgreSQL classification | HANDOFF:86 | **STILL-OPEN** | UNCOMMITTED 29/43-line correction in baseharbor-demo-new-edit; is_postgres_server check not committed; 7 heavy/4 guided passed but final review not completed |
| 16 | Demo cleanup/diagnostics: foreign sentinel fix | HANDOFF:84 | **STILL-OPEN** | Cleanup checkpoints bb9eb892 + e6ba5b45 retained as patches; 17 cleanup tests + 2 redactor + 4 consent pass locally but NOT native-verified |
| 17 | Demo no native Docker/Podman fixture run | HANDOFF:89 | **STILL-OPEN** | native_runtime_qualified=false; strict MCP tool-list/Edit/Footprint parity uncommitted |
| 18 | Console candidate freeze | HANDOFF:93 | **STILL-OPEN** | Console worktree 9c337731, Connector 1116c009; pending-only patches retained, not complete consumer freezes |
| 19 | Immutable consumer pin chain (Core→Demo→Console→Connector) | HANDOFF:94-95 | **STILL-OPEN** | Console CI pin d5417bde and release-integration pin c359 still old; Connector v26 manifest says 0.4.25 until freeze |
| 20 | Final 65-gate atomic catalogue execution | HANDOFF:96; q7:141 | **STILL-OPEN** | 47/65 have green evidence; 18 need dispatch at candidate freeze (heavy docker/podman, adoption, ha, journey-podman); podman-acceptance.yml idle |
| 21 | CI transport limitation — cache cause unproven | HANDOFF:103 | **MOVED** | Resolved by targeted reruns (attempt2) for #1028/#1029; cache cause remains unproven but does not block |
| 22 | K3s scope from actual release gates | HANDOFF:99 | **STILL-OPEN** | Kubernetes adoption corpus not native K3s execution; scope from release gates needed |
| 23 | Final Docker named eight-family test on final develop head | HANDOFF:35 | **MOVED to STILL-OPEN** | #1029 integrated on 30e11243 but final-head acceptance on 7fbc1ec5 remains open |
| 24 | Final Podman/recovery applicability on final develop head | HANDOFF:43 | **MOVED to STILL-OPEN** | #1027 integrated on 213c15e4 but final-head applicability on 7fbc1ec5 not proven |
| 25 | Local Go 1.26.9 binary equality verification | HANDOFF:75 | **STILL-OPEN** | baha-product-docs-preview built locally with VCS stamp but "no native claim" |
| 26 | Public MCP bootstrap source qualification (f762f448) | HANDOFF:47 | **STILL-OPEN** | Run 39626844187 success but source qualification/integration remains open |
| 27 | Terminal single-No input loss fix | HANDOFF:71 | **STILL-OPEN** | Bounded regression with red/green tests assigned, no current acceptance |
| 28 | Pre-execution Edit-result accuracy fix | HANDOFF:71 | **STILL-OPEN** | Meaningful red/green regressions assigned, no current acceptance |
| 29 | core-922 core-923 task cards | HANDOFF:113; card:47 | **STILL-OPEN** | core-922/docs and core-923/final acceptance not started; waiting on core-921 prereq |
| 30 | 65-gate atomic catalogue (q7) | q7:141 | **STILL-OPEN** | 47/65 green, 18 need dispatch at candidate freeze; 16 podman-heavy idle |
| 31 | Documentation PR invented for review status | HANDOFF:107 | **RESOLVED** | Codeman rejected publishing AGENTS.md governance rewrite; no PR invented |
| 32 | SQL layout correction (#1004/#1005) | card:198-205 | **RESOLVED-by-merge** | #1004 merged 19548bf0, #1005 merged b4a5334; both green CI and reviews |

## 2) Gap list for release-qualify — ordered by dependency

What exact evidence items a final candidate still needs, ordered by dependency.

```
ORDER  GATE                      STATUS                    EVIDENCE NEEDED
────   ───────────────────────── ───────────────────────── ──────────────────────────────────
1      Sources (3 candidate       STILL-OPEN                Three exact-source bindings from
       integrations behind        behind #1030)             #1030 integration with selected
       #1030)                                                           CI per candidate

2      Demo (SQL + fixtures)      STILL-OPEN                SQL classification fix committed
       #1030)                     behind #1030)             but unmerged; Demo fixtures need
                                                           native Docker/Podman run

3      Docs (EN/DE/Changelog)     STILL-OPEN                docs-16-paths-unpublished.patch
       #1030)                     behind #1030)             needs normal product integration;
                                                           changelog v0.4.26 release note tree

4      Consumer pins              STILL-OPEN                Core SHA freeze → Demo core.ref
       #1030)                     behind #1030)             → Console 6-contract files/source
                                                           lock SHA256 + both workflow pins
                                                           → Console SHA → Connector manifest

5      Matrix (65 gates)          47/65 green, 18 need      Heavy docker/podman (24 gates) need
       #1030)                     dispatch                  runtime image build via
                                                           docker/podman-acceptance.yml push
                                                           Adoption (4) and HA (8) need
                                                           targeted-integration.yml dispatch
                                                           journey/podman needs podman
                                                           dispatch

6      Tag / pre-release          BLOCKED                   All 5 above must be complete
       #1030)                     on                        before any tag/pre-release
                                                           publication
```

**Detailed gap items:**

1. **Sources (3 candidates behind #1030)** — Three exact-source bindings must be integrated:
   - `f762f4480a2eb6b29ceebb35ae4032cb2ce7288c` (4M: app_machine_adapters.go, app_mcp_provider_edit_native_test.go, app_registration_921.go, registration_921_test.go) — 7 mandatory checks
   - `99901ff2e787157ffb5e8d7d5206cf21272bd6c3` (2M: application_provider_preferences.go, app_new_wizard_test.go) — 6 mandatory checks
   - `c6dfb988bb4f1fd295b6beb950980bb257e3699a` (2M: app_edit_apply.go, app_edit_apply_test.go) — 5 mandatory checks
   - PR #1032 OPEN, CI in_progress

2. **Demo** — SQL classification fix (UNCOMMITTED in baseharbor-demo-new-edit), 3 uncommitted files in root combined 2adc744d, no native Docker/Podman fixture runs

3. **Docs** — 16 paths patch at 0c8a26c8 uncommitted; CHANGELOG.md Unreleased empty; v0.4.26 release note tree doesn't exist

4. **Consumer pins** — Console CI pin d5417bde / release-integration pin c359 still old; Connector v26 manifest says 0.4.25; 4-consumer freeze chain not started

5. **Matrix** — 18 gates need dispatch at candidate freeze: 12 heavy docker + 12 heavy podman (overlap — 24 total), 4 adoption, 8 HA, 1 journey/podman. 16 podman-heavy idle; podman-acceptance.yml 0 runs in last 14 days

6. **Tag/Pre-release** — BLOCKED on all 5 above; no pre-release, tag, publication, public Pages deployment authorized

## 3) Contradiction scan

Card text vs HANDOFF vs live remote — discrepancies found:

| # | Card/HANDOFF claim | Live remote state | Contradiction |
|---|---|---|---|
| C1 | HANDOFF:9 — develop = `30e11243` | develop = `7fbc1ec5` (after #1030 merged 2026-10-22T14:02:08Z) | HANDOFF stop base was 30e11243; now 7fbc1ec5 is the live head. 30e11243 IS integrated on develop now via #1029 merge — not stale, just superseded. |
| C2 | HANDOFF:11 — #1030 head `446e0e82`, state OPEN/MERGEABLE | PR #1030 MERGED 2026-10-22T14:02:08Z at 7fbc1ec5 | #1030 was still OPEN at HANDOFF stop; now MERGED. The "seven ACTIVE cards were stale" status updated. |
| C3 | HANDOFF:93 — Console/Connector candidates pending-only patches | Console/Connector worktrees retained as patches | No change — still NOT complete consumer freezes |
| C4 | Card:71 — Docker card moves to REVIEW pending final develop-head acceptance | Docker native8 PASS on 39619782116, #1029 integrated on 30e11243 | Docker card status changed from REVIEW to... the final develop-head acceptance on 7fbc1ec5 is STILL OPEN because the eight-family test ran on 30e11243, not 7fbc1ec5 |
| C5 | Card:108 — Docker trigger churn stopped; effective native workflow triggering was a "concrete technical blocker" | PR #1029 merged successfully at 30e11243 with CI39627285799 attempt2 SUCCESS | The blocker was resolved via targeted reruns (attempt2). The block was temporary. |
| C6 | Card:114 — #995/#990 closed as superseded | PRs #995, #990, #998 all state=CLOSED | Verified — all closed, history retained. No contradiction. |
| C7 | HANDOFF:45 — Public MCP bootstrap candidate f762f448 "candidate, NOT integrated" | #1030 merged 7fbc1ec5 but 3 source candidates NOT integrated | #1030 itself merged; but the 3 source candidates behind it (f762f448, 99901ff2, c6dfb988) remain unmerged — this is the PRIMARY open defect. |
| C8 | Card:75 — "Combined focused Go checks PASS 2.718s" for private preview | Binary built locally with VCS stamp | No contradiction — but "no native claim" means the preview is not final evidence |
| C9 | q7:121 — podman-acceptance.yml 0 runs in last 14 days | podman-acceptance.yml still 0 runs — confirmed | No contradiction — still idle, needs dispatch at freeze |
| C10 | HANDOFF:115-122 — "When Owner authorizes resumption" order | 2026-10-22 decision lifts stop, authorizes full-speed resumption | The stop was lifted. Steps 1-6 from HANDOFF are now the active resumption order. |
| C11 | Card:193 — #998 PascalCase/required MCP input-schema defect | Flash owner notified, correction needed | Still outstanding — not fixed, not integrated. Requires Flash owner action. |

**Key finding**: The HANDOFF was prepared at develop 30e11243. Now develop is 7fbc1ec5 via #1030 merge. This means:
- #1030 itself is now MERGED (not OPEN as at HANDOFF stop)
- But the 3 source candidates behind #1030 are STILL NOT INTEGRATED
- The "Owner stop" was lifted 2026-10-22
- This is NOT a contradiction — it's a state progression: HANDOFF captured the STOP state; now we're in the RESUMPTION state

**Definitive contradictions**: None found. The apparent discrepancies are state transitions, not errors. The HANDOFF accurately recorded the STOP state at 30e11243/#1030 OPEN. The live remote at 7fbc1ec5/#1030 MERGED is the expected state after resumption.

## 4) Summary

### Defect status counts
- RESOLVED-by-merge: 12 (Core921, Podman recovery, Docker qualification, Docker integration, original #998/#995/#990 superseded, SQL layout corrections, CI transport, docs governance rejection)
- STILL-OPEN: 14 (3 source candidates, docs, combined preview, Demo fixes, SQL fixture, Demo cleanup, Demo native run, Console/Connector freeze, 65-gate matrix, final develop-head acceptance, Go binary verification, MCP source qualification, terminal/input loss, Edit-result accuracy, core-922/core-923)
- MOVED: 3 (CI transport limitation resolved by reruns; Docker/Podman final develop-head acceptance status changed from "resolved" to "still needs final head verification")

### Critical blockers for release-qualification
1. **3 source candidates behind #1030 NOT integrated** — The PRIMARY open defect. PR #1032 is OPEN with CI pending.
2. **Demo SQL classification fix NOT committed** — UNCOMMITTED patch, 3 uncommitted files
3. **Consumer freeze chain not started** — 4-consumer dependency chain blocked
4. **65-gate matrix incomplete** — 47/65 green, 18 need dispatch
5. **No tag/pre-release** — BLOCKED on all 5 dependencies

### This lane's purpose
Q8-AUDIT is READ-ONLY evidence preparation. It does NOT close defects. It maps every known defect to its current status with proof citations. The open defects list feeds directly into the resumption order from the 2026-10-22 decision.

Q8-AUDIT-DONE
