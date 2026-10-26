# P2 shadow runbook — paper end-to-end receipt chain (2026-10-26)

Role: PR2 shadow-run planner (paper, no activation). Target: one genuine
end-to-end chain `runner -> writer -> receipt-commit -> fresh-live-verification
-> Owner-approval -> automatic task link -> final-check consumption`.

Baseline / frozen interfaces: `baseharbor` work/r27-minimal-receipt @
ec82d476126291d372df2c13bd36273767574869 (develop 433a5702). Scripts pinned from
that SHA; the product branch's `main`-adjacent tree no longer contains them.
Interface source of truth (frozen): `baseharbor/scripts/receipt_writer.py` and
`baseharbor/scripts/receipt_verifier.py`. Tests: `baseharbor/scripts/test_receipt_chain.py`
(all 6 pass locally against the pinned scripts).

Working copy for this runbook: worktree `/home/marcel/dev/baseharbor-ecosystem/worktrees/p2-ustrack`,
branch `p2/ustrack` from 084b3abc. No push, no product change.

## PLAN

1. Re-read FREEZE-PR1.md + RECEIPT-PR1.md for the callable interface and branch model.
2. Pin the two scripts from ec82d476 into the worktree; read writer, verifier, test.
3. Read minimal-ci-receipts.md (EN) for the plan-fixture contract and activation points.
4. Reconstruct the chain on paper, script-by-script, argument-for-argument, against the pinned source.
5. Locate the single manual Owner step, the missing task-link step, the 3 yml activation points, and the abort/rollback criteria.
6. Write this runbook directly as evidence (no commit).

---

## 1. Exact procedure with every script call + arguments

### Repo layout the chain walks on
- Product repo `baseharbor`: contains `scripts/receipt_writer.py` and
  `scripts/receipt_verifier.py`, plus the producer `ci.yml` with the uniquely
  named gate job `PR · Public Contracts` and the `trusted-pr-plan` artifact.
- Planning repo `baseharbor-plan`: the **write target** of both scripts.
  Receipts live at `evidence/receipts/public-contracts/<run>-<attempt>.json`;
  the bounded plan fixture lives at `evidence/receipt-plans/public-contracts.json`
  and is the immutable reference the writer reads by commit SHA (`<plan>`).

Both scripts are pure stdlib + git + `gh`; the GitHub REST header is
`X-GitHub-Api-Version: 2026-03-10`. No secrets, no signing, no archive service.

### Step 0 — producer (existing trusted planner, unchanged)
Not a receipt script. The existing trusted planner in the Actions run emits the
uniquely named artifact `trusted-pr-plan` (bounded ZIP, one `pr-plan.json`,
schema `baseharbor.pr-gate-plan/v1`, `coverage_status: resolved`,
`selected_gates` contains `integration/static/public-contracts`, correct
head/base/integration binding). This is the witness the live verifier reads.

### Step 1 — runner: post-run workflow calls the writer
The producer workflow that finishes cannot be awaited by itself (it triggered the
run). A **separate** `workflow_run` post-run caller (PR 2's `.github/workflows/receipt-post-run.yml`,
currently DORMANT) invokes the writer. This is the "runner" of the chain.

Exact call (writer CLI, `receipt_writer.py:48-63`):

    python3 scripts/receipt_writer.py \
      --planning <path-to-baseharbor-plan checkout> \
      --plan <full SHA of the commit that contains the plan fixture> \
      --run <run_id> \
      --attempt 1
  env: GITHUB_EVENT_NAME=workflow_run, GITHUB_REPOSITORY=mcpdev80/baseharbor, GITHUB_REF=refs/heads/main

What `write()` does (`receipt_writer.py:35-45`):
  1. `load_plan(root, plan)` reads and validates the plan fixture by SHA
     (`receipt_verifier.py:101-112`) — gate/repository/workflow_id/head/candidate/base
     SHAs + task path, all 40-char, task matches `tasks/[a-z0-9-]+\.md`.
  2. Builds the canonical receipt dict: `{"gate":"public-contracts", "run":{"id":run,"attempt":1},
     "sha": plan["candidate_sha"], "status":"pass", "plan": <plan SHA>, "approved": None}`.
  3. `receipt(raw)` enforces the strict schema + canonical bytes + <500 bytes.
  4. `live(value, plan, api=github)` performs **fresh live verification** against
     GitHub (see §4). `api=github` is the production path; tests pass a fake `api`.
  5. `commit_file(root, path, raw)` writes `evidence/receipts/public-contracts/<run>-1.json`
     in the Planning checkout and commits it as
     `receipt: public-contracts record` -> returns `<receipt_commit>` (the *introduction* commit).

