# r27 shadow-run — checklist: shadow run AFTER approval (lane o221)

Task t-20261027-r27-shadow-run. Write-Set: this file only. Read-only on GitHub.
Run this only AFTER the Owner approval edit for run 40903927718 is live on main.

## 1. Confirm the approval is live on main (read-only)

1.1 `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor-plan fetch origin main`
1.2 `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor-plan show origin/main:evidence/receipts/public-contracts/40903927718-1.json`
     - Expected: the receipt now carries an `"approved"` object (by "mcpdev80",
       a valid RFC3339 `at`, and a full 40-hex `receipt_commit`), and the
       receipt's `sha` is the reviewed candidate `bb42a2fa727711dc42f8b48e8c9f4375db1a649a`,
       `plan` is `17ca27b74d8fa5871beb04260619180b968b59ec`. If `approved` is
       still `null`, stop — the approval has not landed.
1.3 Record the main HEAD that carries the approval:
     `MAIN=$(git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor-plan rev-parse origin/main)`
1.4 Record the approved candidate SHA from the receipt body (do not trust a
     hand-typed value):
     `python3 - <<'PY'` reading the receipt JSON, printing `sha`.

## 2. Retained verifier against the real approved receipt (the shadow proof)

Use a /tmp worktree of origin/main, never push.

2.1 `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor-plan worktree add /tmp/o221plan origin/main`
2.2 Run the verifier with the real main HEAD, the receipt path, and the exact
    approved candidate and plan fixture:
    ```
    python3 evidence/board-2026-10-29/scripts/receipt_verifier.py retained \
      --planning /tmp/o221plan \
      --revision "$MAIN" \
      --receipt evidence/receipts/public-contracts/40903927718-1.json \
      --candidate "<approved candidate 40-hex>" \
      --plan 17ca27b74d8fa5871beb04260619180b968b59ec
    ```
    Expected: `checked: public-contracts`.
    STOP — if the verifier prints `receipt blocked: <reason>` (any reason), the
    shadow proof failed; do not proceed. Capture the exact exit code and message
    in the evidence file.
2.3 `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor-plan worktree remove /tmp/o221plan --force`

## 3. Negative proofs (offline, runs after approval)

3.1 Fill the two input variables from the live chain, then run:
    ```
    PIN_MAIN="$MAIN" CANDIDATE="<approved candidate 40-hex>" \
      evidence/board-2026-10-29/scripts/negative-proofs.sh
    ```
3.2 All three cases must report `PASS … (exit 1)`:
    - CASE 1 wrong candidate sha → blocked
    - CASE 2 wrong plan revision → blocked
    - CASE 3 edited / later approval edit → blocked (`original receipt was edited`)
    STOP — if any case does not exit 1, capture the output; do not proceed.
3.3 Cleanup: `rm -rf /home/marcel/tmp-ops/o204/proof-edited`

## 4. Ruleset activation patch (read-only preflight only unless explicitly authorized)

Source of the exact payload:
`baseharbor-plan/evidence/r27-ci-replacement-2026-10-29/ruleset-patch-preflight.md` (§1 Ist-Zustand, §2 patch + rollback).
Scope: activate ONLY ruleset 25223521 (`develop - required checks`,
`PR Selected Checks` / integration_id 15368) to `enforcement: active`; leave
everything else byte-identical; prepared rollback to `disabled`.

4.1 Immediately before the PUT, re-read the live ruleset JSON as the payload
    baseline:
    `gh api repos/mcpdev80/baseharbor/rulesets/25223521`
4.2 Confirm the live `required_status_checks` context is exactly
    `PR Selected Checks` / `integration_id 15368`, `strict` stays true.
4.3 Bounded activation PUT (run ONLY on explicit Owner cutover approval, never
    in this lane):
    ```
    gh api -X PUT repos/mcpdev80/baseharbor/rulesets/25223521 \
      -f '{"name":"develop - required checks","target":"branch","source_type":"Repository","source":"mcpdev80/baseharbor","enforcement":"active","conditions":{"ref_name":{"exclude":[],"include":["refs/heads/develop"]}},"rules":[{"type":"required_status_checks","parameters":{"strict_required_status_checks_policy":true,"do_not_enforce_on_create":false,"required_status_checks":[{"context":"PR Selected Checks","integration_id":15368}]}}]}'
    ```
4.4 Rollback block (same JSON, `"enforcement":"disabled"`), ready if activation
    must be undone.
STOP — no `gh api -X PUT/POST` is executed here; §4 is the activation recipe,
not a live action.

## 5. Post-activation verification

5.1 Verify the required check is now enforced on a develop PR:
    `gh pr checks <develop-pr> --repo mcpdev80/baseharbor` — the
    `PR Selected Checks` (Controller App) context must gate the merge.
5.2 Confirm legacy enforcement is unchanged and main-released source ruleset
    23754542 stays `active`.
5.3 Record actual selected checks, the run id of the enforcing run, and rollback
    validation in the evidence file.

## 6. Stops

- STOP before §4 unless an explicit Owner cutover approval exists.
- STOP at any `receipt blocked:` from the retained verifier (§2) or any negative
  case that does not exit 1 (§3).
- Never push, never run `workflow_dispatch`, never overwrite another session's
  work. Planning pushes go through the single writer mechanism.

## o221-DONE
