# O35 — PR #118 independent review of the ACTUAL remote head c1285a88

## ROLE
INDEPENDENT REVIEWER (separate from the o23 author and from the CODEX-MERGE session that produced D2PREP). Read-only: no candidate edits, no approval, no merge.

## PLAN

1. Read MERGE-118-D2PREP for why prior reviews were invalid.
2. Fetch the actual demo repo WITHOUT suppressing stderr; confirm the real remote head.
3. Produce the true two-dot diff 896dc3cb..c1285a8 (files + content).
4. Reproduce the multi-container regression against BOTH base and candidate with the executable repro.
5. Trace the exact failure to a concrete code defect in `owned_containers()`.
6. Confirm the existing regression suite coverage (does it catch this?).
7. Assess unpause-before-destroy ordering vs repro118-multiple-owned semantics.
8. Verdict MERGE-READY or CHANGES + recipe.

## WHY PRIOR REVIEWS WERE INVALID (per MERGE-118-D2PREP, verified here)

D2PREP (2026-10-23T00:14) recorded BLOCKED/NOT MERGED for two independent reasons, both confirmed live here:

- The requested reviewed head `d9c78b57` did NOT match the actual live PR head. Live PR head (confirmed via `gh pr view 118`, GraphQL, and fetched Git) is `c1285a889ec56c51dbb7c328fa119cdf16efa1dd`. `c1285a8` has parent `d9c78b57` and adds the Core fetch-preflight workflow fix (`.github/workflows/v0424-targeted.yml` +17/-1). Zero-delta assertion is false.
- Q29 (`q29-118-headcheck.md`) is invalid head-equality evidence: every Git invocation returned "empty, no repo" (2>/dev/null suppressed the failures) and Q29 inferred "no delta" from `wc -l = 0` on empty output. A failed Git invocation with suppressed stderr proves neither identical content nor a reviewed candidate. Q29's MERGE-READY is NOT accepted as gate evidence.

This session repeats the executable `repro118-multiple-owned.py` against the ACTUAL base `896dc3cb` and the ACTUAL candidate `c1285a8`.

## ACTUAL REPO STATE

- Repo: `/home/marcel/dev/baseharbor-ecosystem/baseharbor-demo`, remote `origin git@github.com:mcpdev80/baseharbor-demo.git`.
- `git fetch origin` (stderr NOT suppressed) returned:
  - `d9c78b5..c1285a8 fix/v0426-cleanup-provenance-order -> origin/fix/v0426-cleanup-provenance-order`
  - new branch `work/v0.4.26-ecosystem-release`.
- Live PR #118 (`gh pr view 118`): `state=OPEN`, `isDraft=false`, `mergeStateStatus=CLEAN`, `reviewDecision=""`, head `c1285a8`, updatedAt 2026-10-22T21:45:48Z.
- PR body: O23 cleanup-provenance ordering fix. Files claimed: `.github/workflows/v0424-targeted.yml`, `scripts/failure-runtime-diagnostics.sh`, `tests/debug-cleanup-provenance-order/run.sh`.
- Comments: 0. Reviews: 0. Review requests: none. Labels: none.
- PR checks: `no checks reported on the fix/v0426-cleanup-provenance-order branch`.
- main = `896dc3cb68a8c9b9585940c4696410de5a338c52` (== the frozen demo input D).

## TRUE DIFF 896dc3cb..c1285a8 (two-dot)

Three paths modified (NOT acceptance.sh/diagnostics-only as the task framing asserted):

1. `.github/workflows/v0424-targeted.yml` +17/-1 — Core SHA serveability pre-check before `actions/checkout` (validates 40-hex, hits the GitHub commits API and fails fast when the pinned develop-tip object is momentarily unserveable). Pure CI/infra.
2. `scripts/failure-runtime-diagnostics.sh` +78/-29 — the o23 `owned_resources()` snapshot-before-mutate capture + `owned_containers()` retry-with-backoff + accounted_missing logic. SUBSTANTIVE product-script change.
3. `tests/debug-cleanup-provenance-order/run.sh` +145/0 (new) — mock-based regression for the ordering fix.

