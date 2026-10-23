#!/usr/bin/env python3
"""O50 auto-discovery + unmonitored-lane tripwire fixtures.

Covers the three OPS-1 rows:
1. wait-workers.py auto-discovery: a running+unregistered lane is added to
   observation and a DISCOVERED event fires.
2. spawn-lane.sh registration failure: rc!=0, sid-File written, failure on STDOUT.
3. basics-check.py UNMONITORED flag for running-not-registered sessions.

Run: python3 test_o50_autodiscover.py
"""
import importlib.util
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))


def _load(name, fname):
    spec = importlib.util.spec_from_file_location(name, os.path.join(HERE, fname))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


observed = _load("observed_o50", os.path.join(HERE, "observed.py"))
ww = _load("waitworkers_o50", os.path.join(HERE, "wait-workers.py"))


def sh(cmd, env=None, cwd=None):
    return subprocess.run(cmd, capture_output=True, text=True, env=env, cwd=cwd,
                          timeout=120)


CODEMAN_STUB = '''#!/usr/bin/env bash
if [[ "$1" == "agent" && "$2" == "list" ]]; then cat "$FAKE_LIST"; exit 0; fi
if [[ "$1" == "agent" && "$2" == "send" ]]; then
  echo "$3" >> "$SENDS"
  exit "${SEND_EXIT:-0}"
fi
if [[ "$1" == "agent" && "$2" == "spawn" ]]; then
  echo spawned >> "$SENDS"
  printf '%s\\n' "${SPAWN_OUT}"
  exit "${SPAWN_RC:-0}"
fi
if [[ "$1" == "watch" ]]; then
  # wait-workers watch add: simulate registration failure when WATCH_FAIL is set
  if [[ -n "${WATCH_FAIL:-}" ]]; then exit 1; fi
  exit 0
fi
exit 0
'''


class Stubs:
    def __init__(self, tmp):
        self.tmp = tmp
        self.list_file = os.path.join(tmp, "list.json")
        self.sends = os.path.join(tmp, "sends.log")
        codeman = os.path.join(tmp, "codeman")
        with open(codeman, "w") as fh:
            fh.write(CODEMAN_STUB)
        os.chmod(codeman, 0o755)
        self.codeman = codeman
        with open(self.list_file, "w") as fh:
            fh.write("[]")

    def env(self, **over):
        e = dict(os.environ)
        e.update({"CODMAN_BIN": self.codeman, "FAKE_LIST": self.list_file,
                  "SENDS": self.sends})
        e.update({k: str(v) for k, v in over.items()})
        return e

    def sessions(self, entries):
        with open(self.list_file, "w") as fh:
            json.dump(entries, fh)


class TestAutoDiscovery(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="o50disc-")
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.st = Stubs(self.tmp)
        self.sent = []
        self._o = (ww._send_wake, ww.HERE)
        ww._send_wake = self._capture
        self.sent_here = HERE
        ww.HERE = self.tmp

    def tearDown(self):
        ww._send_wake, ww.HERE = self._o

    def _capture(self, cid, msg, dry, self_id=None, persist=True):
        self.sent.append(msg)
        return True

    def _state(self, registered=("co",)):
        reg = {w: ({"role": "coordinator"} if w == "co" else {}) for w in registered}
        return {"registered": reg, "permission_flag": {}, "last_status": {},
                "delivered": {}, "busy_since": {}, "idle_streak": {}, "names": {},
                "lane_state": {}}

    def test_running_unregistered_lane_auto_added_and_observed(self):
        state = self._state()
        # co is registered, but a running lane "wX" is NOT in the list yet.
        sess = [{"id": "co", "name": "co", "status": "idle"},
                {"id": "wXunreg", "name": "unreg-lane", "status": "busy"}]
        ww._observe_pass(state, sess, "co", True, 100)
        # auto-added to registered in the same pass
        self.assertIn("wXunreg", state["registered"])
        self.assertEqual(state["registered"]["wXunreg"]["role"], "worker")
        self.assertTrue(state["registered"]["wXunreg"].get("coordinator") == "co")
        # DISCOVERED event fired
        self.assertTrue(any("DISCOVERED wXunreg" in m for m in self.sent))
        # and it is now observed (busy accounting ran -> last_status set)
        self.assertEqual(state["last_status"].get("wXunreg"), "busy")

    def test_discovery_idempotent_no_repeat_event(self):
        state = self._state()
        sess = [{"id": "co", "name": "co", "status": "idle"},
                {"id": "wY", "name": "y-lane", "status": "idle"}]
        self.sent = []
        ww._observe_pass(state, sess, "co", True, 100)   # first pass: discovered
        first = [m for m in self.sent if "DISCOVERED wY" in m]
        self.assertEqual(len(first), 1)
        self.sent = []
        ww._observe_pass(state, sess, "co", True, 110)   # already registered
        second = [m for m in self.sent if "DISCOVERED wY" in m]
        self.assertEqual(second, [])

    def test_gone_lane_not_readded(self):
        state = self._state()
        self.sent = []
        ww._observe_pass(state, [{"id": "co", "name": "co", "status": "idle"}],
                         "co", True, 100)   # wZ not in list -> stays undiscovered
        self.assertNotIn("wZ", state["registered"])
        self.assertFalse([m for m in self.sent if "DISCOVERED" in m])


