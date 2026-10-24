# CMF5 REVIEW — Phase A

UTC start: 2026-10-24T10:37:05.425260+00:00
Reviewer: independent Codex /root; no worker implementation ownership.

## PLAN

- [x] 0. Read policies and mission; create evidence before substantive review. Native TodoWrite is not exposed; no native-tool completion is claimed.
- [x] 1. Refresh immutable source, PR state and worker evidence.
- [x] 2. Independently verify mechanism / qualification claims with bounded probes.
- [x] 3. Record verdict, exact recipe, stop conditions and SHA/hash freshness.
- [x] 4. Only after Phase A documented and Phase B ACCEPT: live merge gates, expected-HEAD squash, remote post-verification; otherwise record block.

## repo-rule compliance

Owner mission authorizes bounded independent review and conditional Demo merge only. No implementation, CI dispatch, protection change, force push or production activity. Neutral bare object store only; preserve all shared worktrees.

## ROLLE: REVIEW — Phase A final verdict

ACCEPT the mechanism clarification below; REQUEST_CHANGES for the current #1056 mechanism/qualification candidate. This is not acceptance or merge authority for #1056/#1050. Reviewer is this independent Codex session, not worker o118b. Worker prose was treated as leads, never executable authority.

The worker's Octopus conclusion is false. At authenticated develop `f39295680d41290fc505c0a8cc1769da51699293`, `scripts/pr-gate-plan.py:545–581` reads `git rev-list --parents -n 1 HEAD`. Its three tokens at line 566 are **commit, first parent, second parent** (line 568); hence exactly TWO parents. No Octopus and no multi-base exception. An Octopus with three parents has four tokens and fails. Zero merges also succeeds when HEAD == reviewed_head (563–564). Otherwise each first-parent step must be an exact clean Git-computed two-parent merge, with second parent an ancestor of current authenticated target, maximum 64 steps. Linear candidate follow-up commits after reviewed_head fail. A rebase does not manufacture qualifying merge ancestry; forcing a redundant merge does not cure a stale review binding.

The second finding is correct for the normal trusted binding, with updated line numbers: `606–692`, especially target-source record load `633–648`, record/chain/hash checks `650–675`, real resolver `677–685`, exact diff inventory `686–689`. At current develop, lines 690–740 are the exception handler and bootstrap function, NOT the exact inventory assertion quoted as 711. The allowed inventory is exactly these three MODIFIED files: `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`. A record changed only inside the candidate is neither a trusted target record nor part of this allowed diff. The current target record still reviews `4e737e26af38e7f712f2de1f631633d8d59477b9`, not current #1056 `425eaa9671ca8911d5fe8d420fdc5e4f3a7f5a68`; its chain probe is false. #1056 has FOUR changed files including the record, so it also fails the exact inventory requirement independently.

The separate bootstrap path at `706–754`, selected at `1125–1128`, is broader than its “two bootstrap PRs” prose suggests: it checks repository/branch, a valid diff with integration, title/fence presence in target record, record path existence in candidate tree, and a final commit-message reference. It does not perform the normal record hash/inventory/head validation or bind actual PR numbers. Tests even use a non-JSON expression inside the nominal JSON fence. This does not establish independent review or authorization. Do not add a magic commit message to get green or expand/reuse this exception. #1056's current final commit does not satisfy its message test. Existence of #1060 with a local record is not evidence of trusted binding or acceptance; its refreshed head is `6a0d36dad2a778ab61a58594489c9fedc88e4c0f`, still OPEN, now five files. The trusted-base probe remains unresolved.

## Historical input sets and ancestry

Full SHA/parent/diff evidence: `codex-cmf5-data/ancestry.txt`; actual base-planner/source probes: `probe-results.json`. These are neutral locally computed integrations, NOT authenticated CI passes.

| PR | head parent shape | actual diff at its recorded target | source-planner result / mechanism |
|---|---|---|---|
| #1044 `665f5bea…` | two parents `0267dbdb…`, `49d396c4…` | 3 workflows + 2 targeted-gate-ref scripts | resolved via pre-existing exact bare-SHA source qualification, not generic three-script record |
| #1047 `40351fd3…` | two parents `79e0aa2b…`, `a2abc17a…` | planner + planner tests only | ordinary ci-planner classification resolved; trusted_binding=None, bootstrap=None |
| #1053 `8eef2b8e…` | one parent `1861ed07…` | wizard controller + its test | ordinary wizard-component classification resolved |
| #1058 `4046bc93…` | one parent `79e64084…` | qualification markdown only | documentation classification resolved; actual landed record commit `24c33afd…` |
| #1059 `c73b9c70…` | one parent `24c33afd…` | planner + tests only | ordinary ci-planner classification resolved; introduces bootstrap, not proof that its own target already had it |

