# o106 – Integrate both CI changes for Meta-Gate-Grün, f39-Diagnose

**Mission** (`ops/missions/o106.txt`): Integrate two CI changes so Meta-Gate is green, plus the f39 diagnostic.
**Git scope**: `baseharbor` only, ABSOLUTE paths.
**Owner-Freigabe-Linie**: PR #1056 (Hook-PR) is review-records-verified + tests green; #1056 itself removes the blocking `generik` rule.

## PLAN

### Step 0 — Setup
- Evidence file created here (`evidence/board-2026-10-22/o106-integrate.md`).
- Native todowrite: `[wip:o106]`.

### Step 1 — Merge PR #1056 (baseharbor)
- Command: `gh pr merge 1056 --repo mcpdev80/baseharbor --merge`
- Rationale: Review-Records-geprüft + Tests grün; #1056 entfernt die blocking generik-Regel.

### Step 2 — Rebase #1055 branch onto new develop
- Branch: `work/ci-explicit-candidate-trigger-20261024` (PR #1055)
- Target: new `develop` (force-with-lease allowed)
- Tests must run: `test_ci_candidate_trigger.py` + `test_pr_gate_plan.py`

### Step 3 — Update PR #1055
- Push rebased branch, update PR.

### Step 4 — Post-Merge-CI check on develop (#1056)
- If #1056-Check after merge stays RED on develop: STOP immediately, report BLOCKED with log location.
- All actions documented in this evidence.

### Step 5 — END
- Record: `develop-sha` + `1055-status`.

## repo-rule compliance
- (to be filled)

## Diagnosis: #1056 meta-check stays red despite register #1058 on develop (ROOT CAUSE)

**Finding (2026-10-24, read-only, baseharbor):** PR #1056 is NOT mergeable to green. Do
not force-merge with `--admin`. The persistent fail-closed is a stale/incorrect CI-binding
register, not a pins-code regression.

Evidence chain:
- PR #1056 head `38ba06bb` (commit 3/3), base `develop@24c33af`, files = exactly
  `scripts/pr-check-runner.py`, `scripts/pr-gate-plan.py`, `scripts/test_pr_gate_plan.py`.
- Authoritative CI run 40066777947 (head `38ba06bb`): `PR Selected Checks = failure`.
  Meta-job log: `HEAD is now at 24c33af docs(ci): add Core-921 CI binding qualification
  for #1056 (#1058)` then `BLOCKED: coverage-unresolved: CI execution/evidence impact
  requires specific qualification: scripts/pr-check-runner.py` (exit 1).
- develop HEAD = `24c33af` (PR #1058 MERGED 05:23Z), which added a `docs/internal/ci-binding-qualifications.md`
  register binding `reviewed_head: 4e737e26` with file_hashes
  `05f244d6…`, `7c285725…`, `8564c6cb…`.
- PR #1056 head `38ba06bb` evolved AFTER the register: `24c33af` -> `01132e8c` -> `72431414`
  -> `38ba06bb`. So `4e737e26` is NOT an ancestor of `38ba06bb` (`git merge-base --is-ancestor`
  = NO). The generalized register check (pr-gate-plan.py:647-649) requires
  `core752_reviewed_merge_chain(24c33af, 38ba06bb, 4e737e26)` = TRUE; it is FALSE.
- The three blob SHA-256 at `38ba06bb` (`e4b12652…`, `1b252baf…`, `bef091cc…`) and at `4e737e26`
  (`e4b12652…`, `5ec123e3…`, `3817bef3…`) both DIFFER from the register's recorded digests.
  The register digests match no verified checkout blob (also not old develop `1861ed07`).
- Therefore trusted_binding_checks returns None -> fail-closed ci-executor-unresolved. This is
  exactly the meta-check that is failing. No merge of #1056 can pass until the register is
  corrected to bind THIS candidate (new reviewed_head + matching hashes) via a NEW docs-only PR.

**Merge decision:** Amendment o106c authorizes `gh pr merge 1056 --admin` with a PR comment.
`--admin` bypasses required checks. The pre-merge gate claimed in the amendment
("Meta-Check bis-Merge") is NOT met (authoritative CI = red, fail-closed). Using --admin here
would circumvent a fail-closed gate — forbidden by AGENTS.md ("keine Schutzregel-Bypass",
"Keine grünen Gates behaupten, überspringen"). #1056 NOT merged; reported BLOCKED with log.

## Evidence log

| Step | Timestamp (local) | Action | Result |
|------|-------------------|--------|--------|
| 0 | 2026-10-24 07:2x | Evidence + todowrite ([wip:o106]) created | done |
| 1 | 2026-10-24 07:4x | Diagnose #1056 meta-check fail; read-only; mergedAt=null, state=OPEN | BLOCKED (stale register, see root-cause above) |
| 2 | 2026-10-24 07:5x | rebase #1055 8f40ce4f onto new develop 24c33af (worktree, force-with-lease) | done, clean, no conflicts |
| 3 | 2026-10-24 07:5x | run test_ci_candidate_trigger.py + test_pr_gate_plan.py | BOTH PASS (16 + 8/7/5) |
| 4 | 2026-10-24 07:5x | push rebased #1055 to remote | blocked by gh-SSH proxy (existing refs read-only); pushed to new branch work/1055-rebased-20261024 @ 8f40ce4f |
| 5 | | post-merge CI (only if #1056 merged) | not applicable (1056 not merged) |
| 6 | 2026-10-24 07:5x | END | see below |

## END o106

- develop-sha (baseharbor): 24c33afdc89f166bf4ed747445078fa47489b562
  (docs(ci): add Core-921 CI binding qualification for #1056 (#1058), MERGED 2026-10-24T05:23Z)
- #1056 state: OPEN, mergedAt=null. NOT MERGED. BLOCKED — see root-cause section.
  --admin merge refused: authoritative CI run 40066777947 head 38ba06bb = PR Selected Checks FAILURE
  (fail-closed coverage-unresolved, scripts/pr-check-runner.py). Root cause = stale/incorrect
  ci-binding-qualifications.md register (24c33af binds reviewed_head 4e737e26 + hashes that match
  no real blob; 38ba06bb evolved past it; core752 merge-chain FALSE). Fix = NEW docs-only PR adding
  a corrected register entry binding 38ba06bb (new reviewed_head + SHA-256 blob proof) — same
  mechanism #1058 used. No force-merge.
- #1055 state: OPEN, headRefOid 8f40ce4f (remote unchanged). Rebase onto new develop + both
  required tests PASS locally. Remote ref update blocked by gh-SSH proxy (existing refs read-only;
  writable only via new-branch creation). Rebased candidate pushed to branch
  work/1055-rebased-20261024 @ 8f40ce4f (on develop 24c33af).
- Post-merge CI on develop: not run (no #1056 merge).
- o106-INTEGRATE-DONE: BLOCKED on #1056 register; #1055 rebase/tests complete locally.
