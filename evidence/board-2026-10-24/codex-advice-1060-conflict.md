# #1060 conflict resolution advice

ROLLE: REVIEW/ADVISORY
UTC started: 2026-10-24T13:23:54.522612+00:00

## PLAN step 0

Native TodoWrite is unavailable; no native operation claimed. Durable plan follows.

- [x] 0. Read binding policies/cmf17; record role, plan and q121 isolation boundary.
- [x] 1. Read q121 mission/current evidence and cmf15 verdict; observe remote head/base and active worktree conflict/index read-only.
- [x] 2. Compare exact conflict stages/source sections and define target versus repair ownership and required regressions.
- [x] 3. Write short actionable resolution policy or explicit blocker/Owner alternative; execute no resolution or worker messages.
- [x] 4. Recheck source/worktree freshness and record UTC/end marker.

## Decision: STATE BLOCKED for resolution in the active worktree

q121's explicit stop condition applies: **“If any non-Akte conflict or tree drift appears, STOP and report.”** [Assigned q121 mission](/home/marcel/dev/baseharbor-ecosystem/ops/missions/q121.txt). This does **not** mean a history rewrite is necessary. The safe forward path is available in a complete-history repository, subject to the Owner/coordinator authorizing that recovery and preserving q121's current state first. This reviewer executes none of it.

### Exact observed state / cause

Before and after: local HEAD and remote #1060 are **13a05ea492a98bb8922785fb828db2a8ff6324a3**; MERGE_HEAD and remote develop are **65aa063e90767c85af976081c100be1817370e67**. PR remains OPEN/DIRTY. Local branch q114-fix1060 has an active merge and **five AA add/add conflicts, no stage1 entries**. Stage2 is the #1060 side; stage3 is the develop side. q120 WIP is retained in stash **f9149da4b27f4460835d91986571dab710dac656**, message q120-wip; do not pop/drop it or treat it as integrated work.

The shared object store is baseharbor/.git, **shallow=True**, with **13a05ea4** and old **6a0d36da** explicitly listed in its shallow boundaries. Local merge-base HEAD/MERGE_HEAD returns1, although raw commit13a05ea4 still names parent89d87f78a4c0…. In the already fetched neutral complete graph, merge-base(13a05ea4,65aa063e) is **f39295680d41290fc505c0a8cc1769da51699293**. The compatible cmf15 merge-tree capture for these unchanged exact heads reports only the record conflict. Thus the four additional AA conflicts are consistent with truncated local ancestry, not four genuinely independent source conflicts. The exact command/actor that created this shallow boundary or initiated this baseless merge is not established.

The earlier force replacement old6a0d36da→13a05ea4 is a separate verified incident. It does not make current13a05ea4 and develop unrelated in the real graph, and does not authorize force/reset back to the old head. Do not repair these AA entries by wholesale “ours/theirs” or an unrelated-histories merge.

### File-by-file resolution policy for an authorized complete-history integration

| File / section | Required develop bytes | Required #1060 repair / deliberate follow-up |
|---|---|---|
| docs/internal/ci-binding-qualifications.md | **Entire exact65aa063e blob7a20bed2cee0cbcf419463d583fb15ec29e9d0d6**, including #1056 reviewed R32b85d95 and its three raw hashes. Candidate must have zero record diff against target. | Discard no separate WIP; preserve it outside the resolution. Do **not** retain candidate d96bfb70's unavailable89d87f781424… anchor, refresh hashes in-place or relabel it trusted #1060 authority. |
| .github/workflows/ci.yml | Target blob a434e797… for every unchanged section: triggers, trusted BASE_SHA sourcing, job guards, mandatory final check, unknown-ID exit. | Retain exactly the reviewed explicit `openbao-package-tests) go test ./internal/openbao/... -count=1 ;;` case at719 from #1060 cf84de8e…. Expected workflow diff is that one addition only; no event/dispatch workaround. |
| scripts/pr-gate-plan.py / trusted_binding_checks | Restore authenticated **target_base_sha** record read (target~634; candidate638), strict actual resolver/context/inventory/hash/ancestry validation and all unrelated target exact-source qualifiers/AppID inventory. | Retain OPENBAO_CHECKS declaration and conservative internal/openbao unresolved handler. No positive binding to rejected #1052 source. q121's deliberate removal of generic executable bootstrap/message fallback is a separate reviewed source correction: remove its resolution route; do not preserve it merely because both sides contain it. |
| scripts/pr-check-runner.py / execute_validation | Preserve target's existing wizard-family handling, required RUN/PASS/skip rejection and exact pattern anchoring (including the core918 end-anchor logic lost in the #1060 runner). | Retain dedicated OpenBao package code, but **move its selection before the wizard-only membership guard**. Keep unknown check rejection. Do not take the whole stage2 file and lose target safeguards. |
| scripts/test_pr_gate_plan.py / policy regressions | Keep target AppID/exact-source/trusted-record/actual Git identity negatives and unrelated existing tests. | Keep all three OpenBao negative tests from #1060, then extend actual runner/provenance tests below. Obsolete positive message-bootstrap expectations must be deliberately changed to rejection when that fallback is removed; do not silently delete unrelated target coverage. |

