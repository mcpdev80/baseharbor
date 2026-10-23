#!/usr/bin/env python3
"""Regression tests for the explicit PR candidate trigger contract.

This is a static safety check, not a replacement for GitHub event integration.
"""
import importlib.util
import pathlib
import re
import unittest
from unittest.mock import patch

ROOT = pathlib.Path(__file__).resolve().parents[1]
CI = ROOT / ".github/workflows/ci.yml"
GC = ROOT / ".github/workflows/artifact-gc.yml"

class CandidateTriggerTests(unittest.TestCase):
    def test_ci_uses_only_candidate_pr_events(self):
        source = CI.read_text()
        self.assertRegex(source, r"(?m)^  pull_request:\n(?:    [^\n]*\n)*?    types: \[opened, ready_for_review, labeled\]")
        self.assertNotIn("synchronize", source.split("concurrency:", 1)[0])
        self.assertNotIn("pull_request_target:", source)

    def test_pr_jobs_are_guarded(self):
        source = CI.read_text().split("\njobs:\n", 1)[1]
        jobs = re.split(r"(?m)^  (?=[a-z_]+:\n)", source)
        for block in jobs:
            name = block.split(":", 1)[0].strip()
            if not name or not re.search(r"(?m)^    if: .*github.event_name == 'pull_request'", block):
                continue
            condition = next((line for line in block.splitlines() if line.startswith("    if: ")), "")
            with self.subTest(job=name):
                self.assertIn("ready_for_review", condition)
                self.assertIn("github.event.label.name == 'ci:validate'", condition)
                self.assertIn("github.event.action == 'opened'", condition)

    def test_finalcheck_rejects_stale_candidate(self):
        source = CI.read_text()
        self.assertIn('current.get("head", {}).get("sha") != expected', source)
        self.assertIn('current.get("base", {}).get("sha") != os.environ["EXPECTED_BASE"]', source)
        self.assertIn('current.get("state") != "open"', source)
        self.assertIn('"stale candidate: PR head changed', source)
        self.assertIn('"stale candidate: target base changed', source)
        self.assertIn('python3 "$RUNNER_TEMP/pr-required-final.py"', source)

    def test_real_pr_job_guards_match_candidate_event_matrix(self):
        source = CI.read_text()
        jobs = source.split("\n  pr_plan:\n", 1)[1]
        jobs = "\n  pr_plan:\n" + jobs
        blocks = re.split(r"(?m)^  (?=[a-z_]+:\n)", jobs)
        candidates = {
            "pr_plan", "pr_runner_interface", "pr_consumer_pins",
            "pr_demo_pin", "pr_candidate", "pr_focused", "pr_validations",
            "pr_public_contracts", "pr_runtime_selected", "pr_selected_final",
        }
        observed = set()
        for block in blocks:
            name = block.split(":", 1)[0].strip()
            if name not in candidates:
                continue
            observed.add(name)
            condition = next(
                (line.split("if:", 1)[1] for line in block.splitlines()
                 if line.startswith("    if: ")), ""
            )
            with self.subTest(job=name):
                self.assertIn("github.event.action == 'opened'", condition)
                self.assertIn("!github.event.pull_request.draft", condition)
                self.assertIn("github.event.action == 'ready_for_review'", condition)
                self.assertIn("github.event.action == 'labeled'", condition)
                self.assertIn("github.event.label.name == 'ci:validate'", condition)
                self.assertIn("github.event_name == 'pull_request'", condition)
                self.assertIn("github.base_ref == 'develop'", condition)
        self.assertEqual(observed, candidates)

    def test_workflow_policy_and_final_job_remain_present(self):
        source = CI.read_text()
        self.assertRegex(source, r"(?m)^  workflow_policy:\n    if: .*ci:validate")
        self.assertRegex(source, r"(?m)^  pr_selected_final:\n    name: PR Selected Checks")
        self.assertIn("always() && github.event_name == 'pull_request'", source)

    def test_workflow_policy_always_checks_candidate_contract(self):
        source = CI.read_text()
        self.assertIn("python3 -m unittest scripts/test_ci_candidate_trigger.py -v", source)
        self.assertIn("bash scripts/ci-workflow-policy.sh", source)
        self.assertIn('"$RUNNER_TEMP/bin/actionlint" -shellcheck=', source)

    def test_candidate_submitter_safety_contract(self):
        source = (ROOT / "scripts/submit-pr-candidate.py").read_text()
        self.assertIn('os.environ.get("GITHUB_ACTIONS") == "true"', source)
        self.assertIn('if not os.environ.get("GH_TOKEN"):', source)
        self.assertIn('pr["head"]["sha"] != expected_sha', source)
        self.assertIn('latest["head"]["sha"] != expected_sha', source)
        self.assertIn('pr["state"] != "open"', source)
        self.assertIn('pr["base"]["ref"] != "develop"', source)
        self.assertIn('if "ci:validate" in labels:', source)
        self.assertIn('gh("DELETE",', source)
        self.assertIn('gh("POST",', source)
        self.assertIn('data={"labels": ["ci:validate"]}', source)

    def test_submitter_runtime_behaviour(self):
        path = ROOT / "scripts/submit-pr-candidate.py"
        spec = importlib.util.spec_from_file_location("submit_pr_candidate", path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        sha = "a" * 40
        base = {
            "state": "open", "head": {"sha": sha},
            "base": {"ref": "develop"}, "labels": [],
        }

        with patch.dict("os.environ", {"GITHUB_ACTIONS": "false"}):
            with patch.object(module, "gh", side_effect=[base, base, None]) as api:
                module.publish("example/repo", 15, sha)
                self.assertEqual([call.args[0] for call in api.call_args_list],
                                 ["GET", "GET", "POST"])
                self.assertEqual(api.call_args_list[-1].kwargs["data"],
                                 {"labels": ["ci:validate"]})

            labeled = dict(base, labels=[{"name": "ci:validate"}])
            with patch.object(module, "gh", side_effect=[labeled, None, base, None]) as api:
                module.publish("example/repo", 15, sha)
                self.assertEqual([call.args[0] for call in api.call_args_list],
                                 ["GET", "DELETE", "GET", "POST"])

            stale = dict(base, head={"sha": "b" * 40})
            with patch.object(module, "gh", return_value=stale) as api:
                with self.assertRaisesRegex(RuntimeError, "stale candidate"):
                    module.publish("example/repo", 15, sha)
                self.assertEqual(api.call_count, 1)

            closed = dict(base, state="closed")
            with patch.object(module, "gh", return_value=closed):
                with self.assertRaisesRegex(RuntimeError, "not open"):
                    module.publish("example/repo", 15, sha)

            wrong_base = dict(base, base={"ref": "main"})
            with patch.object(module, "gh", return_value=wrong_base):
                with self.assertRaisesRegex(RuntimeError, "does not target develop"):
                    module.publish("example/repo", 15, sha)

            with patch.object(module, "gh", side_effect=[labeled, None, stale]) as api:
                with self.assertRaisesRegex(RuntimeError, "candidate changed"):
                    module.publish("example/repo", 15, sha)
                self.assertEqual(api.call_count, 3)

        with patch.dict("os.environ", {"GITHUB_ACTIONS": "true"}):
            with patch.object(module, "gh") as api:
                with self.assertRaisesRegex(RuntimeError, "outside GitHub Actions"):
                    module.publish("example/repo", 15, sha)
                api.assert_not_called()

    def test_gc_pr_events_are_candidate_only(self):
        source = GC.read_text()
        self.assertIn("types: [opened, ready_for_review, labeled]", source)
        self.assertIn("github.event.label.name == 'ci:validate'", source)

if __name__ == "__main__":
    unittest.main()
