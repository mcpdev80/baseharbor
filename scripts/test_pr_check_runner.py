"""Check the trusted dynamic validation interface without running native gates."""
import importlib.util
import os
import subprocess
import sys
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
                      planner.INSPECT_CHECKS, planner.RESOLVER_CHECKS):
            emitted |= {check['id'] for check in group}
        self.assertEqual(set(), emitted - runner.VALIDATIONS)

    def test_resolver_checks_have_workflow_dispatch(self):
        workflow = (Path(__file__).parent.parent / '.github' / 'workflows' / 'ci.yml').read_text()
        for check_id in ('resolver-gofmt', 'resolver-package-tests', 'resolver-vet',
                         'resolver-consumer-build'):
            with self.subTest(check_id=check_id):
                self.assertIn(check_id + ')', workflow)

    def test_empty_validation_never_claims_pass(self):
        self.assertEqual([{'check': 'not-required'}],
                         runner.matrix({'required_validation_checks': []})['include'])

    def test_unknown_and_duplicate_check_are_rejected(self):
        for checks in ([{'id': 'unknown'}], [{'id': 'mkdocs-de-strict'}, {'id': 'mkdocs-de-strict'}]):
            with self.subTest(checks=checks):
                with self.assertRaises(ValueError):
                    runner.matrix({'required_validation_checks': checks})




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

    def test_core918_native_security_executor_missing_skip_failed(self):
        names = ["TestNativeSQLInventoryRejectsUnverifiedMaterialRegardlessOfKnownPath","TestNativeSQLInventoryKnownFilesRequireActualInspection","TestNativeSQLGeneratedProjectionRequiresProtectedAncestorAndSource","TestNativeSQLProductionMaterialGateRejectsOversizedAndBrokenPublicSources"]
        check = 'wizard-core918-native-sql-safety-tests'
        self.assertEqual(tuple('^' + n for n in names),
                         runner.WIZARD_TEST_FAMILIES[check])
        self.assertEqual([{'check': check}],
                         runner.matrix({'required_validation_checks': [{'id': check}]})['include'])
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / 'go.mod').write_text('module example.com/nativefixture' + chr(10) * 2 + 'go 1.25' + chr(10))
            pkg = root / 'cmd' / 'baha'
            pkg.mkdir(parents=True)
            fixture = pkg / 'safety_test.go'
            def prepare(missing=None, skipped=None, failed=None):
                lines = ['package main', 'import "testing"']
                for name in names:
                    if name == missing:
                        continue
                    body = 't.Skip("skip")' if name == skipped else 't.Fatal("failure")' if name == failed else ''
                    lines.append('func %s(t *testing.T) { %s }' % (name, body))
                fixture.write_text(chr(10).join(lines) + chr(10))
            prepare()
            self.assertEqual(names, runner.execute_validation(check, root))
            for name in names:
                with self.subTest(missing=name):
                    prepare(missing=name)
                    with self.assertRaisesRegex(RuntimeError, 'zero matching tests'):
                        runner.execute_validation(check, root)
            prepare(skipped=names[0])
            with self.assertRaisesRegex(RuntimeError, 'required test skipped'):
                runner.execute_validation(check, root)
            prepare(failed=names[0])
            with self.assertRaisesRegex(RuntimeError, 'test execution failed'):
                runner.execute_validation(check, root)

