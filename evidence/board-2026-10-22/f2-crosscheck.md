# PR #1030 — Independent Cross-Check of the Q3 Dossier (F2)

**Date:** 2026-10-22
**Reviewer:** F2 cross-check worker (independent of Q3; read-only)
**Under review:** `evidence/board-2026-10-22/q3-1030-dossier.md` (head `446e0e82b965551df24446360f31440880ed7cae`)
**Method:** fresh `git fetch origin --prune` + per-SHA fetches, `git ls-remote`, `gh pr view 1030 --json`, `gh pr checks 1030`, `gh api` rulesets/branch-protection/run, `gh repo view`, git object and ancestry math in `baseharbor`. No product file was edited; only `origin` remote-tracking refs moved.

Legend: **AGREE** = independently reproduced. **DISAGREE** = claim wrong or rationale unsupported. **OMITTED** = material fact Q3 did not examine.

## Item-by-item table

| # | Q3 claim | F2 independent result | Verdict |
|---|---|---|---|
| 1.1 | Local `446e0e82` = "ci: qualify public MCP bootstrap and bounded CLI admission fixes", parent `30e11243`, tree `5c81ddca0c8bcc4a9597924f94708306e066c09d` | `git log -1 --format='%H %P %T'` reproduces exactly | **AGREE** |
| 1.2 | headRefOid = 446e0e82 | `gh pr view 1030 --json headRefOid` matches; base=develop, state=OPEN, not draft | **AGREE** |
| 1.3 | Remote branch head; "Origin is the fork itself" | `ls-remote`: `refs/heads/ci/public-entry-qualified-sources-v0426` = `refs/pull/1030/head` = `446e0e82`, no divergence. But `gh repo view`: `isFork=false`, `parent=null`, PUBLIC, default branch `main` — mcpdev80/baseharbor is the canonical repo, not a fork | **AGREE** head / **DISAGREE** fork premise |
| 1.4 | Merge-base(develop, head) = `30e11243` (identical to develop tip; Handoff SHA) | `git merge-base` = `30e112431b04a7e24e7e7e9ea76a3600e439a988` = `git rev-parse origin/develop` = `ls-remote refs/heads/develop` — head is exactly develop + 1 commit | **AGREE** |
| 1.5 | `MERGEABLE` / `BLOCKED` / `REVIEW_REQUIRED` | `gh pr view` reproduces all three; `reviews=[]`, `reviewRequests=[]` | **AGREE** values |
| 1.6 | "branch protection is not configured on this fork, so BLOCKED refers to the missing review, not a branch-protection blocker" | False on two counts: (a) not a fork; (b) **two ACTIVE rulesets protect `refs/heads/develop`**: `develop - integration branch` (id 23754541) and `develop - required checks` (id 25223521). BLOCKED stems from those rulesets' unmet review rules, which are exactly branch protection | **DISAGREE** |
| 2.1 | Diff stat: 2 files, +76 (38+38), 0 deletions | `git diff origin/develop...446e0e82 --numstat` = 38/0 and 38/0; identical to `..` (2-dot) since merge-base = develop tip | **AGREE** |
| 2.2 | 3 anchor constants (`MCP_PUBLIC_BOOTSTRAP` f762f448 4M, `NEW_TERMINAL_INPUT` 99901ff2 2M, `EDIT_REFUSAL_RESULT` c6dfb988 2M) + 3 tuples injected before `DOCKER_RUNTIME_FIXTURE`, no reorder/removal | Diff hunks at `scripts/pr-gate-plan.py` reproduce this exactly, incl. file-scope dicts matching HANDOFF's 4M/2M/2M inventories | **AGREE** |
| 2.3 | Test file: 3 assertEqual anchor pairs, 3 new `cases` tuples, `prior_source` negative-proof guard (2f32556c-style pre-wrapper source must NOT qualify) | All three constructs present in `scripts/test_pr_gate_plan.py` diff | **AGREE** |
| 2.4 | Verifier and unrelated records untouched | Only the 2 files are in the diff; no `ci-executor-verifier` path present | **AGREE** |
| 3.1 | Run 39630020500 belongs to this head, all ran checks green | `gh api actions/runs/39630020500`: `headSha=446e0e82`, event `pull_request`, workflow `.github/workflows/ci.yml`, conclusion success | **AGREE** |
| 3.2 | Per-check statuses (17 rows: listed pass/skip assignments) | `gh pr checks 1030` reproduces every row's status | **AGREE** |
| 3.3 | Summary "Passing: 9, Skipped: 8" | Arithmetic wrong: actual **7 pass / 10 skip / 0 fail** (17 total). Q3's own table (7 SUCCESS / 10 SKIPPED) contradicts its summary | **DISAGREE** |
| 3.4 | "Skipped checks are conditional gates, not required-for-this-scope" | Verified against `ci.yml` job `if:` conditions: push-only gates (source-preflight, HA docker/podman, validate) and planner-output gates (`runtime_required`, `consumer_mode`, `public_contracts_required`) — all legitimately skipped for a scripts-only PR | **AGREE** |
| 3.5 | "Single required check: PR Selected Checks — PASSED" | Ruleset 25223521 `required_status_checks` = exactly one context `PR Selected Checks`, and it passed. But Q3 reached this blind: `strict_required_status_checks_policy=true` (branch must be current with develop — currently true because merge-base = develop tip) and bypass_actors=[] — this gate cannot be bypassed | **AGREE** outcome / rationale unexamined |
| 4.1 | f762f448 "MISSING locally, needed remote fetch"; tree `60a8ce18...` = declared | In this clone the object was already present (loose, from an earlier SHA fetch); no branch ref contains it (`for-each-ref --contains` empty); `git fetch origin <sha>` succeeds. Tree confirmed `60a8ce186a573fb7277fc56f3ebdde55f4c86ad4` | **AGREE** substance |
| 4.2 | 99901ff2 exists, tree `2753ec2d...`; c6dfb988 exists, tree `3c9ab321...`, parent chain includes 2f32556c | All reproduced (`cat-file`, `rev-parse ^{tree}`, `%P`). Additional precision Q3 missed: 99901ff2 and c6dfb988 are the heads of live remote branches `refs/heads/review-source/new-terminal-cancellable-input-v0426` / `review-source/edit-refusal-result-v0426` | **AGREE** |
| 4.3 | All three: not ancestors of develop or 446e0e82; merge-base with develop `213c15e4` on a separate lineage | `merge-base --is-ancestor` = NO for both targets on all three; `git merge-base <s> origin/develop` = `213c15e4` for all three | **AGREE** |
| 4.4 | "Patch-based introduction required; code of these commits is outside the PR head by design" | Confirmed, and extended (see 6.3): content also not in develop by another route | **AGREE** |
| 5.1 | Checkpoint `815dfdba102c209ad7122e72afba88ae27f9b014` absent; fetch returns `upload-pack: not our ref` | Reproduced verbatim | **AGREE** |
| 5.2 | Declared tree `5c81ddca` exists and equals `446e0e82^{tree}` | `git rev-parse '446e0e82^{tree}'` = `5c81ddca0c8bcc4a9597924f94708306e066c09d` | **AGREE** |
| 5.3 | Verdict CONDITIONAL for the checkpoint | Reasonable reading; HANDOFF itself calls the checkpoint local-only ("remote head446e is authoritative"), which Q3 did not cite | **AGREE** with note |

