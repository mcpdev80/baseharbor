"""Focused Git fixtures for safe pull-request diff-base resolution."""
import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest


spec = importlib.util.spec_from_file_location('pr_diff_base', Path(__file__).with_name('pr-diff-base.py'))
resolver = importlib.util.module_from_spec(spec)
spec.loader.exec_module(resolver)


class GitFixture(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git('init', '-q', '-b', 'main')
        self.git('config', 'user.name', 'Test')
        self.git('config', 'user.email', 'test@example.invalid')
        self.write('shared.txt', 'base\n')
        self.write('delete-me.txt', 'delete\n')
        self.initial = self.commit('initial')

    def git(self, *args, input_data=None, check=True):
        result = subprocess.run(['git', *args], cwd=self.root, input=input_data, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=check)
        return result.stdout.strip()

    def write(self, path, content):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)

    def commit(self, message):
        self.git('add', '-A')
        self.git('commit', '-qm', message)
        return self.git('rev-parse', 'HEAD')

    def branch(self, name, start='HEAD'):
        self.git('switch', '-qc', name, start)

    def integration_commit(self, target, feature):
        self.git('switch', '-q', target)
        self.git('merge', '--no-ff', '-qm', 'integration', feature)
        return self.git('rev-parse', 'HEAD')

    def test_normal_pr_uses_real_merge_base_and_reports_distinct_shas(self):
        self.branch('feature')
        self.write('feature.txt', 'feature\n')
        head = self.commit('feature')
        integration = self.integration_commit('main', 'feature')
        target = self.git('rev-parse', integration + '^1')

        result = resolver.resolve_pr_diff(self.root, target, head, integration)

        self.assertFalse(result['broad_required'])
        self.assertEqual(self.initial, result['merge_base_sha'])
        self.assertEqual(target, result['target_base_sha'])
        self.assertEqual(head, result['head_sha'])
        self.assertEqual(integration, result['integration_sha'])
        self.assertEqual([{'status': 'added', 'path': 'feature.txt'}], result['changes'])

    def test_advanced_target_branch_does_not_enter_pr_diff(self):
        self.branch('feature')
        self.write('feature.txt', 'feature\n')
        head = self.commit('feature')
        self.git('switch', '-q', 'main')
        self.write('target-only.txt', 'advanced target\n')
        target = self.commit('target advance')

        result = resolver.resolve_pr_diff(self.root, target, head)

        self.assertFalse(result['broad_required'])
        self.assertEqual(self.initial, result['merge_base_sha'])
        self.assertEqual([{'status': 'added', 'path': 'feature.txt'}], result['changes'])

    def test_rename_and_delete_are_explicit(self):
        self.branch('feature')
        self.git('mv', 'shared.txt', 'renamed.txt')
        self.git('rm', '-q', 'delete-me.txt')
        head = self.commit('rename and delete')

        result = resolver.resolve_pr_diff(self.root, self.initial, head)

        self.assertFalse(result['broad_required'])
        self.assertIn({'status': 'renamed', 'score': 100,
                       'old_path': 'shared.txt', 'new_path': 'renamed.txt'}, result['changes'])
        self.assertIn({'status': 'deleted', 'path': 'delete-me.txt'}, result['changes'])

    def test_missing_history_requires_broad_checks_with_reason(self):
        result = resolver.resolve_pr_diff(self.root, 'f' * 40, self.initial)

        self.assertTrue(result['broad_required'])
        self.assertEqual('missing_target_base_history', result['reasons'][0]['code'])
        self.assertEqual([], result['changes'])

    def test_conflict_requires_broad_checks(self):
        self.branch('feature')
        self.write('shared.txt', 'feature\n')
        head = self.commit('feature conflict')
        self.git('switch', '-q', 'main')
        self.write('shared.txt', 'target\n')
        target = self.commit('target conflict')

        result = resolver.resolve_pr_diff(self.root, target, head)

        self.assertTrue(result['broad_required'])
        self.assertEqual('integration_conflict', result['reasons'][0]['code'])
        self.assertEqual([], result['changes'])

    def test_multiple_best_merge_bases_require_broad_checks(self):
        tree = self.git('rev-parse', self.initial + '^{tree}')
        a1 = self.git('commit-tree', tree, '-p', self.initial, input_data='a1\n')
        b1 = self.git('commit-tree', tree, '-p', self.initial, input_data='b1\n')
        a2 = self.git('commit-tree', tree, '-p', a1, '-p', b1, input_data='a2\n')
        b2 = self.git('commit-tree', tree, '-p', b1, '-p', a1, input_data='b2\n')

        result = resolver.resolve_pr_diff(self.root, a2, b2)

        self.assertTrue(result['broad_required'])
        self.assertEqual('ambiguous_merge_base', result['reasons'][0]['code'])
        self.assertEqual([], result['changes'])

    def test_stale_integration_sha_requires_broad_checks(self):
        self.branch('feature')
        self.write('feature.txt', 'feature\n')
        head = self.commit('feature')

        result = resolver.resolve_pr_diff(self.root, self.initial, head, self.initial)

        self.assertTrue(result['broad_required'])
        self.assertEqual('integration_sha_mismatch', result['reasons'][0]['code'])


if __name__ == '__main__':
    unittest.main()
