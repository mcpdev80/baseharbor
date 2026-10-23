#!/usr/bin/env bash
# dispatch-lane.sh <session-id> <prompt-file> [--dry-run]
#
# Delivery contract per codex-script-audit finding 2: a busy-flip is NOT
# delivery evidence. We record dispatch identity (nonce, baseline message
# count, send rc) in dispatch-log.jsonl, then PROVE the mission landed via DB
# message identity. A resend happens ONLY after verified non-delivery, at most
# once, never while delivery is UNKNOWN. Board currency (finding 4) is a
# precondition: UNKNOWN/stale currency holds dispatch.
#
# Exit codes: 0 delivered/answered, 3 delivered-pending, 4 dialog needs human,
# 5 confirmed stall, 6 observation UNKNOWN (inspect, no auto action),
# 7 helper error, 8 BOARD CURRENCY HOLD, 9 delivered but MONITORING HOLD
# (watch registration failed - lane keeps the mission), 2 usage.
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
CODEMAN="${CODMAN_BIN:-codeman}"
BASICS_PY="${BASICS_CHECK_PY:-$DIR/basics-check.py}"
DRY=0
pos=()
for a in "$@"; do
  case "$a" in
    --dry-run) DRY=1 ;;
    -*) echo "dispatch-lane: unknown flag $a" >&2; exit 2 ;;
    *) pos+=("$a") ;;
  esac
