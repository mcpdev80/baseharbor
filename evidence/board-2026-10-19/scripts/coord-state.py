#!/usr/bin/env python3
"""Coordinator state snapshot (takeover insurance).

Writes a machine-readable picture of what the coordinator is currently
orchestrating, so that a new session can rebuild the whole situation from disk
without relying on any conversation memory:

  lanes      : live codeman sessions (name, status, idle seconds, mode/model)
  missions   : ops/missions/<lane>*.txt -> last END marker found in the lane's
               evidence file (DONE/BLOCKED/none)
  releases   : PR state per repo for the candidate set in candidates.json
  health     : MemAvailable, /tmp usage, watcher daemon, board currency

Usage: coord-state.py [--json] [--quiet]
Costs: one `codeman agent list`, a few `gh` reads (cached 120s per repo).
"""
import argparse
import glob
import json
import os
import re
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.environ.get("BH_ROOT_DIR") or os.path.abspath(os.path.join(HERE, "..", "..", ".."))
MISSIONS = os.path.join(ROOT, "ops", "missions")
EVIDENCE = os.path.join(ROOT, "evidence")
OUT_DIR = os.environ.get("BH_COORD_STATE_DIR") or os.path.join(
    EVIDENCE, "board-" + time.strftime("%Y-%m-%d").replace("-", "-"))
OUT_JSON = os.path.join(OUT_DIR, "coordinator-state-latest.json")
CACHE = os.path.join(HERE, ".coord-state-cache.json")
REPOS = ["mcpdev80/baseharbor", "mcpdev80/baseharbor-demo",
         "mcpdev80/baseharbor-console", "mcpdev80/baseharbor-node-connector"]
CANDIDATES = os.path.join(HERE, "candidates.json")


def sh(cmd, timeout=45):
    try:
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
        return r.returncode, r.stdout or "", r.stderr or ""
    except Exception as exc:
        return 128, "", str(exc)


def lanes():
    rc, out, err = sh(["codeman", "agent", "list", "--json"])
    if rc != 0:
        return None, "agent list failed rc=%s (%s)" % (rc, err.strip()[:120])
    try:
        data = json.loads(out or "[]")
    except Exception as exc:
        return None, "agent list malformed: %s" % exc
    now = time.time()
    rows = []
    for s in data:
        dm = s.get("displayModel") or {}
        rows.append({"id": s.get("id"), "name": s.get("name"), "status": s.get("status"),
                     "idle_s": int(now - (s.get("lastActivityAt") or 0) / 1000.0),
                     "mode": s.get("mode"), "model": dm.get("model"), "self": s.get("self")})
    return sorted(rows, key=lambda r: (r.get("name") or "")), None


def end_markers():
    """lane name -> (status, evidence path) using ops/missions + evidence files."""
    found = {}
    for path in glob.glob(os.path.join(MISSIONS, "*.txt")):
        lane = re.match(r"^(f\d+[a-z]?|q\d+[a-z]?|o\d+[a-z]?|cmx?\w*|ctb\d*|rw\w*|rv\w*)",
                        os.path.basename(path)[:-4])
        if not lane:
            continue
        lane = lane.group(1)
        best = found.get(lane)
        stamp = os.path.getmtime(path)
        if best is None or stamp > best[1]:
            found[lane] = (path, stamp)
    out = {}
    for lane, (mfile, stamp) in found.items():
        ev = []
        for cand in glob.glob(os.path.join(EVIDENCE, "board-*", lane + "*.md")):
            ev.append(cand)
        status, evpath = "dispatched", (ev[0] if ev else None)
        for cand in sorted(ev, key=os.path.getmtime, reverse=True):
            try:
                txt = open(cand, encoding="utf-8", errors="replace").read()
            except Exception:
                continue
            ends = re.findall(r"^END ([A-Z0-9_-]+)$", txt, flags=re.MULTILINE)
            if ends:
                last = ends[-1]
                status = "BLOCKED" if "BLOCKED" in last else "DONE"
                evpath = cand
                break
            if evpath is None:
                evpath = cand
        out[lane] = {"mission": mfile, "state": status, "evidence": evpath}
    return out