class TestSpawnFailureExit(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="o50spawn-")
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.st = Stubs(self.tmp)

    def test_watch_add_failure_writes_sid_and_prints_to_stdout(self):
        # Build a minimal spawn-lane.sh environment: a ready pane + a watcher
        # that FAILS its registration (watcher pid never running).
        rdir = os.path.join(self.tmp, "cfg")
        os.makedirs(rdir)
        for f in ("flash2.json", "qwen36.json", "opencode.json"):
            with open(os.path.join(rdir, f), "w") as fh:
                fh.write("{}")
        home = os.path.join(self.tmp, "home")
        os.makedirs(os.path.join(home, ".config", "opencode"))
        for f in os.listdir(rdir):
            shutil.copy(os.path.join(rdir, f),
                        os.path.join(home, ".config", "opencode", f))
        wstate = os.path.join(self.tmp, "watcher-state.json")
        # watcher pid = a never-running pid so _is_running() is False -> watch add fails
        with open(wstate, "w") as fh:
            json.dump({"watcher": {"pid": 99999999}, "registered": {}}, fh)
        resf = os.path.join(self.tmp, "pool-reservations.json")
        with open(resf, "w") as fh:
            json.dump({}, fh)
        lock = os.path.join(self.tmp, "spawn.lock")
        env = self.st.env(
            WATCHER_STATE=wstate, POOL_RES_FILE=resf, SPAWN_LOCK=os.path.join(self.tmp, "spawn.lock"), HOME=home,
            SPAWN_OUT='{"sessionId":"55555555-2222-3333-4444-555555555555"}',
            PANE_FAIL="", WATCH_FAIL="",
            FAKE_PANE=os.path.join(self.tmp, "pane.txt"),
        )
        # ready pane for the spawned session (Flash build header + ctrl+p prompt)
        with open(os.path.join(self.tmp, "pane.txt"), "w") as fh:
            fh.write("Build @ Flash-4.5\nctrl+p commands\n")
        # Override tmux via a stub whose capture-pane returns the ready Flash
        # pane; the watcher registration (wait-workers watch add) fails because
        # the watcher pid (99999999) is not running -> cmd_watch_add returns 1.
        tmux = os.path.join(self.tmp, "tmux")
        with open(tmux, "w") as fh:
            fh.write('#!/usr/bin/env bash\n'
                     'shift 2\n'
                     '[[ "$1" == "capture-pane" ]] && cat "$FAKE_PANE" && exit 0\n'
                     'exit 0\n')
        os.chmod(tmux, 0o755)
        env["TMUX_BIN"] = tmux
        env["FAKE_PANE"] = os.path.join(self.tmp, "pane.txt")
        r = sh(["bash", os.path.join(HERE, "spawn-lane.sh"), "unreg-lane", "--flash"],
               env=env)
        self.assertEqual(r.returncode, 8, "rc={} stdout={} stderr={}".format(
            r.returncode, r.stdout, r.stderr))
        # sid-File written with the spawned session id
        sidfile = os.path.join(HERE, ".last-spawn-unreg-lane.sid")
        self.assertTrue(os.path.exists(sidfile))
        with open(sidfile) as fh:
            sid = fh.read().strip()
        self.assertEqual(sid, "55555555-2222-3333-4444-555555555555")
        os.remove(sidfile)
        # failure line present on STDOUT (tee'd to both streams)
        self.assertIn("WATCH-ADD-FAILED", r.stdout)


class TestBasicsUnmonitored(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp(prefix="o50unmono-")
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.st = Stubs(self.tmp)
        self.repo = os.path.join(self.tmp, "planrepo")
        os.makedirs(self.repo)
        sh(["git", "init", "--bare", self.repo])

    def test_unregistered_running_lane_flags_unmonitored(self):
        ts = int(time_mod := __import__("time").time() * 1000)
        self.st.sessions([
            {"id": "reg1", "name": "reg-lane", "status": "idle", "createdAt": ts},
            {"id": "unreg1", "name": "unreg-lane", "status": "busy", "createdAt": ts},
        ])
        with open(os.path.join(self.tmp, "pane.txt"), "w") as fh:
            fh.write("Build @ Ornith-9\nctrl+p\n")
        state = os.path.join(self.tmp, "watcher-state.json")
        # only reg1 is registered with the watcher
        with open(state, "w") as fh:
            json.dump({"registered": {"reg1": {}}, "busy_since": {},
                       "idle_streak": {}}, fh)
        plan = os.path.join(self.tmp, "coord-plan.md")
        with open(plan, "w") as fh:
            fh.write("- [x] done\n")
        e = self.st.env(WATCHER_STATE=state, BH_COORDINATOR_PLAN=plan,
                        BH_ROOT_DIR=self.tmp,
                        FAKE_PANE=os.path.join(self.tmp, "pane.txt"))
        # tmux stub: capture-pane returns a ready Flash pane (Build header +
        # ctrl+p); -L codeman args are stripped via shift 2.
        tmux = os.path.join(self.tmp, "tmux")
        with open(tmux, "w") as fh:
            fh.write('#!/usr/bin/env bash\n'
                     'shift 2\n'
                     '[[ "$1" == "capture-pane" ]] && cat "$FAKE_PANE" && exit 0\n'
                     'exit 0\n')
        os.chmod(tmux, 0o755)
        e["TMUX_BIN"] = tmux
        e["FAKE_PANE"] = os.path.join(self.tmp, "pane.txt")
        r = sh([sys.executable, os.path.join(HERE, "basics-check.py"), "--json"], env=e)
        d = json.loads(r.stdout)
        flags = d["flags"]
        unmono = [f for f in flags if f.startswith("UNMONITORED")]
        self.assertEqual(len(unmono), 1)
        self.assertIn("unreg-lane", unmono[0])
        # the registered lane must NOT be flagged; match on word boundary
        self.assertFalse(re.search(r"\breg-lane\b", unmono[0]))


if __name__ == "__main__":
    unittest.main(verbosity=2)
