"""Independent positive, missing, denied and stale evidence checks."""
import importlib.util
import copy
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('final', Path(__file__).with_name('pr-required-final.py'))
final = importlib.util.module_from_spec(spec)
spec.loader.exec_module(final)
A = 'a' * 40
B = 'b' * 40
C = 'c' * 40
PLAN = {'schema':'baseharbor.pr-gate-plan/v1','release':'v0.4.26','target_base_sha':A,'head_sha':B,'merge_base_sha':C,'integration_sha':None,'coverage_status':'resolved','unresolved_coverage':[],'selected_gates':['atomic/static/init'],'required_validation_checks':[{'id':'documentation-audit'}]}
CATALOG = {'gates':[{'id':'atomic/static/init','required_for':['v0.4.26']}]}
REPORT = {'schema':'baseharbor.pr-check-results/v1','target_base_sha':A,'head_sha':B,'merge_base_sha':C,'integration_sha':None,'required_job_conclusions':[{'name':'PR · Focused Python','conclusion':'success'},{'name':'Workflow policy','conclusion':'success'}],'results':[{'kind':'gate','id':'atomic/static/init','conclusion':'success'},{'kind':'validation','id':'documentation-audit','conclusion':'success'}]}

class FinalChecks(unittest.TestCase):
    def test_success(self):
        self.assertEqual([], final.verify(PLAN,REPORT,CATALOG))

    def test_missing_forged_and_failed_authenticated_prerequisites(self):
        for conclusion in ('failure','cancelled','skipped',None):
            with self.subTest(conclusion=conclusion):
                report=copy.deepcopy(REPORT)
                report['required_job_conclusions'][1]['conclusion']=conclusion
                self.assertTrue(final.verify(PLAN,report,CATALOG))
        report=copy.deepcopy(REPORT)
        report['required_job_conclusions']=[]
        self.assertTrue(final.verify(PLAN,report,CATALOG))
        report=copy.deepcopy(REPORT)
        report['required_job_conclusions'].append({'name':'PR · Focused Python','conclusion':'success'})
        self.assertTrue(final.verify(PLAN,report,CATALOG))

    def test_blocking_conclusions(self):
        for conclusion in ('failure','cancelled','timed_out','skipped','pending','blocked',None):
            with self.subTest(conclusion=conclusion):
                report=copy.deepcopy(REPORT)
                report['results'][0]['conclusion']=conclusion
                self.assertTrue(final.verify(PLAN,report,CATALOG))

    def test_missing_and_duplicate(self):
        report=copy.deepcopy(REPORT)
        report['results'].pop()
        self.assertTrue(final.verify(PLAN,report,CATALOG))
        report=copy.deepcopy(REPORT)
        report['results'].append(report['results'][0])
        self.assertTrue(final.verify(PLAN,report,CATALOG))

    def test_stale_integrated_inputs(self):
        for field in ('head_sha','target_base_sha','merge_base_sha','integration_sha'):
            with self.subTest(field=field):
                report=copy.deepcopy(REPORT)
                report[field]=A if field != 'target_base_sha' else B
                self.assertTrue(final.verify(PLAN,report,CATALOG))

    def test_zero_gate_validation_only_passes(self):
        plan=copy.deepcopy(PLAN)
        plan['selected_gates']=[]
        report=copy.deepcopy(REPORT)
        report['results']=[r for r in report['results'] if r['kind']=='validation']
        self.assertEqual([],final.verify(plan,report,CATALOG))
        report['results'][0]['conclusion']='cancelled'
        self.assertTrue(final.verify(plan,report,CATALOG))

    def test_unresolved_blocks_even_with_successful_validation(self):
        plan=copy.deepcopy(PLAN)
        plan['selected_gates']=[]
        plan['coverage_status']='unresolved'
        plan['unresolved_coverage']=['unreviewed CI workflow impact']
        report=copy.deepcopy(REPORT)
        report['results']=[r for r in report['results'] if r['kind']=='validation']
        self.assertTrue(any('coverage-unresolved' in x for x in final.verify(plan,report,CATALOG)))

    def test_empty_unknown_and_not_required(self):
        plan=copy.deepcopy(PLAN)
        plan['selected_gates']=[]
        self.assertTrue(final.verify(plan,REPORT,CATALOG))
        report=copy.deepcopy(REPORT)
        report['results'].append({'kind':'gate','id':'other','conclusion':'success'})
        self.assertTrue(final.verify(PLAN,report,CATALOG))
        plan=copy.deepcopy(PLAN)
        plan['selected_gates']=['unknown/gate']
        self.assertTrue(final.verify(plan,REPORT,CATALOG))

if __name__ == '__main__':
    unittest.main()
