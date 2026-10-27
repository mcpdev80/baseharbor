# Z18 — PR3 receipts-as-mandatory cutover plan (PLAN, read-only, evidence only)

ROLE: PLAN (read-only, evidence only). No product file changed, no commit, no
push, no ruleset edit.

Author: Codeman (mission z18), AgentGateway session.

UTC host time confirmed via `basics-check.py` turn-start before this write.

Write scope: `evidence/board-2026-10-26/z18-pr3-plan.md` (baseharbor-plan) ONLY.

## Sources (all read, all live on develop @ 577aac4a unless noted)

- `baseharbor-plan/evidence/r27-ci-replacement-2026-10-26/` — CLAIM, CODEMAN-START,
  PR2-SHADOW-PREP, ACCEPTANCE-PR1, RECEIPT-PR1 (all read).
- `evidence/board-2026-10-26/p3-cutover-checklist.md` (read, authoritative cutover
  scope), `p2-shadow-runbook.md` (read), `SHADOW-RUN-RECORD.md` (read).
- `evidence/board-2026-10-26/z5-reviews-1096.md`, `z6-reviews-1097.md`, `z4-planning-shadowrun.md`,
  `z7-covmap.md` (read).
- `gh api repos/mcpdev80/baseharbor/rulesets` (read): 3 rulesets.
- `gh api …/rulesets/{25223521,23754542}` (read): develop / main bodies.
- `.github/workflows/ci.yml @ 577aac4a` (read): `pr_selected_final` body at ci.yml:793-847.
- `scripts/receipt_verifier.py @ 577aac4a` (read): live/approve/retained gate contract.
- develop branch heads: `develop=577aac4a`, `main=6409815`. PR #1096 OPEN (head b7b21dad, +cov commits). PR #1097 MERGED.

---

## 0. Realitätsabgleich (turn-start facts)