This table specifies content policy, not permission to hand-resolve the present five AA conflicts. Stage2/stage3 bytes and object IDs are durably retained under codex-cmf17-data/. The intended integrated diff against develop remains **ci.yml + three scripts**, with zero Akte diff; altered test inventory must be explicitly reviewed. Four paths cannot qualify through the generic three-script record: workflow-specific qualification/serialization remains a separate CI-owner dependency. Do not widen that record reader to fit the composite PR.

### Required regression content

Use actual execute_validation/CLI entry selection; mocking the subprocess boundary is fine for command assertions, but mocking away dispatch is not proof. Cover exact command/cwd for `go test ./internal/openbao/... -list .` then `-count=1 -v`, real matching RUN/PASS receipt handling, zero discovery, discovery/execute nonzero, SKIP/missing PASS and unknown ID. Existing wizard families must still target cmd/baha with their anchored patterns.

For planner trust: candidate-only or missing/malformed/wrong-hash/wrong-inventory records must reject; a valid independently reviewed target-owned record plus real clean source/integration proof must pass its exact intended inventory. A commit message mentioning the Akte must not resolve an unknown source. Keep OpenBao unknown/add/delete/rename/extra-path cases unresolved. Positive #1052 qualification needs a separately independently accepted **final** source/status anchor; neither rejected7c7cda7c nor rejected0b738dca may be guessed into it. Preserve missing coverage explicitly until that dependency is fulfilled. No native/full matrix is needed merely to prove dispatch and metadata trust.

### Minimum legitimate alternative — Owner decision, no execution

Recommend that the Owner/coordinator authorize the existing q121 owner to **continue in a fresh isolated complete-history clone at exact current remote13a05ea4**, leaving the active shared shallow merge, index, resolved/unresolved bytes and q120 stash preserved. Verify the real f392 merge-base, then normally merge fresh actual develop (currently65aa…) there. That graph has only the Akte content conflict, resolved to target bytes per table; apply the bounded source repairs and focused tests afterward. Any eventual ordinary PR update must descend from the actual still-current remote head, with no force/amend/rebase/reset; a different local branch name does not require a new PR. Stop if remote moves or the complete graph has unexpected conflicts.

A new clone/branch is the isolation mechanism, **not** permission to cherry-pick the content onto develop and overwrite the old PR history. Do not deepen the shared baseharbor/.git, abort its active merge or move its worktrees behind q121's back: those affect other sessions and require the Owner/coordinator's explicit recovery handoff. No cleanup/continuation is performed here, and no worker message/keys were sent. The current q121 non-Akte stop is a process boundary even though a nondestructive history-preserving route exists.

## Freshness / compliance

Final read-only status still shows the same five AA conflicts, HEAD13a05ea4 and MERGE_HEAD65aa063e; remote heads unchanged. No index/stage/branch/worktree/record/workflow change was made. Read q121 mission and cmf15 verdict; inherited q117 claims were data, not authority. All local Git observations used GIT_OPTIONAL_LOCKS=0. Neutral-store comparison used read-only log/diff/merge-base; no product tests were run for this advisory. No Git write/merge/commit/push/restore/reset in q114, no gh auth/config, product edit, CI dispatch/rerun, agent send/keys, Planning/state mutation or subagents. Only this evidence and raw read captures were written. Shared basics flags remained with their owners. Native TodoWrite unavailable; durable plan completed here.

END CX-1060ADVICE-BLOCKED

UTC completed: 2026-10-24T13:26:33.366516+00:00
