#!/usr/bin/env python3
"""Read-only observer for codeman worker sessions.

Polls `codeman agent list --json` every ~5s and exits as soon as at least one
of the given session ids/names reaches a terminal-ish state (idle, exit,
blocked) or the session is gone. This helper never starts, sends, interrupts
or removes any session.
"""
import argparse
import glob
import hashlib
import json
import os
import re
import signal
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import observed

TERMINAL_STATES = {"idle", "exit", "blocked"}


def _list_sessions():
    try:
        out = subprocess.run(
            ["codeman", "agent", "list", "--json"],
            capture_output=True,
            text=True,
            timeout=30,
        )
    except (subprocess.SubprocessError, FileNotFoundError) as exc:
        sys.stderr.write("wait-workers: could not run codeman: {}\n".format(exc))
        return None
    if out.returncode != 0:
        sys.stderr.write(
            "wait-workers: codeman exited with {}\n".format(out.returncode)
        )
        return None
    try:
        return json.loads(out.stdout)
    except json.JSONDecodeError:
        sys.stderr.write("wait-workers: could not parse codeman output\n")
        return None


def _status_for(session, lookup):
    target = lookup.get("id") or lookup.get("name")
    if target is None:
        return "unknown"
    for entry in session:
        if entry.get("id") == target or entry.get("name") == target:
            return entry.get("status") or "unknown"
    return "gone"


def _exit_code_for(session, lookup):
    target = lookup.get("id") or lookup.get("name")
    if target is None:
        return None
    for entry in session:
        if entry.get("id") == target or entry.get("name") == target:
            code = entry.get("exitCode")
            return "" if code is None else str(code)
    return ""


def print_snapshot(ids):
    ok, session = observed.list_sessions()
    if not ok:
        print("STATUS UNKNOWN - codeman observation failed (not gone, not idle)")
        return 1
    for raw in ids:
        entry = observed.session_by_id_or_name(session, raw)
        if entry is None:
            print("STATUS {} gone ".format(raw))
            continue
        status = entry.get("status") or "unknown"
        code = entry.get("exitCode")
        code = "" if code is None else str(code)
        pe = observed.pane_exit(entry)
        extra = " PANE-EXIT={}".format(pe) if pe is not None else ""
        print("STATUS {} {} {}{}".format(raw, status, code, extra))
    return 0


def find_first_event(session, ids):
    for raw in ids:
        for entry in session:
            if entry.get("id") == raw or entry.get("name") == raw:
                status = entry.get("status") or "unknown"
                code = entry.get("exitCode")
                code = "" if code is None else str(code)
                return raw, status, code
    return (ids[-1] if ids else ""), "gone", ""


def wait(ids, poll_interval=5):
    """First API idle is a CANDIDATE, never terminal (audit finding 1).

    Idle fires only after IDLE_CONFIRM_POLLS consecutive observations with no
    tool/generation/pending-dialog activity. Exited panes are reported
    separately as 'exit'. Observation failures are UNKNOWN: alert and keep
    monitoring; never iterate a failed result or treat loss as gone/idle.
    """
    streak = {}
    alerted = False
    while True:
        ok, session = observed.list_sessions()
        if not ok:
            if not alerted:
                sys.stderr.write(
                    "wait-workers: observation UNKNOWN (codeman list failed) - "
                    "retrying, monitoring continues\n")
                alerted = True
            time.sleep(poll_interval)
            continue
        alerted = False
        for raw in ids:
            entry = observed.session_by_id_or_name(session, raw)
            if entry is None:
                return raw, "gone", ""
            sid = entry.get("id") or raw
            pane_ok, pane_text = observed.pane_read(sid)
            state_word, streak[sid] = observed.classify_lane(
                entry, pane_ok, pane_text, streak.get(sid, 0))
            code = entry.get("exitCode")
            code = "" if code is None else str(code)
            if state_word == "idle-confirmed":
                return raw, "idle", code
            if state_word == "exited":
                return raw, "exit", code
            if state_word == "blocked":
                return raw, "blocked", code
        time.sleep(poll_interval)


WATCHER_STATE_KEYS = ("registered", "delivered", "last_status", "watcher", "busy_since", "permission_flag")


def _state_path():
    override = os.environ.get("WATCHER_STATE")
    if override:
        return override
    base = os.path.dirname(os.path.abspath(__file__))
    return os.path.join(base, "watcher-state.json")


def _load_state(path):
    try:
        with open(path, "r", encoding="utf-8") as handle:
            data = json.load(handle)
    except (OSError, json.JSONDecodeError):
        data = {}
    data.setdefault("registered", {})
    data.setdefault("delivered", {})
    data.setdefault("last_status", {})
    data.setdefault("busy_since", {})
    data.setdefault("permission_flag", {})
    data.setdefault("watcher", None)
    if not isinstance(data["registered"], dict):
        data["registered"] = {}
    if not isinstance(data["delivered"], dict):
        data["delivered"] = {}
    if not isinstance(data["last_status"], dict):
        data["last_status"] = {}
    return data


def _save_state(path, data):
    tmp = "{}.tmp".format(path)
    with open(tmp, "w", encoding="utf-8") as handle:
        json.dump(data, handle, indent=2, sort_keys=True)
        handle.write("\n")
    os.replace(tmp, path)


def _write_pidfile(path, pid):
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(str(pid))


def _read_pidfile(path):
    try:
        with open(path, "r", encoding="utf-8") as handle:
            return handle.read().strip()
    except OSError:
        return None


def _watcher_pidfile():
    return _state_path() + ".pid"


def _current_status(session, coordinator_id, worker_id):
    for entry in session:
        if entry.get("id") == worker_id:
            return entry.get("status") or "unknown"
    return "gone"


def _event_log(message, path=None):
    try:
        path = path or os.path.join(os.path.dirname(os.path.abspath(__file__)), "events.log")
        import log_queue
        log_queue.append(path, "{} {}\n".format(int(time.time()), message))
    except Exception:
        pass


def _send_wake(coordinator_id, message, dry_run, self_id=None, persist=True):
    # persist=True: genuine watcher events land in the acked events.log queue.
    # persist=False: periodic watchdog reminders go ONLY to reminders.log, so the
    # reminder stream cannot self-feed the un-acked-events counter (finding 8).
    if persist:
        _event_log(message)
    if dry_run:
        sys.stdout.write("DRY-RUN send -> {}: {}\n".format(coordinator_id, message))
        return True
    if self_id and coordinator_id == self_id:
        # The coordinator IS the reading session: codeman refuses self-send
        # (rc=4 "is me"), which used to turn every wake into a permanent retry
        # and every reminder into "[send-failed, will retry]". The durable log
        # written by the caller is the delivery channel in that case; the
        # coordinator picks it up through events-check.py at turn start.
        return True
    cmd = ["codeman", "agent", "send", coordinator_id, message]
    env = os.environ.copy()
    for pane_var in ("TMUX", "TMUX_PANE"):
        env.pop(pane_var, None)
    if self_id:
        env["CODEMAN_SESSION_ID"] = self_id
    try:
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=60, env=env
        )
    except subprocess.SubprocessError as exc:
        sys.stderr.write("watcher: send failed: {}\n".format(exc))
        return False
    if result.returncode != 0:
        if "is me" in (result.stdout or "") + (result.stderr or ""):
            # codeman self-send refusal on any other call site: durable log is
            # the delivery channel, so this counts as delivered (no retry storm).
            return True
        sys.stderr.write(
            "watcher: send to {} exited {} (retry next poll)\n".format(
                coordinator_id, result.returncode
            )
        )
        return False
    return True


def _notify(state, coordinator_id, message, dry_run, self_id=None):
    """Send now; on failure keep as durable pending notification (finding 3):
    the message already lives in events.log via persist=True, and the retry
    queue re-sends it until acknowledged by a successful delivery."""
    if _send_wake(coordinator_id, message, dry_run, self_id=self_id):
        return True
    queue = state.setdefault("pending_notify", [])
    if message not in queue:
        queue.append(message)
    return False


