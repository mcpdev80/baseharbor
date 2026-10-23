#!/usr/bin/env python3
"""F17 scriptfix fixtures: mock send/list/DB, temporary bare remotes, observed
pane sequences. Covers the Required-verification rows of codex-script-audit.md
findings 1-8. No live worker is ever controlled. Run: python3 test_f17_fixtures.py
"""
import importlib.util
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import tempfile
import time
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))


def _load(name, fname):
    spec = importlib.util.spec_from_file_location(name, os.path.join(HERE, fname))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


observed = _load("observed_fx", os.path.join(HERE, "observed.py"))
ww = _load("waitworkers_fx", os.path.join(HERE, "wait-workers.py"))


def sh(cmd, env=None, cwd=None):
    return subprocess.run(cmd, capture_output=True, text=True, env=env, cwd=cwd,
                          timeout=120)


def git(repo, *args, env=None):
    e = dict(os.environ)
    e.update({"GIT_AUTHOR_NAME": "f", "GIT_AUTHOR_EMAIL": "f@f",
              "GIT_COMMITTER_NAME": "f", "GIT_COMMITTER_EMAIL": "f@f"})
    if env:
        e.update(env)
    r = sh(["git", "-C", repo] + list(args), env=e)
    assert r.returncode == 0, "git {} failed: {}".format(args, r.stderr)
    return r.stdout


CODEMAN_STUB = '''#!/usr/bin/env bash
if [[ -n "${LIST_FAIL:-}" && "$1" == "agent" && "$2" == "list" ]]; then exit 1; fi
if [[ "$1" == "agent" && "$2" == "list" ]]; then cat "$FAKE_LIST"; exit 0; fi
if [[ "$1" == "agent" && "$2" == "send" ]]; then
  echo "$3" >> "$SENDS"
  if [[ -n "${FAIL_FIRST:-}" ]]; then
    if [[ ! -e "${FAIL_FIRST}" ]]; then touch "${FAIL_FIRST}"; exit 0; fi
    exit 1
  fi
  if [[ "${DELIVER:-yes}" == yes ]]; then
    python3 - <<'PY'
import json, os, sqlite3, time
con = sqlite3.connect(os.environ["FAKE_DB"])
ts = int(time.time() * 1000)
con.execute("insert into message(session_id,time_created,data) values(?,?,?)",
            (os.environ["OC_SID"], ts, json.dumps({"role": "user"})))
if os.environ.get("ANSWERED") == "1":
    con.execute("insert into message(session_id,time_created,data) values(?,?,?)",
                (os.environ["OC_SID"], ts + 1, json.dumps({"role": "assistant"})))
con.commit()
PY
  fi
  exit "${SEND_EXIT:-0}"
fi
if [[ "$1" == "agent" && "$2" == "spawn" ]]; then
  echo spawned >> "$SENDS"
  printf '%s\\n' "${SPAWN_OUT}"
  if [[ -n "${SPAWN_LIST_ENTRY:-}" ]]; then
    python3 - <<'PY'
import json, os
p = os.environ["FAKE_LIST"]
d = json.load(open(p))
d.append(json.loads(os.environ["SPAWN_LIST_ENTRY"]))
json.dump(d, open(p, "w"))
PY
  fi
  exit "${SPAWN_RC:-0}"
fi
exit 0
'''

TMUX_STUB = '''#!/usr/bin/env bash
echo "$*" >> "$TMUXLOG"
shift 2
if [[ "$1" == "capture-pane" ]]; then
  [[ -n "${PANE_FAIL:-}" ]] && exit 1
  cat "$FAKE_PANE"; exit 0
fi
if [[ "$1" == "display-message" ]]; then
  [[ -n "${PANE_FAIL:-}" ]] && exit 1
  echo "${PANE_TITLE:-t}"
  exit 0
fi
if [[ "$1" == "show-environment" ]]; then
  if [[ -s "$ENVSTATE" ]]; then echo "OPENCODE_CONFIG=$(cat "$ENVSTATE")"; fi
  exit 0
fi
if [[ "$1" == "set-environment" ]]; then
  if [[ "$2" == "-gu" ]]; then : > "$ENVSTATE"; else echo "$4" > "$ENVSTATE"; fi
  exit 0
fi
if [[ "$1" == "send-keys" ]]; then
  echo "send-keys $*" >> "$TMUXLOG"; exit 0
fi
exit 0
'''


class Stubs:
    """Fake codeman/tmux binaries + a fake opencode DB in a temp dir."""

    def __init__(self, tmp):
        self.tmp = tmp
        self.list_file = os.path.join(tmp, "list.json")
        self.sends = os.path.join(tmp, "sends.log")
        self.pane = os.path.join(tmp, "pane.txt")
        self.db = os.path.join(tmp, "oc.db")
        self.tmuxlog = os.path.join(tmp, "tmux.log")
        self.envstate = os.path.join(tmp, "envstate")
        codeman = os.path.join(tmp, "codeman")
        tmux = os.path.join(tmp, "tmux")
        for p, body in ((codeman, CODEMAN_STUB), (tmux, TMUX_STUB)):
            with open(p, "w") as fh:
                fh.write(body)
            os.chmod(p, 0o755)
        self.codeman, self.tmux = codeman, tmux
        with open(self.pane, "w") as fh:
            fh.write("idle prompt\nctrl+p commands\n")
        with open(self.list_file, "w") as fh:
            fh.write("[]")

    def env(self, **over):
        e = dict(os.environ)
        e.update({"CODMAN_BIN": self.codeman, "TMUX_BIN": self.tmux,
                  "FAKE_LIST": self.list_file, "SENDS": self.sends,
                  "FAKE_PANE": self.pane, "FAKE_DB": self.db,
                  "OPENCODE_DB": self.db, "TMUXLOG": self.tmuxlog,
                  "ENVSTATE": self.envstate})
        e.update({k: str(v) for k, v in over.items()})
        return e

    def sessions(self, entries):
        with open(self.list_file, "w") as fh:
            json.dump(entries, fh)

    def dbmk(self, sessions, messages=None):
        con = sqlite3.connect(self.db)
        con.execute("create table if not exists session(id text primary key,"
                    " title text, time_created int)")
        con.execute("create table if not exists message(id integer primary key"
                    " autoincrement, session_id text, time_created int, data text)")
        for sid, title, ts in sessions:
            con.execute("insert into session values(?,?,?)", (sid, title, ts))
        for sid, role, ts in (messages or []):
            con.execute("insert into message(session_id,time_created,data)"
                        " values(?,?,?)", (sid, ts, json.dumps({"role": role})))
        con.commit()
        con.close()

    def send_count(self, needle=""):
        if not os.path.exists(self.sends):
            return 0
        with open(self.sends) as fh:
            lines = [l for l in fh if l.strip()]
        if needle:
            return len([l for l in lines if needle in l])
        return len(lines)


class TempCase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="f17fx-")
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.st = Stubs(self.tmp)


# ---------------------------------------------------------------------------
# Finding 1: shared observed-state classifier
# ---------------------------------------------------------------------------