class Core754MCPExecutionTests(unittest.TestCase):
    def test_exact_two_tests_are_mandatory_in_existing_runner(self):
        profile = runner.WIZARD_TEST_FAMILIES['wizard-core754-mcp-tests']
        self.assertEqual((r'^TestExpectedPreconditionsShareTypedCLIMCPErrors$',
                          r'^TestMCPReadEntryExpectedPreconditions$'), profile)
        self.assertEqual([{'check': 'wizard-core754-mcp-tests'}],
                         runner.matrix({'required_validation_checks': [{'id': 'wizard-core754-mcp-tests'}]})['include'])

    def test_actual_go_regex_selection_and_execution(self):
        # No project imports or toolchain download; Go's regexp engine is
        # exercised by the actual runner on an isolated package.
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'go.mod').write_text('module example.com/core754fixture\n\ngo 1.22\n')
            pkg = root / 'cmd' / 'baha'
            pkg.mkdir(parents=True)
            (pkg / 'core754_test.go').write_text(
                'package main\n\nimport "testing"\n\n'
                'func TestExpectedPreconditionsShareTypedCLIMCPErrors(t *testing.T) {}\n'
                'func TestMCPReadEntryExpectedPreconditions(t *testing.T) {}\n'
                'func TestExpectedPreconditionsShareTypedCLIMCPErrorsExtra(t *testing.T) { t.Fatal("extra matched") }\n'
                'func TestMCPReadEntryExpectedPreconditionsExtra(t *testing.T) { t.Fatal("extra matched") }\n')
            self.assertEqual(
                ['TestExpectedPreconditionsShareTypedCLIMCPErrors',
                 'TestMCPReadEntryExpectedPreconditions'],
                runner.execute_validation('wizard-core754-mcp-tests', root))

    def test_actual_workflow_loads_runner_from_trusted_base(self):
        workflow = (Path(__file__).resolve().parent.parent /
                    '.github' / 'workflows' / 'ci.yml').read_text()
        self.assertIn('wizard-*-tests)', workflow)
        self.assertIn('git show "$BASE_SHA":scripts/pr-check-runner.py', workflow)
        self.assertIn('python3 "$RUNNER_TEMP/pr-check-runner.py" execute --check "$VALIDATION"', workflow)
        self.assertNotIn('git show "$HEAD_SHA":scripts/pr-check-runner.py', workflow)

    def test_actual_workflow_route_receipt_execution_uses_trusted_base(self):
        # Receipt suites must execute once in their selected validation job,
        # via the trusted base runner. The always-on focused job must not
        # re-execute them. A candidate-owned runner cannot define discovery.
        workflow = (Path(__file__).resolve().parent.parent /
                    '.github' / 'workflows' / 'ci.yml').read_text()
        start = workflow.index('            receipt-execution-tests)\n')
        fragment = workflow[start:workflow.index('              ;;', start)]
        self.assertIn('git fetch --no-tags origin "$BASE_SHA"', fragment)
        self.assertIn('git show "$BASE_SHA":scripts/pr-check-runner.py', fragment)
        self.assertIn('--check "$VALIDATION"', fragment)
        focused = workflow.split('  pr_focused:\n', 1)[1].split('  pr_validations:\n', 1)[0]
        self.assertNotIn('receipt-execution-tests', focused)
        planner = (Path(__file__).resolve().parent / 'pr-gate-plan.py').read_text()
        self.assertIn("'id': 'receipt-execution-tests'", planner)
        self.assertNotIn(
            'python3 scripts/pr-check-runner.py execute --check '
            'receipt-execution-tests', workflow)

    def test_both_required_families_execute_or_block(self):
        from subprocess import CompletedProcess
        from unittest.mock import patch
        names = ('TestExpectedPreconditionsShareTypedCLIMCPErrors',
                 'TestMCPReadEntryExpectedPreconditions')
        def run_case(missing=None, skipped=None, failed=None):
            seen = []
            def fake_run(args, **kwargs):
                pattern = args[args.index('-list') + 1] if '-list' in args else args[args.index('-run') + 1]
                name = next(name for name in names if name in pattern)
                if '-list' in args:
                    return CompletedProcess(args, 0, '' if name == missing else name + '\n', '')
                seen.append(name)
                status = 'SKIP' if name == skipped else 'FAIL' if name == failed else 'PASS'
                output = '=== RUN   ' + name + '\n--- ' + status + ': ' + name + ' (0.00s)\n'
                return CompletedProcess(args, 1 if name == failed else 0, output, '')
            with patch.object(runner.subprocess, 'run', side_effect=fake_run):
                if missing:
                    with self.assertRaisesRegex(RuntimeError, 'zero matching tests'):
                        runner.execute_validation('wizard-core754-mcp-tests', '.')
                elif skipped:
                    with self.assertRaisesRegex(RuntimeError, 'required test skipped'):
                        runner.execute_validation('wizard-core754-mcp-tests', '.')
                elif failed:
                    with self.assertRaisesRegex(RuntimeError, 'test execution failed'):
                        runner.execute_validation('wizard-core754-mcp-tests', '.')
                else:
                    self.assertEqual(list(names), runner.execute_validation('wizard-core754-mcp-tests', '.'))
            return seen
        self.assertEqual(list(names), run_case())
        for name in names:
            with self.subTest(missing=name):
                self.assertEqual(list(names[:names.index(name)]), run_case(missing=name))
            with self.subTest(skipped=name):
                self.assertEqual(list(names[:names.index(name)+1]), run_case(skipped=name))
            with self.subTest(failed=name):
                self.assertEqual(list(names[:names.index(name)+1]), run_case(failed=name))

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

    def test_receipt_registration_files_exist(self):
        # R27-F2 registration exactness: every module the runner will execute
        # must exist as a scripts/ file, so the execution list can never drift
        # into ghosts or silently-skipped glob survivors.
        repo = Path(__file__).resolve().parent.parent
        for module in runner.RECEIPT_EXECUTION_SUITES:
            with self.subTest(module=module):
                self.assertTrue((repo / 'scripts' / (module + '.py')).is_file(),
                                'registered receipt suite does not exist: ' + module)
        self.assertNotIn('test_receipt*.py',
                         ' '.join(runner.RECEIPT_EXECUTION_SUITES))

    def test_receipt_execution_selects_exact_registered_modules(self):
        # The check must invoke unittest with the explicit registered module
        # list - never a filename glob that can silently drop suites.
        from unittest.mock import patch
        from subprocess import CompletedProcess
        captured = {}

        def fake_run(argv, **kwargs):
            captured['argv'] = list(argv)
            output = ''.join(
                'test_x (%s.Cases.test_x) ... ok\n' % module
                for module in runner.RECEIPT_EXECUTION_SUITES)
            return CompletedProcess(argv, 0, output + 'Ran 5 tests\nOK\n', '')
        with patch.object(runner.subprocess, 'run', side_effect=fake_run):
            executed = runner.execute_validation(
                'receipt-execution-tests',
                str(Path(__file__).resolve().parent.parent))
        self.assertNotIn('discover', captured['argv'])
        self.assertNotIn('-p', captured['argv'])
        for module in runner.RECEIPT_EXECUTION_SUITES:
            self.assertIn(module, captured['argv'])
        self.assertEqual([module + '.py' for module in
                          runner.RECEIPT_EXECUTION_SUITES], executed)

    def test_receipt_execution_fails_closed(self):
        repo = str(Path(__file__).resolve().parent.parent)
        from unittest.mock import patch
        from subprocess import CompletedProcess
        ok_output = ''.join(
            'test_x (%s.Cases.test_x) ... ok\n' % module
            for module in runner.RECEIPT_EXECUTION_SUITES) + 'Ran 5 tests\nOK\n'
        with self.subTest(failing_run=True):
            with patch.object(runner.subprocess, 'run',
                              return_value=CompletedProcess([], 1, 'FAILED', '')):
                with self.assertRaisesRegex(RuntimeError, 'execution failed'):
                    runner.execute_validation('receipt-execution-tests', repo)
        with self.subTest(zero_tests=True):
            with patch.object(runner.subprocess, 'run',
                              return_value=CompletedProcess([], 0, 'Ran 0 tests\nOK\n', '')):
                with self.assertRaisesRegex(RuntimeError, 'no receipt test executed'):
                    runner.execute_validation('receipt-execution-tests', repo)
        with self.subTest(dropped_suite=True):
            partial = ok_output.replace(
                'test_receipt_postrun.', 'other_module.')
            with patch.object(runner.subprocess, 'run',
                              return_value=CompletedProcess([], 0, partial, '')):
                with self.assertRaisesRegex(RuntimeError, 'did not execute'):
                    runner.execute_validation('receipt-execution-tests', repo)
        with self.subTest(missing_file=True):
            import tempfile
            with tempfile.TemporaryDirectory() as empty_repo:
                with self.assertRaisesRegex(RuntimeError, 'registered receipt suite missing'):
                    runner.execute_validation('receipt-execution-tests', empty_repo)
