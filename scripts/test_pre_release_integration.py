import importlib.util
import json
import os
from unittest.mock import patch
import pathlib
import sys
import tempfile
import unittest


spec = importlib.util.spec_from_file_location('integration', pathlib.Path(__file__).with_name('pre-release-integration.py'))
integration = importlib.util.module_from_spec(spec)
spec.loader.exec_module(integration)


class ContractCheckTests(unittest.TestCase):
    def check(self, events, code=0):
        with tempfile.TemporaryDirectory() as root:
            log = pathlib.Path(root) / 'go-test.jsonl'
            output = '\n'.join(json.dumps(event) for event in events)
            command = [sys.executable, '-c', f'import sys; print({output!r}); sys.exit({code})']
            return integration.checked_tests(command, log)


    def test_native_gate_rejects_foreign_or_unprepared_runtime(self):
        for environment in [{}, {'BASEHARBOR_TEST_RUNTIME': 'podman'}]:
            with patch.dict(os.environ, environment, clear=True):
                with self.assertRaises(ValueError):
                    integration.configure_runtime_environment('integration/docker/core-bootstrap')

    def test_native_gate_enables_all_required_runtime_regressions(self):
        with patch.dict(os.environ, {'BASEHARBOR_TEST_RUNTIME': 'docker'}, clear=True):
            self.assertTrue(integration.configure_runtime_environment('integration/docker/core-bootstrap'))
            for key in ['BASEHARBOR_CORE_BOOTSTRAP_ACCEPTANCE',
                        'BASEHARBOR_BUG_HUNT_LIFECYCLE_ACCEPTANCE',
                        'BASEHARBOR_TEMPLATE_BUILD_ACCEPTANCE']:
                self.assertEqual(os.environ[key], '1')
        with patch.dict(os.environ, {}, clear=True):
            self.assertFalse(integration.configure_runtime_environment('integration/static/public-contracts'))
            self.assertEqual(dict(os.environ), {})

    def test_coreless_gates_require_selected_runtime_and_enable_real_tests(self):
        for runtime in ['docker', 'podman']:
            gate = 'integration/' + runtime + '/coreless-lifecycle'
            with patch.dict(os.environ, {'BASEHARBOR_TEST_RUNTIME': runtime}, clear=True):
                self.assertTrue(integration.configure_runtime_environment(gate))
                self.assertEqual(os.environ['BASEHARBOR_CORELESS_ACCEPTANCE'], '1')
                self.assertEqual(os.environ['BASEHARBOR_IDENTITY_LIFECYCLE_ACCEPTANCE'], '1')
                self.assertEqual(os.environ['BASEHARBOR_BACKEND_LIFECYCLE_ACCEPTANCE'], '1')
                self.assertEqual(os.environ['BASEHARBOR_RENOVATE_RECOVERY_ACCEPTANCE'], '1')
            with patch.dict(os.environ, {}, clear=True):
                with self.assertRaises(ValueError):
                    integration.configure_runtime_environment(gate)

    def test_release_proof_requires_each_selective_fixture_on_both_engines(self):
        required = {'^TestCorelessFreshWorkloadRuntimeLifecycle$',
                    '^TestSelectiveIdentityRuntimeAddRemoveReAdd$',
                    '^TestSelectiveBackendRuntimeAddRemoveReAdd$',
                    '^TestRenovateVolumeRecoveryRuntimeAcceptance$',
                    '^TestApplicationCapabilityLifecycleExistingCore$'}
        for runtime in ['docker', 'podman']:
            selections = integration.CHECKS['integration/' + runtime + '/coreless-lifecycle']
            expressions = [args[args.index('-run') + 1] for args in selections]
            self.assertEqual(set(expressions), required)
            self.assertEqual(len(expressions), len(required))
            for expression in expressions:
                name = expression.removeprefix('^').removesuffix('$')
                package = selections[expressions.index(expression)][0].removeprefix('./')
                sources = pathlib.Path(__file__).resolve().parents[1].joinpath(package)
                self.assertTrue(any('func ' + name + '(' in p.read_text()
                                    for p in sources.glob('*_test.go')), name)

    def test_backend_subcase_skip_rejects_otherwise_passing_native_proof(self):
        with self.assertRaises(ValueError):
            self.check([{'Action': 'pass', 'Package': 'core',
                         'Test': 'TestSelectiveIdentityRuntimeAddRemoveReAdd'},
                        {'Action': 'pass', 'Package': 'core',
                         'Test': 'TestSelectiveBackendRuntimeAddRemoveReAdd/sql'},
                        {'Action': 'skip', 'Package': 'core',
                         'Test': 'TestSelectiveBackendRuntimeAddRemoveReAdd/cache'},
                        {'Action': 'pass', 'Package': 'core',
                         'Test': 'TestSelectiveBackendRuntimeAddRemoveReAdd'}])

    def test_zero_exit_without_tests_is_rejected(self):
        with self.assertRaises(ValueError):
            self.check([{'Action': 'pass', 'Package': 'empty/package'}])

    def test_skipped_required_checks_cannot_produce_success(self):
        with self.assertRaises(ValueError):
            self.check([{'Action': 'pass', 'Package': 'core', 'Test': 'TestPassed'},
                        {'Action': 'skip', 'Package': 'core', 'Test': 'TestNotRun'}])

    def test_partial_pass_before_process_failure_is_rejected(self):
        with self.assertRaises(ValueError):
            self.check([{'Action': 'pass', 'Package': 'core', 'Test': 'TestPassed'}], code=1)

    def test_passed_checks_retain_names_and_original_log_binding(self):
        result = self.check([{'Action': 'pass', 'Package': 'core', 'Test': 'TestRequired'}])
        self.assertEqual(result['passed'], ['core/TestRequired'])
        self.assertEqual(result['log'], 'go-test.jsonl')


if __name__ == '__main__':
    unittest.main()
