#!/usr/bin/env bash
# r27 shadow-run NEGATIVE PROOFS (lane o204 / o221; hardened for o227).
#
# negative-proofs.sh
#
# Three bounded, fully OFFLINE cases. No hosted dispatches, no gh run rerun,
# no network beyond the planning clone read. Each case invokes
# receipt_verifier.py (mode "retained") and captures its explicit
# expected-failure output.
#
# o227 hardening: the synthetic Case-3 receipt id defaults to the non-colliding
# test id 99999999999 (overridable via the RECEIPT_ID env var) so it never
# equals a real main-chain run id (not the superseded 40903927718, not the
# tested 40921493079) and cannot stage an already-committed receipt. The binders
# PIN_MAIN / CANDIDATE / RUN_ID / PLAN default to the real main chain; PIN_MAIN
# is the Planning main (origin/main of baseharbor-plan) read at runtime.
#
# Contract (baseharbor/scripts/receipt_verifier.py, mode "retained"):
#   retained --planning <dir> --revision <sha> --receipt <path> \
#            --candidate <40hex> --plan <sha>
#   - passes only when the committed receipt at the canonical path, verified
#     against the Git history at --revision, matches --candidate and --plan and
#     carries exactly one Owner approval edit introduced at --plan.
#   - fails closed (exit 1) on any wrong candidate, wrong plan revision, edited
#     approval, or a non-committed / non-canonical receipt.
#
# These three negatives run AFTER the receipt for RUN_ID has been Owner-approved
# on main: the acceptance edit is already in the main tree, so cases 1 and 2
# exercise the verifier directly against the real receipt (read-only, no push).
# Case 3 (tampered approval edit) rebuilds a tiny synthetic workspace so each
# proof isolates exactly one variable. The workspace lives under
# /home/marcel/tmp-ops/o204/proof-edited; the planning clone is never pushed.
set -euo pipefail

ECOSYSTEM_DIR="/home/marcel/dev/baseharbor-ecosystem"
PLAN_DIR="$ECOSYSTEM_DIR/baseharbor-plan"
O204_ROOT="/home/marcel/tmp-ops/o224"
EDITED_DIR="$O204_ROOT/proof-edited"
SCRIPTS_DIR="$ECOSYSTEM_DIR/evidence/board-2026-10-29/scripts"

# Inputs from the first automatic receipt (run 40921493079). Point these at the
# real main chain before running:
#
#   - PIN_MAIN  : the 40-hex Planning main HEAD that carries the Owner approval
#                 edit. This is the review pin for cases 1 and 2. It stays a
#                 placeholder until set (see below) because the script must fail
#                 closed if the operator forgot to pin the real chain.
#   - CANDIDATE : the reviewed integration/candidate SHA carried by the receipt.
#   - RUN_ID    : the run id of the approved receipt under test; the receipt is
#                 read from evidence/receipts/public-contracts/<RUN_ID>-1.json
#                 inside the Planning working copy.
#   - PLAN      : the plan revision the receipt is bound to.
#
# Planning main is the baseharbor-plan repository's main branch (origin/main).
# This is the *Planning* main, not any product main: the verifier reads receipt
# blobs and plan fixtures from the Planning clone's Git history, never from a
# product checkout. It is read at runtime from the planning clone's origin/main
# (git -C "$PLAN_DIR rev-parse origin/main"), so the value stays anchored to
# whatever main HEAD actually carries the approval edit when the run starts.
#
# Fill the four binders before running:
#   PIN_MAIN   <- `git -C "$PLAN_DIR rev-parse origin/main"` after the edit lands
#   CANDIDATE  <- the `sha` field of the approved receipt (verified unchanged)
#   RUN_ID     <- the run id of the approved receipt under test
#   PLAN       <- the plan revision bound to that receipt
# The script refuses to proceed while PIN_MAIN or CANDIDATE are unset or still
# at their placeholder values.
PIN_MAIN="${PIN_MAIN:-SET_ME_TO_MAIN_HEAD_40HEX}"
CANDIDATE="${CANDIDATE:-SET_ME_TO_APPROVED_CANDIDATE_SHA_40HEX}"
RUN_ID="${RUN_ID:-40921493079}"
RECEIPT="evidence/receipts/public-contracts/${RUN_ID}-1.json"
PLAN="${PLAN:-eef674d0b7d9214724417b215495d500cc1c24b2}"
# Synthetic Case-3 receipt lives under a run id that does not exist on the real
# main chain, so commit B introduces a *new* path (the verifier requires exactly
# that). The default 99999999999 is a non-colliding test-id range; it never
# equals a real run id (including the superseded 40903927718) and cannot stage
# an already-committed receipt. Override it with the RECEIPT_ID environment
# variable when another isolated id is wanted. Isolated, offline, never pushed.
SYNTH_RUN_ID="${RECEIPT_ID:-99999999999}"
SYNTH_RECEIPT="evidence/receipts/public-contracts/${SYNTH_RUN_ID}-1.json"
WRONG_CANDIDATE="deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
BAD_REV="deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