class TestClassifier(unittest.TestCase):
    def test_busy_idle_busy_tool_never_idle(self):
        busy = {"id": "x", "status": "busy"}
        idle = {"id": "x", "status": "idle"}
        s, k = observed.classify_lane(busy, True, "ctrl+p", 0)
        self.assertEqual(s, "busy")
        s, k = observed.classify_lane(idle, True, "ctrl+p", k)
        self.assertEqual(s, "idle-candidate")          # 1st sample: NOT terminal
        s, k = observed.classify_lane(idle, True, "esc interrupt working", k)
        self.assertEqual(s, "tool-active")             # mid-tool flip resets
        self.assertEqual(k, 0)
        s, k = observed.classify_lane(idle, True, "ctrl+p", k)
        self.assertEqual(s, "idle-candidate")
        s, k = observed.classify_lane(idle, True, "ctrl+p", k)
        self.assertEqual(s, "idle-confirmed")          # 2-poll confirmation

    def test_pending_permission_and_exit_and_gone(self):
        idle = {"id": "x", "status": "idle"}
        s, _ = observed.classify_lane(idle, True, "Permission required\n[y] yes", 1)
        self.assertEqual(s, "pending-dialog")
        s, _ = observed.classify_lane({"id": "x", "status": "idle",
                                       "paneExit": {"status": 1}}, True, "bye", 0)
        self.assertEqual(s, "exited")
        s, _ = observed.classify_lane(None, True, "", 0)
        self.assertEqual(s, "gone")

    def test_unreadable_pane_is_unknown_not_idle(self):
        s, _ = observed.classify_lane({"id": "x", "status": "idle"}, False, "", 0)
        self.assertEqual(s, "unknown")

    def test_api_failure_never_iterated(self):
        old = observed.RUN

        def boom(cmd, timeout=30):
            raise OSError("codeman gone")
        observed.RUN = boom
        try:
            ok, data = observed.list_sessions()
            self.assertFalse(ok)
            self.assertEqual(data, [])
        finally:
            observed.RUN = old

    def test_malformed_json_is_unknown(self):
        old = observed.RUN

        class R:
            returncode = 0
            stdout = "{not json"
        observed.RUN = lambda cmd, timeout=30: R()
        try:
            ok, _ = observed.list_sessions()
            self.assertFalse(ok)
        finally:
            observed.RUN = old


class TestWaitConfirmed(TempCase):
    def test_single_idle_does_not_return(self):
        calls = {"n": 0}
        seq = [(True, [{"id": "w1", "status": "busy"}]),
               (True, [{"id": "w1", "status": "idle"}]),
               (True, [{"id": "w1", "status": "busy"}]),   # tool boundary flip
               (True, [{"id": "w1", "status": "idle"}]),
               (True, [{"id": "w1", "status": "idle"}])]

        def fake_list():
            r = seq[min(calls["n"], len(seq) - 1)]
            calls["n"] += 1
            return r
        o1, o2, o3 = ww.observed.list_sessions, ww.observed.pane_read, ww.time.sleep
        ww.observed.list_sessions = fake_list
        ww.observed.pane_read = lambda sid, timeout=10: (True, "ctrl+p")
        ww.time.sleep = lambda s: None
        try:
            _t, status, _c = ww.wait(["w1"], poll_interval=0)
        finally:
            ww.observed.list_sessions, ww.observed.pane_read, ww.time.sleep = o1, o2, o3
        self.assertEqual(status, "idle")
        self.assertGreaterEqual(calls["n"], 5)   # never fired on the first sample

    def test_api_loss_alerts_then_recovers(self):
        calls = {"n": 0}

        def fake_list():
            calls["n"] += 1
            if calls["n"] == 2:
                return False, []      # transient loss mid-monitoring
            return True, [{"id": "w1", "status": "idle"}]
        o1, o2, o3 = ww.observed.list_sessions, ww.observed.pane_read, ww.time.sleep
        ww.observed.list_sessions = fake_list
        ww.observed.pane_read = lambda sid, timeout=10: (True, "ctrl+p")
        ww.time.sleep = lambda s: None
        try:
            _t, status, _c = ww.wait(["w1"], poll_interval=0)
        finally:
            ww.observed.list_sessions, ww.observed.pane_read, ww.time.sleep = o1, o2, o3
        self.assertEqual(status, "idle")   # survived the UNKNOWN; not "gone"


# ---------------------------------------------------------------------------
# Finding 3: dialog observation semantics via _observe_pass
# ---------------------------------------------------------------------------

class TestDialogPass(unittest.TestCase):
    def setUp(self):
        self.sent = []
        self.rem = []
        self._o = (ww._send_wake, ww._reminder, ww.observed.pane_read)
        ww._send_wake = self._capture
        ww._reminder = self._note

    def tearDown(self):
        ww._send_wake, ww._reminder, ww.observed.pane_read = self._o

    def _capture(self, cid, msg, dry, self_id=None, persist=True):
        self.sent.append(msg)
        return getattr(self, "send_ok", True)

    def _note(self, state, key, text, mono, cid, dry, critical=False):
        self.rem.append((key, text))
        return True

    def _state(self, workers=("co", "w1")):
        reg = {w: ({"role": "coordinator"} if w == "co" else {}) for w in workers}
        return {"registered": reg, "permission_flag": {}, "last_status": {},
                "delivered": {}, "busy_since": {}, "idle_streak": {},
                "names": {}, "lane_state": {}}

    def test_failed_pane_reads_never_emit_resolved(self):
        state = self._state()
        state["permission_flag"]["d-active:w1"] = ["Allow once"]
        ww.observed.pane_read = lambda sid, timeout=10: (False, "")
        for i in range(4):
            ww._observe_pass(state, [{"id": "w1", "status": "idle"}], "co", True,
                             100 + i * 10)
        self.assertFalse([m for m in self.sent if "no longer observed" in m])
        self.assertTrue(state["permission_flag"].get("d-active:w1"))
        self.assertTrue([k for k, _t in self.rem if k.startswith("observe:pane")])

    def test_three_clean_resolve_with_recurrence(self):
        state = self._state(workers=("w1",))
        # 1 active dialog, 3 clean (resolve), then check only ONE "no longer observed"
        # We send only 1 active + 3 clean, NOT a second recurrence.
        # The test verifies that exactly 1 resolve happens after 3 clean polls.
        panes = ["\nAllow once\n", "clean\n", "clean\n", "clean\n"]
        idx = {"i": 0}

        def pr(sid, timeout=10):
            t = panes[min(idx["i"], len(panes) - 1)]
            idx["i"] += 1
            return True, t
        ww.observed.pane_read = pr
        for _ in range(len(panes)):
            ww._observe_pass(state, [{"id": "w1", "status": "idle"}], "co", True, 500)
        resolved = [m for m in self.sent if "no longer observed" in m]
        self.assertEqual(len(resolved), 1)          # exactly one RESOLVED after 3 clean polls
        # Current wording (audit finding 3): RESOLVED proves idle before saying
        # runnable. Here the 3 clean polls also confirmed the idle streak
        # (status idle + IDLE_CONFIRM_POLLS), so the runnable claim is proven.
        self.assertIn("and lane confirmed idle/runnable", resolved[0])
        self.assertEqual(len([m for m in self.sent if "DIALOG" in m]), 1)

    def test_two_clean_then_unknown_then_clean_no_premature_resolve(self):
        state = self._state(workers=("w1",))
        seq = iter([(True, "\nAllow once\n"), (True, "c\n"), (True, "c\n"),
                    (False, ""), (True, "c\n"), (True, "c\n"), (True, "c\n")])
        ww.observed.pane_read = lambda sid, timeout=10: next(seq, (True, "c\n"))
        for _ in range(7):
            ww._observe_pass(state, [{"id": "w1", "status": "idle"}], "co", True, 500)
        self.assertEqual(len([m for m in self.sent if "no longer observed" in m]), 1)

    def test_coordinator_scanned(self):
        state = self._state()
        ww.observed.pane_read = lambda sid, timeout=10: (
            True, "Permission required\n") if sid == "co" else (True, "ok\n")
        ww._observe_pass(state, [{"id": "co", "status": "idle"},
                                 {"id": "w1", "status": "idle"}], "co", True, 100)
        self.assertTrue(any("co DIALOG" in m for m in self.sent))

    def test_quoted_phrase_outside_tail_not_dialog(self):
        state = self._state(workers=("w1",))
        filler = "\n".join("transcript line %d" % i for i in range(40))
        ww.observed.pane_read = lambda sid, timeout=10: (
            True, 'user quoted "Allow once" earlier\n' + filler + "\nready\n")
        ww._observe_pass(state, [{"id": "w1", "status": "idle"}], "co", True, 100)
        self.assertFalse(any("DIALOG" in m for m in self.sent))
        self.assertFalse(state["permission_flag"].get("d-active:w1"))

    def test_aggregate_pending_with_one_still_pending_lane(self):
        state = self._state(workers=("w1", "w2"))
        ww.observed.pane_read = lambda sid, timeout=10: (
            True, "Permission required\n") if sid == "w1" else (True, "clean\n")
        ww._observe_pass(state, [{"id": "w1", "status": "idle"},
                                 {"id": "w2", "status": "idle"}], "co", True, 100)
        self.assertTrue(state["dialog_pending"])

    def test_send_failure_persists_pending_notify(self):
        state = self._state(workers=("w1",))
        self.send_ok = False
        ww.observed.pane_read = lambda sid, timeout=10: (True, "Permission required\n")
        ww._observe_pass(state, [{"id": "w1", "status": "idle"}], "co", True, 100)
        self.assertTrue(state["pending_notify"])

    def test_key_suggestion_only_from_rendered_options(self):
        d = observed.detect_dialog("x\nAllow once [a]\n")
        self.assertEqual(observed.dialog_key_suggestion(d), "a")
        d2 = observed.detect_dialog("Yes, proceed (y)\n")
        self.assertEqual(observed.dialog_key_suggestion(d2), "Enter")
        d3 = observed.detect_dialog("Permission required\n")
        self.assertIsNone(observed.dialog_key_suggestion(d3))

    def test_idle_confirmation_and_unknown_emit_no_events(self):
        state = self._state(workers=("w1",))
        ww.observed.pane_read = lambda sid, timeout=10: (True, "ctrl+p")
        sess = [{"id": "w1", "status": "idle"}]
        ww._observe_pass(state, sess, "co", True, 10)
        self.assertFalse(any(" w1 idle" in m or "w1 idle" in m for m in self.sent))
        ww._observe_pass(state, sess, "co", True, 20)
        self.assertTrue(any("w1 idle" in m for m in self.sent))
        self.sent = []
        ww.observed.pane_read = lambda sid, timeout=10: (False, "")
        ww._observe_pass(state, sess, "co", True, 30)
        self.assertEqual(self.sent, [])   # UNKNOWN emits no events, no advice

    def test_gone_lane_keeps_names_from_snapshot(self):
        state = self._state(workers=("w1",))
        state["last_status"]["w1"] = "busy"
        ww.observed.pane_read = lambda sid, timeout=10: (True, "x\n")
        ww._observe_pass(state, [{"id": "w1", "name": "q9-ghost", "status": "busy"}],
                         "co", True, 5)          # record name
        ww._observe_pass(state, [], "co", True, 10)   # session gone
        self.assertTrue(any("w1 gone" in m for m in self.sent))
        self.assertTrue(state["names"].get("w1") == "q9-ghost")


