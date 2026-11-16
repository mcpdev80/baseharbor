import importlib.util
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('planner', Path(__file__).with_name('targeted-atomic-plan.py'))
planner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(planner)


class RemainingPlanTests(unittest.TestCase):
    def test_release_requirements_remain_the_only_gate_inventory(self):
        requirements = json.loads(Path(__file__).with_name('release-requirements.json').read_text())
        plan = planner.remaining_plan(requirements, 'v0.4.23')
        actual = ['atomic/' + row['runtime'] + '/' + row['gate'] for row in plan]
        expected = {row['id'] for row in requirements['gates']
                    if 'v0.4.23' in row['required_for']
                    and row['id'].startswith(('atomic/docker/', 'atomic/podman/'))
                    and not row['id'].endswith('/guided')}
        self.assertEqual(set(actual), expected)
        self.assertEqual(len(actual), len(set(actual)))
        self.assertEqual([row['runtime'] for row in plan[:8]], ['docker', 'podman'] * 4)

    def test_unknown_release_and_missing_runtime_fail_closed(self):
        with self.assertRaises(ValueError):
            planner.remaining_plan({'releases': [], 'gates': []}, 'v0.4.23')
        with self.assertRaises(ValueError):
            planner.remaining_plan({'releases': ['v0.4.23'], 'gates': []}, 'v0.4.23')


if __name__ == '__main__':
    unittest.main()