def _run_watch_loop(state, state_path, coordinator_id, poll_interval, dry_run):
    sys.stdout.write(
        "watcher: loop running for coordinator {} (dry_run={})\n".format(
            coordinator_id, dry_run
        )
    )
    while True:
        if not _owns_pidfile():
            sys.stderr.write(
                "watcher: own pid {} not in pidfile {}, aborting\n".format(
                    os.getpid(), _watcher_pidfile()
                )
            )
            return 1
        fresh = _load_state(state_path)
        if state.get("watcher") and state["watcher"].get("pid") == os.getpid():
            fresh["watcher"] = state["watcher"]
        state = fresh
        dirty = False
        obs_ok, session = observed.list_sessions()
        wall_now, mono_now = time.time(), _mono_now()
        wref = state["watcher"] if state.get("watcher") else {}
        last_wall = wref.get("last_wall", wall_now)
        last_mono = wref.get("last_mono", mono_now)
        if abs((wall_now - last_wall) - (mono_now - last_mono)) > 120:
            wref["last_heartbeat"] = mono_now
            message = (
                "WATCHER EVENT: CLOCK JUMP {:.0f}s (VM suspend/resume) - "
                "refetch remote refs and reconcile the board before ANY action".format(
                    (wall_now - last_wall) - (mono_now - last_mono)
                )
            )
            if _send_wake(coordinator_id, message, dry_run, self_id=coordinator_id):
                sys.stdout.write("watcher: clock-jump alert delivered\n")
        elif mono_now - wref.get("last_heartbeat", mono_now) >= 3600:
            wref["last_heartbeat"] = mono_now
            _send_wake(
                coordinator_id,
                "WATCHER EVENT: HEARTBEAT (60min) - verify remote reality "
                "(fetch develop+plan, gh pr states) and the board; if any lane "
                "is busy, capture its pane tail for dialog stalls.",
                dry_run, self_id=coordinator_id,
            )
            sys.stdout.write("watcher: heartbeat delivered\n")
        wref["last_wall"], wref["last_mono"] = wall_now, mono_now
        state["watcher"] = wref
        dirty = True
        # pending_notify retry limited to 1x per message. After 1 failed
        # retry, drop the message and let the next real event carry the
        # same content (or silently advance). No poll-spam from indefinite
        # retransmission when coordinator persistently returns busy.
        queue = state.setdefault("pending_notify", [])
        retry_count = state.get("pending_retry_count", {})
        for msg in list(queue):
            if _send_wake(coordinator_id, msg, dry_run, self_id=coordinator_id, persist=False):
                queue.remove(msg)
                retry_count.pop(msg, None)
                dirty = True
            else:
                # First retry failed: drop this message, no more retries.
                retry_count[msg] = retry_count.get(msg, 0) + 1
                if retry_count[msg] > 1:
                    queue.remove(msg)
                    dirty = True
        state["pending_retry_count"] = retry_count
        if not obs_ok:
            # Finding 1: transient API loss must NOT stop monitoring or be
            # interpreted as gone/idle; alert (deduped) and retry next poll.
            _reminder(state, "observe:api",
                      "API observation UNKNOWN (codeman list failed) - no state "
                      "transitions this poll; monitoring continues, retry next poll",
                      mono_now, coordinator_id, dry_run)
        else:
            dirty = _observe_pass(state, session, coordinator_id,
                                  dry_run, mono_now) or dirty
        _watchdog_layer(state, coordinator_id, dry_run, mono_now)
        dirty = True
        if dirty:
            _save_state(state_path, state)
        time.sleep(poll_interval)


def _discover(state, session, coordinator_id, dry_run, mono_now):
    """OPS-1 auto-discovery: diff the live list against registered; any live id
    not present is ADDED to observation immediately (same role=worker,
    coordinator=coordinator) and a DISCOVERED event is emitted. The worker is
    registered BEFORE the loop below so it is observed in this very pass with
    the identical classifier as registered lanes."""
    live_ids = set()
    for s in session:
        if s.get("id"):
            live_ids.add(s["id"])
    discovered = []
    for live_id in sorted(live_ids):
        if live_id in state["registered"]:
            continue
        state["registered"][live_id] = {
            "added_at": _now_ms(),
            "role": "worker",
            "coordinator": coordinator_id,
            "discovered": True,
        }
        dirty_msg = "WATCHER EVENT: DISCOVERED {} (running, not registered - " \
                    "auto-adding to observation)".format(live_id)
        _notify(state, coordinator_id, dirty_msg, dry_run, self_id=live_id)
        discovered.append(live_id)
    if discovered:
        sys.stdout.write(
            "wait-workers: auto-discovered {} lane(s): {}\n".format(
                len(discovered), ", ".join(discovered)))
    return discovered