# ---------------------------------------------------------------------------
# Finding 8: events offsets + reminder dedupe
# ---------------------------------------------------------------------------

class TestEvents(TempCase):
    def setUp(self):
        super().setUp()
        self.log = os.path.join(self.tmp, "events.log")
        self.ackf = os.path.join(self.tmp, "events-ack.json")
        open(self.log, "a").close()

    def run_e(self, args=()):
        return sh([sys.executable, os.path.join(HERE, "events-check.py")] +
                  list(args), env=self.st.env(EVENTS_LOG=self.log,
                                             EVENTS_ACK=self.ackf))

    def append(self, line):
        with open(self.log, "a") as fh:
            fh.write(line + "\n")

    def test_same_second_and_backward_clock_still_visible(self):
        t = int(time.time())
        self.append("{} A".format(t))
        self.assertIn(" A", self.run_e(["--ack"]).stdout)
        self.append("{} B".format(t))            # same second after ACK
        self.append("{} C".format(t - 99999))    # backward clock jump
        out = self.run_e().stdout
        self.assertIn(" B", out)
        self.assertIn(" C", out)

    def test_malformed_preserved_not_fatal(self):
        self.append("garbage-no-timestamp")
        self.append("{} good".format(int(time.time())))
        r = self.run_e()
        self.assertEqual(r.returncode, 0)
        self.assertIn("preserved-malformed", r.stdout)
        self.assertIn("good", r.stdout)

    def test_torn_write_not_acked_until_complete(self):
        with open(self.log, "a") as fh:
            fh.write("{} partial".format(int(time.time())))   # no newline
        self.assertNotIn("partial", self.run_e(["--ack"]).stdout)
        with open(self.log, "a") as fh:
            fh.write("\n")
        self.assertIn("partial", self.run_e().stdout)

    def test_rotation_rescans(self):
        # Pre-rotation: long line at small offset
        self.append("{} old-padded-line-12345678901234567890".format(int(time.time())))
        self.run_e(["--ack"])
        # Post-rotation: shorter file than the ack position triggers size-shrink detection
        with open(self.log, "w") as fh:
            fh.write("{} after-rot\n".format(int(time.time())))
        r = self.run_e()
        self.assertIn("after-rot", r.stdout)
        self.assertIn("ROTATED", r.stderr)

    def test_ack_covers_only_observed_snapshot(self):
        self.append("{} one".format(int(time.time())))
        self.assertIn("one", self.run_e(["--ack"]).stdout)
        self.append("{} two".format(int(time.time())))
        out = self.run_e().stdout
        self.assertNotIn("one", out)
        self.assertIn("two", out)


class TestReminderDedupe(unittest.TestCase):
    def setUp(self):
        self._o = (ww._send_wake, ww.HERE)
        self.tmp_here = tempfile.mkdtemp(prefix="f17rem-")
        ww.HERE = self.tmp_here
        self.calls = []

    def tearDown(self):
        if self.tmp_here.startswith(tempfile.gettempdir()):
            shutil.rmtree(self.tmp_here, ignore_errors=True)
        ww._send_wake, ww.HERE = self._o

    def test_critical_unchanged_deduped_not_persisted(self):
        ww._send_wake = lambda cid, msg, dry, self_id=None, persist=True: (
            self.calls.append(persist) or True)
        state = {}
        self.assertTrue(ww._reminder(state, "board:dirty", "dirty(1)", 100,
                                     "co", True, critical=True))
        self.assertFalse(ww._reminder(state, "board:dirty", "dirty(1)", 160,
                                      "co", True, critical=True))
        self.assertTrue(ww._reminder(state, "board:dirty", "dirty(2)", 170,
                                     "co", True, critical=True))
        self.assertEqual(self.calls, [False, False])  # reminders never queue events

    def test_send_failure_bounded_retry(self):
        ok = {"v": False}
        ww._send_wake = lambda *a, **k: ok["v"]
        state = {}
        ww._reminder(state, "k", "text", 100, "co", True, critical=True)
        ww._reminder(state, "k", "text", 101, "co", True, critical=True)  # gap30
        ok["v"] = True
        self.assertTrue(ww._reminder(state, "k", "text", 131, "co", True,
                                     critical=True))
