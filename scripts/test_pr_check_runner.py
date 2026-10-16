"""Check the trusted dynamic validation interface without running native gates."""
import importlib.util
from pathlib import Path
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
                      planner.CAPABILITY_CHECKS, planner.CI_CHECKS, planner.INSPECT_CHECKS):
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


if __name__ == '__main__':
    unittest.main()