def _observe_pass(state, session, coordinator_id, dry_run, mono_now):
    """One full observation pass (findings 1+3).

    Dialog scanning INCLUDES the coordinator; worker accounting excludes it.
    Idle events require classifier confirmation; UNKNOWN observations never
    produce RESOLVED/gone/idle conclusions. OPS-1 auto-discovery runs FIRST so
    a running-but-unregistered lane joins observation in this same pass."""
    dirty = False
    names = state.setdefault("names", {})
    for s in session:
        if s.get("id"):
            names[s["id"]] = s.get("name") or ""
    _discover(state, session, coordinator_id, dry_run, mono_now)
    entries = {s.get("id"): s for s in session}
    for worker_id in sorted(state["registered"]):
        # Finding 3: coordinator panes are scanned for dialogs too;
        # only worker accounting (busy alerts) excludes the coordinator.
        is_co = state["registered"][worker_id].get("role") == "coordinator"
        entry = entries.get(worker_id)
        pane = "codeman-{}".format(worker_id[:8])
        pane_ok, pane_text = observed.pane_read(worker_id)
        flags = state.setdefault("permission_flag", {})
        akey, ckey = "d-active:" + worker_id, "d-clear:" + worker_id
        if pane_ok:
            dialog = observed.detect_dialog(pane_text)
            if dialog:
                flags[akey] = list(dialog["markers"])
                flags.pop(ckey, None)
                last = flags.get("notify:" + worker_id)
                if last is None or mono_now - last > 120:
                    flags["notify:" + worker_id] = mono_now
                    key = observed.dialog_key_suggestion(dialog)
                    if key:
                        advice = ("inspect pane {} tail, send-keys -t {} {} ONLY if the "
                                  "rendered command/scope is legitimate (advisory, not "
                                  "granted)".format(pane, pane, key))
                    else:
                        advice = ("inspect pane {} tail; no key derivable from the rendered "
                                  "options - human decision required, no automatic "
                                  "approval".format(pane))
                    _notify(state, coordinator_id,
                            "WATCHER EVENT: {} DIALOG now ({}) - {}".format(
                                worker_id[:8], "/".join(dialog["markers"]), advice),
                            dry_run, self_id=worker_id)
                dirty = True
            elif flags.get(akey):
                clears = flags.get(ckey, 0) + 1
                flags[ckey] = clears
                dirty = True
                if clears >= 3:
                    del flags[akey]
                    flags.pop(ckey, None)
                    runnable = (entry is not None and (entry.get("status") or "") == "idle"
                                and state.get("idle_streak", {}).get(worker_id, 0)
                                >= observed.IDLE_CONFIRM_POLLS)
                    _notify(state, coordinator_id,
                            "WATCHER EVENT: dialog no longer observed in pane {} "
                            "(3 successful clean polls){}".format(
                                pane, " - and lane confirmed idle/runnable" if runnable
                                else " - runnable status NOT yet confirmed"),
                            dry_run, self_id=worker_id)
            state["dialog_pending"] = any(k.startswith("d-active:") for k in flags)
        else:
            # Finding 3 stop criterion: UNKNOWN pane observations never
            # advance the clear counter, never emit RESOLVED/resume advice.
            _reminder(state, "observe:pane:" + worker_id[:8],
                      "PANE observation UNKNOWN for {} - dialog state unverified, "
                      "flag retained, no RESOLVED emitted".format(pane),
                      mono_now, coordinator_id, dry_run)
        streaks = state.setdefault("idle_streak", {})
        st_word, streaks[worker_id] = observed.classify_lane(
            entry, pane_ok, pane_text, streaks.get(worker_id, 0))
        state.setdefault("lane_state", {})[worker_id] = st_word
        status = _current_status(session, coordinator_id, worker_id)
        _compact_watch(state, worker_id, status)
        previous = state["last_status"].get(worker_id)
        if st_word in {"busy", "tool-active", "pending-dialog"}:
            if st_word == "pending-dialog":
                flags[akey] = flags.get(akey) or ["dialog"]
                state["dialog_pending"] = True
            api_status = (entry or {}).get("status") or st_word
            if previous != api_status:
                # Write canonical status word in the busy branch, not raw api_status.
                # This prevents flapping API states from reopening transitions
                # that would trigger duplicate event emissions.
                state["last_status"][worker_id] = st_word
                dirty = True
            busy = state.setdefault("busy_since", {})
            bentry = busy.get(worker_id)
            if bentry is None:
                busy[worker_id] = {"since": mono_now, "alerted": False}
                dirty = True
            else:
                if (not is_co and not bentry.get("alerted")
                        and mono_now - bentry["since"] > 2400):
                    bentry["alerted"] = True
                    _notify(state, coordinator_id,
                            "WATCHER EVENT: {} busy>40min (check pane for "
                            "unanswered permission dialog)".format(worker_id),
                            dry_run, self_id=worker_id)
                dirty = True
            continue
        if st_word == "idle-candidate":
            # single idle sample: not terminal, no re-task/rotate advice
            continue
        if st_word == "unknown":
            _reminder(state, "observe:lane:" + worker_id[:8],
                      "lane {} observation UNKNOWN (API idle, pane unverifiable) - "
                      "no idle/re-task/rotate conclusion drawn".format(worker_id[:8]),
                      mono_now, coordinator_id, dry_run)
            continue
        norm = {"idle-confirmed": "idle", "exited": "exit",
                "blocked": "blocked", "gone": "gone"}[st_word]
        if worker_id in state.get("busy_since", {}):
            del state["busy_since"][worker_id]
            dirty = True
        if previous == norm:
            continue
        message = "WATCHER EVENT: {} {}".format(worker_id, norm)
        if norm == "idle" and previous == "busy":
            # Reuse the captured snapshot (finding 1): no second lookup.
            nm = names.get(worker_id) or ""
            if nm and not nm.startswith(("w1", "codex")) and _lane_evidence(nm) is None:
                message += " [GHOST-CANDIDATE: no evidence file for " + nm + "]"
        if st_word == "exited":
            message += " (paneExit observed - NOT proof of completion; verify evidence)"
        # Advance last_status BEFORE _notify so dedup works regardless of
        # delivery success. The event is appended to events.log via persist=
        # True inside _notify/_send_wake; if delivery to coordinator fails
        # (busy -> exit 4), last_status is already set so the next poll
        # will see previous == norm and skip re-emission entirely.
        state["last_status"][worker_id] = norm
        dirty = True
        ok = _notify(state, coordinator_id, message, dry_run, self_id=worker_id)
        if ok:
            state["delivered"]["{}:{}".format(worker_id, norm)] = {
                "coordinator": coordinator_id,
                "at": _now_ms(),
                "status": norm,
            }
            dirty = True
            sys.stdout.write("watcher: delivered {} {}\n".format(worker_id, norm))
        # last_status is now set regardless of delivery success, closing
        # the loop so the dedup gate (if previous == norm: continue) blocks
        # re-emission even when _notify returns False.
        # (if previous == norm: continue) blocks re-emission even when
        # _notify returns False because the coordinator is busy.
    return dirty


def _daemonize():
    if os.fork() <= 0:
        return
    os._exit(0)


def _spawn_background(coordinator_id, poll_interval, dry_run):
    pid = os.fork()
    if pid > 0:
        # Parent side: the real loop will rewrite the pidfile with its own pid
        # once it has sessionized; report the intermediate pid to the caller.
        _write_pidfile(_watcher_pidfile(), pid)
        sys.stdout.write(
            "watcher: background loop started, pid {}\n".format(pid)
        )
        return 0
    if os.fork() > 0:
        os._exit(0)
    os.setsid()
    sys.stdout.flush()
    sys.stderr.flush()
    log_path = os.path.join(os.path.dirname(_state_path()), "watcher.log")
    with open(os.devnull, "r") as stdin_fd, \
         open(log_path, "a") as stdout_fd, \
         open(log_path, "a") as stderr_fd:
        os.dup2(stdin_fd.fileno(), 0)
        os.dup2(stdout_fd.fileno(), 1)
        os.dup2(stderr_fd.fileno(), 2)
    _write_pidfile(_watcher_pidfile(), os.getpid())
    state = _load_state(_state_path())
    state["watcher"] = {
        "coordinator": coordinator_id,
        "pid": os.getpid(),
        "started_at": _now_ms(),
        "dry_run": dry_run,
    }
    _save_state(_state_path(), state)
    _run_watch_loop(state, _state_path(), coordinator_id, poll_interval, dry_run)
    return 0


def _is_running(state):
    watcher = state.get("watcher")
    if not watcher or not watcher.get("pid"):
        return False
    return _signal_present(int(watcher["pid"]))


def _owns_pidfile():
    path = _watcher_pidfile()
    raw = _read_pidfile(path)
    if not raw:
        return False
    try:
        return _signal_present(int(raw)) and int(raw) == os.getpid()
    except ValueError:
        return False


def _signal_present(pid):
    try:
        os.kill(pid, 0)
    except OSError:
        return False
    return True


def _kill_dryrun_leak():
    import re
    import time as _time

    pattern = re.compile(r"coord-\S.*--dry-run")
    killed = []
    for entry in os.listdir("/proc"):
        if not entry.isdigit():
            continue
        try:
            with open("/proc/{}/cmdline".format(entry), "rb") as handle:
                cmdline = handle.read().replace(b"\x00", b" ").decode(
                    "utf-8", "replace"
                )
        except OSError:
            continue
        if not pattern.search(cmdline):
            continue
        try:
            pid = int(entry)
            if pid == os.getpid():
                continue
        except ValueError:
            continue
        if not _signal_present(pid):
            continue
        try:
            os.kill(pid, 15)
        except OSError:
            continue
        killed.append(pid)
    for pid in killed:
        for _ in range(50):
            if not _signal_present(pid):
                break
            _time.sleep(0.1)
    if killed:
        sys.stderr.write(
            "watcher: cleared leftover dry-run watcher(s) {}\n".format(
                sorted(killed)
            )
        )
    return killed


def cmd_watch_start(coordinator_id, poll_interval, dry_run):
    _kill_dryrun_leak()
    state = _load_state(_state_path())
    if _is_running(state):
        sys.stderr.write(
            "watcher: already running as pid {}\n".format(state["watcher"]["pid"])
        )
        return 1
    state["registered"][coordinator_id] = {
        "added_at": _now_ms(),
        "role": "coordinator",
    }
    state["watcher"] = {
        "coordinator": coordinator_id,
        "started_at": _now_ms(),
        "dry_run": dry_run,
    }
    _save_state(_state_path(), state)
    _spawn_background(coordinator_id, poll_interval, dry_run)
    return 0