class NormalPackageExecutionTests(unittest.TestCase):
    def test_cli_package_runs_non_guided_tests_and_propagates_failure(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'go.mod').write_text('module example.test/cli\n\ngo 1.25\n')
            package = root / 'cmd/baha'
            package.mkdir(parents=True)
            test = package / 'regression_test.go'
            test.write_text('package main\nimport "testing"\nfunc TestNewRegression(t *testing.T) {}\n')
            self.assertEqual(['./cmd/baha'], runner.execute_validation('cli-package-tests', root))
            test.write_text('package main\nimport "testing"\nfunc TestNewRegression(t *testing.T) { t.Fatal("regression") }\n')
            with self.assertRaisesRegex(RuntimeError, 'CLI package tests failed'):
                runner.execute_validation('cli-package-tests', root)

    def test_delivery_package_requires_real_passing_tests(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'go.mod').write_text('module example.test/delivery\n\ngo 1.25\n')
            with self.assertRaisesRegex(RuntimeError, 'delivery package missing'):
                runner.execute_validation('delivery-package-tests', root)
            package = root / 'internal/delivery'
            package.mkdir(parents=True)
            (package / 'contract.go').write_text('package delivery\n')
            with self.assertRaisesRegex(RuntimeError, 'lacks passing test execution'):
                runner.execute_validation('delivery-package-tests', root)
            test = package / 'contract_test.go'
            test.write_text('package delivery\nimport "testing"\nfunc TestContract(t *testing.T) {}\n')
            self.assertEqual(['./internal/delivery'], runner.execute_validation('delivery-package-tests', root))
            for action in ('t.Fatal("failure")', 't.Skip("not executed")'):
                test.write_text('package delivery\nimport "testing"\nfunc TestContract(t *testing.T) {' + action + '}\n')
                with self.assertRaises(RuntimeError):
                    runner.execute_validation('delivery-package-tests', root)

    def test_authority_packages_require_actual_passing_test_execution(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / 'internal/coreinstallation').mkdir(parents=True)
            rows = [{'Package': 'example.test/internal/coreinstallation', 'Test': 'TestAuthority', 'Action': 'run'},
                    {'Package': 'example.test/internal/coreinstallation', 'Test': 'TestAuthority', 'Action': 'pass'},
                    {'Package': 'example.test/internal/coreinstallation', 'Action': 'pass'}]
            import json
            for events, valid in [(rows, True), ([], False), (rows[:1], False)]:
                with self.subTest(events=events), patch.object(runner.subprocess, 'run',
                        return_value=subprocess.CompletedProcess([], 0, '\n'.join(json.dumps(row) for row in events))):
                    if valid:
                        self.assertEqual(['./internal/coreinstallation'], runner.execute_validation('authority-package-tests', tmp))
                    else:
                        with self.assertRaises(RuntimeError):
                            runner.execute_validation('authority-package-tests', tmp)