die() { echo "negative-proofs: FATAL: $*" >&2; exit 2; }
command -v jq >/dev/null 2>&1 || die "jq not found"
[ "${PIN_MAIN}" != "SET_ME_TO_MAIN_HEAD_40HEX" ] || die "PIN_MAIN unset — set it to the main HEAD that holds the Owner approval edit"
[ "${CANDIDATE}" != "SET_ME_TO_APPROVED_CANDIDATE_SHA_40HEX" ] || die "CANDIDATE unset — set it to the reviewed candidate SHA of the approved receipt"

prove() {
  local label="$1" expected="$2"; shift 2
  local out rc
  # Bounded call: the verifier is offline and must return in well under 20s.
  # `set +e` is required here: a captured non-zero exit would otherwise trip
  # errexit on the assignment (out="$(...)") before we can inspect $?.
  set +e
  out="$(timeout 20 "$@" 2>&1)"; rc=$?
  set -e
  if [ "$rc" -eq 124 ]; then
    echo "FAIL  $label (TIMEOUT after 20s — verifier hung)"
    return 1
  fi
  if { [ "$expected" = "block" ] && [ "$rc" -ne 0 ]; } \
     || { [ "$expected" = "pass" ] && [ "$rc" -eq 0 ]; }; then
    echo "PASS  $label (exit $rc)"
  else
    echo "FAIL  $label (exit $rc, expected $expected)"
    echo "$out" | sed 's/^/      /'
    return 1
  fi
}

# ---- CASE 3 prelude: a synthetic workspace that holds a tampered approval edit
#     (only built when case 3 runs). Cases 1 and 2 use the real main tree. ----
echo "negative-proofs: RUN_ID=${RUN_ID}  RECEIPT=${RECEIPT}"
echo "negative-proofs: PIN_MAIN=${PIN_MAIN}  CANDIDATE=${CANDIDATE}  PLAN=${PLAN}"
echo


echo "----- CASE 1: wrong candidate sha (expected: blocked) -----"
prove "wrong candidate" block \
  python3 "$SCRIPTS_DIR/receipt_verifier.py" retained \
    --planning "$PLAN_DIR" --revision "$PIN_MAIN" --receipt "$RECEIPT" \
    --candidate "$WRONG_CANDIDATE" --plan "$PLAN"
echo

echo "----- CASE 2: wrong plan revision that never holds the receipt (expected: blocked) -----"
prove "wrong plan revision" block \
  python3 "$SCRIPTS_DIR/receipt_verifier.py" retained \
    --planning "$PLAN_DIR" --revision "$PIN_MAIN" --receipt "$RECEIPT" \
    --candidate "$CANDIDATE" --plan "$BAD_REV"
echo

echo "----- CASE 3: edited approval object (expected: blocked) -----"
# Re-author a synthetic workspace that holds a valid approved receipt, then
# tamper an unrelated receipt field in the single allowed approval edit. The
# verifier requires the approval edit to be the single allowed edit introduced
# at --plan; any other change to the blob fails canonical(). Isolated offline.
rm -rf "$EDITED_DIR"; mkdir -p "$EDITED_DIR"
git clone --quiet "$PLAN_DIR" "$EDITED_DIR" 2>/dev/null \
  || die "could not clone planning repo to $EDITED_DIR"

# commit A: plan fixture (exactly the 7 load_plan fields, candidate_sha=CANDIDATE)
python3 - "$EDITED_DIR" "$PLAN" "$CANDIDATE" <<'PYEOF'
import json, os, sys
root, plan, cand = sys.argv[1:4]
os.makedirs(os.path.join(root, "evidence", "receipt-plans"), exist_ok=True)
fixture = {"gate": "public-contracts", "repository": "mcpdev80/baseharbor",
           "workflow_id": 352609139, "head_sha": "96a96362d3682bdf35748c5331a6f2452c01b4c8",
           "candidate_sha": cand, "base_sha": "9e50d8a4ab6c358ad45da6b337c0e67c64373264",
           "task": "tasks/t-20261029-r27-ci-producer.md"}