Idempotency: a second identical call returns the **same** introduction commit
(`commit_file.py:17-23`), not a new commit. Conflicting content or a dirty
checkout blocks with a ReceiptError (nonzero exit).

### Step 2 — fresh live verification (already inside the writer, and standalone)
`receipt_verifier.py live` (`:127-203`) requires: `approved is None`, `attempt==1`
(no Actions rerun), `sha==candidate_sha`, correct run identity/workflow/event,
successful run, exactly one `PR · Public Contracts` job + one `PR · Trusted Plan`
job (both successful, correct head_sha/run_id), bounded `trusted-pr-plan` artifact
(not expired, right workflow_run/head_sha), and the unzipped `pr-plan.json`
binding to head/base/integration. The ZIP is read **only during the live window**;
nothing is retained. Any API/git/schema/identity failure → nonzero exit.

Standalone live read (no edit):

    python3 scripts/receipt_verifier.py live \
      --planning <plan checkout> --revision <any Planning commit> \
      --receipt evidence/receipts/public-contracts/<run>-1.json \
      --candidate <candidate_sha> --plan <plan SHA>

### Step 3 — Owner-approval commit (the ONLY manual/privileged step)
`receipt_verifier.py approve` (`:234-252`) re-verifies the receipt, re-runs live,
then makes the **sole allowed edit**: replaces `"approved": null` with
`{"by":"mcpdev80", "at":"<UTC ISO8601 Z>", "receipt_commit": <introduction commit>}`
and commits the result as `receipt: public-contracts approve`.

Exact call (same args as live, plus privileged env):

    python3 scripts/receipt_verifier.py approve \
      --planning <plan checkout> --revision <introduction receipt commit> \
      --receipt evidence/receipts/public-contracts/<run>-1.json \
      --candidate <candidate_sha> --plan <plan SHA>
  env: GITHUB_ACTOR=mcpdev80, GITHUB_TRIGGERING_ACTOR=mcpdev80,
       GITHUB_EVENT_NAME=workflow_dispatch,
       GITHUB_REPOSITORY=mcpdev80/baseharbor, GITHUB_REF=refs/heads/main

Who/what signs: exactly the **Owner** `mcpdev80`, via `workflow_dispatch` on
protected `main` from a clean privileged runner. The script checks both
`GITHUB_ACTOR` and `GITHUB_TRIGGERING_ACTOR == OWNER`; a contributor/fork job
fails the identity gate and never gets a Planning write token. This is the one
manual Owner action in the whole chain. `--revision` must be the introduction
receipt commit, obtained from the protected Planning branch (never a PR-supplied
branch).

### Step 4 — automatic task link (PR 2)
The link is written by the **planning side**, not by either baseharbor script.
Per CODEMAN-START §"Integrate without activating early" and W2: "automatic
gate-plan/task reference" is generated by the **existing trusted planner** inside
the PR-2 post-run workflow (`receipt-post-run.yml`), which regenerates the bounded
plan fixture `evidence/receipt-plans/public-contracts.json` with `"task":
"tasks/<task-id>.md"` from the real Planning task. The writer consumes it via
`load_plan`/`receipt["plan"]`.

Paper call that represents the link (produced by the trusted planner in the
post-run workflow, before the writer runs):

    <trusted planner> writes evidence/receipt-plans/public-contracts.json with
    task: "tasks/t-<id>.md"  (Planning repo commit = the <plan> SHA)

### Step 5 — final-check consumption (shadow)
Not yet cutover (that is PR 3). The shadow consumes the receipt in **retained**
mode, which never calls GitHub and binds the approved history locally:

    python3 scripts/receipt_verifier.py retained \
      --planning <plan checkout> --revision <commit after the approval edit> \
      --receipt evidence/receipts/public-contracts/<run>-1.json \
      --candidate <candidate_sha> --plan <plan SHA>