def cmd_watch_add(worker_id, coordinator_id):
    state = _load_state(_state_path())
    if not _is_running(state):
        sys.stderr.write("watcher: not running, start first\n")
        return 1
    state["registered"][worker_id] = {
        "added_at": _now_ms(),
        "coordinator": coordinator_id,
    }
    _save_state(_state_path(), state)
    sys.stdout.write("watcher: registered {}\n".format(worker_id))
    return 0


def cmd_watch_remove(worker_id):
    state = _load_state(_state_path())
    if worker_id not in state["registered"]:
        sys.stderr.write("watcher: {} not registered\n".format(worker_id))
        return 1
    del state["registered"][worker_id]
    state["delivered"] = {
        key: value
        for key, value in state["delivered"].items()
        if not key.startswith(worker_id + ":")
    }
    _save_state(_state_path(), state)
    sys.stdout.write("watcher: removed {}\n".format(worker_id))
    return 0


def cmd_watch_list():
    state = _load_state(_state_path())
    running = _is_running(state)
    watcher = state.get("watcher") or {}
    sys.stdout.write(
        "watcher running={} pid={}\n".format(
            running,
            watcher.get("pid", ""),
        )
    )
    for worker_id in sorted(state["registered"]):
        info = state["registered"][worker_id]
        sys.stdout.write(
            "REGISTERED {} role={} coordinator={}\n".format(
                worker_id,
                info.get("role", "worker"),
                info.get("coordinator", ""),
            )
        )
    return 0


def cmd_watch_stop():
    state = _load_state(_state_path())
    watcher = state.get("watcher")
    if not watcher or not watcher.get("pid"):
        sys.stdout.write("watcher: nothing to stop\n")
        return 0
    pid = int(watcher["pid"])
    stopped = False
    for signal_number in (15, 9):
        try:
            os.kill(pid, signal_number)
            stopped = True
        except OSError:
            break
        if not _signal_present(pid):
            break
        time.sleep(1)
    if not stopped or _signal_present(pid):
        sys.stderr.write("watcher: failed to stop pid {}\n".format(pid))
        return 1
    state["watcher"] = None
    _save_state(_state_path(), state)
    try:
        os.remove(_watcher_pidfile())
    except OSError:
        pass
    sys.stdout.write("watcher: stopped pid {}\n".format(pid))
    return 0



def _mono_now():
    try:
        with open("/proc/uptime") as fh:
            return float(fh.read().split()[0])
    except Exception:
        return time.monotonic()


def _now_ms():
    return int(time.time() * 1000)


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="Read-only observer that waits for a codeman worker "
        "session to reach an idle/exit/blocked state or disappear."
    )
    parser.add_argument(
        "--once",
        action="store_true",
        help="Print a single status snapshot of all given sessions and exit.",
    )
    parser.add_argument(
        "sessions",
        nargs="+",
        help="One or more session ids or names to observe.",
    )
    args = parser.parse_args(argv)

    if args.once:
        return print_snapshot(args.sessions)
    target, status, code = wait(args.sessions)
    print("EVENT {} {} {}".format(target, status, code))
    return 0


WATCH_POLL_INTERVAL_DEFAULT = 10


def _build_watch_parser_on(container):
    subparsers = container.add_subparsers(
        dest="watch_command", metavar="{start,add,remove,list,stop}"
    )
    subparsers.required = True

    start = subparsers.add_parser("start", help="Start the background loop.")
    start.add_argument(
        "coordinator",
        help="Coordinator session id receiving WATCHER EVENT messages.",
    )
    start.add_argument(
        "--poll-interval",
        type=int,
        default=WATCH_POLL_INTERVAL_DEFAULT,
        help="Seconds between polls (default: 10).",
    )
    start.add_argument(
        "--dry-run",
        action="store_true",
        help="Log wake-ups instead of sending them.",
    )

    add = subparsers.add_parser("add", help="Register a worker while running.")
    add.add_argument("worker")
    add.add_argument(
        "--coordinator",
        help="Coordinator id that registered this worker.",
    )

    remove = subparsers.add_parser("remove", help="Unregister a worker.")
    remove.add_argument("worker")

    subparsers.add_parser("list", help="Show registration and running state.")

    subparsers.add_parser("stop", help="Stop the background loop.")


def main(argv=None):
    top = argparse.ArgumentParser(
        prog="wait-workers",
        description="Read-only observer plus an optional background WATCHER "
        "mode for codeman worker sessions.",
    )
    subparsers = top.add_subparsers(
        dest="command", metavar="{default,watch}"
    )

    default = subparsers.add_parser(
        "default",
        help="Wait for sessions (default when no subcommand is given).",
    )
    default.add_argument(
        "--once",
        action="store_true",
        help="Print a single status snapshot of all given sessions and exit.",
    )
    default.add_argument(
        "sessions",
        nargs="+",
        help="One or more session ids or names to observe.",
    )

    watch_top = subparsers.add_parser(
        "watch", help="Background WATCHER mode."
    )
    _build_watch_parser_on(watch_top)
    args = top.parse_args(argv)

    # argparse nests the chosen subparser's namespace directly on the shared
    # namespace under its dest (the subparsers action dest == "command"). The
    # "watch" subparser carries its own sub-subcommand in args.watch_command.
    if getattr(args, "watch_command", None):
        return _dispatch_watch(args)

    if getattr(args, "once", False):
        return print_snapshot(args.sessions)
    target, status, code = wait(args.sessions)
    print("EVENT {} {} {}".format(target, status, code))
    return 0


def _dispatch_watch(watch):
    if watch.watch_command == "start":
        return cmd_watch_start(
            watch.coordinator, watch.poll_interval, watch.dry_run
        )
    if watch.watch_command == "add":
        return cmd_watch_add(
            watch.worker, getattr(watch, "coordinator", None)
        )
    if watch.watch_command == "remove":
        return cmd_watch_remove(watch.worker)
    if watch.watch_command == "list":
        return cmd_watch_list()
    if watch.watch_command == "stop":
        return cmd_watch_stop()
    return 0




def _msg_count(con, title):
    try:
        if not title:
            return None
        row = con.execute(
            "select count(*) from message m join session s on m.session_id=s.id "
            "where s.title like ?", ("%" + title[:40] + "%",)).fetchone()
        return row[0] if row else None
    except Exception:
        return None

def _compact_watch(state, worker_id, status):
    """codeman's own auto-compact is a no-op for opencode sessions (token
    counters read 0 there). This watches the real context from the OpenCode
    DB and sends /compact to the pane when a session is idle at threshold."""
    try:
        if status != "idle":
            return
        cw = state.setdefault("compact_watch", {})
        entry = cw.get(worker_id, {})
        if entry.get("next_poll", 0) > time.time():
            return
        entry["next_poll"] = time.time() + 120
        con = _open_ro_db()
        if con is None:
            return
        info = _pane_meta(worker_id)
        thr = int(info.get("threshold") or 200000)
        if worker_id == state.get("watcher", {}).get("coordinator_id"):
            thr = max(thr, 400000)
        ctx = _session_context(con, info.get("title") or "")
        cw[worker_id] = entry
        if entry.get("verify_at", 0) and time.time() >= entry["verify_at"]:
            prior = entry.get("sent_ctx", 0)
            msgs = _msg_count(con, info.get("title") or "")
            grew = msgs and prior and msgs > entry.get("sent_msgs", 0)
            reduced = ctx and prior and ctx < prior
            below = ctx and ctx < thr
            if not (below or (grew and reduced)):
                # no successful compact signal: event unseen AND consumption unrelieved
                if entry.get("tries", 0) < 2:
                    entry["tries"] = entry.get("tries", 0) + 1
                    entry["verify_at"] = 0
                    entry["next_poll"] = 0
                    _reminder(state, "compact:" + worker_id[:8],
                              "COMPACT UNCONFIRMED on {} (ctx {} vs sent {}, msgs {} vs {}, "
                              "compact event unseen) - check pane/queue manually".format(
                                  worker_id[:8], ctx, prior, msgs,
                                  entry.get("sent_msgs")),
                              time.monotonic(),
                              state.get("watcher", {}).get("coordinator_id", ""),
                              False, critical=True)
                else:
                    entry["verify_at"] = 0
            else:
                entry["sent_ctx"] = 0
                entry["sent_msgs"] = 0
                entry["verify_at"] = 0
                entry["tries"] = 0
            cw[worker_id] = entry
        if entry.get("verify_at", 0):
            return
        if ctx and ctx >= thr:
            pane = "codeman-{}".format(worker_id[:8])
            screen = subprocess.run(
                ["tmux", "-L", "codeman", "capture-pane", "-p", "-t", pane],
                capture_output=True, text=True, timeout=5).stdout
            if "esc interrupt" in screen or "Permission" in screen:
                entry["next_poll"] = time.time() + 60
                return
            subprocess.run(["tmux", "-L", "codeman", "send-keys", "-t", pane,
                            "-l", "/compact"], timeout=5)
            subprocess.run(["tmux", "-L", "codeman", "send-keys", "-t", pane,
                            "Enter"], timeout=5)
            entry["sent_ctx"] = ctx
            entry["sent_msgs"] = _msg_count(con, info.get("title") or "")
            entry["verify_at"] = time.time() + 600
            _send_wake(entry.get("coordinator_target", "") or
                       state.get("watcher", {}).get("coordinator_id", ""),
                       "WATCHER EVENT: {} COMPACT sent (ctx {} >= {})".format(
                           worker_id[:8], ctx, thr), False, self_id=worker_id)
        entry["last_ctx"] = ctx
    except Exception:
        pass


