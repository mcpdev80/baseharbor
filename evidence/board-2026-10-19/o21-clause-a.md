# o21 — clause (a) content-hash anchoring for PR #1007 binding-PR clause

Date: 2026-10-20
Role: Flash (Coordinator) — implementation by Ornith
Repo under study: `mcpdev80/baseharbor` (worktree `worktrees/ci-core921-coverage-binding`)
Branch: `work/ci-core921-coverage-binding` · pushed head `36e4f991` (local `36e4f991`)
  · prior head `a756c361` superseded (its `scripts/repro_clause_a.py` was a
  4th changed file that broke the clause's path-count guard; moved to
  `evidence/board-2026-10-19/scripts/repro_clause_a.py`, repo now carries exactly
  the three qualified files)
Basis: Codex DESIGN RULING (a) in `codex-verdict-core921-round3.md` (rejection of (b) = merge-parent fabrication; refusal of (c) = PR-controlled policy / `Q2` base tampering) + `r4-planner-mode-analysis.md`.

---

## 1. What clause (a) is

The `BINDING_PR_1007` clause in `scripts/pr-gate-plan.py` (the `#1007` self-qualifying binding-PR branch) now qualifies **iff all of**:

1. **(i)** the PR diff against the target base equals exactly the 3 reviewed CI-execution scripts with the reviewed statuses (`scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`, all `modified`);
2. **(ii)** the **content** of each of the 3 files **at `head_sha`** equals its pinned content sha256 (blob contents via `git cat-file blob`, **not** commit shas — this avoids the self-reference paradox Codex flagged for (b));
3. **(iii)** the linear head's **merge-base with the target base is exactly that base along a clean first-parent line** (`core752_binding_linear_ancestry`: head has exactly one 1st parent and `merge-base(head, target) == target`), i.e. no merge fabrication.

Every other clause in `pr-gate-plan.py` is **byte-identical** to `db45db83` (r5). The only edits are: the new `BINDING_PR_1007_REVIEWED_CONTENT_SHA256` dict, the `core752_binding_content_sha256()` helper, and the binding-clause condition + its comment.

## 2. Constants (re-pin = 2-line change)

Two constants move together; re-pinning the reviewed head after #998 final lands is a 2-line edit (the head line + the 3-line content dict):

- `BINDING_PR_1007_REVIEWED_HEAD = '48def9e85e35db90025b4e354350bac22c9fe806'` (unchanged)
- `BINDING_PR_1007_REVIEWED_CONTENT_SHA256 = { … }` (new; pinned blob contents at `48def9e8`)
- `BINDING_PR_1007_LINEAR_PARENT = '706b524489f8410091902e595df50427f80a4eab'` (unchanged)

