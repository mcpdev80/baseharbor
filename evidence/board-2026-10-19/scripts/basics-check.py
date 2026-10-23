#!/usr/bin/env python3
"""Coordinator turn-start basics check: flags, not prose.

Exit codes: 0 = ran (flags may exist); 9 = BOARD currency NOT verified
(--board-json mode); the plain mode keeps exit 0 per its contract.

Finding 4: every git command is return-code checked, a fresh successful
`fetch` happens BEFORE ahead/behind are computed from the refs, and failed
observation surfaces as UNKNOWN currency - never as a silently clear board.
Finding 1/6: model mix comes from verified pane headers, exited panes are
reported separately, and idle lanes require the watcher's confirmed-idle
evidence rather than one raw API sample.
"""
import json
import os
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import observed

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.environ.get("BH_ROOT_DIR") or os.path.abspath(os.path.join(HERE, "..", "..", ".."))
PLAN = os.environ.get("BH_COORDINATOR_PLAN") or os.path.join(
    ROOT, "evidence", "board-2026-10-22", "coordinator-plan.md")
STATE = os.environ.get("WATCHER_STATE") or os.path.join(HERE, "watcher-state.json")
CURRENCY = os.path.join(HERE, "board-currency.json")


def sh(cmd, timeout=30):
    """Checked run -> (rc, stdout, stderr). rc=128 signals a failed observation."""
    try:
        res = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
        return res.returncode, res.stdout or "", res.stderr or ""
    except Exception as exc:
        return 128, "", str(exc)


def board_state():
    """Fresh successful remote observation FIRST, then snapshot arithmetic."""
    plan_repo = os.environ.get("BH_PLAN_REPO") or os.path.join(ROOT, "baseharbor-plan")
    detail = {"repo": plan_repo, "observed_at": int(time.time())}
    rc, out, _ = sh(["git", "-C", plan_repo, "rev-parse", "--is-inside-work-tree"])
    if rc != 0:
        detail.update(currency="unknown", why="not a git worktree (rc {})".format(rc))
        return detail
    rc, branch, _ = sh(["git", "-C", plan_repo, "rev-parse", "--abbrev-ref", "HEAD"])
    if rc != 0:
        detail.update(currency="unknown", why="branch unreadable (rc {})".format(rc))
        return detail
    branch = branch.strip()
    detail["branch"] = branch
    rc, _o, err = sh(["git", "-C", plan_repo, "fetch", "origin", "main"], timeout=60)
    if rc != 0:
        detail.update(currency="unknown",
                      why="fetch origin main FAILED (rc {}): {} - refs may be stale, "
                      "currency NOT verified".format(rc, err.strip()[:120]))
        return detail
    rc, _o, _e = sh(["git", "-C", plan_repo, "rev-parse", "--verify", "origin/main"])
    if rc != 0:
        detail.update(currency="unknown", why="origin/main ref missing after fetch")
        return detail
    rc, ahead, _ = sh(["git", "-C", plan_repo, "rev-list", "--count", "origin/main..HEAD"])
    if rc != 0:
        detail.update(currency="unknown", why="ahead computation failed (rc {})".format(rc))
        return detail
    rc, behind, _ = sh(["git", "-C", plan_repo, "rev-list", "--count", "HEAD..origin/main"])
    if rc != 0:
        detail.update(currency="unknown", why="behind computation failed (rc {})".format(rc))
        return detail
    rc, dirty, _ = sh(["git", "-C", plan_repo, "status", "--porcelain"])
    if rc != 0:
        detail.update(currency="unknown", why="dirty computation failed (rc {})".format(rc))
        return detail
    ahead, behind = ahead.strip(), behind.strip()
    dirty_n = len([l for l in dirty.splitlines() if l.strip()])
    detail.update(ahead=int(ahead or 0), behind=int(behind or 0), dirty=dirty_n)
    if branch != "main":
        detail.update(currency="stale", why="worktree on branch '{}', expected main".format(branch))
    elif int(behind or 0) > 0:
        detail.update(currency="stale", why="remote planning {} commit(s) ahead - REALITAETSABGLEICH".format(behind))
    elif dirty_n > 0:
        detail.update(currency="stale", why="{} uncommitted change(s)".format(dirty_n))
    else:
        detail["currency"] = "ok"
    if int(ahead or 0) > 0:
        detail["unpushed"] = int(ahead)
    return detail


def board_flags(detail):
    flags = []
    cur = detail.get("currency", "unknown")
    if cur == "unknown":
        flags.append("BOARD: currency UNKNOWN ({}) - verify remote BEFORE dispatch, "
                     "never label currency verified from a failed fetch".format(
                         detail.get("why", "no fresh observation")))
    elif cur == "stale":
        flags.append("BOARD: currency STALE ({}) - reconcile before dispatch".format(
            detail.get("why", "")))
    if detail.get("unpushed"):
        flags.append("BOARD: planning has {} unpushed commit(s) - PUBLISH NOW".format(
            detail["unpushed"]))
    return flags


