"""#896 isolated runner lease security fixtures (no privileged runtimes)."""
import importlib.util
from pathlib import Path
import tempfile
import json
import os
import copy
import unittest
from unittest.mock import patch
from types import SimpleNamespace
from concurrent.futures import ThreadPoolExecutor

spec = importlib.util.spec_from_file_location(
    "isolation", Path(__file__).with_name("pr-runner-isolation.py"))
isolation = importlib.util.module_from_spec(spec)
spec.loader.exec_module(isolation)
ORIGINAL_VERIFY = isolation.verify_hosted_unprivileged_admission
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
}


class IsolationFixtures(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        admission = patch.object(isolation, 'verify_hosted_unprivileged_admission', return_value=True)
        admission.start()
        self.addCleanup(admission.stop)

    def test_two_distinct_pr_lanes(self):
        first = isolation.prepare(REQUEST, self.root, "100", 1, 2, "mcpdev80/baseharbor", 932)
        second = isolation.prepare(REQUEST, self.root, "101", 1, 2, "mcpdev80/baseharbor", 932)
        self.assertNotEqual(first["isolation_root"], second["isolation_root"])
        self.assertNotEqual(first["network"], second["network"])
        self.assertNotEqual(first["volume_prefix"], second["volume_prefix"])
        for lease in (first, second):
            self.assertTrue((Path(lease["isolation_root"]) / "recovery").is_dir())
            self.assertTrue((Path(lease["isolation_root"]) / "secrets").is_dir())

    def test_concurrent_capacity_is_atomic(self):
        def acquire(n):
            try:
                return isolation.prepare(REQUEST, self.root, str(200+n), 1, 1, "mcpdev80/baseharbor", 932)
            except RuntimeError:
                return None
        with ThreadPoolExecutor(max_workers=8) as pool:
            results = list(pool.map(acquire, range(8)))
        self.assertEqual(1, sum(item is not None for item in results))
        self.assertEqual(1, len(list(self.root.glob("pr-*"))))

    def test_fork_or_unadmitted_cannot_acquire(self):
        with patch.object(isolation, "verify_hosted_unprivileged_admission", side_effect=PermissionError("denied")):
            with self.assertRaises(PermissionError):
                isolation.prepare(REQUEST, self.root, "103", 1, 2, "mcpdev80/baseharbor", 932)
        self.assertEqual([], list(self.root.iterdir()))

    def test_capacity_cannot_clean_peer(self):
        first = isolation.prepare(REQUEST, self.root, "100", 1, 1, "mcpdev80/baseharbor", 932)
        with self.assertRaises(RuntimeError):
            isolation.prepare(REQUEST, self.root, "101", 1, 1, "mcpdev80/baseharbor", 932)
        self.assertTrue(Path(first["isolation_root"]).exists())

    def test_failure_cancel_cleanup_requires_ownership(self):
        lease = isolation.prepare(REQUEST, self.root, "100", 1, 1, "mcpdev80/baseharbor", 932)
        with self.assertRaises(PermissionError):
            isolation.cleanup(self.root, lease["lane_id"], "101", 1)
        for status in ("failure", "cancelled"):
            result = isolation.cleanup(self.root, lease["lane_id"], "100", 1)
            self.assertEqual("pending-runtime-cleanup", result["state"])
            self.assertTrue(result["cleanup_required"])

    def test_verified_final_cleanup_releases_only_owned_lane(self):
        first = isolation.prepare(REQUEST, self.root, "100", 1, 2, "mcpdev80/baseharbor", 932)
        second = isolation.prepare(REQUEST, self.root, "101", 1, 2, "mcpdev80/baseharbor", 932)
        absent = SimpleNamespace(returncode=0, stderr=b"", stdout=b"")
        with patch.object(isolation.subprocess, "run", return_value=absent):
            receipt = isolation.finalize_cleanup(self.root, first["lane_id"], "100", 1)
        self.assertEqual("verified-empty-and-released", receipt["state"])
        self.assertFalse(Path(first["isolation_root"]).exists())
        self.assertTrue(Path(second["isolation_root"]).exists())
        third = isolation.prepare(REQUEST, self.root, "102", 1, 2, "mcpdev80/baseharbor", 932)
        self.assertNotEqual(third["lane_id"], second["lane_id"])

    def test_cleanup_never_releases_unverified_runtime(self):
        lease = isolation.prepare(REQUEST, self.root, "100", 1, 1, "mcpdev80/baseharbor", 932)
        cases = [SimpleNamespace(returncode=0, stderr=b"", stdout=("bh-" + lease["lane_id"][:20] + "-container").encode()),
                 SimpleNamespace(returncode=125, stderr=b"permission denied", stdout=b"")]
        for result in cases:
            with self.subTest(result=result):
                with patch.object(isolation.subprocess, "run", return_value=result):
                    with self.assertRaises(RuntimeError):
                        isolation.finalize_cleanup(self.root, lease["lane_id"], "100", 1)
                self.assertTrue(Path(lease["isolation_root"]).exists())
        with patch.object(isolation.subprocess, "run", side_effect=OSError("absent binary")):
            with self.assertRaises(RuntimeError):
                isolation.finalize_cleanup(self.root, lease["lane_id"], "100", 1)

    def test_hosted_admission_uses_event_identity_without_api(self):
        with tempfile.TemporaryDirectory() as temporary:
            event_path = Path(temporary) / "event.json"
            payload = {'repository': {'id': 1360667669, 'full_name': 'mcpdev80/baseharbor',
                                      'owner': {'id': 22}},
                       'pull_request': {'number': 932, 'state': 'open',
                                        'base': {'ref': 'develop', 'sha': REQUEST['target_base_sha']},
                                        'head': {'sha': REQUEST['head_sha'], 'repo': {'id': 1360667669}},
                                        'user': {'id': 22}}}
            environment = {'GITHUB_ACTIONS': 'true', 'RUNNER_ENVIRONMENT': 'github-hosted',
                           'GITHUB_EVENT_NAME': 'pull_request',
                           'GITHUB_REPOSITORY': 'mcpdev80/baseharbor',
                           'GITHUB_REPOSITORY_ID': '1360667669',
                           'GITHUB_RUN_ID': '100', 'GITHUB_RUN_ATTEMPT': '1',
                           'GITHUB_REF': 'refs/pull/932/merge',
                           'GITHUB_SHA': REQUEST['integration_sha'],
                           'GITHUB_EVENT_PATH': str(event_path)}
            event_path.write_text(json.dumps(payload))
            with patch.dict(os.environ, environment, clear=True):
                self.assertTrue(ORIGINAL_VERIFY(REQUEST, '100', 1, 'mcpdev80/baseharbor', 932))
            for changes in ({'RUNNER_ENVIRONMENT': 'self-hosted'},
                            {'GITHUB_SHA': 'f' * 40}, {'GITHUB_EVENT_NAME': 'workflow_dispatch'},
                            {'GITHUB_RUN_ATTEMPT': '2'}, {'GITHUB_REPOSITORY': 'foreign/repo'}):
                with self.subTest(environment=changes), patch.dict(os.environ, {**environment, **changes}, clear=True):
                    with self.assertRaises(PermissionError):
                        ORIGINAL_VERIFY(REQUEST, '100', 1, 'mcpdev80/baseharbor', 932)
            cases = []
            for field, value in [('number', 933), ('state', 'closed'), ('user', {'id': 55}),
                                 ('base', {'ref': 'main', 'sha': REQUEST['target_base_sha']}),
                                 ('head', {'sha': REQUEST['head_sha'], 'repo': {'id': 99}})]:
                denied = copy.deepcopy(payload)
                denied['pull_request'][field] = value
                cases.append(denied)
            for denied in cases:
                event_path.write_text(json.dumps(denied))
                with self.subTest(payload=denied), patch.dict(os.environ, environment, clear=True):
                    with self.assertRaises(PermissionError):
                        ORIGINAL_VERIFY(REQUEST, '100', 1, 'mcpdev80/baseharbor', 932)
            event_path.unlink()
            event_path.symlink_to(Path(temporary) / 'missing')
            with patch.dict(os.environ, environment, clear=True), self.assertRaises(PermissionError):
                ORIGINAL_VERIFY(REQUEST, '100', 1, 'mcpdev80/baseharbor', 932)

    def test_demo_only_request_is_accepted_without_private_consumers(self):
        minimal=dict(REQUEST, consumer_pins={"demo_sha":"1"*40})
        runtime,gate=isolation.validate(minimal,"100",1,1)
        self.assertEqual(("docker","atomic/docker/backup-restore"),(runtime,gate))
        for pins in ({"demo_sha":"1"*40,"unexpected":"2"*40},
                     {"console_sha":"2"*40},
                     {"demo_sha":"main"}):
            with self.subTest(pins=pins):
                with self.assertRaises(ValueError):
                    isolation.validate(dict(REQUEST,consumer_pins=pins),"100",1,1)

    def test_malformed_and_cross_runtime_request(self):
        bad = dict(REQUEST, runtime="podman")
        with self.assertRaises(ValueError):
            isolation.prepare(bad, self.root, "100", 1, 1, "mcpdev80/baseharbor", 932)
        bad = dict(REQUEST, head_sha="main")
        with self.assertRaises(ValueError):
            isolation.prepare(bad, self.root, "100", 1, 1, "mcpdev80/baseharbor", 932)


if __name__ == "__main__":
    unittest.main()
