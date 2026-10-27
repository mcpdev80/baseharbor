# X3 SHADOW — single receipt shadow-run COMMAND PLAN (paper, no activation)

ROLE: X3 PR2 shadow-run command planner (plan only, no activation). This document
is the exact chronological command chain for ONE genuine end-to-end receipt run
of the PR2 shadow chain: `runner -> writer -> receipt-commit -> fresh-live-
verification -> Owner-approval -> automatic task link -> final-check
consumption`. No command here is executed in this mission; this is the Owner
dispatch plan.

Author: Codeman, AgentGateway mission lane for `ops/missions/x3.txt`.
UTC host time confirmed via turn-start (`basics-check.py`) — see §0.
Write scope: this evidence file in `evidence/board-2026-10-26/` only. No product
file changed, no Planning write, no commit, no push.

## 0. Sources and ground truth (verified in this run)

- `ops/missions/x3.txt` — the task contract (plan-only shadow command plan).
- `evidence/board-2026-10-26/p2-shadow-runbook.md` — paper chain, steps 0-5.
- `baseharbor-plan/evidence/r27-ci-replacement-2026-10-26/PR2-SHADOW-PREP.md`.
- `baseharbor-plan/tasks/t-20261026-r27-receipt-pilot.md` (state: waiting,
  depends on `t-20261017-release-0-4-27-updates` (NOT accepted) +
  `t-20261022-r27-evidence-contract` (accepted)).
- `baseharbor-plan/decisions/2026-10-26-minimal-receipts.md`.
- Frozen interfaces pinned from **ec82d476** (`ci: add minimal one-gate receipt
  writer and verifier`): `baseharbor/scripts/receipt_writer.py` (writer CLI
  `:48-63`, `write()` `:35-45`, `commit_file()` `:17-23`) and
  `baseharbor/scripts/receipt_verifier.py` (`load_plan` `:101-112`, `live`
  `:127-203`, `retained` `:217-231`, `approve` `:234-252`, `receipt()`
  schema `:60-87`, `github()` `:114-123`).
- EN/DE docs: `docs/development/minimal-ci-receipts.md`, `docs/de/development/minimal-ci-receipts.md`.

ec82d476 is the parent of the current PR1090 draft: PR1090 HEAD `e12bd1fd` adds
the three dormant ymls, `pr-gate-plan.py`, `test_pr_gate_plan*`, the second
receipt test suite, the allowlist patch and doc fixes on top of the frozen
two-script + one-test skeleton at ec82d476. The pinned frozen baseline therefore
is the PR1 one-gate skeleton (writer + verifier + `test_receipt_chain.py`).

## 1. Precheck list (run BEFORE the chain; any NO except PR2-merged blocks)

| # | Check | Command | Expected |
|---|-------|---------|----------|
| P0 | Host UTC time sane | `date -u +%Y-%m-%dT%H:%M:%SZ` + `basics-check.py` turn-start | monotonic, no clock-jump flagged |
| P1 | gh authenticated as `mcpdev80` | `gh auth status` | account `mcpdev80`, SSH git |
| P2 | PR1 (1090) accepted | `gh pr view 1090 --repo mcpdev80/baseharbor --json state,mergedAt,mergeCommit` | **state=OPEN, mergedAt=null** — currently NOT accepted (see §2) |
| P3 | PR2 merged? | `gh pr list --repo mcpdev80/baseharbor --search "receipt in:title" --json number,state,mergedAt` | PR1090 is the receipt PR; merged? currently **OFFEN** (see §2) |
| P4 | Protected Planning revision pinned? | `gh api repos/mcpdev80/baseharbor-plan/branches/main --jq '.protection.enabled'` | Planning `main` branch protection is **disabled** (`enabled:false`) — no enforced protection/pin; the pinned receipt is the frozen ec82d476 baseline, not a protected Planning revision (see §3) |
| P5 | Frozen scripts present & runnable | `git -C worktrees/p2-ustrack show ec82d476:scripts/receipt_writer.py >/dev/null && python3 worktrees/p2-ustrack/scripts/test_receipt_chain.py` | 6 tests OK |
| P6 | Clean Planning checkout for the run target | `git -C <plan-checkout> status --porcelain` | empty (clean) |
| P7 | Git author configured in run target | `git -C <plan-checkout> config user.email` | non-empty (writes need an author) |
| P8 | `gh` can reach the run provenance for the chosen run_id | `gh api "repos/mcpdev80/baseharbor/actions/runs/<run_id>"` | 200 + successful run (for a real run) |

Abort-before-run: **P2 = NO** (PR1 not accepted), **P3 = OFFEN** (no genuine
PR2 run yet), **P4 = no protected Planning revision pinned**. Per the pilot card
and p2-shadow-runbook §3 these are the open pre-PR2 gates. This command plan is
the paper runbook that becomes executable only when P2/P3 flip and a real run_id
with live provenance exists.

