"""Focused selector regressions for native Keycloak and PostgreSQL dependency."""
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "ci-keycloak-single-impact.sh"


def selected(*paths):
    proc = subprocess.run(
        ["bash", str(SCRIPT)],
        input="\n".join(paths) + "\n",
        text=True, capture_output=True, check=True,
    )
    return proc.stdout.strip()


class KeycloakSingleNativeRouteTests(unittest.TestCase):
    def test_keycloak_identity_changes_select_native_both_runtimes(self):
        for path in (
            "internal/identityprovider/keycloak_single.go",
            "internal/identityprovider/keycloak_core_sql.go",
            "internal/identityprovider/keycloak_single_native_ci_test.go",
            "cmd/baha/application_identity_placement_runtime_test.go",
            "cmd/baha/app_identity_production_runtime_test.go",
        ):
            with self.subTest(path=path):
                self.assertEqual("true", selected(path))

    def test_nonidentity_sql_and_docs_do_not_boot_keycloak(self):
        self.assertEqual("false", selected(
            "internal/runtime/assets/compose-single.yaml",
            "internal/runtime/openbao_storage.go",
            "cmd/baha/control_plane_single_native_ci_test.go",
            "docs/reference/core-resources.md",
        ))
        self.assertEqual("false", selected())

    def test_mixed_paths_still_select_identity(self):
        self.assertEqual(
            "true", selected("docs/index.md", "internal/identityprovider/keycloak_files.go"),
        )

    def test_explicit_keycloak_ha_rejects_skipped_or_absent_native_pass(self):
        workflow = (ROOT / ".github/workflows/targeted-integration.yml").read_text()
        self.assertIn("TestKeycloakHARuntimeFailoverAcceptanceInCI", workflow)
        self.assertIn("required native Keycloak HA test failed or skipped", workflow)
        self.assertIn("required native Keycloak HA test did not run and pass", workflow)
        self.assertIn("keycloak-ha-test.jsonl", workflow)

    def test_existing_runtime_workflows_reuse_narrow_develop_selector(self):
        for workflow in (
            ".github/workflows/docker-runtime-acceptance.yml",
            ".github/workflows/podman-acceptance.yml",
        ):
            with self.subTest(workflow=workflow):
                text = (ROOT / workflow).read_text()
                self.assertIn("      - develop", text)
                self.assertIn("ci-keycloak-single-impact.sh", text)
                self.assertIn("needs.select_develop.outputs.identity_required == 'true'", text)
                self.assertIn("  keycloak_single_native:", text)
                self.assertIn("TestKeycloakSingleNativeSQLInCI", text)
                self.assertIn("Required native Keycloak SQL acceptance skipped or failed", text)
                self.assertIn("workflow_call:", text)
                self.assertIn("workflow_dispatch:", text)
                self.assertIn("github.event_name != 'push' || github.ref_name != 'develop'", text)


if __name__ == "__main__":
    unittest.main()
