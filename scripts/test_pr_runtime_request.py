"""Contract fixture tests; no native runtime is started."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('runtime_request', Path(__file__).with_name('pr-runtime-request.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
PLAN = {'schema':'baseharbor.pr-gate-plan/v1','target_base_sha':'a'*40,
        'head_sha':'b'*40,'merge_base_sha':'c'*40,'integration_sha':'d'*40,
        'selected_gates':['atomic/docker/backup-restore','atomic/static/init']}
CATALOG = {'gates':[{'id':name,'required_for':['v0.4.26']} for name in PLAN['selected_gates']]}
PINS = {'demo_sha':'1'*40,'console_sha':'2'*40,'connector_sha':'3'*40}


class RequestTests(unittest.TestCase):
    def test_exact_selected_runtime_gate(self):
        rows = module.requests(PLAN, CATALOG, PINS)
        self.assertEqual(1, len(rows))
        self.assertEqual('atomic/docker/backup-restore', rows[0]['gate_id'])
        self.assertEqual('d'*40, rows[0]['integration_sha'])
        self.assertEqual('existing-verifier-only', rows[0]['evidence'])
        self.assertEqual('requires-trusted-isolated-runner', rows[0]['admission'])

    def test_no_runtime_is_not_a_fake_result(self):
        plan = dict(PLAN, selected_gates=['atomic/static/init'])
        self.assertEqual([], module.requests(plan, CATALOG, PINS))

    def test_missing_integration_or_pins_blocks(self):
        for plan, pins in [(dict(PLAN, integration_sha=None), PINS),
                           (PLAN, dict(PINS, demo_sha='main')),
                           (dict(PLAN, head_sha='bad'), PINS)]:
            with self.subTest(plan=plan, pins=pins):
                with self.assertRaises(ValueError):
                    module.requests(plan, CATALOG, pins)

    def test_extra_gate_or_duplicate_blocks(self):
        for gates in [['atomic/docker/backup-restore', 'other/runtime'],
                      ['atomic/docker/backup-restore'] * 2]:
            with self.subTest(gates=gates):
                with self.assertRaises(ValueError):
                    module.requests(dict(PLAN, selected_gates=gates), CATALOG, PINS)


if __name__ == '__main__':
    unittest.main()
