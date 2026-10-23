# O72 — Full Audit-Finding → Merge-SHA Closure Matrix

Lane: `o72-auditfull` · Tag: `[wip:o72]` · Recorded: 2026-10-23 UTC
Mode: **READ-ONLY evidence.** No card-status changes, no product writes, no CI triggered.
Reference baseline: baseharbor develop HEAD `1861ed07888a1ec22c218ca491d23ee54f48e261` (C2-final).

## PLAN

- [x] 0. Read the dispatched prompt (`prompts/o72.txt`) and binding policies; establish this read-only audit plan.
- [x] 1. Refresh remote product state and pin develop HEAD `1861ed07` (verified live via `gh`); pin the F21 closure memo as the source of findings.
- [x] 2. For every original audit finding (q8 numbering, q19 live mapping, per `f21-defectclose.md`): resolve the fix PR / merge SHA, verify `gh pr view` mergeCommit, and run `git merge-base --is-ancestor <sha> 1861ed07` for IN-C2.
- [x] 3. Build the line-by-line matrix (no aggregate claims); classify each as in-c2 | open | superseded.
- [x] 4. Close: append `END O72-AUDITFULL-DONE` with per-status count and open-remnants list.

Tool note: native TodoWrite/todowrite used for lane tracking. No product edits; all verification is read-only git/gh.

## Inputs and reproducible checks

- GitHub authenticated identity: `mcpdev80` (`gh api user --jq .login`).
- Baseharbor develop HEAD (live, twice): `1861ed07888a1ec22c218ca491d23ee54f48e261`.
- Planning main HEAD (live): `c41cd2cf5d9d2ed7ba18bc346c0cd1ee4f2148e8` (task at remote `review`, owner `Codex (Maintainer complete v0.4.26 defect audit)`, blocker empty; old OWNER STOP is historical data, not resumption authority).
- Finding set source: `f21-defectclose.md` rows 1–32 (q8 numbering), cross-referenced with `q8-defect-audit.md`, `q19-defect-ledger.md`, and verdicts `board-2026-10-19..22`.
- For each baseharbor PR: `gh pr view <n> -R mcpdev80/baseharbor --json state,mergeCommit,mergedAt`; ancestry: `git -C baseharbor merge-base --is-ancestor <sha> 1861ed07888a1ec22c218ca491d23ee54f48e261` (returned 0 ⇒ IN-C2).
- Console / connector / demo PRs checked live via `gh pr list`/`gh pr view` against `mcpdev80/baseharbor-{console,node-connector,demo}`.

## Full audit-finding → merge-SHA matrix (rows 1–32)