`CORE_921_PRODUCTION_REVIEWED_HEAD = 'a66e5de664b22c295a8b5ae918a9c2dfe4c6e327'` is **unchanged** (per task note; the tiny re-pin to the #998 final head follows later with #998).

### sha256 list (blob contents at `48def9e8`, verified against the worktree)

```
ec6b22d98eb9191d170af5149d489add136ec3d2d692d4e1008cb18f616b76ed  scripts/pr-check-runner.py
57cdefa1457ac51019cfe6468b14af1f7ade265df9ece79cd40ba784f1d2616d  scripts/pr-gate-plan.py
679fdef9770ed6e2bc35fe81fc4bae0fea23e1cf7ae1e2525824b3ee8ffe2b6e  scripts/test_pr_gate_plan.py
```

## 3. Planner self-repro (CI-equivalent inputs, head = reviewed head `48def9e8`)

`scripts/repro_clause_a.py` builds a clean base-anchored linear fixture carrying the exact reviewed content, resolves it through `pr-diff-base.py`, and runs `pr-gate-plan.plan`. Output:

```
== binding-PR #1007 content-hash pins ==
   BINDING_PR_1007_REVIEWED_HEAD = 48def9e85e35db90025b4e354350bac22c9fe806
   BINDING_PR_1007_LINEAR_PARENT = 706b524489f8410091902e595df50427f80a4eab
   scripts/pr-check-runner.py     pinned=ec6b22…  blob=ec6b22…  [ok]
   scripts/pr-gate-plan.py        pinned=57cdef…  blob=57cdef…  [ok]
   scripts/test_pr_gate_plan.py   pinned=679fde…  blob=679fde…  [ok]

== CI-equivalent PR #1007 diff inputs ==
   target_base_sha = <fixture develop tip>
   head_sha        = 48def9e8 (reviewed head, bound by the clause)
   integration_sha = <integration commit>
   merge_base_sha  = <fixture develop tip>   (== target base, clean first parent)
   changed files:  scripts/pr-check-runner.py [modified],
                   scripts/pr-gate-plan.py [modified],
                   scripts/test_pr_gate_plan.py [modified]
   broad_required = False

== pr-gate-plan plan result ==
   coverage_status = RESOLVED
   unresolved_coverage = []
   broad_required = False
   selected_gates = []
   reasons = ['reviewed #1007 binding-PR CI-execution self-qualification: exact source hash, complete changed status set and linear head whose single parent is an ancestor of the target base only']
   required_validation_checks = ci-executor-verifier-tests, ci-policy, pr-planner-tests

RESULT: RESOLVED through BINDING_PR_1007 content-hash clause (a).
```

## 4. Test suite (all green)

New tests in `scripts/test_pr_gate_plan.py`, class `BindingPR1007ContentHashAnchoringTests`:

- `test_exact_content_set_and_base_anchored_lineage_resolves` — **positive**: fixture head carries the exact reviewed blob contents (verified equal to the pins), branches from the target base → `resolved`, `CI_CHECKS` required.
- `test_same_files_different_content_stays_unresolved` — **negative**: same 3 files / statuses, but blob content at head ≠ pinned sha256 → `unresolved`.
- `test_off_base_lineage_stays_unresolved` — **negative**: content matches the pins, but the linear head's single 1st parent is a side commit (not the target base) → `core752_binding_linear_ancestry` returns False → `unresolved`.

Existing `test_binding_pr_1007_resolves_on_linear_head_and_rejects_other_heads` updated to exercise the content-hash + ancestry proofs on the real fixture (both patched to the fixture repo).

Totals:

| suite | ran | result |
|---|---|---|
| `test_pr_gate_plan.py` | 68 | OK |
| `test_pr_registration_ci.py` | 9 | OK |
| `test_pr_diff_base.py` | 13 | OK |
| `test_local_qualification` (+ others) | 26 | OK |
| `test_pr_required_final` | 10 | OK |
| `test_pr_runner_isolation` | 10 | OK |
| `test_pr_runtime_request` | 5 | OK |
| `test_pr_selected_runtime_matrix` | 5 | OK |

Known **pre-existing** failure (present on `db45db83`, unrelated to this change): `test_pr_verified_atomic_evidence` and `test_pr_wizard_workflow_route` fail to import `scripts/pre-release-resume.py` (file absent from the repo). Not introduced and not in scope.

## 5. Rule compliance

- **Codex ruling (a) honored; (b)/(c) rejected.** (b) — no extra merge parents fabricated; `core752_binding_linear_ancestry` still rejects any head whose single parent ≠ the base or that carries a 2nd parent. (c) — no `Q2` override, no base rewrite, no PR-controlled policy checkout; the anchor stays the immutable reviewed head + pinned blob contents on the trusted side.
- **Self-reference paradox avoided.** The content pin is the **blob** sha256 (`git cat-file blob`), not the reviewed head's own commit sha, so the pin cannot reference the commit it anchors and remain forgeable.
- **Fail-closed.** Content mismatch OR non-base-anchored lineage OR wrong file set/status each independently yields `unresolved`; `CI_CHECKS` are required, never waived.
- **r5 byte-identity.** Only the binding clause, its comment, the new content dict, and the new helper changed; every other reviewed-source clause is unchanged.
- **Re-pin hygiene.** `BINDING_PR_1007_REVIEWED_HEAD` and `CORE_921_PRODUCTION_REVIEWED_HEAD = a66e5de6` retain current values; the content dict is structured so re-pin = editing the head + dict lines.

## 6. Commits / push

- `2c616116` ci(core-921): bind binding-PR clause to content-hash anchored reviewed source (#1007)
- `36e4f991` chore(ci): keep binding PR to exactly three qualified files — pushed to `origin/work/ci-core921-coverage-binding`
- Repro re-run against `36e4f991` (`scripts/repro_clause_a.py` run from `evidence/board-2026-10-19/scripts/` with ROOT pointed at the worktree scripts dir): `coverage_status = resolved`, 3-file set, `CI_CHECKS` required — see §3 output.

Evidence refs: `codex-verdict-core921-round3.md`, `logs/codex-r3-raw-pane-capture.txt`, `r4-planner-mode-analysis.md`, `baseharbor/AGENTS.md`, `baseharbor/docs/DEVELOPMENT_GUIDELINES.md`.

O21-CLAUSE-A-DONE (implementation, repro, tests, push complete; pending Codex 6.1 final acceptance — board status move to `review` is the maintainer/acceptance step).

## 7. Ruling 3a guard set — trusted-base policy rewrite (DONE)

Codex ruling 3a forbids loading the reviewed head/content from the candidate PR
branch. The previous `core752_binding_linear_ancestry` anchor (inline
`BINDING_PR_1007_REVIEWED_CONTENT_SHA256` dict of blob sha256, plus the
`core752_binding_content_sha256` helper) reads the reviewed content *onto the
candidate* — exactly what 3a rejects. Rewritten so the clause loads its
qualification record exclusively from the authenticated target base revision.

Ruling guard set, mapped to the rewritten clause:

1. **Frozen reviewed head H.** `core_921` pin unchanged (`a66e5de6`).
2. **Record via reviewed prerequisite policy.** `scripts/ci-binding-qualifications.json`
   (repository, target_branch, reviewed_head, path/status inventory, full
   file hashes, required checks) is authored on the trusted target base, never
   on the candidate PR branch.
3. **Loaded only from authenticated base.** `load_binding_qualification_record(target_base_sha)`
   reads `git show <target_base>:scripts/ci-binding-qualifications.json` and
   fails closed (returns `None`) on unreadable blob / bad revision / unparsable
   JSON / missing fields (`_valid_binding_record`).
4. **head_sha == H OR clean trusted-target merge.** The binding head #1007 is a
   linear single commit, so the clause requires `identifiers['head_sha'] == record['reviewed_head']`
   (verified) and clean integration parents `core752_clean_integration_parents(target_base, head, integration)`
   (parents exactly `[target_base, head]`); no fabricated merge parents, no base rewrite.
5. **Complete diff inventory + reviewed content.** `_binding_inventory_matches`
   requires the full changed-path/status set to equal the record's `files` map
   (no extras, no renames/deletions); `_binding_content_hashes_verified`
   re-hashes each reviewed blob at `head_sha` and compares to the base pin.
6. **Preserved integration parents.** `core752_clean_integration_parents`
   enforces the two-field `[target_base, head]` contract.

Deleted: `BINDING_PR_1007_REVIEWED_CONTENT_SHA256` dict and
`core752_binding_content_sha256` helper (their reviewed content lived on the
candidate); retained: `BINDING_PR_1007_REVIEWED_FILES`, `core752_binding_linear_ancestry`.

- **Repro — both outputs (`evidence/board-2026-10-19/scripts/repro_binding_trusted_base.py`, run from `evidence/board-2026-10-19/scripts/` with the worktree scripts dir as arg):**
  - CASE 1 (record absent from base → current PR branch): `coverage_status = unresolved`,
    `reasons = ['scripts/pr-check-runner.py: ci-executor-unresolved', 'scripts/pr-gate-plan.py: ci-planner', 'scripts/test_pr_gate_plan.py: ci-planner']`.
  - CASE 2 (record loaded from authenticated develop): `coverage_status = resolved`,
    `reasons = ['reviewed #1007 binding-PR qualification via trusted base policy record: head equals the frozen reviewed candidate, complete diff inventory and content hashes verified, clean trusted-target merge with parents [target_base, head] only']`.
- **Re-run against new pushed head `e2060ea0` (repro moved to `evidence/board-2026-10-19/scripts/`, PR touches exactly the three qualified files):**
  - CASE 1: `coverage_status = unresolved`, `reasons = ['scripts/pr-check-runner.py: ci-executor-unresolved', 'scripts/pr-gate-plan.py: ci-planner', 'scripts/test_pr_gate_plan.py: ci-planner']`.
  - CASE 2: `coverage_status = resolved`, `reasons = ['reviewed #1007 binding-PR qualification via trusted base policy record: head equals the frozen reviewed candidate, complete diff inventory and content hashes verified, clean trusted-target merge with parents [target_base, head] only']`.
- **Tests — `test_pr_gate_plan.BindingPR1007TrustedBasePolicyTests` (6), all pass; full `test_pr_gate_plan.py` 68 OK.**
  New fixtures build the candidate head H on its own branch (blobs exist to hash),
  then add/omit the record on the develop target base and re-resolve against the new tip;
  fail-closed cases: absent record, mismatched `reviewed_head`, mismatched
  `file_hashes`, extra candidate file. `plan()` gained a `repository='.'`
  parameter threaded to the helpers; all existing callers unchanged.
- **r5 byte-identity.** Only the binding clause + new helpers changed; every other
  reviewed-source clause is byte-identical.
- **Re-pin hygiene.** `BINDING_PR_1007_REVIEWED_FILES` unchanged; the `core_921` pin
  stays `a66e5de6` (re-pin after #998 freeze is a separate 2-line commit — not combined here).

- `99d0749c` ci(core-921): bind binding-PR clause to trusted base policy (Codex ruling 3a) — pushed to `origin/work/ci-core921-coverage-binding`.

## repo-rule compliance

Only the binding clause and its new helpers in `scripts/pr-gate-plan.py` changed;
every other clause is byte-identical. `scripts/test_pr_gate_plan.py` replaced the
obsolete `BINDING_PR_1007_REVIEWED_HEAD` references (removed from the plan module
under ruling 3a) with parse-from-base fixtures; no other test touched. `plan()`
gained a `repository='.'` parameter threaded to helpers; all existing callers
unchanged. Full local verification green: `test_pr_gate_plan.py` 68 OK,
`test_pr_registration_ci.py` 9 OK, `test_pr_diff_base.py` 13 OK,
`test_local_qualification` (+ others) 26 OK, `test_pr_required_final` 10 OK,
`test_pr_runner_isolation` 10 OK, `test_pr_runtime_request` 5 OK,
`test_pr_selected_runtime_matrix` 5 OK.

## 8. Ruling 3a — DONE (implementation, repro, tests, push complete; pending Codex 6.1 final acceptance)

- `99d0749c` ci(core-921): bind binding-PR clause to trusted base policy (Codex ruling 3a).
- `e2060ea0` ci(core-921): keep binding PR to exactly the three qualified files — repro moved to
  `evidence/board-2026-10-19/scripts/repro_binding_trusted_base.py`; PR delta vs prior head touches
  exactly `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py` with
  `BINDING_PR_1007_REVIEWED_FILES` = 3 entries (`scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`,
  `scripts/test_pr_gate_plan.py`).
- Full local verification green against `e2060ea0`: `test_pr_gate_plan.py` 68 OK, `test_pr_registration_ci.py` 9
  OK, `test_pr_diff_base.py` 13 OK, `test_local_qualification` 3 OK, `test_pr_required_final` 10 OK,
  `test_pr_runner_isolation` 10 OK, `test_pr_runtime_request` 5 OK, `test_pr_selected_runtime_matrix` 5 OK.
- Board stays active (correction round of ruling 3a). Final acceptance move to `#status/review` is the
  Maintainer (Codex 6.1) step, not Flash's.

O21-RULING3A-DONE
