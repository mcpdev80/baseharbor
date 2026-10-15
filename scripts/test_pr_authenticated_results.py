"""Verified Actions adapter regression tests: missing, forged, stale, and failed."""
import importlib.util
from pathlib import Path
import unittest

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


if __name__ == '__main__':
    unittest.main()