# ---------------------------------------------------------------------------
# Finding 4: board currency on temporary bare remotes + dispatch hold
# ---------------------------------------------------------------------------

class TestBoardCurrency(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="f17board-")
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.bare = os.path.join(self.tmp, "remote.git")
        os.makedirs(self.bare)
        sh(["git", "init", "--bare", self.bare])
        sh(["git", "-C", self.bare, "symbolic-ref", "HEAD", "refs/heads/main"])
        self.work = os.path.join(self.tmp, "work")
        sh(["git", "clone", self.bare, self.work])
        git(self.work, "checkout", "-B", "main")
        with open(os.path.join(self.work, "seed.md"), "w") as fh:
            fh.write("x\n")
        git(self.work, "add", ".")
        git(self.work, "commit", "-m", "seed")
        git(self.work, "push", "-u", "origin", "main")

    def run_basics(self, repo=None):
        e = dict(os.environ)
        e["BH_PLAN_REPO"] = repo or self.work
        return sh([sys.executable, os.path.join(HERE, "basics-check.py"),
                   "--board-json"], env=e)

    def parse(self, r):
        return json.loads(r.stdout)

    def test_clean_ok(self):
        r = self.run_basics()
        self.assertEqual(r.returncode, 0)
        self.assertEqual(self.parse(r)["currency"], "ok")

    def test_ahead_unpushed_flag_not_stale(self):
        with open(os.path.join(self.work, "n.md"), "w") as fh:
            fh.write("n\n")
        git(self.work, "add", ".")
        git(self.work, "commit", "-m", "local")
        d = self.parse(self.run_basics())
        self.assertEqual(d["currency"], "ok")
        self.assertTrue([f for f in d["flags"] if "unpushed" in f])

    def test_behind_stale_and_divergence(self):
        other = os.path.join(self.tmp, "other")
        sh(["git", "clone", self.bare, other])
        with open(os.path.join(other, "r.md"), "w") as fh:
            fh.write("r\n")
        git(other, "add", ".")
        git(other, "commit", "-m", "remote")
        git(other, "push", "origin", "HEAD:refs/heads/main")
        r = self.run_basics()
        self.assertEqual(r.returncode, 9)
        self.assertEqual(self.parse(r)["currency"], "stale")
        # divergence: also commit locally
        with open(os.path.join(self.work, "l.md"), "w") as fh:
            fh.write("l\n")
        git(self.work, "add", ".")
        git(self.work, "commit", "-m", "local-too")
        d = self.parse(self.run_basics())
        self.assertEqual(d["currency"], "stale")
        self.assertTrue(d["detail"].get("unpushed"))

    def test_fetch_failure_is_unknown(self):
        broke = os.path.join(self.tmp, "broken-remote")
        os.makedirs(broke)
        sh(["git", "init", broke])
        with open(os.path.join(broke, "a.md"), "w") as fh:
            fh.write("a\n")
        git(broke, "add", ".")
        git(broke, "commit", "-m", "a")
        git(broke, "remote", "add", "origin", os.path.join(self.tmp, "gone.git"))
        r = self.run_basics(repo=broke)
        self.assertEqual(r.returncode, 9)
        d = self.parse(r)
        self.assertEqual(d["currency"], "unknown")
        self.assertIn("fetch", d["detail"]["why"].lower())

    def test_missing_origin_main_is_unknown(self):
        empty = os.path.join(self.tmp, "empty.git")
        os.makedirs(empty)
        sh(["git", "init", "--bare", empty])
        w2 = os.path.join(self.tmp, "w2")
        sh(["git", "clone", empty, w2])
        with open(os.path.join(w2, "a.md"), "w") as fh:
            fh.write("a\n")
        git(w2, "add", ".")
        git(w2, "commit", "-m", "a")
        d = self.parse(self.run_basics(repo=w2))
        self.assertEqual(d["currency"], "unknown")

    def test_wrong_branch_stale(self):
        git(self.work, "checkout", "-b", "topic")
        d = self.parse(self.run_basics())
        self.assertEqual(d["currency"], "stale")
        self.assertIn("topic", d["detail"]["why"])

    def test_dirty_stale(self):
        with open(os.path.join(self.work, "dirt.md"), "w") as fh:
            fh.write("d\n")
        self.assertEqual(self.parse(self.run_basics())["currency"], "stale")


