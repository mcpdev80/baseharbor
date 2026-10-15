"""#896 isolated runner lease security fixtures (no privileged runtimes)."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
from concurrent.futures import ThreadPoolExecutor

spec = importlib.util.spec_from_file_location(
    "isolation", Path(__file__).with_name("pr-runner-isolation.py"))
isolation = importlib.util.module_from_spec(spec)
spec.loader.exec_module(isolation)
REQUEST = {
    "schema": "baseharbor.pr-runtime-request/v1",
    "gate_id": "atomic/docker/backup-restore",
    "runtime": "docker",
    "target_base_sha": "a" * 40,
    "head_sha": "b" * 40,
    "merge_base_sha": "c" * 40,
    "integration_sha": "d" * 40,
    "consumer_pins": {"demo_sha": "1" * 40, "console_sha": "2" * 40,
                      "connector_sha": "3" * 40},
    "admission": "requires-trusted-isolated-runner",
    "evidence": "existing-verifier-only",
}


class IsolationFixtures(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)

    def test_two_distinct_pr_lanes(self):
        first = isolation.prepare(REQUEST, self.root, "run100", 1, 2, trusted=True)
        second = isolation.prepare(REQUEST, self.root, "run101", 1, 2, trusted=True)
        self.assertNotEqual(first["isolation_root"], second["isolation_root"])
        self.assertNotEqual(first["network"], second["network"])
        self.assertNotEqual(first["volume_prefix"], second["volume_prefix"])
        for lease in (first, second):
            self.assertTrue((Path(lease["isolation_root"]) / "recovery").is_dir())
            self.assertTrue((Path(lease["isolation_root"]) / "secrets").is_dir())

    def test_concurrent_capacity_is_atomic(self):
        def acquire(n):
            try:
                return isolation.prepare(REQUEST, self.root, "run" + str(n), 1, 1, trusted=True)
            except RuntimeError:
                return None
        with ThreadPoolExecutor(max_workers=8) as pool:
            results = list(pool.map(acquire, range(8)))
        self.assertEqual(1, sum(item is not None for item in results))
        self.assertEqual(1, len(list(self.root.glob("pr-*"))))

    def test_fork_or_unadmitted_cannot_acquire(self):
        with self.assertRaises(PermissionError):
            isolation.prepare(REQUEST, self.root, "fork", 1, 2)
        self.assertEqual([], list(self.root.iterdir()))

    def test_capacity_cannot_clean_peer(self):
        first = isolation.prepare(REQUEST, self.root, "run100", 1, 1, trusted=True)
        with self.assertRaises(RuntimeError):
            isolation.prepare(REQUEST, self.root, "run101", 1, 1, trusted=True)
        self.assertTrue(Path(first["isolation_root"]).exists())

    def test_failure_cancel_cleanup_requires_ownership(self):
        lease = isolation.prepare(REQUEST, self.root, "run100", 1, 1, trusted=True)
        with self.assertRaises(PermissionError):
            isolation.cleanup(self.root, lease["lane_id"], "run101", 1)
        for status in ("failure", "cancelled"):
            result = isolation.cleanup(self.root, lease["lane_id"], "run100", 1)
            self.assertEqual("pending-runtime-cleanup", result["state"])
            self.assertTrue(result["cleanup_required"])

    def test_malformed_and_cross_runtime_request(self):
        bad = dict(REQUEST, runtime="podman")
        with self.assertRaises(ValueError):
            isolation.prepare(bad, self.root, "run100", 1, 1, trusted=True)
        bad = dict(REQUEST, head_sha="main")
        with self.assertRaises(ValueError):
            isolation.prepare(bad, self.root, "run100", 1, 1, trusted=True)


if __name__ == "__main__":
    unittest.main()
