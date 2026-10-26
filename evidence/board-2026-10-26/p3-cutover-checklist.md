# P3 — final-check cutover checklist (paper, read-only, no activation)

ROLE: PR3 cutover planner. Paper prep only. No product file changed.
No push, no commit, no merge in the product repo.

Worktree (read-only): `/home/marcel/dev/baseharbor-ecosystem/worktrees/p3-cutover`
detached at `34159b1f` (`integrate: L4 allowlist sync`, HEAD of `work/r27-minimal-receipt`).
Target release: **v0.4.27 Receipts (PR3)**.
Source of truth: Owner decision `baseharbor-plan/decisions/2026-10-26-minimal-receipts.md`,
`evidence/board-2026-10-26/p2-shadow-runbook.md`, `evidence/board-2026-10-26/FREEZE-PR1.md`,
`docs/development/minimal-ci-receipts.md` (EN) + `docs/de/development/minimal-ci-receipts.md` (DE),
`.github/workflows/ci.yml` @ `34159b1f`, protected-branch rulesets via `gh api` (read).

## Scope boundary (from the decision sequence)

PR1 = contract + two scripts + tests/docs. PR2 = one genuine shadow gate with
automatic task link + authenticated Owner approval. PR3 = **final-check cutover**
after an accepted positive case plus the three rejected cases (wrong SHA, dead run,
edited receipt). The decision explicitly says: *"PR 3 cuts over only the accepted
consumption path, then further gates may follow."* This document does not implement
anything — it pinpoints exactly what PR3 must change, how the current required
final check works, and what still has to be true before it runs.

---

## 1. Exact cutover = which files/lines differ from the 3 dormant ymls

### The 3 dormant ymls are NOT the cutover surface
`receipt-shadow.yml` / `receipt-post-run.yml` / `receipt-approval.yml` are the
**producer + runner + approval** side (P2). They stay dormant and unchanged by PR3.
The p2-shadow-runbook §4 maps each to a guard:
- `receipt-shadow.yml` (W1): activates the `PR · Public Contracts` gate job +
  `trusted-pr-plan` artifact.
- `receipt-post-run.yml` (W2): activates the `workflow_run` runner → writer.
- `receipt-approval.yml` (W3): activates the Owner `workflow_dispatch` → approve.

None of them consumes the receipt. PR3 changes **only the consumer side**.

### The single consumer: `pr_selected_final` in `.github/workflows/ci.yml`
The current "required final check" is the job `PR Selected Checks`
(`ci.yml:793-847`), registered as the required status check
`PR · Selected Checks` (integration_id 15368) on `develop` in ruleset
`25223521` ("develop - required checks", `gh api …/rulesets/25223521`). It is the
one required check that blocks merges into `develop`.

Its fail-closed body (`ci.yml:811-846`) runs `scripts/pr-required-final.py`:

```
python3 "$RUNNER_TEMP/pr-required-final.py" \
  --plan pr-plan.json --results "$RUNNER_TEMP/pr-results.json" \
  --requirements scripts/release-requirements.json
```

`pr-final verify()` (`ci:pr-required-final.py:11-135`) computes `expected_jobs` from
the trusted plan (`selected_gates` / `required_validation_checks` /
`required_consumer_dependencies`) and asserts, against authenticated live job
conclusions (`pr-authenticated-results.py`), that every required job is
`success` and that **no not-required gate/validation is reported PASS**.

### What PR3 must change (line-precise)
The cutover replaces the PR1/PR2 receipt **consumption** in the final check with
the Owner-approved Receipt. The concrete edits live in the final-check step, NOT
in the three dormant ymls:

1. **`ci.yml` `pr_selected_final` job (P3 cutover edit).**
   - Add the Receipt consumption as a new/modified step after
     `pr-required-final.py`, OR replace the public-contracts branch of the
     `if`/verification with the retained-verifier call. The retained call
     (p2-runbook §Step 5):
     ```
     python3 scripts/receipt_verifier.py retained \
       --planning <plan checkout> --revision <afterApproveSha> \
       --receipt evidence/receipts/public-contracts/<run>-1.json \
       --candidate <candidate_sha> --plan <plan SHA>
     ```
   - `--revision` must come from the **protected Planning branch** (`main` in
     baseharbor-plan), never from the PR-supplied `pr-plan.json` or PR head.
   - The retained call only runs when the public-contracts gate is required
     (`needs.pr_plan.outputs.public_contracts_required == 'true'`) — same trigger
     the current `pr_public_contracts` job uses (`ci.yml:738`), so the cutover
     preserves "only consume the receipt for the gate the plan actually selected."

2. **`scripts/pr-required-final.py` (P3 cutover edit, the authoritative fail-closed verifier).**
   - The final check must accept **either** the existing authenticated gate
     results **or** a verified Owner-approved Receipt for the public-contracts
     gate — but never treat a Receipt as a blanket PASS. The Receipt is the
     witness for exactly one gate, not a substitute for `pr_required_final` over
     the whole plan.

