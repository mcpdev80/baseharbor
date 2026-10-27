# R1 — Review PR1-Zweitblock: L4 (`29115985`+`cb01d090`) + L6 (`83d53e86`) on `084b3abc`

ROLE: REVIEW (read-only product; evidence only). UTC: 2026-10-26 ~22:10 +0100.
Reviewer: R1 (Codeman REVIEW session).
Candidate: commit `83d53e86` "ci(plan): complete exact path mapping for receipt control-plane sources",
branch `l6/ustrack`, base `084b3abc` (integrate: L3 policy expectations).
Candidate commit graph: `83d53e86` → `34159b1f` (integrate: L4) → `cb01d090` → `29115985` → `084b3abc`.
PR1-Zweitblock = L4 allowlist (`29115985`, `cb01d090`) + L6 mapping (`83d53e86`).

## VERDICT: FREIGEBEN (PR1-Akzeptanz nach grüner CI)

All five mission checks pass. The candidate is a self-contained, read-only-path-mapping
block: no check-trigger or runner logic is altered beyond the mapping, the two L4 files
are mutually consistent, all four required suites are green locally, and the complete
final inventory (all 11 path-classes) resolves with empty unresolved and no `broad_required`.

---

## (1) L6 mapping completeness vs CI-BLOCKED (Run 40399422956 / 11 path-classes)

The candidate maps every final receipt control-plane path to the smallest matching EXISTING
check. The 11 path-classes from the L6 lane map exactly as follows (from `l6-mapping-complete.md`
and verified by `scripts/test_pr_gate_plan_receipt_paths.py`):

| Path-class | Mapping target | Check(s) |
| --- | --- | --- |
| receipt_writer.py / receipt_verifier.py | pr-planner-tests (Focused Python) | existing |
| test_receipt_chain.py | pr-planner-tests (load_tests discovery) | existing |
| test_receipt_writer.py | pr-planner-tests (neu in load_tests-Discovery) | existing lane |
| test_receipt_verifier.py | pr-planner-tests (neu in load_tests-Discovery) | existing lane |
| test_ci_workflow_policy_receipts.py | pr-planner-tests (neu in load_tests-Discovery) | existing lane |
| ci-workflow-policy.sh | CI_CHECKS (ci-policy + planner + executor-verifier) | existing neighbor pattern |
| test_pr_gate_plan_receipt_paths.py | CI_CHECKS (runs in planner lane) | existing lane |
| receipt-approval.yml / receipt-post-run.yml / receipt-shadow.yml | WORKFLOW_SYNTAX_CHECKS + CI_CHECKS (PLANNING_WORKFLOWS precedent) | NOT native |

Verdict: **correct and ENG**. No Breit-Erlassung. Each of the three receipt-*.yml keeps a
job-level `never-true` guard and is registered in the `ci-workflow-policy.sh` allowlist
(shown at `l4-allowlist.md`), so the change exercises only workflow syntax + policy — exactly
the `PLANNING_WORKFLOWS` precedent for `pages.yml`. The receipts-Python paths go through the
Focused-Python `load_tests` discovery mechanism (not a new gate); the ymls go through syntax +
policy (not native qualification). This is the exact remedy the CI-BLOCKED run demanded.

Independent proof (`evidence/board-2026-10-26/scripts/r1-block-proof.py`):
```
paths= 14
coverage_status= resolved
unresolved= []
selected_gates= []
ids<=VALIDATIONS= True
broad_required= False
ALL-8-VALIDATIONS-OK=True
R1-PROOF-OK
```
All 8 validation IDs (`ci-actionlint`, `ci-executor-verifier-tests`, `ci-policy`,
`documentation-audit`, `mkdocs-de-strict`, `mkdocs-en-strict`, `pages-candidate-build`,
`pr-planner-tests`) exist in `pr-check-runner.VALIDATIONS`. No new gate or check name.

## (2) NO change to check-trigger / runner logic beyond mapping

`git diff 084b3abc..83d53e86 --stat`:
```
 scripts/ci-workflow-policy.sh               |  2 +
 scripts/pr-gate-plan.py                     | 45 +++++++++++++---
 scripts/test_ci_workflow_policy_receipts.py | 80 +-
 scripts/test_pr_gate_plan.py                |  7 ++-
 scripts/test_pr_gate_plan_receipt_paths.py  | 50 ++
```
`scripts/pr-gate-plan.py` changes are limited to (a) extended `RECEIPT_PILOT_SCRIPTS`, (b) the
new `CI_POLICY_SOURCES` set, (c) the new `RECEIPT_DORMANT_WORKFLOWS` set, and (d) the four
`elif` branches in `plan()` that route those sets to existing check classes. No
`pr-check-runner.py` change, no ci.yml validation route change, no runner-whitelist change.
The `elif` chain still ends at `path.startswith('.github/workflows/')` → `runtime-workflow-unresolved`
→ native qualification, so active workflow changes stay conservative. Verified by the retained
regression `test_active_workflow_change_stays_native_unresolved` (passes).

