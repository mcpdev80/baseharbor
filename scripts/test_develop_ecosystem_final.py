"""Failure injection for the final develop integration result."""
import importlib.util
from pathlib import Path
import unittest

file = Path(__file__).with_name("develop-ecosystem-final.py")
spec = importlib.util.spec_from_file_location("develop_final", file)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class DevelopFinalTests(unittest.TestCase):
    def setUp(self):
        self.jobs = {name: {"result": "success"} for name in module.REQUIRED}

    def test_all_required_success(self):
        self.assertTrue(module.require_success(self.jobs))

    def test_failure_injection(self):
        for name in module.REQUIRED:
            for state in ("failure", "skipped", "cancelled", "neutral", None):
                with self.subTest(name=name, state=state), self.assertRaises(ValueError):
                    module.require_success({**self.jobs, name: {"result": state}})

    def test_missing_jobs_or_invalid_json_shape(self):
        for name in module.REQUIRED:
            with self.subTest(name=name), self.assertRaises(ValueError):
                module.require_success({key: value for key, value in self.jobs.items() if key != name})
        for invalid in (None, [], "success", {}):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                module.require_success(invalid)


if __name__ == "__main__":
    unittest.main()
