# Minimal CI receipts

This reference defines the one-gate receipt pilot. Production workflows and required checks are unchanged; local fixtures do not qualify the walking skeleton.

## Receipt and plan

The only pilot gate is `public-contracts`, mapped to `PR · Public Contracts` in `.github/workflows/ci.yml`. One canonical ASCII JSON file, including its newline, must be strictly below 500 bytes. The six fields are fixed. No logs, output payloads, signature keys, attestation bundles or archive service are stored.

```json
{"approved":null,"gate":"public-contracts","plan":"<full Planning commit SHA>","run":{"attempt":1,"id":123},"sha":"<full tested integration SHA>","status":"pass"}
```

The path in baseharbor-plan is `evidence/receipts/public-contracts/<run>-<attempt>.json`. Full 40-character SHAs are required. `sha` identifies the tested PR integration commit, not merely the PR head. `plan` identifies the immutable Planning commit containing `evidence/receipt-plans/public-contracts.json`:

```json
{
  "gate": "public-contracts",
  "repository": "mcpdev80/baseharbor",
  "workflow_id": 123,
  "head_sha": "<full PR head SHA>",
  "candidate_sha": "<full tested integration SHA>",
  "base_sha": "<full PR base SHA>",
  "task": "tasks/<task-id>.md"
}
```

The trusted integration owner generates that small plan from the existing trusted planner in PR 2. No person copies run IDs or source hashes to author receipts. Placeholder values above are documentation, not eligible evidence. Unknown gates and Actions reruns are deliberately unsupported in this first pilot: submit a fresh run. Never edit the active producer to make the pilot pass.

## Two scripts

| Entry point | Inputs | Result |
| --- | --- | --- |
| `scripts/receipt_writer.py` | `--planning`, `--plan`, `--run`, `--attempt` | Live-verified unapproved receipt and its local introduction commit |
| `scripts/receipt_verifier.py live` | `--planning`, `--revision`, `--receipt`, `--candidate`, `--plan` | Exact committed receipt verified against live GitHub evidence |
| `scripts/receipt_verifier.py approve` | Same arguments, authenticated Owner workflow context | Fresh live verification followed by the sole approval edit and local commit |
| `scripts/receipt_verifier.py retained` | Same arguments, trusted protected Planning revision | Offline verification of the approved history and required candidate/plan |

The scripts use Python standard libraries, Git and GitHub CLI. The REST header is `X-GitHub-Api-Version: 2026-03-10`. Git/API/schema/identity failures exit nonzero. Output contains references only. A clean Planning checkout and configured Git author are required for writes.

The workflow caller must publish each local commit by an ordinary fast-forward push and check its result before reporting publication or running the next stage. A rejected push is not success: fetch current Planning history and retry the scoped operation from a clean checkout, preserving concurrent work. Identical writer retries return the original introduction commit; conflicting content blocks. PR 1 does not install a publisher workflow.

## Live witness and Owner approval

The live verifier requires the expected repository numeric identity, workflow, event, latest attempt 1, successful overall run and uniquely successful gate/plan jobs. API `head_sha` alone cannot prove the tested integration commit. Therefore the verifier reads the existing, uniquely named `trusted-pr-plan` artifact only while live: bounded ZIP, one `pr-plan.json`, no extraction, exact base/head/integration binding and selected pilot gate. It retains no artifact or contents. Missing/expired/ambiguous evidence blocks approval. This small live read closes a source-identity gap that copied API metadata and Git history alone cannot close.

Approval is an explicit Owner decision through `workflow_dispatch` on protected `main`. Trusted code checks `GITHUB_ACTOR` and `GITHUB_TRIGGERING_ACTOR`, then performs live verification again. The sole allowed edit replaces `approved:null` with `by`, UTC `at` and `receipt_commit`, the exact introduction commit. A workflow must not accept caller-provided identity variables as authorization. Fork/contributor jobs receive no Planning write token; the privileged job executes only protected source on a clean runner.

After approval, the retained verifier never calls GitHub. The caller must obtain `--revision` from the protected Planning branch, never from a PR-supplied branch or arbitrary local history. It requires the unchanged original receipt and exactly one approval edit; later edits, including edits subsequently reverted, block. Protected Planning writers are trusted: Git history provides no independent cryptographic proof of their honesty, and complete repository loss is not covered.

## Bootstrap and acceptance

PR 1 contains these scripts, focused tests and EN/DE documentation only. It qualifies through the existing legacy path. The exact target-side path mapping lives in `scripts/pr-gate-plan.py`: the receipt control-plane paths select the existing planner checks, and the receipt control-plane test paths additionally require the `receipt-execution-tests` check. That check is a trusted runner profile that executes the real receipt suites (`test_receipt_writer.py`, `test_receipt_verifier.py`, `test_receipt_chain.py`, `test_receipt_tasklink.py`, `test_ci_workflow_policy_receipts.py`) under `PYTHONPATH=scripts` and fails closed when a suite does not actually run, so the selected CI lane now executes the receipt result rather than merely matching a filename. The check is wired into the focused selected lane in `.github/workflows/ci.yml`; removals and renames of the receipt paths stay unresolved and unresolved coverage is never replaced with an exemption.

PR 2 proves one ordinary contributor PR end to end: producer, writer, published receipt, live verifier, Owner approval, automatic task link and shadow final-check consumption. The post-run completion cannot be awaited by the producer workflow that triggers it. PR 3 cuts over only the accepted consumption path, then further gates may follow.

Acceptance requires one genuine successful GitHub path and three rejected cases: wrong SHA, dead run ID, edited receipt. Local tests use real temporary Git histories and fake GitHub responses; they are not hosted qualification. The retained positive also runs with API access forbidden. No full runtime matrix is needed for this code.

Developer summaries have at most three lines: checked, validly reused, not required with a reason. Not-required is a plan decision, never a PASS receipt. Reuse selection and summary workflow integration are later work; this PR does not implement a second evidence authority.

<!-- shadow-run contributor receipt probe 2026-10-28 -->
