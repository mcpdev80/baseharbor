import importlib.util
import pathlib
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("metrics", pathlib.Path(__file__).with_name("ci-run-metrics.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class RunMetricsTests(unittest.TestCase):
    def test_known_queue_and_unknown_queue_are_distinct_and_attempts_retained(self):
        first = {"name": "docker/security", "id": 1, "run_attempt": 1, "conclusion": "failure", "created_at": "2026-10-05T12:00:00Z", "started_at": "2026-10-05T12:01:00Z", "completed_at": "2026-10-05T12:03:00Z"}
        second = dict(first, id=2, run_attempt=2, conclusion="success", created_at=None)
        with tempfile.TemporaryDirectory() as directory:
            result = module.collect([{"jobs": [first]}, {"jobs": [second]}], pathlib.Path(directory))
        self.assertEqual(2, len(result["jobs"]))
        self.assertEqual(60, result["jobs"][0]["queue_seconds"])
        self.assertIsNone(result["jobs"][1]["queue_seconds"])
        self.assertEqual(120, result["slowest_jobs"][0]["duration_seconds"])


if __name__ == "__main__":
    unittest.main()