class TestDispatchHoldAndDelivery(TempCase):
    def setUp(self):
        super().setUp()
        self.b_ok = os.path.join(self.tmp, "basics-ok.sh")
        with open(self.b_ok, "w") as fh:
            fh.write('#!/usr/bin/env bash\n'
                     'echo \'{"currency":"ok","flags":[]}\'\nexit 0\n')
        os.chmod(self.b_ok, 0o755)
        self.b_hold = os.path.join(self.tmp, "basics-hold.sh")
        with open(self.b_hold, "w") as fh:
            fh.write('#!/usr/bin/env bash\n'
                     'echo \'{"currency":"unknown","flags":["BOARD: x"]}\'\nexit 9\n')
        os.chmod(self.b_hold, 0o755)
        self.pf = os.path.join(self.tmp, "mission.txt")
        with open(self.pf, "w") as fh:
            fh.write("m\n")
        self.sid = "a1b2c3d4-0000-0000-0000-000000000001"
        self.ts = int(time.time() * 1000)
        self.st.sessions([{"id": self.sid, "name": "w-lane", "status": "idle",
                           "createdAt": self.ts}])
        self.wstate = os.path.join(self.tmp, "watcher-state.json")
        with open(self.wstate, "w") as fh:
            json.dump({"watcher": {"pid": os.getpid()}, "registered": {}}, fh)

    def dispatch(self, basics=None, **over):
        env = self.st.env(BASICS_CHECK_PY=basics or self.b_ok, OC_SID="oc1",
                          WATCHER_STATE=self.wstate, VERIFY_WAIT=1,
                          STALL_WAIT=1, **over)
        return sh(["bash", os.path.join(HERE, "dispatch-lane.sh"), self.sid,
                   self.pf], env=env)

    def test_stale_currency_holds_without_sending(self):
        r = self.dispatch(basics=self.b_hold)
        self.assertEqual(r.returncode, 8)
        self.assertEqual(self.st.send_count(), 0)
        self.assertIn("BOARD-CURRENCY-HOLD", r.stderr)

    def test_fast_completion_not_judged_by_busy_flip(self):
        self.st.dbmk([("oc1", "w-lane", self.ts)],
                     [("oc1", "user", 1), ("oc1", "assistant", 2)])
        r = self.dispatch(ANSWERED="1")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self.st.send_count(), 1)

    def test_idle_staying_idle_never_double_sends(self):
        self.st.dbmk([("oc1", "w-lane", self.ts)],
                     [("oc1", "user", 1), ("oc1", "assistant", 2)])
        # Current contract (finding 2): delivery IS proven by message identity,
        # but a healthy-observed idle lane that never starts the mission is the
        # documented rc-5 confirmed queue stall - owner decides respawn, no
        # resend. The guarded invariant (exactly one send) stays.
        r = self.dispatch()
        self.assertEqual(r.returncode, 5, r.stderr)
        self.assertEqual(self.st.send_count(), 1)

    def test_lost_delivery_exactly_one_retry(self):
        self.st.dbmk([("oc1", "w-lane", self.ts)])
        r = self.dispatch(DELIVER="no")
        self.assertEqual(r.returncode, 6, r.stderr)
        self.assertEqual(self.st.send_count(), 2)

    def test_send_rc_failure_proven_loss(self):
        self.st.dbmk([("oc1", "w-lane", self.ts)])
        r = self.dispatch(SEND_EXIT="1", DELIVER="no")
        self.assertEqual(r.returncode, 6, r.stderr)
        self.assertEqual(self.st.send_count(), 2)

    def test_empty_db_classic_probe_is_unknown_not_success(self):
        self.st.dbmk([("oc1", "w-lane", self.ts)])
        r = sh([sys.executable, os.path.join(HERE, "lane-stalled.py"), self.sid, "0"],
               env=self.st.env())
        self.assertEqual(r.returncode, 6)

    def test_ambiguous_two_sessions_created_together(self):
        self.st.dbmk([("oc1", "other-a", self.ts), ("oc2", "other-b", self.ts + 900)])
        r = self.dispatch()
        self.assertEqual(r.returncode, 6, r.stderr)
        self.assertEqual(self.st.send_count(), 1)

    def test_db_unavailable_unknown_single_send(self):
        r = self.dispatch(OPENCODE_DB=os.path.join(self.tmp, "gone.db"))
        self.assertEqual(r.returncode, 6, r.stderr)
        self.assertEqual(self.st.send_count(), 1)

    def test_watcher_down_visible_monitoring_hold(self):
        # Use a PID that does NOT exist so _is_running() returns False
        # -> watch add fails -> dispatch returns rc 9 (MONITORING-HOLD)
        fake_gone_pid = os.getpid() + 99999
        self.st.dbmk([("oc1", "w-lane", self.ts)], [("oc1", "user", 1)])
        with open(self.wstate, "w") as fh:
            json.dump({"watcher": {"pid": fake_gone_pid}, "registered": {}}, fh)
        # ANSWERED=1: only a delivered+answered lane reaches the watch-add
        # step; an idle unconsumed mission is legitimately judged stalled (rc 5)
        # BEFORE registration. MONITORING-HOLD propagation (finding 2) is
        # exercised on the path that actually registers the watcher.
        r = self.dispatch(ANSWERED="1")
        self.assertEqual(r.returncode, 9, r.stderr)
        self.assertIn("MONITORING-HOLD", r.stderr)
        self.assertEqual(self.st.send_count(), 1)

    def test_permission_dialog_no_resend(self):
        self.st.dbmk([("oc1", "w-lane", self.ts)])
        with open(self.st.pane, "w") as fh:
            fh.write("x\nAllow once [a]\n")
        r = self.dispatch(DELIVER="no")
        self.assertEqual(r.returncode, 4, r.stderr)
        self.assertEqual(self.st.send_count(), 1)

    def test_tool_busy_is_not_mission_receipt(self):
        self.st.dbmk([("oc1", "w-lane", self.ts)], [("oc1", "user", 1)])
        with open(self.st.pane, "w") as fh:
            fh.write("running\nesc interrupt\n")
        r = self.dispatch(DELIVER="no")
        self.assertEqual(r.returncode, 6, r.stderr)
        self.assertIn("UNPROVEN", r.stderr)
        self.assertEqual(self.st.send_count(), 1)

    def test_working_flip_still_one_send_identity_proven(self):
        self.st.sessions([{"id": self.sid, "name": "w-lane", "status": "busy",
                           "createdAt": self.ts}])
        self.st.dbmk([("oc1", "w-lane", self.ts)],
                     [("oc1", "user", 1), ("oc1", "assistant", 2)])
        r = self.dispatch()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self.st.send_count(), 1)

    def test_dry_run_sends_nothing(self):
        self.st.dbmk([("oc1", "w-lane", self.ts)],
                     [("oc1", "user", 1), ("oc1", "assistant", 2)])
        r = sh(["bash", os.path.join(HERE, "dispatch-lane.sh"), self.sid, self.pf,
                "--dry-run"], env=self.st.env(OC_SID="oc1",
                                              BASICS_CHECK_PY=self.b_ok))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self.st.send_count(), 0)
        self.assertIn("DRY-RUN", r.stderr)

    def test_dispatch_log_identity_recorded(self):
        self.st.dbmk([("oc1", "w-lane", self.ts)], [("oc1", "user", 1)])
        logf = os.path.join(HERE, "dispatch-log.jsonl")
        before = open(logf).read() if os.path.exists(logf) else ""
        try:
            self.dispatch()
            with open(logf) as fh:
                entry = json.loads(fh.read().strip().splitlines()[-1])
        finally:
            with open(logf, "w") as fh:
                fh.write(before)
        self.assertIn("dispatch-", entry["nonce"])
        self.assertIn("baseline_users", entry)

    def tearDown(self):
        logf = os.path.join(HERE, "dispatch-log.jsonl")
        if os.path.exists(logf):
            os.remove(logf)


# ---------------------------------------------------------------------------
# Findings 5+6: completion accounting and task-specific utilization
# (via one full _watchdog_layer pass with ROOT redirected to a fixture tree)
# ---------------------------------------------------------------------------

PLAN_PY_STUB = '''#!/usr/bin/env python3
import os, sys
mode = sys.argv[1] if len(sys.argv) > 1 else "list"
if mode == "list":
    if os.environ.get("PLAN_LIST_FAIL"):
        sys.exit(3)
    print("core-1               ready    v0.4.x   Title one  [tasks/core-1-x.md]")
    print("core-2               ready    v0.4.x   Title two  [tasks/core-2-y.md]")
    print("core-3               ready    v0.4.x   Title three [tasks/core-3-z.md]")
    sys.exit(0)
tid = sys.argv[2]
if tid == "core-1":
    print("state: ready")
elif tid == "core-2":
    print("state: claimed")
    print("  owner: somebody (session)")
elif tid == "core-3":
    print("state: ready")
    print("  depends on core-888: pending")
sys.exit(0)
'''


