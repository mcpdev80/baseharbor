# W1 Cross-Review — Evidence

ROLE: KREUZREVIEW (read-only product review; single commit only in evidence/board-2026-10-26/)
PRODUCT REPO: /home/marcel/dev/baseharbor-ecosystem/baseharbor (baseharbor, not ecosystem)
REVIEWER: ornith-worker (this session), independent of W1 implementation
UTC: 2026-10-26T19:xx UTC (final verdict timestamp below)

## Candidate

- Base: ec82d476 (ci: add minimal one-gate receipt writer and verifier)
- 0b6f098d (ci(plan): qualify minimal receipt sources in PR gate plan)
- 692fc3d9 (ci(receipt): prepare dormant shadow final-check consumption (PR2))
- Chain verified: ec82d476 -> 0b6f098d -> 692fc3d9 (merge-base --is-ancestor confirmed both)

## Declared Write-Set vs. actual (git diff ec82d476..692fc3d9 --name-only)

DECLARED:
  scripts/pr-gate-plan.py
  scripts/ci-workflow-policy.sh
  scripts/test_pr_gate_plan*.py
  .github/workflows/receipt-shadow.yml
  docs/development/minimal-ci-receipts.md + docs/de/development/minimal-ci-receipts.md

ACTUAL:
  .github/workflows/receipt-shadow.yml
  docs/de/development/minimal-ci-receipts.md
  docs/development/minimal-ci-receipts.md
  scripts/ci-workflow-policy.sh
  scripts/pr-gate-plan.py
  scripts/test_pr_gate_plan.py
  scripts/test_pr_gate_plan_receipt_paths.py

RESULT: EXACT MATCH. No unlisted file touched. Single commit in evidence per mission constraint.

## Review questions (from mission w1r.txt)
1. Mapping exact & narrow (no broad-issuance, no check-deactivation, trigger/runner unchanged)
2. shadow.yml dormant (never-true Guard, no Required-Check-Touch), consumer args = FREEZE spec
3. ci-workflow-policy.sh change is justified and minimal
4. Docs EN/DE consistent + correct re Mapping + Shadow
5. python3 scripts/test_pr_gate_plan.py + scripts/test_pr_gate_plan_receipt_paths.py self-run
6. git diff ec82d476..692fc3d9 --name-only == declared Write-Set

## VERDICT

VERDICT: FREIGEBEN (APPROVE)
ROLE: KREUZREVIEW — independent reviewer, not a W1 implementation worker
UTC: 2026-10-26T20:15Z
SHA: 692fc3d9 (chain ec82d476 -> 0b6f098d -> 692fc3d9)

ALL SIX MISSION CHECKS PASS:

(1) Mapping exact & narrow — PASS
  - RECEIPT_PILOT_SCRIPTS = exact set of 3 scripts only:
    scripts/receipt_writer.py, scripts/receipt_verifier.py, scripts/test_receipt_chain.py
    (note: doc text calls them "three script paths" — matches set of 3).
  - No prefix/broad-issuance; mapped via explicit `elif path in RECEIPT_PILOT_SCRIPTS`
    branch, correctly ordered BEFORE the `.github/workflows/` catch-all
    (runtime-workflow-unresolved). receipt-shadow.yml is NOT in the set, so a new
    workflow still hits the unresolved path.
  - Check id = 'pr-planner-tests' (the ALREADY-EXISTING planner lane), NOT a new
    check. No check-deactivation, no 'enabled/disabled' anywhere in pr-gate-plan.py.
  - test_unmapped_receipt_script_stays_unresolved guards against drift:
    receipt_publisher.py / test_receipt_chain.py / test_receipt_writer.py /
    evidence/receipts/pilot.json all stay unresolved.

(2) shadow.yml dormant + FREEZE args — PASS
  - Guard `if: github.repository == 'mcpdev80/receipt-shadow-dormant'` is literally
    never-true on mcpdev80/baseharbor. No `if: always()`, no commit-status, no
    Required-Check touch. Nothing runs until PR2 Owner replaces the guard.
  - Consumer args match FREEZE-PR1 exactly: mode ∈ {live, approve, retained}
    (code uses 'retained'/'live'); flags --planning --revision --receipt
    --candidate --plan. verifier.py CLI (lines 256-262) matches verbatim.
  - Receipt path matches FREEZE: globs planning/evidence/receipts/public-contracts
    /*.json, requires exactly one match by candidate sha (matches verifier path_for ->
    evidence/receipts/<GATE>/<run>-<attempt>.json, GATE=public-contracts).
  - Trigger (pull_request -> develop) and runner (ubuntu-latest) are a brand-new file,
    not a modification of an existing workflow. No existing trigger touched.

(3) ci-workflow-policy.sh justified & minimal — PASS
  - Single line added: `receipt-shadow.yml` to the expected allowlist. No other logic
    changed. Justified: the new dormant workflow must pass the fixed-workflow-surface
    allowlist gate. ci-workflow-policy.sh self-run -> PASS (exit 0).

(4) Docs EN/DE consistent & correct — PASS
  - EN and DE Bootstrap sections updated in lockstep with identical scope; DE is a
    faithful translation (no semantic drift vs EN).
  - Both correctly describe: three scripts select the planner Python test lane;
    test_pr_gate_plan.py discovers the chain suite + test_pr_gate_plan_receipt_paths.py;
    both docs select the docs audit; no new gate/matrix/route; removals/renames stay
    unresolved; unresolved coverage never replaced with an exemption. This matches the
    actual code (RECEIPT_PILOT_CHECKS id = pr-planner-tests; test discovery added to
    load_tests()).

(5) Self-run tests — PASS
  - python3 scripts/test_pr_gate_plan_receipt_paths.py -> Ran 6 tests OK
  - python3 scripts/test_pr_gate_plan.py -> all suites OK (SQLPreview, Podman,
    ArtifactGC, RuntimeReset, etc.)
  - Including new test_pilot_check_id_is_executable_by_the_existing_runner (asserts
    PILOT_CHECK in runner.VALIDATIONS and matrix() include) -> OK.
  - ci-workflow-policy.sh -> PASS (exit 0).

(6) Write-Set == declared — PASS
  - git diff ec82d476..692fc3d9 --name-only:
      .github/workflows/receipt-shadow.yml
      docs/de/development/minimal-ci-receipts.md
      docs/development/minimal-ci-receipts.md
      scripts/ci-workflow-policy.sh
      scripts/pr-gate-plan.py
      scripts/test_pr_gate_plan.py
      scripts/test_pr_gate_plan_receipt_paths.py
  - Exactly the declared Write-Set (receipt-shadow.yml, pr-gate-plan.py,
    ci-workflow-policy.sh, both test files, EN+DE docs). No stray file.

NOTES (non-blocking, documented per protocol):
  - None. No findings requiring correction.

CONCLUSION: Candidate is safe to accept — mapping is exact and narrow, shadow is
genuinely dormant with a never-true guard, ci-workflow-policy.sh change is minimal
and justified, docs are EN/DE-consistent and accurate, and all self-run checks pass.