def _open_ro_db():
    try:
        import sqlite3
        src = os.path.expanduser("~/.local/share/opencode/opencode.db")
        con = sqlite3.connect("file:" + src + "?mode=ro", uri=True, timeout=5)
        return con
    except Exception:
        try:
            import shutil, sqlite3, tempfile
            src = os.path.expanduser("~/.local/share/opencode/opencode.db")
            dst = os.path.join(tempfile.gettempdir(), "watcher_ro.db")
            shutil.copy(src, dst)
        except Exception:
            return None
        for ext in ("-wal", "-shm"):
            if os.path.exists(src + ext):
                shutil.copy(src + ext, dst + ext)
        return sqlite3.connect("file:{}?mode=ro".format(dst), uri=True)
    except Exception:
        return None


def _pane_meta(worker_id):
    out = {"title": "", "threshold": 200000}
    try:
        res = subprocess.run(["codeman", "agent", "list", "--json"],
               capture_output=True, text=True, timeout=20)
        for s in json.loads(res.stdout):
            if s.get("id") == worker_id:
                out["title"] = s.get("name") or ""
                out["threshold"] = s.get("autoCompactThreshold") or 200000
    except Exception:
        pass
    try:
        res = subprocess.run(["tmux", "-L", "codeman", "display-message", "-p",
                              "-t", "codeman-{}".format(worker_id[:8]),
                              "#{pane_title}"], capture_output=True, text=True, timeout=5)
        if res.stdout.strip():
            out["title"] = res.stdout.strip().split("|")[-1].strip()
    except Exception:
        pass
    return out


def _session_context(con, title):
    if not title:
        return 0
    try:
        row = con.execute(
            "select s.id from session s where s.title like ? "
            "order by s.time_updated desc limit 1",
            ("%" + title[:48] + "%",)).fetchone()
        if not row:
            return 0
        tok = con.execute(
            "select json_extract(json(data),'$.tokens.input')+"
            "coalesce(json_extract(json(data),'$.tokens.output'),0)+"
            "coalesce(json_extract(json(data),'$.tokens.cache.read'),0)+"
            "coalesce(json_extract(json(data),'$.tokens.cache.write'),0) "
            "from message where session_id=? order by time_created desc limit 30",
            (row[0],)).fetchall()
        vals = [int(v[0]) for v in tok if v and v[0]]
        return max(vals) if vals else 0
    except Exception:
        return 0



HERE = os.path.dirname(os.path.abspath(__file__))
ROOT_DIR = os.path.abspath(os.path.join(HERE, "..", "..", ".."))


POOL_LIMITS = {"flash": 1, "ornith": 2, "qwen": 2}
REMINDER_COOLDOWN = 900


def _pane_class(worker_id):
    pane_ok, text = observed.pane_read(worker_id)
    return observed.model_class(pane_ok, text)


def _reminder(state, key, text, mono_now, coordinator_id, dry_run, critical=False):
    """Finding 8: reminders are health notifications, not work events.

    They go ONLY to reminders.log (persist=False), never into the acked
    events.log queue, so a reminder can no longer inflate the un-acked
    counter that triggers it. Unchanged conditions (same signature) respect
    the cooldown even when critical; a CHANGED critical condition fires
    immediately once. Send failures retry with a bounded gap without flooding.
    """
    rem = state.setdefault("reminders", {})
    entry = rem.get(key)
    body = text[:200]
    sig = hashlib.md5(body.encode("utf-8", "replace")).hexdigest()
    if entry and entry.get("sig") == sig and mono_now < entry.get("next", 0):
        return False
    message = "WATCHDOG REMINDER[{}]: {}".format(key, body)
    rlog = os.path.join(HERE, "reminders.log")
    if _send_wake(coordinator_id, message, dry_run, self_id=coordinator_id,
                  persist=False):
        _event_log(message, path=rlog)
        rem[key] = {"next": mono_now + REMINDER_COOLDOWN, "sent": mono_now,
                    "fails": 0, "sig": sig}
        return True
    fails = (entry or {}).get("fails", 0) + 1
    gap = 30 if fails < 3 else REMINDER_COOLDOWN
    rem[key] = {"next": mono_now + gap, "sent": (entry or {}).get("sent", mono_now),
                "fails": fails, "sig": (entry or {}).get("sig") or sig}
    if fails <= 1:
        _event_log(message + " [send-failed, will retry]", path=rlog)
    return False





def _todo_counts_by_time(createdAt_ms):
    con = _open_ro_db()
    if con is None or not createdAt_ms:
        return None
    try:
        row = con.execute(
            "select id from session where abs(time_created - ?) < 30000 "
            "order by abs(time_created - ?) limit 1",
            (int(createdAt_ms), int(createdAt_ms))).fetchone()
        if not row:
            return None
        rows = con.execute(
            "select status, count(*) from todo where session_id=? group by status",
            (row[0],)).fetchall()
        return dict(rows)
    except Exception:
        return None


def _git(plan, args, timeout=30):
    """Checked git call -> (rc, stdout). Never silently zero (finding 4)."""
    try:
        res = subprocess.run(["git", "-C", plan] + args, capture_output=True,
                             text=True, timeout=timeout)
        return res.returncode, (res.stdout or "").strip()
    except Exception as exc:
        return 128, str(exc)


def _board_watcher(state, coordinator_id, dry_run, mono_now):
    """Finding 4: single authoritative currency source with checked returns.

    basics-check.py --board-json performs fetch FIRST, then computes
    ahead/behind/dirty from that snapshot; failed observation is surfaced as
    UNKNOWN currency (dispatch hold), never as a silently clear board.
    """
    try:
        res = subprocess.run([sys.executable, os.environ.get(
            "BASICS_CHECK_PY", os.path.join(HERE, "basics-check.py")),
                              "--board-json"], capture_output=True, text=True,
                             timeout=120)
    except Exception as exc:
        _reminder(state, "board", "BOARD checker FAILED to run ({}) - currency "
                  "UNKNOWN, NOT treated as clear".format(exc), mono_now,
                  coordinator_id, dry_run, critical=True)
        return
    try:
        data = json.loads(res.stdout)
    except Exception:
        _reminder(state, "board", "BOARD checker output unparsable (rc {}) - currency "
                  "UNKNOWN, NOT treated as clear".format(res.returncode), mono_now,
                  coordinator_id, dry_run, critical=True)
        return
    if res.returncode not in (0, 9):
        _reminder(state, "board", "BOARD checker rc {} flags timeout/error - "
                  "currency UNKNOWN".format(res.returncode), mono_now,
                  coordinator_id, dry_run, critical=True)
        return
    for f in data.get("flags", []):
        if f.startswith("BOARD:"):
            _reminder(state, "board", f + " - publish/reconcile planning; "
                      "dispatch-hold until currency verified", mono_now,
                      coordinator_id, dry_run, critical=True)
    _completion_accounting(state, coordinator_id, dry_run, mono_now)