## 2. Current board state at plan time (2026-10-26T23:4xZ)

- **PR1 (1090) "ci: minimal one-gate receipts" — OPEN, not accepted.**
  `state=OPEN`, `mergedAt=null`, `mergeable=MERGEABLE`, no reviews. Branch
  `work/r27-minimal-receipt` → `develop`. HEAD `e12bd1fd`. Files added on top of
  frozen ec82d476: 3 dormant ymls, `pr-gate-plan.py`, `test_pr_gate_plan*.py`,
  2nd receipt test suite, `ci-workflow-policy.sh` allowlist patch, EN/DE docs.
  The pilot card's acceptance requires "accepted PR1 one-gate skeleton"; PR1 is
  not yet accepted by the Maintainer. Decision doc pinned at
  `decisions/2026-10-26-minimal-receipts.md`.
- **PR2 (receipt shadow run) — no genuine run exists.** The three receipt ymls
  are DORMANT in PR1090 (`receipt-post-run.yml`/`receipt-approval.yml` carry a
  never-true `if:` guard). No real Actions run has produced a
  `trusted-pr-plan` artifact, so no `<run_id>` with live provenance exists to
  drive the writer. P3 = OFFEN.
- **Protected Planning revision — not pinned.** Planning `main` branch
  protection is disabled (`enabled:false`); there is no enforced protected
  revision. The authoritative "pinned" reference in this ecosystem is the frozen
  baseharbor baseline **ec82d476**, which both scripts write into / read from in
  the Planning checkout (`--planning <baseharbor-plan checkout>`). `--revision`
  for `approve`/`retained` must come from a clean protected Planning branch,
  never a PR-supplied branch.
- Related tasks: `t-20261026-r27-receipt-pilot` (waiting, this pilot),
  `t-20261022-r27-evidence-contract` (accepted = PR1 skeleton),
  `t-20261022-r27-evidence-acceptance` (active), `t-20261022-r27-evidence-
  resolver` (active).

## 3. Repo layout the chain walks on

- Product repo `baseharbor`: `scripts/receipt_writer.py` (writer),
  `scripts/receipt_verifier.py` (live/approve/retained), the producer `ci.yml`
  with the uniquely named gate job `PR · Public Contracts` and the
  `trusted-pr-plan` artifact.
- Planning repo `baseharbor-plan`: the **write target** of both scripts.
  Receipts → `evidence/receipts/public-contracts/<run>-<attempt>.json`; the
  bounded plan fixture → `evidence/receipt-plans/public-contracts.json` (read by
  SHA via `--plan`). Both scripts pure stdlib + git + `gh`; GitHub REST header
  `X-GitHub-Api-Version: 2026-03-10`. No secrets, no signing, no archive service.

## 4. Exact command chain (one genuine run)

Shared across all four calls: `--planning <baseharbor-plan checkout>`,
`--plan <commit that contains the plan fixture>` (the trusted-planner's plan
commit), `--candidate <candidate_sha>` (the tested integration SHA, 40 hex), and
the receipt path `evidence/receipts/public-contracts/<run>-1.json`.

### Step 0 — producer (existing trusted planner, unchanged)
Not a receipt script. The existing Actions producer run emits the uniquely named
bounded artifact `trusted-pr-plan` (one `pr-plan.json`, schema
`baseharbor.pr-gate-plan/v1`, `coverage_status: resolved`,
`selected_gates` contains `integration/static/public-contracts`, correct
head/base/integration binding). This is the witness the live verifier reads.
Command: **the producer workflow itself** — not a local command. It must run a
successful `pull_request` run on `ci.yml` producing the artifact. Until the
dormant ymls are activated by PR2 commits this step does not exist.

### Step 1 — runner → writer (receipt_writer.py:48-63, write():35-45)
A separate `workflow_run` post-run caller (`receipt-post-run.yml`, DORMANT)
invokes the writer. Exact call:

    python3 scripts/receipt_writer.py \
      --planning <path-to-baseharbor-plan checkout> \
      --plan <full SHA of the commit that contains the plan fixture> \
      --run <run_id> --attempt 1
    env: GITHUB_EVENT_NAME=workflow_run,
         GITHUB_REPOSITORY=mcpdev80/baseharbor,
         GITHUB_REF=refs/heads/main

