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
PLAN = {'schema':'baseharbor.pr-gate-plan/v1','release':'v0.4.26','target_base_sha':A,'head_sha':B,'merge_base_sha':C,'integration_sha':None,'selected_gates':['atomic/static/init'],'required_validation_checks':[{'id':'documentation-audit'}]}
CATALOG = {'gates':[{'id':'atomic/static/init','required_for':['v0.4.26']}]}
REPORT = {'schema':'baseharbor.pr-check-results/v1','target_base_sha':A,'head_sha':B,'merge_base_sha':C,'integration_sha':None,'results':[{'kind':'gate','id':'atomic/static/init','conclusion':'success'},{'kind':'validation','id':'documentation-audit','conclusion':'success'}]}

class FinalChecks(unittest.TestCase):
    def test_success(self):
        self.assertEqual([], final.verify(PLAN,REPORT,CATALOG))

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
