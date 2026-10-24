# Connector #10 — conditional merge gate

ROLLE: MERGE — Owner-authorized conditional merge; dependency graph checked first.
UTC started: 2026-10-24T09:51:50.294837+00:00
UTC final capture: 2026-10-24T09:52:50.531510+00:00
Account verified: `mcpdev80` only.

## Owner mandate

Mission `ops/missions/cmxconn.txt`, Marcel 2026-10-24, states:

> `--squash --admin` gilt fuer baseharbor UND die Consumer-Repos (demo, console, node-connector) - Ziel: Ecosystem-Freeze, damit pre-release.yml:117/:122 und W1 durchkommen.

The mission explicitly makes merging conditional on checking final Demo/Console dependencies first and requires BLOCKED if the impending upstream merges will require different pins. This specific mandate was evaluated; no additional Owner approval was requested. The earlier coordinator policies were read, and no broader coordinator activation or release authority was inferred. No required CI/protection was bypassed.

## PLAN

- [x] 0. Read coordinator policies and cmxconn.txt; record conditional Owner mandate and this plan. The mission defines no explicit PLAN step 0. Native TodoWrite is unavailable; this evidence file is the persistent checklist.
- [x] 1. FIRST inspect actual Demo/Console main tips, pending merges and #10's immutable bindings. Result: final-main dependency prerequisite is not fulfilled.
- [x] 2. Record BLOCKED at the dependency gate. Capture unchanged HEAD/file hashes and existing checks as evidence; defer full merge-eligibility/protection audit because no merge may proceed.
- [x] 3. Stop before merge; recheck remote #10 state and connector main; record dependency order and suggest #9 supersession without closing it. Post-merge steps are inapplicable because no merge was attempted.

## Outcome

**BLOCKED — Demo and Console final-main dependency freeze is outstanding.** #10 remains OPEN/unmerged. No `gh pr merge`, admin override, push, code correction, CI rerun, label, workflow dispatch or PR closure was executed.

## Dependency graph checked before any merge action

| Component | #10 immutable binding | Actual remote main | Pending source / result |
| --- | --- | --- | --- |
| Core | `1861ed07888a1ec22c218ca491d23ee54f48e261` | Not compared to moving Core main: C is the canonical immutable anchor | Correct Core C; not the blocking field |
| Demo | `25c319697fd3d89d894943e0f44cd3d5dde8b198` | `82d042bc9b2393733858fd5fc9dd42a7f5491ea9` | #120 OPEN/unmerged at `bd86c7c3cbf7ebfb6dc5631c917ab244c5ed79c5`; final merged D not available |
| Console | `69d1d14007fa43dc0c0457c3347280be7f802818` | `e7d1e4a1d603158dece19bbc25a8e267d07cde5e` | #9 OPEN/unmerged at `6017de2845fa3e4e408c69038a0a5bdd7e82cad9`; still needs final D, then final merged U |
| Connector | PR #10 `e832d5651752121dea9484e18422d3e6459f9a72` | `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e` | Must consume final D/U before this final-main freeze merge |

Demo and Console main were queried with the mission's exact `gh api repos/mcpdev80/<repo>/commits/main --jq .sha` commands. Binding locations are Connector `integration-candidate.json:4` (Core), `:5` (Demo) and `:6` (Console).

**Historical-pin exception considered:** `codex-verdict-connector10.md` accepts the existing values as a valid historical snapshot, but explicitly says this does not complete the final-main ecosystem freeze. Its merge-readiness recipe says to avoid intermediate merges and repin in Demo → Console → Connector order. It does not select D0/U0 as the final immutable release inputs instead of the pending upstream main merges. Thus the review's source acceptance is not sufficient to exercise the mission's historical-final-choice exception. The mission also specifically instructs not to merge when Demo #120 will change the final Demo main pin. This is the observed situation; no new approval question is needed to follow that stop instruction.

## Candidate and affected-file identity before/after

| Item | Before = after |
| --- | --- |
| #10 HEAD | `e832d5651752121dea9484e18422d3e6459f9a72` |
| #10 target/base | main / `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e` |
| `integration-candidate.json` SHA-256 | `0915a4a8e4f8c31c3bf07c6f122b28bf301b55f3e94e3b34c7b5459073f219a0` |
| Connector remote main | `7c7093d9671dfaa1aafeb92d62bbe9d074f9f47e` |
| PR state | OPEN, `merged=false` |

Affected-file bytes were fetched at the exact immutable HEAD through GitHub contents twice; hashes match the independent source review at this exact HEAD. No foreign worktree was read or changed. There is no new merge commit or new connector main SHA.

## Existing checks observed

`gh pr checks 10 --repo mcpdev80/baseharbor-node-connector` reports:

| Check | Result | Run/job |
| --- | --- | --- |
| source | PASS | Run [40081652928](https://github.com/mcpdev80/baseharbor-node-connector/actions/runs/40081652928), job 120836283698 |
| Actual encrypted transport | SKIPPED | Same run, job 120836304169 |
| Managed native enrollment | SKIPPED | Same run, job 120836284408 |

Prior independent review records run attempt 1, exact `head_sha=e832d5651752121dea9484e18422d3e6459f9a72`, GitHub Actions app 15368. This session reused that identity evidence and observed the current PR check summary; it did not rerun or relabel checks. Source PASS does not override the failed dependency prerequisite or turn skipped native checks into acceptance. Since the first merge prerequisite failed, no complete final live-rules/merge-path audit was performed and no claim of overall merge readiness is made.

## Recommended continuation, without executing it

1. Resolve Demo #120's own findings and verify its final candidate. When allowed, merge it and record actual final Demo main D, including any separately needed runtime implementation before declaring final D.
2. Console #9 `integration-candidate.json:5` must become D, while `:4` stays C=`1861ed07888a1ec22c218ca491d23ee54f48e261`. Obtain fresh exact-candidate review/checks, merge when allowed and record actual final Console main U.
3. Connector #10 `integration-candidate.json:5–6` must become D/U; `:4` stays C. Preserve its owner's branch/history. Obtain new exact-SHA independent review, applicable green checks and final live rule/ownership/acceptance audit, then use an expected-HEAD guard for any authorized squash merge. Record actual final connector main K. This existing review cannot automatically cover the repinned candidate.
4. Reconcile baseharbor #1050's `docs/releases/v0.4.26.consumer-pins.json` from immutable C/D/U/K. Primary fields at lines 44/49/56/64 become C/D/U/K; repeated Demo refs at 59/67, Console refs at 31/68 and Connector refs at 7/14/21/25/35 must match. Preserve common Core C at 51/58/66; do not introduce Core self-reference. Verify source-owned graph and required checks/evidence.

Connector #9 remains a superseded source candidate per the independent #10 review: #10 preserves its Demo/Console edits and fixes its wrong Core pin. **Suggestion only:** have the coordinator close #9 as superseded by #10; this session did not close it or send a comment. No post-merge log is claimed because the merge did not occur.

## Evidence

`codex-merge-connector10-data/` retains the live main-tip queries, PR metadata, candidate content, before/after hashes and check summary. No Planning acceptance or release state was changed. Native TodoWrite could not be used because that tool is not exposed.

END CX-CONNECTOR10-MERGE-BLOCKED — final Demo/Console main pins unavailable; required order demo → console → connector.