Two-dot diff SHA256: `4a8221975beee46e4a24596620edd956d5541d1ad2fad12abc5b2243750a1f20`
- `c1285a8^{tree}` = `fa727cb925ee3d17457d74abc401e4a98229d918`
- `896dc3cb^{tree}` = `f4abc1bbf99e293cef855872592300e541fa0cb3`
- `c1285a8` parent = `d9c78b571740319629d9190abecb13233d1e8b30` (which already contains the faulty diagnostics; the only delta from `d9c78b5`→`c1285a8` is the workflow file).

## REPRO118-MULTIPLE-OWNED — ACTUAL RESULTS

Re-ran `evidence/board-2026-10-22/scripts/repro-118-multiple-owned.py` against the live repo (read-only, git-archive extract per revision):

- base `896dc3cb`: exit **0**, `outcome=success`, unpauses BOTH owned IDs.
- candidate `c1285a8`: exit **1**, `outcome=failure`, `errors=["owned runtime evidence unavailable: ValueError"]`, unpauses NEITHER.

The multi-container regression reproduced exactly as D2PREP recorded. The candidate did NOT fix it.

## ROOT-CAUSE TRACE (concrete defect in the candidate)

Traced through the candidate's `owned_containers()` with injected diagnostics against a two-owned-container-in-one-project hostile inventory:

- `resources` correctly = {`(proj, postgres-1)`, `(proj, app-1)`}.
- Native `ps -a` returns BOTH rows (rc=0, both present).
- The per-row match loop sets `match=(identity_a, row_a)` for postgres AND `match=(identity_e, row_e)` for app.
- Control-flow defect: the outer `for project` loop builds `match` incrementally, but after the attempt/backoff loop the code executes a SINGLE `if match: identity,row = match; selected[identity] = row`. This stores ONLY the last match of the loop iteration (`identity_e`/app) and DISCARDS the earlier `identity_a`/postgres match. `selected` ends up with 1 entry instead of 2.
- Downstream `missing = resources - selected` = {(proj,postgres-1)}; the daemon-state recheck finds no exited/dead container (the mock returns rc=5 for inspect of an unknown id and the state lookup finds neither exited nor dead), so `accounted_missing=[]` → assertion `canonical runtime resources lack verified native ownership` raises.

This is a genuine multi-container selection/complete per-resource accounting bug introduced by the o23 rewrite, NOT the auto-collection race the PR body describes. The PR's retry-with-backoff and accounted_missing logic never get exercised for the "two owned containers, both live" case because the first match is silently dropped before the accounting path.

## EXISTING SUITE COVERAGE — does it catch this?

- `scripts/test_owned_runtime_diagnostics.py` loads as a unittest module.
- base: ran=17, failures=0, errors=0. candidate: ran=17, failures=0, errors=0.
- The 17 tests do NOT include a "two owned containers in the SAME project, both live" case. The hostile fixtures use one owned id plus foreign/conflicting/unlisted ids, so the single-discard bug is not triggered. The official suite is therefore NOT sufficient to gate this change; it must be extended.

## UNPAUSE-BEFORE-DESTROY ORDERING vs repro118 SEMANTICS

