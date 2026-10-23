# O15 · Prepare-evidence ledger — t-20261017-release-0-4-26-prepare

READ-ONLY evidence assembly. No planning files edited, no product writes.
Card: `t-20261017-release-0-4-26-prepare` (v0.4.26 · prepare stage · status: waiting).
Built from: card note, HANDOFF.md (codeman-2026-10-21), o13b-matrix-check.md, git develop log, `gh api`.

## PLAN
- [x] 1. Read task card + HANDOFF + o13b matrix heat (read-only)
- [x] 2. develop SHA history since 2026-10-21 15:00Z (git log)
- [x] 3. Merged PRs: run id + conclusion + artifacts (gh api, read-only)
- [x] 4. Acceptance-checklist status lines (MET/PENDING/BLOCKED) with proof links
- [x] 5. Remaining gaps ordered by dependency (HANDOFF + o13b)
- [x] 6. END O15-PREP-EVID-DONE

---

## 1) develop SHA history since 2026-10-21 15:00Z (baseharbor)

develop HEAD (frozen candidate tree): **`7fbc1ec51dee853c57783ce29f4d350e259bbdff`**

| Commit | Date (Z) | Author | Merged PR | Subject |
|---|---|---|---|---|
| `7fbc1ec5…` | 2026-10-22 14:02Z | Marcel | #1030 | ci: qualify public MCP bootstrap and bounded CLI admission fixes |
| `30e11243…` | 2026-10-21 15:43Z | Marcel | #1029 | fix(ci): integrate fully proven isolated Docker runtime acceptance |
| `00882c3c…` | 2026-10-21 15:34Z | Marcel | #1028 | ci: qualify exact native-proven Docker runtime fixtures |
| `213c15e4…` | 2026-10-21 15:18Z | Marcel | #1027 | fix(podman): complete isolated backup recovery and preserve shared resources |
| `fe2d0c4a…` | 2026-10-21 15:04Z | Marcel | #1026 | feat(core-921): complete CLI and MCP registration with native provider-edit proof |
| `1aa96693…` | 2026-10-21 14:52Z | Marcel | #1025 | ci: await mandatory authenticated job results from the trusted base |
| `0950bef1…` | 2026-10-21 14:46Z | Marcel | #1024 | ci: qualify trusted final-check dependencies and bounded API readiness |
| `f2006e70…` | 2026-10-21 14:15Z | Marcel | #1022 | fix(runtime): detach provider preferences from withdrawn runtime intent |
| `9f2c67b2…` | 2026-10-21 14:01Z | Marcel | #1021 | ci: qualify native-proven retirement, registration and PG17 recovery sources |

9 commits since cutoff. `#1026–#1030` merged directly by Marcel (no PR check-runs on those merge SHAs — they are pre-cutoff trusted-base activations per HANDOFF). The only PR with a live check-matrix on its merge SHA is **#1030** (`7fbc1ec5`).

## 2) Merged PRs: run id + conclusion + artifacts

### #1030 — `7fbc1ec5` (current develop HEAD, live matrix)
Matrix from `gh api …/commits/7fbc1ec5/check-runs` (15 check-runs).

| Gate | Conclusion | Run id | Job | Proof |
|---|---|---|---|---|
| static · mcp | **success** | 39799704368 | 107867010445 | https://github.com/mcpdev80/baseharbor/actions/runs/39799704368 |
| docker · guided | **failure** | 39802282511 | 107874404300 | https://github.com/mcpdev80/baseharbor/actions/runs/39802282511 |
| docker · data-capabilities | **failure** | 39802793795 | 107875847777 | https://github.com/mcpdev80/baseharbor/actions/runs/39802793795 |
| podman · guided | **failure** | 39800160390 | 107868323773 | https://github.com/mcpdev80/baseharbor/actions/runs/39800160390 |
| podman · data-capabilities | **failure** | 39799744162 | 107867121967 | https://github.com/mcpdev80/baseharbor/actions/runs/39799744162 |
| resolve (×5) | success | 39799704368 / 39802282511 / 39800160390 / 39799744162 | — | same runs |
| Reference Journey (×4) | skipped | 39802793795 / 39802282511 / 39800160390 / 39799744162 | — | same runs |

