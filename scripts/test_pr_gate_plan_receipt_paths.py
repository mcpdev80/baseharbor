"""Exact path-mapping regressions for the minimal-receipts pilot sources."""
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('planner', ROOT / 'pr-gate-plan.py')
planner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(planner)
runner_spec = importlib.util.spec_from_file_location('check_runner', ROOT / 'pr-check-runner.py')
runner = importlib.util.module_from_spec(runner_spec)
runner_spec.loader.exec_module(runner)
MAP = json.loads((ROOT / 'pr-gate-map-v1.json').read_text())
GATES = ['integration/static/public-contracts']
REQ = {'releases': ['v0.4.26'],
       'gates': [{'id': g, 'required_for': ['v0.4.26']} for g in GATES]}
SCRIPTS = sorted(planner.RECEIPT_PILOT_SCRIPTS)
DOCUMENTS = ['docs/development/minimal-ci-receipts.md',
             'docs/de/development/minimal-ci-receipts.md']
DOC_IDS = {check['id'] for check in planner.DOC_CHECKS}
PILOT_CHECK = planner.RECEIPT_PILOT_CHECKS[0]['id']


def diff(changes):
    return {'target_base_sha': 'a' * 40, 'head_sha': 'b' * 40,
            'merge_base_sha': 'c' * 40, 'integration_sha': 'd' * 40,
            'broad_required': False, 'reasons': [], 'changes': changes}


def changes(*names, status='added'):
    return [{'status': status, 'path': path} for path in names]


class ReceiptPilotPathTests(unittest.TestCase):
    def check_ids(self, result):
        return {check['id'] for check in result['required_validation_checks']}

    def test_exact_pilot_inventory_resolves_via_planner_lane_and_docs_audit(self):
        result = planner.plan(diff(changes(*SCRIPTS, *DOCUMENTS)), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['unresolved_coverage'])
        self.assertEqual([], result['selected_gates'])
        self.assertFalse(result['broad_required'])
        self.assertEqual({PILOT_CHECK} | DOC_IDS, self.check_ids(result))

    def test_each_pilot_script_qualifies_with_the_planner_lane(self):
        for path in SCRIPTS:
            with self.subTest(path=path):
                result = planner.plan(diff(changes(path, status='modified')), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertEqual({PILOT_CHECK}, self.check_ids(result))
                self.assertEqual([], result['selected_gates'])

    def test_pilot_documents_qualify_with_the_existing_docs_audit(self):
        for path in DOCUMENTS:
            with self.subTest(path=path):
                result = planner.plan(diff(changes(path, status='modified')), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertEqual(DOC_IDS, self.check_ids(result))
                self.assertNotIn(PILOT_CHECK, self.check_ids(result))

    def test_unmapped_receipt_script_stays_unresolved(self):
        for path in ('scripts/receipt_publisher.py', 'scripts/receipt_chain_test.py',
                     'scripts/test_receipt_writer.py', 'evidence/receipts/pilot.json'):
            with self.subTest(path=path):
                result = planner.plan(diff(changes(path)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertIn(path, ' '.join(result['unresolved_coverage']))

    def test_pilot_check_id_is_executable_by_the_existing_runner(self):
        self.assertIn(PILOT_CHECK, runner.VALIDATIONS)
        plan = planner.plan(diff(changes(*SCRIPTS)), REQ, MAP)
        self.assertEqual([{'check': check_id} for check_id in sorted(
            {row['id'] for row in plan['required_validation_checks']})],
            runner.matrix(plan)['include'])

    def test_pilot_sources_select_no_native_gate_and_no_new_workflow(self):
        result = planner.plan(diff(changes(*SCRIPTS, *DOCUMENTS)), REQ, MAP)
        self.assertEqual([], result['selected_gates'])
        ids = self.check_ids(result)
        self.assertNotIn('ci-actionlint', ids)
        self.assertNotIn('ci-policy', ids)


if __name__ == '__main__':
    unittest.main()
