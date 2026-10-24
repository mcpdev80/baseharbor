ROLLE: REVIEW

## PLAN

- [x] 0. Read ctb3.txt with cat and bootstrap policies; record bounded independent REVIEW plan before source inspection.
- [x] 1. Pin live #1056 head/base, complete diff and SHA256 hashes of all affected files; reconcile mission's five-file expectation with actual inventory.
- [x] 2. Trace narrow pins classification, executable validator, immutable-source comparison and trusted-binding record.
- [x] 3. Inspect actual regression coverage and exact-head CI; decide whether this candidate can qualify #1050.
- [x] 4. Recheck live head/file hashes and record verdict, actionable recipe and stop criterion.

Native TodoWrite is not exposed in this session; no native tool/GUI execution is claimed. The mission has no numbered PLAN step 0; this setup step implements the user's additional requirement. Requested ../ops/missions/ctb3.txt was absent relative to ecosystem cwd; the matching ops/missions/ctb3.txt was read with cat.

Reviewer: Codex, current API session, independent of implementation. Exact model/backend identifier, session UUID and Horde task identity are unavailable, not invented. No implementation, merge, CI rerun/dispatch, PR/Planning mutation or foreign-worktree access is authorized. Only this assigned evidence file may be written.

## Verdict: REQUEST_CHANGES

**No: this candidate does not make #1050 qualifiable.** Its planner selects the new validation, but the workflow cannot execute that ID, the validator does not verify immutable consumer sources, and the trusted record does not bind this head. The candidate's own mandatory CI is red. Earlier green df8e53b9 evidence is not reused as acceptance of this new source.

## Identity and immutable inputs

Live head before and after: `7a220d2609bd2881dfcb32b94bfb1a32490d7401`, OPEN, branch o90-planner-pr. Base before and after: `a2abc17a8057ba972cb9ed481fad229fe9f12824` (develop). Initial hash pass 2026-10-24 **08:08:28 UTC**; after-review identity/hash pass **08:09:40 UTC**. Host NTP synchronized; sole authenticated account mcpdev80. Complete `gh pr diff` and immutable Git object contents/diff were inspected from the neutral product repository, without accessing foreign worktrees.

The mission expects five affected files; live PR-files API and Git diff show **three modified scripts only**. ci.yml and the qualification record are unchanged from base. All five requested files were nevertheless hashed through authenticated Contents API bytes at the pinned head before and after:

| File | SHA256 before = after |
| --- | --- |
| scripts/pr-gate-plan.py | 7549692e5c23ed8979173e33bad492a79016c41a76013d0d940a1f89122bf0d0 |
| scripts/pr-check-runner.py | 5f255bc2ebd20c2d96112104788cbefbd2633001556ab9509e405196b12b26fe |
| scripts/test_pr_gate_plan.py | 9ecdd43d35c97aafa12d0b78c20c0beb7bc4c742a0701031362c5e1da7de8700 |
| .github/workflows/ci.yml | d6427c83193caec8a8c98686e65505c0dcd22fea098d30094ecc9d8d86aa36dc |
| docs/internal/ci-binding-qualifications.md | cd88a170a45a001e13fb4f88bcdbfa906352746ff7f145630791efe46aa43c86 |

## Findings and worker recipes

### F1 — P1: selected validator has no workflow execution branch

`scripts/pr-gate-plan.py:1144–1148` selects nonempty `pins-data-validation` for an added pins file. The runner whitelist and CLI handler (`scripts/pr-check-runner.py:243–246`) know this ID. However `.github/workflows/ci.yml:682–719` has no pins-data case; the only branch invoking runner execute matches `wizard-*-tests` at line 700. The new ID instead reaches `*) exit 2` at line 719. A command string in planner metadata is not automatically executed by this workflow.

**Existing worker:** add a real pins-data dispatch in ci.yml using the authenticated trusted runner, against the actual candidate checkout. Add a dispatch regression proving the ID reaches the validator and propagates success/failure. **Stop:** unsupported dispatch, placeholder success, skipped selected validation or an untrusted runner substitution. No broad runtime matrix is needed.