Per-gate artifact links (from per-run check-runs):
- docker guided fail: https://github.com/mcpdev80/baseharbor/actions/runs/39802282511/job/119906009516
- docker data-capabilities fail: https://github.com/mcpdev80/baseharbor/actions/runs/39802793795/job/119909507904
- podman guided fail: https://github.com/mcpdev80/baseharbor/actions/runs/39800160390/job/119901659865
- podman data-capabilities fail: https://github.com/mcpdev80/baseharbor/actions/runs/39799744162/job/119897230556
- static mcp pass: https://github.com/mcpdev80/baseharbor/actions/runs/39799704368/job/119896631523

**#1030 overall: NOT GREEN** — 1 static pass, 4 runtime selected-gates failed, 4 reference_journey skipped. Per AGENTS.md "no green gates by assertion": do not treat resolve/success as matrix pass.

### Accepted pre-cutoff integrations (HANDOFF §2, run ids from HANDOFF, read-only)
| Work | PR / merge SHA | Handoff run id |
|---|---|---|
| Core921 CLI/MCP registration | #1026 `fe2d0c4a` | 39621449266 |
| Complete isolated Podman recovery | #1027 `213c15e4` | 39623691937 |
| Independent exact Docker qualification | #1028 `00882c3c` | 39625993397 (attempt2) |
| Exact native-proven Docker integration | #1029 `30e11243` | 39627285799 (attempt2) |

Real native receipts (from HANDOFF §3): Docker 8-family `c1337192` run 39619782116, ZIP SHA256 `dbec35c8…`; rootless Podman recovery `77eb9a12` run 39605258769, SHA256 `98db0dd3…`; public MCP bootstrap `f762f448` run 39626844187, ZIP SHA256 `a1c4270f…`.

## 3) Acceptance-checklist status for t-20261017-release-0-4-26-prepare

Based on card "Goal/Scope/Acceptance", RELEASE_WORKFLOW#prepare, HANDOFF, o13b matrix heat.
Legend: **MET** proof present & authoritative · **PENDING** in progress / partial · **BLOCKED** prerequisite not accepted.

