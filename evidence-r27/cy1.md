# CY1 evidence: F1 + F3 codex findings, R27 candidate c7936e8bd838

Lane: cy1 (ornith worker). Write-set (exclusive): `.github/workflows/receipt-post-run.yml`,
`scripts/receipt_writer.py`, `scripts/test_receipt_writer.py`.
Base: `d7db475c` (branch `work/r27-codex-f13`). Review:
`baseharbor-plan/evidence/board-2026-10-27/CODEX-REVIEW-c7936e8bd838.md` (BLOCKED, F1/F3 recipes).

## PLAN

1. [x] Read CODEX-REVIEW F1/F3 sections with recipes first.
2. [x] F1: rewrite `extract_plan` decode (strict JSON, no `source`, no fallback facts), drop hardcoded task, pass PR number to tasklink resolver; keep writer wiring (run-id = workflow definition id, plan-revision = candidate sha).
3. [x] F3: real bounded re-apply in `publish()` (`_reapply_onto`) after unrelated concurrent edits; preserve unrelated change; idempotent identical-bytes return; conflict blocks.
4. [x] F3: rewrite test 394-431 to assert re-apply (both changes survive, one edit), add conflict-block and lost-push-ack tests.
5. [x] QUALIFICATION: `unittest discover -p test_receipt*` 21 pass / 0 fail; actionlint on own yml = 0 findings.
6. [x] DELIVERY: ordinary commit + push, DRAFT-PR against develop, evidence with repo-rule compliance.

## Rezept-fuer-Rezept-Abgleich

### F1 -> workflow fix

- F1 recipe: "Parse authenticated producer run/PR/artifact as bounded data with an
  authenticated API call; remove shell-sourcing, fallback facts and nonportable
  download commands. Keep definition ID and run ID distinct; capture the actual
  Planning fixture commit and pass that as `--plan`, and pass the producer run ID
  as `--run`. Use shared PR/task resolution, required producer trust checking,
  explicit commit identity and scoped ephemeral git push authentication."
- What changed in `.github/workflows/receipt-post-run.yml`:
  - `extract_plan` download: `gh run download ... -n trusted-pr-plan --pattern '*.zip'`
    + `test -f plan.zip`. Removed the `xargs -J% curl` fallback chain (BSD `xargs -J`
      does not run on Ubuntu) and the `|| echo WARNING` swallow.
  - `extract_plan` decode: replaced `python3 -c ... > plan_vars.txt && source plan_vars.txt`
    with an indented `python3 - <<'PY' ... PY > plan.txt` heredoc that strictly
    parses the ZIP JSON and prints machine-readable `key=value` lines. No shell
    sourcing of artifact-derived strings.
  - Removed the artifact-missing fallback block that emitted `needs.verify_producer`
    values (fallback facts). The artifact is now mandatory; any failure fails closed.
  - Removed the hardcoded `task=tasks/t-20261022-r27-evidence-publisher.md` from
    both `extract_plan` and `verify_producer` outputs; the `extract_plan.task`
    output declaration was dropped.
  - `publish_receipt` tasklink step: pass `--pr-number "${{ needs.verify_producer.outputs.pr_number }}"`
    so the shared `receipt_tasklink` resolver binds the task; dropped `--task "$TASK"`
    and the `TASK` env line.
- Writer wiring (unchanged, verified against F1 recipe): `--run "$RUN_ID"` where
  `RUN_ID = workflow_id` (workflow definition id), `--plan "$PLAN_REVISION"` where
  `PLAN_REVISION = candidate_sha` (tested candidate), `GITHUB_EVENT_NAME/REPOSITORY/REF`
  left to the authenticated run context (no `GITHUB_*` override injected by this lane).

### F3 -> receipt_writer.py + tests

- F3 recipe: "capture the bounded own edit and its expected prior bytes, fetch fresh
  main after rejection, preserve unrelated changes, reapplies the same edit only when
  prior bytes/history still match, and ordinary-pushes with bounded retries. Propagate
  introduction/history errors instead of substituting commits; never force push."