3. **`scripts/pr-selected_final.py` / the `--revision` sourcing** if PR3 introduces
   a dedicated consumer script (p2-runbook shows `retained` exists in
   `receipt_verifier.py`; wiring it into the required check is the cutover).

### How this differs from the 3 dormant ymls
- The 3 ymls are **producer/runner/approval** (they *create* the receipt). PR3
  touches the **consumer** (`pr_selected_final` + `pr-required-final.py`).
- No new workflow file is added by PR3; the three dormant files keep their
  never-true `if:` guards (`evidence/…/a2-pr1-acceptance-PREP.md:109-113`) and stay
  frozen.
- No existing workflow/trigger/required-check is **renamed or removed**. The
  required-status context `PR · Selected Checks` keeps its id; only its body's
  public-contracts branch changes. Removals/renames of receipt script paths stay
  "unresolved" per the EN/DE doc (`docs/development/minimal-ci-receipts.md` §Bootstrap).
- `ci-workflow-policy.sh` allowlist is untouched by PR3 (the 3 ymls were an L4
  allowlist concern for P2; `a2-pr1-acceptance-PREP.md:61` shows the known
  `test_ci_workflow_policy_receipts.py` fail-until-L4). PR3 does not activate them.

---

## 2. Regelachweis: how the cutover prevents "Not-required Gates → PASS-Receipt"

### The rule is in `pr-required-final.py:124-131` (fail-closed, not a PASS receipt)

```python
for (kind, identifier), row in index.items():
    if kind not in ('gate', 'validation'):
        errors.append('unknown result kind: ' + str(kind))
    if kind == 'gate' and identifier not in required and row.get('conclusion') == SUCCESS:
        errors.append('not-required gate incorrectly reported PASS: ' + identifier)
    if kind == 'validation' and identifier not in validation and row.get('conclusion') == SUCCESS:
        errors.append('not-required validation incorrectly reported PASS: ' + identifier)
```

`verify()` returns a non-empty error list → `main()` prints `BLOCKED: …` and
exits non-zero → the required check **fails**. This is the fail-closed guarantee:
a not-required gate that reports `success` is rejected outright, never recorded.

### Why the Receipt cannot grant a PASS to a not-required gate
- The Receipt (`{"gate":"public-contracts", …}`) is **one-gate** by schema: it
  witnesses exactly the `public-contracts` gate for one `(run, attempt)`. It
  carries no information about other gates, so it cannot assert success for a
  not-required gate.
- `retained` (`receipt_verifier.py:217-231`) verifies the receipt's internal
  integrity (Owner approval present, original bytes unchanged at the introduction
  commit, exactly one approval edit, no later edit, blob match). It never
  re-derives or extends gate results.
- `pr_required_final` still computes `expected_jobs` from the trusted plan and
  checks authenticated live conclusions for the whole required set. The Receipt
  is consumed as an **additional** witness for the public-contracts gate only;
  the not-required-gate guard (`:127-128`) keeps blocking any other gate that
  slips a `success` conclusion.
- The developer-summary logic (`ci.yml:862-972`) lists skipped/unnecessary jobs
  under **"Not required"** as a plan decision — it never emits a PASS receipt for
  them. EN/DE doc: *"Not-required is a plan decision, never a PASS receipt."*

### Where "plan reason instead of receipt" lives in the code
- Plan reason: `pr-gate-plan.py` (`ci.yml:458`) produces `selected_gates` /
  `required_validation_checks` / `coverage_status`. `pr_required_final` reads
  `selected_gates` and only treats those gates as required (`:39-42`, `:95-99`).
- Receipt: `retained` consumption is gated on `public_contracts_required == true`
  (same predicate as `pr_public_contracts` at `ci.yml:738`), so it fires only
  where the plan explicitly selected the gate.
- The two never merge into a single PASS source: the plan decides required vs
  not-required; the Receipt witnesses only the one selected gate.

---

## 3. Rollback path

Receipts are **single-file, single-edit by design** (`p2-runbook §5`). Rollback is
non-destructive:

- **Pre-merge (cutover PR itself not merged):** the only artifact is PR3's own
  branch/PR. Rollback = do not merge, or squash-merge revert. No product state
  changed; the legacy required check `PR · Selected Checks` stays authoritative on
  `develop` until the cutover merges and goes green.
- **Post-merge (cutover on main/develop):** revert the PR3 commit(s) via a normal
  revert PR on the protected branch. Because:
  - The three dormant ymls are untouched → producer/runner/approval revert to
    P2 behavior instantly.
  - `retained` never calls GitHub; reverting the consumer step only removes the
    receipt call, leaving `pr_required_final` as the sole final verifier.
  - The Receipt introduction commit is immutable and idempotent
    (`receipt_writer.py:commit_file` idempotency; `p2-runbook §Step 1`); a blocked
    live/approve never leaves a partial write, so there is no corrupted receipt to
    clean up.
- **Protected-branch safety:** `develop` has `required_status_checks`
  (`PR · Selected Checks`) and `main` enforces PR + squash + no-force
  (`gh api …/rulesets/25223521` and `/23754542`). A revert goes through the same
  PR path; no force-push, no protection bypass.