| # | Card acceptance item | Status | Proof |
|---|---|---|---|
| 1 | Canonical audit sections 1–8 + §9 atomic-first prereqs; RELEASE_WORKFLOW#prepare re-read | PENDING | No prepared audit/evidence section exists on this card yet (card still `#status/planned`); see HANDOFF as latest state |
| 2 | All assigned implementation/CI/docs/consumer tasks accepted; release-note/epic criteria verified | BLOCKED | depends_on lists ~30 prereqs; only Core921/binding + 3 Podman slices + #1030 core integrations accepted per HANDOFF |
| 3 | Implementation/security/ownership/standards + CLI/JSON/MCP parity | PENDING | static mcp pass (#1030); CLI parity verified locally in HANDOFF but not as final-candidate CI |
| 4 | Current Core/BaseHarbor / Demo / Console / Node Connector compatibility | BLOCKED | Console `9c337731` + Connector `1116c009` are pending-only worktrees, not complete consumer freezes (HANDOFF §Consumer) |
| 5 | Real supported developer journeys, auth/mTLS, cleanup evidence | PENDING | Demo native fixtures exist (e28/e6/cleanup) but no verified Docker/rootless-Podman native run (`native_runtime_qualified=false`, HANDOFF §Demo) |
| 6 | Concise EN/DE docs/reference/specs, nav/links, changelog | PENDING | 16 unpublished docs paths `0c8a26c8` exist; not integrated into final candidate, Pages candidate not validated |
| 7 | Human release message, compat/security/upgrade/deferred notes | PENDING | q11/q12/q13 relnote drafts exist (board); final message not authored |
| 8 | Strict Pages candidate builds/workflow without deployment | PENDING | Pages candidate not validated (HANDOFF §Unpublished docs) |
| 9 | Freeze immutable ecosystem SHAs, candidate tree, gate requirements, verifier-compatible evidence | PENDING | develop tree frozen at `7fbc1ec5`; three source candidates NOT yet integrated into one candidate; gates not frozen (#1030 matrix fails) |
| 10 | Verify all required targeted checks, runtime/platform gates, consumer receipts | BLOCKED | #1030 matrix: 4 runtime selected-gates FAIL; 34–36 gates have no fresh green evidence (o13b) |
| 11 | Reviewable readiness dossier + exact authorized-run plan | PENDING | Dossier = this ledger; authorized-run plan not issued (pre-release not authorized) |
| 12 | Preparation does NOT authorize full pre-release/promotion/tag/publication | MET | Card + HANDOFF both explicit "PRE-READY: NO / OWNER REQUESTED STOP" |

## 4) Remaining gaps (ordered by dependency)

Source: HANDOFF.md (codeman-2026-10-21) + o13b-matrix-check.md. Gaps ordered so each depends only on earlier items.

### Tier 0 — hard blockers
1. **Owner stop in force.** HANDOFF.md is `OWNER REQUESTED STOP`; no resumption/publication authorized without Marcel. (Blocks everything below.)
2. **#1030 matrix not green.** docker guided/data-capabilities + podman guided/data-capabilities FAIL; reference_journey skipped. Must fix/qualify before candidate freeze. → run id 39802282511/39802793795/39800160390/39799744162.

### Tier 1 — Core candidate (depends on Tier 0 #2 clearing)
3. **Accept #1030 then integrate 3 frozen sources separately**, each with expected SHA + selected CI:
   - `f762f448` (4M: app_machine_adapters, app_mcp_provider_edit_native_test, app_registration_921, registration_921_test) — 7 mandatory checks
   - `99901ff2` (2M: application_provider_preferences, app_new_wizard_test) — 6 checks
   - `c6dfb988` (2M: app_edit_apply, app_edit_apply_test) — 5 checks; `2f32556c` Edit source gets **no** exemption (BLOCKED on its 2 atomic runtime reqs).

### Tier 2 — Demo (depends on Tier 1 #3 + SQL classification fix)
4. **SQL server-classification dirty patch**: `is_postgres_server` correction (29/43 lines UNCOMMITTED in `baseharbor-demo-new-edit`) — fixes one-server guard; not reviewed/committed (BLOCKED on review).
5. **Integrate full Demo**: e28 + e6 + MCP run wiring; no verified Docker/rootless-Podman native run yet (`native_runtime_qualified=false`).
6. **Consumer pins**: Demo core.ref ← Core develop SHA, then Demo SHA → Console.

### Tier 3 — Docs + consumers (depends on Tier 1 + Tier 2)
7. **Integrate 16 docs paths** into final candidate; validate EN/DE + Pages + reference/coverage candidate, no deployment.
8. **Freeze four consumers** in noncircular order: Core develop SHA → Demo core.ref → Demo SHA → Console six contract files + source-lock SHA256 + workflow pins → Console SHA → Connector `work/v0.4.26-integrations-docs` branch/manifest → exact Connector SHA. Console CI pin `d5417bde` / release-integration pin `c359` still old.

### Tier 4 — Final matrix (depends on Tiers 1–3)
9. **~34–36 gates with no fresh (7d) green evidence** (o13b): 22 heavy runtime (11 docker + 11 podman) + 4 adoption + 1 journey/podman + ~7 podman-light.
   - Heavy docker/podman + journey + adoption → `pre-release.yml` (362658451) `only_gates` or `targeted-integration.yml` (372069921) `gate` choice.
   - HA (8) → legacy `work/v0.4.24-core-ha-updates` / `work/v0.4.25-contract-minimal-core` pushes; do NOT push develop to fake.
   - `full-destroy` = heavy, route through pre-release `only_gates`, not the light runner.
10. **Final develop native checks + release-source-preflight + final integrated SHA verification** outstanding (HANDOFF §Consumer last line).
11. **K3s scope** must come from actual release gates; existing Kubernetes adoption corpus is not native K3s (do not invent native K3s PASS).

### Dead / non-issues (already resolved, not blockers)
- `release-candidate/v0.4.25-final-f325b93c` — DEAD (only historical failed run).
- #998/#995/#990/#1007/#1011/#1014 — CLOSED as superseded (HANDOFF).
- No verified max-11 container limit exists — do not carry as blocker.

---

## repo-rule compliance (read-only)
- `git log` on baseharbor develop (local, no remote push).
- `gh api` read-only: commits/check-runs, workflow runs, refs. No dispatches, no pushes, no product writes.
- Read only planning files (card, HANDOFF, o13b, release note); **no planning files edited** — report only.
- Evidence written to `evidence/board-2026-10-22/o15-prepare-evidence.md` per planning rules.

O15-PREP-EVID-DONE
