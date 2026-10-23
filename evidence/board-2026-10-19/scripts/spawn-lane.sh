#!/usr/bin/env bash
# NOTE: --auto-compact-* is cosmetic for opencode lanes (codeman token counter reads 0);
# the watcher compact_watch (wait-workers.py, 200K default) is the real mechanism.
# spawn-lane.sh <name> [--qwen|--flash]
# Mandatory lane spawn (audit finding 7): capacity admission + pending
# reservation INSIDE the same flock as the spawn, so two same-model callers
# cannot both pass a preflight and overshoot the limit. Unknown occupancy is a
# HOLD, never a free slot. The environment is restored by an EXIT trap that is
# installed BEFORE any mutation, so a spawn/parse failure cannot leak the
# global override (f8-incident class). Readiness requires the rendered model
# header AND a live readable prompt; watcher registration must succeed before
# the lane is reported ready - failed lanes are retained for diagnosis.
# Codex sessions are never spawned here.
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
name=""; override="$HOME/.config/opencode/opencode.json"; want="ornith"
for a in "$@"; do
  case "$a" in
    --qwen) override="$HOME/.config/opencode/qwen36.json"; want="qwen" ;;
    --flash) override="$HOME/.config/opencode/flash2.json"; want="flash" ;;
    -*) echo "spawn-lane: unknown flag $a" >&2; exit 2 ;;
    *) name="$a" ;;
  esac
done
[[ -n "$name" ]] || { echo "usage: spawn-lane.sh <name> [--qwen|--flash]" >&2; exit 2; }
[[ -f "$override" ]] || { echo "spawn-lane: config missing: $override" >&2; exit 1; }
LOCK="${SPAWN_LOCK:-$HOME/.codeman/.spawn-lane.lock}"
mkdir -p "$(dirname "$LOCK")" 2>/dev/null || true
TMUX="${TMUX_BIN:-tmux}"
CODEMAN="${CODMAN_BIN:-codeman}"

PREV_ENV="$(mktemp 2>/dev/null || echo /tmp/spawn-lane-prev.$$.env)"
"$TMUX" -L codeman show-environment -g OPENCODE_CONFIG 2>/dev/null \
  | sed -n 's/^OPENCODE_CONFIG=//p' > "$PREV_ENV" || true
RESTORED=0
restore() {
  [[ "$RESTORED" == 1 ]] && return
  RESTORED=1
  if [[ -s "$PREV_ENV" ]]; then
    "$TMUX" -L codeman set-environment -g OPENCODE_CONFIG "$(cat "$PREV_ENV")" >/dev/null 2>&1 || true
  else
    "$TMUX" -L codeman set-environment -gu OPENCODE_CONFIG >/dev/null 2>&1 || true
  fi
  RES_OWNER="$$" python3 "$DIR/pool-admission.py" release >/dev/null 2>&1 || true
  rm -f "$PREV_ENV" 2>/dev/null || true
}
trap restore EXIT INT TERM

exec 9>"$LOCK" || { echo "spawn-lane: cannot open lock $LOCK" >&2; exit 1; }
flock 9 || { echo "spawn-lane: cannot acquire lock" >&2; exit 1; }

# Capacity admission WITH reservation inside the lock (unknown => hold).
adm="$(RES_OWNER=$$ python3 "$DIR/pool-admission.py" check "$want" 2>&1)"
adm_rc=$?
if [[ $adm_rc -ne 0 ]]; then
  echo "spawn-lane: ADMISION-HOLD [$adm]" >&2
  exit $(( adm_rc == 5 ? 4 : adm_rc ))
fi
echo "spawn-lane: $adm" >&2

"$TMUX" -L codeman set-environment -g OPENCODE_CONFIG "$override" || {
  echo "spawn-lane: set-environment failed (trap restores)" >&2; exit 1; }
spawn_json="$("$CODEMAN" agent spawn baseharbor -m opencode -n "$name" --timeout 45000 --json 2>/dev/null)"
sp_rc=$?
"$TMUX" -L codeman set-environment -gu OPENCODE_CONFIG >/dev/null 2>&1 || true
if [[ $sp_rc -ne 0 ]]; then
  echo "spawn-lane: codeman spawn FAILED rc=$sp_rc - reservation/env cleaned by trap, lane NOT created" >&2
  exit 1
fi
sid="$(printf '%s' "$spawn_json" | python3 -c "import json,sys
try:
    d=json.loads(sys.stdin.read() or '{}')
except Exception:
    d={}
print(d.get('sessionId') or d.get('id') or '')")"
[[ -n "$sid" ]] || { echo "spawn-lane: no session id resolved (spawn/parse failure) - trap cleans env+reservation" >&2; exit 1; }
# the live session now occupies the slot; drop the pending reservation
RES_OWNER=$$ python3 "$DIR/pool-admission.py" release >/dev/null 2>&1 || true

case "$want" in
  flash) want_re="Flash" ;;
  qwen) want_re="3\.6" ;;
  *) want_re="Ornith" ;;
esac
# Readiness gate: rendered model header AND a live readable prompt (finding 7:
# header-only was proven insufficient for dispatch readiness).
ident=""; ready=0
for _ in $(seq 1 20); do
  sleep 2
  pane_text="$("$TMUX" -L codeman capture-pane -p -t "codeman-${sid:0:8}" 2>/dev/null)"
  pane_rc=$?
  [[ $pane_rc -ne 0 || -z "$pane_text" ]] && continue   # unreadable pane != ready
  ident="$(printf '%s' "$pane_text" | grep -aoE "Build . [A-Za-z0-9 ._-]+" | sort -u | tail -1 || true)"
  [[ -n "$ident" ]] || continue
  if printf '%s' "$pane_text" | grep -aqiE "esc interrupt|│ >|ctrl|up/down"; then
    ready=1; break
  fi
done
if ! [[ "${ident:-}" =~ $want_re ]]; then
  echo "spawn-lane: IDENTITY-MISMATCH want=$want got='${ident:-none}' sid=$sid (lane preserved, decide manually)" >&2
  exit 3
fi
if [[ $ready -ne 1 ]]; then
  echo "spawn-lane: NOT-READY: header seen but no live prompt after 40s - sid=$sid preserved for diagnosis, do NOT dispatch to it" >&2
  exit 7
fi
echo "spawn-lane: VERIFIED $want ($ident) live prompt ok" >&2
PW="$(tr '\0' '\n' < "/proc/$(pgrep -f 'codeman web' | head -1)/environ" 2>/dev/null | sed -n 's/^CODEMAN_PASSWORD=//p')" || PW=""
thr=200000
case "$override" in
  *flash2.json) thr=400000 ;;
esac
resp="$(curl -sf -u "admin:$PW" -X POST -H 'Content-Type: application/json' \
  -d "{\"enabled\":true,\"threshold\":$thr}" \
  "http://127.0.0.1:3000/api/sessions/$sid/auto-compact" 2>/dev/null || true)"
echo "$resp" | grep -q '"success":true' || echo "spawn-lane: WARN auto-compact not confirmed for $sid" >&2
if ! python3 "$DIR/wait-workers.py" watch add "$sid" >/dev/null 2>&1; then
  echo "spawn-lane: WATCH-ADD-FAILED for $sid - lane NOT marked ready for dispatch (MONITORING HOLD), lane preserved; fix watcher then 'watch add' manually" >&2
  echo "$sid" >&2
  exit 8
fi
echo "$sid"