`retained` (`:217-231`) requires: `approved is not None`; the approval commit
descends from the introduction commit; the original receipt bytes are unchanged at
the introduction commit; the introduction commit *introduced* the path (first
edit); exactly one edit exists since (`approved` write) with no later edits (even
reverts block); and the approval-commit blob equals the current bytes. This is the
offline final-check consumption of the Owner-approved receipt.

### Full paper chain (arguments in one place)
    runner:  writer        --planning P --plan <planSha> --run <runId> --attempt 1   (env: workflow_run on main)
    verify:  verifier live --planning P --revision <planSha> --receipt .../<run>-1.json --candidate <candSha> --plan <planSha>
    approve: verifier approve --planning P --revision <receiptIntroSha> --receipt .../<run>-1.json --candidate <candSha> --plan <planSha>  (env: workflow_dispatch, ACTOR=mcpdev80)
    consume: verifier retained --planning P --revision <afterApproveSha> --receipt .../<run>-1.json --candidate <candSha> --plan <planSha>

All four share `--planning` (Planning checkout), `--plan` = the plan-fixture commit,
`--candidate` = the tested integration SHA, and the receipt path
`evidence/receipts/public-contracts/<run>-1.json`.

## 2. The single manual Owner step

`scripts/receipt_verifier.py approve` is the only step that requires a human-owned
privileged action. Signing party: Owner **`mcpdev80`**, authenticated via
`GITHUB_ACTOR` + `GITHUB_TRIGGERING_ACTOR` on a `workflow_dispatch` on protected
`main` from a clean privileged runner. The script itself does the signing: it
verifies, re-runs live, and makes the single edit `approved:null -> {by,at,receipt_commit}`
then commits it (`receipt_verifier.py:234-252`). No other script or caller may
produce that edit; a fork/contributor job is rejected at the identity gate. The
approved receipt bytes stay <500 and canonical.

## 3. Automatic task link — missing building block (do NOT build it here)

Missing script/interface: **there is no code in PR 1 that regenerates the plan
fixture with the real task reference.** The task link is produced by the
**existing trusted planner** (the PR-2 post-run producer), which must write
`evidence/receipt-plans/public-contracts.json` with the correct `"task":
"tasks/<id>.md"`. Neither `receipt_writer.py` nor `receipt_verifier.py` writes
the task link — the writer only reads it (`load_plan`) and stamps it as
`receipt["plan"]`. Building this is a **KLARER FEHLBAUSTEIN** per mission item 3
and CODEMAN-START (§"no extra production script/service"; keep task linking
bounded in the workflow).

Proposed interface (trusted planner / post-run workflow, PR 2):
- Input: accepted producer run `{run_id, workflow_id, head_sha, candidate_sha,
  base_sha}` + the current Planning task note path for this gate.
- Action: produce a minimal plan dict and commit it in `baseharbor-plan` on
  protected main, returning the new commit SHA as `<plan>`.

    struct PlanFixture {
      gate: "public-contracts",
      repository: "mcpdev80/baseharbor",
      workflow_id: <int>,
      head_sha: <40>, candidate_sha: <40>, base_sha: <40>,
      task: "tasks/<task-id>.md"   // the automatic link
    }
    // written to evidence/receipt-plans/public-contracts.json in baseharbor-plan

- Contract already enforced by the pinned `load_plan` (`receipt_verifier.py:101-112`):
  exactly those 7 fields, 40-char SHAs, task regex `tasks/[a-z0-9-]+\.md`.
- The writer's `--plan <that commit>` is the only downstream reference to it.

## 4. Activation points in the three ymls (line-precise)

The three workflows are **DORMANT in PR 1** and only activated by PR 2 commits
(FREEZE-PR1 §"Exclusive write paths"). They do not yet exist on the frozen
baseline ec82d476; the activation points are the lines that would be added.
W1 (producer side) owns `receipt-shadow.yml`; W2 owns
`receipt-post-run.yml`; W3 owns `receipt-approval.yml`.

