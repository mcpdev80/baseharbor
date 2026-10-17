"""Check the trusted dynamic validation interface without running native gates."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('runner', Path(__file__).with_name('pr-check-runner.py'))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)
PLAN = {'required_validation_checks': [{'id': 'mkdocs-en-strict'}, {'id': 'documentation-audit'}],
        'target_base_sha': 'a'*40, 'head_sha': 'b'*40, 'merge_base_sha': 'c'*40,
        'integration_sha': None}


class RunnerTests(unittest.TestCase):
    def test_only_required_validations_are_in_matrix(self):
        self.assertEqual([{'check': 'mkdocs-en-strict'}, {'check': 'documentation-audit'}],
                         runner.matrix(PLAN)['include'])

    def test_inspect_validation_ids_are_runnable(self):
        for check_id in ('inspect-gofmt', 'inspect-package-tests', 'inspect-vet',
                         'inspect-consumer-tests', 'inspect-consumer-build'):
            with self.subTest(check_id=check_id):
                self.assertEqual([{'check': check_id}],
                                 runner.matrix({'required_validation_checks': [{'id': check_id}]})['include'])

    def test_planner_emitted_ids_are_never_rejected_by_runner(self):
        # Coupling guard for the repository-inspect lane: every validation id the
        # trusted planner can emit must be whitelisted by the runner, otherwise a
        # reconcile PR would fail closed forever with no actionable lane.
        import importlib.util
        pspec = importlib.util.spec_from_file_location('planner', Path(__file__).with_name('pr-gate-plan.py'))
        planner = importlib.util.module_from_spec(pspec)
        pspec.loader.exec_module(planner)
        emitted = set()
        for group in (planner.DOC_CHECKS, planner.CODE_CHECKS, planner.CLI_CHECKS,
                      planner.WIZARD_BASE_CHECKS, planner.CAPABILITY_CHECKS,
                      planner.CI_CHECKS, planner.WORKFLOW_SYNTAX_CHECKS,
                      planner.INSPECT_CHECKS):
            emitted |= {check['id'] for check in group}
        self.assertEqual(set(), emitted - runner.VALIDATIONS)

    def test_empty_validation_never_claims_pass(self):
        self.assertEqual([{'check': 'not-required'}],
                         runner.matrix({'required_validation_checks': []})['include'])

    def test_unknown_and_duplicate_check_are_rejected(self):
        for checks in ([{'id': 'unknown'}], [{'id': 'mkdocs-de-strict'}, {'id': 'mkdocs-de-strict'}]):
            with self.subTest(checks=checks):
                with self.assertRaises(ValueError):
                    runner.matrix({'required_validation_checks': checks})

    def test_static_failure_never_authenticates_validation(self):
        for status in ('failure', 'skipped', 'cancelled', 'timed_out', 'blocked'):
            with self.subTest(status=status):
                results = runner.report(PLAN, status, 'success')
                self.assertTrue(all(x['conclusion'] == 'blocked' for x in results['results']))

    def test_no_runtime_gate_can_be_forged_from_validation_job(self):
        result = runner.report(PLAN, 'success', 'success')
        self.assertEqual({'validation'}, {row['kind'] for row in result['results']})


    def test_all_wizard_profile_ids_are_runnable(self):
        for check_id in runner.WIZARD_TEST_FAMILIES:
            with self.subTest(check_id=check_id):
                self.assertEqual([{'check': check_id}],
                                 runner.matrix({'required_validation_checks': [{'id': check_id}]})['include'])

    def test_first_wave_executes_real_go_tests_and_discovers_matches(self):
        tests = {
            'wizard-app-edit-load-tests': 'TestAppEditLoadRoundTrip',
            'wizard-capability-selection-tests': 'TestAppCapabilitySelectionZero',
            'wizard-footprint-presenter-tests': 'TestAppFootprintPresenterUnknown',
            'wizard-change-presentation-tests': 'TestAppChangePreviewPartial',
        }
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'go.mod').write_text('module example.com/wizardfixture\n\ngo 1.25\n')
            pkg = root / 'cmd' / 'baha'
            pkg.mkdir(parents=True)
            (pkg / 'wizard_profiles_test.go').write_text(
                'package main\n\nimport "testing"\n\n' +
                '\n'.join('func %s(t *testing.T) {}' % name for name in list(tests.values()) + ['TestAppChangeResultPresenterPartial']) + '\n')
            for check_id, test_name in tests.items():
                with self.subTest(check_id=check_id):
                    matched = runner.execute_validation(check_id, root)
                    self.assertIn(test_name, matched)

    def test_zero_matching_tests_fail_closed_even_when_go_test_would_exit_zero(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'go.mod').write_text('module example.com/wizardfixture\n\ngo 1.25\n')
            pkg = root / 'cmd' / 'baha'
            pkg.mkdir(parents=True)
            (pkg / 'other_test.go').write_text(
                'package main\n\nimport "testing"\n\nfunc TestUnrelated(t *testing.T) {}\n')
            with self.assertRaisesRegex(RuntimeError, 'zero matching tests'):
                runner.execute_validation('wizard-app-edit-load-tests', root)

    def test_combined_profiles_require_every_family(self):
        families = {
            'wizard-change-presentation-tests':
                ['TestAppChangePreviewPartial', 'TestAppChangeResultPresenterPartial'],
            'wizard-app-edit-tests':
                ['TestAppEditApplyConfirmed', 'TestAppEditCommandInput'],
            'wizard-registration-tests':
                ['TestAppRegistrationCICommandHelpAndJSONDiscovery',
                 'TestCommandTreeCIRouterPathsUnique',
                 'TestMachineCoverageCIRealTreeAndJSONClassification',
                 'TestMachineDocumentationTracksRegistryAndCommandTree',
                 'TestHumanCommandIndexTracksExactCommandMetadata',
                 'TestMCPRegistrationCIInProcessDiscoverAndInvoke'],
        }
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'go.mod').write_text('module example.com/wizardfixture\n\ngo 1.25\n')
            pkg = root / 'cmd' / 'baha'
            pkg.mkdir(parents=True)
            for profile, names in families.items():
                with self.subTest(profile=profile):
                    for omitted in [None] + names:
                        cases = [n for n in names if n != omitted]
                        (pkg / 'wizard_profiles_test.go').write_text(
                            'package main\nimport "testing"\n' +
                            '\n'.join('func %s(t *testing.T) {}' % n for n in cases) + '\n')
                        if omitted is None:
                            self.assertEqual(set(names), set(runner.execute_validation(profile, root)))
                        else:
                            with self.assertRaisesRegex(RuntimeError, 'zero matching tests'):
                                runner.execute_validation(profile, root)

    def test_skipped_required_test_is_not_success(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'go.mod').write_text('module example.com/wizardfixture\n\ngo 1.25\n')
            pkg = root / 'cmd' / 'baha'
            pkg.mkdir(parents=True)
            (pkg / 'wizard_profiles_test.go').write_text(
                'package main\nimport "testing"\n'
                'func TestAppEditLoadSkip(t *testing.T) { t.Skip("not executed") }\n')
            with self.assertRaisesRegex(RuntimeError, 'required test skipped'):
                runner.execute_validation('wizard-app-edit-load-tests', root)


class Core754MCPExecutionTests(unittest.TestCase):
    def test_exact_two_tests_are_mandatory_in_existing_runner(self):
        profile = runner.WIZARD_TEST_FAMILIES['wizard-core754-mcp-tests']
        self.assertEqual((r'^TestExpectedPreconditionsShareTypedCLIMCPErrors\Z',
                          r'^TestMCPReadEntryExpectedPreconditions\Z'), profile)
        self.assertEqual([{'check': 'wizard-core754-mcp-tests'}],
                         runner.matrix({'required_validation_checks': [{'id': 'wizard-core754-mcp-tests'}]})['include'])

    def test_real_two_family_execution_and_fail_closed(self):
        names = ('TestExpectedPreconditionsShareTypedCLIMCPErrors',
                 'TestMCPReadEntryExpectedPreconditions')
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'go.mod').write_text('module example.com/mcpcifixture\n\ngo 1.25\n')
            package = root / 'cmd' / 'baha'
            package.mkdir(parents=True)
            def write_tests(omit=None, skipped=None, failed=None):
                funcs = []
                for name in names:
                    if name == omit:
                        continue
                    action = ('t.Skip("required family skipped")' if name == skipped else
                              't.Fatal("required family failed")' if name == failed else '')
                    funcs.append('func %s(t *testing.T) { %s }' % (name, action))
                (package / 'preconditions_test.go').write_text(
                    'package main\nimport "testing"\n' + '\n'.join(funcs) + '\n')
            write_tests()
            self.assertEqual(list(names), runner.execute_validation('wizard-core754-mcp-tests', root))
            for missing in names:
                write_tests(omit=missing)
                with self.subTest(missing=missing):
                    with self.assertRaisesRegex(RuntimeError, 'zero matching tests'):
                        runner.execute_validation('wizard-core754-mcp-tests', root)
            for skipped in names:
                write_tests(skipped=skipped)
                with self.subTest(skipped=skipped):
                    with self.assertRaisesRegex(RuntimeError, 'required test skipped'):
                        runner.execute_validation('wizard-core754-mcp-tests', root)
            for failed in names:
                write_tests(failed=failed)
                with self.subTest(failed=failed):
                    with self.assertRaisesRegex(RuntimeError, 'test execution failed'):
                        runner.execute_validation('wizard-core754-mcp-tests', root)

    def test_missing_and_skipped_mandatory_tests_block(self):
        from unittest.mock import patch
        from subprocess import CompletedProcess
        listed = CompletedProcess([], 0, 'TestExpectedPreconditionsShareTypedCLIMCPErrors\n', '')
        with patch.object(runner.subprocess, 'run', return_value=listed):
            with self.assertRaisesRegex(RuntimeError, 'did not execute'):
                runner.execute_validation('wizard-core754-mcp-tests', '.')
        with patch.object(runner.subprocess, 'run', return_value=CompletedProcess([], 0, '', '')):
            with self.assertRaisesRegex(RuntimeError, 'zero matching tests'):
                runner.execute_validation('wizard-core754-mcp-tests', '.')
if __name__ == '__main__':
    unittest.main()