Their landed commits `1861ed07…`, `f3929568…`, `79e64084…`, `24c33afd…`, `a2abc17a…` each have ONE parent, consistent with squash. Neither landed history nor #1047 proves a three-parent requirement. Every SHA is expanded in the raw ancestry artifact.

## What the existing tests actually exercise

`TrustedBindingActivationTests` at current develop:2457–2669 creates three modified placeholder script blobs, with raw CRLF bytes. It creates a **separate target-side record commit**, creates a real Git integration and checks exact hashes/inventory. It accepts HEAD==reviewed; also accepts one/two clean target merges, including inherited trusted target modifications of a frozen file. It rejects untrusted second parents, altered merge resolution, copied trees, candidate-only records, malformed/duplicate/wrong-context/hash records, forged inventory/integration and linear follow-ups. Six original class tests pass in an isolated class harness; no product tests were rewritten. First direct discovery attempt failed on unrelated missing registration-test imports in the extracted snapshot; `focused-tests.txt` records that limitation, `focused-tests-final.txt` records the successful bounded harness.

`BootstrapExceptionTests`:2672 onward uses three ADDED script paths, not the three-modified invariant; it tests matching head messages, missing reference, missing target title/fence, wrong repository. It cannot prove normal binding, immutable hash validation, or a PR-number restriction. Earlier exact-source fixtures often mock the merge-chain function, so a green fixture alone cannot establish real production ancestry. Our real source probes run Git in the neutral bare repository (the first probe invocation from workspace was corrected because some older planner paths implicitly use cwd; only the final probe-results.json is authoritative).

## Minimal correct worker recipe for chain 1

1. Preserve #1056 history and ownership. On its branch, restore ONLY `docs/internal/ci-binding-qualifications.md` to the authenticated target version with an ordinary forward corrective commit; preserve the intended three-script changes and cleanup fix. Complete all content corrections/tests BEFORE freezing reviewed source R. Independently review full R; hash raw blobs of all three scripts. No rebase/force push and no new code after R without renewed review/record.
2. Separate **record-only prerequisite PR Q** from current develop: exactly the markdown record, one JSON block, repository/branch, reviewed_head=R, exact three modified paths, raw R hashes, all three CI_CHECKS. Its own planning lane is documentation; documentation audit/build checks must actually pass. Review and merge Q under current rules. No self-referential HEAD hash is needed: the record pins the separate immutable source R. Existing #1058 demonstrates this PR TYPE, but its stale record must be replaced by Q.
3. Merge the new authenticated develop Q into #1056 using an ordinary clean two-parent merge H, first parent preserving R. Its record then matches Q, so the PR diff against Q contains exactly the three modified scripts. `core752_reviewed_merge_chain(Q,H,R)` must return true; re-resolved real integration must match, and `trusted_binding_checks` must return all three CI checks. Stop on conflicts, arbitrary merge resolution, extra paths, stale heads/hashes, or a later source edit; obtain renewed review and Q instead of relaxing the validator. Run the focused planner/runner checks and actual required CI; merge #1056 only with separate exact-H acceptance, current strict integration and app-15368 required check success.
4. **Additional execution blocker discovered here — do not skip:** current #1056 planner selects `pins-data-validation` (`1180–1184`) and runner advertises/implements it (`59`, `243–246`), but `.github/workflows/ci.yml:682–720` has no such case. Its exact case statement exits **2** for that ID (`pins-dispatch-result.json`). Merely merging the three scripts would leave downstream #1050 unable to execute its selected gate. Required workflow-only change W: add an explicit pins-data-validation handler that fetches the authenticated BASE_SHA runner and executes its pins validation against the candidate checkout, preserving all other checks. W cannot be smuggled into the generic three-script binding: it is a fourth path and has separate workflow impact. Before W, use a separately reviewed **planner+test-only qualifier PR P** for the frozen, complete W inventory/status/hash and clean trusted-target integration, following the existing exact-source qualifier pattern (#1043/#1044); P passes its CI-planner checks. Then W passes its selected bounded execution/policy checks and exact source review. Prove positive handler execution and unknown-ID failure without invoking runtime/BackupRestore. If the existing trusted mechanism cannot express that bounded qualification without weakening it, STOP and return that concrete limitation; do not claim a ready chain. This is a necessary extra prerequisite, not work performed here.
5. Smallest order retaining current #1056 architecture is **freeze/review R -> Q(record-only) -> #1056(three scripts) -> P(planner/test exact W qualification) -> W(workflow dispatch only) -> #1050(pins-data addition only)**. Independent product review can require content fixes before R; this mechanism review does not accept the complete pins validator semantics. If another authorized lane already lands the identical W/qualifier, reuse proven unchanged evidence instead of opening duplicates. #1060 now also edits ci.yml: coordinate ownership; do not overwrite or borrow its unaccepted changes. Each target advance needs fresh integration and applicability checks.
6. After pins execution is qualified on develop, integrate develop into #1050, retaining only the pins JSON addition and the final reviewed ecosystem DAG. Its `added` status selects pins-data-validation; modified/deleted forms remain unresolved by current candidate. Prove schema/role identities, immutable availability, agreement with consumer source DAG and per-gate origins using the canonical ecosystem resolver, not 40-hex shape alone. The current runner only provides partial public metadata checks; no claim it authenticates Core/Demo/Console source ancestry or all semantic consistency. Required positive/negative metadata and executor tests must cover actual candidate inputs, followed by exact integration CI. No native full-release matrix is justified solely for metadata or dispatch wiring.