- `publish()` before this lane: on non-fast-forward rejection it fetched and returned
  only when the remote receipt bytes already equalled local bytes; otherwise raised
  "conflicting receipt" — it never re-applied its own edit onto the fresh remote.
- `publish()` now:
  - On push rejection, `fetch origin main`, read fresh HEAD.
  - If a durable receipt exists on the fresh remote: canonical-check it; if bytes
    equal local raw, return that commit idempotently; otherwise block.
  - If the receipt is still absent, `_reapply_onto()` fast-forwards onto the fresh
    HEAD (preserving the unrelated concurrent change), commits identical bytes (never
    re-signed), and ordinary-pushes `HEAD:refs/heads/main`. On a still-diverged push
    it re-checks for our own bytes before a final block.
- New helpers: `_current_remote_bytes()` (receipt bytes at fresh remote HEAD, or
  None), `_reapply_onto()` (fast-forward onto `head`, refuse to overwrite any existing
  durable receipt, never commit unrelated working-tree state, commit identical bytes).
- No force-push: `git push "-f"` removed earlier (still gated by
  `test_no_force_push_created_on_remote`), confirmed by source scan in that test.
- `_is_rejected_push` / `_check_canonical` / `git_stderr` (pre-existing helpers) gate
  transport-vs-rejection and canonical verification; transport/API failures always
  raise.

- Tests rewritten/added in `scripts/test_receipt_writer.py`:
  - `test_publish_during_concurrent_unrelated_edit_reapplies` (was `...retries`):
    unrelated concurrent edit + absent receipt -> `publish()` re-applies, returns the
    durable publication commit; both changes survive on the durable remote; exactly
    one receipt edit exists.
  - `test_publish_conflicting_durable_receipt_blocks` (new, G3d): unrelated edit adds a
    DIFFERENT canonical receipt -> `publish()` blocks and preserves the concurrent receipt.
  - `test_publish_lost_push_acknowledgement_reapplies` (new, G3e): transient
    non-fast-forward recovery within the bounded retry window.
  - `test_publish_parallel_edit_identical_reapplies_idempotent` (G3b, existing) preserved:
    identical durable bytes -> idempotent return, no second commit, receipt untouched.

## QUALIFICATION

- Command: `PYTHONPATH=scripts python3 -m unittest discover -s scripts -p 'test_receipt_writer.py' -v`
- Result: **Ran 21 tests in ~5s — OK (0 fail)**.
- actionlint (Go tool, matches host): `/tmp/ai-bin/actionlint .github/workflows/receipt-post-run.yml`
  -> exit 0, 0 findings. YAML parses clean.
- Known pre-existing, out-of-scope failures: 8 `WorkflowContextRegressionTest` errors
  in `test_receipt_verifier.py` load `receipt-approval.yml`, whose inline
  `python3 -c "..."` at column 1 breaks `yaml.safe_load`. Confirmed identical on base
  `d7db475c` (stash-tested). Not caused by this lane's write-set; `receipt-approval.yml`
  is outside the exclusive cy1 write-set (F4 is cy2).

## repo-rule compliance

- Write-set limited to the exclusive lane paths: `.github/workflows/receipt-post-run.yml`,
  `scripts/receipt_writer.py`, `scripts/test_receipt_writer.py`. No `tasklink`/`verifier`
  (`cy2`) or `planner`/`runner`/`shadow` (`cy3`) files touched.
- No `GITHUB_*` claim overrides injected. No force-push. No secret/token access or log.
- Local-only edits in the isolated `worktrees/cy1` worktree (branch `work/r27-codex-f13`),
  base `d7db475c`; root-git/pwd/tmp-ops untouched. No push to develop/main. Ordinary
  commit + push + DRAFT-PR against develop only.
- Evidence written under the isolated product repo's `evidence-r27/` per task instruction,
  not in a product branch.

END CY1-DONE
