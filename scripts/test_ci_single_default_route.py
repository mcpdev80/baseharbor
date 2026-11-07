"""Selection regressions for native single-control-plane acceptance."""
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SELECTOR = ROOT / "scripts" / "ci-single-default-impact.sh"


def selected(*paths):
    result = subprocess.run(
        ["bash", str(SELECTOR)],
        input="\n".join(paths) + "\n",
        text=True,
        capture_output=True,
        check=True,
    )
    return result.stdout.strip()


class SingleDefaultCIRouteTests(unittest.TestCase):
    def test_selects_only_relevant_native_control_plane_sources(self):
        for path in (
            "cmd/baha/runtime_ports_test.go",
            "cmd/baha/runtime_start.go",
            "cmd/baha/control_plane_single_native_ci_test.go",
            "internal/runtime/stack.go",
            "internal/runtime/stack_test.go",
            "internal/runtime/assets/compose.yaml",
            "internal/runtime/assets/compose-single.yaml",
        ):
            with self.subTest(path=path):
                self.assertEqual("true", selected(path))

    def test_does_not_launch_expensive_native_runtime_for_unrelated_changes(self):
        self.assertEqual(
            "false",
            selected(
                "docs/de/index.md",
                "README.md",
                "internal/development/source_snapshot_test.go",
                "cmd/baha/app_version_view_test.go",
                "cmd/baha/path_prompt_context.go",
                "internal/identityprovider/keycloak_files.go",
            ),
        )
        self.assertEqual("false", selected())

    def test_mixed_change_still_selects_native_acceptance(self):
        self.assertEqual(
            "true",
            selected("docs/index.md", "internal/runtime/assets/compose-single.yaml"),
        )

    def test_both_existing_runners_gate_develop_push_by_selector(self):
        for workflow in (
            ".github/workflows/docker-runtime-acceptance.yml",
            ".github/workflows/podman-acceptance.yml",
        ):
            with self.subTest(workflow=workflow):
                text = (ROOT / workflow).read_text(encoding="utf-8")
                self.assertIn("      - develop", text)
                self.assertIn("ci-single-default-impact.sh", text)
                self.assertIn("needs.select_develop.outputs.required == 'true'", text)
                self.assertIn("TestControlPlaneSingleDefaultNativeRuntimeInCI", text)
                self.assertIn("  single_default_native:", text)
                self.assertIn("github.event_name != 'push' || github.ref_name != 'develop'", text)
                self.assertIn("workflow_dispatch:", text)
                self.assertIn("workflow_call:", text)
                self.assertIn("      - 'runtime-validation/", text)


if __name__ == "__main__":
    unittest.main()