def _end_timestamp(text):
    """Wall time embedded in an END line ('END <TOKEN> @<epoch>') if present."""
    m = re.search(r"END \S+ @(\d{9,11})", text)
    return int(m.group(1)) if m else None


def _evidence_files():
    """END candidates from BOTH supported locations: the workspace evidence
    board-*/ dirs and baseharbor-plan/evidence/** (finding 5)."""
    out = []
    for d in sorted(glob.glob(os.path.join(ROOT_DIR, "evidence", "board-*"))):
        try:
            out += [os.path.join(d, fn) for fn in os.listdir(d) if fn.endswith(".md")]
        except Exception:
            pass
    pev = os.path.join(ROOT_DIR, "baseharbor-plan", "evidence")
    for root, _dirs, files in os.walk(pev):
        out += [os.path.join(root, fn) for fn in files if fn.endswith(".md")]
    return out


def _published_at_planning(basename):
    """True only if the evidence file itself is committed at a path OR a task
    card under tasks/ references it. HANDOFF prose mentions are deliberately
    NOT publication evidence (finding 5: historical name text suppressed the
    alert before). Unreadable repo/refs -> None (UNKNOWN, keep pending)."""
    plan = os.path.join(ROOT_DIR, "baseharbor-plan")
    rc, _ = _git(plan, ["rev-parse", "--verify", "HEAD"], timeout=15)
    if rc != 0:
        return None
    unknown = False
    for ref in ("origin/main", "HEAD"):
        rc, out = _git(plan, ["ls-tree", "-r", "--name-only", ref], timeout=30)
        if rc != 0:
            unknown = True
            continue
        if any(line.strip().endswith("/" + basename) or line.strip() == basename
               for line in (out or "").splitlines()):
            return True
        rc, _o = _git(plan, ["grep", "-l", "-F", basename, ref, "--", "tasks/"],
                      timeout=30)
        if rc == 0:
            return True
        if rc != 1:
            unknown = True
    if unknown:
        return None
    return False


def _completion_accounting(state, coordinator_id, dry_run, mono_now):
    """Finding 5: durable per-completion identity (file + END token) with a
    first-seen timestamp, independent of session presence; grace is measured
    from the END timestamp (or first observation) to NOW, and publication is
    verified against the remote Planning commit, not an mtime or a name
    mention in historical HANDOFF prose."""
    comp = state.setdefault("completions", {})
    now = time.time()
    for f in _evidence_files():
        key = os.path.relpath(f, ROOT_DIR)
        if key in comp:
            continue
        try:
            txt = open(f, encoding="utf-8", errors="replace").read()
        except Exception:
            continue
        tail = txt[-400:]
        if "-DONE" not in tail:
            continue
        end_ts = _end_timestamp(txt) or int(os.path.getmtime(f))
        comp[key] = {"file": f, "end_ts": end_ts, "first_seen": int(now),
                     "published": False}
        dirty = True
    for key, meta in comp.items():
        if meta.get("published"):
            continue
        pub = _published_at_planning(os.path.basename(key))
        if pub is True:
            meta["published"] = True
        elif pub is None:
            _reminder(state, "publish:observe:" + os.path.basename(key)[:20],
                      "Planning publication check UNKNOWN for {} - completion stays "
                      "PENDING, not cleared".format(os.path.basename(key)), mono_now,
                      coordinator_id, dry_run)
    pending = [(k, m) for k, m in comp.items()
               if not m.get("published")
               and now - m.get("end_ts", m.get("first_seen", now)) > 900]
    if pending:
        _reminder(state, "flash:publish",
                  "END delivered but not published at remote Planning >15min: {} - "
                  "publish Planning-Abschluss (per task/lane, verified at commit)".format(
                      [os.path.basename(k) for k, _ in pending[:4]]),
                  mono_now, coordinator_id, dry_run, critical=True)


def _lane_evidence(name):
    """Lane/task evidence in both supported locations and active dates.
    Lane prefixes use a hyphen boundary; explicit Lane metadata handles
    unrelated task filenames without accepting another lane prefix."""
    if not name:
        return None
    fn = name + ".md"
    task_name = re.sub(r"^[a-z]+[0-9]+-", "", name)
    candidates = []
    roots = glob.glob(os.path.join(ROOT_DIR, "evidence", "board-*"))
    roots += [os.path.join(ROOT_DIR, "baseharbor-plan", "evidence")]
    for base in roots:
        for root, dirs, files in os.walk(base):
            dirs[:] = [d for d in dirs if d != "prompts"]
            for filename in files:
                if not filename.endswith(".md"):
                    continue
                p = os.path.join(root, filename)
                stem = filename[:-3]
                matched = filename == fn or stem.startswith(name + "-") or (
                    task_name != name and stem == task_name)
                if not matched:
                    try:
                        with open(p, encoding="utf-8", errors="replace") as fh:
                            header = fh.read(4096)
                        matched = bool(re.search(
                            r"(?im)^\s*(?:##\s*)?Lane:\s*`?" + re.escape(name) + r"`?(?:\s|$|[.(])",
                            header))
                    except OSError:
                        pass
                if matched:
                    candidates.append(p)
    if not candidates:
        return None
    return max(candidates, key=lambda p: os.path.getmtime(p))


def _todo_counts(oc_id, title=""):
    con = _open_ro_db()
    if con is None:
        return None
    try:
        if oc_id and str(oc_id).startswith("ses_"):
            rows = con.execute(
                "select status, count(*) from todo where session_id=? group by status",
                (oc_id,)).fetchall()
        else:
            rows = con.execute(
                "select t.status, count(*) from todo t join session s on t.session_id=s.id "
                "where s.title like ? group by t.status",
                ("%" + (title or "")[:40] + "%",)).fetchall()
        return dict(rows)
    except Exception:
        return None


def _worker_plan_checks(state, live, coordinator_id, mono_now, dry_run):
    for sess in live:
        wid, name, status = sess.get("id", ""), sess.get("name", "") or "", sess.get("status", "")
        if not name or name.startswith("w1") or "codex" in name or wid == coordinator_id:
            continue
        if observed.pane_exit(sess) is not None:
            continue
        if _pane_class(wid) == "unknown":
            continue
        lane_st = state.get("lane_state", {}).get(wid) or status
        idle_confirmed = lane_st in ("idle-confirmed", "idle") or (
            status == "idle" and state.get("idle_streak", {}).get(wid, 0)
            >= observed.IDLE_CONFIRM_POLLS)
        f = _lane_evidence(name)
        if f is None:
            bs = state.get("busy_since", {}).get(wid)
            if status == "busy" and isinstance(bs, dict) and mono_now - bs.get("since", mono_now) > 240:
                _reminder(state, "planfile:" + name, name + " busy>4min with NO evidence/PLAN file",
                          mono_now, coordinator_id, dry_run)
            continue
        try:
            txt = open(f, encoding="utf-8", errors="replace").read()
        except Exception:
            continue
        done = txt.count("- [x]")
        open_ = txt.count("- [ ]")
        ended = "END " in txt and "-DONE" in txt
        age_min = (time.time() - os.path.getmtime(f)) / 60
        if (not ended) and idle_confirmed and os.path.getsize(f) < 3000 \
                and txt.count("- [x]") == 0 and "DONE" not in txt[-300:]:
            _reminder(state, "stub:" + name,
                      "{} evidence is a <3KB stub with zero done steps while idle-confirmed - "
                      "re-dispatch or rotate lane".format(name),
                      mono_now, coordinator_id, dry_run)
        if status == "busy" and age_min > 40 and not ended:
            _reminder(state, "planstale:" + name,
                      "{} busy but PLAN file untouched {:.0f}min (open {} done {})".format(
                          name, age_min, open_, done),
                      mono_now, coordinator_id, dry_run)
        todos = _todo_counts_by_time(sess.get("createdAt") or 0)
        if todos is not None:
            pending = sum(v for k, v in todos.items() if k in ("pending", "in_progress"))
            if not todos and (open_ + done) > 0:
                _reminder(state, "todos:" + name,
                          "{} has PLAN checkboxes but NO native todos (GUI blind)".format(name),
                          mono_now, coordinator_id, dry_run)
            elif todos and pending == 0 and open_ > 0 and not ended:
                _reminder(state, "todos:" + name,
                          "{} SYNC: native todos all completed but {} open checkboxes in {}".format(
                              name, open_, os.path.basename(f)),
                          mono_now, coordinator_id, dry_run)
            elif pending > 0 and done == 0 and open_ == 0 and ended:
                _reminder(state, "todos:" + name,
                          "{}: END marker present but native todos still {} pending - close them".format(
                              name, pending), mono_now, coordinator_id, dry_run)



