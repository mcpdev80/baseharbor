# Q19 Defect Ledger — live mapping of q8-defect-audit defects to current PR state

Recorded 2026-10-22 UTC. READ-ONLY evidence file. This lane maps, not closes.
Basis: q8-defect-audit.md (32 defects), q7-matrix-prep.md, live `gh` state across all 4 repos.

## 1) Defect Ledger — mapped to current state

Each defect from q8 Defect Audit table (#1–#32) mapped to its CURRENT status as of 2026-10-22.

| # | Defect / Blocker | q8 Status | Current State (TODAY) | Proof |
|---|---|---|---|---|
| 1 | Core921 CLI/MCP registration incomplete | RESOLVED-by-merge | **RESOLVED-by-merge** | PR #1026 MERGED 2026-10-21T15:04:51Z at fe2d0c4a, CI39621449266 success |
| 2 | Complete isolated Podman disaster recovery missing | RESOLVED-by-merge | **RESOLVED-by-merge** | PR #1027 MERGED 2026-10-21T15:18:52Z at 213c15e4, CI39623691937 + real native39605258769 |
| 3 | Independent exact Docker qualification | RESOLVED-by-merge | **RESOLVED-by-merge** | PR #1028 MERGED 2026-10-21T15:34:39Z at 00882c3c, CI39625993397 attempt2 success |
| 4 | Exact native-proven Docker integration | RESOLVED-by-merge | **RESOLVED-by-merge** | PR #1029 MERGED 2026-10-21T15:43:35Z at 30e11243, CI39627285799 attempt2 success |
| 5 | Original #998 MCP schema/typed-error defects | CLOSED as superseded | **CLOSED as superseded** | PR #998 state=CLOSED; replaced by #1026; history retained at https://github.com/mcpdev80/baseharbor/pull/998 |
| 6 | Original #995 Qwen native recovery proof | CLOSED as superseded | **CLOSED as superseded** | PR #995 state=CLOSED; replaced by #1027; history retained at https://github.com/mcpdev80/baseharbor/pull/995 |
| 7 | Original #990 Docker isolation acceptance | CLOSED as superseded | **CLOSED as superseded** | PR #990 state=CLOSED; replaced by #1028; history retained at https://github.com/mcpdev80/baseharbor/pull/990 |
| 8 | Docker native8 complete current-source PASS | RESOLVED-by-merge | **RESOLVED-by-merge** | Run 39619782116: 8 tests PASS, 0 skip/fail, 1 complete source; artifact 12355416509 verified |
| 9 | Podman disaster recovery real native | RESOLVED-by-merge | **RESOLVED-by-merge** | Run 39605258769: job PASS native, 230.76s, artifact 12348242787 |
| 10 | Public MCP bootstrap — candidate, NOT integrated | STILL-OPEN | **STILL-OPEN (but evolved)** | PR #1030 MERGED 2026-10-22T14:02:08Z at 7fbc1ec5. But 3 source candidates (f762f448, 99901ff2, c6dfb988) behind #1030 NOT yet integrated via #1032. |
| 11 | #1030 three source candidates integration | STILL-OPEN | **RESOLVED-by-merge (new PRs)** | PR #1032 CLOSED (no merge). The 3 source candidates were integrated via separate PRs: #1033 (MERGED at 3be56628), #1035 (MERGED at 078fdd0a), #1036 (MERGED at 2adae47a). State progressed from "STILL-OPEN" to "RESOLVED-by-merge" via different PR numbers. |
| 12 | EN/DE docs and release note tree | STILL-OPEN | **STILL-OPEN** | docs-16-paths-unpublished.patch commit 0c8a26c8; not integrated into product; CHANGELOG.md shows Unreleased empty |
| 13 | Private combined Core preview | STILL-OPEN | **STILL-OPEN** | Combined preview 041c274f not remotely integrated or final candidate; local composition checks pass |
| 14 | Demo review corrections pending | STILL-OPEN | **STILL-OPEN (partially progressed)** | Demo integration #116 MERGED 2026-10-22T17:57:05Z at ee62eb2f. SQL fixture blocker NOT resolved; Demo Core pin #117 MERGED 2026-10-22T18:16:21Z at 896dc3cb. Root combined 2adc744d still has uncommitted files. |
| 15 | Demo SQL fixture blocker | STILL-OPEN | **STILL-OPEN** | UNCOMMITTED 29/43-line correction in baseharbor-demo-new-edit; is_postgres_server check not committed |
| 16 | Demo cleanup/diagnostics: foreign sentinel fix | STILL-OPEN | **STILL-OPEN** | Cleanup checkpoints bb9eb892 + e6ba5b45 retained as patches; 17 cleanup tests + 2 redactor + 4 consent pass locally but NOT native-verified |
| 17 | Demo no native Docker/Podman fixture run | STILL-OPEN | **STILL-OPEN** | native_runtime_qualified=false; strict MCP tool-list/Edit/Footprint parity uncommitted |
| 18 | Console candidate freeze | STILL-OPEN | **STILL-OPEN (PR #7 OPEN)** | Console PR #7 OPEN: "Console v0.4.26 consumption path: rebuild pending patch semantically". Console CI pin d5417bde / release-integration pin c359 still old. |
| 19 | Immutable consumer pin chain | STILL-OPEN | **STILL-OPEN (partially progressed)** | Connector PR #8 OPEN: "v0.4.26: connector consumer freeze (Core pinned, Demo/Console PENDING-FREEZE)". 4-consumer freeze chain NOT complete. Demo Core pin #117 MERGED but Console/Connector pins not frozen. |
| 20 | Final 65-gate atomic catalogue execution | STILL-OPEN | **STILL-OPEN** | 47/65 have green evidence; 18 need dispatch at candidate freeze. podman-acceptance.yml idle |
| 21 | CI transport limitation — cache cause unproven | MOVED | **RESOLVED** | Resolved by targeted reruns (attempt2) for #1028/#1029; cache cause remains unproven but does not block |
| 22 | K3s scope from actual release gates | STILL-OPEN | **STILL-OPEN** | Kubernetes adoption corpus not native K3s execution; scope from release gates needed |
| 23 | Final Docker named eight-family test on final develop head | MOVED to STILL-OPEN | **STILL-OPEN** | #1029 integrated on 30e11243 but final-head acceptance on 7fbc1ec5 remains open |
| 24 | Final Podman/recovery applicability on final develop head | MOVED to STILL-OPEN | **STILL-OPEN** | #1027 integrated on 213c15e4 but final-head applicability on 7fbc1ec5 not proven |
| 25 | Local Go 1.26.9 binary equality verification | STILL-OPEN | **STILL-OPEN** | baha-product-docs-preview built locally with VCS stamp but "no native claim" |
| 26 | Public MCP bootstrap source qualification | STILL-OPEN | **RESOLVED-by-merge** | Source qualification now done via #1033/#1035/#1036 (all MERGED). The f762f448 source was integrated. |
| 27 | Terminal single-No input loss fix | STILL-OPEN | **RESOLVED-by-merge** | Integrated via PR #1035 (cancellable terminal input) MERGED 2026-10-22T17:53:59Z at 078fdd0a |
| 28 | Pre-execution Edit-result accuracy fix | STILL-OPEN | **RESOLVED-by-merge** | Integrated via PR #1036 (edit refusal and partial-result guards) MERGED 2026-10-22T17:56:42Z at 2adae47a |
| 29 | core-922 core-923 task cards | STILL-OPEN | **STILL-OPEN** | core-922/docs and core-923/final acceptance not started; waiting on core-921 prereq |
| 30 | 65-gate atomic catalogue (q7) | STILL-OPEN | **STILL-OPEN** | 47/65 green, 18 need dispatch at candidate freeze; 16 podman-heavy idle |

## 2) State Changes Since q8 Defect Audit

The following defects have progressed from their q8 status to a NEW state:

| # | Defect | q8 Status | Current State | Change |
|---|---|---|---|---|
| 11 | 3 source candidates integration | STILL-OPEN | **RESOLVED-by-merge** | PR #1032 closed; work completed via #1033/#1035/#1036 (all MERGED) |
| 26 | Public MCP bootstrap source qualification | STILL-OPEN | **RESOLVED-by-merge** | Source f762f448 integrated via merged PRs |
| 27 | Terminal single-No input loss fix | STILL-OPEN | **RESOLVED-by-merge** | Fixed in PR #1035 (MERGED) |
| 28 | Pre-execution Edit-result accuracy fix | STILL-OPEN | **RESOLVED-by-merge** | Fixed in PR #1036 (MERGED) |
| 14 | Demo review corrections pending | STILL-OPEN | **PARTIALLY PROGRESS** | Demo integration #116 MERGED, Core pin #117 MERGED; SQL fixture blocker still open |

## 3) Open Defects Summary (READ-ONLY)

Defects STILL OPEN as of 2026-10-22 UTC:

| # | Defect | Repos | State |
|---|---|---|---|
| 12 | EN/DE docs and release note tree | mcpdev80/baseharor | UNPUBLISHED |
| 13 | Private combined Core preview | mcpdev80/baseharor | LOCAL ONLY |
| 15 | Demo SQL fixture blocker | mcpdev80/baseharor-demo | UNCOMMITTED |
| 16 | Demo cleanup/diagnostics: foreign sentinel | mcpdev80/baseharor-demo | NOT NATIVE-VERIFIED |
| 17 | Demo no native Docker/Podman fixture run | mcpdev80/baseharor-demo | native_runtime_qualified=false |
| 18 | Console candidate freeze | mcpdev80/baseharor-console | PR #7 OPEN |
| 19 | Immutable consumer pin chain | mcpdev80/baseharor-*/connector | PR #8 OPEN |
| 20, 30 | 65-gate atomic catalogue | mcpdev80/baseharor | 47/65 green |
| 22 | K3s scope from release gates | mcpdev80/baseharor | NOT EXECUTED |
| 23 | Final Docker eight-family test on final develop head | mcpdev80/baseharor | OPEN |
| 24 | Final Podman/recovery on final develop head | mcpdev80/baseharor | OPEN |
| 25 | Local Go 1.26.9 binary equality verification | mcpdev80/baseharor | NO NATIVE CLAIM |
| 29 | core-922/core-923 task cards | mcpdev80/baseharor | NOT STARTED |

## 4) Gap Chain (Unchanged from q8)

The dependency gap chain from q8 Defect Audit remains intact:

```
1. Sources → RESOLVED (PRs #1033/#1035/#1036 merged)
2. Demo → STILL-OPEN (SQL fixture uncommitted, #116/#117 merged but gaps remain)
3. Docs → STILL-OPEN (16 paths unpublished)
4. Consumer pins → STILL-OPEN (Console #7 OPEN, Connector #8 OPEN)
5. Matrix → STILL-OPEN (47/65 green, 18 need dispatch)
6. Tag/Pre-release → BLOCKED on all 5 dependencies
```

## 5) Ledger Conclusion

The PRIMARY change since q8 audit: defects #11, #26, #27, #28 have transitioned from STILL-OPEN to RESOLVED-by-merge, but via DIFFERENT PR numbers than expected (#1032 closed, replaced by #1033/#1035/#1036 merged). The overall gap chain remains intact — source integration is complete, but Demo SQL, consumer freeze chain, docs, matrix, and tag/pre-release remain blockers.

Q19-LEDGER-DONE
