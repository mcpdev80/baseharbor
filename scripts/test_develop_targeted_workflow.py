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
        self.assertIn("needs: [resolve, ecosystem_source, selected_gate, consumer_integration]", content)
        self.assertIn('"consumer_integration"):', content)
        self.assertIn("uses: ./.github/workflows/pre-release.yml", content)
        self.assertIn("develop_mode: true", content)
        self.assertIn("only_gates: integration/docker/remote-target,integration/podman/remote-target,integration/static/live-console", content)
        self.assertIn("if result != \"success\"", content)

    def test_release_workflow_reuses_actual_native_jobs_and_final(self):
        release = (Path(__file__).parents[1] / ".github/workflows/pre-release.yml").read_text()
        self.assertIn("develop_mode:", release)
        self.assertIn("scripts/develop-ecosystem-source.py", release)
        for job in ("integration_remote_target_docker:", "integration_remote_target_podman:",
                    "integration_live_console:", "  checks:"):
            self.assertIn(job, release)
        self.assertIn("assert job['result'] == expected", release)
        self.assertIn('test "$ONLY_GATES" = "integration/docker/remote-target,integration/podman/remote-target,integration/static/live-console"', release)

    def test_missing_consumer_sha_cannot_launch_dual_runtime(self):
        content = self.workflow
        self.assertIn('[[ "$DISPATCH_CONSOLE_REF" =~ ^[0-9a-f]{40}$ ]]', content)
        self.assertIn('[[ "$DISPATCH_CONNECTOR_REF" =~ ^[0-9a-f]{40}$ ]]', content)
        self.assertIn('[[ "$ref" =~ ^[0-9a-f]{40}$ ]]', content)


if __name__ == "__main__":
    unittest.main()
