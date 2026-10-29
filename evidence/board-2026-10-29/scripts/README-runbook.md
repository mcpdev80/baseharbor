# r27 shadow-run chain — runbook (lane o204)

This runbook describes how to run the **real** hosted r27 shadow chain. The
scripts it references (`chain-run.sh`, `negative-proofs.sh`) are currently only
executable in **dry / offline mode**; the hosted positive chain is armed only
after the product PRs **#1200** and **#1201** merge (see gaps below).

## Contracts this runbook is built on (verified 2026-10-29)

The harness invokes the **real** BaseHarbor scripts, so its contract is the
source of truth. Never invent flags — if a script's argparse differs from this
file, fix the file to match the script, not the reverse.

### `receipt_tasklink.py` (`baseharbor/scripts/receipt_tasklink.py`)

```
--planning <dir>     Planning repo checkout
--workflow-id <int>  CI workflow definition id of the producer run
--head <40hex>       PR head sha
--candidate <40hex>  integration/candidate sha   (pr-plan.json → integration_sha)
--base <40hex>       PR base sha                 (pr-plan.json → target_base_sha)
--pr-number <n>      auto-resolves the task link (needs GITHUB_TOKEN)
--task <ref>         explicit task link          (alternative to --pr-number)
--dry-run            print the fixture to stdout, never commit
```

`--task` and `--pr-number` are mutually exclusive; omitting both is an error.
`extract_plan` refuses any run attempt other than **1**.

### `receipt_verifier.py` (`baseharbor/scripts/receipt_verifier.py`, mode `retained`)

```
retained --planning <dir> --revision <sha> --receipt <path> \
         --candidate <40hex> --plan <sha>
```

Passes only when the committed receipt at the canonical path
`evidence/receipts/<gate>/<run-id>-<attempt>.json`, verified against the Git
history at `--revision`, matches `--candidate` and `--plan`, is canonical, and
carries exactly one Owner approval edit introduced at `--plan`. Any wrong
candidate, wrong plan revision, edited approval, or non-committed receipt fails
closed (exit 1). `--plan` is the plan-fixture commit; the receipt's `plan`
field must equal it.

## Known gaps that keep the hosted chain dormant (from producer-diagnostics)

1. **TRUSTED_SOURCE** — `receipt-approval.yml` helper code lives on `develop`
   (not on remote `main`), so the reviewed controller can't be resolved. Need
   the pinned reviewed controller `dd8fb61d614b59252c9d29121380b5893d006828`
   registered as `TRUSTED_SOURCE` on `main`.
2. **Workflow registration** — `receipt-approval.yml` / `receipt-post-run.yml`
   are develop-only; only `validate.yml` is registered under
   `.github/workflows/` on `origin/main`.
3. **Token** — `PLANNING_RECEIPT_WRITE_TOKEN` secret is absent, so
   `receipt_writer.publish` fails closed. Owner must add the scoped write token.
4. **Auto-trigger arming** — `receipt-post-run.yml` fires on `workflow_run` of
   "CI · Source & Quality" (run 40833893562 → workflow_id 352609139). The
   trigger is armed; only the post-run publication token blocks it.

## Step 1 — positive dry-run against a real producer run

```bash
export GITHUB_TOKEN="$(gh auth token)"          # gh auth login if empty
evidence/board-2026-10-29/scripts/chain-run.sh 40833893562 1 --dry-run --pr-number 1199
```

Expected (run 40833893562, known-good inputs):

```
chain-run: run=40833893562 attempt=1 workflow_id=352609139 pr=1199
chain-run: integration=8515e6272f03aa274ed3ea215143b9e93680be71 \
           base=9e50d8a4... head=96a96362...
tasklink: auto-resolved task from PR 1199: tasks/t-20261018-r27-artifact-resolution.md
{"base_sha":"9e50d8a4...","candidate_sha":"8515e627...","gate":"public-contracts",
 "head_sha":"96a96362...","repository":"mcpdev80/baseharbor",
 "task":"tasks/t-20261018-r27-artifact-resolution.md","workflow_id":352609139}
5ecabc17797aaf21a454cb83fab20e4ddc49eb7c
chain-run: OK (dry)
```

Guards: `--publish-main` is refused; `attempt != 1` is refused; a fake
`refs/heads/main` in any `*_REF` env is refused.

## Step 2 — negative proofs (offline, no hosted dispatch)

```bash
evidence/board-2026-10-29/scripts/negative-proofs.sh
```

Three bounded cases, each asserting the intended rejection:

1. **wrong candidate sha** → `receipt does not match required candidate/plan`
2. **plan revision that never holds the fixture** → same rejection
3. **edited approval object** (tampered status) → canonical() mismatch

Each must exit non-zero (`PASS … (exit 1)`). The proof workspace is built once
under `/home/marcel/tmp-ops/o204/proof`.

## Step 3 — the hosted positive chain (AFTER #1200 + #1201 merge)

After the product PRs merge and the four gaps above are closed, the real chain
runs the producer run, then the post-run writer, then the Owner approval, then
the retained shadow. Commands (adapt run id / pr number / token as needed):

```bash
# (a) producer dry-run / apply — mirrors chain-run.sh without --dry-run once
#     TRUSTED_SOURCE is registered and the token exists:
export GITHUB_TOKEN="$(gh auth token)"
PLANNING_RECEIPT_WRITE_TOKEN="<scoped write secret>"
evidence/board-2026-10-29/scripts/chain-run.sh <RUN_ID> 1 --pr-number <PR>

# (b) Owner approval via the hosted workflow (receipt-approval.yml):
gh actions workflow-dispatch \
  -r mcpdev80/baseharbor \
  -w receipt-approval.yml \
  --event-name workflow_dispatch \
  -f run_id=<RUN_ID> \
  -f candidate_sha=8515e6272f03aa274ed3ea215143b9e93680be71 \
  -f base_sha=9e50d8a4ab6c358ad45da6b337c0e67c64373264 \
  -f head_sha=96a96362d3682bdf35748c5331a6f2452c01b4c8 \
  -f workflow_id=352609139 \
  -f pr_number=<PR> \
  -f plan_revision=<plan-revision-sha>

# (c) retained check of the approved publication (offline, exact published inputs):
python3 evidence/board-2026-10-29/scripts/receipt_verifier.py retained \
  --planning <planning-checkout> \
  --revision <approved-publication-sha> \
  --receipt evidence/receipts/public-contracts/<RUN_ID>-1.json \
  --candidate 8515e6272f03aa274ed3ea215143b9e93680be71 \
  --plan <plan-revision-sha>
# expected: "checked: public-contracts"

# (d) retained check with a wrong candidate (must be rejected):
python3 evidence/board-2026-10-29/scripts/receipt_verifier.py retained \
  --planning <planning-checkout> \
  --revision <approved-publication-sha> \
  --receipt evidence/receipts/public-contracts/<RUN_ID>-1.json \
  --candidate deadbeefdeadbeefdeadbeefdeadbeefdeadbeef \
  --plan <plan-revision-sha>
# expected: "receipt blocked: receipt does not match required candidate/plan"
```

`--plan` is always the **plan fixture revision** (the commit that holds
`evidence/receipt-plans/<gate>.json`); the receipt's `plan` field must equal it.

## Cleanup

The harness creates scratch resources under `/home/marcel/tmp-ops/o204/`:

```bash
rm -rf /home/marcel/tmp-ops/o204/{wt-shadow,plan-scratch,proof,proof-edited,art-*}
```

No product branch is ever touched; the planning clone is never pushed.
