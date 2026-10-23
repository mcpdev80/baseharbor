#!/usr/bin/env python3
"""Pool capacity admission with pending reservations (audit finding 7).

Designed to run INSIDE the spawn-lane.sh flock so capacity admission and the
reservation land in the same critical section as the spawn itself.
Counts live worker panes by verified model header; the coordinator is excluded
by registered identity (role in watcher-state / watcher coordinator field),
not by lane-name letters; exited panes are excluded. ANY failed observation
(list, pane, unreadable header) means occupancy UNKNOWN -> hold, never a
phantom free slot.

Modes: check <want> (admit+reserve), release (drop caller's reservation).
Exit: 0 admitted, 4 pool full, 5 occupancy UNKNOWN.
"""
import json
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import observed

HERE = os.path.dirname(os.path.abspath(__file__))
LIMITS = {"flash": 1, "ornith": 2, "qwen": 2}


def resfile():
    return os.environ.get("POOL_RES_FILE") or os.path.join(HERE, "pool-reservations.json")


def load():
    try:
        with open(resfile()) as fh:
            d = json.load(fh)
    except Exception:
        d = {}
    out = {}
    for pid, e in (d or {}).items():
        if os.path.isdir("/proc/{}".format(pid)) or str(pid) == str(os.environ.get("RES_OWNER", "")):
            out[pid] = e
    return out


def store(d):
    tmp = resfile() + ".tmp"
    with open(tmp, "w") as fh:
        json.dump(d, fh, sort_keys=True)
    os.replace(tmp, resfile())


def main(argv):
    mode = argv[0] if argv else "check"
    owner = str(os.environ.get("RES_OWNER", os.getpid()))
    if mode == "release":
        d = load()
        d.pop(owner, None)
        store(d)
        return 0
    if mode != "check" or len(argv) < 2:
        print("usage: pool-admission.py check <model>|release")
        return 7
    want = argv[1]
    ok, sessions = observed.list_sessions()
    if not ok:
        print("OCCUPANCY UNKNOWN: session list observation failed - HOLD")
        return 5
    co_ids = set()
    try:
        # Active state file per the WATCHER_STATE override contract that every
        # other consumer honors (wait-workers._state_path); a hardcoded default
        # here read the WRONG state and missed the coordinator identity.
        with open(os.environ.get("WATCHER_STATE") or
                  os.path.join(HERE, "watcher-state.json")) as fh:
            st = json.load(fh)
        co_ids |= {k for k, v in st.get("registered", {}).items()
                   if isinstance(v, dict) and v.get("role") == "coordinator"}
        w = st.get("watcher") or {}
        if isinstance(w, dict) and w.get("coordinator"):
            co_ids.add(w["coordinator"])
    except Exception:
        pass
    counts = {"flash": 0, "ornith": 0, "qwen": 0}
    for s in sessions:
        sid, nm = s.get("id", ""), (s.get("name") or "")
        if sid in co_ids or nm.lower().startswith("w1") or "codex" in nm.lower():
            continue
        if observed.pane_exit(s) is not None:
            continue
        pok, ptext = observed.pane_read(sid)
        if not pok:
            print("OCCUPANCY UNKNOWN: pane unreadable for {} - HOLD".format(
                nm or sid[:8]))
            return 5
        cls = observed.model_class(pok, ptext)
        if cls == "unknown":
            print("OCCUPANCY UNKNOWN: model header missing for {} - HOLD".format(
                nm or sid[:8]))
            return 5
        counts[cls] = counts.get(cls, 0) + 1
    pending = load()
    for _pid, e in pending.items():
        m = e.get("model")
        if m in counts:
            counts[m] += 1
    lim = LIMITS.get(want, 2)
    if counts.get(want, 0) >= lim:
        print("POOL-FULL {} {}/{} (incl. reservations)".format(want, counts.get(want, 0), lim))
        return 4
    pending[owner] = {"model": want, "ts": int(time.time())}
    store(pending)
    print("ADMITTED {} live+reserved {}/{}".format(want, counts.get(want, 0), lim))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