def _plan_task_state(plan_dir, plan_py, tid):
    """Checked plan.py show -> (rc, state, deps). rc!=0 means UNKNOWN."""
    try:
        res = subprocess.run([sys.executable, plan_py, "show", tid],
                             capture_output=True, text=True, timeout=30,
                             cwd=plan_dir)
    except Exception:
        return 128, "", {}
    state_word = ""
    deps = {}
    for line in (res.stdout or "").splitlines():
        m = re.match(r"state:\s*(\S+)", line)
        if m:
            state_word = m.group(1)
        d = re.match(r"\s*depends on (\S+): (\S+)", line)
        if d:
            deps[d.group(1)] = d.group(2)
    if res.returncode != 0:
        return res.returncode, "", deps
    return 0, state_word, deps


def _owner_stop_active():
    """Latest authoritative handoff/plan docs decide: OWNER REQUESTED STOP with
    no newer explicit resume marker inside the same/newest document binds."""
    paths = []
    for root, _dirs, files in os.walk(os.path.join(ROOT_DIR, "baseharbor-plan",
                                                    "evidence")):
        paths += [os.path.join(root, fn) for fn in files if fn == "HANDOFF.md"]
    paths.append(os.path.join(ROOT_DIR, "evidence", "board-2026-10-22",
                              "coordinator-plan.md"))
    newest = {"ts": 0.0, "stop": False}
    for p in paths:
        try:
            m = os.path.getmtime(p)
            txt = open(p, encoding="utf-8", errors="replace").read()
        except Exception:
            continue
        if m < newest["ts"]:
            continue
        stopped = bool(re.search(r"OWNER REQUESTED STOP", txt)) and not re.search(
            r"OWNER RESUME|STOP LIFTED|WIEDERAUFNAHME FREIGEGEBEN", txt, re.I)
        newest = {"ts": m, "stop": stopped}
    return newest["stop"]


def _known_actionable_work(state, session):
    """Finding 6: EVERY inbox line is parsed with its OWN WIP marker and the
    marker is resolved to an exact known lane, independent of instantaneous
    busy status. Planning readiness is verified through plan.py's real task
    contract with checked return codes; text that merely looks ready is never
    actionable, and observation failures are UNKNOWN (not clear, not work)."""
    tasks = []
    known = [s.get("name") or "" for s in session]
    known += [n for n in state.get("names", {}).values() if n]
    known = [n for n in known if n]
    try:
        with open(os.path.join(ROOT_DIR, "evidence", "board-2026-10-22",
                               "delegation-inbox.txt")) as fh:
            for line in fh:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                tag = line.split(":", 1)[0]
                m = re.search(r"\[wip:([a-z0-9-]+)\]", line, re.I)
                if m:
                    marker = m.group(1).lower()
                    lanes = [n for n in known
                             if n.lower() == marker or n.lower().startswith(marker + "-")]
                    if len(lanes) == 1:
                        tasks.append({"tag": tag, "text": line[:80], "actionable": False,
                                      "owned_by": lanes[0]})
                    else:
                        tasks.append({"tag": tag, "text": line[:80], "actionable": False,
                                      "stale_wip": True})
                else:
                    tasks.append({"tag": tag, "text": line[:80], "actionable": True})
    except Exception as exc:
        tasks.append({"tag": "INBOX", "text": "inbox unreadable ({})".format(exc),
                      "actionable": False, "observe_error": True})
    plan_dir = os.environ.get("BH_PLAN_DIR") or os.path.join(ROOT_DIR, "baseharbor-plan")
    plan_py = os.path.join(plan_dir, "scripts", "plan.py")
    try:
        res = subprocess.run([sys.executable, plan_py, "list", "--ready"],
                             capture_output=True, text=True, timeout=60, cwd=plan_dir)
    except Exception as exc:
        tasks.append({"tag": "PLANLIST", "text": "plan.py failed ({})".format(exc),
                      "actionable": False, "observe_error": True})
        return tasks
    if res.returncode != 0:
        tasks.append({"tag": "PLANLIST", "text": "plan.py list rc={}".format(res.returncode),
                      "actionable": False, "observe_error": True})
        return tasks
    ids = [l.split()[0] for l in (res.stdout or "").splitlines()
           if l.strip() and " ready" in l][:6]
    for tid in ids:
        rc, st, deps = _plan_task_state(plan_dir, plan_py, tid)
        key = "ready-card:" + tid
        if rc != 0:
            tasks.append({"tag": key, "text": "plan.py show rc={} (state UNKNOWN)".format(rc),
                          "actionable": False, "observe_error": True})
            continue
        if st != "ready":
            tasks.append({"tag": key, "text": "derived state '{}' - not actionable".format(st),
                          "actionable": False, "claimed": st in ("claimed", "active", "review")})
            continue
        unmet = sorted(d for d, s in deps.items() if s != "accepted")
        if unmet:
            tasks.append({"tag": key, "text": "unmet dependencies {}".format(unmet[:4]),
                          "actionable": False, "dependency": True})
            continue
        tasks.append({"tag": key, "text": "verified ready per plan contract",
                      "actionable": True})
    return tasks