## (3) L4-allowlist sync consistent with policy script (two files, no third path)

- Commit `29115985`: `scripts/ci-workflow-policy.sh` — added `receipt-approval.yml`,
  `receipt-post-run.yml` (alphabetical before `receipt-shadow.yml`).
- Commit `cb01d090`: `scripts/test_ci_workflow_policy_receipts.py` — `EXPECTED_ALLOWLIST`
  mirrors the shell array exactly (23 entries each, identical order). The two obsolete
  negative tests were reconciled to the registered truth (L4 evidence documents this
  reconciliation as the reason the suite is green).
- No third file was touched to sync the allowlist. The shell array and the test map are
  byte-for-byte consistent for the receipt entries.

## (4) Suite green (candidate `83d53e86`, worktree `l6-ustrack`)

| Suite | Result |
| --- | --- |
| `python3 scripts/test_pr_gate_plan.py` | OK (planner lane incl. discovery of receipt suites + 10 mapping regressions) |
| `python3 scripts/test_pr_gate_plan_receipt_paths.py` | 10/10 OK, exit 0 |
| `python3 scripts/test_ci_workflow_policy_receipts.py` | 5 OK, 0 VIOLATION, exit 0 |
| `python3 scripts/test_receipt_chain.py` | 6 OK, exit 0 |
| `python3 scripts/test_receipt_writer.py` | 11 OK, exit 0 |
| `bash scripts/ci-workflow-policy.sh` | PASS: 22 approved workflow files, exit 0 |
| `bash -n scripts/ci-workflow-policy.sh` | OK |
| `r1-block-proof.py` | R1-PROOF-OK |

## (5) W7 bootstrap (develop-copy) — does it qualify exactly THIS block?

The W7 plansync entry (`R27_PLANSYNC_REVIEWED_HEAD`/`_REVIEWED_FILES`, exact-head self-referential
bootstrap modeled on `RUNTIME_STEP_CAP_REVIEWED_HEAD` / `DISPATCH_GATE_REF_REVIEWED_HEAD`) is a
**separate** mission in the `baseharbor` repo (`worktrees/w7-plansync`, branch `w7/plansync`, base
`origin/develop`). It is NOT part of candidate `83d53e86`:
- `grep R27_PLANSYNC / plansync / 7229516a worktrees/l6-ustrack/scripts/pr-gate-plan.py` → no hit.
- The candidate (`l6/ustrack`, base `084b3abc`) lives on the baseharbor-plan/integrate branch, not
  on develop's planner. The W7 bootstrap is develop's self-qualification for its own diff.

The W7 bootstrap is a *planned* mechanism, not yet landed (evidence `w7-plansync.md` is still a
pending template, Step 0 unchecked, all simulation sections `pending`). Its purpose is to let
develop's planner qualify PR-B (`develop: only this 3+plannerfile` diff) once PR1 and the
`l6/ustrack` block are re-integrated into develop. It does not need to qualify THIS candidate as it
stands today — this candidate's own planner already maps the receipt paths to existing checks
(proven by the suite + proof above).

Edge case worth flagging for the re-integration owner: once `l6/ustrack` is re-integrated into
develop, develop's planner must route the newly-added `CI_POLICY_SOURCES` / `RECEIPT_DORMANT_WORKFLOWS`
sets without a native gate. The W7 bootstrap (exact-head self-reference) is the intended vehicle for
that; it must pin W7's final commit SHA as `REVIEWED_HEAD` (self-reference, exactly as the prior
CI self-qualifications do) and list the changed planner + L6 files. Until W7 lands, the develop-based
`baseharbor-plan` lane for PR1-Zweitblock would resolve the receipt paths only through the planner's
own `elif` routing on the integrate branch — which is what this candidate proves. No defect in THIS
candidate; the W7 bootstrap is a downstream re-integration prerequisite, not a blocker of `83d53e86`.

## repo-rule compliance

- No new gate, no new check/validation name; only existing `VALIDATIONS` referenced
  (`baseharbor` AGENTS "as little as possible", AGENTS "Evidence: automatically").
- Trigger/runner logic untouched beyond the mapping (check #2).
- No push performed; work isolated in `worktrees/l6-ustrack`; one mapping commit on top of the
  two L4 commits already on the integrate branch.
- Read-only review: only `evidence/board-2026-10-26/scripts/r1-block-proof.py` written.

## Open item (non-blocker)

The local CI-BLOCKED run 40399422956 is not reproducible from local files here; the green state is
established by the four required local suites + the `ci-workflow-policy.sh` real gate + the
independent proof script. The mandatory green Mandatory-CI for PR1 remains the gate before merge;
this review certifies the candidate is mapping-correct and self-contained, not the CI run itself.

END R1-REVIEW-DONE / END R1-BLOCKED
