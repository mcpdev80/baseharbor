#!/usr/bin/env python3
"""Shared observed-state classifier for codeman lane observation.

Implements the worker recipes of codex-script-audit.md findings 1-3 and 6:
every collector distinguishes SUCCESSFUL observation from UNKNOWN (command
failure, nonzero rc, malformed JSON, unreadable pane). UNKNOWN is never
silently mapped to idle/gone/clear/success. API idle requires
IDLE_CONFIRM_POLLS consecutive polls plus absence of current tool/generation
or pending-dialog activity. Exited panes are reported separately.

Fixture injection points: RUN (subprocess runner), CODMAN_BIN, TMUX_BIN,
OPENCODE_DB.
"""
import json
import os
import re
import subprocess
import sys

IDLE_CONFIRM_POLLS = 2
RUN_TIMEOUT = 30
API_BUSY = {"busy", "working"}
TOOL_HINT = "esc interrupt"
DIALOG_MARKERS = ("Allow once", "Permission required", "Yes, proceed")
DIALOG_TAIL_LINES = 15


def _run(cmd, timeout=RUN_TIMEOUT):
    return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)


RUN = _run  # fixture override point


def codeman(args, timeout=RUN_TIMEOUT):
    """-> (ok, stdout). ok=False means the observation is UNKNOWN."""
    base = [os.environ.get("CODMAN_BIN", "codeman")] + list(args)
    try:
        res = RUN(base, timeout=timeout)
    except Exception:
        return False, ""
    if getattr(res, "returncode", 1) != 0:
        return False, getattr(res, "stdout", "") or ""
    return True, getattr(res, "stdout", "") or ""


def list_sessions():
    """-> (ok, sessions list). Never returns (True, None)."""
    ok, out = codeman(["agent", "list", "--json"])
    if not ok:
        return False, []
    try:
        data = json.loads(out)
    except Exception:
        return False, []
    if not isinstance(data, list):
        return False, []
    return True, data


def pane_read(sid, timeout=10):
    """-> (ok, text). Failed capture is UNKNOWN, never empty-but-clean."""
    base = os.environ.get("TMUX_BIN", "tmux")
    try:
        res = RUN([base, "-L", "codeman", "capture-pane", "-p", "-t",
                   "codeman-" + str(sid)[:8]], timeout=timeout)
    except Exception:
        return False, ""
    if getattr(res, "returncode", 1) != 0:
        return False, ""
    text = getattr(res, "stdout", "") or ""
    if not text.strip():
        # zero rc but empty screen: not a trustworthy observation
        return False, ""
    return True, text


def pane_exit(entry):
    """Nonzero exit status evidence for a session entry, or None if running."""
    pe = entry.get("paneExit")
    if isinstance(pe, dict):
        st = pe.get("status")
        if st is None:
            return None
        try:
            st = int(st)
        except (TypeError, ValueError):
            return "unknown"
        return st if st != 0 else None
    ec = entry.get("exitCode")
    if ec not in (None, ""):
        try:
            ec = int(ec)
        except (TypeError, ValueError):
            return "unknown"
        return ec if ec != 0 else None
    return None


def detect_dialog(text):
    """Active-prompt discrimination: only the rendered pane tail counts.

    Transcript body text that merely quotes a dialog phrase is NOT an active
    prompt. -> dialog dict or None.
    """
    lines = [l for l in (text or "").splitlines() if l.strip()
             and not re.search(r"Ask Codex to do anything|\^g\b.*(?:draft|edit|message)|(?:tip|shortcut).*\^g", l, re.I)]
    tail = "\n".join(lines[-DIALOG_TAIL_LINES:])
    markers = tuple(m for m in DIALOG_MARKERS if m in tail)
    if not markers:
        return None
    return {"markers": markers, "tail": tail}


def dialog_key_suggestion(dialog):
    """Derive a suggested key ONLY from rendered options; None => inspect manually.

    Never grants anything; this is advisory text for a human/coordinator.
    """
    tail = dialog.get("tail", "")
    for m in dialog.get("markers", ()):
        pat = re.compile(re.escape(m) + r"[^\n]*\(([A-Za-z])\)|\(([A-Za-z])\)[^\n]*" + re.escape(m) +
                         r"|\[(?![ ])([A-Za-z])\][^\n]*?" + re.escape(m) +
                         r"|" + re.escape(m) + r"[^\n]*?\[([A-Za-z])\]", re.I)
        hit = pat.search(tail)
        if hit:
            key = next(g for g in hit.groups() if g)
            if key.lower() in ("enter", "esc"):
                return key
            return {"y": "Enter"}.get(key.lower(), key)
    return None