| # | Finding (q8) | Fix-PR / merge reference (SHA-exact, live-verified) | Ancestry / state | Status |
|---|---|---|---|---|
| 1 | Core921 CLI/MCP registration incomplete | #1026 → `fe2d0c4a8c1df2d15058e338abb78adeb4cc9b83` (merged 2026-10-21T15:04:51Z) | IN-C2 | in-c2 |
| 2 | Isolated Podman disaster recovery | #1027 → `213c15e43ac9f3c333ad5eabed202475eefb501f` (2026-10-21T15:18:52Z) | IN-C2 | in-c2 |
| 3 | Independent exact Docker qualification | #1028 → `00882c3c35dc1b1822095bec286cb5519fed53bc` (2026-10-21T15:34:39Z) | IN-C2 | in-c2 |
| 4 | Native-proven Docker integration | #1029 → `30e112431b04a7e24e7e7e9ea76a3600e439a988` (2026-10-21T15:43:35Z) | IN-C2 | in-c2 |
| 5 | #998 MCP schema/typed-error | superseded by #1026 (`fe2d0c4a` IN-C2) | supersede-source IN-C2 | superseded |
| 6 | #995 Qwen native recovery | superseded by #1027 (`213c15e4` IN-C2) | supersede-source IN-C2 | superseded |
| 7 | #990 Docker isolation acceptance | superseded by #1028 (`00882c3c` IN-C2) | supersede-source IN-C2 | superseded |
| 8 | Docker native8 PASS | run 39619782116 (retired-CI id); source `30e11243` IN-C2 | source IN-C2 | in-c2 |
| 9 | Podman recovery real native | run 39605258769 (retired-CI id); source `213c15e4` IN-C2 | source IN-C2 | in-c2 |
| 10 | Public MCP bootstrap candidate | #1030 → `7fbc1ec51dee853c57783ce29f4d350e259bbdff` (2026-10-22T14:02:08Z) | IN-C2 | in-c2 |
| 11 | 3 source candidates integration | #1033 `3be56628ad635e353bd5d783679cb651edc82818` / #1035 `078fdd0a57092f2649c92c93df901a70b3d2b11f` / #1036 `2adae47afddc299d06dbc34eb16a8e011c14b6dc` — all IN-C2 | all IN-C2 | in-c2 |
| 12 | EN/DE docs + release-note tree | #1031 → `103f58b185f0b0b91a5ac1eed14ffb048235f308` (2026-10-22T16:48:07Z); correction #1041 → `9f4916be333d0f99d7e40001441d9dbb87e993e2` (2026-10-23T07:20:10Z). Patch `0c8a26c8` NOT on develop — superseded by `103f58b1` | both IN-C2 | in-c2 |
| 13 | Private combined Core preview | no fix-PR; local composition `041c274f` — acceptance item, no develop reference | no develop ref | open (acceptance-gap) |
| 14 | Demo review corrections | demo #116 → `ee62eb2ffe65` / #117 → `896dc3cb68a8` / #118 → `82d042bc9b23` — all IN demo main (merged 2026-10-22/23) | IN demo main | fixed-in-demo |
| 15 | Demo SQL fixture blocker | demo #116 `ee62eb2f` (is_postgres_server change; `git log -S` verified) | IN demo main | fixed-in-demo |
| 16 | Demo cleanup foreign sentinel | demo #118 `82d042bc` + `8b26585` | IN demo main | fixed-in-demo |
| 17 | Demo native Docker/Podman fixture run | fixtures integrated via `ee62eb2f`; final native qualification receipts are gate evidence, no separate fix-PR | no fix-PR | open (acceptance-gap) |
| 18 | Console candidate freeze | console #7 **DRAFT** (`work/v0426-console-consumption`, 2026-10-22T14:48:03Z) — live `gh` check | not merged | open |
| 19 | Immutable consumer pin chain | connector #8 **DRAFT** (`work/v0426-connector-consumption`, 2026-10-22T15:03:04Z); demo pin `896dc3cb` IN main (pins `2adae47a`); demo #119 CLOSED unmerged | not merged | open |
| 20 | Final 65-gate atomic catalogue | gate execution, not code — no fix-PR; runs on `1861ed07` required | no fix-PR | open (acceptance-gap) |
| 21 | CI transport limitation | resolved by attempt2 reruns (`00882c3c`, `30e11243` IN-C2); CI fixes #1039 `49d396c48d2dee8b65cd59497c9f74692954975c` / #1043 `5be7dabd5c40a99e1623eb1a72cae943f9e0470d` / #1044 `1861ed07` — all IN-C2 | all IN-C2 | in-c2 |
| 22 | K3s scope from release gates | scope decision from release gates — no fix-PR | no fix-PR | open (acceptance-gap) |
| 23 | Final Docker eight-family test on final develop head | source `30e11243` IN-C2; final-head run on `1861ed07` outstanding | source IN-C2 | open (acceptance-gap) |
| 24 | Final Podman/recovery on final develop head | source `213c15e4` IN-C2; final-head run on `1861ed07` outstanding | source IN-C2 | open (acceptance-gap) |
| 25 | Go 1.26.9 binary equality verification | no develop fix-PR; verification item | no fix-PR | open (acceptance-gap) |
| 26 | MCP bootstrap source qualification | develop-side `3be56628` IN-C2; cited `f762f448` = #1033 PR-branch tip, NOT on develop | source IN-C2 | in-c2 (SHA corrected) |
| 27 | Terminal single-No input loss | #1035 → `078fdd0a57092f2649c92c93df901a70b3d2b11f` (2026-10-22T17:53:59Z) | IN-C2 | in-c2 |
| 28 | Pre-execution Edit-result accuracy | #1036 → `2adae47afddc299d06dbc34eb16a8e011c14b6dc` (2026-10-22T17:56:42Z) | IN-C2 | in-c2 |
| 29 | core-922/core-923 cards | core-922 docs merged `103f58b1`+`9f4916be` IN-C2; core-923 active (owns gaps 13,17,20,22,23,24,25 + 18/19) | — | open (core-923 active) |
| 30 | 65-gate catalogue (q7 duplicate) | same as #20 | — | open (acceptance-gap) |
| 31 | Invented docs PR (governance rewrite) | RESOLVED by decision — no PR existed/published | no PR | resolved-no-code |
| 32 | SQL layout #1004/#1005 | `19548bf0`, `b4a5334` — both IN-C2 | both IN-C2 | in-c2 |

