import importlib.util
import os
import pathlib
import subprocess
import sys
import tempfile
import threading
import unittest

spec = importlib.util.spec_from_file_location("qualification", pathlib.Path(__file__).with_name("local-qualification.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class LocalQualificationTests(unittest.TestCase):
    def test_independent_checks_overlap_and_failure_remains_visible(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            rendezvous = (
                "import pathlib,sys,time; "
                "p=pathlib.Path(sys.argv[1]); p.touch(); "
                "other=pathlib.Path(sys.argv[2]); deadline=time.monotonic()+3; "
                "exec('while not other.exists() and time.monotonic()<deadline: time.sleep(.01)'); "
                "sys.exit(0 if other.exists() else 9)"
            )
            checks = [
                ("first", [sys.executable, "-c", rendezvous, str(root / "a"), str(root / "b")]),
                ("second", [sys.executable, "-c", rendezvous, str(root / "b"), str(root / "a")]),
                ("failure", [sys.executable, "-c", "raise SystemExit(7)"]),
            ]
            results = module.run_checks(checks, root, root, 2, os.environ.copy(), 5, threading.Event())
            by_name = {result["name"]: result for result in results}
            self.assertEqual("pass", by_name["first"]["status"])
            self.assertEqual("pass", by_name["second"]["status"])
            self.assertEqual("fail", by_name["failure"]["status"])
            self.assertEqual(7, by_name["failure"]["exit_code"])

    def test_timeout_and_cancellation_never_pass(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            command = [sys.executable, "-c", "import time; time.sleep(30)"]
            outcome = module.run_check("timeout", command, root, root, os.environ.copy(), .1, threading.Event())
            self.assertEqual("timeout", outcome["status"])
            self.assertEqual(124, outcome["exit_code"])
            cancelled = threading.Event()
            cancelled.set()
            outcome = module.run_check("cancelled", command, root, root, os.environ.copy(), 5, cancelled)
            self.assertEqual("cancelled", outcome["status"])

    def test_snapshot_preserves_dirty_inputs_and_isolated_git_index(self):
        with tempfile.TemporaryDirectory() as temporary:
            root, snapshot = pathlib.Path(temporary) / "source", pathlib.Path(temporary) / "snapshot"
            root.mkdir()
            module.git(root, "init", "--quiet")
            (root / "tracked").write_text("original\n")
            module.git(root, "add", "tracked")
            module.git(root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "fixture")
            (root / "tracked").write_text("staged\n")
            module.git(root, "add", "tracked")
            (root / "tracked").write_text("working\n")
            (root / "untracked").write_text("new input\n")
            (root / "link").symlink_to("tracked")
            before = module.git(root, "diff", "--cached", "--binary")
            receipt = module.snapshot_source(root, snapshot)
            self.assertTrue(receipt["working_tree_changes"])
            self.assertEqual("working\n", (snapshot / "tracked").read_text())
            self.assertEqual("new input\n", (snapshot / "untracked").read_text())
            self.assertTrue((snapshot / "link").is_symlink())
            self.assertEqual(before, module.git(root, "diff", "--cached", "--binary"))
            (root / "tracked").write_text("later edit\n")
            self.assertEqual("working\n", (snapshot / "tracked").read_text())


if __name__ == "__main__":
    unittest.main()
