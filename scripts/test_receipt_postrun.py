"""Contract checks for automatic, commit-only receipts."""
import unittest
from pathlib import Path

import yaml


class CommitReceiptWorkflowTest(unittest.TestCase):
    def test_merged_pr_uses_trusted_source_and_scoped_token(self):
        workflow = yaml.safe_load(Path('.github/workflows/receipt-post-run.yml').read_text())
        trigger = workflow.get('on', workflow.get(True))
        self.assertEqual({'closed'}, set(trigger['pull_request_target']['types']))
        self.assertEqual(['develop'], trigger['pull_request_target']['branches'])
        job = workflow['jobs']['receipt']
        self.assertIn('github.event.pull_request.merged == true', job['if'])
        steps = job['steps']
        source = steps[0]
        self.assertEqual('${{ github.sha }}', source['with']['ref'])
        self.assertFalse(source['with']['persist-credentials'])
        self.assertFalse(any('head.sha' in str(step) for step in steps))
        publisher = steps[-1]
        self.assertIn('--commit-pr', publisher['run'])
        self.assertIn('PLANNING_RECEIPT_WRITE_TOKEN', publisher['env'])
        self.assertFalse(any('PLANNING_RECEIPT_WRITE_TOKEN' in str(step)
                             for step in steps[:-1]))
        self.assertNotIn('actions', workflow['permissions'])
        self.assertIn("github.ref == 'refs/heads/main'", job['if'])
        self.assertIn('workflow_dispatch', trigger)
        self.assertTrue(trigger['workflow_dispatch']['inputs']['pr_number']['required'])
        self.assertIn('inputs.pr_number', publisher['env']['PR_NUMBER'])


if __name__ == '__main__':
    unittest.main()
