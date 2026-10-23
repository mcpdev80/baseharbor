#!/usr/bin/env python3
"""Regression tests for the explicit PR candidate trigger contract.

This is a static safety check, not a replacement for GitHub event integration.
"""
import pathlib
import re
import unittest

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
        self.assertIn('python3 "$RUNNER_TEMP/pr-required-final.py"', source)

    def test_gc_pr_events_are_candidate_only(self):
        source = GC.read_text()
        self.assertIn("types: [opened, ready_for_review, labeled]", source)
        self.assertIn("github.event.label.name == 'ci:validate'", source)

if __name__ == "__main__":
    unittest.main()
