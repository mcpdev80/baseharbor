# ROLLE: REVIEW — frozen source preparation S39 / S42

## PLAN

- [x] Read binding policies; refresh product and verify full PR HEADs live.
- [x] Read both preparation diffs against prior tips and compare resulting inventories with the selector recipe.
- [x] Verify planner-file equality, residual qualification identifiers, Python syntax, YAML and preserved product blobs.
- [x] Publish per-source verdicts, worker correction recipe and stop criteria; recheck remote HEADs.

## Verdict

| Source | Verdict | Blocking reason |
| --- | --- | --- |
| S39 / #1039 `0de29debf4f91ee3de735ef6d2008eeec880bcdb` | **CHANGES** | Qualification tuple/test case remain after their constants were removed; test file has an IndentationError; timeout precedence remains wrong; planner files remain in PR inventory. |
| S42 / #1042 `267152e8ef1c8873bb1ec6518d4d714052ad8e10` | **CHANGES** | Qualification identifiers are gone and all five product blobs are preserved, but four residual blank lines leave two extra planner paths in the exact PR inventory. |

**Neither SHA is accepted as the final source anchor for Q.** Correct through the existing workers, freeze new SHAs and return for independent review before constructing the qualification entries. These are preparation verdicts, not merge/Planning/release acceptance.

## Review identity and immutable inputs

Reviewer: this independent Codex REVIEW session, not either implementing worker; no Horde task was created for this directly assigned review. Observed 2026-10-23 ~07:24–07:28 UTC. Read COST_CONTROL then GITHUB_AUTHORITY; product/Planning rules and the preceding selector recipe remain applicable. Authenticated account `mcpdev80`; product and Planning remotes refreshed. Both PR HEADs were queried again after inspection and remained the exact SHAs above.

| Input | Verified value |
| --- | --- |
| #1039 sole parent / prior reviewed tip | `a699510402dd64c57ae9e8d694f5287f2d051ff4` |
| #1042 sole parent / prior reviewed tip | `7bdd46fd7cba2a78c56eebe0da7076dc6a252321` |
| PR API base / source merge-base D0 | `2f005813dc6d7fa94de164388b2531d025af31bc` |
| Current fetched develop | `9f4916be333d0f99d7e40001441d9dbb87e993e2` |
| Accepted planner blob, D0 AND current develop | `98a9229c9126d8a6d242d44c4de523c69a6f843e` |
| Accepted planner-test blob, D0 AND current develop | `1a1eb3de903363677c95edac416c614551de9b57` |

Complete preparation diffs were read against the actual parents, and resulting PR inventories were compared against D0. Current develop's two planner files still equal D0 exactly; the newer target does not excuse these differences.

| Diff | SHA256 of `git diff --binary` output |
| --- | --- |
| S39 parent → source | `9cdb8788c37e7d59d4cc927353f4d7f318853587fb575e980192b20c485855bd` |
| S42 parent → source | `e12bc8e8104a6898ced4b3cae611cadb5dcf123e12ddfbec3bb57e87651f446f` |
| D0...S39 | `e6cf0d58248b9f591201625d890cc25ea2617735ce5a41583daba572cde677a9` |
| D0...S42 | `0c62cf92f355b65e69c99babeb4fc5910418f3e7f188655a5994e707e7e01e14` |

## S39 findings and worker recipe

Line references in this section are at S39.

1. **Blocking incomplete qualification removal:** `scripts/pr-gate-plan.py:749–751` still references `RUNTIME_STEP_CAP_REVIEWED_HEAD/FILES`, while their definitions are gone. The planner compiles, but evaluating this tuple in `plan()` encounters undefined names. `scripts/test_pr_gate_plan.py:1637` also retains the `RUNTIME_STEP_CAP` case. Thus the requested absence of branch-local qualification wiring is NOT met.
2. **Blocking Python syntax regression:** `scripts/test_pr_gate_plan.py:1581` has one extra leading space before the existing MCP assertion. Independent in-memory `compile()` of the exact Git blob fails with `IndentationError: unexpected indent (test_pr_gate_plan.py, line 1581)`. This also means the worker's completion note is not proof of a passing planner suite.
3. **Blocking timeout semantics:** `.github/workflows/pre-release-runtime-suite.yml:201` is now valid single-line YAML, but still tests `profile == 'data' && 20` BEFORE `gate == 'data-capabilities' && 25`. Demo `tests/gate-resources.json:78–80` assigns data-capabilities profile data; it therefore receives **20**, not the required **25**. This explicitly fails step 2 of the preceding recipe.
4. **Blocking source inventory:** both planner files still differ from accepted base, including an extra blank line at `scripts/pr-gate-plan.py:245`. Actual PR inventory is three modified files, whereas S39's qualification requires only the modified runtime-suite workflow.

