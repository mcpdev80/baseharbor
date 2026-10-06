import importlib.util
import json
import pathlib
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("bindings", pathlib.Path(__file__).with_name("verify-attempt-bindings.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class AttemptBindingTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = pathlib.Path(self.directory.name)
        self.files = []
        for family, count in {"gate-evidence": 41, "adoption-evidence": 4, "v0421-ha-evidence": 8, "reference-journey-evidence": 2}.items():
            for number in range(count):
                # Unchanged gate results from attempt 1 remain usable at attempt 2.
                runtime = "podman" if family == "reference-journey-evidence" and number == 1 else "docker"
                logical = f"gate-{number}"
                prefixes = {"gate-evidence": f"pre-release-gate-{runtime}-{logical}-evidence", "adoption-evidence": f"pre-release-adoption-{logical}-evidence", "v0421-ha-evidence": f"pre-release-v0421-ha-{runtime}-{logical}-evidence", "reference-journey-evidence": f"pre-release-reference-journey-evidence-{runtime}"}
                path = self.root / family / f"{prefixes[family]}-123-1" / "manifest.json"
                path.parent.mkdir(parents=True)
                path.write_text(json.dumps({"workflow_run_id": "123", "workflow_run_attempt": 1, "candidate_sha": "a" * 40, "demo_ref": "b" * 40, "runtime": runtime, "gate": logical, "group": logical, "id": f"{runtime}/{logical}", "cleanup_outcome": "success"}))
                self.files.append(path)

    def verify(self):
        module.verify(self.root, "123", "a" * 40, "b" * 40, 2)

    def test_unchanged_older_attempts_remain_valid(self):
        self.verify()

    def test_wrong_pin_run_attempt_or_cleanup_is_rejected(self):
        path = self.files[0]
        original = json.loads(path.read_text())
        for field, value in [("candidate_sha", "c" * 40), ("demo_ref", "c" * 40), ("workflow_run_id", "124"), ("workflow_run_attempt", 2), ("workflow_run_attempt", True), ("cleanup_outcome", "failure"), ("runtime", "podman"), ("gate", "different"), ("id", "different")]:
            with self.subTest(field=field, value=value):
                path.write_text(json.dumps(dict(original, **{field: value})))
                with self.assertRaises(ValueError):
                    self.verify()
        path.write_text(json.dumps(original))
        path.unlink()
        with self.assertRaises(ValueError):
            self.verify()


if __name__ == "__main__":
    unittest.main()
