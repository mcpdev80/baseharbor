"""Verified Actions adapter regression tests: missing, forged, stale, and failed."""
import importlib.util
import copy
import re
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('auth_results', Path(__file__).with_name('pr-authenticated-results.py'))
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
HEAD = 'b'*40
PLAN = {'target_base_sha':'a'*40,'head_sha':HEAD,'merge_base_sha':'c'*40,'integration_sha':'d'*40,
        'required_validation_checks':[{'id':'documentation-audit'}]}
RUN = {'id':123,'run_attempt':1,'event':'pull_request','head_sha':HEAD}
JOBS = [{'name': name,'run_id':123,'run_attempt':1,'conclusion':'success'}
        for name in ('PR · Validation · documentation-audit','PR · Focused Python','Workflow policy')]


class AuthenticatedResultsTests(unittest.TestCase):
    def test_final_job_waits_for_mandatory_authenticated_policy(self):
        workflow = (Path(__file__).resolve().parents[1] /
                    '.github/workflows/ci.yml').read_text()
        final = workflow.split('  pr_selected_final:\n', 1)[1]
        final = re.split(r'^  [A-Za-z_][A-Za-z0-9_]*:\s*$', final,
                         maxsplit=1, flags=re.MULTILINE)[0]
        needs = re.search(r'^    needs: \[(.*?)\]$', final, re.MULTILINE)
        self.assertIsNotNone(needs, 'final check requires explicit job dependencies')
        dependencies = {name.strip() for name in needs.group(1).split(',')}
        # The verifier always authenticates policy and focused-Python results;
        # neither may race the final job, regardless of selected matrix lanes.
        self.assertTrue({'workflow_policy', 'pr_focused', 'pr_plan', 'pr_validations'}
                        <= dependencies, dependencies)
        report = mod.authenticated_results(PLAN, RUN, JOBS, 123, 1, HEAD)
        self.assertIn('Workflow policy',
                      {row['name'] for row in report['required_job_conclusions']})

    def test_adapter_comes_only_from_immutable_trusted_base_and_verifier_stays_pinned(self):
        workflow = (Path(__file__).resolve().parents[1] /
                    '.github/workflows/ci.yml').read_text()
        final = workflow.split('  pr_selected_final:\n', 1)[1]
        origin = [line.strip() for line in final.splitlines()
                  if 'git show' in line and ':scripts/pr-authenticated-results.py' in line]
        self.assertEqual(['git show "${EXPECTED_BASE}:scripts/pr-authenticated-results.py" '
                          '> "$RUNNER_TEMP/pr-authenticated-results.py"'], origin)
        self.assertIn('EXPECTED_BASE: ${{ github.event.pull_request.base.sha }}', final)
        self.assertIn('ref: ${{ github.event.pull_request.base.sha }}', final)
        self.assertIn('git show f4b173d3b07959bd671727800e9c982d19894be4:'
                      'scripts/pr-required-final.py > "$RUNNER_TEMP/pr-required-final.py"', final)

    def test_actual_complete_job_results(self):
        report = mod.authenticated_results(PLAN,RUN,JOBS,123,1,HEAD)
        self.assertEqual([{'kind':'validation','id':'documentation-audit','conclusion':'success'}],
                         report['results'])
        self.assertFalse(any(x['kind']=='gate' for x in report['results']))

    def test_missing_or_duplicate_jobs_block(self):
        for jobs in (JOBS[:1], JOBS + JOBS[:1]):
            with self.subTest(jobs=jobs):
                with self.assertRaises(PermissionError):
                    mod.authenticated_results(PLAN,RUN,jobs,123,1,HEAD)

    def test_stale_or_forged_run_blocks(self):
        for run in (dict(RUN,head_sha='f'*40),dict(RUN,run_attempt=2),
                    dict(RUN,event='push'),dict(RUN,id=999)):
            with self.subTest(run=run):
                with self.assertRaises(PermissionError):
                    mod.authenticated_results(PLAN,run,JOBS,123,1,HEAD)

    def test_negative_job_outcomes_not_pass(self):
        for status in ('cancelled','failure','skipped','timed_out',None):
            jobs = [dict(x,conclusion=status) if x['name'].startswith('PR · Validation') else x for x in JOBS]
            with self.subTest(status=status):
                out=mod.authenticated_results(PLAN,RUN,jobs,123,1,HEAD)
                self.assertEqual('blocked',out['results'][0]['conclusion'])

    def test_native_gate_requires_exact_candidate_job(self):
        native = dict(PLAN, selected_gates=['atomic/docker/backup-restore'], required_consumer_dependencies=['core','demo'])
        with self.assertRaises(PermissionError):
            mod.authenticated_results(native, RUN, JOBS, 123, 1, HEAD)
        jobs = JOBS + [{'name':name,'run_id':123,
                        'run_attempt':1,'conclusion':'success'} for name in
                       ('PR · Exact Candidate','PR · Runner Interface','PR · Demo Pin')]
        result=mod.authenticated_results(native, RUN, jobs, 123, 1, HEAD)
        self.assertFalse(any(x['kind']=='gate' for x in result['results']))
        failed=jobs[:-1]+[dict(jobs[-1], conclusion='failure')]
        blocked=mod.authenticated_results(native, RUN, failed, 123, 1, HEAD)
        self.assertIn('failure',[x['conclusion'] for x in blocked['required_job_conclusions']])

    def test_focused_and_policy_failure_blocks(self):
        for name in ('PR · Focused Python','Workflow policy'):
            jobs=[dict(x,conclusion='failure') if x['name']==name else x for x in JOBS]
            with self.subTest(name=name):
                result=mod.authenticated_results(PLAN,RUN,jobs,123,1,HEAD)
                self.assertIn('failure',[x['conclusion'] for x in result['required_job_conclusions']])