`write()`: `load_plan` validates the fixture by SHA (verifier `:101-112`); builds
canonical receipt `{"gate":"public-contracts","run":{"id":run,"attempt":1},
"sha": candidate_sha, "status":"pass","plan":<plan>,"approved":None}`;
`receipt()` enforces strict schema + canonical bytes + `<500` bytes;
`live(...,api=github)` performs fresh live verification (`:127-203`);
`commit_file()` writes `evidence/receipts/public-contracts/<run>-1.json` in the
Planning checkout and commits `receipt: public-contracts record` → returns
`<receipt_commit>` (the introduction commit). Idempotent: a second identical
call returns the SAME introduction commit (`commit_file.py:17-23`); conflicting
content or a dirty checkout blocks (ReceiptError, nonzero exit).

Expected artifact after Step 1: `evidence/receipts/public-contracts/<run>-1.json`
committed as `receipt: public-contracts record`; print `receipt: <receipt_commit>:<path>`.

### Step 2 — fresh live verification (receipt_verifier.py:127-203, inside writer AND standalone)
Standalone live read (no edit):

    python3 scripts/receipt_verifier.py live \
      --planning <plan checkout> --revision <any Planning commit> \
      --receipt evidence/receipts/public-contracts/<run>-1.json \
      --candidate <candidate_sha> --plan <plan SHA>

`live()` requires: `approved is None`, `attempt==1`, `sha==candidate_sha`, correct
run identity/workflow/event, successful run, exactly one `PR · Public Contracts`
job + one `PR · Trusted Plan` job (both successful, correct head_sha/run_id),
bounded `trusted-pr-plan` artifact (not expired, right workflow_run/head_sha),
unzipped `pr-plan.json` binding to head/base/integration. ZIP read only during
the live window. Any API/git/schema/identity failure → nonzero exit.

Expected artifact: stdout `checked: public-contracts`. Any mismatch → nonzero exit, chain stops.

### Step 3 — Owner-approval commit (receipt_verifier.py:234-252) — the ONLY manual/privileged step
    python3 scripts/receipt_verifier.py approve \
      --planning <plan checkout> --revision <introduction receipt commit> \
      --receipt evidence/receipts/public-contracts/<run>-1.json \
      --candidate <candidate_sha> --plan <plan SHA>
    env: GITHUB_ACTOR=mcpdev80, GITHUB_TRIGGERING_ACTOR=mcpdev80,
         GITHUB_EVENT_NAME=workflow_dispatch,
         GITHUB_REPOSITORY=mcpdev80/baseharbor,
         GITHUB_REF=refs/heads/main

Re-verifies, re-runs live, then the sole allowed edit: `approved:null ->
{by:"mcpdev80", at:"<UTC ISO8601 Z>", receipt_commit:<introduction commit>}` and
commits `receipt: public-contracts approve`. Signing party: Owner `mcpdev80` only
via `workflow_dispatch` on protected `main` from a clean privileged runner; both
`GITHUB_ACTOR` and `GITHUB_TRIGGERING_ACTOR` must equal the Owner (identity gate,
`approve()` `:235-240`). `--revision` must be the introduction receipt commit from
the protected Planning branch, never a PR-supplied branch. Edit is atomic: only
the single `approved` field changes; `commit_file(expected=raw)` blocks anything
else.

Expected artifact: receipt now carries the Owner approval block, committed as
`receipt: public-contracts approve`; stdout `checked: public-contracts`.

### Step 4 — automatic task link (PR2, planning side — NOT a baseharbor script)
Produced by the existing trusted planner inside the post-run workflow
(`receipt-post-run.yml`), which regenerates the bounded plan fixture
`evidence/receipt-plans/public-contracts.json` with `"task":"tasks/<task-id>.md"`
from the real Planning task, BEFORE the writer runs. The writer consumes it via
`load_plan`/`receipt["plan"]`. This is a bounded workflow action, not a separate
producer — a separate producer is a KLARER FEHLBAUSTEIN (mission item 3 +
CODEMAN-START; documented in p2-shadow-runbook §3 and PR2-SHADOW-PREP §3/§5).
Fixture contract enforced by `load_plan` (`:101-112`): exactly 7 fields
(gate, repository, workflow_id, head_sha, candidate_sha, base_sha, task), 40-char
SHAs, task `tasks/[a-z0-9-]+\.md`. No command executes here in this run; it is
the pre-writer step inside Step 1's producer workflow.

### Step 5 — final-check consumption, shadow (retained, receipt_verifier.py:217-231)
Not yet cutover (that is PR3). Shadow consumes in retained mode (never calls
GitHub, binds approved history locally):

    python3 scripts/receipt_verifier.py retained \
      --planning <plan checkout> --revision <commit after the approval edit> \
      --receipt evidence/receipts/public-contracts/<run>-1.json \
      --candidate <candidate_sha> --plan <plan SHA>

`retained()` requires: `approved is not None`; approval commit descends from the
introduction commit; original receipt bytes unchanged at the introduction commit;
introduction commit introduced the path (first edit); exactly one edit since
(the approval write) with no later edits (even reverts block); approval-commit
blob equals current bytes.

