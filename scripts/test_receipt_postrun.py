"""Post-run artifact download: flat pr-plan.json via `gh run download`.

Scope: the CI workflow that fetches the producer's trusted-pr-plan artifact in
the post-run writer chain. The former form used the nonexistent `gh download
artifact` command and an unnecessary nested plan.zip indirection that was never
packed, so writers failed. The producer uploads pr-plan.json directly (flat),
so the workflow fetches it without any zip layer. These tests pin that shape.
"""
import os
import subprocess
import unittest
from pathlib import Path

WORKFLOW = Path(__file__).resolve().parent.parent / ".github" / "workflows" / "receipt-post-run.yml"


class PostRunDownloadTest(unittest.TestCase):
    def test_download_command_is_gh_run_download(self):
        self.assertTrue(WORKFLOW.exists(), "receipt-post-run.yml missing")
        text = WORKFLOW.read_text(encoding="utf-8")
        self.assertIn("gh run download", text)
        self.assertNotIn("gh download artifact", text)
        self.assertIn("pr-plan.json", text)
        self.assertNotIn("plan.zip", text)
        self.assertNotIn("--archive zip", text)

    def test_download_targets_flat_file_in_runner_temp(self):
        text = WORKFLOW.read_text(encoding="utf-8")
        self.assertIn('gh run download "$RUN_ID" --repo mcpdev80/baseharbor -n trusted-pr-plan -D "$RUNNER_TEMP/postrun-dl"', text)
        self.assertIn('test -f "$RUNNER_TEMP/postrun-dl/pr-plan.json"', text)

    def test_decode_reads_flat_plan_path(self):
        text = WORKFLOW.read_text(encoding="utf-8")
        self.assertIn('open(os.environ["PLAN_PATH"])', text)
        self.assertIn('PLAN_PATH="$RUNNER_TEMP/postrun-dl/pr-plan.json"', text)
        self.assertIn('rv.artifact_size_from_path(os.environ["PLAN_PATH"])', text)

    def test_decode_step_reads_artifact_bytes(self):
        text = WORKFLOW.read_text(encoding="utf-8")
        # The artifact payload is binary; decode the flat JSON via open() on the
        # downloaded path, never via a zip indirection.
        self.assertNotIn('zipfile.ZipFile("plan.zip")', text)
        self.assertNotIn('zipfile.ZipFile(buffer', text)

    def test_workflow_is_valid_yaml(self):
        try:
            import yaml
        except ImportError:
            self.skipTest("pyyaml not installed")
        data = yaml.safe_load(WORKFLOW.read_text(encoding="utf-8"))
        self.assertIn("extract_plan", data["jobs"])
        self.assertIn("publish_receipt", data["jobs"])


if __name__ == "__main__":
    unittest.main()
