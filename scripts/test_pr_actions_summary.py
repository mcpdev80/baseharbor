#!/usr/bin/env python3
"""Execute the actual Actions summary embedded in ci.yml with deterministic API fixtures."""
import io
import json
import os
from pathlib import Path
import re
import tempfile
import unittest
from unittest.mock import patch

WORKFLOW = Path(__file__).resolve().parents[1] / ".github/workflows/ci.yml"
HEADER = "      - name: Summarize checked, reused, unnecessary and blocked work"


def summary_source():
    content = WORKFLOW.read_text(encoding="utf-8")
    section = content.split(HEADER, 1)[1]
    block = section.split("          python3 - <<'PY'\n", 1)[1].split("          PY\n", 1)[0]
    return "\n".join(line[10:] if line.startswith("          ") else line
                     for line in block.splitlines()) + "\n"


def job(name, conclusion, seconds=4):
    return {
        "name": name, "conclusion": conclusion,
        "started_at": "2026-10-17T10:00:00Z",
        "completed_at": f"2026-10-17T10:00:{seconds:02d}Z",
        "html_url": "https://github.com/mcpdev80/baseharbor/actions/runs/123/job/42",
    }


class SummaryTests(unittest.TestCase):
    def render(self, jobs=None, plan=None, api_error=False, missing_plan=False):
        jobs = jobs if jobs is not None else []
        plan = plan if plan is not None else {
            "required_validation_checks": [{"id": "ci-policy"}],
            "selected_gates": [],
            "coverage_status": "resolved",
        }
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            if not missing_plan:
                (root / "pr-plan.json").write_text(json.dumps(plan), encoding="utf-8")
            output = root / "summary.md"
            env = {
                "GITHUB_REPOSITORY": "mcpdev80/baseharbor",
                "GITHUB_RUN_ID": "123",
                "GITHUB_RUN_ATTEMPT": "1",
                "GITHUB_STEP_SUMMARY": str(output),
                "GH_TOKEN": "fixture-token",
            }

            class Response:
                def __enter__(self):
                    return io.BytesIO(json.dumps({"jobs": jobs}).encode("utf-8"))
                def __exit__(self, *_):
                    return False

            def api(*_args, **_kwargs):
                if api_error:
                    raise OSError("API unavailable")
                return Response()

            previous = os.getcwd()
            try:
                os.chdir(root)
                with patch.dict(os.environ, env), patch("urllib.request.urlopen", side_effect=api):
                    exec(compile(summary_source(), str(WORKFLOW), "exec"), {})
            finally:
                os.chdir(previous)
            return output.read_text(encoding="utf-8")

    def test_checked_failure_and_duration_links(self):
        rendered = self.render([job("PR · Validation · ci-policy", "success", 7),
                                job("PR · Focused Python", "failure", 12)])
        self.assertIn("### Checked (1)", rendered)
        self.assertIn("### Failed (1)", rendered)
        self.assertIn("7s", rendered)
        self.assertIn("12s", rendered)
        self.assertIn("/actions/runs/123/job/42", rendered)

    def test_required_skip_is_blocked_not_unnecessary(self):
        rendered = self.render([job("PR · Validation · ci-policy", "skipped")])
        self.assertIn("### Blocked (", rendered)
        self.assertIn("### Not required (0)", rendered)

    def test_optional_skip_is_not_required(self):
        rendered = self.render([job("PR · Selected Runtime", "skipped")])
        self.assertIn("### Not required (1)", rendered)
        self.assertIn("### Blocked (", rendered)

    def test_missing_conclusion_is_blocked(self):
        rendered = self.render([job("PR · Focused Python", None)])
        self.assertIn("### Blocked (", rendered)

    def test_only_verified_reuse_is_listed(self):
        plan = {"required_validation_checks": [], "selected_gates": [], "coverage_status": "resolved",
                "verified_reused_evidence": [
                    {"verified": True, "url": "https://github.com/mcpdev80/baseharbor/actions/runs/55"},
                    {"verified": False, "url": "https://github.com/mcpdev80/baseharbor/actions/runs/66"},
                ]}
        rendered = self.render([job("Workflow policy", "success")], plan)
        self.assertIn("### Reused (1)", rendered)
        self.assertIn("/runs/55", rendered)
        self.assertNotIn("/runs/66", rendered)

    def test_empty_api_and_partial_plan_are_blocked(self):
        for kwargs in ({"jobs": []}, {"plan": {}}, {"plan": {"selected_gates": []}}):
            with self.subTest(kwargs=kwargs):
                rendered = self.render(**kwargs)
                self.assertIn("Result overview unavailable", rendered)
                self.assertIn("### Blocked (", rendered)

    def test_missing_plan_or_api_is_blocked(self):
        for kwargs in ({"missing_plan": True}, {"api_error": True}):
            with self.subTest(kwargs=kwargs):
                rendered = self.render(**kwargs)
                self.assertIn("Result overview unavailable", rendered)
                self.assertIn("### Blocked (", rendered)

    def test_unresolved_coverage_is_displayed_as_blocked(self):
        plan = {"required_validation_checks": [], "selected_gates": [],
                "coverage_status": "unresolved"}
        rendered = self.render([job("Workflow policy", "success")], plan)
        self.assertIn("Trusted plan coverage unresolved", rendered)
        self.assertIn("### Blocked (", rendered)

    def test_required_runtime_skip_is_blocked(self):
        plan = {"required_validation_checks": [], "coverage_status": "resolved",
                "selected_gates": ["atomic/docker/lifecycle"]}
        rendered = self.render([job("PR · Selected Runtime", "skipped")], plan)
        self.assertIn("### Blocked (", rendered)


    def test_verified_reuse_invalid_urls_and_duration_are_safe(self):
        base = "https://github.com/mcpdev80/baseharbor/actions/runs/"
        evidence = [
            {"verified": True, "url": "https://example.net/evidence", "duration_seconds": 42},
            {"verified": True, "url": "https://github.com.evil.net/mcpdev80/baseharbor/actions/runs/2"},
            {"verified": True, "url": base + "invalid"},
            {"verified": True, "url": base + "2?extra=1"},
            {"verified": True, "url": 17},
            {"verified": False, "url": base + "3"},
            {"verified": True, "url": base + "4", "duration_seconds": -5},
            {"verified": True, "url": base + "5", "duration_seconds": "12"},
            {"verified": True, "url": base + "6", "duration_seconds": 8},
        ]
        plan = {"required_validation_checks": [], "selected_gates": [],
                "coverage_status": "resolved", "verified_reused_evidence": evidence}
        rendered = self.render([job("Workflow policy", "success")], plan)
        self.assertIn("### Reused (3)", rendered)
        for run in ("4", "5", "6"):
            self.assertIn(base + run, rendered)
        self.assertIn("Plan-marked verified evidence](" + base + "6) · 8s", rendered)
        self.assertEqual(rendered.count("Plan-marked verified evidence"), 3)
        self.assertIn("duration unknown", rendered)
        self.assertNotIn("example.net", rendered)
        self.assertNotIn("evil.net", rendered)

    def test_missing_required_jobs_in_nonempty_api_are_blocked(self):
        plan = {"required_validation_checks": [{"id": "ci-policy"}],
                "selected_gates": [], "coverage_status": "resolved"}
        rendered = self.render([job("Workflow policy", "success")], plan)
        for missing in ("PR · Trusted Plan", "PR · Focused Python",
                        "PR · Validation · ci-policy"):
            self.assertIn("Missing required job: " + missing, rendered)
        self.assertIn("### Blocked (3)", rendered)
        self.assertNotIn("Missing required job: PR Selected Checks", rendered)
        self.assertIn("### Checked (1)", rendered)

    def test_required_runtime_and_contract_jobs_missing_from_api(self):
        plan = {"required_validation_checks": [], "coverage_status": "resolved",
                "selected_gates": ["atomic/podman/backup",
                                   "integration/static/public-contracts"]}
        rendered = self.render([job("Workflow policy", "success")], plan)
        self.assertIn("Missing required job: PR · Selected Runtime", rendered)
        self.assertIn("Missing required job: PR · Public Contracts", rendered)


if __name__ == "__main__":
    unittest.main()