**Existing #1039 worker executes:** inspect and preserve any concurrent work; restore ONLY `scripts/pr-gate-plan.py` and `scripts/test_pr_gate_plan.py` byte-for-byte to the accepted D0 blobs above. This removes the dangling tuple/case, indentation error and residual whitespace together. In the workflow expression at line 201, move the data-capabilities/25 clause before the general data/20 clause, preserving backup/recovery/security/default behavior. Batch the correction into one ordinary follow-up commit on this branch; no amend/rebase/force-push. Record the new full S39 SHA and evidence.

**Required proof / stop criterion:** both planner blobs equal the accepted hashes, neither file contains `RUNTIME_STEP_CAP` or `DISPATCH_GATE_REF`, complete PR diff is only the runtime-suite workflow, YAML/actionlint/policy pass, planner tests pass, and timeout cases prove backup=40, recovery=20, data-capabilities with data profile=25, other data=20, security=12, default=8. Do not reuse the rejected S39 SHA in Q. Worker evidence `q33-src1039.md` currently describes removals but provides no successful focused-test output; correct its completion claim and attach actual checks.

## S42 finding and worker recipe

Line references in this section are at S42.

**Blocking exact-inventory mismatch, not a style request:** all `DISPATCH_GATE_REF`/`RUNTIME_STEP_CAP` identifiers are absent from both planner files, and both files compile. However, new blank lines remain in `scripts/pr-gate-plan.py:245,749` and `scripts/test_pr_gate_plan.py:1581,1638`. Neither blob equals accepted D0. The PR therefore has **seven** changed paths, not the five source paths required by step 3. The exact path/status guard in accepted `pr-gate-plan.py:774–780` will reject that five-file qualification. These extra edits can also undermine the intended conflict-free import of Q; they must disappear before the anchor is frozen.

All five product blobs equal the prior tip byte-for-byte: modified `ci.yml`, `pre-release-runtime-suite.yml`, `targeted-demo-acceptance.yml`, and added `targeted-gate-ref.py` plus `test_targeted_gate_ref.py`. The preparation commit touched only the two planner files and preserved the real source implementation.

**Existing #1042 worker executes:** inspect and preserve concurrent work; restore ONLY the two planner files byte-for-byte to the accepted D0 blobs. This removes the four extra blank lines without changing the five product files. Make one ordinary follow-up commit, record the new full S42 SHA, hashes and focused evidence. No rebase/force-push; no product modification is requested by this preparation finding.

**Required proof / stop criterion:** accepted planner blob equality, no branch-local qualification identifiers, and exactly three modified workflows plus two added helper/test paths in the PR diff. Run the existing `python3 scripts/test_targeted_gate_ref.py`, YAML/actionlint/policy and planner checks as required by the preceding recipe; preserve prior still-valid evidence where the verifier proves identical inputs. A separate `q34-src1042` Markdown evidence file was not found in ecosystem or Planning evidence during this review; publish its actual checks/handoff rather than treating the idle-session notification as success. Do not pin the rejected S42 SHA in Q.

## Verification and CI disposition

Independent bounded reproduction read immutable blobs via `git show`, parsed both suite YAML files successfully and compiled both planner/test sources in memory: S39 planner PASS / test IndentationError; S42 both PASS. No product file or worktree was edited, and no broad test/native/CI run was launched by this reviewer. The data-capabilities precedence failure follows directly from the expression and its data profile; no native runtime success is claimed.

Live CI is not green:

- S39 [run 39917349905](https://github.com/mcpdev80/baseharbor/actions/runs/39917349905): `PR · Focused Python` FAIL, `PR Selected Checks` FAIL; trusted plan and workflow policy PASS. The YAML compilation blocker has been repaired; the source defects above remain.
- S42 [run 39917500677](https://github.com/mcpdev80/baseharbor/actions/runs/39917500677): focused Python and workflow policy PASS, `PR Selected Checks` FAIL. This does not establish the future five-file qualification or waive its inventory requirement.

After corrections, independently review the new exact source SHAs first. Then the qualification worker may prepare the separate two-file Q PR using those accepted anchors and source-only maps. Q itself still requires independent review and mandatory green CI before a separately authorized Maintainer merge. No ruleset adjustment, branch-side qualification or manual conflict resolution substitutes for these gates.

Turn-start flags were triaged within scope: pool deficits do not authorize new assignments; the coordinator PLAN already tracks S39/S42. q34 idle/missing-evidence events correspond to the evidence gap above and remain for its coordinator/owner; no unrelated event was falsely acknowledged as completed. No claims, Planning state, worker sessions or GitHub state were changed. Only this requested verdict file was written.

END REVIEW-S39S42-DONE
