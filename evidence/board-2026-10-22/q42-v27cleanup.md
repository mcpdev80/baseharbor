# q42-v27cleanup Evidence — Release-start reconciliation for v0.4.27

## PLAN [wip:q42]

0. evidence board-2026-10-22/q42-v27cleanup.md mit PLAN + todowrite ([wip:q42])
1. Read claim task card via `python3 scripts/plan.py show t-20261017-release-0-4-27-cleanup`
2. List open PRs across 4 product repos (mcpdev80/{baseharbor,baseharbor-demo,baseharbor-console,baseharbor-node-connector})
3. List local branches older than 14 days across 4 repos (origin/ + local branches)
4. Categorize each item into: needs-rebase / superseded / still-open-legit / stale-close-candidate
5. Write evidence table with counts per category
6. END Q42-V27CLEANUP-DONE (with counts per category)

---

## Task card: t-20261017-release-0-4-27-cleanup

- **State:** active
- **Owner:** q42
- **Priority:** high
- **Depends on:** t-20261017-release-0-4-26-verify → NOT accepted
- **Handoff status:** Not started
- **Goal:** Reconcile previous-release leftovers before new development

## Open PR Inventory

### baseharbor (mcpdev80/baseharbor)

| PR # | Title | State | Branch | Created | Category |
|------|-------|-------|--------|---------|----------|
| 1043 | v0.4.26 selector predecessor: qualification entries for frozen S39/S42 (Q) | OPEN | work/selector-q-pr | 2026-10-23 | still-open-legit |
| 1042 | fix(ci): resolve bare-SHA dispatch inputs before atomic gate checkout | OPEN | fix/atomic-bare-sha-dispatch | 2026-10-22 | still-open-legit |
| 1041 | docs(v0.4.26): correct release notes + changelog dates and attribution | MERGED | docs/v0426-notes-corrections | 2026-10-22 | — |
| 1039 | fix(ci): data-capabilities gate step budget 8->25 min per q21 recipe | OPEN | fix/v0426-stepcap-split | 2026-10-22 | still-open-legit |
| 1038 | fix(cli): plan authoritative zero-impact envelope for Edit preview | MERGED | fix/edit-refusal-preview-line | 2026-10-22 | — |
| 1037 | o14 test form3 | CLOSED | o14-test-form3 | 2026-10-22 | superseded |
| 1032 | feat(core-921): integrate reviewed MCP bootstrap and public application edit proposals | CLOSED | integration/core-921-mcp-bootstrap-edit-proposal | 2026-10-22 | superseded |
| 1023 | test(runtime): qualify reviewed Docker healthcheck fixtures on isolated validation branch | CLOSED | validation-source/docker-healthchecks-reviewed | 2026-10-21 | superseded |
| 1014 | ci(core-921): qualify corrected frozen binding against current trusted base | CLOSED | work/core921-binding-corrected-frozen | 2026-10-21 | superseded |

All other PRs (#1015–#1040 except above) are MERGED.

### baseharbor-demo (mcpdev80/baseharbor-demo)

| PR # | Title | State | Category |
|------|-------|-------|----------|
| 118 | fix(demo): cleanup-provenance ordering snapshot before teardown (o23) | MERGED | — |
| 117 | chore(demo): pin Core C 2adae47a in baseharor-core.ref | MERGED | — |
| 116 | Integration: v0.4.26 new-edit native guided + heavy fixtures | MERGED | — |
| 115–101 | All remaining PRs | MERGED | — |
| 106 | test: declare recovery workload exposure contract | CLOSED | superseded |
| 104 | test: converge final v0.4.19 acceptance gates | CLOSED | superseded |
| 89 | test: align v0.4.19 atomic guided contract | CLOSED | superseded |

No open PRs. All are either MERGED or already CLOSED.

### baseharbor-console (mcpdev80/baseharbor-console)

| PR # | Title | State | Branch | Created | Category |
|------|-------|-------|--------|---------|----------|
| 7 | Console v0.4.26 consumption path: rebuild pending patch semantically | OPEN | work/v0426-console-consumption | 2026-10-22 | still-open-legit |
| 3 | v0.4.23: protected Core transport and live adapter integration | OPEN | feature/v0.4.23-live-integration | 2026-10-06 | stale-close-candidate |
| 6, 5, 4 | All other PRs | MERGED | — | — | — |

PR #3 (`feature/v0.4.23-live-integration`) targets `main` from Oct 6 and is superseded by the v0.4.26 consumption work in PR #7. It is a stale-close-candidate.

### baseharbor-node-connector (mcpdev80/baseharbor-node-connector)

| PR # | Title | State | Branch | Created | Category |
|------|-------|-------|--------|---------|----------|
| 8 | v0.4.26: connector consumer freeze (Core pinned, Demo/Console PENDING-FREEZE) | OPEN | work/v0426-connector-consumption | 2026-10-22 | still-open-legit |
| 4 | v0.4.23: Core enrollment and remote lifecycle integration | OPEN | feature/v0.4.23-core-integration | 2026-10-06 | stale-close-candidate |
| 7, 6, 5, 2 | All other PRs | MERGED | — | — | — |

PR #4 (`feature/v0.4.23-core-integration`) targets `main` from Oct 6 and is superseded by the v0.4.26 consumption work in PR #8. It is a stale-close-candidate.

## Branches older than 14 days

Checked `refs/heads/` in all 4 repos via `git clone --mirror` + `git for-each-ref`. All branches were created within the last 14 days. **No branches older than 14 days found.**

## Categorization Table

| Repository | needs-rebase | superseded | still-open-legit | stale-close-candidate |
|------------|:-----------:|:----------:|:----------------:|:-------------------:|
| baseharbor | 0 | 4 | 3 | 0 |
| baseharbor-demo | 0 | 3 | 0 | 0 |
| baseharbor-console | 0 | 0 | 1 | 1 |
| baseharbor-node-connector | 0 | 0 | 1 | 1 |
| **TOTAL** | **0** | **7** | **5** | **2** |

### Details per category

**needs-rebase (0):** No items found that need rebasing. All open PRs target `develop` or `main` and appear current.

**superseded (7):** Already-closed PRs that were superseded by merged work:
- baseharbor: PRs #1037, #1032, #1023, #1014 (4 items)
- baseharbor-demo: PRs #106, #104, #89 (3 items)
All are already CLOSED — no action needed.

**still-open-legit (5):** Open PRs with legitimate active work targeting current release branches:
- baseharbor: PR #1043 (work/selector-q-pr), PR #1042 (fix/atomic-bare-sha-dispatch), PR #1039 (fix/v0426-stepcap-split)
- baseharbor-console: PR #7 (work/v0426-console-consumption)
- baseharbor-node-connector: PR #8 (work/v0426-connector-consumption)

**stale-close-candidate (2):** Open PRs from Oct 6 targeting `main` that are superseded by newer v0.4.26 consumption work:
- baseharbor-console: PR #3 (feature/v0.4.23-live-integration)
- baseharbor-node-connector: PR #4 (feature/v0.4.23-core-integration)

Both target `main` directly, were created Oct 6 (17 days ago), and are superseded by the newer v0.4.26 consumption PRs (#7 and #8). Suggested action: close these PRs with a reference to the v0.4.26 consumption PRs.

---

## END Q42-V27CLEANUP-DONE

Total inventory:
- Open PRs across 4 repos: 8 (5 still-open-legit + 2 stale-close-candidate)
- Closed PRs (superseded): 7
- PRs merged: 51
- Branches older than 14 days: 0
- Categories: needs-rebase=0, superseded=7, still-open-legit=5, stale-close-candidate=2