done
[[ ${#pos[@]} -ge 2 ]] || { echo "usage: dispatch-lane.sh <session-id> <prompt-file> [--dry-run]" >&2; exit 2; }
sid="${pos[0]}"; pf="${pos[1]}"
[[ -f "$pf" ]] || { echo "dispatch-lane: prompt file missing: $pf" >&2; exit 7; }
MSG="Read the file $pf with cat/Read and execute every instruction in it exactly, starting with its PLAN step 0 in your native TodoWrite tool AND your evidence file."
NONCE="dispatch-$(date +%s)-$$-$RANDOM"

log() { # durable dispatch identity record (finding 2)
  printf '%s\n' "$1" >> "$DIR/dispatch-log.jsonl"
}

# --- Precondition: board currency (finding 4) ----------------------------------
if [[ -x "$BASICS_PY" ]]; then
  board_out="$("$BASICS_PY" --board-json 2>/dev/null)"
else
  board_out="$(python3 "$BASICS_PY" --board-json 2>/dev/null)"
fi
board_rc=$?
if [[ $board_rc -eq 9 ]]; then
  cur="$(python3 -c "import json,sys;print(json.loads(sys.stdin.read()).get('currency','unknown'))" <<<"$board_out" 2>/dev/null || echo unparseable)"
  echo "dispatch-lane: BOARD-CURRENCY-HOLD (currency=$cur) - reality check/reconcile FIRST; nothing dispatched" >&2
  log "{\"nonce\":\"$NONCE\",\"sid\":\"$sid\",\"outcome\":\"currency-hold\",\"currency\":\"$cur\"}"
  exit 8
elif [[ $board_rc -ne 0 ]]; then
  echo "dispatch-lane: board checker FAILED rc=$board_rc - currency UNKNOWN, dispatch HELD" >&2
  log "{\"nonce\":\"$NONCE\",\"sid\":\"$sid\",\"outcome\":\"currency-unknown\",\"rc\":$board_rc}"
  exit 8
fi

# --- Resolve exact session (read-only) -----------------------------------------
if ! "$CODEMAN" agent list --json 2>/dev/null | python3 -c "
import json,sys
t=sys.argv[1]
sys.exit(0 if sum(s.get('id')==t or s.get('name')==t or (len(t)>=8 and str(s.get('id','')).startswith(t)) for s in json.load(sys.stdin))==1 else 1)" "$sid"; then
  echo "dispatch-lane: session $sid not resolvable - UNKNOWN, nothing sent" >&2
  log "{\"nonce\":\"$NONCE\",\"sid\":\"$sid\",\"outcome\":\"unknown-session\"}"
  exit 6
fi
if [[ $DRY -eq 1 ]]; then
  base_out="$(python3 "$DIR/lane-stalled.py" "$sid" --baseline)"
  base_rc=$?
  echo "dispatch-lane DRY-RUN: session resolved, currency ok, baseline_rc=$base_rc ($base_out); NO text sent, code path validated" >&2
  log "{\"nonce\":\"$NONCE\",\"sid\":\"$sid\",\"outcome\":\"dry-run\",\"baseline_rc\":$base_rc}"
  exit 0
fi

# --- Baseline message identity ------------------------------------------------
base_out="$(python3 "$DIR/lane-stalled.py" "$sid" --baseline)"
base_rc=$?
base_users=""
if [[ $base_rc -eq 0 ]]; then
  base_users="$(sed 's/users=//' <<<"${base_out%% *}" | tr -cd '0-9')"
fi

probe_resend() { # verify via message identity; NEVER blindly resend
  local out rc
  if [[ -z "$base_users" ]]; then return 6; fi
  out="$(python3 "$DIR/lane-stalled.py" "$sid" --verify-delivery "$base_users" "${VERIFY_WAIT:-25}" 2>/dev/null)"
  rc=$?
  if [[ $rc -eq 3 && "$out" == tool-active* ]]; then
    # Busy/tool activity is NOT mission receipt: identity did not advance.
    echo "dispatch-lane: tool-active but message identity did not advance - delivery UNPROVEN, inspect lane" >&2
    return 6
  fi
  return $rc
}

if [[ $base_rc -ne 0 ]]; then
  echo "dispatch-lane: baseline UNKNOWN (rc=$base_rc) - sending at most once, NO automatic resend until delivery can be proven" >&2
  "$CODEMAN" agent send "$sid" "$MSG" >/dev/null 2>&1
  send_rc=$?
  log "{\"nonce\":\"$NONCE\",\"sid\":\"$sid\",\"outcome\":\"blind-single-send\",\"send_rc\":$send_rc,\"baseline_rc\":$base_rc}"
  [[ $send_rc -eq 0 ]] || { echo "dispatch-lane: send FAILED rc=$send_rc, delivery unconfirmed" >&2; exit 6; }
  echo "dispatch-lane: SENT-UNVERIFIED - inspect lane-stalled/pane before ANY further action" >&2
  exit 6
fi

"$CODEMAN" agent send "$sid" "$MSG" >/dev/null 2>&1
send_rc=$?
log "{\"nonce\":\"$NONCE\",\"sid\":\"$sid\",\"baseline_users\":$base_users,\"send_rc\":$send_rc,\"resends\":0}"
if [[ $send_rc -ne 0 ]]; then
  # Proven non-delivery by send rc: exactly one retry allowed.
  echo "dispatch-lane: send rc=$send_rc (proven loss), ONE retry" >&2
  "$CODEMAN" agent send "$sid" "$MSG" >/dev/null 2>&1 || {
    echo "dispatch-lane: retry send FAILED too - stop, inspect" >&2; exit 6; }
  probe_resend ""
  rc=$?
else
  probe_resend ""
  rc=$?
fi

case $rc in
  3|0)
    if [[ $rc -eq 3 ]]; then
      python3 "$DIR/lane-stalled.py" "$sid" "${STALL_WAIT:-90}" >/dev/null 2>&1; rc2=$?
      if [[ $rc2 -eq 5 ]]; then
        # A stall verdict must not contradict a live working lane nor rest on an
        # UNKNOWN observation (findings 1+2: stall is never guessed).
        lane_state="$(python3 "$DIR/observed.py" classify "$sid" 2>/dev/null)"; ls_rc=$?
        if [[ $ls_rc -ne 0 ]]; then
          echo "dispatch-lane: delivered but lane state UNKNOWN after wait - retain lane, inspect, no stall verdict" >&2
          exit 6
        fi
        case "$lane_state" in
          busy|working|tool-active)
            echo "dispatch-lane: delivered, lane still working - no stall verdict, monitoring continues" >&2 ;;
          *)
            echo "dispatch-lane: delivered but STALLED after wait - respawn decision belongs to owner, no auto resend" >&2
            exit 5 ;;
        esac
      fi
    fi
    echo "dispatch-lane: DELIVERED (message identity proven), no busy-flip guessing" >&2
    python3 "$DIR/wait-workers.py" watch add "$sid" >/dev/null 2>&1 || {
      echo "dispatch-lane: MONITORING-HOLD - delivered but watcher registration FAILED; fix watcher, do NOT re-dispatch" >&2
      exit 9; }
    exit 0 ;;
  4)
    echo "dispatch-lane: delivered-pending behind an ACTIVE DIALOG - human decision required, no auto-approval, no resend" >&2
    python3 "$DIR/wait-workers.py" watch add "$sid" >/dev/null 2>&1 || true
    exit 4 ;;
  1)
    # Verified non-delivery of THIS dispatch exactly once -> one retry total.
    echo "dispatch-lane: verified non-delivery, ONE corrective resend" >&2
    "$CODEMAN" agent send "$sid" "$MSG" >/dev/null 2>&1 || { echo "dispatch-lane: resend failed" >&2; exit 6; }
    log "{\"nonce\":\"$NONCE\",\"sid\":\"$sid\",\"resends\":1}"
    base_now="$(python3 "$DIR/lane-stalled.py" "$sid" --baseline | sed 's/users=//' | sed 's/ .*//')"
    python3 "$DIR/lane-stalled.py" "$sid" --verify-delivery "${base_now:-0}" "${VERIFY_WAIT:-25}" >/dev/null 2>&1
    rc3=$?
    if [[ $rc3 -eq 3 || $rc3 -eq 0 ]]; then
      echo "dispatch-lane: recovered after the single proven-loss retry" >&2
      python3 "$DIR/wait-workers.py" watch add "$sid" >/dev/null 2>&1 || { echo "dispatch-lane: MONITORING-HOLD" >&2; exit 9; }
      exit 0
    fi
    echo "dispatch-lane: still unconfirmed (rc=$rc3) - STOP: no third send; inspect pane/DB (exit 6)" >&2
    exit 6 ;;
  5)
    echo "dispatch-lane: CONFIRMED stall with healthy observation - respawn decision belongs to owner; NOT resending" >&2
    exit 5 ;;
  6|7)
    echo "dispatch-lane: delivery/execution UNKNOWN (rc=$rc) - retain the lane, inspect before ANY repeat action" >&2
    exit 6 ;;
  *)
    echo "dispatch-lane: probe returned unexpected rc=$rc" >&2
    exit 7 ;;
esac