- `baseharbor/.github/workflows/receipt-shadow.yml` (W1, DORMANT): activates the
  `PR · Public Contracts` gate job that the live verifier keys on
  (`receipt_verifier.py:161-162`, `name == "PR · Public Contracts"`) and emits the
  `trusted-pr-plan` artifact (`:178-187`). Activation = this job running on
  `pull_request` and publishing the artifact.
- `baseharbor/.github/workflows/receipt-post-run.yml` (W2, DORMANT): activates the
  **runner** — a `workflow_run` caller (matching `ci.yml`, event `pull_request`)
  that (a) regenerates the plan fixture with the task link, then (b) invokes the
  writer with the env guard
  `GITHUB_EVENT_NAME=workflow_run && GITHUB_REPOSITORY=mcpdev80/baseharbor &&
  GITHUB_REF=refs/heads/main` (`receipt_writer.py:56-59`). This is the only line
  that authorizes a producer-side receipt write.
- `baseharbor/.github/workflows/receipt-approval.yml` (W3, DORMANT): activates the
  Owner approval — a `workflow_dispatch` job on protected `main` that runs the
  `approve` mode with the Owner env guard
  `GITHUB_ACTOR=mcpdev80 && GITHUB_TRIGGERING_ACTOR=mcpdev80 &&
  GITHUB_EVENT_NAME=workflow_dispatch && GITHUB_REPOSITORY=mcpdev80/baseharbor &&
  GITHUB_REF=refs/heads/main` (`receipt_verifier.py:235-240`). This is the only
  line that authorizes a privileged Planning write.

Line numbers will be concrete only after the PR 2 commits land; until then these
are the exact guard conditions / job names / env gates that each file must
introduce, mapped to the pinned verifier/writer lines above.

## 5. Abort / rollback criteria (all exit nonzero → step stops)

Producer/writer (`receipt_writer.py:56-63`):
- Not a trusted post-run context (wrong/absent `GITHUB_EVENT_NAME`,
  `GITHUB_REPOSITORY`, `GITHUB_REF`) → blocked before any write.
- Dirty Planning checkout, symlink receipt, path escaping the Planning root,
  conflicting existing receipt, or unexpected edit history → blocked.

Live verification (`receipt_verifier.py:127-203`, also the approve pre-edit):
- `approved != None` (must use retained), `attempt != 1` (no Actions reruns),
  `sha != candidate_sha` (wrong candidate), wrong run identity/workflow/event,
  unsuccessful run/missing or ambiguous gate or plan job, wrong head/source SHA,
  missing/expired/ambiguous `trusted-pr-plan` artifact, ZIP/plan binding mismatch.

Approval (`receipt_verifier.py:234-252`):
- Any Owner-identity/env gate miss (contributor/fork, wrong event, wrong ref) →
  blocked; also any live-block from above. The edit is atomic: only the single
  `approved` field changes; anything else fails the `commit_file(expected=raw)`
  check.

Retained consumption (`receipt_verifier.py:217-231`):
- `approved is None`, approval commit not an ancestor of the requested revision,
  original receipt edited, introduction commit did not introduce the path, more
  than one edit since introduction (including reverts), or approval-commit blob
  mismatch.

Rollback semantics (paper): receipts are single-file, single-edit by design. The
introduction commit is immutable and idempotent; a failed live/approve never
leaves a partial write (writer commits only after a successful live check;
approve only after re-verify). A rejected push is not success — re-fetch current
Planning history, re-apply from a clean checkout, preserve concurrent work, never
force-push or overwrite others. A blocked step simply halts the chain; there is
no destructive rollback, only non-mutation and re-run from the introduction commit.

---

Verification performed for this shadow run:
- Pinned `receipt_writer.py`, `receipt_verifier.py`, `test_receipt_chain.py` from
  ec82d476 into worktree p2/ustrack.
- `python3 scripts/test_receipt_chain.py` → 6 tests OK (real temp Git + fake live
  API), confirming the writer→verify→approve→retained contract and the negatives
  (wrong SHA, dead run, edited+reverted receipt, strict JSON, Owner identity).
- No product file changed; no Planning file written; no commit, no push.

END P2-DONE