## Per-repository live PR summary

- **baseharbor** (develop): #1026/#1027/#1028/#1029/#1030/#1031/#1033/#1035/#1036/#1039/#1041/#1043/#1044 all MERGED, mergeCommit IN-C2 (ancestry returned 0). #1037 CLOSED (unmerged, `o14-test-form3`, base main — unrelated).
- **baseharbor-console**: #7 `work/v0426-console-consumption` **DRAFT** (not merged, finding 18); #8 `f25/console2-live-workflow-proofs` OPEN (unrelated CI scope).
- **baseharbor-node-connector**: #8 `work/v0426-connector-consumption` **DRAFT** (not merged, finding 19).
- **baseharbor-demo**: #116 `ee62eb2f` / #117 `896dc3cb` / #118 `82d042bc` all MERGED on demo main; #119 CLOSED unmerged.

## Status count

- **in-c2 / fixed**: 15 rows (1, 2, 3, 4, 8, 9, 10, 11, 12, 21, 26, 27, 28, 32 → in-c2; 5, 6, 7 → superseded-via-IN-C2; 14, 15, 16 → fixed-in-demo).
- **superseded**: 3 rows (5, 6, 7).
- **open** (code not merged or acceptance item outstanding): 14 rows (13, 17, 18, 19, 20, 22, 23, 24, 25, 30, plus 29 as the active owner card; 31 resolved-no-code).

Totals: 32 findings — 18 covered by verified develop/demo SHAs (in-c2 + fixed-in-demo + superseded-via-IN-C2), 3 superseded, 11 open remnants (code-defect residue under core-923; acceptance-gaps requiring final-head `1861ed07` runs; console/connector consumer-freeze drafts #7/#8).

## Open remnants (line-by-line, not established in-c2)

- 13 — Private combined Core preview: acceptance item, no develop fix-PR. Next trigger: Core preview acceptance on `1861ed07`.
- 17 — Demo native Docker/Podman fixture run: gate evidence still owed (no separate fix-PR).
- 18 — Console candidate freeze: console #7 DRAFT → promote to mergeable + merge before closure.
- 19 — Immutable consumer pin chain: connector #8 DRAFT → freeze + merge; demo #119 pin `1861ed07` still unmerged.
- 20/30 — Final 65-gate atomic catalogue: gate run on `1861ed07` required (not code).
- 22 — K3s scope: release-gate decision artifact; no code residue.
- 23 — Final Docker eight-family test on final develop head: run on `1861ed07`.
- 24 — Final Podman/recovery on final develop head: run on `1861ed07`.
- 25 — Go 1.26.9 binary equality verification: verification item.
- 29 — core-923 owns all of the above gaps + 18/19; proposal: stay `active`.

## Closure

Every finding that has a develop/demo fix reference is established line-by-line against `1861ed07` (baseharbor) or demo main — no aggregate claim is made. Residue is confined to the 11 open remnants above; code-defect closure is established for the 18 covered rows. No card was moved; all proposals belong to the Maintainer/Owner (core-923 active, audit task `blocked` by Owner stop).

END O72-AUDITFULL-DONE
