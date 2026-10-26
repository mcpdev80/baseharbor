import importlib.util
import json
import pathlib
import tempfile
import unittest


spec = importlib.util.spec_from_file_location(
    "evidence_attempts", pathlib.Path(__file__).with_name("select-evidence-attempts.py")
)
selector = importlib.util.module_from_spec(spec)
spec.loader.exec_module(selector)


class EvidenceAttemptsTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = pathlib.Path(self.temporary.name)

    def artifact(self, family, key, attempt, outcome, run="123"):
        path = self.root / family / f"{key}-{run}-{attempt}"
        path.mkdir(parents=True)
        (path / "manifest.json").write_text(json.dumps({"outcome": outcome}))
        return path

    def test_latest_success_retains_failed_history(self):
        old = self.artifact("gate-evidence", "docker-identity", 1, "failure")
        new = self.artifact("gate-evidence", "docker-identity", 2, "success")
        selected = selector.select_attempts(self.root, "123", ["gate-evidence"])
        self.assertEqual(selected[0][2:], (2, new))
        self.assertFalse(old.exists())
        retained = self.root / "superseded-evidence" / "gate-evidence" / old.name
        self.assertEqual(json.loads((retained / "manifest.json").read_text())["outcome"], "failure")

    def test_latest_failure_overrides_older_success(self):
        self.artifact("gate-evidence", "docker-identity", 1, "success")
        new = self.artifact("gate-evidence", "docker-identity", 3, "failure")
        selected = selector.select_attempts(self.root, "123", ["gate-evidence"])
        self.assertEqual(selected[0][3], new)
        self.assertEqual(json.loads((new / "manifest.json").read_text())["outcome"], "failure")

    def test_evidence_only_rerun_can_use_earlier_journey_attempt(self):
        journey = self.artifact("reference-journey-evidence", "journey", 1, "success")
        selected = selector.select_attempts(self.root, "123", ["reference-journey-evidence"])
        self.assertEqual(selected[0][3], journey)

    def test_attempts_are_numeric_and_gates_independent(self):
        self.artifact("gate-evidence", "docker-guided", 2, "failure")
        latest = self.artifact("gate-evidence", "docker-guided", 10, "success")
        sibling = self.artifact("gate-evidence", "podman-guided", 1, "success")
        selected = selector.select_attempts(self.root, "123", ["gate-evidence"])
        self.assertEqual({entry[3] for entry in selected}, {latest, sibling})

    def test_foreign_run_rejected_before_archiving_other_families(self):
        old = self.artifact("gate-evidence", "docker-guided", 1, "failure")
        self.artifact("gate-evidence", "docker-guided", 2, "success")
        self.artifact("reference-journey-evidence", "journey", 1, "success", run="999")
        with self.assertRaisesRegex(ValueError, "wrong source run"):
            selector.select_attempts(self.root, "123", ["gate-evidence", "reference-journey-evidence"])
        self.assertTrue(old.exists())
        self.assertFalse((self.root / "superseded-evidence").exists())

    def test_missing_or_empty_family_rejected(self):
        with self.assertRaises(ValueError):
            selector.select_attempts(self.root, "123", ["gate-evidence"])
        (self.root / "gate-evidence").mkdir()
        with self.assertRaisesRegex(ValueError, "empty evidence family"):
            selector.select_attempts(self.root, "123", ["gate-evidence"])

    def test_symlinked_artifact_rejected(self):
        foreign = self.root / "foreign"
        foreign.mkdir()
        family = self.root / "gate-evidence"
        family.mkdir()
        (family / "docker-guided-123-1").symlink_to(foreign, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, "unexpected evidence directory"):
            selector.select_attempts(self.root, "123", ["gate-evidence"])


if __name__ == "__main__":
    unittest.main()
