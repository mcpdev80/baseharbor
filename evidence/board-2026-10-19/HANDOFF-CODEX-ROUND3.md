# Codex round-3 handoff (coordinator w1-baseharbor -> reviewer session fe5dff6c, 2026-10-20)

Session continuity: you are the retained round-1/2 reviewer. Two independent requests.

## Part 1 - DESIGN RULING REQUESTED (#1007 self-qualification anchoring)

Machine-verified facts:

1. Pushed head db45db83 (refs/heads/work/ci-core921-coverage-binding) carries the clause
   whose guard requires head_sha == BINDING_PR_1007_REVIEWED_HEAD (pinned 48def9e8).
   The PR's own head is db45db83, so the clause can never fire for its own PR-open run.
2. core752_reviewed_merge_chain accepts only head==reviewed or 3-parent merge steps;
   every linear single-commit PR-open head fails all strict clauses (pr-gate-plan.py,
   chain function first loop, len(parts) != 3 -> False).
3. Precedent: #999/#1005 qualified on merge/integration commits, not PR-open heads.
4. Correction budget for this cause is exhausted after two rounds (o12 hung, o15
   documented and stopped cleanly). Repros: o15-binding-final.md, r4-planner-mode-analysis.md,
   q2-ciwebhook-proposal.md (all in this directory).

Requested ruling between:
(a) new clause semantics for self-qualifying binding PRs (content-hash binding +
    first-parent base-anchored lineage, linear heads permitted);
(b) merge-queue / 3-parent integration specifically for #1007;
(c) CI-owner diff-base change per the q2 proposal.
We will implement exactly one option deterministically; a third worker round will not happen.

## Part 2 - #998 ROUND-3 REVIEW (independent of Part 1)

Candidate: a66e5de664b22c295a8b5ae918a9c2dfe4c6e327 (parent 6931ba9c), branch
refs/heads/work/core-921-central-cli-mcp-registration. Delta = one native lifecycle test file.

- Your round-2 blocking requirement (real MCP propose->apply happy path) is proven natively twice:
  o10-native-mcp-proof.md (implementer lane, 202 s, structured negatives with unchanged state)
  and q1-native-reverify.md (independent qwen re-run, fresh isolated dir, exit 0).
- Full verification bundle: q3-998-candidate-verify.md (20/20 sha256 read-only proof,
  build/vet/unit green; sole local FAIL is the pre-existing docker-daemon env test),
  r1-998-prereview.md, r3-998-native-delta-review.md (APPROVE),
  source readability gates PASS (800/200-line audit run locally).
- Integrity baseline: evidence-manifest.txt; narrative: draft-codex-handoff-round3-v2.md.

Please deliver ACCEPTED or CHANGES_REQUESTED for #998 regardless of the #1007 ruling.
