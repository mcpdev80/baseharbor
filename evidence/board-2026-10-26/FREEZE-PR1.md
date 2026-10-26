# FREEZE PR1 — interfaces + disjoint assignments (coordinator, 2026-10-26)

Baseline (frozen): baseharbor work/r27-minimal-receipt @ ec82d476126291d372df2c13bd36273767574869
(based on develop 433a5702). Instruction source:
https://github.com/mcpdev80/baseharbor-plan/blob/main/evidence/r27-ci-replacement-2026-10-26/CODEMAN-START.md

## File hashes of frozen candidate (sha256)
- d6f830d98e3d47e8404b1ff8b7b000946ac132b8b35cf5266bfc102950ab14b4  scripts/receipt_writer.py
- 17e955d6ef0a35243d85c3febf1456c721d73fa76aeb69c31c2e6cea01f61561  scripts/receipt_verifier.py
- 2f864f34c51bebd5c9a4714405d62454b380b34904cf54e1b68571319082d7de  scripts/test_receipt_chain.py
- 33d26eaa119177d148771499a076c4ab3225e72395f8069248b3c6458bd1a15e  docs/development/minimal-ci-receipts.md
- 480beaca20f71eedc12de5f9f8f2c17304ccad9a7f19d2155fa3ceeb91ca8400  docs/de/development/minimal-ci-receipts.md

## Callable interface (FROZEN — changes require coordinated revision, no second contract)
writer (scripts/receipt_writer.py):
- commit_file(root, path, raw, *, expected=None)
- write(root, plan_revision, run_id, attempt, api=None) -> receipt dict
  fields: {"gate": GATE, "run": {"id", "attempt"}, "sha": plan["candidate_sha"],
           "status": "pass", "plan": plan_revision, "approved": None}
- CLI: --planning PATH --plan SHA --run INT --attempt INT
verifier (scripts/receipt_verifier.py):
- GATE = "public-contracts"; ReceiptError(ValueError); require; canonical; unique;
  decode(raw, limit); full_sha; receipt(raw); path_for(value); git; blob; load_plan;
  github(path,*,binary); live(value, plan, api=github); verify_file; retained; approve; main
- CLI: mode ∈ {live, approve, retained}; --planning --revision --receipt --candidate --plan
- Receipt path (authoritative for ALL workers/workflows):
  evidence/receipts/<GATE>/<run_id>-<attempt>.json  (in protected Planning repo)
- Size limit: <500 bytes (decode(raw, limit)); six top-level fields exactly.

## Exclusive write paths (disjoint; workers never push the shared branch)
- W1 (f71 lineage): scripts/pr-gate-plan.py, scripts/pr-check-runner.py,
  scripts/test_pr_gate_plan.py, exact qualification register entries,
  docs/development/minimal-ci-receipts.md + DE pair (paired documentation),
  .github/workflows/receipt-shadow.yml (DORMANT, PR2 commit)
- W2: scripts/receipt_writer.py, scripts/test_receipt_writer.py,
  .github/workflows/receipt-post-run.yml (DORMANT, PR2 commit)
- W3: scripts/receipt_verifier.py, scripts/test_receipt_verifier.py,
  scripts/test_receipt_chain.py, .github/workflows/receipt-approval.yml (DORMANT, PR2 commit)

Workflow agreement: receipt path + script arguments above; GATE constant must stay
"public-contracts" in all three files; no worker invents flags/paths.

## Branch model
Workers implement in worktrees/branches from ec82d476 (w2/r27-writer, w3/r27-verifier,
W1 continues on f71 branch lineage). Coordinator integrates (merge into shared
work/r27-minimal-receipt), then pushes. PR1 vs dormant PR2 commits kept separable.

## Cross-review rotation (after each primary slice)
W1 reviews W3, W3 reviews W2, W2 reviews W1 — file reservations transfer explicitly.
