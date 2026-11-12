"""Focused Git fixtures for safe pull-request diff-base resolution."""
import importlib.util
import json
import os
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

    def commit_with(self, path, content, message):
        self.write(path, content)
        return self.commit(message)

    def is_ancestor(self, ancestor, descendant):
        return self.git('merge-base', '--is-ancestor', ancestor, descendant,
                        check=False) == ''

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

    def test_unavailable_fetched_target_tip_fails_closed(self):
        result = resolver.resolve_pr_diff(self.root, self.initial, self.initial,
                                          base_tip_sha='f' * 40)
        self.assertTrue(result['broad_required'])
        self.assertEqual('missing_target_tip_history', result['reasons'][0]['code'])
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

    def test_exact_integration_is_accepted(self):
        self.branch('feature')
        self.write('feature.txt', 'feature\n')
        head = self.commit('feature')
        target = self.initial
        integration = self.integration_commit('main', 'feature')

        result = resolver.resolve_pr_diff(self.root, target, head, integration)

        self.assertFalse(result['broad_required'])
        self.assertEqual(integration, result['integration_sha'])
        self.assertEqual([{'status': 'added', 'path': 'feature.txt'}], result['changes'])

    def test_descendant_of_exact_integration_is_rejected(self):
        self.branch('feature')
        self.write('feature.txt', 'feature\n')
        head = self.commit('feature')
        target = self.initial
        self.integration_commit('main', 'feature')
        self.write('unrelated.txt', 'unrelated change\n')
        descendant = self.commit('unrelated change after integration')

        result = resolver.resolve_pr_diff(self.root, target, head, descendant)

        self.assertTrue(result['broad_required'])
        self.assertEqual('integration_sha_mismatch', result['reasons'][0]['code'])
        self.assertEqual([], result['changes'])

    def test_expected_integration_parents_with_modified_tree_is_rejected(self):
        self.branch('feature')
        self.write('feature.txt', 'feature\n')
        head = self.commit('feature')
        target = self.initial
        self.integration_commit('main', 'feature')
        self.write('unrelated.txt', 'injected into merge tree\n')
        self.git('add', '-A')
        forged_tree = self.git('write-tree')
        forged_merge = self.git('commit-tree', forged_tree, '-p', target, '-p', head,
                                input_data='modified merge tree\n')

        result = resolver.resolve_pr_diff(self.root, target, head, forged_merge)

        self.assertTrue(result['broad_required'])
        self.assertEqual('integration_tree_mismatch', result['reasons'][0]['code'])
        self.assertEqual([], result['changes'])

    def test_stale_integration_sha_requires_broad_checks(self):
        self.branch('feature')
        self.write('feature.txt', 'feature\n')
        head = self.commit('feature')

        result = resolver.resolve_pr_diff(self.root, self.initial, head, self.initial)

        self.assertTrue(result['broad_required'])
        self.assertEqual('integration_sha_mismatch', result['reasons'][0]['code'])


    def test_stale_event_base_proceeds_to_tree_check_and_normal_restores_diff(self):
        # The develop-ci contract calls pr-diff-base.py WITHOUT --base-tip. With
        # no fetched base tip, the resolver anchors on the event base.sha and
        # A forward-moved synthetic merge must not be accepted unless the
        # target branch tip is fetched and the lineage can be verified.
        self.branch('feature')
        self.write('feature.txt', 'reviewed feature\\n')
        head = self.commit('feature')
        self.git('switch', '-q', 'main')
        self.write('target-only.txt', 'trusted target advance\\n')
        target = self.commit('trusted target advance')
        tree = self.git('merge-tree', '--write-tree', target, head).splitlines()[0]
        integration = self.git('commit-tree', tree, '-p', target, '-p', head,
                               input_data='GitHub exact integration\\n')
        stale = resolver.resolve_pr_diff(self.root, self.initial, head, integration)
        self.assertTrue(stale['broad_required'])
        self.assertEqual('integration_tree_mismatch', stale['reasons'][0]['code'])
        actual = resolver.resolve_pr_diff(self.root, self.initial, head, integration,
                                          base_tip_sha=target)
        self.assertFalse(actual['broad_required'], actual['reasons'])
        self.assertEqual(target, actual['target_base_sha'])
        self.assertEqual([{'path': 'feature.txt', 'status': 'added'}], actual['changes'])
        self.git('switch', '-q', 'feature')
        self.git('merge', '--no-ff', '-qm', 'normal trusted target merge', target)
        merged = self.git('rev-parse', 'HEAD')
        tree = self.git('merge-tree', '--write-tree', target, merged).splitlines()[0]
        current = self.git('commit-tree', tree, '-p', target, '-p', merged,
                           input_data='new exact GitHub integration\\n')
        actual = resolver.resolve_pr_diff(self.root, target, merged, current)
        self.assertFalse(actual['broad_required'], actual['reasons'])
        self.assertEqual(target, actual['merge_base_sha'])
        self.assertEqual([{'path': 'feature.txt', 'status': 'added'}],
                         actual['changes'])

    def test_legacy_no_tip_accepts_forward_parent_with_same_tree(self):
        self.branch('feature')
        head = self.commit_with('feature.txt', 'feature\n', 'feature')
        self.git('switch', '-q', 'main')
        self.git('commit', '--allow-empty', '-qm', 'target advance without tree change')
        target = self.git('rev-parse', 'HEAD')
        tree = self.git('merge-tree', '--write-tree', target, head).splitlines()[0]
        candidate = self.git('commit-tree', tree, '-p', target, '-p', head,
                             input_data='candidate\n')
        legacy = resolver.resolve_pr_diff(self.root, self.initial, head, candidate)
        self.assertFalse(legacy['broad_required'], legacy['reasons'])
        self.assertEqual(self.initial, legacy['target_base_sha'])
        verified = resolver.resolve_pr_diff(self.root, self.initial, head, candidate,
                                            base_tip_sha=target)
        self.assertFalse(verified['broad_required'], verified['reasons'])
        self.assertEqual(target, verified['target_base_sha'])


    def test_advanced_base_must_belong_to_fetched_target_branch(self):
        self.branch('feature')
        head = self.commit_with('feature.txt', 'feature\n', 'feature')
        self.git('switch', '-q', 'main')
        target = self.commit_with('trusted.txt', 'trusted\n', 'trusted')
        self.branch('side', self.initial)
        side = self.commit_with('side.txt', 'untrusted\n', 'side')
        tree = self.git('merge-tree', '--write-tree', side, head).splitlines()[0]
        integration = self.git('commit-tree', tree, '-p', side, '-p', head,
                               input_data='forged integration\n')
        result = resolver.resolve_pr_diff(self.root, self.initial, head, integration,
                                          base_tip_sha=target)
        self.assertTrue(result['broad_required'])
        self.assertEqual('integration_sha_mismatch', result['reasons'][0]['code'])
        self.assertEqual([], result['changes'])

    def test_advanced_base_requires_exact_clean_integration_tree(self):
        self.branch('feature')
        head = self.commit_with('feature.txt', 'feature\n', 'feature')
        self.git('switch', '-q', 'main')
        target = self.commit_with('trusted.txt', 'trusted\n', 'trusted')
        self.write('injected.txt', 'not in either input\n')
        self.git('add', '-A')
        forged_tree = self.git('write-tree')
        integration = self.git('commit-tree', forged_tree, '-p', target, '-p', head,
                               input_data='forged integration tree\n')
        result = resolver.resolve_pr_diff(self.root, self.initial, head, integration,
                                          base_tip_sha=target)
        self.assertTrue(result['broad_required'])
        self.assertEqual('integration_tree_mismatch', result['reasons'][0]['code'])
        self.assertEqual([], result['changes'])

    def test_workflow_replans_using_verified_advanced_base_policy(self):
        # Exercise the actual workflow shell against immutable Git commits,
        # including an event base older than GitHub's synthetic merge parent.
        helper = Path(__file__).with_name('pr-diff-base.py').read_text()
        # The event-time resolver predates the new command-line contract. It
        # must never be executed merely to discover the candidate's parent.
        self.write('scripts/pr-diff-base.py', 'raise SystemExit("old resolver invoked")\n')
        event_base = self.commit('event base with old resolver')
        self.branch('feature')
        head = self.commit_with('feature.txt', 'feature\n', 'feature')
        self.git('switch', '-q', 'main')
        self.write('scripts/pr-diff-base.py', helper + '\n# actual target policy\n')
        self.write('target-only.txt', 'trusted\n')
        target = self.commit('advanced target with its own trusted resolver')
        tree = self.git('merge-tree', '--write-tree', target, head).splitlines()[0]
        candidate = self.git('commit-tree', tree, '-p', target, '-p', head,
                             input_data='GitHub merge\n')
        # The later fetched branch tip is an ancestry witness, not a source
        # of executable code for this older candidate.
        self.write('scripts/pr-diff-base.py', 'raise SystemExit("tip resolver invoked")\n')
        tip = self.commit('later target with different resolver')
        self.git('update-ref', 'refs/heads/develop', tip)
        self.git('remote', 'add', 'origin', str(self.root))
        self.git('checkout', '-q', '--detach', event_base)
        workflow = (Path(__file__).resolve().parent.parent /
                    '.github/workflows/ci.yml').read_text()
        begin = workflow.index('          git fetch --no-tags origin "$BASE_SHA" "$HEAD_SHA" "$INTEGRATION_SHA" refs/heads/develop:refs/remotes/origin/pr-target-tip')
        end = workflow.index('          # One immutable, locally checked controller', begin)
        shell = '\n'.join(line.removeprefix('          ') for line in
                          workflow[begin:end].splitlines())
        with tempfile.TemporaryDirectory() as runner_temp:
            env = dict(os.environ, BASE_SHA=event_base, HEAD_SHA=head,
                       INTEGRATION_SHA=candidate, RUNNER_TEMP=runner_temp)
            result = subprocess.run(['bash', '-euo', 'pipefail', '-c', shell],
                                    cwd=self.root, env=env, text=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            self.assertEqual(0, result.returncode, result.stderr)
            plan = json.loads((Path(runner_temp) / 'pr-diff.json').read_text())
        self.assertFalse(plan['broad_required'], plan['reasons'])
        self.assertEqual(target, plan['target_base_sha'])
        self.assertEqual(resolver.resolve_pr_diff(self.root, target, head, candidate), plan,
                         'trusted binding must be able to reproduce the exact diff')
        self.assertEqual(target, self.git('rev-parse', 'HEAD'))
        self.assertIn('# actual target policy', (self.root / 'scripts/pr-diff-base.py').read_text())

    def test_final_check_rederives_base_instead_of_trusting_plan_output(self):
        self.branch('feature')
        head = self.commit_with('feature.txt', 'feature\n', 'feature')
        self.git('switch', '-q', 'main')
        target = self.commit_with('target.txt', 'target\n', 'advanced target')
        tree = self.git('merge-tree', '--write-tree', target, head).splitlines()[0]
        candidate = self.git('commit-tree', tree, '-p', target, '-p', head,
                             input_data='GitHub merge\n')
        self.git('update-ref', 'refs/heads/develop', target)
        self.git('remote', 'add', 'origin', str(self.root))
        workflow = (Path(__file__).resolve().parent.parent /
                    '.github/workflows/ci.yml').read_text()
        begin = workflow.index('          git fetch --no-tags origin "$EVENT_BASE"')
        end = workflow.index('          python3 -c \'import json,os;', begin)
        shell = '\n'.join(line.removeprefix('          ') for line in
                          workflow[begin:end].splitlines())
        base_env = dict(os.environ, EVENT_BASE=self.initial, EXPECTED_HEAD=head,
                        EXPECTED_INTEGRATION=candidate, PLAN_BASE=target)
        result = subprocess.run(['bash', '-euo', 'pipefail', '-c', shell],
                                cwd=self.root, env=base_env, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(0, result.returncode, result.stderr)
        result = subprocess.run(['bash', '-euo', 'pipefail', '-c', shell],
                                cwd=self.root, env=dict(base_env, PLAN_BASE=self.initial),
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertNotEqual(0, result.returncode, 'stale plan base must fail closed')

    def test_final_linear_candidate_allows_verified_target_advance(self):
        planned_base = self.commit_with('one.txt', 'one\n', 'planned base')
        later_base = self.commit_with('two.txt', 'two\n', 'later base')
        head = self.commit_with('feature.txt', 'feature\n', 'linear feature')
        self.git('update-ref', 'refs/heads/develop', later_base)
        self.git('remote', 'add', 'origin', str(self.root))
        workflow = (Path(__file__).resolve().parent.parent /
                    '.github/workflows/ci.yml').read_text()
        begin = workflow.index('          git fetch --no-tags origin "$EVENT_BASE"')
        end = workflow.index('          python3 -c \'import json,os;', begin)
        shell = '\n'.join(line.removeprefix('          ') for line in
                          workflow[begin:end].splitlines())
        env = dict(os.environ, EVENT_BASE=self.initial, EXPECTED_HEAD=head,
                   EXPECTED_INTEGRATION=head, PLAN_BASE=planned_base)
        result = subprocess.run(['bash', '-euo', 'pipefail', '-c', shell],
                                cwd=self.root, env=env, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(0, result.returncode, result.stderr)
        # A plan selecting a commit outside target history remains blocked.
        result = subprocess.run(['bash', '-euo', 'pipefail', '-c', shell],
                                cwd=self.root, env=dict(env, PLAN_BASE=head),
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertNotEqual(0, result.returncode)

    def test_reversed_exact_parents_accepted_order_agnostic(self):
        # The authoritative-basis decision relaxes parent ORDER: a merge whose
        # two parents are exactly the known target base and PR head (either
        # order) is accepted; only unknown/divergent parents or wrong history
        # fail closed.
        self.branch('feature')
        self.write('feature.txt', 'feature\\n')
        head = self.commit('feature')
        tree = self.git('merge-tree', '--write-tree', self.initial, head).splitlines()[0]
        reversed_merge = self.git('commit-tree', tree, '-p', head, '-p', self.initial,
                                   input_data='reversed parents\\n')
        result = resolver.resolve_pr_diff(self.root, self.initial, head, reversed_merge)
        self.assertFalse(result['broad_required'], result['reasons'])
        self.assertEqual([{'path': 'feature.txt', 'status': 'added'}], result['changes'])

    def test_extra_parent_rejected_even_with_clean_tree(self):
        self.branch('feature')
        self.write('feature.txt', 'feature\\n')
        head = self.commit('feature')
        tree = self.git('merge-tree', '--write-tree', self.initial, head).splitlines()[0]
        extra = self.git('commit-tree', tree, '-p', self.initial,
                        input_data='unrelated side input\\n')
        integration = self.git('commit-tree', tree, '-p', self.initial, '-p', head,
                               '-p', extra, input_data='extra parent\\n')
        result = resolver.resolve_pr_diff(self.root, self.initial, head, integration)
        self.assertTrue(result['broad_required'])
        self.assertEqual('integration_sha_mismatch', result['reasons'][0]['code'])



    def test_linear_rebased_head_resolves_with_base_tip(self):
        # Mission case (i): a rebased head is a linear descendant of the base
        # branch tip; GitHub exposes github.sha == head with a single parent.
        # With the fetched base tip, the resolver accepts the linear head and
        # diffs merge_base(==base)..head over the unchanged merge-tree check.
        self.branch('feature')
        self.write('feature.txt', 'rebased feature\n')
        head = self.commit('feature')
        self.git('switch', '-q', 'main')
        base_tip = self.commit_with('target-only.txt', 'trusted target advance\n',
                                    'trusted target advance')
        # A rebased/squashed head lands directly on base_tip with a single parent.
        rebased_head = self.git('commit-tree',
                                self.git('rev-parse', head + '^{tree}'),
                                '-p', base_tip, input_data='rebased onto base tip')
        self.assertTrue(self.is_ancestor(base_tip, rebased_head))

        result = resolver.resolve_pr_diff(self.root, self.initial, rebased_head,
                                          rebased_head, base_tip_sha=base_tip)

        self.assertFalse(result['broad_required'], result['reasons'])
        self.assertEqual(rebased_head, result['integration_sha'])
        self.assertEqual(base_tip, result['base_tip_sha'])
        self.assertEqual(base_tip, result['merge_base_sha'])
        self.assertEqual([{'status': 'added', 'path': 'feature.txt'},
                          {'status': 'deleted', 'path': 'target-only.txt'}], result['changes'])

    def test_develop_forward_moved_merge_ref_resolves_with_fetched_tip(self):
        # Mission case (ii): the event merge-ref was built against an OLD target
        # (event base.sha), develop has moved forward, and CI now fetches the new
        # base branch tip. The base parent (old base) is an ancestor of the
        # fetched tip and a descendant of event base.sha -> resolved.
        self.branch('feature')
        self.write('feature.txt', 'reviewed feature\n')
        head = self.commit('feature')
        self.git('switch', '-q', 'main')
        old_base = self.commit_with('old.txt', 'old base state\n', 'old base state')
        tree = self.git('merge-tree', '--write-tree', old_base, head).splitlines()[0]
        integration = self.git('commit-tree', tree, '-p', old_base, '-p', head,
                               input_data='GitHub build against old base\n')
        self.git('switch', '-q', 'main')
        new_base_tip = self.commit_with('more-target.txt', 'develop kept moving\n',
                                        'develop moved forward')
        self.assertTrue(self.is_ancestor(old_base, new_base_tip))

        result = resolver.resolve_pr_diff(self.root, old_base, head, integration,
                                          base_tip_sha=new_base_tip)

        self.assertFalse(result['broad_required'], result['reasons'])
        self.assertEqual(old_base, result['target_base_sha'])
        self.assertEqual([{'status': 'added', 'path': 'feature.txt'}], result['changes'])

    def test_third_parent_on_integration_still_fails_closed(self):
        # Mission case (iii): a forged integration with three parents -> blocked.
        self.branch('feature')
        self.write('feature.txt', 'feature\n')
        head = self.commit('feature')
        self.git('switch', '-q', 'main')
        base_tip = self.commit_with('base.txt', 'base tip advance\n', 'base tip advance')
        self.git('switch', '-q', 'feature')
        self.write('side.txt', 'intruder\n')
        side = self.commit('side input')
        self.git('switch', '-q', 'main')
        self.git('merge', '--no-ff', '-qm', 'forged', 'feature')
        forged = self.git('rev-parse', 'HEAD')
        forged = self.git('commit-tree', self.git('rev-parse', forged + '^{tree}'),
                          '-p', base_tip, '-p', forged, '-p', side,
                          input_data='three parents')

        result = resolver.resolve_pr_diff(self.root, base_tip, forged, forged,
                                          base_tip_sha=base_tip)

        self.assertTrue(result['broad_required'])
        self.assertEqual('integration_sha_mismatch', result['reasons'][0]['code'])

    def test_head_missing_from_integration_still_fails_closed(self):
        # Mission case (iii): the base parent is present but the second parent is
        # an unrelated SHA, not the PR head -> blocked.
        self.branch('feature')
        self.write('feature.txt', 'feature\n')
        head = self.commit('feature')
        self.git('switch', '-q', 'main')
        base_tip = self.commit_with('base.txt', 'base tip\n', 'base tip')
        self.git('switch', '-q', 'feature')
        self.write('other.txt', 'other branch\n')
        other_head = self.commit('other branch')
        tree = self.git('merge-tree', '--write-tree', base_tip, other_head).splitlines()[0]
        forged = self.git('commit-tree', tree, '-p', base_tip, '-p', other_head,
                          input_data='wrong head')

        result = resolver.resolve_pr_diff(self.root, base_tip, head, forged,
                                          base_tip_sha=base_tip)

        self.assertTrue(result['broad_required'])
        self.assertEqual('integration_sha_mismatch', result['reasons'][0]['code'])


if __name__ == '__main__':
    unittest.main()
