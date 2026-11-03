"""The Docker selector must come from a ready, correctly scoped fixture."""
import importlib.util
import unittest
from pathlib import Path


spec = importlib.util.spec_from_file_location(
    "broker_project", Path(__file__).with_name("ci-broker-project.py"))
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


class BrokerProjectTests(unittest.TestCase):
    def test_accepts_current_runtime_project_from_ready_status(self):
        status = {"target": "ci", "application": "broker-demo", "environment": "dev",
                  "ready": True, "project": "bh-ci-broker-demo-dev"}
        self.assertEqual("bh-ci-broker-demo-dev", helper.project_from_status(status, "broker-demo"))

    def test_rejects_wrong_owner_unready_and_invalid_selector(self):
        status = {"target": "ci", "application": "broker-demo", "environment": "dev",
                  "ready": True, "project": "bh-ci-broker-demo-dev"}
        for changes in ({"target": "other"}, {"application": "other-demo"},
                        {"environment": "prod"}, {"ready": False},
                        {"project": "bad selector"}, {"project": ""}):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                helper.project_from_status(dict(status, **changes), "broker-demo")