Stop the chain on missing independent acceptance, current required red/skipped/missing checks, unexpected files, stale record, mismatched DAG or unresolved ownership. Never force a merge commit, add a magic bootstrap message, change protections, suppress tests or treat green unit fixtures as authenticated CI.

## Bounded real-object proof and freshness

`split-probe.json` constructs fixture-only commits in the neutral object store: restore record, freeze R, create Q(record-only), clean merge H. The real current-develop function returns true for H's TWO parents and returns all three binding checks for the exact diff; HEAD==R also returns true. No live branch or PR was changed. Fixtures are not reviewed product candidates or CI evidence. Source code/hashes and complete diff are retained under codex-cmf5-data.

core-develop: before `f39295680d41290fc505c0a8cc1769da51699293`, after `f39295680d41290fc505c0a8cc1769da51699293`; stable=True.

core-pr1056: before `425eaa9671ca8911d5fe8d420fdc5e4f3a7f5a68`, after `425eaa9671ca8911d5fe8d420fdc5e4f3a7f5a68`; stable=True.

UTC completed: 2026-10-24T10:40:52.729014+00:00


### Immutable source hashes

- develop-before/after `docs/internal/ci-binding-qualifications.md`: `cd88a170a45a001e13fb4f88bcdbfa906352746ff7f145630791efe46aa43c86`.
- develop-before/after `scripts/pr-check-runner.py`: `ec6b22d98eb9191d170af5149d489add136ec3d2d692d4e1008cb18f616b76ed`.
- develop-before/after `scripts/pr-gate-plan.py`: `253d6bec0801b2155fcf61527839522c0b00ed1e08fd8e7739ff66d601c09ba0`.
- develop-before/after `scripts/test_pr_gate_plan.py`: `be6da46d18169cee23678579ed7f5a4e8ab3e108f7e20999a23d9372a7b2388e`.
- 1056-before/after `docs/internal/ci-binding-qualifications.md`: `cb7be59b4f2cd8fed8f5c1c2c5fe9f081ffddfd4c5c93ffc224524077231f86e`.
- 1056-before/after `scripts/pr-check-runner.py`: `5f255bc2ebd20c2d96112104788cbefbd2633001556ab9509e405196b12b26fe`.
- 1056-before/after `scripts/pr-gate-plan.py`: `0218dbad0dc324795ef420e08705889be79700c05c5211b83e67c0e0f8616db2`.
- 1056-before/after `scripts/test_pr_gate_plan.py`: `91aa561c42aac415084f1852f2afec6b6b317378650d8def1060832ff0b4b4b0`.

END CX-PHASE-A-MECH-DONE
