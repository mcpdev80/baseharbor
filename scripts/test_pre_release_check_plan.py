"""Normal release selection preserves every required check and rejects unknown ones."""
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).parent
spec = importlib.util.spec_from_file_location('release_check_plan', ROOT / 'pre-release-check-plan.py')
plan = importlib.util.module_from_spec(spec)
spec.loader.exec_module(plan)


class ReleaseCheckPlanTests(unittest.TestCase):
    def setUp(self):
        self.inventory = json.loads((ROOT / 'release-requirements.json').read_text())

    def test_every_required_check_is_scheduled_without_reuse(self):
        for release in self.inventory['releases']:
            with self.subTest(release=release):
                matrices = plan.select(self.inventory, release)
                expected = sum(release in g['required_for'] for g in self.inventory['gates'])
                self.assertEqual(expected, sum(len(values) for values in matrices.values()))

    def test_targeted_selection_runs_only_requested_checks(self):
        result = plan.select(self.inventory, 'v0.4.26', 'atomic/docker/lifecycle,integration/static/live-console')
        self.assertEqual(['lifecycle'], result['light_docker'])
        self.assertEqual(['integration/static/live-console'], result['integration'])
        self.assertEqual(2, sum(map(len, result.values())))

    def test_missing_duplicate_or_unknown_selection_cannot_pass(self):
        for request in ['unknown', 'atomic/docker/lifecycle,atomic/docker/lifecycle', ' atomic/docker/lifecycle', ',']:
            with self.subTest(request=request), self.assertRaises(ValueError):
                plan.select(self.inventory, 'v0.4.26', request)
        self.inventory['gates'].append({'id':'atomic/docker/new-unimplemented-check','required_for':['v0.4.26']})
        with self.assertRaisesRegex(ValueError, 'no executor'):
            plan.select(self.inventory, 'v0.4.26')


if __name__ == '__main__':
    unittest.main()