def main(argv):
    board_only = "--board-json" in argv
    detail = board_state()
    if board_only:
        try:
            tmp = CURRENCY + ".tmp"
            with open(tmp, "w") as fh:
                json.dump(detail, fh, sort_keys=True)
            os.replace(tmp, CURRENCY)
        except Exception:
            pass
        print(json.dumps({"currency": detail.get("currency", "unknown"),
                          "detail": detail, "flags": board_flags(detail)}))
        return 0 if detail.get("currency") == "ok" else 9
    flags = []
    try:
        st = os.statvfs("/tmp")
        free_mb = st.f_bavail * st.f_frsize // 1048576
        if free_mb < 1500:
            flags.append("DISK: /tmp only {}MB free (<1500) - bun EFS extraction will fail, sweep before ANY spawn".format(free_mb))
    except Exception:
        pass
    rc, ev, err = sh([sys.executable, os.path.join(HERE, "events-check.py")])
    if rc != 0:
        flags.append("EVENTS: checker FAILED rc={} ({}) - queue state UNKNOWN, not clear".format(
            rc, err.strip()[:80]))
    else:
        pend = [l for l in ev.strip().splitlines() if l]
        if pend:
            flags.append("EVENTS: {} un-acked (run events-check.py --ack after processing)".format(len(pend)))
    rrc, rout, rerr = sh([sys.executable, os.path.join(HERE, "events-check.py"), "--reminders", "--json"])
    try:
        reminder_summary = json.loads(rout) if rrc == 0 else {"unread": None, "error": rerr.strip()}
    except ValueError:
        reminder_summary = {"unread": None, "error": "malformed reminder checker output"}
    if reminder_summary.get("unread"):
        flags.append("REMINDERS: {} unread (events-check.py --reminders; acknowledge after processing)".format(reminder_summary["unread"]))
    flags.extend(board_flags(detail))
    rc_l, out_l, _ = sh([os.environ.get("CODMAN_BIN", "codeman"), "agent", "list", "--json"])
    if rc_l != 0:
        flags.append("OBS: codeman session list FAILED (rc {}) - pool/idle state UNKNOWN, "
                     "NOT treated as clear".format(rc_l))
        slist = None
    else:
        try:
            slist = json.loads(out_l)
            if not isinstance(slist, list):
                raise ValueError("not a list")
        except Exception as exc:
            flags.append("OBS: session list output malformed ({}) - pool/idle UNKNOWN".format(exc))
            slist = None
    now = time.time()
    state = {}
    try:
        state = json.load(open(STATE))
    except Exception:
        pass
    busy_since = state.get("busy_since", {})
    idle_streak = state.get("idle_streak", {})
    mix = {"flash": 0, "ornith": 0, "qwen": 0, "codex": 0}
    idle_over = []
    exited = []
    busy_over = []
    unconfirmed = []
    if slist is not None:
        for s in slist:
            name = s.get("name", "")
            status = s.get("status", "")
            sid = s.get("id", "")
            low = name.lower()
            if low.startswith("w1"):
                continue
            pe = observed.pane_exit(s)
            if pe is not None:
                exited.append("{} (paneExit {})".format(name or sid[:8], pe))
                continue
            if "codex" in low:
                mix["codex"] += 1
                continue
            pok, ptext = observed.pane_read(sid or name)
            cls = observed.model_class(pok, ptext)
            if cls == "unknown":
                flags.append("OBS: model identity UNKNOWN for {} - excluded from pool "
                             "counts (not inferred from name letters)".format(name or sid[:8]))
            elif cls in mix:
                mix[cls] += 1
            if status == "idle":
                if sid and idle_streak.get(sid, 0) >= observed.IDLE_CONFIRM_POLLS:
                    idle_over.append(name)
                elif pok:
                    unconfirmed.append(name)
            bsy = busy_since.get(sid)
            if status == "busy" and isinstance(bsy, dict) and time.monotonic() - bsy.get("since", 0) > 2400:
                busy_over.append("{} busy>40min".format(name))
    if exited:
        flags.append("EXITED PANES (separate signal, NOT task completion; verify evidence, "
                     "then rm or restart): {}".format(", ".join(sorted(set(exited)))))
    if idle_over:
        flags.append("IDLE LANES (idle-CONFIRMED by 2-poll watch evidence; verify+rm or "
                     "re-task): {}".format(", ".join(sorted(set(idle_over)))))
    if unconfirmed:
        flags.append("OBS: {} idle lane(s) NOT yet 2-poll confirmed (no watcher evidence) - "
                     "not reported as terminal".format(len(unconfirmed)))
    flags.extend(busy_over)
    want = {"flash": 1, "ornith": 2, "qwen": 2}
    for host, n in want.items():
        if mix.get(host, 0) < n:
            flags.append("POOL: {} worker(s) below target ({}<{}) if real work exists".format(host, mix.get(host, 0), n))
    if os.path.exists(PLAN):
        age_min = (now - os.path.getmtime(PLAN)) / 60
        if age_min > 45:
            flags.append("PLAN: coordinator-plan.md untouched {:.0f}min".format(age_min))
        try:
            txt = open(PLAN).read()
            open_steps = txt.count("- [ ]")
            if open_steps:
                flags.append("PLAN: {} open step(s), newest lane work must appear here".format(open_steps))
        except Exception as exc:
            flags.append("PLAN: coordinator-plan unreadable ({}) - UNKNOWN".format(exc))
    if "--json" in argv:
        print(json.dumps({"flags": flags, "board": detail.get("currency", "unknown"), "reminder-summary": reminder_summary}))
        return 0
    print("BASICS: " + ("OK" if not [f for f in flags if not f.startswith("PLAN:")] else "FLAGS"))
    for f in flags:
        print(" - " + f)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