class AuthenticatedReadinessTests(unittest.TestCase):
    def setUp(self):
        self.elapsed = 0
        self.original_read_run = mod.read_run
        self.completed = [dict(job, status='completed') for job in JOBS]
        clock = patch.object(mod.time, 'monotonic', side_effect=lambda: self.elapsed)
        sleeper = patch.object(mod.time, 'sleep', side_effect=self.advance)
        reader = patch.object(mod, 'read_run', return_value=(RUN, self.completed))
        self.clock = clock.start()
        self.sleep = sleeper.start()
        self.reader = reader.start()
        self.addCleanup(clock.stop)
        self.addCleanup(sleeper.stop)
        self.addCleanup(reader.stop)

    def advance(self, seconds):
        self.elapsed += seconds

    def read(self):
        return mod.read_authenticated_results(PLAN, 'mcpdev80/baseharbor', 123, 1, HEAD, 'token')

    def test_complete_results_do_not_wait_for_unrelated_jobs(self):
        unrelated = dict(name='optional lane', run_id=123, run_attempt=1,
                         status='in_progress', conclusion=None)
        self.reader.return_value = RUN, self.completed + [unrelated]
        result = self.read()
        self.assertEqual('success', result['results'][0]['conclusion'])
        self.reader.assert_called_once()
        self.sleep.assert_not_called()

    def test_each_required_nonterminal_job_is_reauthenticated_until_success(self):
        for index in range(len(self.completed)):
            for status in ('queued', 'in_progress', 'waiting', 'pending', 'requested'):
                with self.subTest(index=index, status=status):
                    pending = [dict(job, status=status, conclusion=None) if i == index else job
                               for i, job in enumerate(self.completed)]
                    self.reader.reset_mock()
                    self.sleep.reset_mock()
                    self.reader.side_effect = [(RUN, pending), (RUN, self.completed)]
                    result = self.read()
                    self.assertEqual('success', result['results'][0]['conclusion'])
                    self.assertTrue(all(row['conclusion'] == 'success'
                                        for row in result['required_job_conclusions']))
                    self.assertEqual(2, self.reader.call_count)
                    self.sleep.assert_called_once_with(3)

    def test_nonterminal_lifecycle_never_accepts_an_early_success_conclusion(self):
        pending = [dict(job, status='in_progress') for job in self.completed]
        self.reader.side_effect = [(RUN, pending), (RUN, self.completed)]
        self.read()
        self.assertEqual(2, self.reader.call_count)

    def test_nonterminal_timeout_has_finite_clock_and_poll_bound(self):
        self.reader.return_value = RUN, [dict(job, status='in_progress', conclusion=None)
                                       for job in self.completed]
        with self.assertRaises(TimeoutError):
            self.read()
        self.assertEqual(45, self.elapsed)
        self.assertLessEqual(self.reader.call_count, 16)
        self.assertEqual(15, self.sleep.call_count)

    def test_terminal_failure_skip_or_missing_conclusion_fail_fast_even_with_other_pending(self):
        for conclusion in ('failure', 'cancelled', 'skipped', 'timed_out', None):
            with self.subTest(conclusion=conclusion):
                jobs = copy.deepcopy(self.completed)
                jobs[0].update(conclusion=conclusion)
                jobs[1].update(status='in_progress', conclusion=None)
                self.reader.return_value = RUN, jobs
                self.reader.reset_mock()
                self.sleep.reset_mock()
                result = self.read()
                self.assertEqual('blocked', result['results'][0]['conclusion'])
                self.reader.assert_called_once()
                self.sleep.assert_not_called()

    def test_missing_duplicate_forged_and_unknown_lifecycle_fail_without_retry(self):
        forged = [dict(job, run_attempt=2) if i == 0 else job
                  for i, job in enumerate(self.completed)]
        cross_run = [dict(job, run_id=999) if i == 0 else job
                     for i, job in enumerate(self.completed)]
        unknown = [dict(job, status='invented') if i == 0 else job
                   for i, job in enumerate(self.completed)]
        variants = [(RUN, self.completed[:-1]), (RUN, self.completed + self.completed[:1]),
                    (dict(RUN, head_sha='f' * 40), self.completed),
                    (dict(RUN, run_attempt=2), self.completed), (RUN, forged),
                    (RUN, cross_run), (RUN, unknown)]
        for snapshot in variants:
            with self.subTest(snapshot=snapshot):
                self.reader.return_value = snapshot
                self.reader.reset_mock()
                self.sleep.reset_mock()
                with self.assertRaises(PermissionError):
                    self.read()
                self.reader.assert_called_once()
                self.sleep.assert_not_called()

    def test_authentication_is_repeated_after_pending_snapshot(self):
        pending = [dict(job, status='in_progress', conclusion=None) for job in self.completed]
        self.reader.side_effect = [(RUN, pending), (dict(RUN, head_sha='f' * 40), self.completed)]
        with self.assertRaises(PermissionError):
            self.read()
        self.assertEqual(2, self.reader.call_count)
        self.sleep.assert_called_once_with(3)

    def test_transport_failure_and_expired_api_budget_fail_closed(self):
        self.reader.side_effect = OSError('API unavailable')
        with self.assertRaises(OSError):
            self.read()
        self.sleep.assert_not_called()
        with patch.object(mod, 'api_json') as api:
            with self.assertRaises(TimeoutError):
                self.original_read_run('mcpdev80/baseharbor', 123, 1, 'token', deadline=0)
            api.assert_not_called()

    def test_each_api_request_has_remaining_deadline_budget(self):
        api_responses = [RUN, {'jobs': self.completed}]
        with patch.object(mod, 'api_json', side_effect=api_responses) as api:
            self.original_read_run('mcpdev80/baseharbor', 123, 1, 'token', deadline=7)
        self.assertEqual([7, 7], [call.kwargs['timeout'] for call in api.call_args_list])


if __name__ == '__main__':
    unittest.main()