- The o23 PR body asserts the fix captures the owner inventory ONCE (Core's non-mutating `destroy --all --json`) before any teardown. That snapshot-before-mutate premise is present in the candidate.
- The failing repro `repro118-multiple-owned.py` is a READ-ONLY base/candidate comparison of `--unpause-owned` against a hostile native inventory with TWO owned containers. It asserts that BOTH owned IDs are selected, unpaused, and accounted. base passes; candidate fails the same assertion.
- The ordering fix (snapshot-before-teardown) is orthogonal to the selection bug: even with a correct snapshot, `owned_containers()` still drops the first of two same-project containers, so `preparation` reports incomplete. The o23 `run.sh` regression test passes on the candidate only because its mock auto-collects exactly ONE project (projB exited) — a single-accountable case that never exercises the two-live-container path. The regression test does not cover the repro's scenario.

## ACCEPTANCE.SH / DIAGNOSTICS SCOPE

The candidate is NOT acceptance.sh/diagnostics-only. Two-dot diff touches a CI workflow, the substantive `failure-runtime-diagnostics.sh` product script, AND adds a new regression test. The diagnostics script change is the locus of the regression.

## repo-rule compliance

- baseharbor-demo AGENTS.md: read CONTRIBUTING.md, Planning ownership protocol, keep acceptance with Maintainer. No Planning claim/status changed. No product files edited by this review (read-only).
- No GitHub approval, merge, thread resolution, CI rerun, or protection change. No candidate edits. Read-only evidence only.

## VERDICT

**CHANGES — NOT MERGE-READY.**

The candidate head `c1285a8` is a legitimate, scoped o23 + Core-preflight change (workflow + diagnostics + regression test), but it does NOT resolve the multi-container regression that is the stated reason PR #118 exists. The executable repro reproduces the blocking failure against the actual remote head: base passes, candidate fails with `canonical runtime resources lack verified native ownership`. Live PR checks are empty (none configured on this branch), and there is no green mandatory evidence to weigh. Per the owner's stop-on-invalid/uncertain-gates posture inherited from D2PREP, this is not merge-able in its current state.

## FIX REZEPT (unsetzbare Korrektur)

File: `scripts/failure-runtime-diagnostics.sh`, function `owned_containers(resources)`.

Root defect: the inner per-row loop accumulates `match`, but only the final `match` of each `for project` iteration is committed to `selected`, dropping earlier same-project matches. Fix so every qualifying row within a project is committed, not just the last:

- Commit inside the match, e.g. move `if match: identity,row = match; selected[identity]=row` to store on the FIRST qualifying row, OR replace the single-commit with `if match: for (_i,_r) in <collected-this-pass>: selected.setdefault(_i,_r)`. The ambiguity guard (`identity in selected and selected[identity] != row`) already protects against double-assigning the same id to different rows — keep it.
- Keep semantics identical: both compose label families required, ambiguity still raises `owned runtime evidence ambiguous`, `token(service)` guard intact, no new mutable preview, no fallback that hides a genuinely-lost container.

Test requirement (must be added to `scripts/test_owned_runtime_diagnostics.py`, then the suite must be the gating check):

- Add a two-owned-containers-in-one-project hostile case (mirrors `repro118-multiple-owned.py`): preview lists `proj-postgres-1` AND `proj-app-1`; native `ps -a` returns both with distinct 12–64 hex ids, both `exited`/`running`, both `token(service)`-valid. Assert `--unpause-owned` returns `preparation=0/destroy=…/outcome=success` and unpauses BOTH ids (receipt shows 2 selected, 0 errors). This must FAIL on the current candidate and PASS after the fix.

Stop-criteria: repro `repro118-multiple-owned.py` returns exit 0 for the candidate; the new suite case passes; existing 17 cases still pass; two-dot diff is diagnostics-only + the one new regression case (no workflow/other-scope creep) OR, if the Core-preflight workflow fix is retained, it is called out explicitly as non-diagnostic and separately reviewed.

## NEXT STATES

- Re-run the executable `repro118-multiple-owned.py` against the corrected candidate head; require exit 0.
- Re-run the full `scripts/test_owned_runtime_diagnostics.py` suite; require 0 failures/errors.
- Only after the corrected head reproduces clean on both, re-verify live PR head == reviewed head, empty review threads (hasNextPage=false), and current mandatory CI before any Maintainer merge.
- No merge, approval, or D2 mint in this review.

END O35-118-REVIEW-DONE.