- develop HEAD `577aac4a` (matches mission). Shadow chain is COMPLETE: SHADOW-RUN-RECORD.md
  v0.4.27 proves one real end-to-end chain on 2026-10-27T02:55Z — producer run 40428162731
  (PR #1094 bd342b91) SUCCESS → receipt 06f25dcf → **hosted Owner approval** dispatch 40440882700
  SUCCESS (actor mcpdev80, `--ref develop`) → patch `git am` → planning 8411672 → retained-verify rc=0.
- `dev_workflow_approve` approval workflow is live **only with `--ref develop` + in-shell
  `GITHUB_REF=refs/heads/main` hack** (SHADOW-RUN-RECORD line 10). This is the documented
  boundary PR3 must remove: the cutover must run approve from `refs/heads/main`, not develop.
- Retained / live / approve modes all proven. Skeleton ACCEPTED, deadline 2026-10-29T23:35:50Z
  met on day 1 (Tag 1). PR #1096 (cov-map) near merge.
- PR1 `084b3abc` ≠ frozen `ec82d476` is resolved in spirit: PR1 scripts are accepted and the
  shadow run re-verified against live develop; the one-gate skeleton qualifies. The remaining
  Owner question is only the *exact* cutover trigger SHA, not PR1 correctness.

---

## 1. What must change in branch protection to make PR3 a real mandatory gate WITHOUT bypass

Rulesets (`gh api …/rulesets`, read-only):

| Ruleset | id | applies | rule body |
|---|---|---|---|
| `develop - required checks` | 25223521 | `refs/heads/develop` | `required_status_checks` **strict**, context `PR · Selected Checks` (integration_id 15368) |
| `develop - integration branch` | 23754541 | `refs/heads/develop` | (integration-branch rule, no required check) |
| `main - released source` | 23754542 | `refs/heads/main` | `pull_request` (PR + squash + no-force + required-thread-resolution), deletion/non-fast-forward/linear-history |

`main` has **no** `required_status_checks` rule (correct — no release gate on main).
`develop` has exactly one required-status-check: `PR · Selected Checks`.

**Key finding — the cutover does NOT need to touch branch protection at all.**
`PR · Selected Checks` (`ci.yml:793`) is the *body* of the required check; the
*context string and integration_id* are the protection binding and must stay
constant. PR3 only changes what the **body** (`pr_selected_final`, ci.yml:811-847)
computes for the public-contracts gate. The required-status-check context
`PR · Selected Checks` keeps its id/integration (no ruleset edit, no new rule,
no bypass). That is exactly why there is no rule-bypass: the mandatory gate stays
on `develop`, its context is unchanged, and the new receipt path is wired into the
same fail-closed verifier.

**The only real rule-surface risk is the fixed-workflow allowlist**, not branch
protection:

- `scripts/ci-workflow-policy.sh @ 577aac4a` (`expected=(…)` allowlist, 1317 lines)
  already lists `receipt-approval.yml`, `receipt-post-run.yml`, `receipt-shadow.yml`
  (lines 18-20). So the three dormant ymls pass policy on develop today. **No
  allowlist edit required** for PR3.
- `test_ci_workflow_policy_receipts.py` is the FailUntil-L4 guard — still dormant;
  PR3 does not arm the ymls, it only consumes the receipt, so this guard stays green-by-dormancy.

**Rule-change summary (the answer to mission item 2):**

1. **Branch protection: NO change.** Context `PR · Selected Checks` / integration_id 15368
   on `develop` stays the mandatory binding. Do **not** add a new ruleset, add a second
   required context, or change `strict_required_status_checks_policy` (currently true). Any
   change there is either unnecessary or, if it weakened the check, a bypass.
2. **Allowlist: NO change** — receipt-*.yml already allowed on develop. Only a *new*
   workflow file would need an allowlist entry; PR3 adds none.
3. The only legitimate rule edit PR3 could ever need is a *product* `coverage_status`
   qualification (e.g. an Owner-geradted preview rule for `internal/application/**`), and
   even that is a `pr-gate-plan.py` concern, not a ruleset change. It is orthogonal to the
   receipt cutover.

---

## 2. PR3 target image (max 1 page per section)

### (a) Which workflows become mandatory under PR3

PR3 does **not** add new mandatory workflows and does **not** arm the three dormant
ymls (their `if:` guards stay never-true; verified on develop:
`receipt-shadow.yml:23`, `receipt-post-run.yml:24`, `receipt-approval.yml:64`).
It makes the **already-passing shadow receipt** the authoritative witness for the
`integration/static/public-contracts` gate on the final check. Concretely:

- **Mandatory (PR3):** the `PR · Public Contracts` gate — which was already a
  required gate (`public_contracts_required == true`) — now qualifies a merge
  *through an Owner-approved Receipt* in addition to (not instead of) an authenticated
  live run, when PR3 is live on `develop`.
- **Not changed / still dormant:** `receipt-shadow.yml` (producer), `receipt-post-run.yml`
  (runner), `receipt-approval.yml` (Owner approval dispatch). Their trigger `if:`
  guards are untouched; PR3 only *consumes* what they would produce.
- **Mandatory body that changes:** `pr_selected_final` (`PR · Selected Checks`),
  ci.yml:793-847 — the single consumer.

### (b) Receipt-gate job placement in ci.yml — WITHOUT self-qualification

Placement target: **`pr_selected_final`, the `Enforce every required check` step
(ci.yml:811-847)**, NOT a new job and NOT the three dormant ymls. Two additive edits:

1. **After the existing `pr-required-final.py` call (ci.yml:847), add a single
   retained-consumption step**, still inside `pr_selected_final`, still under
   `contents: read` / `actions: read`:

   ```yaml
   - name: Consume Owner-approved Receipt for public-contracts gate (PR3)
     if: always() && needs.pr_plan.outputs.public_contracts_required == 'true' && needs.pr_public_contracts.result == 'success'
     env:
       GH_TOKEN: ${{ github.token }}
     run: |
       set -euo pipefail
       python3 scripts/receipt_verifier.py retained \
         --planning "$GITHUB_WORKSPACE/planning" \
         --revision <protected main Planning commit> \
         --receipt evidence/receipts/public-contracts/<run>-1.json \
         --candidate "$EXPECTED_HEAD" --plan <plan fixture SHA>
   ```

   Constraints that keep this a real gate (no self-qualification):
   - `--revision` **must** come from the *protected Planning branch* (`main` in
     baseharbor-plan), never from the PR-supplied `pr-plan.json` or PR head
     (`receipt_verifier.py:blob` requires a full 40-char SHA from the read-only
     Planning checkout). This is the anti-self-qualification anchor: a PR cannot
     point `--revision` at its own blob.
   - The step is additive and **fail-closed**: if the receipt is missing/editted/dead,
     `retained` raises (`receipt_verifier.py:217-231`) and the step exits non-zero.
   - `--candidate` is the PR head (`EXPECTED_HEAD`), `--plan` the fixture SHA bound to
     that head — the same integrity the shadow run already verified.
   - No new workflow file is added (allowlist untouched); the three dormant ymls keep
     their never-true `if:` guards.

2. **`scripts/pr-required-final.py` (the authoritative fail-closed verifier,
   `pr-final verify()` at ci.yml:811-847 body):** accept the Receipt as an
   *additional* witness for the `public-contracts` gate only — never as a blanket
   PASS. The not-required-gate guard (`pr-required-final.py:124-131`) must keep
   blocking any not-required gate/validation that reports PASS. The Receipt is
   one-gate by schema (`receipt_verifier.py:61-62`), so it cannot extend to other
   gates. `pr_required_final` still computes `expected_jobs` from the trusted plan
   over the whole required set; the receipt is consumed as one more witness, not a
   substitute.

**Bootstrap / pinned-clean consideration (mission item b, "trusted-plan bootstrap"):**
`pr_selected_final` already fetches a pinned cleanup SHA `c6db56c`
(ci.yml:832-839, commit `ci: stage full native selection … regression coverage`).
The PR3 retained-consumption step should either reuse that same pinned cleanup SHA
or pin its own exact SHA — never a branch ref — so the cutover body is reproducible
and the Receipt verification uses the same reviewed `receipt_verifier.py` the shadow
run used. Do not let the producer `trusted-pr-plan` (PR1/PR2, dormant) feed the
PR3 final body; PR3 consumes the Receipt, not a fresh producer run.

### (c) Rollout order + rollback

Rollout (no new required check, no ruleset edit — lowest-risk path):

1. **Gate on `develop` first.** PR3 lands on `develop` behind its own PR
   (`base_ref == develop`, same predicate the current `pr_public_contracts` uses at
   ci.yml:738). The retained-consumption step only arms when
   `public_contracts_required == true` AND `needs.pr_public_contracts.result ==
   success` — i.e. the gate already passed authenticated live, the Receipt is a
   second witness, not the only one. This preserves the current fail-closed
   guarantee during rollout: a develop PR still needs a green `PR · Public Contracts`
   live run; the Receipt only *adds* a verifiable Owner signature.
2. **Prove the cutover body green on `develop`** (real contributor PR, not a shadow
   replay): producer (dormant, replayed) → receipt → Owner approval from
   `refs/heads/main` (the `GITHUB_REF` hack removed) → PR3 retained-consumption step
   green. Only then merge PR3 to `develop`.
3. **No separate main-gate step** — `main` has no required-status-checks rule and
   remains PR + squash + no-force. The mandatory receipts gate stays on `develop`.

Rollback (non-destructive, per p3-cutover-checklist §3 / PR2-SHADOW-PREP §4):

- **Pre-merge:** PR3 branch/PR not merged → do not merge, or squash-merge revert.
  Legacy `PR · Selected Checks` (authenticated live run) stays authoritative.
- **Post-merge:** revert PR3's commit(s) via a normal revert PR on protected `develop`.
  The three dormant ymls stay untouched (producer/runner/approval unaffected), and
  `retained` never calls GitHub, so reverting the consumer step leaves
  `pr_required_final` as the sole final verifier.
- **Introduction commit is immutable + idempotent** (`receipt_writer.py` `commit_file`).
  A blocked live/approve never leaves a partial receipt. Reverts go through the same
  PR path — no force-push, no protection bypass.

### (d) Offene Owner-Entscheidungen (1 line each)

- **Cutover trigger SHA:** accept the already-passed shadow receipt (577aac4a scripts) as the PR3 witness authority, or pin a fresh reviewed `receipt_verifier.py` SHA — do not re-qualify PR1 for this.
- **Remove the `GITHUB_REF=refs/heads/main` on develop hack:** confirm approve must run from real `refs/heads/main` in PR3 (verified: verifier needs all 5 env vars, `receipt_verifier.py:235-240`).
- **Consume-on-develop vs consume-on-main:** PR3 plan is consume-on-develop (keeps mandatory gate on develop); approve signature lives on main — confirm this split.
- **New `internal/application/**` qualification rule:** whether a `pr-gate-plan.py` Owner-geradted preview rule is needed for the cov-map PR (#1096) is orthogonal to PR3 and may stay open.
- **Deadline regime:** skeleton ACCEPTED on day 1 — confirm whether PR3 proceeds under T0+72h or is parked post-deadline per the "stop infra expansion, keep eligible product work" clause.

---

## 3. Deadline-Risiko 2026-10-29T23:35:50Z (realistisch)

- **Skeleton is ACCEPTED, not pending.** SHADOW-RUN-RECORD.md proves the one-gate
  chain end-to-end (hosted Owner approval, rc=0 retained) on 2026-10-27; day 1 of the
  T0+72h window. The "infrastructure expansion stops after deadline" risk clause is
  therefore **not active today** — the gate is qualified.
- **Realistic risk is not the clock, it is the Owner decision on the cutover body**
  (§2d): PR3's *cutover* (consume Receipt as authoritative on develop) is a NEW step
  beyond the accepted skeleton and still needs the explicit Owner go-ahead. The deadline
  pressures nothing that is already qualified; it pressures the optional cutover.
- **Skeleton-IST-akzeptiert ⇒ Infra-Ausbau darf weiter.** Per the mission rule and the
  regime: accepted skeleton means eligible infra work (PR3 cutover prep) continues; no
  Abbruch. The concrete blocker to watch is the missing `refs/heads/main` approve path,
  which PR3 removes — already structurally possible (5 env vars present on develop,
  verified PASS) but only exercised via the develop hack so far.
- **Net:** low clock-risk; the honest blocker is the Owner cutover decision + removing
  the develop hack, not the deadline. If no Owner go-ahead lands before 2026-10-29T23:35:50Z,
  stop the cutover (not the qualified skeleton) and route eligible product work through
  the legacy authenticated run — the opposite of activating PR3 early.

---

## repo-rule compliance

- PLAN role, read-only. No product file created/edited/committed; no commit, no push,
  no ruleset/required-check/allowlist edit.
- Protected-branch rulesets, ci.yml, receipt_verifier.py, ci-workflow-policy.sh all read
  via `gh api` (read endpoints) or the local pinned `baseharbor/scripts/receipt_verifier.py`.
- All SHAs/runs PRs verified live (develop 577aac4a, main 6409815, PR #1096 OPEN, #1097 MERGED).
- No fake PASS, no invented acceptance; deadline assessed against the accepted skeleton.

## END Z18-DONE (PLAN, read-only)
