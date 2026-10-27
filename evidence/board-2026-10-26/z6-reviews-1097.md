# Z6 Review — PR #1097 (baseharbor)

**ROLE:** REVIEW (read-only). Head via `gh pr view 1097` (sha `86fa57839984a5d5d1354f176272d63c2edeff0c`, base `develop`, head-branch `z2/approve-fix`).
**VERDICT: REWORK** (file:line). Two defects: one hard (verifier env contract), one must-be-decided (workflow-surface policy).

---

## (1) Diff scope — PASS
`gh pr diff 1097` touches exactly one path: `.github/workflows/receipt-approval.yml` (+32 / −6). No other file in the Write-Set.

## (2) Pin realism — PASS (with note)
- checkout `3d3c42e5aac5ba805825da76410c181273ba90b1` → real tag `v7.0.1`, and here also the `v7` tag. Pin matches its label `# v7.0.1`. **Live.**
- setup-python `5fda3b95a4ea91299a34e894583c3862153e4b97` → real tag `v7.0.0`. Pin matches tag SHA. **Live.** The PR comment labels it `# v7`; that is a cosmetic mismatch — the pinned SHA is the *v7.0.0* commit, not the *current* `v7` tag (`6d4fd22b…`, which also equals `v7.1.0`). Not a dead pin: the SHA resolves and is immutable, and pinning to an exact SHA is the documented best practice (semantic-version tags move). Comment should read `# v7.0.0` to stay truthful.
- The dead pin that broke run 40431294924 (`setup-python@08d27766 … # v5.6.0`) is correctly retired in this PR.

## (3) approve-step logic vs `scripts/receipt_verifier.py` approve — **FAIL** (REWORK)

Verifier `approve(root, revision, path, candidate, plan_revision)` (`receipt_verifier.py`, commit `ec82d476`) gates on **five** env vars, all AND-ed:
```
GITHUB_ACTOR == OWNER
GITHUB_TRIGGERING_ACTOR == OWNER
GITHUB_EVENT_NAME == "workflow_dispatch"
GITHUB_REPOSITORY == REPOSITORY   ("mcpdev80/baseharbor")
GITHUB_REF == "refs/heads/main"
```
The PR approve step only sets `GITHUB_ACTOR` and `GITHUB_TRIGGERING_ACTOR`. **Missing: `GITHUB_EVENT_NAME`, `GITHUB_REPOSITORY`, `GITHUB_REF`.** Even on a manual dispatch on `main`, `GITHUB_REPOSITORY` is *not* in the step env block (env blocks only inherit what the workflow declares), so the `require(...)` returns `"authenticated Owner action on protected main required"` and the run is blocked → the exact failure the PR claims to fix is *not* fixed. This is the core defect.

- `--planning "$GITHUB_WORKSPACE/planning"` is correct: `verify_file`/`blob` read the receipt blob at `--revision` off the read-only Planning checkout, and `commit_file` re-reads + re-writes the same path (verified: `target.parent.resolve().is_relative_to(root.resolve())` bounds the path inside Planning). Correct.
- `--revision "$PLANNING_REVISION"` = cloned Planning HEAD. `verify_file` requires the receipt's `plan == plan_revision` and `plan["candidate_sha"] == candidate`; the approve path binds to that revision's blob, not a moving target.
- **Race (mission Q3): safe.** `merge-base --is-ancestor revision HEAD` only needs `revision` to be an ancestor of HEAD. After approve commits (HEAD = revision+1) the check still holds, so re-running approve is idempotent and the `plan_revision`/`revision` anchoring cannot be outrun by Planning main advancing in between. The `blob` `<4096` byte cap and canonical re-serialization (`commit_file(..., expected=raw)`) are what actually hold the candidate — not the live Planning HEAD. Correct design; the PR's race worry is unfounded.
- `GITHUB_ACTOR` consumption: verified line-by-line that the verifier reads `os.environ.get("GITHUB_ACTOR")` and `GITHUB_TRIGGERING_ACTOR`; PR wires both from `${{ github.actor }}`. Fine — but only 2 of 5.

## (4) Patch artifact + git-am anchoring — PASS
Approve commits exactly one file (`commit_file` does a single `git commit -- <path>`). Artifact `planning/approve-*.patch` via `upload-artifact@v4`. `blob < 4096` bytes in `verify_file` bounds the source, so the approve patch is well under 64 KB. Verified a single-commit approve patch applies cleanly via `git am --3way` in a scratch Planning repo. git-am anchoring sound.

## (5) workflow-policy-suite + actionlint — **FAIL** (REWORK, must be decided by Owner)
- **workflow-policy:** `scripts/ci-workflow-policy.sh` enforces a **fixed 19-file workflow surface** on develop. develop carries neither `receipt-approval.yml` nor an allowlist entry for it. I placed the dormant file into a develop worktree and re-ran the suite → `EXIT 1`, "GitHub Actions workflow policy violation. BaseHarbor uses a fixed workflow surface." The PR must either (a) add `receipt-approval.yml` to the allowlist in the reviewed-suite step, or (b) extend an existing targeted workflow/gate instead of adding a file. As-is the suite fails.
- **actionlint:** `actionlint` is not installed in this environment; could not run it locally. Substituted `yaml.safe_load` (parses; top-level `on`→boolean-key is the standard Actions quirk, not an error) and manual inspection. Owner may want a real actionlint pass. Documented, not blocking.

## (6) Dormant rule — PASS
No new `on:` trigger armed (still `workflow_dispatch` only). Job-level `if: github.repository == '…dormant'` never-true guard stays; approve remains a manual **dispatch + Owner-actor** edit. PR does not touch existing workflows or required checks. Consistent with the dormancy contract.

---

## REWORK items
1. **`.github/workflows/receipt-approval.yml` approve-step env (REWORK, blocking):** add the three missing verifiable env entries, each scoped to the workflow context:
   - `GITHUB_EVENT_NAME: workflow_dispatch`
   - `GITHUB_REPOSITORY: ${{ github.repository }}`
   - `GITHUB_REF: ${{ github.ref }}`
   (Owner confirms `refs/heads/main` is the intended dispatch branch before shipping.) Without these the verifier re-blocks and the PR re-creates run 40431294924.
2. **workflow-policy (REWORK / Owner decision):** resolve the fixed-surface violation — add `receipt-approval.yml` to the reviewed allowlist, or fold the dormant approve path into an existing workflow/gate. Either way it must pass `scripts/ci-workflow-policy.sh` on develop before merge.

Note (non-blocking): setup-python comment label `# v7` should read `# v7.0.0` to match the pinned SHA (`5fda3b95…` = tag `v7.0.0`; the live `v7` tag is `6d4fd22b…`).

---

Evidence: this review is read-only; no candidate modified. Pins verified against `gh api …/git/ref/tags/…`. Policy re-run captured in a throwaway develop worktree (removed). git-am sanity check in scratch repo.

END Z6-BLOCKED — REWORK (file:line as above).