p = os.path.join(root, "evidence", "receipt-plans", "public-contracts.json")
with open(p, "w") as f:
    json.dump(fixture, f, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
    f.write("\n")
PYEOF
git -C "$EDITED_DIR" add evidence/receipt-plans/public-contracts.json
git -C "$EDITED_DIR" -c user.name=o204 -c user.email=o204@local \
  commit -q -m "o204 plan fixture"
P1="$(git -C "$EDITED_DIR" rev-parse HEAD)"

# commit B: author receipt (approved=null, plan=P1, candidate=CANDIDATE)
python3 - "$EDITED_DIR" "$P1" "$CANDIDATE" "$SYNTH_RUN_ID" <<'PYEOF'
import json, os, sys
root, p1, cand, synth = sys.argv[1:5]
d = os.path.join(root, "evidence", "receipts", "public-contracts")
os.makedirs(d, exist_ok=True)
receipt = {"gate": "public-contracts", "run": {"id": int(synth), "attempt": 1},
    "sha": cand, "status": "pass", "approved": None, "plan": p1}
p = os.path.join(d, f"{synth}-1.json")
with open(p, "w") as f:
    json.dump(receipt, f, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
    f.write("\n")
PYEOF
git -C "$EDITED_DIR" add "$SYNTH_RECEIPT"
git -C "$EDITED_DIR" -c user.name=o204 -c user.email=o204@local \
  commit -q -m "o204 author receipt"
AUTHREV="$(git -C "$EDITED_DIR" rev-parse HEAD)"

# commit C: a clean, valid approval edit at the auth rev (the one edit the
# verifier would accept).
python3 - "$EDITED_DIR" "$AUTHREV" "$P1" "$CANDIDATE" "$SYNTH_RUN_ID" <<'PYEOF'
import json, os, sys
root, authrev, p1, cand, synth = sys.argv[1:6]
p = os.path.join(root, "evidence", "receipts", "public-contracts", f"{synth}-1.json")
with open(p) as f: cur = json.load(f)
cur["approved"] = {"by": "mcpdev80", "at": "2026-10-29T07:20:00Z", "receipt_commit": p1}
with open(p, "w") as f:
    json.dump(cur, f, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
    f.write("\n")
PYEOF
git -C "$EDITED_DIR" add "$SYNTH_RECEIPT"
git -C "$EDITED_DIR" -c user.name=o204 -c user.email=o204@local \
  commit -q -m "o204 approval edit"
AUTHED="$(git -C "$EDITED_DIR" rev-parse HEAD)"

# commit D: a second, tampering edit that mutates the approval timestamp. The
# verifier allows exactly one approval edit introduced at --plan; any later edit
# to the blob fails canonical() -> "original receipt was edited".
python3 - "$EDITED_DIR" "$AUTHED" "$SYNTH_RUN_ID" <<'PYEOF'
import json, os, sys
root, authed, synth = sys.argv[1:4]
p = os.path.join(root, "evidence", "receipts", "public-contracts", f"{synth}-1.json")
with open(p) as f: cur = json.load(f)
cur["approved"]["at"] = "2026-10-30T07:20:00Z"
with open(p, "w") as f:
    json.dump(cur, f, sort_keys=True, separators=(",", ":"), ensure_ascii=True)
    f.write("\n")
PYEOF
git -C "$EDITED_DIR" add "$SYNTH_RECEIPT"
git -C "$EDITED_DIR" -c user.name=o204 -c user.email=o204@local \
  commit -q -m "o204 later approval edit"
EDIT_HEAD="$(git -C "$EDITED_DIR" rev-parse HEAD)"
prove "edited receipt" block \
  python3 "$SCRIPTS_DIR/receipt_verifier.py" retained \
    --planning "$EDITED_DIR" --revision "$EDIT_HEAD" --receipt "$SYNTH_RECEIPT" \
    --candidate "$CANDIDATE" --plan "$P1"
echo

echo "negative-proofs: all three bounded negative cases behaved as intended."