class TestWatchdogFixtures(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.mkdtemp(prefix="f17wd-")
        self._o = (ww.ROOT_DIR, ww._send_wake, ww._reminder, ww._pane_class,
                   ww._owner_stop_active, ww._published_at_planning,
                   ww._open_ro_db, observed.list_sessions)
        self._env = dict(os.environ)
        ww.ROOT_DIR = self.root
        self.sent = []
        self.rem = []
        ww._send_wake = lambda cid, msg, dry, self_id=None, persist=True: (
            self.sent.append(msg) or True)
        ww._reminder = self._note
        ww._pane_class = lambda wid: "ornith"
        self._owner_stop_cache = [False]
        ww._owner_stop_active = lambda: self._owner_stop_cache[0]
        ww._published_at_planning = lambda base: False
        ww._open_ro_db = lambda: None
        self.sessions = []
        ww.observed.list_sessions = self._ls
        os.environ["CODMAN_BIN"] = "false"   # lane-stalled probes fail fast
        os.environ["GH_BIN"] = "false"       # no live PR observation
        self.tmp_plan = os.path.join(self.root, "planstub")
        pdir = os.path.join(self.tmp_plan, "scripts")
        os.makedirs(pdir)
        pp = os.path.join(pdir, "plan.py")
        with open(pp, "w") as fh:
            fh.write(PLAN_PY_STUB)
        os.chmod(pp, 0o755)
        self.basics_stub = os.path.join(self.root, "basics-stub.py")
        with open(self.basics_stub, "w") as fh:
            fh.write("import json,sys\nprint(json.dumps({'flags': [],"
                     " 'board': 'ok'}))\n")
        os.environ["BASICS_CHECK_PY"] = self.basics_stub
        self._mono = 1000.0
        os.makedirs(os.path.join(self.root, "evidence", "board-2026-10-22"))
        os.makedirs(os.path.join(self.root, "evidence", "board-2026-10-23"))
        os.makedirs(os.path.join(self.root, "baseharbor-plan", "evidence",
                                 "codeman-x"))
        self.addCleanup(shutil.rmtree, self.root, True)

    def tearDown(self):
        (ww.ROOT_DIR, ww._send_wake, ww._reminder, ww._pane_class,
         ww._owner_stop_active, ww._published_at_planning, ww._open_ro_db,
         ww.observed.list_sessions) = self._o
        os.environ.clear()
        os.environ.update(self._env)

    def _ls(self):
        if isinstance(self.sessions, tuple):
            return self.sessions          # pre-formed (ok, data) pair
        return (True, list(self.sessions))

    @staticmethod
    def _pane_read(sid, timeout=10):
        return (True, "Build @ Ornith-9\nctrl+p")

    def _note(self, state, key, text, mono, cid, dry, critical=False):
        self.rem.append((key, text))
        return True

    def _run(self, state, sessions, plan_list_fail=None, published=None):
        self.sessions = sessions
        ww._published_at_planning = published or (lambda base: False)
        old = dict(os.environ)
        os.environ["BH_PLAN_DIR"] = self.tmp_plan
        if plan_list_fail:
            os.environ["PLAN_LIST_FAIL"] = "1"
        mono = self._mono
        self._mono += 61                  # clear the 60s watchdog gate
        try:
            ww._watchdog_layer(state, "co", True, mono)
        finally:
            os.environ.clear()
            os.environ.update(old)
        return state

    @staticmethod
    def _state():
        return {"watcher": {"last_watchdog": 0, "pid": os.getpid()}, "reminders": {},
                "registered": {"co": {"role": "coordinator"}},
                "names": {}, "last_status": {}, "delivered": {},
                "busy_since": {}, "idle_streak": {}, "permission_flag": {},
                "completions": {}, "lane_state": {}, "pending_notify": []}

    @staticmethod
    def _ev(name, sub="board-2026-10-22", end_ts=None):
        d = os.path.join(ww.ROOT_DIR, "evidence", sub)
        p = os.path.join(d, name)
        tail = "END {}-DONE @{}".format(name.split("-")[0], end_ts or int(time.time()))
        with open(p, "w") as fh:
            fh.write("# {}\n\n## PLAN\n\n- [x] 0. step\n\n{}\n".format(name, tail))
        return p

    def test_stale_end_new_after_old_board_commit_fires_after_grace(self):
        old = int(time.time()) - 7200
        self._ev("lane-a.md", end_ts=old)
        state = self._run(self._state(), [])           # register first_seen
        self._run(state, [])                           # grace now exceeds 15min
        pub = [t for k, t in self.rem if k == "flash:publish"]
        self.assertTrue(pub, "aged END must fire reminder regardless of board commit age")
        self.assertIn("lane-a.md", pub[0])

    def test_unpublished_end_not_cleared_by_removal_or_old_commit(self):
        old = int(time.time()) - 7200
        self._ev("lane-b.md", end_ts=old)
        state = self._run(self._state(),
                          [{"id": "xb", "name": "lane-b", "status": "idle"}])
        self.rem = []
        self._run(state, [])   # session REMOVED from listing
        self.assertTrue([t for k, t in self.rem if k == "flash:publish"])

    def test_next_day_and_nested_evidence_locations(self):
        self._ev("lane-c.md", sub="board-2026-10-23", end_ts=int(time.time()) - 7200)
        p = os.path.join(ww.ROOT_DIR, "baseharbor-plan", "evidence", "codeman-x",
                        "lane-d.md")
        with open(p, "w") as fh:
            fh.write("END lane-d-DONE @{}\n".format(int(time.time()) - 7200))
        state = self._run(self._state(), [])
        self.rem = []
        self._run(state, [])
        texts = " ".join(t for k, t in self.rem if k == "flash:publish")
        self.assertIn("lane-c.md", texts)
        self.assertIn("lane-d.md", texts)

    def test_published_completion_retires_even_if_aged(self):
        self._ev("lane-e.md", end_ts=int(time.time()) - 86400)
        state = self._run(self._state(), [])
        self.rem = []
        self._run(state, [], published=lambda base: True)
        self.assertFalse([t for k, t in self.rem if k == "flash:publish"])

    def test_handoff_prose_alone_does_not_retire(self):
        # _published_at_planning checks commits, never HANDOFF text:
        hand = os.path.join(ww.ROOT_DIR, "baseharbor-plan", "evidence",
                            "codeman-x", "HANDOFF.md")
        with open(hand, "w") as fh:
            fh.write("historical mention of lane-f.md only\n")
        self._ev("lane-f.md", end_ts=int(time.time()) - 7200)
        state = self._run(self._state(), [])
        self.rem = []
        self._run(state, [])
        self.assertTrue([t for k, t in self.rem if k == "lane-f.md" or
                         (k == "flash:publish" and "lane-f.md" in t)])

    def test_reused_lane_prefix_exact_match(self):
        self._ev("q30-card.md")
        self.assertIsNone(ww._lane_evidence("q3"))
        self.assertTrue(ww._lane_evidence("q30-card"))

    def test_wip_owner_idle_still_owned_other_task_reminds(self):
        self._ev("q30-card.md")
        inbox = os.path.join(ww.ROOT_DIR, "evidence", "board-2026-10-22",
                             "delegation-inbox.txt")
        with open(inbox, "w") as fh:
            fh.write("# hdr\nTASKA: a work - O lane [wip:o9]\n"
                     "TASKB: b work - Q lane\n")
        sessions = [{"id": "o9id", "name": "o9-lane", "status": "idle"},
                    {"id": "co", "name": "w1-co", "status": "busy"}]
        state = self._run(self._state(), sessions)
        delegate = [t for k, t in self.rem if k == "flash:delegate"]
        self.assertTrue(delegate, "free task B must remind despite unrelated idle WIP lane")
        self.assertIn("TASKB", delegate[0])
        self.assertNotIn("TASKA", delegate[0])

    def test_owner_stop_suppresses_delegate(self):
        inbox = os.path.join(ww.ROOT_DIR, "evidence", "board-2026-10-22",
                             "delegation-inbox.txt")
        with open(inbox, "w") as fh:
            fh.write("TASKB: b work - Q lane\n")
        self._owner_stop_cache[0] = True
        state = self._run(self._state(),
                          [{"id": "x", "name": "o9-lane", "status": "idle"}])
        self.assertFalse([t for k, t in self.rem if k == "flash:delegate"])
        self.assertTrue([t for k, t in self.rem if k == "owner-stop"])

    def test_stale_wip_marker_flagged_not_dispatched(self):
        inbox = os.path.join(ww.ROOT_DIR, "evidence", "board-2026-10-22",
                             "delegation-inbox.txt")
        with open(inbox, "w") as fh:
            fh.write("TASKC: c - F lane [wip:zz]\n")
        state = self._run(self._state(), [])
        self.assertTrue([t for k, t in self.rem if k.startswith("wip:stale")])
        self.assertFalse([t for k, t in self.rem if k == "flash:delegate"
                          and "TASKC" in t])

    def test_claimed_ready_looking_card_never_dispatched(self):
        inbox = os.path.join(ww.ROOT_DIR, "evidence", "board-2026-10-22",
                             "delegation-inbox.txt")
        open(inbox, "w").write("")
        state = self._run(self._state(), [{"id": "x", "name": "o9-l", "status": "idle"}])
        delegate = [t for k, t in self.rem if k == "flash:delegate"]
        self.assertTrue(delegate)
        self.assertIn("core-1", delegate[0])   # only the genuinely ready card
        self.assertNotIn("core-2", delegate[0])  # claimed
        self.assertNotIn("core-3", delegate[0])  # unmet dependency

    def test_failed_planning_query_reported_not_fabricated(self):
        inbox = os.path.join(ww.ROOT_DIR, "evidence", "board-2026-10-22",
                             "delegation-inbox.txt")
        open(inbox, "w").write("")
        self._run(self._state(), [], plan_list_fail="1")
        self.assertTrue([t for k, t in self.rem if k.startswith("work:observe")])
        self.assertFalse([t for k, t in self.rem if k == "flash:delegate"])

    def test_no_safe_work_reports_nothing(self):
        inbox = os.path.join(ww.ROOT_DIR, "evidence", "board-2026-10-22",
                             "delegation-inbox.txt")
        with open(inbox, "w") as fh:
            fh.write("TASKA: a - O lane [wip:o9]\n")
        os.environ["PLAN_LIST_FAIL"] = "1"
        try:
            state = self._run(self._state(),
                              [{"id": "o9id", "name": "o9-lane", "status": "idle"}])
        finally:
            os.environ.pop("PLAN_LIST_FAIL", None)
        delegate = [t for k, t in self.rem if k == "flash:delegate"]
        self.assertFalse(delegate)

    def test_exited_idle_lane_separate_not_idlenoend(self):
        state = self._run(self._state(),
                          [{"id": "g1", "name": "q3-ghost", "status": "idle",
                            "paneExit": {"status": 1}}])
        self.rem = []
        state["idle_streak"]["g1"] = 2
        state["lane_state"]["g1"] = "exited"
        self._run(state, [{"id": "g1", "name": "q3-ghost", "status": "idle",
                            "paneExit": {"status": 1}}])
        self.assertFalse([t for k, t in self.rem if k.startswith("idlenoend")])

    def test_api_failure_reported_not_silently_clear(self):
        self._run(self._state(), (False, []))   # list observation FAILED
        self.assertTrue([t for k, t in self.rem if k == "observe:api"])


# ---------------------------------------------------------------------------
# Finding 7: spawn admission + cleanup
# ---------------------------------------------------------------------------

class TestSpawn(TempCase):
    def setUp(self):
        super().setUp()
        self.rdir = os.path.join(self.tmp, "cfg")
        os.makedirs(self.rdir)
        for f in ("flash2.json", "qwen36.json", "opencode.json"):
            with open(os.path.join(self.rdir, f), "w") as fh:
                fh.write("{}")
        self.cfg = os.path.join(self.rdir, "opencode.json")
        self._old_home = os.environ.get("HOME")
        os.environ["HOME"] = self.home = os.path.join(self.tmp, "home")
        self.addCleanup(setattr, os.environ, "HOME", self._old_home)
        os.makedirs(os.path.join(self.home, ".config", "opencode"), exist_ok=True)
        for f in os.listdir(self.rdir):
            shutil.copy(os.path.join(self.rdir, f),
                        os.path.join(self.home, ".config", "opencode", f))
        self.wstate = os.path.join(self.tmp, "watcher-state.json")
        # Use os.getpid() as watcher PID - the test process IS running,
        # and subprocess _is_running() will find it alive via os.kill.
        with open(self.wstate, "w") as fh:
            json.dump({"watcher": {"pid": os.getpid()}, "registered": {
                "c0ffee00-1111": {"role": "coordinator"}}}, fh)
        self.resf = os.path.join(self.tmp, "pool-reservations.json")

    def spawn(self, name, model_flag="--flash", **over):
        env = self.st.env(WATCHER_STATE=self.wstate, POOL_RES_FILE=self.resf,
                          SPAWN_LOCK=os.path.join(self.tmp, "spawn.lock"), **over)
        cmd = ["bash", os.path.join(HERE, "spawn-lane.sh"), name]
        if model_flag:
            cmd.append(model_flag)
        return sh(cmd, env=env)

    def test_concurrent_last_slot_admits_exactly_one(self):
        # Ensure pool is empty (reservation file cleared)
        with open(self.resf, "w") as fh:
            json.dump({}, fh)
        env1 = self.st.env(WATCHER_STATE=self.wstate, POOL_RES_FILE=self.resf,
                           SPAWN_LOCK=os.path.join(self.tmp, "spawn.lock"),
                           SPAWN_OUT='{"sessionId":"11111111-2222-3333-4444-555555555555"}',
                           SPAWN_LIST_ENTRY=json.dumps(
                               {"id": "11111111-2222-3333-4444-555555555555",
                                "name": "flash-a", "status": "busy"}))
        with open(self.st.pane, "w") as fh:
            fh.write("Build @ Flash-4.5\nctrl+p commands\n")
        p1 = subprocess.Popen(["bash", os.path.join(HERE, "spawn-lane.sh"),
                               "flash-a", "--flash"], env=env1,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                              text=True)
        time.sleep(0.3)
        r2 = self.spawn("flash-b", "--flash",
                        SPAWN_OUT='{"sessionId":"22222222-2222-3333-4444-555555555555"}')
        o1, e1 = p1.communicate(timeout=90)
        ok = [p1.returncode == 0, r2.returncode == 4]
        self.assertTrue(ok[0], "first spawn stdout={} stderr={}".format(o1, e1))
        self.assertTrue(ok[1], "second must be pool-full, got rc={} {}".format(
            r2.returncode, r2.stderr))
        self.assertEqual(self.st.send_count("spawned"), 1)

    def test_unavailable_list_is_hold_not_free_slot(self):
        r = self.spawn("flash-x", "--flash", LIST_FAIL="1")
        self.assertEqual(r.returncode, 4, r.stderr)
        self.assertIn("HOLD", r.stderr)
        self.assertEqual(self.st.send_count("spawned"), 0)

    def test_unreadable_pane_is_hold(self):
        self.st.sessions([{"id": "ornith-01", "name": "ornith-a", "status": "busy"}])
        r = self.spawn("flash-x", "--flash", PANE_FAIL="1")
        self.assertEqual(r.returncode, 4, r.stderr)
        self.assertEqual(self.st.send_count("spawned"), 0)

    def test_coordinator_excluded_by_identity(self):
        # Ensure pool is empty and watcher is running
        with open(self.resf, "w") as fh:
            json.dump({}, fh)
        # Write watcher state with test process PID so _is_running() returns True
        with open(self.wstate, "w") as fh:
            json.dump({"watcher": {"pid": os.getpid()}, "registered": {
                "c0ffee00-1111": {"role": "coordinator"}}}, fh)
        self.st.sessions([{"id": "c0ffee00-1111", "name": "flash-co",
                           "status": "busy"}])
        with open(self.st.pane, "w") as fh:
            fh.write("Build @ Flash-4.5\nctrl+p\n")
        r = self.spawn("flash-a", "--flash",
                       SPAWN_OUT='{"sessionId":"33333333-2222-3333-4444-555555555555"}')
        self.assertEqual(r.returncode, 0, r.stderr)

    def test_failed_parse_restores_environment(self):
        with open(self.st.envstate, "w") as fh:
            fh.write("pre-existing-config")
        r = self.spawn("flash-x", "--flash", SPAWN_OUT="not-json{{{")
        self.assertEqual(r.returncode, 1, r.stderr)
        with open(self.st.envstate) as fh:
            self.assertEqual(fh.read().strip(), "pre-existing-config")

    def test_watch_add_failure_means_not_ready(self):
        with open(self.st.pane, "w") as fh:
            fh.write("Build @ Ornith-9\nctrl+p\n")
        with open(self.wstate, "w") as fh:
            json.dump({"watcher": {"pid": 99999999}, "registered": {}}, fh)
        r = self.spawn("orn-x", "",
                       SPAWN_OUT='{"sessionId":"44444444-2222-3333-4444-555555555555"}')
        self.assertEqual(r.returncode, 8, r.stderr)
        self.assertIn("44444444", r.stderr)   # lane preserved for diagnosis


# ---------------------------------------------------------------------------
# Finding 1/6 surfaces in basics-check
# ---------------------------------------------------------------------------

class TestBasicsObservation(TempCase):
    def test_mix_from_pane_not_letters_exited_separate_idle_confirmed(self):
        ts = int(time.time() * 1000)
        self.st.sessions([
            {"id": "s-o9", "name": "o9-lane", "status": "idle", "createdAt": ts},
            {"id": "s-zz", "name": "zz-probe", "status": "idle",
             "paneExit": {"status": 1}, "createdAt": ts},
            {"id": "s-f9", "name": "f9-lane", "status": "idle", "createdAt": ts},
        ])
        with open(self.st.pane, "w") as fh:
            fh.write("Build @ Ornith-9\n")
        state = os.path.join(self.tmp, "watcher-state.json")
        with open(state, "w") as fh:
            json.dump({"busy_since": {}, "idle_streak": {"s-o9": 2, "s-f9": 0,
                                                         "s-zz": 5}}, fh)
        plan = os.path.join(self.tmp, "coord-plan.md")
        with open(plan, "w") as fh:
            fh.write("- [x] done\n")
        repo = os.path.join(self.tmp, "planrepo")
        os.makedirs(repo)
        sh(["git", "init", "--bare", repo])
        r = sh([sys.executable, os.path.join(HERE, "basics-check.py"), "--json"],
               env=self.st.env(WATCHER_STATE=state,
                               BH_COORDINATOR_PLAN=plan, BH_ROOT_DIR=self.tmp))
        d = json.loads(r.stdout)
        self.assertIn("reminder-summary", d)
        self.assertIn("unread", d["reminder-summary"])
        flags = d["flags"]
        self.assertIn("BOARD: currency UNKNOWN", " ".join(flags)[:200])
        text = " || ".join(flags)
        self.assertTrue([f for f in flags if f.startswith("IDLE LANES")
                         and "o9-lane" in f])
        self.assertFalse([f for f in flags if "f9-lane" in f and
                          f.startswith("IDLE LANES")])  # streak 0: NOT confirmed
        self.assertTrue([f for f in flags if f.startswith("EXITED PANES")
                         and "zz-probe" in f])
        self.assertFalse([f for f in flags if f.startswith("IDLE LANES")
                          and "zz-probe" in f])
        pool = [f for f in flags if f.startswith("POOL: flash")][0]
        self.assertIn("0<1", pool)   # f9-lane header says Ornith, not letters


class TestOpsFixes(unittest.TestCase):
    def test_exact_session_id_overrides_ambiguous_title_and_late_start(self):
        con = sqlite3.connect(":memory:")
        con.execute("create table session(id text, title text, time_created int)")
        con.executemany("insert into session values(?,?,?)", [
            ("ses_exact", "same", 999999), ("ses_other", "same", 1)])
        self.assertEqual(observed.map_lane_to_oc_session(con, 1, "same", "ses_exact"),
                         (True, "ses_exact"))
        con.close()

    def test_late_start_probe_exact_and_approx(self):
        with tempfile.TemporaryDirectory() as tmp:
            fx = Stubs(tmp)
            fx.dbmk([("ses_exact", "worker", 999999)], [("ses_exact", "user", 2)])
            entry = {"id": "worker-uuid", "name": "worker", "createdAt": 1,
                     "opencodeSessionId": "ses_exact"}
            fx.sessions([entry])
            cmd = [sys.executable, os.path.join(HERE, "lane-stalled.py"), "worker-uuid", "--baseline"]
            r = sh(cmd, env=fx.env())
            self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
            self.assertNotIn("approx", r.stderr)
            del entry["opencodeSessionId"]
            fx.sessions([entry])
            r = sh(cmd, env=fx.env())
            self.assertEqual(r.returncode, 0, r.stderr + r.stdout)
            self.assertIn("approx", r.stderr)

    def test_chrome_ignored_real_dialog_retained(self):
        self.assertIsNone(observed.detect_dialog("Ask Codex to do anything Permission required\n^g draft Yes, proceed"))
        self.assertIsNotNone(observed.detect_dialog("Permission required\nAllow once [a]\nAsk Codex to do anything\n^g draft"))

    def test_rotation_retains_unread_and_cursor_across_larger_replacement(self):
        import log_queue
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "events.log")
            ack = path + '.ack'
            log_queue.append(path, "1 first\n")
            self.assertEqual(log_queue.read(path, ack, True)[0], ["1 first"])
            huge = "2 " + "x" * (log_queue.LIMIT + 1)
            log_queue.append(path, huge + "\n")
            log_queue.append(path, "3 after\n")
            self.assertEqual(log_queue.read(path, ack)[0], [huge, "3 after"])
            log_queue.read(path, ack, True)
            log_queue.append(path, "0 backwards\n")
            self.assertEqual(log_queue.read(path, ack)[0], ["0 backwards"])

    def test_reminders_json_and_ack_separate_from_events(self):
        import log_queue
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, 'reminders.log')
            log_queue.append(path, '1 unread reminder\n')
            env = dict(os.environ, REMINDERS_LOG=path, REMINDERS_ACK=path+'.ack')
            cmd = [sys.executable, os.path.join(HERE, 'events-check.py'), '--reminders', '--json']
            self.assertEqual(json.loads(sh(cmd, env=env).stdout)['unread'], 1)
            self.assertEqual(sh(cmd + ['--ack'], env=env).returncode, 0)
            self.assertEqual(json.loads(sh(cmd, env=env).stdout)['unread'], 0)

    def test_task_named_evidence_both_locations_no_prefix_collision(self):
        old = ww.ROOT_DIR
        try:
            with tempfile.TemporaryDirectory() as tmp:
                ww.ROOT_DIR = tmp
                for location in ['evidence/board-2026-10-22', 'baseharbor-plan/evidence/board-2026-10-22']:
                    root = os.path.join(tmp, location)
                    os.makedirs(root)
                    p = os.path.join(root, 'task-implementation.md')
                    with open(p, 'w') as fh:
                        fh.write('Lane: `f19-opsmatcher`\n## PLAN\n')
                    self.assertEqual(ww._lane_evidence('f19-opsmatcher'), p)
                    self.assertIsNone(ww._lane_evidence('f1'))
                    os.unlink(p)
        finally:
            ww.ROOT_DIR = old

    def test_gone_notification_dedupes_even_on_send_failure(self):
        old = ww._notify
        calls = []
        ww._notify = lambda *args, **kw: calls.append(args[2]) or False
        try:
            state = {"registered": {"missing": {}}, "last_status": {}, "delivered": {}}
            ww._observe_pass(state, [], 'co', True, 10)
            ww._observe_pass(state, [], 'co', True, 11)
            self.assertEqual(sum('missing gone' in m for m in calls), 1)
        finally:
            ww._notify = old


if __name__ == "__main__":
    unittest.main(verbosity=2)
