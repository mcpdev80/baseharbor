"""Normal workflow dependencies gate PR completion without an Actions API."""
import copy
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('final', Path(__file__).with_name('pr-required-final.py'))
final = importlib.util.module_from_spec(spec)
spec.loader.exec_module(final)


stage_spec = importlib.util.spec_from_file_location(
    'stage', Path(__file__).with_name('pr-stage-boundary.py'))
stage = importlib.util.module_from_spec(stage_spec)
stage_spec.loader.exec_module(stage)


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

    def test_staged_successful_docker_guided_gate_is_accepted(self):
        self.plan['selected_gates'] = ['atomic/docker/guided']
        self.plan['required_consumer_dependencies'] = ['core', 'demo']
        self.plan = stage.stage(self.plan)
        self.assertEqual(['atomic/docker/guided'], self.plan['selected_gates'])
        self.assertEqual([], self.plan['deferred_integration_gates'])
        for name in ('pr_candidate', 'pr_runner_interface',
                     'pr_runtime_selected', 'pr_demo_pin'):
            self.needs[name] = {'result': 'success'}
        self.assertEqual([], final.verify(self.plan, self.needs))
        self.needs['pr_runtime_selected'] = {'result': 'failure'}
        self.assertTrue(final.verify(self.plan, self.needs))

    def test_public_contracts_and_private_dependency(self):
        self.plan['selected_gates'] = ['integration/static/public-contracts']
        self.plan['required_consumer_dependencies'] = ['core', 'console']
        self.needs['pr_public_contracts'] = {'result': 'success'}
        self.assertTrue(final.verify(self.plan, self.needs))
        self.needs['pr_consumer_pins'] = {'result': 'success'}
        self.assertEqual([], final.verify(self.plan, self.needs))

    def test_coreless_gates_require_selected_integration_executor(self):
        self.plan['selected_gates'] = [
            'integration/docker/coreless-lifecycle',
            'integration/podman/coreless-lifecycle']
        self.needs['pr_selected_integration'] = {'result': 'success'}
        self.needs['pr_demo_pin'] = {'result': 'success'}
        self.assertEqual([], final.verify(self.plan, self.needs))
        self.needs['pr_selected_integration']['result'] = 'skipped'
        self.assertTrue(final.verify(self.plan, self.needs))
        self.needs['pr_selected_integration']['result'] = 'success'
        self.needs['pr_demo_pin']['result'] = 'skipped'
        self.assertTrue(final.verify(self.plan, self.needs))

        self.plan['selected_gates'] = []
        self.plan['deferred_integration_gates'] = [
            'integration/docker/coreless-lifecycle',
            'integration/podman/coreless-lifecycle']
        self.assertTrue(final.verify(self.plan, self.needs))
        self.plan['deferred_integration_gates'].append('integration/unknown')
        self.assertTrue(final.verify(self.plan, self.needs))

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

    def test_selected_go_unit_shards_are_mandatory(self):
        for selection in ('go-unit-tests', 'cli-package-tests'):
            with self.subTest(selection=selection):
                plan = {**self.plan, 'required_validation_checks': [{'id': selection}]}
                self.assertTrue(final.verify(plan, self.needs))
                needs = copy.deepcopy(self.needs)
                needs['pr_unit_shards'] = {'result': 'success'}
                self.assertEqual([], final.verify(plan, needs))
                for result in ('failure', 'skipped', 'cancelled'):
                    needs['pr_unit_shards']['result'] = result
                    self.assertTrue(final.verify(plan, needs))

    def test_candidate_added_plan_does_not_replace_trusted_base_plan(self):
        candidate_plan = {**self.plan,
                          'selected_gates': ['atomic/new-runtime/lifecycle'],
                          'required_validation_checks': [
                              *self.plan['required_validation_checks'],
                              {'id': 'candidate-only-validation'}]}
        # The final verifier receives only the trusted base plan. Candidate
        # planner output and advisory outcomes are kept outside this argument.
        self.assertEqual([], final.verify(self.plan, self.needs))
        self.assertNotEqual([], final.verify(candidate_plan, self.needs))

    def test_candidate_added_job_failure_is_not_part_of_base_workflow_requirements(self):
        self.needs['pr_candidate_advisory'] = {'result': 'failure'}
        self.assertEqual([], final.verify(self.plan, self.needs))

    def test_optional_failure_in_the_final_dependency_graph_is_not_ignored(self):
        self.needs['pr_demo_pin'] = {'result': 'failure'}
        self.assertTrue(final.verify(self.plan, self.needs))

    def test_release_only_atomic_static_gate_is_deferred_not_pr_executed(self):
        # Finding (0428-208 #1793): `atomic/static/mcp` is a release-only static
        # gate whose executable proof runs only at release/qualify
        # (pre-release.yml:663 static_gates ->
        # pre-release-runtime-suite.yml:267, GATE=mcp runs tests/mcp/run.sh),
        # never inside a PR. It must be
        # classified DEFERRED release-only by the stage boundary and must NOT
        # require any PR-executed job in the final verifier: no PR executor
        # exists, and no silent-pass is created by mapping it to pr_focused.
        self.plan['required_consumer_dependencies'] = ['core', 'demo']
        self.plan['selected_gates'] = ['atomic/static/mcp']
        staged = stage.stage(self.plan)
        # (a) mcp is pulled out of the selected list and recorded as deferred;
        # no required PR-check exists for it, and verify passes the remaining
        # base workflow jobs without any mcp proof running in the PR.
        self.assertEqual([], staged['selected_gates'])
        self.assertEqual(['atomic/static/mcp'], staged['deferred_integration_gates'])
        self.assertEqual([], final.verify(staged, self.needs))
        # (b) A failing PR-executed job still blocks -- the deferral is real and
        # visible, not a silent cover for a missing mcp proof.
        failing = copy.deepcopy(self.needs)
        failing['pr_validations'] = {'result': 'failure'}
        self.assertTrue(final.verify(staged, failing))

    def test_unsupported_deferred_gate_still_blocks(self):
        # The mcp deferral widens the allow-list by exactly one known release
        # gate; an invented deferred gate is still rejected.
        self.plan['selected_gates'] = []
        self.plan['deferred_integration_gates'] = ['atomic/static/phantom']
        self.assertTrue(final.verify(self.plan, self.needs))

    def test_focused_static_gates_are_deferred_not_pr_executed(self):
        # Pivot for the cancelled pr_focused mapping (0428-208 #1797): shell-ux
        # and ci-fast-check are release-only atomic/static/* gates whose
        # executable proof runs only at release/qualify (pre-release.yml:663
        # static_gates -> pre-release-runtime-suite.yml:267), NEVER inside a PR.
        # The stage boundary now classifies them DEFERRED release-only, so they
        # must map to NO PR executor and must NOT be pulled into pr_focused.
        self.plan['required_consumer_dependencies'] = ['core', 'demo']
        self.plan['selected_gates'] = ['atomic/static/shell-ux',
                                       'atomic/static/ci-fast-check']
        staged = stage.stage(self.plan)
        # Both are pulled out of the selected list and recorded as deferred;
        # no required PR-check exists for them.
        self.assertEqual([], staged['selected_gates'])
        self.assertEqual(['atomic/static/shell-ux',
                          'atomic/static/ci-fast-check'],
                         staged['deferred_integration_gates'])
        # With them deferred, verify passes the remaining base workflow jobs
        # WITHOUT any pr_focused job (which is absent from needs).
        self.assertEqual([], final.verify(staged, self.needs))
        # A failing PR-executed job still blocks -- the deferral is real and
        # visible, not a silent cover for missing shell-ux/ci-fast-check proof.
        failing = copy.deepcopy(self.needs)
        failing['pr_validations'] = {'result': 'failure'}
        self.assertTrue(final.verify(staged, failing))

    def test_unknown_atomic_static_gate_still_has_no_executor(self):
        # Only the enumerated deferred gates are honoured; any other
        # atomic/static/* gate still yields the no-executor error in the final
        # verifier and is rejected as an unsupported deferred gate when staged.
        self.plan['required_consumer_dependencies'] = ['core', 'demo']
        self.plan['selected_gates'] = ['atomic/static/phantom']
        self.assertTrue(final.verify(self.plan, self.needs))
        self.plan['selected_gates'] = []
        self.plan['deferred_integration_gates'] = ['atomic/static/phantom']
        self.assertTrue(final.verify(self.plan, self.needs))

    def test_mcp_deferral_unchanged_by_focused_mapping(self):
        # Restoring ATOMIC_STATIC_FOCUSED_GATES must NOT pull mcp into the
        # focused executor: mcp stays release-only deferred and requires no PR
        # check. (Regression guard against the focused set swallowing mcp.)
        self.plan['required_consumer_dependencies'] = ['core', 'demo']
        self.plan['selected_gates'] = ['atomic/static/mcp']
        staged = stage.stage(self.plan)
        self.assertEqual([], staged['selected_gates'])
        self.assertEqual(['atomic/static/mcp'], staged['deferred_integration_gates'])
        self.assertEqual([], final.verify(staged, self.needs))


if __name__ == '__main__':
    unittest.main()
