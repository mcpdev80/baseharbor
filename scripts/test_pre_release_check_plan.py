"""Normal release selection preserves every required check and rejects unknown ones."""
import importlib.util
import json
import shutil
import subprocess
import tempfile
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

    def test_v0428_adds_only_fast_check_and_moves_no_other_gate(self):
        # card 0428-208 / #1746 (Owner authorization 2026-11-23): the ci-fast-check
        # requirement moves to v0.4.28 and no other gate is moved. The v0.4.28 SET is
        # {atomic/static/ci-fast-check, atomic/static/mcp}: the mcp static gate is not
        # moved or changed by this PR — it CARRIES forward into every later release via
        # the release loader's required_for carry (scripts/pre-release-requirements.py),
        # so the selection counts it. The v0.4.28-requirement itself is unchanged; the
        # executor decision (static_gates dispatch shape) lives separately in the suite
        # workflow and is asserted there below.
        self.assertIn('v0.4.28', self.inventory['releases'])
        v0427 = {g['id'] for g in self.inventory['gates'] if 'v0.4.27' in g.get('required_for', [])}
        v0428 = {g['id'] for g in self.inventory['gates'] if 'v0.4.28' in g.get('required_for', [])}
        self.assertEqual(v0428, {'atomic/static/ci-fast-check', 'atomic/static/mcp'})
        self.assertTrue({'atomic/docker/failure'} & v0427)
        selected = plan.select(self.inventory, 'v0.4.28', 'atomic/static/ci-fast-check')
        self.assertEqual(['ci-fast-check'], selected['static'])
        # The PR's delta is additive: besides the new ci-fast-check gate, the base's
        # atomic/static/mcp gate carries into v0.4.28, so two static gates run.
        self.assertEqual(2, sum(map(len, selected.values())))
        workflow = (ROOT.parent / '.github/workflows/pre-release-runtime-suite.yml').read_text()
        self.assertIn('ci-fast-check) bash scripts/ci-fast-check.sh ;;', workflow)

    def test_uncatalogued_static_gate_fails_with_id_and_fix(self):
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            shutil.copy(ROOT / 'pre-release-check-plan.py', temp)
            inventory = json.loads((ROOT / 'release-requirements.json').read_text())
            inventory['gates'].append({
                'id': 'atomic/static/test-uncatalogued',
                'required_for': ['v0.4.28'],
                'jobs': ['static_gates'],
            })
            (temp / 'release-requirements.json').write_text(json.dumps(inventory))
            result = subprocess.run(
                ['python3', str(temp / 'pre-release-check-plan.py'), '--tag', 'v0.4.28'],
                text=True, capture_output=True, check=False,
            )
        self.assertNotEqual(0, result.returncode)
        self.assertIn('atomic/static/test-uncatalogued', result.stderr)
        self.assertIn('register each ID in STATIC and the pre-release-runtime-suite dispatch', result.stderr)

    def test_missing_duplicate_or_unknown_selection_cannot_pass(self):
        for request in ['unknown', 'atomic/docker/lifecycle,atomic/docker/lifecycle', ' atomic/docker/lifecycle', ',']:
            with self.subTest(request=request), self.assertRaises(ValueError):
                plan.select(self.inventory, 'v0.4.26', request)
        self.inventory['gates'].append({'id':'atomic/docker/new-unimplemented-check','required_for':['v0.4.26']})
        with self.assertRaisesRegex(ValueError, 'missing release-suite executor'):
            plan.select(self.inventory, 'v0.4.26')


if __name__ == '__main__':
    unittest.main()
