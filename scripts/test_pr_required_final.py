"""Normal workflow dependencies gate PR completion without an Actions API."""
import copy
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('final', Path(__file__).with_name('pr-required-final.py'))
final = importlib.util.module_from_spec(spec)
spec.loader.exec_module(final)


class FinalChecks(unittest.TestCase):
    def setUp(self):
        self.plan = {'schema': 'baseharbor.pr-gate-plan/v1', 'head_sha': 'a'*40,
                     'integration_sha': 'b'*40, 'coverage_status': 'resolved',
                     'unresolved_coverage': [], 'selected_gates': [],
                     'required_validation_checks': [{'id': 'documentation-audit'}]}
        self.needs = {name: {'result': 'success'} for name in final.BASE_JOBS}
        self.needs['pr_runtime_selected'] = {'result': 'skipped'}

    def test_validation_only(self):
        self.assertEqual([], final.verify(self.plan, self.needs, head='a'*40, integration='b'*40))

    def test_required_missing_failed_cancelled_skipped(self):
        for name in final.BASE_JOBS:
            for result in ('failure', 'cancelled', 'skipped', None):
                with self.subTest(name=name, result=result):
                    needs = copy.deepcopy(self.needs)
                    needs[name] = {'result': result}
                    self.assertTrue(final.verify(self.plan, needs))
            needs = copy.deepcopy(self.needs)
            del needs[name]
            self.assertTrue(final.verify(self.plan, needs))

    def test_runtime_requires_candidate_interface_matrix_and_pin(self):
        self.plan['selected_gates'] = ['atomic/docker/lifecycle', 'atomic/podman/lifecycle']
        self.plan['required_consumer_dependencies'] = ['core', 'demo']
        required = ('pr_candidate', 'pr_runner_interface', 'pr_runtime_selected', 'pr_demo_pin')
        self.assertTrue(final.verify(self.plan, self.needs))
        self.needs.update({name: {'result': 'success'} for name in required})
        self.assertEqual([], final.verify(self.plan, self.needs))
        for name in required:
            needs = copy.deepcopy(self.needs)
            needs[name]['result'] = 'skipped'
            self.assertTrue(final.verify(self.plan, needs))

    def test_public_contracts_and_private_dependency(self):
        self.plan['selected_gates'] = ['integration/static/public-contracts']
        self.plan['required_consumer_dependencies'] = ['core', 'console']
        self.needs['pr_public_contracts'] = {'result': 'success'}
        self.assertTrue(final.verify(self.plan, self.needs))
        self.needs['pr_consumer_pins'] = {'result': 'success'}
        self.assertEqual([], final.verify(self.plan, self.needs))

    def test_unknown_empty_unresolved_and_stale_are_not_success(self):
        for change in ({'selected_gates': ['ha/docker/data']},
                       {'required_validation_checks': []},
                       {'coverage_status': 'unresolved'},
                       {'unresolved_coverage': ['unknown source']},
                       {'selected_gates': [None]}):
            with self.subTest(change=change):
                self.assertTrue(final.verify({**self.plan, **change}, self.needs))
        self.assertTrue(final.verify(self.plan, self.needs, head='c'*40))
        self.assertTrue(final.verify(self.plan, self.needs, integration='c'*40))

    def test_optional_failure_is_not_ignored(self):
        self.needs['pr_demo_pin'] = {'result': 'failure'}
        self.assertTrue(final.verify(self.plan, self.needs))


if __name__ == '__main__':
    unittest.main()