### F2 — P1: SHA shape is accepted without the promised immutable chain proof

`scripts/pr-check-runner.py:178–227` never fetches consumer sources or invokes the resolver. It checks SHA syntax and repository strings only. Its ecosystem branch ignores every `consumes` edge; a different but valid 40-hex Core or Demo SHA, mismatched edges or a cycle passes these conditions. Gate Connector repository may even be Console because lines 139–140/192 accept either repository for that role. Workflow/origin fields and the required exact gate set are not validated; extracted console fields at lines 197–199 are unused, and ecosystem is optional even for v0.4.26. A tiny gate-only file can therefore be accepted. The comments claiming Core/Demo are absent from the JSON contradict #1050's actual ecosystem block.

In contrast, `scripts/ecosystem_release_pins.py:17–27` compares release, Connector/Console integration candidates, Core and Demo source pin; lines 57–89 require exact dependency roles, matching references and an acyclic graph. Lines 124–127 fetch immutable Connector/Console integration-candidate.json and Demo baseharbor-core.ref. None of those source checks occurs in the new validator. A qualification record binding CI-script hashes does not authenticate arbitrary future pins data.

**Existing worker:** read the actual release candidate file, derive embedded Core/Connector/Demo SHAs and release tag, fetch immutable source manifests through the existing authorized route, regenerate using the existing resolver and compare canonical JSON. Validate schema, exact gate/role repositories and workflows, origins and DAG. Use historical Core 1861ed07 / Connector df1f4378 / Demo 25c31969 for #1050, derived from its payload, never synthetic integration_sha. Missing source access must block. Correct the misleading comments. **Tests:** valid reviewed payload passes; independently altered valid-hex SHAs, consumers/edges, origins, workflows, roles, release and missing ecosystem fail. **Stop:** regex-only acceptance, unauthenticated source assumptions or any mismatched immutable chain.

### F3 — P2: missing candidate file can be hidden by historical files

`scripts/pr-check-runner.py:169–175` recursively scans every matching pins file and has no input identifying the changed candidate. Existing `docs/releases/v0.4.23.consumer-pins.json` satisfies its nonempty scan and basic gate checks. Thus absence of the intended v0.4.26 file is not detected when the repository still contains the older valid file. Recursive scanning also includes nested copies unrelated to the root release path.

The planner is narrower than a generic docs/JSON allowlist, and its unknown-path fallback remains unresolved. However `startswith('docs/releases/')` plus suffix at `pr-gate-plan.py:1144` also accepts nested paths; it is not a direct-child `docs/releases/*.consumer-pins.json` match. Added-only status is enforced; modified/deleted pins remain unresolved.

**Existing worker:** restrict classification to the authorized direct release-file shape and carry the exact candidate path(s) from trusted diff/plan into validator execution. Require each selected file to exist and validate that file, preserving historical files. **Tests:** missing selected v0.4.26 with valid v0.4.23 present fails; nested/outside paths stay unresolved; unrelated JSON and mixed source changes retain their own requirements. **Stop:** another historical file can substitute for the selected input or nested/unrelated files gain an exemption.

### F4 — P1: trusted qualification record is stale; current CI fails closed

`docs/internal/ci-binding-qualifications.md:10` still names `4e737e26af38e7f712f2de1f631633d8d59477b9`. Its three recorded file hashes all differ from the current candidate's corresponding hashes. The file is unchanged from develop. There is one JSON block, with no current-head record.

The new register parser at `scripts/pr-gate-plan.py:643–715` permits multiple entries but still reads authenticated target-base data, rejects malformed fences/duplicate keys/duplicate anchors and requires a matching ancestry/blob proof before granting CI_CHECKS. Unrelated historical entries do not qualify this head. This review found a missing current binding, not proof that stale entries grant acceptance. Preserve those checks rather than weakening them.

