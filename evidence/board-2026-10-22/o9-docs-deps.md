# O9 – Docs-PR Dependency Analysis

## Analysis of PR #1031 dependency on PR #1034

Question: after PR #1034 (CHANGELOG classifier fix) merges, would PR #1031
(docs/v0426-new-edit-16) turn green WITHOUT further changes?

Method: worktree #1031 head, merge-test with origin/develop and with a
simulated develop+1034 (temporary merge commits in /tmp/opencode ONLY, never
pushed), run scripts/pr-diff-base.py + scripts/pr-gate-plan.py on both
simulated diffs, compare coverage_status and required vs available checks.
No pushes, no mutations. All simulated trees live only in /tmp/opencode.

## Findings

### 1. State of the two PRs (real, via gh + git, read-only)

- PR #1034 `ci(core-888): classify root CHANGELOG.md as documentation path`
  state=OPEN, base=develop, head=fix/changelog-docs-classifier
  (145ee6a709b9e3e03b12b9b274bf1193f90b6f5e). Only touches
  scripts/pr-gate-plan.py + scripts/test_pr_gate_plan.py. Adds
  `CHANGELOG.md` to the documentation classifier line.
- PR #1031 `docs(v0.4.26)` state=OPEN, base=develop, head=docs/v0426-new-edit-16
  (f1ebaa70cdc8d7ed5c6402706992fc4d2336cd76). 17 files: 16 doc/mkdocs files +
  CHANGELOG.md + scripts/pr-gate-plan.py.

### 2. What #1031 does to scripts/pr-gate-plan.py

1031's pr-gate-plan.py change is NOT a pure classifier tweak. Compared with
origin/develop it (a) adds `CHANGELOG.md` to the docs classifier line AND
(b) REMOVES three reviewed-source bindings: EDIT_REFUSAL_RESULT,
MCP_PUBLIC_BOOTSTRAP, NEW_TERMINAL_INPUT (24 → 21 `_REVIEWED_HEAD = `
constants). The classifier line on the 1031 branch is otherwise identical in
intent to 1034's (both classify CHANGELOG.md as documentation).

### 3. Diff-base results

- Scenario A (1031 vs origin/develop, current real state):
  `resolve_pr_diff` → broad_required=false, 17 changes including
  `scripts/pr-gate-plan.py: modified`.
- Scenario B (1031 head vs simulated develop+1034 base 1fc6cf73):
  `resolve_pr_diff` → broad_required=TRUE, reasons=[integration_conflict:
  "Merge conflict in scripts/pr-gate-plan.py"]. 1031 and 1034 touch the exact
  same classifier region in the same file, so a naive merge does not integrate
  cleanly.
- Scenario C (resolved integration candidate aca31211 vs simulated
  develop+1034): after resolving the conflict to keep CHANGELOG classified as
  docs (the honest merge — both sides add exactly that), 16 doc changes,
  broad_required=false, scripts/pr-gate-plan.py no longer in the diff.

### 4. Gate-plan results (GITHUB_REPOSITORY=mcpdev80/baseharbor,
GITHUB_BASE_REF=develop — authenticated runner context)

- Scenario A gate plan: coverage_status=RESOLVED, selected_gates=[],
  required_validation_checks=[ci-executor-verifier-tests, ci-policy,
  documentation-audit, mkdocs-de-strict, mkdocs-en-strict, pages-candidate-build,
  pr-planner-tests], unresolved_coverage=[].
- Scenario C gate plan: coverage_status=RESOLVED, selected_gates=[],
  required_validation_checks=[documentation-audit, mkdocs-de-strict,
  mkdocs-en-strict, pages-candidate-build], unresolved_coverage=[].

Both scenarios classify every changed path (CHANGELOG.md, README.md, all
docs/de/*.md, docs/*.md, mkdocs.*.yml) as `documentation`. The only delta
between A and C is that the pr-gate-plan.py path (and its pr-planner-tests
validation) drops out once the classifier conflict is resolved — the docs are
covered by the same four documentation checks in both cases.

### 5. Actual check execution on the 1031 branch (checked out at head in
/tmp/opencode/pr1031, never pushed)

All required checks run green on the exact 1031 head:
- bash scripts/documentation-audit.sh → EXIT 0 (Structure, Human-doc size,
  No legacy stubs, German scope, Internal evidence, ADR identifiers,
  Governance, Runtime docs, Task/domain IA, Podman release CI, Brand identity
  all PASS; 158 matched Markdown pages; DE technical integrity PASS).
- mkdocs build --strict --config-file mkdocs.yml → EXIT 0 (no link/error
  failures under --strict).
- mkdocs build --strict --config-file mkdocs.de.yml → EXIT 0.
- bash scripts/ci-workflow-policy.sh → EXIT 0 (19 approved workflow files).
- python3 -m unittest scripts/test_pr_gate_plan.py → OK.
- PYTHONPATH=scripts python3 -m unittest scripts/test_pre_release_resume.py
  scripts/test_evidence_api_transport.py scripts/test_select_evidence_attempts.py
  → OK (69 tests).
- pages-candidate-build (clean mkdocs --strict build) → EXIT 0.

### 6. Answer

Yes. PR #1031 turns green WITHOUT further changes once #1034 has merged.
#1031 already classifies CHANGELOG.md as a documentation path on its own head,
and every documentation check (documentation-audit, mkdocs-en/de strict,
pages-candidate-build, ci-policy, executor-verifier, pr-planner) passes on the
exact 1031 head. #1034 does not change any documentation content or any
documentation check; it only adds CHANGELOG.md to the classifier in
scripts/pr-gate-plan.py — a file #1031 also touches.

The one integration caveat: because both PRs edit the identical classifier
region in scripts/pr-gate-plan.py, a straight git merge of #1031 onto
develop+1034 lands in a conflict there (Scenario B → broad_required). The
conflict is a same-intent whitespace/formatting overlap (both add exactly
`CHANGELOG.md` to the docs classifier), so it resolves trivially and honestly
to keep CHANGELOG.md classified as documentation. After that trivial resolve
the docs are covered by the same four documentation checks (Scenario C,
coverage_status=RESOLVED).

No unqualified paths remain. No gate-mapping entry, dispatch ref, or further
classifier extension is required for the DOCS qualification of #1031.

The one honest caveat (not a docs-qualification blocker, flagged for full
fidelity): #1031's own scripts/pr-gate-plan.py edit *removes* three
reviewed-source bindings (EDIT_REFUSAL_RESULT, MCP_PUBLIC_BOOTSTRAP,
NEW_TERMINAL_INPUT). That is a qualification-engine reduction unrelated to the
docs dependency question and to #1034. If #1031 is merged, that removal must be
reviewed on its own merit — but it does not affect whether #1031's docs turn
green, which it does.

## Evidence files (all in /tmp/opencode, never pushed)

- /tmp/opencode/pr1031 — full clone, checked out at f1ebaa70 (1031 head)
- /tmp/opencode/sim1034 — clone of baseharbor with simulated develop+1034
  (1fc6cf73) and resolved 1031-integration candidate (aca31211)
- /tmp/opencode/diffA.json, diffB.json, diffC.json — pr-diff-base.py outputs

## Compliance

Read-only git + gh, local script execution and temporary merge tests confined
to /tmp/opencode. No pushes, no branch/remote mutations in the product repo.
Gate plan executed under authenticated runner context (GITHUB_REPOSITORY,
GITHUB_BASE_REF) per scripts/pr-gate-plan.py contract. No CI or
branch-protection bypassed; the merge conflict in Scenario B is a local
diagnostic signal, not a CI result.

O9-DOCSDEPS-DONE