def _flash_meta_checks(state, live, coordinator_id, mono_now, dry_run, session):
    # Finding 5: END/publication accounting is durable and per-completion via
    # _completion_accounting (called from _board_watcher); file-based identity
    # survives session removal and ignores HANDOFF name mentions.
    for s in session:
        nm2 = s.get("name") or ""
        sid2 = s.get("id") or ""
        if not nm2 or nm2.startswith("w1") or "codex" in nm2:
            continue
        if observed.pane_exit(s) is not None:
            continue
        idle_confirmed = state.get("lane_state", {}).get(sid2) == "idle-confirmed" or (
            (s.get("status") or "") == "idle"
            and state.get("idle_streak", {}).get(sid2, 0) >= observed.IDLE_CONFIRM_POLLS)
        if idle_confirmed:
            f2 = _lane_evidence(nm2)
            try:
                ended = bool(f2) and "-DONE" in open(f2, encoding="utf-8",
                                                     errors="replace").read()[-400:]
            except Exception:
                ended = None
            if ended is False or ended is None:
                _reminder(state, "idlenoend:" + nm2[:6],
                          "{} idle-confirmed with NO END marker{} - verify, re-task or rotate".format(
                              nm2, "" if f2 else " and NO evidence file"),
                          mono_now, coordinator_id, dry_run)
    active_words = {"busy", "working", "tool-active", "pending-dialog"}
    busy_workers = [s for s in session
                    if (state.get("lane_state", {}).get(s.get("id", "")) in active_words
                        or (s.get("status") or "") in ("busy", "working"))
                    and not (s.get("name", "") or "").startswith(("w1",))
                    and "codex" not in (s.get("name", "") or "")
                    and observed.pane_exit(s) is None]
    worker_n = len(busy_workers)
    stop = _owner_stop_active()
    tasks = _known_actionable_work(state, session)
    for t in tasks:
        if t.get("stale_wip"):
            _reminder(state, "wip:stale:" + t["tag"],
                      "{}: WIP marker does not resolve to exactly one known lane - "
                      "reconcile marker; no delegation or re-dispatch from it".format(t["tag"]),
                      mono_now, coordinator_id, dry_run)
        elif t.get("observe_error"):
            _reminder(state, "work:observe:" + t["tag"],
                      "{}: work-readiness observation UNKNOWN ({}) - not treated as "
                      "clear or as work".format(t["tag"], t["text"][:80]),
                      mono_now, coordinator_id, dry_run)
    actionable = sorted({t["tag"] for t in tasks if t.get("actionable")})
    if stop:
        _reminder(state, "owner-stop",
                  "OWNER STOP active per latest authoritative handoff - "
                  "delegate/merge suggestions suppressed", mono_now, coordinator_id, dry_run)
    elif session and actionable and worker_n < 5:
        _reminder(state, "flash:delegate",
                  "delegate candidate {} - verified unclaimed ready, free capacity {}/5; "
                  "claim + disjoint write-set + authorization MUST be verified per plan "
                  "contract BEFORE dispatch".format(actionable[:3], 5 - worker_n),
                  mono_now, coordinator_id, dry_run)
    wref = state.setdefault("watcher", {})
    if not stop and mono_now - wref.get("last_mergepr", 0) > 300:
        wref["last_mergepr"] = mono_now
        try:
            res = subprocess.run([os.environ.get("GH_BIN", "gh"), "-R",
                                  "mcpdev80/baseharbor", "pr", "list",
                                  "--state", "open", "--json", "number,createdAt,mergeable"],
                                 capture_output=True, text=True, timeout=30)
            if res.returncode != 0:
                raise RuntimeError("gh rc={}".format(res.returncode))
            for pr in json.loads(res.stdout or "[]"):
                if pr.get("mergeable") == "MERGEABLE" and \
                        time.time() - __import__("datetime").datetime.strptime(
                            pr["createdAt"][:19], "%Y-%m-%dT%H:%M:%S").timestamp() > 1200:
                    _reminder(state, "flash:review#%d" % pr["number"],
                              "PR #{} MERGEABLE >20min - review/merge trigger pending".format(
                                  pr["number"]), mono_now, coordinator_id, dry_run)
        except Exception as exc:
            _reminder(state, "pr:observe", "PR observation UNKNOWN ({}) - no review/merge "
                      "advice drawn".format(exc), mono_now, coordinator_id, dry_run)


def _watchdog_layer(state, coordinator_id, dry_run, mono_now):
    wref = state.get("watcher", {})
    if mono_now - wref.get("last_watchdog", 0) < 60:
        return
    wref["last_watchdog"] = mono_now
    ok_live, live = observed.list_sessions()
    if not ok_live:
        # Finding 1: a failed list is UNKNOWN, never "zero sessions".
        _reminder(state, "observe:api",
                  "watchdog: session observation UNKNOWN (list failed) - pool counts "
                  "NOT evaluated, retry next pass", mono_now, coordinator_id, dry_run)
        _board_watcher(state, coordinator_id, dry_run, mono_now)
        return
    counts = {"flash": 0, "ornith": 0, "qwen": 0}
    unknown_model = []
    coordinator_ids = {k for k, v in state.get("registered", {}).items()
                       if v.get("role") == "coordinator"}
    for sess in live:
        worker_id = sess.get("id", "")
        name = sess.get("name", "") or ""
        if worker_id in coordinator_ids or worker_id == coordinator_id \
                or "codex" in name or name.startswith("w1"):
            continue
        if observed.pane_exit(sess) is not None:
            continue
        cls = _pane_class(worker_id)
        if cls == "unknown":
            unknown_model.append(name or worker_id[:8])
            continue
        counts[cls] = counts.get(cls, 0) + 1
    if unknown_model:
        _reminder(state, "pool:observe",
                  "model identity UNKNOWN for {} - excluded from pool counts, not "
                  "inferred from lane-name letters".format(unknown_model[:4]),
                  mono_now, coordinator_id, dry_run)
    over = [k for k, lim in POOL_LIMITS.items() if counts.get(k, 0) > lim]
    if over:
        _reminder(state, "pool:" + ",".join(over),
                  "POOL VIOLATION real-model counts {} vs limits {} - inspect panes, "
                  "claims and running work, correct manually (scripts never fix)".format(
                      counts, POOL_LIMITS),
                  mono_now, coordinator_id, dry_run, critical=True)
    flags = []
    checker_error = None
    try:
        res = subprocess.run(
            [sys.executable, os.environ.get("BASICS_CHECK_PY",
                                           os.path.join(HERE, "basics-check.py")),
             "--json"],
            capture_output=True, text=True, timeout=60)
        if res.returncode != 0:
            checker_error = "rc {}".format(res.returncode)
        else:
            flags = json.loads(res.stdout).get("flags", [])
    except Exception as exc:
        checker_error = str(exc)
    if checker_error is not None:
        # Finding 4: a failed checker is REPORTED, never an apparently clear run.
        _reminder(state, "checker:basics",
                  "basics-check FAILED ({}) - result NOT treated as clear".format(
                      checker_error[:120]), mono_now, coordinator_id, dry_run,
                  critical=True)
    for f in flags:
        if f.startswith("POOL:"):
            continue
        if f.startswith("BOARD:"):
            continue
        elif f.startswith("EVENTS:"):
            _reminder(state, "events", f + " - process queue then events-check --ack",
                      mono_now, coordinator_id, dry_run, critical=True)
        elif f.startswith("DISK:"):
            _reminder(state, "disk", f, mono_now, coordinator_id, dry_run, critical=True)
        elif f.startswith("PLAN:") or f.startswith("IDLE LANES:") or f.startswith("OBS:"):
            _reminder(state, "plan-lanes", f, mono_now, coordinator_id, dry_run)
    _board_watcher(state, coordinator_id, dry_run, mono_now)
    for s in live:
        nm = s.get("name") or ""
        if not nm or nm.startswith(("w1",)) or "codex" in nm:
            continue
        try:
            r = subprocess.run([sys.executable, os.path.join(HERE, "lane-stalled.py"),
                                s.get("id", ""), "0"], capture_output=True, text=True,
                               timeout=40)
        except Exception as exc:
            _reminder(state, "stall:" + nm[:6],
                      "stall probe could not run for {} ({}) - UNKNOWN, no advice".format(
                          nm, exc), mono_now, coordinator_id, dry_run)
            continue
        if r.returncode == 5:
            _reminder(state, "stall:" + nm[:6],
                      "LANE QUEUE-STALLED {} (confirmed stall with healthy observation: "
                      "mission pending, no assistant output) - respawn after pane "
                      "inspection, do not resend".format(nm),
                      mono_now, coordinator_id, dry_run, critical=True)
        elif r.returncode in (4, 6, 7):
            _reminder(state, "stall:" + nm[:6],
                      "stall observation UNKNOWN/unresolved for {} ({}) - NO resend or "
                      "respawn advice until delivery/execution is observed".format(
                          nm, (r.stdout or "").strip()[:60]),
                      mono_now, coordinator_id, dry_run)
    _worker_plan_checks(state, live, coordinator_id, mono_now, dry_run)
    _flash_meta_checks(state, live, coordinator_id, mono_now, dry_run, live)


if __name__ == "__main__":
    sys.exit(main())

