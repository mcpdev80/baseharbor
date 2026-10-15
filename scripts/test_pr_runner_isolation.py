"""#896 isolated runner lease security fixtures (no privileged runtimes)."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
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
        admission = patch.object(isolation, 'verify_github_admission', return_value=True)
        admission.start()
        self.addCleanup(admission.stop)

    def test_two_distinct_pr_lanes(self):
        first = isolation.prepare(REQUEST, self.root, "100", 1, 2, "mcpdev80/baseharbor", 932, "token")
        second = isolation.prepare(REQUEST, self.root, "101", 1, 2, "mcpdev80/baseharbor", 932, "token")
        self.assertNotEqual(first["isolation_root"], second["isolation_root"])
        self.assertNotEqual(first["network"], second["network"])
        self.assertNotEqual(first["volume_prefix"], second["volume_prefix"])
        for lease in (first, second):
            self.assertTrue((Path(lease["isolation_root"]) / "recovery").is_dir())
            self.assertTrue((Path(lease["isolation_root"]) / "secrets").is_dir())

    def test_concurrent_capacity_is_atomic(self):
        def acquire(n):
            try:
                return isolation.prepare(REQUEST, self.root, str(200+n), 1, 1, "mcpdev80/baseharbor", 932, "token")
            except RuntimeError:
                return None
        with ThreadPoolExecutor(max_workers=8) as pool:
            results = list(pool.map(acquire, range(8)))
        self.assertEqual(1, sum(item is not None for item in results))
        self.assertEqual(1, len(list(self.root.glob("pr-*"))))

    def test_fork_or_unadmitted_cannot_acquire(self):
        with patch.object(isolation, "verify_github_admission", side_effect=PermissionError("denied")):
            with self.assertRaises(PermissionError):
                isolation.prepare(REQUEST, self.root, "103", 1, 2, "mcpdev80/baseharbor", 932, "")
        self.assertEqual([], list(self.root.iterdir()))

    def test_capacity_cannot_clean_peer(self):
        first = isolation.prepare(REQUEST, self.root, "100", 1, 1, "mcpdev80/baseharbor", 932, "token")
        with self.assertRaises(RuntimeError):
            isolation.prepare(REQUEST, self.root, "101", 1, 1, "mcpdev80/baseharbor", 932, "token")
        self.assertTrue(Path(first["isolation_root"]).exists())

    def test_failure_cancel_cleanup_requires_ownership(self):
        lease = isolation.prepare(REQUEST, self.root, "100", 1, 1, "mcpdev80/baseharbor", 932, "token")
        with self.assertRaises(PermissionError):
            isolation.cleanup(self.root, lease["lane_id"], "101", 1)
        for status in ("failure", "cancelled"):
            result = isolation.cleanup(self.root, lease["lane_id"], "100", 1)
            self.assertEqual("pending-runtime-cleanup", result["state"])
            self.assertTrue(result["cleanup_required"])

    def test_authenticated_admission_rejects_spoofed_run(self):
        run = {'event':'pull_request','repository':{'id':10},'run_attempt':1,
               'head_sha':REQUEST['head_sha']}
        pr = {'base':{'ref':'develop','sha':REQUEST['target_base_sha']},
              'head':{'sha':REQUEST['head_sha'],'repo':{'id':10}},
              'state':'open','merge_commit_sha':REQUEST['integration_sha'],
              'user':{'id':22}}
        catalog = {'id':10,'owner':{'id':22}}
        with patch.object(isolation, 'github_json', side_effect=[run,pr,catalog]):
            self.assertTrue(isolation.verify_github_admission(
                REQUEST, "100", 1, "mcpdev80/baseharbor", 932, "token"))
        pr['user']['id'] = 55
        with patch.object(isolation, 'github_json', side_effect=[run,pr,catalog]):
            with self.assertRaises(PermissionError):
                isolation.verify_github_admission(
                    REQUEST, "100", 1, "mcpdev80/baseharbor", 932, "token")
        run['head_sha'] = 'f'*40
        with patch.object(isolation, 'github_json', side_effect=[run,pr,catalog]):
            with self.assertRaises(PermissionError):
                isolation.verify_github_admission(
                    REQUEST, "100", 1, "mcpdev80/baseharbor", 932, "token")

    def test_malformed_and_cross_runtime_request(self):
        bad = dict(REQUEST, runtime="podman")
        with self.assertRaises(ValueError):
            isolation.prepare(bad, self.root, "100", 1, 1, "mcpdev80/baseharbor", 932, "token")
        bad = dict(REQUEST, head_sha="main")
        with self.assertRaises(ValueError):
            isolation.prepare(bad, self.root, "100", 1, 1, "mcpdev80/baseharbor", 932, "token")


if __name__ == "__main__":
    unittest.main()