## What Q3 missed (OMITTED)

| # | Omission | Evidence |
|---|---|---|
| 6.1 | **CODEOWNERS + mandatory code-owner review.** `.github/CODEOWNERS` on develop assigns `* @mcpdev80`; ruleset 23754541 sets `require_code_owner_review=true`. One approving review from any account is NOT sufficient — the merge additionally needs a code-owner approval by @mcpdev80, plus `require_last_push_approval=true`, thread resolution, and `required_linear_history` + `allowed_merge_methods=["squash"]`. Q3's "READY TO MERGE once review is obtained" understates the review gate, and squash-only means the merge commit SHA will differ from `446e0e82` (relevant to HANDOFF's "accept only while exact head/checks remain valid") | `gh api repos/mcpdev80/baseharbor/rulesets/23754541`; `git show origin/develop:.github/CODEOWNERS` |
| 6.2 | **Develop IS protected — via rulesets, not classic branch protection.** `gh api branches/develop/protection` returns 404 "Branch not protected", which likely misled Q3; the enforcement lives in two ACTIVE rulesets (deletion, non-fast-forward, PR review rules, required checks). Calling the remote "a fork without branch protection" is factually wrong | `gh api repos/mcpdev80/baseharbor/rulesets`; `gh repo view --json isFork` |
| 6.3 | **No frozen source reached develop by another route.** Q3 checked ancestry only (NO both ways). Content-level check closes the "another route" gap: at develop tip `30e11243`, every modified scope file of each source still differs from the reviewed version, while develop's copies of the other sources' untouched scope files equal the `213c15e4` baseline — i.e. no squash-equivalent integration of f762f448/99901ff2/c6dfb988 has occurred | `git diff --name-status 213c15e4 <s>` per source; per-path `git diff origin/develop <s>` non-empty for each source's own scope files |
| 6.4 | **Skip-count arithmetic** (7/10 actual vs 9/8 stated) — minor, but the dossier is a maintainer decision record | `gh pr checks 1030` |
| 6.5 | **PR has no review requests at all** (`reviewRequests=[]`) — not just "no approval yet"; nobody has even been asked | `gh pr view 1030 --json reviewRequests` |

## Repo-rule notes

- HANDOFF (`baseharbor-plan/evidence/codeman-2026-10-21/HANDOFF.md`, "OWNER REQUESTED STOP") is honored as read-only verification; this cross-check starts nothing, merges nothing and pushes nothing.
- Per the delegation contract this file is the deliverable; it lives under `evidence/board-2026-10-22/`, not in the product branch.

## repo-rule compliance

- `baseharbor/AGENTS.md` §"Work and verify": checked actual branch, remote HEAD (`ls-remote`), task scope and existing PR before any conclusion; performed zero edits (only `git fetch`, read-only `gh` calls) — confirms "Checkpoints are leads; confirm their claims" by independently re-deriving every Q3 claim.
- §"CI execution discipline": inspected the current candidate's CI result (run 39630020500) without pushing any candidate or triggering any run; no reruns, no cancellation, no full-matrix launch.
- §"Release tasks": pre-merge review only; no readiness declaration, no release/tag/promotion action; this document explicitly does not accept or qualify anything.
- Read-only obligations honored: `baseharbor` working tree untouched; no branch, tag, comment, review or check was created on GitHub.

## Final verdict

**CHALLENGE Q3** — every merge-relevant object fact reproduces (head, diff, checks, frozen sources, checkpoint), but Q3's governance layer is wrong: develop is ruleset-protected on a non-fork canonical repo (merge needs approving + code-owner + last-push review, squash-only, strict up-to-date `PR Selected Checks`), and its check tally (9/8) is a miscount (actual 7 pass / 10 skip).

F2-CROSSCHECK-DONE