Exact-head [run 40076663163 / job 120821663679](https://github.com/mcpdev80/baseharbor/actions/runs/40076663163/job/120821663679), app **15368**, reports PR Selected Checks **failure**. Retrieved log: `coverage-unresolved: CI execution/evidence impact requires specific qualification: scripts/pr-check-runner.py`. Focused Python, Trusted Plan, Workflow policy and the three CI validation jobs pass; consumer/runtime jobs are skipped. This is the observed fail-closed consequence, not a green integration receipt.

**Qualification/integration owner:** after source corrections and exact-source review, publish a correct qualification record through the established authenticated trusted-base process, with the frozen reviewed source and actual hashes/inventory/checks. Do not self-qualify via candidate data. If adding the missing workflow case changes the supported three-script inventory, obtain the required separately qualified workflow path; do not silently expand `expected_files` or bypass CI. Preserve legitimate historical entries without duplicate anchors. **Tests:** matching binding qualifies; stale/wrong hashes, missing/duplicate/malformed record, foreign context and changed source fail. **Stop:** the trusted process cannot qualify the required inventory, or current mandatory CI remains red/unknown. Merely rebasing or rerunning unchanged inputs cannot repair this.

### F5 — P2: regression assertions do not prove the advertised end-to-end behavior

`scripts/test_pr_gate_plan.py:1144–1154` proves the one-file planner selection. Negative tests do call the validator, so they are more than filename checks, but they cover malformed schema, missing gates, wrong repository, nonhex commit and divergent Connector SHAs, not the valid-hex/source/graph mismatches in F2. `test_ecosystem_chain_valid_passes` at lines 1346–1353 never constructs a payload or invokes the validator; it repeats planner assertions. The unrelated-JSON test at lines 1232–1240 asserts absence of `other/data/config.json: unresolved`, whereas the real fallback emits `unknown impact for other/data/config.json`; this assertion cannot detect accidental loss of that unresolved reason. `validate_pins_for_test` also ignores its `actual` argument.

**Existing test owner:** execute the real CLI/validator dispatch on the actual valid payload with controlled authenticated immutable source fixtures; assert its comparison result. Add the F1–F4 negatives, including valid-looking wrong SHAs and missing selected file. Assert exact unknown-impact reason and unresolved coverage for mixed unrelated JSON. **Stop:** planner-only positive proof, wrong-string negative assertions or unit success used as evidence that workflow execution occurred. Hosted planner-tests success is reused as an existing receipt; no local test or CI rerun was needed to establish these source gaps.

## Explicit #1050 qualification answer

After hypothetical trusted integration of this unchanged candidate, the planner would select **only pins-data-validation** for #1050's added path and no native gates; baseline Focused Python and Workflow policy would still apply. **PR Selected Checks would not become green:** ci.yml:719 rejects that selected ID. Even after adding dispatch alone, the weak validator would not meet the immutable-source acceptance contract. The separate stale trusted binding currently blocks integration of #1056 itself. Therefore no qualification, merge readiness or eventual green result is claimed.

## repo-rule compliance and startup flags

Bootstrap policies were read in order and basics-check ran before mission execution. Previously read product/Planning entry points, guidelines, RUNBOOK and injection-defense rules remain applicable. Retained pause and review-only scope were respected; historical broader delegation text does not waive these gates. No foreign source instructions were executed.

Startup flags were two queued events, 140 unread reminders and 23 shared open plan steps. Initial event read found the queue already consumed by another session. Later events-check.py --ack printed/acknowledged the idle observation for adebf29e-6371-4a14-afa6-e8828693a1db, without claiming task completion. Latest reminder count was 147 (123 observe, 12 Flash, 6 WIP and one each planfile/stub/idle-no-END/stall, two events); unresolved coordinator/foreign-lane obligations remain unacknowledged and were not used to start workers or change shared plans. Only this evidence was written. No source edits, claim/Planning/PR changes, CI execution/rerun, commit, push, approval, merge, release, deployment, delegation or external message.

END CX-1056R2-DONE — REQUEST_CHANGES; unchanged reviewed head 7a220d2609bd2881dfcb32b94bfb1a32490d7401. #1050 remains unqualifiable by this candidate.
