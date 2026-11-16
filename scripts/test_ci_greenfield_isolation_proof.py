"""Regression tests for the fail-closed 0427-133 native proof verifier."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

MODULE_PATH = Path(__file__).with_name("ci-greenfield-isolation-proof.py")
SPEC = importlib.util.spec_from_file_location("native_greenfield_proof", MODULE_PATH)
proof = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(proof)

SHA = "a" * 40


class NativeGreenfieldProofTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.report = Path(self.directory.name) / "go.jsonl"
        Path(str(self.report) + ".sha").write_text(SHA + "\n", encoding="utf-8")

    def write_events(self, actions=("run", "pass"), package_pass=True, extra=()):
        entries = [{"Package": proof.PACKAGE, "Test": proof.TEST, "Action": action} for action in actions]
        entries.extend(extra)
        if package_pass:
            entries.append({"Package": proof.PACKAGE, "Action": "pass"})
        self.report.write_text(
            "".join(json.dumps(e) + "\n" for e in entries), encoding="utf-8"
        )

    def check(self):
        proof.verify_runtime(self.report, Path(str(self.report) + ".sha"), SHA, "docker")

    def test_exact_pass(self):
        self.write_events()
        self.check()

    def test_missing_native_test_rejected(self):
        self.write_events(actions=())
        with self.assertRaisesRegex(ValueError, "requires one real PASS"):
            self.check()

    def test_skipped_native_test_rejected(self):
        self.write_events(actions=("run", "skip"))
        with self.assertRaisesRegex(ValueError, "requires one real PASS"):
            self.check()

    def test_test_failure_rejected(self):
        self.write_events(actions=("run", "fail"))
        with self.assertRaisesRegex(ValueError, "requires one real PASS"):
            self.check()

    def test_package_failure_rejected(self):
        self.write_events(extra=({"Package": proof.PACKAGE, "Action": "fail"},))
        with self.assertRaisesRegex(ValueError, "contains a failure"):
            self.check()

    def test_package_teardown_missing_rejected(self):
        self.write_events(package_pass=False)
        with self.assertRaisesRegex(ValueError, "package teardown"):
            self.check()

    def test_wrong_sha_rejected(self):
        self.write_events()
        Path(str(self.report) + ".sha").write_text("b" * 40, encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "SHA mismatch"):
            self.check()

    def test_missing_report_rejected(self):
        with self.assertRaisesRegex(ValueError, "missing"):
            self.check()

    def test_malformed_report_rejected(self):
        self.report.write_text("{broken\n", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "invalid Go JSON"):
            self.check()


if __name__ == "__main__":
    unittest.main()