def classify_lane(entry, pane_ok, pane_text, streak):
    """Shared observed-state classifier -> (state, streak).

    states: busy | tool-active | pending-dialog | idle-candidate |
            idle-confirmed | exited | blocked | gone | unknown
    """
    if entry is None:
        return "gone", 0
    pe = pane_exit(entry)
    if pe is not None:
        return "exited", 0
    status = (entry.get("status") or "").lower()
    if status in API_BUSY:
        return "busy", 0
    if status == "blocked":
        return "blocked", 0
    if status in ("exit", "exited"):
        return "exited", 0
    if status != "idle":
        return "unknown", streak
    if not pane_ok:
        # API says idle but the pane cannot prove absence of activity.
        return "unknown", streak
    if detect_dialog(pane_text):
        return "pending-dialog", 0
    if TOOL_HINT in pane_text:
        return "tool-active", 0
    streak += 1
    if streak >= IDLE_CONFIRM_POLLS:
        return "idle-confirmed", streak
    return "idle-candidate", streak


def model_class(pane_ok, text):
    if not pane_ok:
        return "unknown"
    joined = " ".join(sorted(set(re.findall(r"Build . ([A-Za-z0-9 ._-]+)", text or ""))))
    if "Flash" in joined:
        return "flash"
    if "3.6" in joined:
        return "qwen"
    if "Ornith" in joined:
        return "ornith"
    return "unknown"


def open_db():
    path = os.environ.get(
        "OPENCODE_DB", os.path.expanduser("~/.local/share/opencode/opencode.db"))
    try:
        import sqlite3
        con = sqlite3.connect("file:" + path + "?mode=ro", uri=True, timeout=5)
        con.execute("select count(*) from message").fetchone()
        return con
    except Exception:
        return None


def map_lane_to_oc_session(con, created_ms, name="", session_id=""):
    """Exact session.id first; title/time fallback only when unambiguous.

    -> (ok, oc_session_id | reason). Ambiguity/empty/error are UNKNOWN, not success.
    """
    if con is None:
        return False, "db-unavailable"
    try:
        if session_id:
            row = con.execute("select id from session where id=?", (session_id,)).fetchone()
            if row:
                return True, row[0]
        if name:
            rows = con.execute(
                "select id from session where lower(title)=lower(?)", (name,)).fetchall()
            if len(rows) == 1:
                return True, rows[0][0]
            if len(rows) > 1:
                return False, "ambiguous-title"
        if created_ms:
            rows = con.execute(
                "select id from session where abs(time_created-?)<=30000",
                (int(created_ms),)).fetchall()
            if len(rows) == 1:
                return True, rows[0][0]
            if len(rows) > 1:
                return False, "ambiguous-creation-window"
        return False, "no-db-session"
    except Exception as exc:
        return False, "db-error:{}".format(exc)


def message_roles(con, oc_id):
    """Ordered (time_created, role) rows; None on any failure (UNKNOWN)."""
    if con is None:
        return None
    try:
        rows = con.execute(
            "select time_created, data from message where session_id=? "
            "order by time_created, id", (oc_id,)).fetchall()
    except Exception:
        return None
    out = []
    for ts, data in rows:
        try:
            out.append((ts, json.loads(data).get("role")))
        except Exception:
            out.append((ts, "malformed"))
    return out


def session_by_id_or_name(sessions, target):
    for entry in sessions:
        if entry.get("id") == target or entry.get("name") == target:
            return entry
    return None


def main(argv=None):
    """CLI for shell consumers: `observed.py classify <session-target>` ->
    prints one state word on stdout, exit 0; exit 9 = observation UNKNOWN."""
    argv = list(argv if argv is not None else sys.argv[1:])
    if argv[:1] == ["classify"] and len(argv) > 1:
        ok, sessions = list_sessions()
        if not ok:
            print("unknown")
            return 9
        entry = session_by_id_or_name(sessions, argv[1])
        if entry is None:
            print("gone")
            return 0
        sid = entry.get("id") or argv[1]
        pok, ptext = pane_read(sid)
        state, _ = classify_lane(entry, pok, ptext, 0)
        print(state)
        return 0 if state not in ("unknown",) else 9
    print("usage: observed.py classify <session-target>")
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
