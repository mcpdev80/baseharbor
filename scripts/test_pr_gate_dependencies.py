import importlib.util
from pathlib import Path
import unittest
import json

spec = importlib.util.spec_from_file_location("dependencies", Path(__file__).with_name("pr-gate-dependencies.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
CATALOG = {"gates": [
    {"id": "atomic/docker/backup-restore", "required_for": ["v0.4.26"], "dependencies": ["core", "demo"]},
    {"id": "integration/static/live-console", "required_for": ["v0.4.26"], "dependencies": ["core", "demo", "console", "connector"]},
    {"id": "atomic/static/init", "required_for": ["v0.4.26"], "dependencies": ["core", "demo"]},
]}

class Dependencies(unittest.TestCase):
    def test_actual_release_backup_requirements(self):
        catalog=json.loads(Path(__file__).with_name("release-requirements.json").read_text())
        for runtime in ("docker", "podman"):
            with self.subTest(runtime=runtime):
                plan={"selected_gates":["atomic/"+runtime+"/backup-restore"]}
                result=module.dependency_mode(plan,catalog)
                self.assertEqual({"dependencies":["core","demo"],"mode":"demo"},result)

    def test_backup_is_core_demo_only(self):
        result = module.dependency_mode({"selected_gates": ["atomic/docker/backup-restore"]}, CATALOG)
        self.assertEqual("demo", result["mode"])
        self.assertEqual(["core", "demo"], result["dependencies"])

    def test_private_consumers_only_when_required(self):
        result = module.dependency_mode({"selected_gates": ["integration/static/live-console"]}, CATALOG)
        self.assertEqual("ecosystem", result["mode"])

    def test_unknown_never_exempts(self):
        with self.assertRaises(ValueError):
            module.dependency_mode({"selected_gates": ["unlisted/gate"]}, CATALOG)

    def test_missing_dependency_never_exempts(self):
        with self.assertRaises(ValueError):
            module.dependency_mode({"selected_gates": ["atomic/static/init"]},
                                   {"gates": [{"id":"atomic/static/init","required_for":["v0.4.26"],"dependencies":[]}]})

if __name__ == "__main__":
    unittest.main()