def pr_states():
    """PR numbers from candidates.json; cached 120s per repo to spare the API."""
    try:
        wanted = json.load(open(CANDIDATES))
    except Exception:
        return {"unknown": "candidates.json missing/unreadable"}
    cache, cache_ok = {}, False
    try:
        c = json.load(open(CACHE))
        if time.time() - c.get("at", 0) < 120:
            cache, cache_ok = c.get("repos", {}), True
    except Exception:
        pass
    result, notes = {}, []
    for repo in REPOS:
        if cache_ok and repo in cache:
            result[repo] = cache[repo]
            continue
        numbers = [n for n, r in wanted.items() if r == repo]
        rows, bad = {}, False
        for n in numbers:
            rc, out, err = sh(["gh", "pr", "view", str(n), "--repo", repo, "--json",
                               "state,headRefOid,mergeStateStatus",
                               "--jq", '[.state, .headRefOid[0:9], .mergeStateStatus] | @tsv'])
            if rc != 0:
                bad = True
                notes.append("%s#%s gh read FAILED (%s) - UNKNOWN, not 'closed'" %
                             (repo, n, err.strip()[:80]))
                continue
            parts = out.strip().split("\t")
            rows[str(n)] = {"state": parts[0] if parts else "?",
                            "head": parts[1] if len(parts) > 1 else "?",
                            "merge": parts[2] if len(parts) > 2 else "?"}
        result[repo] = {"prs": rows, "observed_at": int(time.time()),
                        "currency": "unknown" if bad else "ok"}
    try:
        os.makedirs(HERE, exist_ok=True)
        json.dump({"at": time.time(), "repos": result}, open(CACHE, "w"))
    except Exception:
        pass
    if notes:
        result["_notes"] = notes
    return result


def health():
    h = {}
    try:
        mem = {}
        for line in open("/proc/meminfo"):
            p = line.split()
            if p:
                mem[p[0].rstrip(":")] = int(p[1])
        h["mem_available_mb"] = mem.get("MemAvailable", 0) // 1024
    except Exception:
        h["mem_available_mb"] = None
    try:
        st = os.statvfs("/tmp")
        h["tmp_free_mb"] = st.f_bavail * st.f_frsize // 1048576
    except Exception:
        h["tmp_free_mb"] = None
    h["tmpdir"] = os.environ.get("TMPDIR", "/tmp")
    h["tmpdir_is_tmpfs"] = h["tmpdir"].startswith("/tmp")
    try:
        w = json.load(open(os.path.join(HERE, "watcher-state.json")))["watcher"]
        h["watcher"] = {"coordinator": w.get("coordinator"), "pid": w.get("pid"),
                        "alive": os.path.exists("/proc/%s" % w.get("pid"))}
    except Exception as exc:
        h["watcher"] = {"unknown": str(exc)[:120]}
    rc, out, _ = sh([sys.executable, os.path.join(HERE, "basics-check.py"), "--board-json"])
    h["board_currency"] = "unknown"
    if rc in (0, 9):
        try:
            h["board_currency"] = json.loads(out).get("currency", "unknown")
        except Exception:
            pass
    return h