Expected artifact: stdout `checked: public-contracts` — offline final-check
consumption of the Owner-approved receipt.

### Full paper chain (arguments aligned)

    runner:  writer        --planning P --plan <planSha> --run <runId> --attempt 1   (env: workflow_run on main)
    verify:  verifier live --planning P --revision <planSha> --receipt .../<run>-1.json --candidate <candSha> --plan <planSha>
    approve: verifier approve --planning P --revision <receiptIntroSha> --receipt .../<run>-1.json --candidate <candSha> --plan <planSha>  (env: workflow_dispatch, ACTOR=mcpdev80)
    consume: verifier retained --planning P --revision <afterApproveSha> --receipt .../<run>-1.json --candidate <candSha> --plan <planSha>

All four share `--planning` (Planning checkout), `--plan` = the plan-fixture
commit, `--candidate` = the tested integration SHA, and the receipt path
`evidence/receipts/public-contracts/<run>-1.json`.

## 5. Where the Owner-dispatch exactly happens

The single Owner dispatch is **Step 3** (`receipt_verifier.py approve`): the only
step requiring a human-owned privileged action. It fires on `workflow_dispatch`
on protected `main` from a clean privileged runner with the Owner env guard. Every
other step (Step 0 producer, Step 1 writer, Step 2/4 automation, Step 5 retained)
runs unattended. The command plan above is the Owner-dispatch runbook; the
Dispatcher (Codeman) hands Step 3 to the Owner via the post-run workflow only once
P2/P3 are green.

## 6. Timeout / fail rules

- Every script exits **nonzero** on any Git/API/schema/identity failure and prints
  `receipt blocked: <reason>` — a nonzero exit is the hard stop for that step; the
  chain does not continue.
- `live()`/`approve()` never call GitHub from `retained`; `approve()` never writes
  without a passing identity gate.
- `commit_file()` requires a clean Planning checkout and rejects symlinks, path
  escapes, conflicting receipts and unexpected edit history (nonzero exit).
- Idempotent introduction: a repeat of Step 1 returns the same commit, not a new
  one. No destructive rollback exists — a blocked step halts; re-run from the
  introduction commit. A rejected Planning push is re-fetched and re-applied from
  a clean checkout, never force-pushed.
- Plan-only guard for THIS mission: no product file created/edited/committed, no
  Planning write on the product side, no commit, no push, no workflow activated,
  no real gate run.

## 7. Abort / rollback criteria (all nonzero exit → step stops; cited to pinned source)

Producer/writer (`receipt_writer.py:48-63`, `write():35-45`, `commit_file():17-23`):
- trusted post-run context missing/wrong → blocked before any write
  (`GITHUB_EVENT_NAME`/`GITHUB_REPOSITORY`/`GITHUB_REF` guard, `:56-59`).
- dirty Planning checkout, symlink receipt, path escape, conflicting existing
  receipt, unexpected edit history → blocked.

Live verification (`receipt_verifier.py:127-203`, also the approve pre-edit):
- `approved != None` (use retained), `attempt != 1` (no reruns), `sha !=
  candidate_sha` (wrong candidate), wrong run/workflow/event provenance,
  unsuccessful run, missing/ambiguous gate or plan job, wrong head/source SHA,
  missing/expired/ambiguous `trusted-pr-plan` artifact, ZIP/plan binding mismatch.

Approval (`receipt_verifier.py:234-252`, `approve():234-252`):
- any Owner-identity/env gate miss (contributor/fork, wrong event, wrong ref) →
  blocked; also any live-block from above. Single-`approved`-field edit enforced
  by `commit_file(expected=raw)`.

Retained consumption (`receipt_verifier.py:217-231`):
- `approved is None`, approval commit not an ancestor of `--revision`, original
  receipt edited, introduction commit did not introduce the path, more than one
  edit since introduction (including reverts), approval-commit blob mismatch.

Rollback (paper): receipts are single-file, single-edit by design. Introduction
commit is immutable and idempotent; a failed live/approve never leaves a partial
write. A rejected push is not success — re-fetch current Planning history,
re-apply from a clean checkout, preserve concurrent work, never force-push.

## repo-rule compliance

- baseharbor-plan evidence only. No product file created/edited/committed; no
  product write; no commit; no push; no workflow activated; no real gate run.
- All facts above verified against the frozen ec82d476 scripts in
  `worktrees/p2-ustrack` (6 local tests OK) and live `gh` PR state for 1090.
- ec82d476 is the pinned frozen baseline (parent of the current PR1090 draft);
  PR1090/PR2 is still OPEN (no genuine shadow run); no protected Planning revision
  is pinned.

END X3-DONE
