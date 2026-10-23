#!/usr/bin/env python3
"""lane-stalled.py <codeman-session-id> [mode args] - delivery/execution probe.

Exit codes (finding 2: UNKNOWN is never success, stall is never guessed):
  0 answered      assistant content observed after the baseline
  1 not-delivered PROVEN non-delivery: healthy DB observation, baseline known,
                  no new user message after wait (dispatch-lane may retry ONCE)
  3 delivered     mission user row present, execution pending/running
  4 dialog        active permission dialog at pane tail (needs human)
  5 stalled       confirmed queue stall: user rows pending, no assistant output
  6 unknown       session/DB mapping ambiguous, empty DB, DB unavailable
  7 helper-error  unexpected failure

Modes:
  lane-stalled.py <sid> [wait]                 classic watchdog probe
  lane-stalled.py <sid> --baseline             print "users=<N> oc=<id>"; exit 0/6
  lane-stalled.py <sid> --verify-delivery <baseline_users> [wait]
"""
import json
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import observed


def resolve(sid):
    """Exact session resolution via codeman list; failure is UNKNOWN (6)."""
    ok, sessions = observed.list_sessions()
    if not ok:
        print("unknown: session list observation failed")
        return None, None
    exact = [s for s in sessions if s.get("id") == sid or s.get("name") == sid]
    matches = exact or [s for s in sessions if len(str(sid)) >= 8 and str(s.get("id", "")).startswith(str(sid))]
    if len(matches) == 1:
        return matches[0], sessions
    print("unknown-session")
    return None, None


def main(argv):
    if not argv or argv[0] == "--help":
        print("usage: lane-stalled.py <sid> [wait|--baseline|--verify-delivery N [wait]]")
        return 2
    sid = argv[0]
    args = argv[1:]
    sess, _all = resolve(sid)
    if sess is None:
        return 6
    con = observed.open_db()
    if con is None:
        print("unknown: DB unavailable")
        return 6
    candidates = [sess.get(k) for k in ("opencodeSessionId", "openCodeSessionId", "sessionId", "id")]
    exact_id = next((v for v in candidates if v and con.execute(
        "select id from session where id=?", (v,)).fetchone()), "")
    ok, mapping = observed.map_lane_to_oc_session(con, sess.get("createdAt") or 0,
                                                  sess.get("name") or "", exact_id)
    if ok and not exact_id:
        print("approx: title/time fallback; exact OpenCode session ID unavailable", file=sys.stderr)
    if not ok:
        print("unknown: {}".format(mapping))
        return 6
    oc = mapping
    rows = observed.message_roles(con, oc)
    if rows is None:
        print("unknown: message read failed")
        return 6
    user_positions = [i for i, (_t, role) in enumerate(rows) if role == "user"]
    baseline_users = len(user_positions)
    if "--baseline" in args:
        print("users={} oc={}".format(baseline_users, oc))
        return 0
    if args and args[0] == "--verify-delivery":
        try:
            base = int(args[1])
        except (IndexError, ValueError):
            print("unknown: missing baseline")
            return 6
        wait = int(args[2]) if len(args) > 2 else 20
        deadline = time.time() + wait
        while True:
            rows2 = observed.message_roles(con, oc)
            if rows2 is None:
                print("unknown: message read failed mid-verify")
                return 6
            users2 = sum(1 for _t, r in rows2 if r == "user")
            if users2 > base:
                print("delivered users={} -> {}".format(base, users2))
                return 3
            pok, ptext = observed.pane_read(sess.get("id") or sid)
            if pok and observed.detect_dialog(ptext):
                print("dialog pending (mission not provable in DB, pane shows dialog)")
                return 4
            if pok and observed.TOOL_HINT in ptext:
                print("tool-active: DB shows no new user row but pane proves live work")
                return 3
            if time.time() >= deadline:
                # A SUCCESSFUL observation showing no new user message proves
                # non-delivery only if the pane ALSO shows no evidence of the
                # mission running. Pane UNKNOWN -> not proven -> no resend.
                pok2, ptext2 = observed.pane_read(sess.get("id") or sid)
                if not pok2:
                    print("unknown: DB shows nothing new AND pane unreadable")
                    return 6
                print("not-delivered: healthy DB shows no new user row; pane clean")
                return 1
            time.sleep(4)
    # classic watchdog mode: is the LAST dispatch answered / stalled / running?
    wait = int(args[0]) if args and args[0].lstrip("-").isdigit() else 0
    deadline = time.time() + wait
    while True:
        if not rows:
            print("unknown: empty DB conversation (cannot distinguish idle-fresh "
                  "from undelivered mission without baseline)")
            return 6
        last_user = max((i for i, r in enumerate(rows) if r[1] == "user"), default=-1)
        last_assistant = max((i for i, r in enumerate(rows) if r[1] == "assistant"),
                             default=-1)
        pok, ptext = observed.pane_read(sess.get("id") or sid)
        if pok and observed.detect_dialog(ptext):
            print("dialog")
            return 4
        if pok and observed.TOOL_HINT in ptext:
            print("running-tool")
            return 3
        if last_assistant > last_user:
            print("answered")
            return 0
        if time.time() >= deadline:
            print("stalled users-after-assistant={} pane_ok={}".format(
                max(last_user - last_assistant, 0), pok))
            return 5
        time.sleep(5)
        rows = observed.message_roles(con, oc)
        if rows is None:
            print("unknown: message read failed mid-wait")
            return 6


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