def resume_brief(snap):
    """What a fresh coordinator session must do FIRST, derived only from disk."""
    lines = ["COORDINATOR RESUME BRIEF (aus Platte, ohne Session-Gedaechtnis)"]
    running, settled = [], []
    for r in (snap.get("lanes") or []):
        if isinstance(r, dict) is False:
            continue
        tag = (r.get("name") or "?")
        if r.get("self"):
            lines.append("  eigene Session: %s (%s)" % (tag, r.get("status")))
            continue
        if r.get("status") == "busy":
            running.append(tag)
        else:
            m = (snap.get("missions") or {}).get(tag.split("-")[0], {})
            settled.append("%s [%s %s]" % (tag, m.get("state", "?"),
                                           os.path.basename(m.get("evidence") or "-")))
    lines.append("  Laeuft (nicht stoeren, nur beobachten): " + (", ".join(running) or "keine"))
    if settled:
        lines.append("  Idle/fertig -> Evidence pruefen, Ergebnis an Review/Merge-Lane weiterreichen"
                     " oder Lane rotieren: " + "; ".join(settled))
    todo = []
    for repo, v in (snap.get("candidates") or {}).items():
        if repo.startswith("_"):
            continue
        for n, r in sorted((v.get("prs") or {}).items(), key=lambda x: int(x[0])):
            if r.get("state") != "OPEN":
                continue
            if r.get("merge") in ("CLEAN", "BLOCKED", "BEHIND"):
                todo.append("%s#%s %s %s" % (repo.split("/")[-1], n, r.get("head"), r.get("merge")))
    lines.append("  Kandidaten offen (Merge/Rebase/Review noetig): " + ("; ".join(todo) or "keine"))
    plan = os.path.join(ROOT, "evidence", "board-2026-10-22", "coordinator-plan.md")
    try:
        open_steps = [l.strip() for l in open(plan, encoding="utf-8", errors="replace")
                      if l.strip().startswith("- [ ]")]
        lines.append("  Offene Schritte im coordinator-plan.md (%d), neueste zuerst:" % len(open_steps))
        for s in open_steps[-8:]:
            lines.append("    " + s[:150])
    except Exception as exc:
        lines.append("  coordinator-plan.md NICHT lesbar (%s) - Stand UNBEKANNT" % exc)
    hh = snap.get("health") or {}
    lines.append("  Gesundheit: mem=%sMB tmp_free=%sMB watcher=%s board=%s" % (
        hh.get("mem_available_mb"), hh.get("tmp_free_mb"),
        (hh.get("watcher") or {}).get("alive"), hh.get("board_currency")))
    lines.append("  naechster Schritt der Daemon-Regel: basics-check.py + events-check.py --ack")
    return "\n".join(lines)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--quiet", action="store_true")
    ap.add_argument("--resume", action="store_true",
                    help="print the take-over checklist instead of the table")
    a = ap.parse_args()
    lane_rows, lane_err = lanes()
    snap = {"at": int(time.time()), "root": ROOT,
            "lanes": lane_rows if lane_rows is not None else {"unknown": lane_err},
            "missions": end_markers(),
            "candidates": pr_states(),
            "health": health()}
    try:
        os.makedirs(OUT_DIR, exist_ok=True)
        tmp = OUT_JSON + ".tmp"
        with open(tmp, "w") as fh:
            json.dump(snap, fh, indent=1, sort_keys=True)
        os.replace(tmp, OUT_JSON)
        snap["written"] = OUT_JSON
    except Exception as exc:
        snap["write_error"] = str(exc)[:160]
    if a.json:
        print(json.dumps(snap))
    elif a.resume:
        print(resume_brief(snap))
    elif not a.quiet:
        print("coord-state -> %s" % snap.get("written", snap.get("write_error")))
        for r in (lane_rows or []):
            m = snap["missions"].get((r.get("name") or "").split("-")[0], {})
            print("  %-16s %-5s idle=%-5s %s %s" % (r.get("name"), r.get("status"),
                                                    r.get("idle_s"), m.get("state", "?"),
                                                    os.path.basename(m.get("evidence") or "")))
        hh = snap["health"]
        print("  health: mem=%sMB tmp_free=%sMB watcher=%s board=%s" % (
            hh.get("mem_available_mb"), hh.get("tmp_free_mb"),
            hh.get("watcher", {}).get("alive"), hh.get("board_currency")))
    return 0


if __name__ == "__main__":
    sys.exit(main())
