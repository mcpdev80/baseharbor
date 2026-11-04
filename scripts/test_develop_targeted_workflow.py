"""Regression contract for the existing targeted dual-runtime acceptance route."""
from pathlib import Path
import unittest


class TargetedDevelopWorkflowTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.workflow = (Path(__file__).parents[1] / ".github/workflows/targeted-demo-acceptance.yml").read_text()

    def test_source_preflight_is_required_for_dual_runtime(self):
        content = self.workflow
        self.assertIn("needs: [resolve, ecosystem_source]", content)
        self.assertIn("needs.ecosystem_source.result == 'success'", content)
        self.assertIn("scripts/develop-ecosystem-source.py", content)

    def test_both_runtime_paths_and_required_final(self):
        content = self.workflow
        self.assertIn('{"runtime":"docker","gate":"lifecycle"}', content)
        self.assertIn('{"runtime":"podman","gate":"lifecycle"}', content)
        self.assertIn("needs: [resolve, ecosystem_source, selected_gate]", content)
        self.assertIn("if result != \"success\"", content)

    def test_missing_consumer_sha_cannot_launch_dual_runtime(self):
        content = self.workflow
        self.assertIn('[[ "$DISPATCH_CONSOLE_REF" =~ ^[0-9a-f]{40}$ ]]', content)
        self.assertIn('[[ "$DISPATCH_CONNECTOR_REF" =~ ^[0-9a-f]{40}$ ]]', content)
        self.assertIn('[[ "$ref" =~ ^[0-9a-f]{40}$ ]]', content)


if __name__ == "__main__":
    unittest.main()