class GoJSONPackageTests(unittest.TestCase):
    PROFILES = {
        'authority-package-tests': 'internal/coreinstallation',
        'delivery-package-tests': 'internal/delivery',
        'deployment-package-tests': 'internal/deployment',
        'broker-package-tests': 'internal/runtimebroker',
        'cli-package-tests': 'cmd/baha',
        'logs-package-tests': 'internal/logs',
    }

    def events(self, package, action='pass', include_package=True):
        import json
        rows = [{'Package': 'example.test/' + package, 'Test': 'TestRequired', 'Action': 'run'},
                {'Package': 'example.test/' + package, 'Test': 'TestRequired', 'Action': action}]
        if include_package:
            rows.append({'Package': 'example.test/' + package, 'Action': 'pass'})
        return '\n'.join(json.dumps(row) for row in rows)

    def test_diagnostics_are_not_parsed_as_json_for_any_package_profile(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            for check, package in self.PROFILES.items():
                (Path(tmp) / package).mkdir(parents=True, exist_ok=True)
                with self.subTest(check=check), patch.object(runner.subprocess, 'run',
                        return_value=subprocess.CompletedProcess([], 0, self.events(package),
                                                                 'go: downloading example.test/module v1.0.0\n')) as run:
                    self.assertEqual(['./' + package], runner.execute_validation(check, tmp))
                    self.assertEqual(subprocess.PIPE, run.call_args.kwargs['stderr'])

    def test_failed_build_and_malformed_output_retain_diagnostics(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            for check, package in self.PROFILES.items():
                (Path(tmp) / package).mkdir(parents=True, exist_ok=True)
                for code, stdout, stderr, expected in [
                    (1, '', 'compiler failure: undefined symbol', 'exit 1.*compiler failure'),
                    (0, 'broken json', 'tool diagnostic', 'invalid Go JSON: broken jsontool diagnostic'),
                    (0, '[]', 'tool diagnostic', 'invalid Go events'),
                    (0, self.events(package, action='fail'), '', 'reported failure'),
                ]:
                    with self.subTest(check=check, stdout=stdout), patch.object(runner.subprocess, 'run',
                            return_value=subprocess.CompletedProcess([], code, stdout, stderr)):
                        with self.assertRaisesRegex(RuntimeError, expected):
                            runner.execute_validation(check, tmp)

    def test_package_success_requires_executed_tests_and_terminal_package_pass(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            for check, package in self.PROFILES.items():
                (Path(tmp) / package).mkdir(parents=True, exist_ok=True)
                for stdout in ('', self.events(package, action='skip'),
                               self.events(package, include_package=False)):
                    with self.subTest(check=check, stdout=stdout), patch.object(runner.subprocess, 'run',
                            return_value=subprocess.CompletedProcess([], 0, stdout, '')):
                        with self.assertRaisesRegex(RuntimeError, 'lacks passing test execution'):
                            runner.execute_validation(check, tmp)

    def test_cli_and_logs_allow_optional_native_skip_beside_passing_unit_tests(self):
        import json
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            for check, package in self.PROFILES.items():
                (Path(tmp) / package).mkdir(parents=True, exist_ok=True)
                stdout = self.events(package) + '\n' + '\n'.join(json.dumps(row) for row in [
                    {'Package': 'example.test/' + package, 'Test': 'TestNativeOptIn', 'Action': 'run'},
                    {'Package': 'example.test/' + package, 'Test': 'TestNativeOptIn', 'Action': 'skip'}])
                with self.subTest(check=check), patch.object(runner.subprocess, 'run',
                        return_value=subprocess.CompletedProcess([], 0, stdout, '')):
                    if check in ('cli-package-tests', 'logs-package-tests'):
                        self.assertEqual(['./' + package], runner.execute_validation(check, tmp))
                    else:
                        with self.assertRaisesRegex(RuntimeError, 'lacks passing test execution'):
                            runner.execute_validation(check, tmp)

    def test_deployment_package_uses_real_go_tests(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'go.mod').write_text('module example.test/deployment\n\ngo 1.25\n')
            package = root / 'internal/deployment'
            with self.assertRaisesRegex(RuntimeError, 'deployment package missing'):
                runner.execute_validation('deployment-package-tests', root)
            package.mkdir(parents=True)
            (package / 'store.go').write_text('package deployment\n')
            with self.assertRaisesRegex(RuntimeError, 'lacks passing test execution'):
                runner.execute_validation('deployment-package-tests', root)
            test = package / 'store_test.go'
            test.write_text('package deployment\nimport "testing"\nfunc TestStore(t *testing.T) {}\n')
            self.assertEqual(['./internal/deployment'], runner.execute_validation('deployment-package-tests', root))
            for action in ('t.Fatal("failure")', 't.Skip("not executed")'):
                test.write_text('package deployment\nimport "testing"\nfunc TestStore(t *testing.T) {' + action + '}\n')
                with self.assertRaises(RuntimeError):
                    runner.execute_validation('deployment-package-tests', root)

class OpenBaoPackageExecutionTests(unittest.TestCase):
    def test_new_service_issuer_test_executes_and_can_fail_outside_bootstrap_families(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'go.mod').write_text('module example.test/openbao\n\ngo 1.25\n')
            package = root / 'internal/openbao'
            package.mkdir(parents=True)
            tests = ['package openbao', 'import "testing"'] + [
                'func %s(t *testing.T) {}' % name for name in runner.OPENBAO_REQUIRED_TESTS]
            tests.append('func TestNativeOptional(t *testing.T) { t.Skip("native opt-in") }')
            fixture = package / 'service_issuer_test.go'
            for failed in (False, True):
                fixture.write_text('\n'.join(tests + [
                    'func TestServiceIssuerNewBehavior(t *testing.T) {' +
                    ('t.Fatal("new ServiceIssuer regression")' if failed else '') + '}\n']))
                if failed:
                    with self.assertRaisesRegex(RuntimeError, 'new ServiceIssuer regression'):
                        runner.execute_validation('openbao-package-tests', root)
                else:
                    self.assertEqual(list(runner.OPENBAO_REQUIRED_TESTS),
                                     runner.execute_validation('openbao-package-tests', root))

    def test_go_diagnostic_is_separate_and_required_bootstrap_skip_still_fails(self):
        import json
        from unittest.mock import patch
        names = runner.OPENBAO_REQUIRED_TESTS
        package = 'example.test/internal/openbao'
        for skipped in (None, names[0]):
            rows = []
            for name in names:
                rows.extend([{'Package': package, 'Test': name, 'Action': 'run'},
                             {'Package': package, 'Test': name,
                              'Action': 'skip' if name == skipped else 'pass'}])
            rows.append({'Package': package, 'Action': 'pass'})
            results = [subprocess.CompletedProcess([], 0, '\n'.join(names), ''),
                       subprocess.CompletedProcess([], 0, '\n'.join(json.dumps(row) for row in rows),
                                                   'go: downloading example.test/module v1.0.0\n')]
            with self.subTest(skipped=skipped), patch.object(runner.subprocess, 'run', side_effect=results) as run:
                if skipped:
                    with self.assertRaisesRegex(RuntimeError, 'required OpenBao tests missing PASS'):
                        runner.execute_validation('openbao-package-tests', '.')
                else:
                    self.assertEqual(list(names), runner.execute_validation('openbao-package-tests', '.'))
                self.assertNotIn('-run', run.call_args.args[0])
                self.assertEqual(subprocess.PIPE, run.call_args.kwargs['stderr'])

    def test_authority_includes_new_installauthority_leaf(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / 'internal/installauthority').mkdir(parents=True)
            with patch.object(runner, 'go_package_tests', return_value=['./internal/installauthority']) as execute:
                self.assertEqual(['./internal/installauthority'],
                                 runner.execute_validation('authority-package-tests', tmp))
                self.assertEqual(['./internal/installauthority'], execute.call_args.args[1])

if __name__ == '__main__':
    unittest.main()