- **Rejected push is not success:** re-fetch current Planning/product history,
  re-apply from a clean checkout, preserve concurrent work, never force-push
  (`p2-runbook §Rollback semantics`; EN/DE doc §Live witness).

No destructive rollback exists by design — only non-mutation and re-run from the
introduction commit.

---

## 4. Still-open prerequisites (honest — nothing invented)

These are **not** met. PR3 cannot run until each is satisfied.

| # | Prerequisite | Status (2026-10-26) | Evidence |
| --- | --- | --- | --- |
| 1 | **PR1 accepted (Maintainer)** — the one-gate skeleton (scripts + tests + EN/DE docs) is accepted so the legacy route and the frozen interfaces are authoritative. | **NOT accepted.** Reviewed baseline `ec82d476` passed 6/6, 11/11, 21/21 + o145; but the shipped candidate `084b3abc` ≠ the reviewed/frozen SHA (W2 hardening + L1/L2/L3 dormant slices on top). Acceptance needs either independent review of the added slices or pinning the PR1 merge to `ec82d476`. T0 / T0+72h deadline fields are still **LEER**. | `a2-pr1-acceptance-PREP.md:10-41,47-49,163-168`; `FREEZE-PR1.md:7-12` |
| 2 | **PR2 genuine shadow run passed** — one real contributor PR producing → writing → committing → live-verifying → Owner-approving → auto-linking → shadow-consuming a Receipt end to end on GitHub, with all three negatives. | **NOT run.** Pure paper chain in `p2-shadow-runbook.md` against pinned ec82d476 scripts (6 local tests OK). Producer/runner/approval ymls are DORMANT. | `p2-shadow-runbook.md:1-279` |
| 3 | **Three rejected cases verified live** — wrong candidate SHA, nonexistent run ID, edited (and edited-then-reverted) receipt. | **Covered by unit tests** (real temp-git + fake-GitHub) but **not yet on the live GitHub path** (that is PR2's to prove). | `a2-pr1-acceptance-PREP.md:142-155` |
| 4 | **Owner decision (Marcel)** on (a) the `084b3abc` vs `ec82d476` PR1-merge question and (b) explicit go-ahead to author PR3 cutover. | **Open.** This is an Owner/Coordinator decision, not a planner decision. | `a2-pr1-acceptance-PREP.md:163-168`; decision doc |
| 5 | **Automatic task link** — existing trusted planner writes `evidence/receipt-plans/public-contracts.json` with `"task": "tasks/<id>.md"` for the real Planning task. | **Missing building block** (not to be built in PR1; `p2-runbook §3`). Needed for PR2. | `p2-shadow-runbook.md:172-201` |
| 6 | **Protected Planning checkout write access** for receipt introduction + approval commits on `main` (trusted writers). | Configured in model (`p2-runbook §Step 3`), not verified in this workspace. | — |

No prerequisite is satisfied. PR3 is **paper-prep only** and must not be activated
before #1 (accepted PR1) and #2 (accepted positive PR2 shadow run) are confirmed.

---

## 5. Timeline check against the T0+72h regime

Regime (task `t-20261022-r27-evidence-acceptance.md:72`): *T0 = actual PR1
acceptance (never W1/contract acceptance). Absolute UTC deadline = **T0 + 72 h**.
At expiry without an accepted skeleton: stop infrastructure expansion and route
eligible product work through adequate trusted legacy checks; publish the concrete
blocker. No silent extension, fake PASS or waived product coverage.*

- T0 is **empty** — PR1 has not been accepted (`a2-pr1-acceptance-PREP.md:47-49`).
- Therefore the absolute T0+72h deadline is **undefined until the Maintainer fills
  T0**. This checklist does not compute a date on an empty T0.
- Current wall-clock: **2026-10-26 20:32 UTC**. The decision, FREEZE-PR1, A2 prep,
  and this P3 prep are all dated 2026-10-26 — same operational day as the PR1
  acceptance attempt.
- Consequence for PR3: **PR3 cannot be timed off a real T0 yet**, and per the
  regime it must **not** be activated on a hypothetical deadline. The honest state
  is: T0 unfilled → deadline unfilled → PR3 blocked on accepted PR1 + accepted PR2
  shadow run. If the Maintainer fills T0 and T0+72h passes without an accepted
  skeleton, the regime mandates *stopping* PR-type infrastructure expansion and
  routing eligible product work through the already-adequate trusted legacy path —
  the opposite of activating PR3.

Net: no timeline pressure to cut over. PR3 waits on acceptance, not on a clock.

---

## Repo-rule compliance
- Read-only worktree detached at `34159b1f`; no product file created/edited/committed.
- No push, no commit, no merge, no force, no ruleset/required-check edit.
- Protected-branch rulesets read via `gh api` (read endpoint only).
- All quoted code and line numbers verified against the live worktree / pinned ec82d476.
- Prerequisites reported honestly; no fake PASS, no invented acceptance.

## END P3-DONE (paper prep, read-only)
