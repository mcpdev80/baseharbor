import importlib.util
import json
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
