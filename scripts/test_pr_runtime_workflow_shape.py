"""Regression: the reusable runtime workflow always schedules exactly one gate job.

An empty/unknown reuse hint must execute the native gate, never suppress the
only workflow_call job. The gate always exists; only its steps are selected.
"""
from pathlib import Path
import re
import unittest

WORKFLOW = Path(__file__).resolve().parent.parent / ".github/workflows/pre-release-runtime-suite.yml"


class RuntimeWorkflowRoutingTests(unittest.TestCase):
    def test_exactly_one_unconditional_reusable_gate_job(self):
        text = WORKFLOW.read_text()
        jobs_section = text.split("jobs:\n", 1)[1].split("\n'on':", 1)[0]
        names = re.findall(r"^  ([a-z][a-z0-9_-]*):$", jobs_section, re.MULTILINE)
        self.assertEqual(["gate"], names)
        gate_header = jobs_section.split("    steps:\n", 1)[0]
        self.assertNotRegex(gate_header, r"(?m)^    if:")
        self.assertIn("reuse_from:", text)

    def test_reused_path_is_only_a_report_step(self):
        text = WORKFLOW.read_text()
        self.assertIn("if: inputs.reuse_from != ''", text)
        self.assertIn("This step does not execute native tests", text)

    def test_all_execution_steps_require_no_reuse(self):
        text = WORKFLOW.read_text().split("jobs:\n", 1)[1].split("\n'on':", 1)[0]
        blocks = re.split(r"(?m)^    - (?:name|uses): ", text)[1:]
        self.assertGreaterEqual(len(blocks), 15)
        self.assertIn("if: inputs.reuse_from != ''", blocks[0])
        for index, block in enumerate(blocks[1:], 1):
            with self.subTest(step=index):
                # Multiline conditions still begin with the same safe predicate.
                self.assertRegex(block, r"(?m)^      if: inputs.reuse_from == ''(?: && .+)?$")
                self.assertEqual(1, len(re.findall(r"(?m)^      if:", block)),
                                 "duplicate YAML if key in runtime step")


if __name__ == "__main__":
    unittest.main()
