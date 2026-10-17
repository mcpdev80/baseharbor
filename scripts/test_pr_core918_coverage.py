"""Exact core-918 coverage: real Git merge proof, no path-only bypass."""
import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("core918_planner", ROOT / "pr-gate-plan.py")
planner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(planner)
checks = ['integration/static/public-contracts', 'atomic/docker/lifecycle',
          'atomic/podman/lifecycle']
req = {'releases': ['v0.4.26'],
       'gates': [{'id': name, 'required_for': ['v0.4.26']} for name in checks]}
import json
mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())


class Core918TrustedCoverageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git('init', '-q')
        self.git('config', 'user.name', 'Fixture Reviewer')
        self.git('config', 'user.email', 'review@example.test')
        for path, status in planner.CORE_918_REVIEWED_FILES.items():
            item = self.root / path
            item.parent.mkdir(parents=True, exist_ok=True)
            if status == 'modified':
                item.write_text('base\\n')
        self.git('add', '.')
        self.git('commit', '--allow-empty', '-qm', 'trusted base')
        self.base = self.git('rev-parse', 'HEAD')
        self.git('branch', 'trusted')
        self.git('checkout', '-qb', 'source')
        for name in planner.CORE_918_REVIEWED_FILES:
            item = self.root / name
            item.write_text('reviewed\\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'independently reviewed source change')
        self.reviewed = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-q', 'trusted')
        (self.root / 'policy-marker.txt').write_text('accepted trusted target\\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'advance develop with unrelated policy')
        self.target = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-q', 'source')
        self.git('merge', '--no-ff', '-qm', 'normal develop merge', 'trusted')
        self.merged = self.git('rev-parse', 'HEAD')

    def git(self, *args):
        proc = subprocess.run(['git', '-C', str(self.root), *args],
                              text=True, capture_output=True)
        self.assertEqual(0, proc.returncode, proc.stderr)
        return proc.stdout.strip()

    def input_for(self, head, changes=None):
        tree = self.git('merge-tree', '--write-tree', self.target, head).splitlines()[0]
        integration = self.git('commit-tree', tree, '-p', self.target, '-p', head,
                               '-m', 'validated PR integration')
        changes = changes or [{'path': p, 'status': s}
                              for p, s in planner.CORE_918_REVIEWED_FILES.items()]
        return {'target_base_sha': self.target, 'head_sha': head,
                'merge_base_sha': self.base, 'integration_sha': integration,
                'changes': changes, 'broad_required': False, 'reasons': []}

    def planned(self, head, changes=None, required=req):
        actual = self.input_for(head, changes)
        real = planner.core752_reviewed_merge_chain
        with patch.object(planner, 'core752_reviewed_merge_chain',
                          side_effect=lambda target, sha, reviewed=None:
                          real(target, sha, self.reviewed if reviewed ==
                               planner.CORE_918_REVIEWED_HEAD else reviewed, self.root)):
            return planner.plan(actual, required, mapping)

    def test_unmodified_reviewed_source_and_clean_develop_merge_resolve(self):
        for head in [self.reviewed, self.merged]:
            result = self.planned(head)
            self.assertEqual('resolved', result['coverage_status'])
            self.assertEqual(set(planner.CORE_918_REQUIRED_GATES), set(result['selected_gates']))
            ids = {c['id'] for c in result['required_validation_checks']}
            self.assertTrue({'wizard-app-edit-tests', 'wizard-machine-adapter-tests',
                             'cli-gofmt', 'cli-vet', 'cli-build', 'source-contracts'} <= ids)

    def test_followup_change_is_not_qualified(self):
        (self.root / 'cmd/baha/app_apply.go').write_text('unreviewed change\\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'unreviewed follow-up')
        self.assertEqual('unresolved', self.planned(self.git('rev-parse','HEAD'))['coverage_status'])

    def test_renamed_deleted_extra_and_missing_file_are_denied(self):
        base = [{'path': p, 'status': s}
                for p, s in planner.CORE_918_REVIEWED_FILES.items()]
        for mutated in [base[:-1],
                        base + [{'path': 'cmd/baha/unreviewed.go', 'status': 'added'}],
                        [dict(change, status='deleted') if change['path'] == base[0]['path']
                         else change for change in base],
                        [dict(change, status='renamed', old_path=change['path'],
                              new_path='new.go') if change['path'] == base[0]['path']
                         else change for change in base]]:
            with self.subTest(changes=mutated):
                result=self.planned(self.merged, mutated)
                self.assertEqual('unresolved', result['coverage_status'])

    def test_missing_native_gate_cannot_resolve(self):
        incomplete = {'releases': ['v0.4.26'],
                      'gates': [{'id': 'atomic/docker/lifecycle',
                                 'required_for': ['v0.4.26']}]}
        self.assertEqual('unresolved', self.planned(self.merged, required=incomplete)['coverage_status'])


if __name__ == '__main__':
    unittest.main()
