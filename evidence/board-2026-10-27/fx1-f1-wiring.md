# F1 Wiring Audit Dossier — Lane fx1 (ornith, read-only)

Candidate SHA: `94f35d02` (stg6: integrate pcx FIX A (corrected) + f1x plan-facts fix (corrected))
Audit method: `git worktree add /tmp/fx1-audit 94f35d02`, all greps/sed against that checkout.
Note: `worktrees/fx1` already existed in this repo at `839e5cc9` (branch `work/r27-fix-approval-joboutputs`), so the prompt's `git worktree add ... --detach 94f35d02` hit "already exists" — reported by the LANE as `END FX1-BOOTFAIL`. The worktree dir was occupied; audit executed against the same candidate SHA in a separate temp worktree instead. Read-only throughout; no gh/PR/rerun/product writes.

---

## A) Jobs that call receipt_* or pr-gate scripts (.github/workflows/ at 94f35d02)

| # | Workflow file | Job name (line) | Step / invoked script | Invoked path (line) | Path EXISTS in worktree |
|---|---------------|-----------------|-----------------------|---------------------|--------------------------|
| 1 | ci.yml | `pr_plan` (431) | resolve trusted PR inputs | `scripts/pr-diff-base.py` (~457) | YES |
| 2 | ci.yml | `pr_plan` (431) | resolve trusted PR inputs | `scripts/pr-gate-plan.py` (458) | YES |
| 3 | ci.yml | `pr_plan` (431) | resolve trusted PR inputs | `scripts/pr-gate-dependencies.py` (465, via `git show`) | YES |
| 4 | ci.yml | `pr_plan` (431) | resolve trusted PR inputs | `scripts/release-requirements.json` (466) | YES |
| 5 | receipt-approval.yml | `approve` (49) | approve receipt | `scripts/receipt_verifier.py` (90) | YES |
| 6 | receipt-post-run.yml | `extract_plan` (64) | extract facts | `import receipt_verifier as rv` (134) | YES |
| 7 | receipt-post-run.yml | `publish_receipt` (169) | automatic task-link | `scripts/receipt_tasklink.py` (248) | YES |
| 8 | receipt-post-run.yml | `publish_receipt` (169) | publish receipt | `scripts/receipt_writer.py` (291) | YES |
| 9 | receipt-shadow.yml | `receipt_shadow` (58) | retained receipt | `scripts/receipt_verifier.py` (122) | YES |

All other workflows in `.github/workflows/` (artifact-gc, docker-runtime-acceptance, host-trust-owned-e2e, mcp-lifecycle-acceptance, nextjs-greenfield-e2e, observability-acceptance, pages, podman-acceptance, pre-release-runtime-suite, pre-release, release, renovate, runtime-broker, runtime-image, runtime-s3-acceptance, supplemental-release-validation, targeted-demo-acceptance, targeted-integration) contain NO `receipt_*` / `pr-gate` invocation.

Note: `scripts/pr-check-runner.py` is invoked in ci.yml `pr_plan` (line 460, `matrix`) and exists; it is the execution-side mirror (its `RECEIPT_EXECUTION_SUITES` at scripts/pr-check-runner.py:103 lists the receipt suites). `scripts/pr-gate-plan.py` at scripts/pr-gate-plan.py:660 defines `RECEIPT_CONTROL_PLANE_SCRIPTS` and `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS`.

## B) discover -p 'test_receipt_postrun*'

`discover` is NOT a python entrypoint in this repo (`scripts/discover.py` does not exist; `which discover` empty). Answered by direct source scan instead.

`test_receipt_postrun.py` (22990 bytes) EXISTS in the candidate at `scripts/test_receipt_postrun.py` and in current `baseharbor` HEAD.

Is `test_receipt_postrun.py` wired into CI / workflows? YES, via the planner + check-runner gate, NOT a direct `python3 scripts/test_receipt_postrun.py` step in any yml:
- `scripts/pr-gate-plan.py` `RECEIPT_CONTROL_PLANE_TEST_SCRIPTS` (scripts/pr-gate-plan.py:662) includes `scripts/test_receipt_postrun.py`.
- `CI_CHECKS[].command` for `receipt-execution-tests` (scripts/pr-gate-plan.py:681) runs `python3 -m unittest ... scripts/test_receipt_tasklink.py scripts/test_receipt_postrun.py`.
- `scripts/pr-check-runner.py` `RECEIPT_EXECUTION_SUITES` (scripts/pr-check-runner.py:104) includes `test_receipt_postrun`.
- `scripts/test_pr_gate_plan.py` references `scripts/test_receipt_postrun.py` at lines 1207 and 3587.

Direct `test_receipt_postrun` references in `.github/workflows/`: NONE (grep of `.github/workflows/` returns only receipt_verifier/tasklink/writer invocations). So: wired-into-gate = JA, direct-yml-step = NEIN.

## C) "F1 core wired" — was ist verdrahtet, was nur getestet

Verdrahtet (executed by CI gate / live workflows at 94f35d02):
- Receipt control-plane scripts exist and are invoked: `receipt_verifier.py` (approval + shadow), `receipt_tasklink.py` (post-run), `receipt_writer.py` (post-run) — receipts 6-8 above.
- PR-gate planning: `pr-diff-base.py`, `pr-gate-plan.py`, `pr-gate-dependencies.py`, `pr-check-runner.py`, `release-requirements.json` in `pr_plan` — 1-4 above.
- Test suites executed through the `receipt-execution-tests` gate: `test_receipt_writer/verifier/chain/ci_workflow_policy_receipts/tasklink/postrun` — all exist, `test_receipt_postrun.py` confirmed present.

Nur getestet / nicht als direktes yml-Step verdrahtet:
- `scripts/test_receipt_postrun.py` exists and runs under the `receipt-execution-tests` CI gate, but there is no direct `python3 scripts/test_receipt_postrun.py` line inside any workflow yml; its execution is indirect via `pr-check-runner.py execute --check receipt-execution-tests` (scripts/pr-check-runner.py:108 whitelist).

End FX1-DONE
