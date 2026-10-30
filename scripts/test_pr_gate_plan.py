"""Trusted diff-to-check planner contract and fail-closed regression fixtures."""
import importlib.util
import copy
import hashlib
import json
import os
import subprocess
import sys
import tempfile
from unittest.mock import patch
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('planner', ROOT / 'pr-gate-plan.py')
planner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(planner)
wizard_check = planner.wizard_check
MAP = json.loads((ROOT / 'pr-gate-map-v1.json').read_text())
GATES = ['atomic/docker/data-capabilities', 'atomic/podman/data-capabilities',
         'integration/static/public-contracts', 'atomic/docker/lifecycle',
         'atomic/podman/lifecycle', 'journey/docker', 'journey/podman', 'ha/docker/data']
REQ = {'releases': ['v0.4.26'],
       'gates': [{'id': g, 'required_for': ['v0.4.26']} for g in GATES]}


def diff(changes, broad=False):
    return {'target_base_sha': 'a'*40, 'head_sha': 'b'*40,
            'merge_base_sha': 'c'*40, 'integration_sha': 'd'*40,
            'broad_required': broad, 'reasons': [{'code': 'conflict', 'message': 'conflict'}] if broad else [],
            'changes': changes}


def paths(*names):
    return [{'status': 'modified', 'path': path} for path in names]


def planner_core752_merge(root, target, head, reviewed):
    return planner.core752_reviewed_merge_chain(target, head, reviewed, root)


def git_rev_parse(root, ref):
    result = subprocess.run(['git', '-C', str(root), 'rev-parse', ref],
                            text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(result.stderr.strip())
    return result.stdout.strip()


def git_commit_tree(root, target, head):
    tree = subprocess.run(['git', '-C', str(root), 'merge-tree', '--write-tree',
                           target, head], text=True, capture_output=True).stdout.strip().splitlines()[0]
    result = subprocess.run(['git', '-C', str(root), 'commit-tree', tree,
                             '-p', target, '-p', head, '-m', 'pins fixture'],
                            text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(result.stderr.strip())
    return result.stdout.strip()


def git_merge_base(root, a, b):
    result = subprocess.run(['git', '-C', str(root), 'merge-base', a, b],
                            text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(result.stderr.strip())
    return result.stdout.strip()


def git_init(root):
    subprocess.run(['git', '-C', str(root), 'init', '-q'], check=True, capture_output=True)
    subprocess.run(['git', '-C', str(root), 'config', 'user.name', 'pins fixture'],
                   check=True, capture_output=True)
    subprocess.run(['git', '-C', str(root), 'config', 'user.email', 'pins@example.invalid'],
                   check=True, capture_output=True)
    subprocess.run(['git', '-C', str(root), 'config', 'commit.gpgsign', 'false'],
                   check=True, capture_output=True)
    subprocess.run(['git', '-C', str(root), 'checkout', '-b', 'develop'],
                   check=True, capture_output=True)
    subprocess.run(['git', '-C', str(root), 'commit', '-q', '--allow-empty', '-m', 'pins base'],
                   check=True, capture_output=True)


def git_add_commit(root):
    """Commit the working tree (with the added pins file) onto a pins branch.

    develop stays at the clean base so the PR diff is exactly the pins add.
    """
    subprocess.run(['git', '-C', str(root), 'checkout', '-qb', 'pins'],
                   check=True, capture_output=True)
    subprocess.run(['git', '-C', str(root), 'add', '.'], check=True, capture_output=True)
    subprocess.run(['git', '-C', str(root), 'commit', '-q', '-m', 'add consumer-pins'],
                   check=True, capture_output=True)


def validate_pins_for_test(actual, root):
    spec = importlib.util.spec_from_file_location(
        'pr_check_runner', ROOT / 'pr-check-runner.py')
    runner = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(runner)
    return runner.validate_pins_data('pins-data-validation', root)



class AppIDFixtureExactQualificationTests(unittest.TestCase):
    def test_only_exact_reviewed_fixture_source_qualifies(self):
        reviewed = paths(*planner.APPID_FIX_REVIEWED_FILES)
        with patch.object(planner, 'core752_reviewed_merge_chain', return_value=True):
            result = planner.plan(diff(reviewed), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['selected_gates'])
        self.assertTrue({'source-contracts', 'cli-build', 'cli-vet',
                         'cli-guided-tests'}.issubset(
            {c['id'] for c in result['required_validation_checks']}))
        variants = [reviewed[:1], reviewed + paths('cmd/baha/mcp_tools.go'),
                    [{'path': reviewed[0]['path'], 'status': 'deleted'}, reviewed[1]],
                    paths('cmd/baha/unreviewed_test.go',
                          'internal/application/manifest_test.go')]
        for changes in variants:
            with self.subTest(changes=changes), patch.object(
                    planner, 'core752_reviewed_merge_chain', return_value=True):
                self.assertEqual('unresolved',
                    planner.plan(diff(changes), REQ, MAP)['coverage_status'])
        for integration in (None, '0'*40):
            altered = diff(reviewed)
            altered['integration_sha'] = integration
            with patch.object(planner, 'core752_reviewed_merge_chain',
                              return_value=False):
                self.assertEqual('unresolved',
                    planner.plan(altered, REQ, MAP)['coverage_status'])


class AppIDEnvRobustTestExactQualificationTests(unittest.TestCase):
    def test_only_exact_reviewed_test_only_source_qualifies(self):
        self.assertEqual('1ec67f8d4b259d62bd108ee678f25c288b8b0564',
                         planner.APPID_ENV_ROBUST_TEST_REVIEWED_HEAD)
        reviewed = [{'status': status, 'path': path} for path, status
                    in sorted(planner.APPID_ENV_ROBUST_TEST_REVIEWED_FILES.items())]
        with patch.object(planner, 'core752_reviewed_merge_chain', return_value=True):
            result = planner.plan(diff(reviewed), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['selected_gates'])
        self.assertEqual({'source-contracts'},
                         {c['id'] for c in result['required_validation_checks']})
        added = next(ch for ch in reviewed if ch['path'] ==
                     'internal/application/protected_tempdir_test.go')
        status_flipped = copy.deepcopy(reviewed)
        status_flipped[status_flipped.index(added)]['status'] = 'modified'
        variants = [reviewed[:5], reviewed + paths('internal/application/manifest.go'),
                    [{'path': reviewed[0]['path'], 'status': 'deleted'}] + reviewed[1:],
                    status_flipped,
                    paths('internal/application/unreviewed_test.go',
                          *[ch['path'] for ch in reviewed[1:]])]
        for changes in variants:
            with self.subTest(changes=changes), patch.object(
                    planner, 'core752_reviewed_merge_chain', return_value=True):
                self.assertEqual('unresolved',
                    planner.plan(diff(changes), REQ, MAP)['coverage_status'])
        for integration in (None, '0'*40):
            altered = diff(reviewed)
            altered['integration_sha'] = integration
            with patch.object(planner, 'core752_reviewed_merge_chain',
                              return_value=False):
                self.assertEqual('unresolved',
                    planner.plan(altered, REQ, MAP)['coverage_status'])
        with patch.object(planner, 'core752_reviewed_merge_chain', return_value=False):
            self.assertEqual('unresolved',
                planner.plan(diff(reviewed), REQ, MAP)['coverage_status'])


class Core751ImportCoverageProposalTests(unittest.TestCase):
    def test_candidate_selects_real_checks_but_fails_closed_without_source_binding(self):
        changes = [{'path': path, 'status': status}
                   for path, status in planner.INSPECT_IMPORT_CANDIDATE_FILES.items()]
        result = planner.plan(diff(changes), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertEqual(planner.INSPECT_IMPORT_REQUIRED_IDS,
                         {check['id'] for check in result['required_validation_checks']})
        self.assertFalse(result['selected_gates'])
        self.assertTrue(result['unresolved_coverage'])

    def test_extra_rename_delete_and_unreviewed_content_stay_unresolved(self):
        candidate = [{'path': path, 'status': status}
                     for path, status in planner.INSPECT_IMPORT_CANDIDATE_FILES.items()]
        variants = [
            candidate,
            candidate + [{'path': 'internal/repositoryinspect/inspect.go', 'status': 'modified'}],
            candidate[:1],
            [{'path': candidate[0]['path'], 'status': 'deleted'}],
            [{'path': candidate[0]['path'], 'status': 'renamed',
              'old_path': candidate[0]['path'], 'new_path': 'internal/repositoryinspect/new.go'}],
        ]
        for changes in variants:
            with self.subTest(changes=changes):
                result = planner.plan(diff(changes), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
        stale = diff(candidate)
        stale['integration_sha'] = None
        self.assertEqual('unresolved', planner.plan(stale, REQ, MAP)['coverage_status'])


class Core917ExactQualificationTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.repository = Path(temporary.name)
        self.git('init', '-q')
        self.git('config', 'user.name', 'Core917 policy fixture')
        self.git('config', 'user.email', 'fixture@example.test')
        for name, status in planner.CORE_917_REVIEWED_FILES.items():
            path = self.repository / name
            path.parent.mkdir(parents=True, exist_ok=True)
            if status == 'modified':
                path.write_text('baseline\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'trusted base')
        self.git('branch', 'trusted')
        self.git('checkout', '-qb', 'source')
        for name in planner.CORE_917_REVIEWED_FILES:
            (self.repository / name).write_text('reviewed source\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'reviewed twelve-file source')
        self.reviewed = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-q', 'trusted')
        (self.repository / 'trusted-policy.txt').write_text('accepted policy\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'advance trusted target')
        self.target = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-q', 'source')
        self.git('merge', '--no-ff', '-qm', 'clean target integration', 'trusted')
        self.head = self.git('rev-parse', 'HEAD')
        rspec = importlib.util.spec_from_file_location('new_owner_diff', ROOT / 'pr-diff-base.py')
        self.resolver = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(self.resolver)
        self.proof = planner.core752_reviewed_merge_chain

    def git(self, *args):
        result = subprocess.run(['git', '-C', str(self.repository), *args],
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(0, result.returncode, result.stderr)
        return result.stdout.strip()

    def resolved_diff(self, head):
        tree = self.git('merge-tree', '--write-tree', self.target, head).splitlines()[0]
        integration = self.git('commit-tree', tree, '-p', self.target, '-p', head,
                               '-m', 'exact PR integration')
        actual = self.resolver.resolve_pr_diff(self.repository, self.target, head, integration)
        self.assertFalse(actual['broad_required'], actual['reasons'])
        return actual

    def qualify(self, actual, requirements=REQ):
        def proof(target, head, reviewed=None):
            return self.proof(target, head, reviewed, self.repository)
        with patch.object(planner, 'CORE_917_REVIEWED_HEAD', self.reviewed), \
             patch.object(planner, 'core752_reviewed_merge_chain', side_effect=proof):
            return planner.plan(actual, requirements, MAP)

    def test_exact_source_and_real_clean_trusted_merge_require_all_checks(self):
        for head in (self.reviewed, self.head):
            with self.subTest(head=head):
                actual = self.resolved_diff(head)
                result = self.qualify(actual)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertEqual(planner.CORE_917_REQUIRED_GATES, set(result['selected_gates']))
                self.assertEqual({'source-contracts', 'cli-guided-tests', 'cli-vet', 'cli-build',
                                  'cli-gofmt', 'wizard-app-new-tests', 'wizard-controller-tests'},
                                 {check['id'] for check in result['required_validation_checks']})
        self.assertEqual(self.target, self.resolved_diff(self.head)['merge_base_sha'])

    def test_same_paths_changed_content_and_forged_merge_are_rejected(self):
        name = 'internal/deployment/deployment_store.go'
        (self.repository / name).write_text('unreviewed mutation\n')
        self.git('add', '.')
        tree = self.git('write-tree')
        ordinary = self.git('commit-tree', tree, '-p', self.head, '-m', 'unreviewed followup')
        forged = self.git('commit-tree', tree, '-p', self.reviewed, '-p', self.target,
                          '-m', 'unreviewed tree in nominal trusted merge')
        for head in (ordinary, forged):
            with self.subTest(head=head):
                actual = self.resolved_diff(head)
                self.assertEqual(set(planner.CORE_917_REVIEWED_FILES),
                                 {change['path'] for change in actual['changes']})
                self.assertEqual('unresolved', self.qualify(actual)['coverage_status'])

    def test_extra_renamed_deleted_paths_and_status_mismatch_are_rejected(self):
        actual = self.resolved_diff(self.head)
        variants = []
        added = json.loads(json.dumps(actual))
        added['changes'].append({'path': 'internal/deployment/extra.go', 'status': 'added'})
        variants.append(added)
        for change in ({'path': 'internal/deployment/deployment_store.go', 'status': 'deleted'},
                       {'path': 'internal/deployment/deployment_store.go', 'status': 'added'},
                       {'old_path': 'internal/deployment/deployment_store.go',
                        'new_path': 'internal/deployment/renamed.go', 'status': 'renamed'}):
            bad = json.loads(json.dumps(actual))
            index = next(i for i, row in enumerate(bad['changes'])
                         if row.get('path') == 'internal/deployment/deployment_store.go')
            bad['changes'][index] = change
            variants.append(bad)
        for bad in variants:
            with self.subTest(changes=bad['changes']):
                self.assertEqual('unresolved', self.qualify(bad)['coverage_status'])

    def test_invalid_or_missing_integration_and_missing_gate_fail_closed(self):
        actual = self.resolved_diff(self.head)
        old_integration = self.resolved_diff(self.reviewed)['integration_sha']
        stale = self.resolver.resolve_pr_diff(self.repository, self.target,
                                             self.head, old_integration)
        self.assertTrue(stale['broad_required'])
        self.assertEqual('unresolved', self.qualify(stale)['coverage_status'])
        for key, value in (('integration_sha', None), ('integration_sha', 'bad'),
                           ('broad_required', True), ('head_sha', 'f' * 40)):
            with self.subTest(key=key, value=value):
                bad = dict(actual, **{key: value})
                self.assertEqual('unresolved', self.qualify(bad)['coverage_status'])
        catalog = json.loads(json.dumps(REQ))
        catalog['gates'] = [gate for gate in catalog['gates']
                            if gate['id'] != 'atomic/podman/lifecycle']
        self.assertEqual('unresolved', self.qualify(actual, catalog)['coverage_status'])
        requirements = json.loads((ROOT / 'release-requirements.json').read_text())
        result = self.qualify(actual, requirements)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(planner.CORE_917_REQUIRED_GATES, set(result['selected_gates']))


class WizardBootstrapPolicyTests(unittest.TestCase):
    def test_real_trusted_merge_and_unreviewed_followup(self):
        rspec = importlib.util.spec_from_file_location('wizard_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(resolver)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            def git(*args):
                result = subprocess.run(['git', '-C', str(root), *args],
                    text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(0, result.returncode, result.stderr)
                return result.stdout.strip()
            git('init', '-q')
            git('config', 'user.name', 'Wizard policy fixture')
            git('config', 'user.email', 'fixture@example.test')
            for path in planner.WIZARD_CI_REVIEWED_PATHS - {'scripts/test_pr_wizard_workflow_route.py'}:
                file = root / path
                file.parent.mkdir(parents=True, exist_ok=True)
                file.write_text('baseline\n')
            git('add', '.')
            git('commit', '-qm', 'trusted shared baseline')
            git('branch', 'trusted')
            git('checkout', '-qb', 'source')
            for path in planner.WIZARD_CI_REVIEWED_PATHS:
                (root / path).write_text('independently reviewed\n')
            git('add', '.')
            git('commit', '-qm', 'reviewed seven-file wizard CI change')
            reviewed = git('rev-parse', 'HEAD')
            git('checkout', '-q', 'trusted')
            (root / 'accepted-policy.txt').write_text('target policy\n')
            git('add', '.')
            git('commit', '-qm', 'advance trusted target')
            target = git('rev-parse', 'HEAD')
            git('checkout', '-q', 'source')
            git('merge', '--no-ff', '-qm', 'clean trusted-target integration', 'trusted')
            head = git('rev-parse', 'HEAD')
            git('checkout', '-q', 'trusted')
            git('merge', '--no-ff', '-qm', 'verified PR integration', 'source')
            integration = git('rev-parse', 'HEAD')
            actual_proof = planner.core752_reviewed_merge_chain
            def proof(target_sha, head_sha, reviewed_sha=None):
                return actual_proof(target_sha, head_sha, reviewed_sha, root)
            with patch.object(planner, 'WIZARD_CI_REVIEWED_HEAD', reviewed), \
                 patch.object(planner, 'core752_reviewed_merge_chain', side_effect=proof):
                actual = resolver.resolve_pr_diff(root, target, head, integration)
                self.assertFalse(actual['broad_required'], actual['reasons'])
                self.assertEqual(target, actual['merge_base_sha'])
                planned = planner.plan(actual, REQ, MAP)
                self.assertEqual('resolved', planned['coverage_status'])
                self.assertEqual([], planned['selected_gates'])
                self.assertEqual({c['id'] for c in planner.CI_CHECKS},
                                 {c['id'] for c in planned['required_validation_checks']})
                stale = resolver.resolve_pr_diff(root, target, head, reviewed)
                self.assertEqual('unresolved', planner.plan(stale, REQ, MAP)['coverage_status'])
                # A normal extra source commit, even touching an approved path,
                # is not a clean target merge and must lose the qualification.
                git('checkout', '-q', 'source')
                (root / 'scripts/pr-check-runner.py').write_text('unreviewed follow-up\n')
                git('add', '.')
                git('commit', '-qm', 'unreviewed change in the same file inventory')
                changed = git('rev-parse', 'HEAD')
                git('checkout', '-q', '-b', 'negative-integration', target)
                git('merge', '--no-ff', '-qm', 'new PR integration', 'source')
                changed_integration = git('rev-parse', 'HEAD')
                bad = resolver.resolve_pr_diff(root, target, changed, changed_integration)
                self.assertFalse(bad['broad_required'], bad['reasons'])
                self.assertEqual('unresolved', planner.plan(bad, REQ, MAP)['coverage_status'])


class RegistrationBootstrapPolicyTests(unittest.TestCase):
    def test_real_trusted_merge_and_unreviewed_followup(self):
        rspec = importlib.util.spec_from_file_location('wizard_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(resolver)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            def git(*args):
                result = subprocess.run(['git', '-C', str(root), *args],
                    text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(0, result.returncode, result.stderr)
                return result.stdout.strip()
            git('init', '-q')
            git('config', 'user.name', 'Wizard policy fixture')
            git('config', 'user.email', 'fixture@example.test')
            for path in {path for path, status in planner.REGISTRATION_CI_REVIEWED_FILES.items() if status == 'modified'}:
                file = root / path
                file.parent.mkdir(parents=True, exist_ok=True)
                file.write_text('baseline\n')
            git('add', '.')
            git('commit', '-qm', 'trusted shared baseline')
            git('branch', 'trusted')
            git('checkout', '-qb', 'source')
            for path in planner.REGISTRATION_CI_REVIEWED_FILES:
                (root / path).parent.mkdir(parents=True, exist_ok=True)
                (root / path).write_text('independently reviewed\n')
            git('add', '.')
            git('commit', '-qm', 'reviewed six-file registration CI change')
            reviewed = git('rev-parse', 'HEAD')
            git('checkout', '-q', 'trusted')
            (root / 'accepted-policy.txt').write_text('target policy\n')
            git('add', '.')
            git('commit', '-qm', 'advance trusted target')
            target = git('rev-parse', 'HEAD')
            git('checkout', '-q', 'source')
            git('merge', '--no-ff', '-qm', 'clean trusted-target integration', 'trusted')
            head = git('rev-parse', 'HEAD')
            git('checkout', '-q', 'trusted')
            git('merge', '--no-ff', '-qm', 'verified PR integration', 'source')
            integration = git('rev-parse', 'HEAD')
            actual_proof = planner.core752_reviewed_merge_chain
            def proof(target_sha, head_sha, reviewed_sha=None):
                return actual_proof(target_sha, head_sha, reviewed_sha, root)
            with patch.object(planner, 'REGISTRATION_CI_REVIEWED_HEAD', reviewed), \
                 patch.object(planner, 'core752_reviewed_merge_chain', side_effect=proof):
                actual = resolver.resolve_pr_diff(root, target, head, integration)
                self.assertFalse(actual['broad_required'], actual['reasons'])
                self.assertEqual(target, actual['merge_base_sha'])
                planned = planner.plan(actual, REQ, MAP)
                self.assertEqual('resolved', planned['coverage_status'])
                self.assertEqual([], planned['selected_gates'])
                self.assertEqual({c['id'] for c in planner.CI_CHECKS + planner.CODE_CHECKS + planner.CLI_CHECKS},
                                 {c['id'] for c in planned['required_validation_checks']})
                stale = resolver.resolve_pr_diff(root, target, head, reviewed)
                self.assertEqual('unresolved', planner.plan(stale, REQ, MAP)['coverage_status'])
                # A normal extra source commit, even touching an approved path,
                # is not a clean target merge and must lose the qualification.
                git('checkout', '-q', 'source')
                (root / 'scripts/pr-check-runner.py').write_text('unreviewed follow-up\n')
                git('add', '.')
                git('commit', '-qm', 'unreviewed change in the same file inventory')
                changed = git('rev-parse', 'HEAD')
                git('checkout', '-q', '-b', 'negative-integration', target)
                git('merge', '--no-ff', '-qm', 'new PR integration', 'source')
                changed_integration = git('rev-parse', 'HEAD')
                bad = resolver.resolve_pr_diff(root, target, changed, changed_integration)
                self.assertFalse(bad['broad_required'], bad['reasons'])
                self.assertEqual('unresolved', planner.plan(bad, REQ, MAP)['coverage_status'])


class Core921ProductionRegistrationCoverageTests(unittest.TestCase):
    def _checkout_and_git(self, git):
        git('init', '-q')
        git('config', 'user.name', 'core-921 fixture')
        git('config', 'user.email', 'fixture@example.test')

    def _make_reviewed_head(self, git, content, msg):
        # Trusted shared baseline for the files that exist on develop (modified
        # paths only); added paths are created fresh on the reviewed branch.
        for path in {path for path, status in planner.CORE_921_PRODUCTION_REVIEWED_FILES.items() if status == 'modified'}:
            (root / path).parent.mkdir(parents=True, exist_ok=True)
            (root / path).write_text('baseline\n')
        git('add', '.')
        git('commit', '-qm', 'trusted shared baseline')
        git('branch', 'trusted')
        git('checkout', '-qb', 'source')
        for path, status in planner.CORE_921_PRODUCTION_REVIEWED_FILES.items():
            (root / path).parent.mkdir(parents=True, exist_ok=True)
            (root / path).write_text(content)
        git('add', '.')
        git('commit', '-qm', msg)
        return git('rev-parse', 'HEAD')

    def _merge_integration(self, git, target, head):
        git('checkout', '-q', 'trusted')
        (root / 'accepted-policy.txt').write_text('target policy\n')
        git('add', '.')
        git('commit', '-qm', 'advance trusted target')
        target = git('rev-parse', 'HEAD')
        git('checkout', '-q', 'source')
        git('merge', '--no-ff', '-qm', 'clean trusted-target integration', 'trusted')
        head = git('rev-parse', 'HEAD')
        git('checkout', '-q', 'trusted')
        git('merge', '--no-ff', '-qm', 'verified PR integration', 'source')
        integration = git('rev-parse', 'HEAD')
        return target, head, integration

    def test_exact_reviewed_bundle_resolves_with_full_check_set(self):
        rspec = importlib.util.spec_from_file_location('wizard_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(resolver)
        with tempfile.TemporaryDirectory() as temporary:
            global root
            root = Path(temporary)
            def git(*args):
                result = subprocess.run(['git', '-C', str(root), *args],
                    text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(0, result.returncode, result.stderr)
                return result.stdout.strip()
            self._checkout_and_git(git)
            reviewed = self._make_reviewed_head(git, 'independently reviewed\n', 'reviewed core-921 production registration bundle')
            target, head, integration = self._merge_integration(git, None, None)
            actual_proof = planner.core752_reviewed_merge_chain
            def proof(target_sha, head_sha, reviewed_sha=None):
                return actual_proof(target_sha, head_sha, reviewed_sha, root)
            with patch.object(planner, 'CORE_921_PRODUCTION_REVIEWED_HEAD', reviewed), \
                 patch.object(planner, 'core752_reviewed_merge_chain', side_effect=proof):
                actual = resolver.resolve_pr_diff(root, target, head, integration)
                self.assertFalse(actual['broad_required'], actual['reasons'])
                self.assertEqual(target, actual['merge_base_sha'])
                planned = planner.plan(actual, REQ, MAP)
                self.assertEqual('resolved', planned['coverage_status'])
                self.assertEqual([], planned['selected_gates'])
                expected_checks = {c['id'] for c in
                                    planner.CI_CHECKS + planner.CODE_CHECKS +
                                    planner.WORKFLOW_SYNTAX_CHECKS + planner.CLI_CHECKS + planner.DOC_CHECKS +
                                    planner.WIZARD_BASE_CHECKS +
                                    [wizard_check('wizard-registration-tests'),
                                     wizard_check('wizard-machine-adapter-tests'),
                                     wizard_check('wizard-provider-choice-tests'),
                                     wizard_check('wizard-core921-tests')]}
                self.assertEqual(expected_checks,
                                 {c['id'] for c in planned['required_validation_checks']})
                # The affected families must actually be selected to run.
                self.assertIn('wizard-registration-tests',
                              {c['id'] for c in planned['required_validation_checks']})
                self.assertIn('wizard-core921-tests',
                              {c['id'] for c in planned['required_validation_checks']})

    def test_changed_status_and_extra_path_do_not_resolve(self):
        rspec = importlib.util.spec_from_file_location('wizard_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(resolver)
        with tempfile.TemporaryDirectory() as temporary:
            global root
            root = Path(temporary)
            def git(*args):
                result = subprocess.run(['git', '-C', str(root), *args],
                    text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(0, result.returncode, result.stderr)
                return result.stdout.strip()
            self._checkout_and_git(git)
            # Reviewed bundle with a corrupted added handler: an added path is
            # instead deleted, so the changed set is no longer exact and the
            # clause must fail closed even on a clean trusted-target merge.
            reviewed = self._make_reviewed_head(git, 'reviewed\n', 'reviewed core-921 production registration bundle')
            git('checkout', '-q', 'source')
            (root / 'cmd/baha/registration_921_test.go').unlink()
            git('add', '.')
            git('commit', '-qm', 'corrupted core-921 production bundle')
            target, head, integration = self._merge_integration(git, None, None)
            actual_proof = planner.core752_reviewed_merge_chain
            def proof(target_sha, head_sha, reviewed_sha=None):
                return actual_proof(target_sha, head_sha, reviewed_sha, root)
            with patch.object(planner, 'CORE_921_PRODUCTION_REVIEWED_HEAD', reviewed), \
                 patch.object(planner, 'core752_reviewed_merge_chain', side_effect=proof):
                actual = resolver.resolve_pr_diff(root, target, head, integration)
                self.assertFalse(actual['broad_required'], actual['reasons'])
                planned = planner.plan(actual, REQ, MAP)
                self.assertEqual('unresolved', planned['coverage_status'])
                planned = planner.plan(actual, REQ, MAP)
                self.assertEqual('unresolved', planned['coverage_status'])

    def test_mutated_status_and_extra_missing_path_do_not_resolve(self):
        rspec = importlib.util.spec_from_file_location('wizard_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(resolver)
        with tempfile.TemporaryDirectory() as temporary:
            global root
            root = Path(temporary)
            def git(*args):
                result = subprocess.run(['git', '-C', str(root), *args],
                    text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(0, result.returncode, result.stderr)
                return result.stdout.strip()
            self._checkout_and_git(git)
            # Reviewed 15-file bundle, then two corruptions that each break the
            # exact-match contract: a modified handler file is instead marked as
            # an added path (wrong status) and one reference doc is dropped
            # (missing path). The changed set is no longer the exact reviewed
            # set, so the clause fails closed even on a clean trusted-target
            # merge chain.
            reviewed = self._make_reviewed_head(git, 'reviewed\n', 'reviewed core-921 production registration bundle')
            git('checkout', '-q', 'source')
            (root / 'cmd/baha/mcp_tools.go').write_text('reviewed and re-added\n')
            git('add', '.')
            git('commit', '-qm', 'reviewed re-added handler')
            git('checkout', '-q', 'source')
            (root / 'docs/reference/mcp.md').unlink()
            git('add', '.')
            git('commit', '-qm', 'dropped reference doc')
            target, head, integration = self._merge_integration(git, None, None)
            actual_proof = planner.core752_reviewed_merge_chain
            def proof(target_sha, head_sha, reviewed_sha=None):
                return actual_proof(target_sha, head_sha, reviewed_sha, root)
            with patch.object(planner, 'CORE_921_PRODUCTION_REVIEWED_HEAD', reviewed), \
                 patch.object(planner, 'core752_reviewed_merge_chain', side_effect=proof):
                actual = resolver.resolve_pr_diff(root, target, head, integration)
                self.assertFalse(actual['broad_required'], actual['reasons'])
                planned = planner.plan(actual, REQ, MAP)
                self.assertEqual('unresolved', planned['coverage_status'])

    def test_real_unreviewed_commit_on_reviewed_bundle_fails_real_verifier(self):
        # A follow-up unreviewed commit on top of the exact reviewed bundle must
        # stay unresolved under the REAL ancestry verifier (no stubbed proof),
        # because the reviewed head is no longer an ancestor of the PR head. The
        # verifier runs against the fixture's own temporary repository, so the
        # fixture HEADs exist there and the rejection is caused by ancestry, not
        # by a missing commit in the verification path.
        rspec = importlib.util.spec_from_file_location('wizard_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(resolver)
        with tempfile.TemporaryDirectory() as temporary:
            global root
            root = Path(temporary)
            def git(*args):
                result = subprocess.run(['git', '-C', str(root), *args],
                    text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(0, result.returncode, result.stderr)
                return result.stdout.strip()
            self._checkout_and_git(git)
            reviewed = self._make_reviewed_head(git, 'reviewed\n', 'reviewed core-921 production registration bundle')
            # Actual unreviewed commit on top of the reviewed bundle: same file
            # inventory, but the reviewed head is superseded.
            git('checkout', '-q', 'source')
            (root / 'cmd/baha/commands.go').write_text('unreviewed follow-up\n')
            git('add', '.')
            git('commit', '-qm', 'unreviewed follow-up on reviewed bundle')
            unreviewed_head = git('rev-parse', 'HEAD')
            # Sanity: the real verifier (against the fixture's own repository)
            # rejects the unreviewed head as an ancestor of itself, proving the
            # verifier is not stubbed to False.
            self.assertFalse(planner.core752_reviewed_merge_chain(
                unreviewed_head, unreviewed_head, reviewed, root))
            target, head, integration = self._merge_integration(git, None, None)
            actual_proof = planner.core752_reviewed_merge_chain
            def proof(target_sha, head_sha, reviewed_sha=None):
                return actual_proof(target_sha, head_sha, reviewed_sha, root)
            # Real ancestry verifier against the fixture repository (not stubbed,
            # not run against '.'): the clause fails closed because the reviewed
            # head is not an ancestor of the PR head.
            with patch.object(planner, 'CORE_921_PRODUCTION_REVIEWED_HEAD', reviewed), \
                 patch.object(planner, 'core752_reviewed_merge_chain', side_effect=proof):
                actual = resolver.resolve_pr_diff(root, target, head, integration)
                self.assertFalse(actual['broad_required'], actual['reasons'])
                planned = planner.plan(actual, REQ, MAP)
                self.assertEqual('unresolved', planned['coverage_status'])

    def test_extra_missing_path_and_wrong_status_do_not_resolve(self):
        # Wrong status: a modified handler is instead committed as an added path
        # (the path set still contains it but the status is wrong). Extra path:
        # an unrelated file is added. Missing path: a reference doc is dropped.
        # Each breaks the exact reviewed set and fails closed on a clean merge.
        rspec = importlib.util.spec_from_file_location('wizard_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(resolver)
        with tempfile.TemporaryDirectory() as temporary:
            global root
            root = Path(temporary)
            def git(*args):
                result = subprocess.run(['git', '-C', str(root), *args],
                    text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(0, result.returncode, result.stderr)
                return result.stdout.strip()
            self._checkout_and_git(git)
            reviewed = self._make_reviewed_head(git, 'reviewed\n', 'reviewed core-921 production registration bundle')
            # Wrong status + missing/extra paths via a real rename: mcp_tools.go
            # (reviewed as modified) is renamed to a new path, so the diff no
            # longer shows it as the reviewed modified path (it is missing) and
            # a new path appears (extra). The exact reviewed status set is
            # broken, so the clause fails closed on a clean trusted merge.
            git('mv', 'cmd/baha/mcp_tools.go', 'cmd/baha/mcp_tools_renamed.go')
            (root / 'cmd/baha/mcp_tools_renamed.go').write_text('unreviewed\n')
            git('add', '.')
            git('commit', '-qm', 'rename mcp_tools to new path with unreviewed content')
            # Missing path: a reference doc the reviewed set requires is gone.
            (root / 'docs/reference/mcp.md').unlink()
            git('add', '.')
            git('commit', '-qm', 'missing reference doc')
            target, head, integration = self._merge_integration(git, None, None)
            actual_proof = planner.core752_reviewed_merge_chain
            def proof(target_sha, head_sha, reviewed_sha=None):
                return actual_proof(target_sha, head_sha, reviewed_sha, root)
            with patch.object(planner, 'CORE_921_PRODUCTION_REVIEWED_HEAD', reviewed), \
                 patch.object(planner, 'core752_reviewed_merge_chain', side_effect=proof):
                actual = resolver.resolve_pr_diff(root, target, head, integration)
                self.assertFalse(actual['broad_required'], actual['reasons'])
                planned = planner.plan(actual, REQ, MAP)
                self.assertEqual('unresolved', planned['coverage_status'])

    def test_wrong_status_only_stays_unresolved(self):
        # The path set is byte-identical to the reviewed bundle and the merge is
        # a clean trusted-target merge, but one reviewed file is reported with
        # the wrong status (reviewed as 'modified', reported as 'added'). The
        # exact-match contract rejects it purely by status mutation even though
        # the ancestry proof would otherwise pass. The ancestry verifier runs
        # against the fixture's own repository, so this rejection is caused by
        # the status flip, not by a missing commit or broken merge.
        rspec = importlib.util.spec_from_file_location('wizard_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(resolver)
        with tempfile.TemporaryDirectory() as temporary:
            global root
            root = Path(temporary)
            def git(*args):
                result = subprocess.run(['git', '-C', str(root), *args],
                    text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(0, result.returncode, result.stderr)
                return result.stdout.strip()
            self._checkout_and_git(git)
            reviewed = self._make_reviewed_head(git, 'reviewed\n', 'reviewed core-921 production registration bundle')
            target, head, integration = self._merge_integration(git, None, None)
            actual = resolver.resolve_pr_diff(root, target, head, integration)
            # Flip one reviewed 'modified' path to the wrong 'added' status; the
            # path inventory and tree are otherwise unchanged.
            flipped = [dict(change, status='added') if change['path'] == 'cmd/baha/mcp_tools.go'
                       else change for change in actual['changes']]
            actual['changes'] = flipped
            # Sanity: the real verifier still accepts the clean merge, so the
            # ancestry proof is not what fails the clause in this fixture.
            self.assertTrue(planner.core752_reviewed_merge_chain(
                target, head, reviewed, root))
            actual_proof = planner.core752_reviewed_merge_chain
            def proof(target_sha, head_sha, reviewed_sha=None):
                return actual_proof(target_sha, head_sha, reviewed_sha, root)
            with patch.object(planner, 'CORE_921_PRODUCTION_REVIEWED_HEAD', reviewed), \
                 patch.object(planner, 'core752_reviewed_merge_chain', side_effect=proof):
                self.assertFalse(actual['broad_required'], actual['reasons'])
                planned = planner.plan(actual, REQ, MAP)
                self.assertEqual('unresolved', planned['coverage_status'])


class MappingTests(unittest.TestCase):
    def test_edit_navigation_uses_actual_sql_new_edit_gates(self):
        for path in ['cmd/baha/app_edit.go', 'cmd/baha/app_edit_test.go']:
            with self.subTest(path=path):
                result = planner.plan(diff(paths(path)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertEqual(['atomic/docker/data-capabilities', 'atomic/podman/data-capabilities'],
                                 result['selected_gates'])
                self.assertIn('wizard-app-edit-tests',
                              {check['id'] for check in result['required_validation_checks']})

    def test_documentation_requires_validation_but_zero_release_gates(self):
        for name in ['docs/how-to/example.md', 'docs/internal/reference.md',
                     'README.md', 'CHANGELOG.md', 'mkdocs.yml']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['broad_required'])
                self.assertEqual([], result['selected_gates'])
                self.assertEqual(4, len(result['required_validation_checks']))
                self.assertFalse(result['unresolved_coverage'])

    def test_changelog_plus_docs_paths_classify_as_documentation(self):
        result = planner.plan(diff(paths('CHANGELOG.md', 'docs/how-to/example.md',
                  'docs/internal/reference.md', 'README.md',
                  'CONTRIBUTING.md', 'mkdocs.yml')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertFalse(result['broad_required'])
        self.assertEqual([], result['selected_gates'])
        self.assertEqual([], result['unresolved_coverage'])
        checks = {c['id'] for c in result['required_validation_checks']}
        self.assertEqual({'documentation-audit', 'mkdocs-en-strict',
                          'mkdocs-de-strict', 'pages-candidate-build'}, checks)

    def test_code_picks_contract_without_private_consumers(self):
        for name in ['cmd/baha/guided_application_read_context.go', 'contracts/app.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual(['integration/static/public-contracts'],
                                 result['selected_gates'])
                self.assertEqual('resolved', result['coverage_status'])
                self.assertIn('source-contracts', {c['id'] for c in result['required_validation_checks']})

    def test_cli_requires_compilation_and_real_targeted_tests(self):
        result = planner.plan(diff(paths('cmd/baha/guided_application_read_context.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        commands = {c['command'] for c in result['required_validation_checks']}
        self.assertIn('go vet ./cmd/baha', commands)
        self.assertIn('go build ./cmd/baha', commands)
        self.assertIn("go test ./cmd/baha -run '^TestGuided' -count=1", commands)

    def test_mutating_cli_cannot_claim_readonly_coverage(self):
        for name in ['cmd/baha/app_backup_restore.go',
                     'cmd/baha/app_apply.go', 'cmd/baha/app.go',
                     'cmd/baha/new_command.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                commands = {c['command'] for c in result['required_validation_checks']}
                self.assertIn('go build ./cmd/baha', commands)
                self.assertIn('go vet ./cmd/baha', commands)
                self.assertNotIn('journey/docker', result['selected_gates'])

    def test_workflow_risk_does_not_silently_resolve(self):
        for name in ['.github/workflows/ci.yml',
                     '.github/workflows/artifact-gc.yml',
                     '.github/workflows/pre-release.yml',
                     '.github/workflows/docker-runtime-acceptance.yml',
                     '.github/workflows/release.yml',
                     'scripts/ci-evidence.sh',
                     'scripts/verify-journey-evidence.py']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                self.assertIn('ci-executor-verifier-tests',
                              {c['id'] for c in result['required_validation_checks']})
                self.assertEqual([], result['selected_gates'])
                commands = {c['command'] for c in result['required_validation_checks']}
                self.assertTrue(any(c.startswith('PYTHONPATH=scripts python3 -m unittest') for c in commands))

    def test_documentation_workflow_has_validation_without_native_gates(self):
        result = planner.plan(diff(paths('.github/workflows/pages.yml')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['selected_gates'])
        self.assertIn('documentation-audit', {c['id'] for c in result['required_validation_checks']})

    def test_capability_allowlist_runs_package_own_tests(self):
        for name in ['internal/capability/domain_test.go',
                     'internal/capability/integration_contract_test.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                commands = {check['command'] for check in result['required_validation_checks']}
                self.assertIn('go test ./internal/capability/... -count=1', commands)

    def test_reconcile_source_runs_focused_inspect_checks_resolved(self):
        # core-752 permanent lane: only the exact reviewed reconciliation files
        # resolve to the focused package checks, with zero native gates.
        for name in ['internal/repositoryinspect/reconcile.go',
                     'internal/repositoryinspect/reconcile_default_instance_test.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertFalse(result['broad_required'])
                self.assertEqual([], result['selected_gates'])
                ids = {c['id'] for c in result['required_validation_checks']}
                self.assertEqual({'inspect-gofmt', 'inspect-package-tests',
                                  'inspect-vet', 'inspect-consumer-tests',
                                  'inspect-consumer-build'}, ids)
                commands = {c['command'] for c in result['required_validation_checks']}
                self.assertIn('go test ./internal/repositoryinspect/ -count=1', commands)
                self.assertIn("go test ./cmd/baha -run 'Inspect|Reconcile' -count=1", commands)

    def test_inspect_projection_allowlist_runs_focused_inspect_checks_resolved(self):
        # core-750 permanent lane: only the exact reviewed projection files
        # resolve to the focused package checks, with zero native release gates.
        for name in ['internal/repositoryinspect/inspect.go',
                     'internal/repositoryinspect/workload_source_parity_test.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertFalse(result['broad_required'])
                self.assertEqual([], result['selected_gates'])
                ids = {c['id'] for c in result['required_validation_checks']}
                self.assertEqual({'inspect-gofmt', 'inspect-package-tests',
                                  'inspect-vet', 'inspect-consumer-tests',
                                  'inspect-consumer-build'}, ids)
                commands = {c['command'] for c in result['required_validation_checks']}
                self.assertIn('go test ./internal/repositoryinspect/ -count=1', commands)
                self.assertIn("go test ./cmd/baha -run 'Inspect|Reconcile' -count=1", commands)

    def test_unreviewed_inspect_production_stays_fail_closed(self):
        # A blanket resolution of every repositoryinspect file must never be
        # claimed: any unreviewed production file stays unresolved, exactly like
        # capability/application implementation paths.
        for name in ['internal/repositoryinspect/compose_detection.go',
                     'internal/repositoryinspect/workload_source.go',
                     'internal/repositoryinspect/model.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                commands = {c['command'] for c in result['required_validation_checks']}
                self.assertIn('go test ./internal/repositoryinspect/ -count=1', commands)

    def test_resolver_routing_allowlist_runs_focused_resolver_checks_resolved(self):
        # core-1220 permanent lane: only the exact reviewed resolver files resolve
        # to the focused package checks, with zero native release gates.
        for name in ['internal/runtime/resolver/resolver.go',
                     'internal/runtime/resolver/resolver_test.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertFalse(result['broad_required'])
                self.assertEqual([], result['selected_gates'])
                ids = {c['id'] for c in result['required_validation_checks']}
                self.assertEqual({'resolver-gofmt', 'resolver-package-tests',
                                  'resolver-vet', 'resolver-consumer-build'}, ids)
                commands = {c['command'] for c in result['required_validation_checks']}
                self.assertIn('go test ./internal/runtime/resolver/ -count=1', commands)
                self.assertIn('go build -o "$RUNNER_TEMP/baha-resolver" ./cmd/baha', commands)

    def test_unreviewed_resolver_routing_stays_fail_closed(self):
        # A blanket resolution of every resolver file must never be claimed:
        # any unreviewed production file stays unresolved, exactly like the
        # inspect/capability/application implementation paths.
        for name in ['internal/runtime/resolver/extra.go',
                     'internal/runtime/resolver/plan.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                commands = {c['command'] for c in result['required_validation_checks']}
                self.assertIn('go test ./internal/runtime/resolver/ -count=1', commands)

    def test_reconcile_with_other_package_source_stays_fail_closed(self):
        # A reviewed reconcile change bundled with an unreviewed inspect file
        # must never inherit the narrow resolved coverage.
        result = planner.plan(diff(paths(
            'internal/repositoryinspect/reconcile.go',
            'internal/repositoryinspect/detector.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['broad_required'])
        self.assertIn('detector.go', str(result['unresolved_coverage']))
        ids = {c['id'] for c in result['required_validation_checks']}
        self.assertEqual({'inspect-gofmt', 'inspect-package-tests', 'inspect-vet',
                          'inspect-consumer-tests', 'inspect-consumer-build'}, ids)

    def test_non_reconcile_inspect_source_remains_unresolved(self):
        # No blanket permission for arbitrary repositoryinspect changes: a new
        # reconcile*-named production file, an unreviewed file, and a nested
        # subpackage all stay fail-closed unresolved.
        for name in ['internal/repositoryinspect/detector.go',
                     'internal/repositoryinspect/model.go',
                     'internal/repositoryinspect/findings.go',
                     'internal/repositoryinspect/manifest.go',
                     'internal/repositoryinspect/reconcile_security.go',
                     'internal/repositoryinspect/reconcile_other_test.go',
                     'internal/repositoryinspect/subpackage/reconcile_security.go',
                     'internal/repositoryinspectx/reconcile.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                ids = {c['id'] for c in result['required_validation_checks']}
                if name.startswith('internal/repositoryinspect/'):
                    self.assertEqual({'inspect-gofmt', 'inspect-package-tests',
                                      'inspect-vet', 'inspect-consumer-tests',
                                      'inspect-consumer-build'}, ids)

    def test_application_and_capability_production_remain_unresolved(self):
        for name in ['internal/application/manifest.go',
                     'internal/application/footprint.go',
                     'internal/application/provider_union.go',
                     'internal/capability/registry.go',
                     'internal/capability/lifecycle.go',
                     'internal/capability/provider_placement.go',
                     'internal/capability/registry_test.go',
                     'internal/capability/new_provider.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                self.assertNotEqual(set(GATES), set(result['selected_gates']))
                if name.startswith('internal/capability/'):
                    commands = {c['command'] for c in result['required_validation_checks']}
                    self.assertIn('go test ./internal/capability/... -count=1', commands)

    def test_docker_deployment_scripts_cannot_claim_lifecycle_only_coverage(self):
        for name in ['deploy/docker/service.yaml',
                     'deploy/docker/backup.sh',
                     'deploy/docker/security.yml',
                     'deploy/docker/production-setup.sh']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                self.assertNotEqual(['atomic/docker/lifecycle'], result['selected_gates'])

    def test_ci_evidence_selects_policy_planner_executor_verifier(self):
        for name in ['scripts/pr-gate-plan.py', 'scripts/test_pr_gate_plan.py']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertEqual([], result['selected_gates'])
                self.assertEqual({'ci-policy', 'pr-planner-tests', 'ci-executor-verifier-tests',
                                  'receipt-execution-tests'},
                                 {c['id'] for c in result['required_validation_checks']})

    def test_unknown_never_auto_selects_full_release_catalog(self):
        for name in ['go.mod', 'Dockerfile', 'internal/operatorauth/policy.go',
                     'config/deleted.yaml', 'spec/generated.pb.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                self.assertNotEqual(set(GATES), set(result['selected_gates']))

    def test_rename_and_delete_paths_are_not_lost(self):
        result = planner.plan(diff([
            {'status': 'renamed', 'old_path': 'docs/a.md', 'new_path': 'go.mod'},
            {'status': 'deleted', 'path': 'docs/deleted.md'}]), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertEqual(4, len(result['required_validation_checks']))
        self.assertEqual([], result['selected_gates'])
        self.assertIn('go.mod', str(result['unresolved_coverage']))

    def test_invalid_and_empty_inputs_fail_closed_without_matrix(self):
        base = diff(paths('docs/readme.md'))
        mutations = [
            {'head_sha': 'abc'}, {'merge_base_sha': None},
            {'target_base_sha': 4}, {'integration_sha': False},
            {'broad_required': 'false'}, {'changes': [None]},
            {'reasons': 'invalid'}, {'changes': []},
            {'changes': [{'status': 'unmerged', 'path': 'docs/x.md'}]},
            {'changes': [{'status': 'added', 'path': '../docs/x.md'}]},
            {'broad_required': True},
        ]
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                candidate = dict(base)
                candidate.update(mutation)
                result = planner.plan(candidate, REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                self.assertTrue(result['broad_required'])

    def test_immutable_diff_inputs_and_optional_integration(self):
        candidate = diff(paths('docs/readme.md'))
        candidate['integration_sha'] = None
        result = planner.plan(candidate, REQ, MAP)
        for key in ('target_base_sha', 'head_sha', 'merge_base_sha', 'integration_sha'):
            self.assertEqual(candidate[key], result[key])

    def test_missing_mapped_catalog_gate_is_unresolved(self):
        catalog = {'releases': ['v0.4.26'], 'gates': [
            {'id': 'journey/podman', 'required_for': ['v0.4.26']}]}
        result = planner.plan(diff(paths('cmd/baha/app.go')), catalog, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertEqual([], result['selected_gates'])

    def test_exact_core893_review_bundle_requires_original_three_gates(self):
        reviewed = sorted(planner.AUDITED_CORE_893_PATHS)
        inventory = json.loads((ROOT / 'release-requirements.json').read_text())
        expected = planner.CORE_893_GATE_QUALIFICATION
        trusted = diff(paths(*reviewed))
        trusted['head_sha'] = planner.CORE_893_REVIEWED_HEAD_SHA
        trusted['merge_base_sha'] = planner.CORE_893_TRUSTED_MERGE_BASE_SHA
        complete = planner.plan(trusted, inventory, MAP)
        self.assertEqual('resolved', complete['coverage_status'])
        self.assertEqual(expected, set(complete['selected_gates']))
        self.assertEqual({'pr-planner-tests', 'ci-policy', 'ci-executor-verifier-tests',
                          'receipt-execution-tests'},
                         {c['id'] for c in complete['required_validation_checks']})
        for changed in (reviewed[:-1], reviewed + ['scripts/ci-unknown.py'],
                        ['.github/workflows/ci.yml']):
            with self.subTest(changed=changed):
                result = planner.plan(dict(trusted, changes=paths(*changed)), inventory, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
        for override in ({'head_sha': 'f' * 40},
                         {'merge_base_sha': 'e' * 40},
                         {'integration_sha': None},
                         {'integration_sha': 'invalid'},
                         {'broad_required': True, 'reasons': [{'code': 'integration_sha_mismatch',
                           'message': 'wrong integration parents'}]}):
            with self.subTest(override=override):
                result = planner.plan(dict(trusted, **override), inventory, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
        # Same complete path set, different unreviewed commit contents:
        # the immutable commit SHA is different and MUST NOT inherit qualification.
        unreviewed = dict(trusted, head_sha='c' * 40)
        self.assertEqual(set(reviewed), set(x['path'] for x in unreviewed['changes']))
        self.assertEqual('unresolved', planner.plan(unreviewed, inventory, MAP)['coverage_status'])
        insufficient = planner.plan(trusted, REQ, MAP)
        self.assertEqual('unresolved', insufficient['coverage_status'])

    def test_core752_real_git_merge_preserves_reviewed_qualification(self):
        # Construct real commits and resolve the *actual* diff/integration,
        # not substituted SHA strings. All work stays in a temporary repository.
        resolver_spec = importlib.util.spec_from_file_location('resolver', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(resolver_spec)
        resolver_spec.loader.exec_module(resolver)
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            def git(*args):
                return subprocess.check_output(['git', '-C', str(root), *args], text=True).strip()
            subprocess.run(['git', 'init', '-q', str(root)], check=True)
            git('config', 'user.email', 'test@example.invalid')
            git('config', 'user.name', 'CI Fixture')
            (root / 'base.txt').write_text('initial\\n')
            git('add', '.')
            git('commit', '-qm', 'trusted base')
            base = git('rev-parse', 'HEAD')
            git('branch', 'target')
            git('checkout', '-qb', 'source')
            for path in planner.CORE_752_CI_PATHS:
                p = root / path
                p.parent.mkdir(parents=True, exist_ok=True)
                p.write_text('old\\n')
            git('add', '.')
            git('commit', '-qm', 'unreviewed baseline scaffolding')
            # Both branches share the exact old files at the review base.
            shared = git('rev-parse', 'HEAD')
            git('branch', '-f', 'target', shared)
            for path in planner.CORE_752_CI_PATHS:
                (root / path).write_text('reviewed\\n')
            git('add', '.')
            git('commit', '-qm', 'independently reviewed CI change')
            reviewed = git('rev-parse', 'HEAD')
            git('checkout', 'target')
            (root / 'base.txt').write_text('trusted target update\\n')
            git('add', '.')
            git('commit', '-qm', 'trusted policy on develop')
            target = git('rev-parse', 'HEAD')
            git('checkout', 'source')
            git('merge', '--no-ff', '-m', 'normally merge trusted develop', 'target')
            merged_head = git('rev-parse', 'HEAD')
            # Exact simulated PR integration: first parent target, second PR head.
            git('checkout', '-q', 'target')
            git('merge', '--no-ff', '--no-commit', 'source')
            git('commit', '-qm', 'synthetic PR integration')
            integration = git('rev-parse', 'HEAD')
            verified_chain = planner.core752_reviewed_merge_chain
            with patch.object(planner, 'CORE_752_CI_HEAD', reviewed), \
                 patch.object(planner, 'CORE_752_CI_MERGE_BASE', shared), \
                 patch.object(planner, 'core752_reviewed_merge_chain',
                              side_effect=lambda target_sha, head_sha:
                              verified_chain(target_sha, head_sha, reviewed, root)):
                actual = resolver.resolve_pr_diff(root, target, merged_head, integration)
                self.assertFalse(actual['broad_required'], actual['reasons'])
                self.assertEqual(target, actual['merge_base_sha'])
                qualified = planner.plan(actual, REQ, MAP)
                self.assertEqual('resolved', qualified['coverage_status'])
                self.assertEqual([], qualified['selected_gates'])
                self.assertEqual({'pr-planner-tests','ci-policy','ci-executor-verifier-tests',
                                  'receipt-execution-tests'},
                                 {v['id'] for v in qualified['required_validation_checks']})
                # Unreviewed source changes, including SAME FIVE PATHS, fail.
                git('checkout', 'source')
                (root / sorted(planner.CORE_752_CI_PATHS)[0]).write_text('unreviewed\\n')
                git('add', '.')
                git('commit', '-qm', 'unreviewed modification of same paths')
                unreviewed = git('rev-parse', 'HEAD')
                self.assertFalse(verified_chain(target, unreviewed, reviewed, root))
                bad_diff = resolver.resolve_pr_diff(root, target, unreviewed)
                self.assertEqual('unresolved', planner.plan(bad_diff, REQ, MAP)['coverage_status'])

    def test_actual_repository_release_catalog(self):
        requirements = json.loads((ROOT / 'release-requirements.json').read_text())
        self.assertEqual(65, len([g for g in requirements['gates']
                                  if 'v0.4.26' in g['required_for']]))
        docs = planner.plan(diff(paths('docs/how-to/guide.md')), requirements, MAP)
        self.assertEqual('resolved', docs['coverage_status'])
        self.assertEqual([], docs['selected_gates'])
        self.assertEqual(4, len(docs['required_validation_checks']))
        unknown = planner.plan(diff(paths('go.mod')), requirements, MAP)
        self.assertEqual('unresolved', unknown['coverage_status'])
        self.assertNotEqual(65, len(unknown['selected_gates']))

    def test_consumer_contract_is_fail_closed(self):
        result = planner.plan(diff(paths('go.mod')), REQ, MAP)
        self.assertIn('MUST fail closed', result['validation_contract'])
        self.assertIn('zero selected_gates is valid', result['validation_contract'])


    def test_first_wave_wizard_files_select_exact_hosted_profiles(self):
        profiles = {
            'wizard-app-edit-load-tests': [
                'cmd/baha/app_edit_load.go', 'cmd/baha/app_edit_load_test.go'],
            'wizard-capability-selection-tests': [
                'cmd/baha/app_capability_selection.go', 'cmd/baha/app_capability_selection_test.go'],
            'wizard-footprint-presenter-tests': [
                'cmd/baha/app_footprint_presenter.go', 'cmd/baha/app_footprint_presenter_test.go'],
            'wizard-change-presentation-tests': [
                'cmd/baha/app_change_preview.go', 'cmd/baha/app_change_preview_test.go',
                'cmd/baha/app_change_result_presenter.go',
                'cmd/baha/app_change_result_presenter_test.go'],
        }
        for expected, names in profiles.items():
            for name in names:
                with self.subTest(path=name):
                    result = planner.plan(diff(paths(name)), REQ, MAP)
                    self.assertEqual('resolved', result['coverage_status'])
                    self.assertEqual([], result['selected_gates'])
                    ids = {row['id'] for row in result['required_validation_checks']}
                    self.assertIn(expected, ids)
                    self.assertTrue({'source-contracts', 'cli-gofmt',
                                     'cli-vet', 'cli-build'} <= ids)
                    self.assertNotIn('cli-guided-tests', ids)

    def test_wizard_profile_accepts_ordinary_trusted_target_integration(self):
        candidate = diff(paths('cmd/baha/app_edit_load.go',
                               'cmd/baha/app_edit_load_test.go'))
        candidate['target_base_sha'] = 'e' * 40
        candidate['head_sha'] = 'f' * 40
        candidate['merge_base_sha'] = '1' * 40
        candidate['integration_sha'] = '2' * 40
        result = planner.plan(candidate, REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertIn('wizard-app-edit-load-tests',
                      {row['id'] for row in result['required_validation_checks']})

    def test_wizard_unknown_nested_and_boundary_mutations_fail_closed(self):
        for change in [
            [{'status': 'added', 'path': 'cmd/baha/wizard/app_edit_load.go'}],
            [{'status': 'added', 'path': 'cmd/baha/app_edit_load_helper.go'}],
            [{'status': 'deleted', 'path': 'cmd/baha/app_edit_load_test.go'}],
            [{'status': 'type_changed', 'path': 'cmd/baha/app_capability_selection.go'}],
            [{'status': 'renamed', 'old_path': 'cmd/baha/app_edit_load.go',
              'new_path': 'cmd/baha/app_edit_load_v2.go'}],
        ]:
            with self.subTest(change=change):
                result = planner.plan(diff(change), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])

    def test_later_mutating_wizard_wiring_selects_only_targeted_lifecycle_gates(self):
        result = planner.plan(diff(paths('cmd/baha/app_edit_apply.go',
                                         'cmd/baha/app_edit_apply_test.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'],
                         result['selected_gates'])
        self.assertIn('wizard-app-edit-tests',
                      {row['id'] for row in result['required_validation_checks']})

    def test_later_readonly_profiles_remain_runtime_free(self):
        for name, check_id in [
            ('cmd/baha/app_provider_choice.go', 'wizard-provider-choice-tests'),
            ('cmd/baha/app_wizard_controller.go', 'wizard-controller-tests'),
            ('cmd/baha/app_footprint_show.go', 'wizard-footprint-command-tests'),
            ('cmd/baha/app_machine_adapters.go', 'wizard-machine-adapter-tests'),
        ]:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertEqual([], result['selected_gates'])
                self.assertIn(check_id, {row['id'] for row in result['required_validation_checks']})

class ReceiptControlPlaneMappingTests(unittest.TestCase):
    def test_receipt_control_plane_script_maps_to_ci_class(self):
        for path in ['scripts/receipt_writer.py', 'scripts/receipt_verifier.py',
                     'scripts/receipt_tasklink.py']:
            with self.subTest(path=path):
                result = planner.plan(diff(paths(path)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                checks = {c['id'] for c in result['required_validation_checks']}
                self.assertEqual({'pr-planner-tests', 'ci-policy',
                                  'ci-executor-verifier-tests',
                                  'receipt-execution-tests'}, checks)

    def test_receipt_control_plane_test_script_maps_to_ci_class(self):
        for path in ['scripts/test_receipt_writer.py', 'scripts/test_receipt_verifier.py',
                     'scripts/test_receipt_chain.py',
                     'scripts/test_ci_workflow_policy_receipts.py',
                     'scripts/test_receipt_tasklink.py',
                     'scripts/test_receipt_postrun.py']:
            with self.subTest(path=path):
                result = planner.plan(diff(paths(path)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                checks = {c['id'] for c in result['required_validation_checks']}
                self.assertEqual({'pr-planner-tests', 'ci-policy',
                                  'ci-executor-verifier-tests',
                                  'receipt-execution-tests'}, checks)

    def test_receipt_control_plane_workflow_maps_ci_and_actionlint(self):
        for path in ['.github/workflows/receipt-approval.yml',
                     '.github/workflows/receipt-post-run.yml',
                     '.github/workflows/receipt-shadow.yml']:
            with self.subTest(path=path):
                result = planner.plan(diff(paths(path)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                checks = {c['id'] for c in result['required_validation_checks']}
                self.assertEqual({'ci-policy', 'ci-actionlint', 'pr-planner-tests',
                                  'ci-executor-verifier-tests',
                                  'receipt-execution-tests'}, checks)

    def test_ci_workflow_policy_script_maps_to_ci_class(self):
        path = 'scripts/ci-workflow-policy.sh'
        result = planner.plan(diff(paths(path)), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['unresolved_coverage'])
        self.assertEqual([], result['selected_gates'])
        checks = {c['id'] for c in result['required_validation_checks']}
        self.assertEqual({'pr-planner-tests', 'ci-policy',
                          'ci-executor-verifier-tests',
                          'receipt-execution-tests'}, checks)

    def test_unlisted_receipt_source_stays_unresolved(self):
        result = planner.plan(diff(paths('scripts/receipt_producer.py')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])
        self.assertTrue(any('receipt_producer.py' in item for item in
                            result['unresolved_coverage']))

    def test_extra_unrelated_change_fails_closed(self):
        result = planner.plan(diff(paths('scripts/receipt_writer.py',
                                          'cmd/baha/app_new_command.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(any('cmd/baha/app_new_command.go' in item for item in
                            result['unresolved_coverage']))


# Registration coverage is part of the trusted planner qualification suite.
try:
    from scripts.test_pr_registration_ci import RegistrationPlannerTests, RegistrationExecutorTests, RegistrationWorkflowRoute, RegistrationActualPackageSmoke
except ModuleNotFoundError:
    from test_pr_registration_ci import RegistrationPlannerTests, RegistrationExecutorTests



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
        spec = importlib.util.spec_from_file_location('core918_diff_owner', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(resolver)
        tree = self.git('merge-tree', '--write-tree', self.target, head).splitlines()[0]
        integration = self.git('commit-tree', tree, '-p', self.target, '-p', head,
                               '-m', 'validated PR integration')
        actual = resolver.resolve_pr_diff(self.root, self.target, head, integration)
        if changes is not None:
            actual['changes'] = changes
        return actual

    def planned(self, head, changes=None, required=REQ):
        actual = self.input_for(head, changes)
        real = planner.core752_reviewed_merge_chain
        with patch.object(planner, 'core752_reviewed_merge_chain',
                          side_effect=lambda target, sha, reviewed=None:
                          real(target, sha, self.reviewed if reviewed ==
                               planner.CORE_918_REVIEWED_HEAD else reviewed, self.root)):
            return planner.plan(actual, required, MAP)

    def test_unmodified_reviewed_source_and_clean_develop_merge_resolve(self):
        for head in [self.reviewed, self.merged]:
            actual = self.input_for(head)
            self.assertFalse(actual['broad_required'], actual['reasons'])
            self.assertEqual(self.target if head == self.merged else self.base,
                             actual['merge_base_sha'])
            self.assertEqual(planner.CORE_918_REVIEWED_FILES,
                             {c['path']: c['status'] for c in actual['changes']})
            result = self.planned(head)
            self.assertEqual('resolved', result['coverage_status'])
            self.assertEqual(set(planner.CORE_918_REQUIRED_GATES), set(result['selected_gates']))
            ids = {c['id'] for c in result['required_validation_checks']}
            self.assertTrue({'wizard-app-edit-tests', 'wizard-machine-adapter-tests',
                             'cli-gofmt', 'cli-vet', 'cli-build', 'source-contracts',
                             'wizard-core918-native-sql-safety-tests'} <= ids)

    def test_followup_change_is_not_qualified(self):
        (self.root / 'cmd/baha/app_apply.go').write_text('unreviewed change\\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'unreviewed follow-up')
        actual = self.input_for(self.git('rev-parse', 'HEAD'))
        self.assertFalse(actual['broad_required'], actual['reasons'])
        self.assertEqual(self.target, actual['merge_base_sha'])
        self.assertEqual('unresolved', self.planned(actual['head_sha'])['coverage_status'])

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


class Core754ExecutionWithoutQualificationTests(unittest.TestCase):
    def test_exact_candidate_selects_both_tests_but_remains_unresolved(self):
        candidate = diff([{'status': 'added', 'path': planner.CORE_754_TEST_PATH}])
        candidate['head_sha'] = planner.CORE_754_TEST_HEAD
        planned = planner.plan(candidate, REQ, MAP)
        checks = {row['id'] for row in planned['required_validation_checks']}
        self.assertIn(planner.CORE_754_TEST_CHECK, checks)
        self.assertTrue({'cli-gofmt', 'cli-vet', 'cli-build'} <= checks)
        self.assertEqual('unresolved', planned['coverage_status'])
        self.assertTrue(planned['unresolved_coverage'])
        self.assertEqual([], planned['selected_gates'])

    def test_actual_normal_develop_merge_retains_execution_only(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            def git(*args):
                result = subprocess.run(['git', '-C', str(root), *args],
                                        text=True, stdout=subprocess.PIPE,
                                        stderr=subprocess.PIPE)
                self.assertEqual(result.returncode, 0, result.stderr)
                return result.stdout.strip()
            git('init', '-q')
            git('config', 'user.name', 'Core 754 fixture')
            git('config', 'user.email', 'fixture@example.test')
            (root / 'baseline.txt').write_text('base\\n')
            git('add', '.')
            git('commit', '-qm', 'trusted base')
            git('branch', 'trusted')
            git('checkout', '-qb', 'source')
            path = root / planner.CORE_754_TEST_PATH
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text('two expected MCP test families\\n')
            git('add', '.')
            git('commit', '-qm', 'reviewed core-754 candidate')
            reviewed = git('rev-parse', 'HEAD')
            git('checkout', '-q', 'trusted')
            (root / 'trusted.txt').write_text('independently integrated CI runner\\n')
            git('add', '.')
            git('commit', '-qm', 'advance develop with reviewed CI')
            target = git('rev-parse', 'HEAD')
            git('checkout', '-q', 'source')
            git('merge', '--no-ff', '-qm', 'normal develop integration', 'trusted')
            merged = git('rev-parse', 'HEAD')
            proof = planner.core752_reviewed_merge_chain
            def actual_proof(base, head, reviewed_sha=None):
                return proof(base, head, reviewed_sha, root)
            self.assertTrue(actual_proof(target, merged, reviewed))
            candidate = diff([{'status': 'added', 'path': planner.CORE_754_TEST_PATH}])
            candidate.update({'target_base_sha': target, 'head_sha': merged})
            with patch.object(planner, 'CORE_754_TEST_HEAD', reviewed), \
                 patch.object(planner, 'core752_reviewed_merge_chain', side_effect=actual_proof):
                planned = planner.plan(candidate, REQ, MAP)
                self.assertIn(planner.CORE_754_TEST_CHECK,
                              {c['id'] for c in planned['required_validation_checks']})
                self.assertEqual('unresolved', planned['coverage_status'])
                self.assertNotIn('core-754 test source differs from exact reviewed candidate',
                                 str(planned['unresolved_coverage']))
                path.write_text('unreviewed post-merge modification\\n')
                git('add', '.')
                git('commit', '-qm', 'unreviewed follow-up')
                candidate['head_sha'] = git('rev-parse', 'HEAD')
                rejected = planner.plan(candidate, REQ, MAP)
                self.assertEqual('unresolved', rejected['coverage_status'])
                self.assertIn('core-754 test source differs from exact reviewed candidate',
                              str(rejected['unresolved_coverage']))

    def test_clean_develop_merge_keeps_execution_unresolved(self):
        candidate = diff([{'status': 'added', 'path': planner.CORE_754_TEST_PATH}])
        candidate['head_sha'] = 'e' * 40
        with patch.object(planner, 'core752_reviewed_merge_chain',
                          side_effect=lambda base, head, reviewed=None:
                              reviewed == planner.CORE_754_TEST_HEAD) as proof:
            result = planner.plan(candidate, REQ, MAP)
        self.assertIn(((candidate['target_base_sha'], candidate['head_sha'],
                        planner.CORE_754_TEST_HEAD), {}),
                      [(call.args, call.kwargs) for call in proof.call_args_list])
        self.assertIn(planner.CORE_754_TEST_CHECK,
                      {check['id'] for check in result['required_validation_checks']})
        self.assertEqual('unresolved', result['coverage_status'])

    def test_changed_candidate_and_statuses_cannot_get_qualified(self):
        for head, change in [
            (planner.CORE_754_TEST_HEAD, {'status': 'deleted', 'path': planner.CORE_754_TEST_PATH}),
            ('c'*40, {'status': 'added', 'path': planner.CORE_754_TEST_PATH}),
            (planner.CORE_754_TEST_HEAD, {'status': 'renamed', 'old_path': 'cmd/baha/old_test.go', 'new_path': planner.CORE_754_TEST_PATH}),
        ]:
            with self.subTest(head=head, change=change):
                candidate = diff([change])
                candidate['head_sha'] = head
                result = planner.plan(candidate, REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
class ExactIndependentCore751And991CoverageTests(unittest.TestCase):
    def test_real_git_diff_base_merge_and_unreviewed_followup(self):
        resolver_spec = importlib.util.spec_from_file_location(
            'policy_diff_resolver', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(resolver_spec)
        resolver_spec.loader.exec_module(resolver)
        for reviewed_name, files, expected_checks in (
            ('EDIT_AUTOMATIC_INPUT_REVIEWED_HEAD', planner.EDIT_AUTOMATIC_INPUT_REVIEWED_FILES,
             {c['id'] for c in planner.CODE_CHECKS + planner.CLI_CHECKS +
              planner.WIZARD_BASE_CHECKS} | {'wizard-app-edit-tests'}),
            ('ECOSYSTEM_RESOLVER_REVIEWED_HEAD', planner.ECOSYSTEM_RESOLVER_REVIEWED_FILES,
             {c['id'] for c in planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS}),
            ('EVIDENCE_RETENTION_REVIEWED_HEAD', planner.EVIDENCE_RETENTION_REVIEWED_FILES,
             {c['id'] for c in planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS}),
            ('RUNTIME_RESET_REVIEWED_HEAD', planner.RUNTIME_RESET_REVIEWED_FILES,
             {c['id'] for c in planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS}),
            ('PODMAN_CONSENT_REVIEWED_HEAD', planner.PODMAN_CONSENT_REVIEWED_FILES,
             {c['id'] for c in planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS}),
            ('MCP_PRECONDITIONS_REVIEWED_HEAD', planner.MCP_PRECONDITIONS_REVIEWED_FILES,
             {c['id'] for c in planner.WIZARD_BASE_CHECKS} | {planner.CORE_754_TEST_CHECK}),
            ('SQL_RUNTIME_LAYOUT_REVIEWED_HEAD', planner.SQL_RUNTIME_LAYOUT_REVIEWED_FILES,
             {c['id'] for c in planner.CODE_CHECKS + planner.CLI_CHECKS}),
            ('CORE_751_REVIEWED_HEAD', planner.INSPECT_IMPORT_CANDIDATE_FILES,
             planner.INSPECT_IMPORT_REQUIRED_IDS),
            ('CORE_991_REVIEWED_HEAD', planner.CORE_991_REVIEWED_FILES,
             {c['id'] for c in planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS}),
            ('CORE_982_REVIEWED_HEAD', planner.CORE_982_REVIEWED_FILES,
             {c['id'] for c in planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS}),
             ('GC_999_REVIEWED_HEAD', planner.GC_999_REVIEWED_FILES,
               {c['id'] for c in planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS}),
         ):
            with self.subTest(reviewed=reviewed_name), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                def git(*args):
                    result = subprocess.run(['git', '-C', str(root), *args],
                                            text=True, capture_output=True)
                    self.assertEqual(0, result.returncode, result.stderr)
                    return result.stdout.strip()
                git('init', '-q')
                git('config', 'user.name', 'Review fixture')
                git('config', 'user.email', 'review@example.test')
                for filename, status in files.items():
                    path = root / filename
                    path.parent.mkdir(parents=True, exist_ok=True)
                    if status == 'modified':
                        path.write_text('baseline\n')
                git('add', '.')
                git('commit', '--allow-empty', '-qm', 'base')
                first = git('rev-parse', 'HEAD')
                git('branch', 'trusted')
                git('checkout', '-qb', 'source')
                for filename in files:
                    (root / filename).write_text('reviewed\n')
                git('add', '.')
                git('commit', '-qm', 'reviewed exact source')
                reviewed_sha = git('rev-parse', 'HEAD')
                git('checkout', '-q', 'trusted')
                (root / 'new-trusted-policy.txt').write_text('develop advance\n')
                git('add', '.')
                git('commit', '-qm', 'advance develop')
                target = git('rev-parse', 'HEAD')
                git('checkout', '-q', 'source')
                git('merge', '--no-ff', '-qm', 'normal develop integration', 'trusted')
                merged = git('rev-parse', 'HEAD')
                for head, merge_base in ((reviewed_sha, first), (merged, target)):
                    tree = git('merge-tree', '--write-tree', target, head).splitlines()[0]
                    integrated = git('commit-tree', tree, '-p', target, '-p', head,
                                     '-m', 'expected integration')
                    actual = resolver.resolve_pr_diff(root, target, head, integrated)
                    self.assertFalse(actual['broad_required'], actual['reasons'])
                    self.assertEqual(merge_base, actual['merge_base_sha'])
                    self.assertEqual(files, {c['path']: c['status'] for c in actual['changes']})
                    proof = planner.core752_reviewed_merge_chain
                    with patch.object(planner, reviewed_name, reviewed_sha), patch.object(
                        planner, 'core752_reviewed_merge_chain',
                        side_effect=lambda base, current, reviewed=None:
                            proof(base, current, reviewed, root)):
                        result = planner.plan(actual, REQ, MAP)
                    self.assertEqual('resolved', result['coverage_status'])
                    self.assertEqual(expected_checks, {
                        check['id'] for check in result['required_validation_checks']})
                    self.assertEqual([], result['selected_gates'])
                # Real post-review content modification, evaluated by same diff owner.
                changed = root / next(iter(files))
                changed.write_text('unreviewed follow-up\n')
                git('add', '.')
                git('commit', '-qm', 'unreviewed change')
                new_head = git('rev-parse', 'HEAD')
                tree = git('merge-tree', '--write-tree', target, new_head).splitlines()[0]
                integrated = git('commit-tree', tree, '-p', target, '-p', new_head,
                                 '-m', 'unreviewed integration')
                actual = resolver.resolve_pr_diff(root, target, new_head, integrated)
                self.assertFalse(actual['broad_required'], actual['reasons'])
                proof = planner.core752_reviewed_merge_chain
                with patch.object(planner, reviewed_name, reviewed_sha), patch.object(
                    planner, 'core752_reviewed_merge_chain',
                    side_effect=lambda base, current, reviewed=None:
                        proof(base, current, reviewed, root)):
                    result = planner.plan(actual, REQ, MAP)
                if reviewed_name == 'EDIT_AUTOMATIC_INPUT_REVIEWED_HEAD':
                    self.assertEqual('resolved', result['coverage_status'])
                    self.assertEqual(['atomic/docker/data-capabilities', 'atomic/docker/lifecycle',
                                      'atomic/podman/data-capabilities', 'atomic/podman/lifecycle'],
                                     result['selected_gates'])
                else:
                    self.assertEqual('unresolved', result['coverage_status'])

    def test_replacement_is_exact_three_file_reviewed_candidate(self):
        self.assertEqual('cf0a19bc4c8f398d7ee3fc07a635833168081ff5',
                         planner.CORE_751_REVIEWED_HEAD)
        self.assertEqual({
            'internal/repositoryinspect/capability_detection_runtime.go': 'modified',
            'internal/repositoryinspect/capability_detection_js_context.go': 'added',
            'internal/repositoryinspect/capability_import_detection_test.go': 'added',
        }, planner.INSPECT_IMPORT_CANDIDATE_FILES)
        self.assertEqual(5, len(planner.INSPECT_CHECKS))
        self.assertEqual({c['id'] for c in planner.INSPECT_CHECKS},
                         planner.INSPECT_IMPORT_REQUIRED_IDS)

    def test_missing_extra_rename_delete_changed_status_and_unverified_inputs_block(self):
        for sha_name, files in (
            ('EVIDENCE_RETENTION_REVIEWED_HEAD', planner.EVIDENCE_RETENTION_REVIEWED_FILES),
            ('RUNTIME_RESET_REVIEWED_HEAD', planner.RUNTIME_RESET_REVIEWED_FILES),
            ('PODMAN_CONSENT_REVIEWED_HEAD', planner.PODMAN_CONSENT_REVIEWED_FILES),
            ('MCP_PRECONDITIONS_REVIEWED_HEAD', planner.MCP_PRECONDITIONS_REVIEWED_FILES),
            ('SQL_RUNTIME_LAYOUT_REVIEWED_HEAD', planner.SQL_RUNTIME_LAYOUT_REVIEWED_FILES),
            ('CORE_751_REVIEWED_HEAD', planner.INSPECT_IMPORT_CANDIDATE_FILES),
            ('CORE_991_REVIEWED_HEAD', planner.CORE_991_REVIEWED_FILES),
            ('CORE_982_REVIEWED_HEAD', planner.CORE_982_REVIEWED_FILES),
        ):
            changes = [{'path': path, 'status': status} for path, status in files.items()]
            cases = [changes[:-1],
                     changes + [{'path': 'scripts/unreviewed.py', 'status': 'added'}],
                     [dict(c, status='deleted') if i == 0 else c
                      for i, c in enumerate(changes)],
                     [dict(c, status='renamed', old_path=c['path'],
                          new_path='scripts/renamed.py') if i == 0 else c
                      for i, c in enumerate(changes)],
                     [dict(c, status='modified' if c['status'] == 'added' else 'added')
                      if i == 0 else c for i, c in enumerate(changes)]]
            for variant in cases:
                with self.subTest(sha=sha_name, variant=variant):
                    candidate = diff(variant)
                    with patch.object(planner, 'core752_reviewed_merge_chain',
                                      return_value=True):
                        self.assertEqual('unresolved',
                                         planner.plan(candidate, REQ, MAP)['coverage_status'])
            for missing in ('head_sha', 'integration_sha', 'merge_base_sha'):
                with self.subTest(sha=sha_name, missing=missing):
                    candidate = diff(changes)
                    candidate[missing] = None
                    self.assertEqual('unresolved',
                                     planner.plan(candidate, REQ, MAP)['coverage_status'])


    def test_runtime_sql_layout_binds_reviewed_source_and_existing_hosted_checks(self):
        self.assertEqual('887eece62ffdfe2ba694ceb1f332dc5a74ce7872',
                         planner.SQL_RUNTIME_LAYOUT_REVIEWED_HEAD)
        self.assertEqual({
            'internal/application/backend_probe_test.go': 'modified',
            'internal/application/runtime_environment.go': 'added',
            'internal/application/runtime_postgres.go': 'modified',
        }, planner.SQL_RUNTIME_LAYOUT_REVIEWED_FILES)
        candidate = diff([{'path': path, 'status': status}
                          for path, status in planner.SQL_RUNTIME_LAYOUT_REVIEWED_FILES.items()])
        candidate['head_sha'] = planner.SQL_RUNTIME_LAYOUT_REVIEWED_HEAD
        with patch.object(planner, 'core752_reviewed_merge_chain', return_value=True) as proof:
            result = planner.plan(candidate, REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        proof.assert_called_once_with(candidate['target_base_sha'], candidate['head_sha'],
                                      planner.SQL_RUNTIME_LAYOUT_REVIEWED_HEAD)
        self.assertEqual({'source-contracts', 'cli-guided-tests', 'cli-vet', 'cli-build'},
                         {c['id'] for c in result['required_validation_checks']})
        self.assertEqual([], result['selected_gates'])


class ReadinessRepairCoverageTests(unittest.TestCase):
    def test_only_reviewed_sources_and_complete_inventories_get_runtime_free_checks(self):
        self.assertEqual('673bdcdaed9b81fa7f810b018961c02943c89ca7',
                         planner.EDIT_AUTOMATIC_INPUT_REVIEWED_HEAD)
        self.assertEqual({'cmd/baha/app_edit.go': 'modified',
                          'cmd/baha/app_edit_apply_test.go': 'modified'},
                         planner.EDIT_AUTOMATIC_INPUT_REVIEWED_FILES)
        self.assertEqual('46e80866db0dfeecc10c4472a0f1f585d11ff164',
                         planner.ECOSYSTEM_RESOLVER_REVIEWED_HEAD)
        self.assertEqual({'.github/workflows/pre-release.yml': 'modified',
                          '.github/workflows/release.yml': 'modified',
                          '.github/workflows/targeted-demo-acceptance.yml': 'modified',
                          'scripts/pre-release-resume.py': 'modified',
                          'scripts/test_ecosystem_release_pins.py': 'modified',
                          'scripts/test_pre_release_resume.py': 'modified'},
                         planner.ECOSYSTEM_RESOLVER_REVIEWED_FILES)
        for files, is_edit in ((planner.EDIT_AUTOMATIC_INPUT_REVIEWED_FILES, True),
                               (planner.ECOSYSTEM_RESOLVER_REVIEWED_FILES, False)):
            changes = [{'path': p, 'status': status} for p, status in files.items()]
            variants = [changes[:-1], changes + paths('unreviewed/source.go'),
                        [dict(c, status='deleted') if i == 0 else c
                         for i, c in enumerate(changes)],
                        [dict(c, status='renamed', old_path=c['path'],
                              new_path='unreviewed/renamed.go') if i == 0 else c
                         for i, c in enumerate(changes)],
                        [dict(c, status='added') if i == 0 else c
                         for i, c in enumerate(changes)]]
            if not is_edit:
                # The earlier source-only two-file candidate never covers the
                # completed consumer, publication and approval execution paths.
                variants.append(paths('.github/workflows/pre-release.yml',
                                      'scripts/test_ecosystem_release_pins.py'))
            for variant in variants:
                with self.subTest(is_edit=is_edit, changes=variant), patch.object(
                        planner, 'core752_reviewed_merge_chain', return_value=True):
                    result = planner.plan(diff(variant), REQ, MAP)
                    if result['coverage_status'] == 'resolved':
                        self.assertTrue(is_edit)
                        expected = ['atomic/docker/data-capabilities', 'atomic/podman/data-capabilities']
                        if any(c['path'] == 'cmd/baha/app_edit_apply_test.go' for c in variant):
                            expected += ['atomic/docker/lifecycle', 'atomic/podman/lifecycle']
                        self.assertEqual(sorted(expected), result['selected_gates'])
                    else:
                        self.assertTrue(result['unresolved_coverage'])

    def test_real_git_nonclean_merge_and_unreviewed_filenames_cannot_reuse_exception(self):
        spec = importlib.util.spec_from_file_location('repair_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(resolver)
        for anchor, files in (
                ('EDIT_AUTOMATIC_INPUT_REVIEWED_HEAD', planner.EDIT_AUTOMATIC_INPUT_REVIEWED_FILES),
                ('ECOSYSTEM_RESOLVER_REVIEWED_HEAD', planner.ECOSYSTEM_RESOLVER_REVIEWED_FILES)):
            with self.subTest(anchor=anchor), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                def git(*args):
                    result = subprocess.run(['git', '-C', str(root), *args],
                                            check=True, capture_output=True, text=True)
                    return result.stdout.strip()
                git('init', '-q')
                git('config', 'user.name', 'Repair fixture')
                git('config', 'user.email', 'repair@example.invalid')
                for name in files:
                    path = root / name
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_text('baseline\n')
                git('add', '.')
                git('commit', '-qm', 'baseline')
                git('branch', 'trusted')
                git('checkout', '-qb', 'source')
                for name in files:
                    (root / name).write_text('reviewed\n')
                git('commit', '-qam', 'reviewed source')
                reviewed = git('rev-parse', 'HEAD')
                git('checkout', '-q', 'trusted')
                (root / 'trusted.txt').write_text('accepted target advance\n')
                git('add', '.')
                git('commit', '-qm', 'target advance')
                target = git('rev-parse', 'HEAD')
                clean_tree = git('merge-tree', '--write-tree', reviewed, target).splitlines()[0]
                git('checkout', '-q', 'source')
                git('merge', '--no-ff', '--no-commit', 'trusted')
                (root / next(iter(files))).write_text('unreviewed merge resolution\n')
                git('add', '.')
                git('commit', '-qm', 'nonclean source integration')
                dirty = git('rev-parse', 'HEAD')
                self.assertNotEqual(clean_tree, git('rev-parse', dirty + '^{tree}'))
                tree = git('merge-tree', '--write-tree', target, dirty).splitlines()[0]
                integration = git('commit-tree', tree, '-p', target, '-p', dirty,
                                  '-m', 'actual PR integration')
                actual = resolver.resolve_pr_diff(root, target, dirty, integration)
                self.assertFalse(actual['broad_required'], actual['reasons'])
                proof = planner.core752_reviewed_merge_chain
                self.assertFalse(proof(target, dirty, reviewed, root))
                with patch.object(planner, anchor, reviewed), patch.object(
                        planner, 'core752_reviewed_merge_chain',
                        side_effect=lambda base, head, reviewed=None:
                            proof(base, head, reviewed, root)):
                    result = planner.plan(actual, REQ, MAP)
                if anchor == 'EDIT_AUTOMATIC_INPUT_REVIEWED_HEAD':
                    self.assertEqual('resolved', result['coverage_status'])
                    self.assertEqual(['atomic/docker/data-capabilities', 'atomic/docker/lifecycle',
                                      'atomic/podman/data-capabilities', 'atomic/podman/lifecycle'],
                                     result['selected_gates'])
                else:
                    self.assertEqual('unresolved', result['coverage_status'])
                for key in ('head_sha', 'merge_base_sha', 'integration_sha'):
                    missing = dict(actual, **{key: None})
                    with self.subTest(missing=key):
                        result = planner.plan(missing, REQ, MAP)
                        if result['coverage_status'] == 'resolved':
                            self.assertEqual('EDIT_AUTOMATIC_INPUT_REVIEWED_HEAD', anchor)
                            self.assertEqual(['atomic/docker/data-capabilities', 'atomic/docker/lifecycle',
                                      'atomic/podman/data-capabilities', 'atomic/podman/lifecycle'],
                                             result['selected_gates'])
                        else:
                            self.assertTrue(result['unresolved_coverage'])


class ReviewedSourceQualificationTests(unittest.TestCase):
    def test_immutable_anchors_and_complete_inventories(self):
        self.assertEqual('c6dfb988bb4f1fd295b6beb950980bb257e3699a',
                         planner.EDIT_REFUSAL_RESULT_REVIEWED_HEAD)
        self.assertEqual({'cmd/baha/app_edit_apply.go': 'modified',
                          'cmd/baha/app_edit_apply_test.go': 'modified'},
                         planner.EDIT_REFUSAL_RESULT_REVIEWED_FILES)
        self.assertEqual('c37933d9688615ba574cfc3b9ffc817bd03c4f3e',
                         planner.EDIT_ZERO_PREVIEW_REVIEWED_HEAD)
        self.assertEqual({'cmd/baha/app_edit_apply_result.go': 'modified',
                          'cmd/baha/app_edit_zero_preview_test.go': 'added'},
                         planner.EDIT_ZERO_PREVIEW_REVIEWED_FILES)
        self.assertEqual('75efeef89b61d848d5ea351877ba6423a31e58a6',
                         planner.RUNTIME_STEP_CAP_REVIEWED_HEAD)
        self.assertEqual({'.github/workflows/pre-release-runtime-suite.yml': 'modified'},
                         planner.RUNTIME_STEP_CAP_REVIEWED_FILES)
        self.assertEqual('0267dbdb71f63a2c77e193618e6fba4106e5cd5b',
                         planner.DISPATCH_GATE_REF_REVIEWED_HEAD)
        self.assertEqual({'.github/workflows/ci.yml': 'modified',
                          '.github/workflows/pre-release-runtime-suite.yml': 'modified',
                          '.github/workflows/targeted-demo-acceptance.yml': 'modified',
                          'scripts/targeted-gate-ref.py': 'added',
                          'scripts/test_targeted_gate_ref.py': 'added'},
                         planner.DISPATCH_GATE_REF_REVIEWED_FILES)
        self.assertEqual('f762f4480a2eb6b29ceebb35ae4032cb2ce7288c',
                         planner.MCP_PUBLIC_BOOTSTRAP_REVIEWED_HEAD)
        self.assertEqual({'cmd/baha/app_machine_adapters.go': 'modified',
                          'cmd/baha/app_mcp_provider_edit_native_test.go': 'modified',
                          'cmd/baha/app_registration_921.go': 'modified',
                          'cmd/baha/registration_921_test.go': 'modified'},
                         planner.MCP_PUBLIC_BOOTSTRAP_REVIEWED_FILES)
        self.assertEqual('99901ff2e787157ffb5e8d7d5206cf21272bd6c3',
                         planner.NEW_TERMINAL_INPUT_REVIEWED_HEAD)
        self.assertEqual({'cmd/baha/app_new_wizard_test.go': 'modified',
                          'cmd/baha/application_provider_preferences.go': 'modified'},
                         planner.NEW_TERMINAL_INPUT_REVIEWED_FILES)
        self.assertEqual('c1337192e06e63f8210ace9e5687d8dc8f70f6d2',
                         planner.DOCKER_RUNTIME_FIXTURE_REVIEWED_HEAD)
        self.assertEqual({'.github/workflows/docker-runtime-acceptance.yml': 'modified',
                          'cmd/baha/app_repository_integration_test.go': 'modified',
                          'cmd/baha/app_workload_integration_test.go': 'modified',
                          'cmd/baha/repository_runtime_fixture_test.go': 'added',
                          'scripts/test_ci_docker_runtime_doctor.py': 'added'},
                         planner.DOCKER_RUNTIME_FIXTURE_REVIEWED_FILES)
        self.assertEqual('013d7e4d491afc58a31c7934b371ab0dc35f3346',
                         planner.WORKFLOW_POLICY_DEPENDENCY_REVIEWED_HEAD)
        self.assertEqual({'.github/workflows/ci.yml': 'modified',
                          'scripts/pr-authenticated-results.py': 'modified',
                          'scripts/test_pr_authenticated_results.py': 'modified'},
                         planner.WORKFLOW_POLICY_DEPENDENCY_REVIEWED_FILES)
        self.assertEqual('3f6772fc75243eaceb00c669f8ed1cd4c4b694e4',
                         planner.RETIREMENT_PREFERENCES_REVIEWED_HEAD)
        self.assertEqual({'cmd/baha/application_runtime_retirement_intent.go': 'modified',
                          'cmd/baha/application_runtime_retirement_preferences_test.go': 'added'},
                         planner.RETIREMENT_PREFERENCES_REVIEWED_FILES)
        self.assertEqual('b31fe8918e319e3ad1505ffaaa1209c1343a5d13',
                         planner.CORE_921_PRODUCTION_REVIEWED_HEAD)
        self.assertEqual(20, len(planner.CORE_921_PRODUCTION_REVIEWED_FILES))
        self.assertEqual('added', planner.CORE_921_PRODUCTION_REVIEWED_FILES[
            'cmd/baha/app_mcp_provider_edit_native_test.go'])
        self.assertEqual('77eb9a120f46b42281198fcee1de788c9779be18',
                         planner.PODMAN_PG17_RECOVERY_REVIEWED_HEAD)
        self.assertEqual(17, len(planner.PODMAN_PG17_RECOVERY_REVIEWED_FILES))
        self.assertFalse(set(planner.RETIREMENT_PREFERENCES_REVIEWED_FILES) &
                         set(planner.PODMAN_PG17_RECOVERY_REVIEWED_FILES))

    def test_real_git_exact_sources_and_all_authority_mutations(self):
        spec = importlib.util.spec_from_file_location('native_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(resolver)
        cases = (
            ('EDIT_REFUSAL_RESULT', planner.CODE_CHECKS + planner.CLI_CHECKS +
             planner.WIZARD_BASE_CHECKS),
            ('MCP_PUBLIC_BOOTSTRAP', planner.CODE_CHECKS + planner.CLI_CHECKS +
             planner.WIZARD_BASE_CHECKS + [wizard_check('wizard-machine-adapter-tests'),
                                         wizard_check('wizard-core921-tests')]),
            ('NEW_TERMINAL_INPUT', planner.CODE_CHECKS + planner.CLI_CHECKS +
             planner.WIZARD_BASE_CHECKS + [wizard_check('wizard-app-new-tests')]),
             ('EDIT_ZERO_PREVIEW', planner.CODE_CHECKS + planner.CLI_CHECKS +
              planner.WIZARD_BASE_CHECKS + [wizard_check('wizard-app-edit-tests')]),
             ('RUNTIME_STEP_CAP', planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS),
             ('DISPATCH_GATE_REF', planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS),
            ('DOCKER_RUNTIME_FIXTURE', planner.CI_CHECKS + planner.CODE_CHECKS +
             planner.CLI_CHECKS + planner.WIZARD_BASE_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS),
            ('WORKFLOW_POLICY_DEPENDENCY', planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS),
            ('RETIREMENT_PREFERENCES', planner.CODE_CHECKS + planner.CLI_CHECKS +
             planner.WIZARD_BASE_CHECKS),
            ('UNBOUND_SQL_RETIREMENT', planner.CODE_CHECKS + planner.CLI_CHECKS +
             planner.WIZARD_BASE_CHECKS + [wizard_check('wizard-runtime-retirement-tests')]),
            ('EVIDENCE_REUSE', planner.CI_CHECKS),
            ('PODMAN_PG17_RECOVERY', planner.CI_CHECKS + planner.CODE_CHECKS +
             planner.CLI_CHECKS + planner.WIZARD_BASE_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS),
            ('CORE_921_PRODUCTION', planner.CI_CHECKS + planner.CODE_CHECKS +
             planner.CLI_CHECKS + planner.WIZARD_BASE_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS +
             planner.DOC_CHECKS + [wizard_check(name) for name in (
                 'wizard-registration-tests', 'wizard-machine-adapter-tests',
                 'wizard-provider-choice-tests', 'wizard-core921-tests')]),
        )
        for name, required in cases:
            with self.subTest(source=name), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                files = getattr(planner, name + '_REVIEWED_FILES')
                def git(*args):
                    return subprocess.run(['git', '-C', str(root), *args], check=True,
                                          capture_output=True, text=True).stdout.strip()
                git('init', '-q')
                git('config', 'user.name', 'Native qualification fixture')
                git('config', 'user.email', 'native@example.invalid')
                for path, status in files.items():
                    if status == 'modified':
                        (root / path).parent.mkdir(parents=True, exist_ok=True)
                        (root / path).write_text('baseline\n')
                git('add', '.')
                git('commit', '-qm', 'accepted prerequisite baseline')
                git('branch', 'trusted')
                git('checkout', '-qb', 'source')
                prior_source = None
                if name == 'EDIT_REFUSAL_RESULT':
                    # Same two-file implementation before its mandatory Guided
                    # wrapper: ancestry alone cannot inherit the later review.
                    for path in files:
                        (root / path).write_text('earlier source without required coverage\n')
                    git('commit', '-qam', 'unqualified pre-wrapper source')
                    prior_source = git('rev-parse', 'HEAD')
                for path in files:
                    (root / path).parent.mkdir(parents=True, exist_ok=True)
                    (root / path).write_text('reviewed contents\n')
                git('add', '.')
                git('commit', '-qm', 'independently reviewed source')
                reviewed = git('rev-parse', 'HEAD')
                git('checkout', '-q', 'trusted')
                (root / 'trusted-policy.txt').write_text('independent authority\n')
                git('add', '.')
                git('commit', '-qm', 'trusted target advance')
                target = git('rev-parse', 'HEAD')
                git('checkout', '-q', 'source')
                git('merge', '--no-ff', '-qm', 'clean target integration', 'trusted')
                head = git('rev-parse', 'HEAD')
                def resolved_diff(candidate):
                    tree = git('merge-tree', '--write-tree', target, candidate).splitlines()[0]
                    integration = git('commit-tree', tree, '-p', target, '-p', candidate,
                                      '-m', 'PR integration')
                    return resolver.resolve_pr_diff(root, target, candidate, integration)
                actual = resolved_diff(head)
                proof = planner.core752_reviewed_merge_chain
                def real_proof(base, candidate, anchor=None):
                    return proof(base, candidate, anchor, root)
                def assert_no_qualification(result):
                    self.assertFalse(result['coverage_status'] == 'resolved' and
                                     result['selected_gates'] == [], result)
                with patch.object(planner, name + '_REVIEWED_HEAD', reviewed), patch.object(
                        planner, 'core752_reviewed_merge_chain', side_effect=real_proof):
                    result = planner.plan(actual, REQ, MAP)
                    self.assertEqual('resolved', result['coverage_status'], result)
                    self.assertEqual([], result['selected_gates'])
                    self.assertEqual({c['id'] for c in required},
                                     {c['id'] for c in result['required_validation_checks']})
                    if prior_source is not None:
                        prior_tree = git('merge-tree', '--write-tree', prior_source, target).splitlines()[0]
                        prior_merge = git('commit-tree', prior_tree, '-p', prior_source, '-p', target,
                                          '-m', 'older pre-wrapper source with clean target merge')
                        self.assertFalse(real_proof(target, prior_merge, reviewed))
                        assert_no_qualification(planner.plan(resolved_diff(prior_merge), REQ, MAP))
                    mutations = [dict(actual, changes=actual['changes'][:-1]),
                                 dict(actual, changes=actual['changes'] + paths('unreviewed.go')),
                                 dict(actual, changes=[dict(c, status='deleted') if i == 0 else c
                                     for i, c in enumerate(actual['changes'])])]
                    mutations += [dict(actual, **{key: None}) for key in
                                  ('head_sha', 'target_base_sha', 'merge_base_sha', 'integration_sha')]
                    for mutated in mutations:
                        with self.subTest(mutation=mutated):
                            assert_no_qualification(planner.plan(mutated, REQ, MAP))
                    with patch.object(planner, name + '_REVIEWED_HEAD', 'f' * 40):
                        assert_no_qualification(planner.plan(actual, REQ, MAP))
                    # Same reviewed path inventory with different bytes cannot
                    # inherit authority, even when integrated through real Git.
                    changed = root / next(iter(files))
                    changed.write_text('unreviewed follow-up\n')
                    git('commit', '-qam', 'unreviewed content')
                    unreviewed = git('rev-parse', 'HEAD')
                    self.assertFalse(real_proof(target, unreviewed, reviewed))
                    assert_no_qualification(planner.plan(resolved_diff(unreviewed), REQ, MAP))
                    # A two-parent merge with an injected tree is not the clean
                    # merge of the reviewed source and authenticated target.
                    dirty_tree = git('rev-parse', unreviewed + '^{tree}')
                    dirty = git('commit-tree', dirty_tree, '-p', reviewed, '-p', target,
                                '-m', 'injected merge resolution')
                    self.assertFalse(real_proof(target, dirty, reviewed))
                    assert_no_qualification(planner.plan(resolved_diff(dirty), REQ, MAP))


class BindingPR1007TrustedBasePolicyTests(unittest.TestCase):
    # #1007 binding-PR clause via trusted base policy (Codex ruling 3a). The
    # clause loads its qualification record from the authenticated trusted base
    # revision and never from the candidate branch. These tests build real git
    # fixtures: a develop line that may or may not carry the policy file, a
    # candidate head H with the reviewed content, and a clean integration merge.
    RECORD_PATH = 'docs/internal/ci-binding-qualifications.md'

    def _record(self, reviewed_head_sha, repository='mcpdev80/baseharbor',
                target_branch='develop', required_checks=None):
        import hashlib
        files = dict(planner.BINDING_PR_1007_REVIEWED_FILES)
        # The policy PR is authored against the frozen reviewed head H; its
        # hashes reference H's blobs, which the base checkout does not carry.
        hashes = {path: self._blob_sha(head=reviewed_head_sha, path=path)
                  for path in files}
        return {
            'repository': repository,
            'target_branch': target_branch,
            'reviewed_head': reviewed_head_sha,
            'files': files,
            'file_hashes': hashes,
            'required_checks': required_checks or [c['id'] for c in planner.CI_CHECKS],
        }

    def _blob_sha(self, head, path):
        out = subprocess.run(['git', '-C', str(self._repo_root), 'cat-file',
                              'blob', f'{head}:{path}'],
                              capture_output=True)
        self.assertEqual(0, out.returncode, out.stderr)
        return hashlib.sha256(out.stdout).hexdigest()

    def _record_blob(self, rec):
        # The loader reads docs/internal/ci-binding-qualifications.md and
        # extracts exactly one fenced JSON block from the blob.
        return '```json\n' + json.dumps(rec) + '\n```\n'

    def _build(self, root, *, base_carries_record=True, record=None,
               head_content='reviewed\n'):
        resolver_spec = importlib.util.spec_from_file_location(
            'binding_policy_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(resolver_spec)
        resolver_spec.loader.exec_module(resolver)
        def git(*args):
            result = subprocess.run(['git', '-C', str(root), *args],
                                    text=True, capture_output=True)
            self.assertEqual(0, result.returncode, result.stderr)
            return result.stdout.strip()
        self._repo_root = root
        git('init', '-q')
        git('config', 'user.name', 'Binding fixture')
        git('config', 'user.email', 'fixture@example.test')
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (root / path).parent.mkdir(parents=True, exist_ok=True)
            (root / path).write_text('baseline\n')
        git('add', '.')
        git('commit', '--allow-empty', '-qm', 'develop baseline')
        git('branch', 'develop')
        baseline = git('rev-parse', 'develop')
        # Build the candidate head H on its own branch so its reviewed blobs
        # exist to hash, then detach back to target for the record commit.
        git('checkout', '-q', '-b', 'candidate')
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (root / path).write_text(head_content)
        git('add', '.')
        git('commit', '-qm', 'candidate head H')
        head = git('rev-parse', 'HEAD')
        git('checkout', '-q', 'develop')
        if base_carries_record:
            rec = record if record is not None else self._record(head)
            (root / self.RECORD_PATH).parent.mkdir(parents=True, exist_ok=True)
            (root / self.RECORD_PATH).write_text(self._record_blob(rec))
            git('add', '.')
            git('commit', '-qm', 'trusted base policy for #1007')
        target = git('rev-parse', 'develop')
        self.assertEqual(baseline, git('merge-base', head, target))
        tree = git('merge-tree', '--write-tree', target, head).splitlines()[0]
        integrated = git('commit-tree', tree, '-p', target, '-p', head,
                         '-m', 'binding integration')
        actual = resolver.resolve_pr_diff(root, target, head, integrated)
        self.assertFalse(actual['broad_required'], actual['reasons'])
        self.assertEqual(baseline, actual['merge_base_sha'])
        return target, head, actual, root

    def _run(self, target, head, actual, root):
        import os
        previous = os.getcwd()
        os.chdir(str(root))
        try:
            return planner.plan(actual, REQ, MAP,
                                context_repository='mcpdev80/baseharbor',
                                context_target_branch='develop')
        finally:
            os.chdir(previous)

    def _build_base_with_candidate(self, root, head_content='reviewed\n'):
        # develop at baseline (no policy record yet), candidate head H with the
        # reviewed content. Returns (baseline, head, root) so the caller can add
        # the policy record on develop and re-resolve against the new tip.
        resolver_spec = importlib.util.spec_from_file_location(
            'base_candidate', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(resolver_spec)
        resolver_spec.loader.exec_module(resolver)
        def git(*args):
            result = subprocess.run(['git', '-C', str(root), *args],
                                    text=True, capture_output=True)
            self.assertEqual(0, result.returncode, result.stderr)
            return result.stdout.strip()
        self._repo_root = root
        git('init', '-q')
        git('config', 'user.name', 'Binding fixture')
        git('config', 'user.email', 'fixture@example.test')
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (root / path).parent.mkdir(parents=True, exist_ok=True)
            (root / path).write_text('baseline\n')
        git('add', '.')
        git('commit', '--allow-empty', '-qm', 'develop baseline')
        git('branch', 'develop')
        baseline = git('rev-parse', 'develop')
        git('checkout', '-q', '-b', 'candidate')
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (root / path).write_text(head_content)
        git('add', '.')
        git('commit', '-qm', 'candidate head H')
        head = git('rev-parse', 'HEAD')
        return baseline, head, root

    def _resolve_with_record(self, root, record_factory, *, base_carries_record=True,
                             head_content='reviewed\n',
                             context_repository='mcpdev80/baseharbor',
                             context_target_branch='develop'):
        # Build a base line and a candidate head H once, optionally write a
        # custom policy record onto the base, then resolve the plan.
        # record_factory(head) receives the reviewed head H and returns a dict
        # or a raw blob str to persist as the policy record.
        baseline, head, root = self._build_base_with_candidate(root, head_content)
        git = lambda *a: subprocess.run(['git', '-C', str(root), *a],
                                        text=True, capture_output=True).stdout.strip()
        if base_carries_record:
            record = record_factory(head)
            blob = record if isinstance(record, str) else self._record_blob(record)
            git('checkout', '-q', 'develop')
            (root / self.RECORD_PATH).parent.mkdir(parents=True, exist_ok=True)
            (root / self.RECORD_PATH).write_text(blob)
            git('add', '.'); git('commit', '-qm', 'record on base')
            target = git('rev-parse', 'HEAD')
        else:
            target = baseline
        resolver_spec = importlib.util.spec_from_file_location(
            'resolve_record', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(resolver_spec)
        resolver_spec.loader.exec_module(resolver)
        tree = git('merge-tree', '--write-tree', target, head).splitlines()[0]
        integrated = git('commit-tree', tree, '-p', target, '-p', head,
                         '-m', 'integration')
        actual = resolver.resolve_pr_diff(root, target, head, integrated)
        import os
        previous = os.getcwd()
        os.chdir(str(root))
        try:
            return planner.plan(actual, REQ, MAP,
                                context_repository=context_repository,
                                context_target_branch=context_target_branch)
        finally:
            os.chdir(previous)

    def test_record_present_matching_head_resolves(self):
        target, head, actual, root = self._build(
            Path(tempfile.mkdtemp()), head_content='reviewed\n')
        planned = self._run(target, head, actual, root)
        self.assertEqual('resolved', planned['coverage_status'], planned)
        self.assertEqual([], planned['unresolved_coverage'])
        self.assertEqual([], planned['selected_gates'])
        self.assertEqual(
            {c['id'] for c in planner.CI_CHECKS},
            {c['id'] for c in planned['required_validation_checks']})

    def test_record_absent_fails_closed(self):
        target, head, actual, root = self._build(
            Path(tempfile.mkdtemp()), base_carries_record=False)
        planned = self._run(target, head, actual, root)
        self.assertEqual('unresolved', planned['coverage_status'], planned)
        self.assertTrue(planned['unresolved_coverage'])

    def test_single_block_positive_resolves(self):
        # Exactly one fenced JSON block with matching context resolves.
        tmp = Path(tempfile.mkdtemp())
        planned = self._resolve_with_record(
            tmp, lambda head: self._record(head))
        self.assertEqual('resolved', planned['coverage_status'], planned)
        self.assertEqual([], planned['unresolved_coverage'])

    def test_zero_block_fails_closed(self):
        # A base blob with no fenced JSON block fails closed.
        tmp = Path(tempfile.mkdtemp())
        planned = self._resolve_with_record(
            tmp, lambda _head: 'no fenced json here, just prose\n')
        self.assertEqual('unresolved', planned['coverage_status'], planned)
        self.assertTrue(planned['unresolved_coverage'])

    def test_two_blocks_fails_closed(self):
        # Two fenced JSON blocks fail closed: the loader requires exactly one.
        tmp = Path(tempfile.mkdtemp())

        def two_blocks(_head):
            return ('```json\n' + json.dumps(self._record(_head)) + '\n```\n'
                    + '```json\n' + json.dumps(
                        {**self._record(_head),
                         'required_checks': ['ci-policy']}) + '\n```\n')
        planned = self._resolve_with_record(tmp, two_blocks)
        self.assertEqual('unresolved', planned['coverage_status'], planned)
        self.assertTrue(planned['unresolved_coverage'])

    def test_key_set_mismatch_fails_closed(self):
        # files and file_hashes have differing key sets fails closed.
        tmp = Path(tempfile.mkdtemp())
        # Build the record then drop a hash key so files/file_hashes differ.
        def key_set_record(head):
            record = self._record(head)
            del record['file_hashes']['scripts/test_pr_gate_plan.py']
            return record
        planned = self._resolve_with_record(tmp, key_set_record)
        self.assertEqual('unresolved', planned['coverage_status'], planned)
        self.assertTrue(planned['unresolved_coverage'])

    def test_wrong_repository_identity_fails_closed(self):
        # The record's repository differs from the authenticated context.
        tmp = Path(tempfile.mkdtemp())
        wrong_repo = lambda head: self._record(head)
        def repo_record(head):
            record = self._record(head)
            record['repository'] = 'other-org/baseharbor'
            return record
        planned = self._resolve_with_record(tmp, repo_record)
        self.assertEqual('unresolved', planned['coverage_status'], planned)
        self.assertTrue(planned['unresolved_coverage'])

    def test_wrong_target_branch_identity_fails_closed(self):
        # The record's target_branch differs from the authenticated context.
        tmp = Path(tempfile.mkdtemp())
        def branch_record(head):
            record = self._record(head)
            record['target_branch'] = 'main'
            return record
        planned = self._resolve_with_record(tmp, branch_record)
        self.assertEqual('unresolved', planned['coverage_status'], planned)
        self.assertTrue(planned['unresolved_coverage'])

    def test_candidate_side_record_never_accepted(self):
        # A policy file on the candidate head must not satisfy the clause; only
        # the trusted base revision supplies authoritative pins.
        tmp = Path(tempfile.mkdtemp())
        root = tmp
        self._repo_root = root
        resolver_spec = importlib.util.spec_from_file_location(
            'candidate_record_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(resolver_spec)
        resolver_spec.loader.exec_module(resolver)
        def git(*args):
            result = subprocess.run(['git', '-C', str(root), *args],
                                    text=True, capture_output=True)
            self.assertEqual(0, result.returncode, result.stderr)
            return result.stdout.strip()
        git('init', '-q')
        git('config', 'user.name', 'Binding fixture')
        git('config', 'user.email', 'fixture@example.test')
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (root / path).parent.mkdir(parents=True, exist_ok=True)
            (root / path).write_text('baseline\n')
        git('add', '.')
        git('commit', '--allow-empty', '-qm', 'develop baseline')
        git('branch', 'develop')
        target = git('rev-parse', 'develop')
        git('checkout', '-q', 'develop')
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (root / path).write_text('reviewed\n')
        git('add', '.')
        git('commit', '-qm', 'candidate head H')
        head = git('rev-parse', 'HEAD')
        # Policy file lives only on the candidate head, not the base.
        rec = self._record(head)
        (root / self.RECORD_PATH).parent.mkdir(parents=True, exist_ok=True)
        (root / self.RECORD_PATH).write_text(self._record_blob(rec))
        git('add', '.')
        git('commit', '-qm', 'candidate self-declared pins')
        tree = git('merge-tree', '--write-tree', target, head).splitlines()[0]
        integrated = git('commit-tree', tree, '-p', target, '-p', head,
                         '-m', 'integration')
        actual = resolver.resolve_pr_diff(root, target, head, integrated)
        planned = self._run(target, head, actual, root)
        self.assertEqual('unresolved', planned['coverage_status'], planned)
        self.assertTrue(planned['unresolved_coverage'])

    def test_mismatched_record_head_fails_closed(self):
        # The base record pins a different reviewed head than the candidate: the
        # head_sha != record['reviewed_head'] guard fails closed. The hashes
        # still match (real H blobs) so only the head anchor differs.
        tmp = Path(tempfile.mkdtemp())
        self._repo_root = tmp
        base, head, root = self._build_base_with_candidate(tmp, head_content='reviewed\n')
        good = self._record(head)
        good['reviewed_head'] = '0' * 40
        (root / self.RECORD_PATH).parent.mkdir(parents=True, exist_ok=True)
        (root / self.RECORD_PATH).write_text(self._record_blob(good))
        git = lambda *a: subprocess.run(['git', '-C', str(root), *a],
                                        text=True, capture_output=True).stdout.strip()
        git('add', '.'); git('commit', '-qm', 'wrong the pinned head')
        target = git('rev-parse', 'HEAD')
        tree = git('merge-tree', '--write-tree', target, head).splitlines()[0]
        integrated = git('commit-tree', tree, '-p', target, '-p', head,
                         '-m', 'integration')
        resolver_spec = importlib.util.spec_from_file_location(
            'mmr', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(resolver_spec)
        resolver_spec.loader.exec_module(resolver)
        actual = resolver.resolve_pr_diff(root, target, head, integrated)
        planned = self._run(target, head, actual, root)
        self.assertEqual('unresolved', planned['coverage_status'], planned)
        self.assertTrue(planned['unresolved_coverage'])

    def test_mismatched_record_content_fails_closed(self):
        # The base record hashes differ from the candidate content: content
        # verification fails closed even though head_sha matches the record.
        tmp = Path(tempfile.mkdtemp())
        self._repo_root = tmp
        base, head, root = self._build_base_with_candidate(tmp, head_content='reviewed\n')
        record = self._record(head)
        record['file_hashes'] = {path: 'f' * 64 for path in record['file_hashes']}
        (root / self.RECORD_PATH).parent.mkdir(parents=True, exist_ok=True)
        (root / self.RECORD_PATH).write_text(self._record_blob(record))
        git = lambda *a: subprocess.run(['git', '-C', str(root), *a],
                                        text=True, capture_output=True).stdout.strip()
        git('add', '.'); git('commit', '-qm', 'corrupt the hashes')
        target = git('rev-parse', 'HEAD')
        tree = git('merge-tree', '--write-tree', target, head).splitlines()[0]
        integrated = git('commit-tree', tree, '-p', target, '-p', head,
                         '-m', 'integration')
        resolver_spec = importlib.util.spec_from_file_location(
            'mmrc', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(resolver_spec)
        resolver_spec.loader.exec_module(resolver)
        actual = resolver.resolve_pr_diff(root, target, head, integrated)
        planned = self._run(target, head, actual, root)
        self.assertEqual('unresolved', planned['coverage_status'], planned)
        self.assertTrue(planned['unresolved_coverage'])

    def test_extra_candidate_file_fails_closed(self):
        # An additional changed file on the candidate breaks the exact inventory
        # match against the record, so the clause fails closed.
        tmp = Path(tempfile.mkdtemp())
        root = tmp
        self._repo_root = root
        resolver_spec = importlib.util.spec_from_file_location(
            'extra_file_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(resolver_spec)
        resolver_spec.loader.exec_module(resolver)
        def git(*args):
            result = subprocess.run(['git', '-C', str(root), *args],
                                    text=True, capture_output=True)
            self.assertEqual(0, result.returncode, result.stderr)
            return result.stdout.strip()
        git('init', '-q')
        git('config', 'user.name', 'Binding fixture')
        git('config', 'user.email', 'fixture@example.test')
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (root / path).parent.mkdir(parents=True, exist_ok=True)
            (root / path).write_text('baseline\n')
        git('add', '.')
        git('commit', '--allow-empty', '-qm', 'develop baseline')
        git('branch', 'develop')
        target = git('rev-parse', 'develop')
        git('checkout', '-q', '-b', 'candidate')
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (root / path).write_text('reviewed\n')
        (root / 'scripts/extra_candidate.py').write_text('not reviewed\n')
        git('add', '.')
        git('commit', '-qm', 'candidate head H with extra file')
        head = git('rev-parse', 'HEAD')
        rec = self._record(head)
        git('checkout', '-q', 'develop')
        (root / self.RECORD_PATH).parent.mkdir(parents=True, exist_ok=True)
        (root / self.RECORD_PATH).write_text(self._record_blob(rec))
        git('add', '.')
        git('commit', '-qm', 'trusted base policy for #1007')
        target = git('rev-parse', 'develop')
        tree = git('merge-tree', '--write-tree', target, head).splitlines()[0]
        integrated = git('commit-tree', tree, '-p', target, '-p', head,
                         '-m', 'integration')
        actual = resolver.resolve_pr_diff(root, target, head, integrated)
        planned = self._run(target, head, actual, root)
        self.assertEqual('unresolved', planned['coverage_status'], planned)
        self.assertTrue(planned['unresolved_coverage'])

# OpenBao package-tests validation and fail-closed handling.
class OpenBaoValidationTests(unittest.TestCase):
    def test_openbao_package_tests_is_a_known_check(self):
        """openbao-package-tests must resolve to unresolved for any unreviewed path."""
        self.assertIn('internal/openbao', planner.__dict__.get('OPENBAO_CHECKS', [])[0]['owner'])

    def test_any_unreviewed_openbao_path_stays_fail_closed(self):
        """Unreviewed OpenBao production paths must remain unresolved."""
        for name in ['internal/openbao/unreviewed_security_mutation.go',
                     'internal/openbao/managed_credentials.go',
                     'internal/openbao/application_runtime_client.go',
                     'internal/openbao/dev_access.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                self.assertTrue(
                    any('OpenBao production impact' in u for u in result['unresolved_coverage'])
                )

    def test_openbao_add_delete_rename_stay_unresolved(self):
        """Added, deleted, renamed OpenBao files must stay fail-closed."""
        variants = [
            paths('internal/openbao/bootstrap.go'),
            [{'status': 'deleted', 'path': 'internal/openbao/bootstrap.go'}],
            [{'status': 'renamed', 'old_path': 'internal/openbao/bootstrap.go',
              'new_path': 'internal/openbao/new_bootstrap.go'}],
            paths('internal/openbao/bootstrap.go',
                  'internal/openbao/unreviewed_addition.go'),
            [{'status': 'added', 'path': 'internal/openbao/new_handler.go'}],
        ]
        for changes in variants:
            with self.subTest(changes=changes):
                result = planner.plan(diff(changes), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])

# main() entrypoint must source the authenticated evaluation context from the
# runner environment, never from the candidate PR, and must fail closed when
# either value is absent. These tests drive planner.main() with monkeypatched
# os.environ and a real diff file.
class MainRunnerContextEntrypointTests(unittest.TestCase):
    RECORD_PATH = 'docs/internal/ci-binding-qualifications.md'

    def setUp(self):
        import json
        import hashlib
        self._root = Path(tempfile.mkdtemp())
        resolver_spec = importlib.util.spec_from_file_location(
            'main_ctx_resolver', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(resolver_spec)
        resolver_spec.loader.exec_module(resolver)

        def git(*args):
            result = subprocess.run(['git', '-C', str(self._root), *args],
                                    text=True, capture_output=True)
            self.assertEqual(0, result.returncode, result.stderr)
            return result.stdout.strip()

        git('init', '-q')
        git('config', 'user.name', 'Binding fixture')
        git('config', 'user.email', 'fixture@example.test')
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (self._root / path).parent.mkdir(parents=True, exist_ok=True)
            (self._root / path).write_text('baseline\n')
        git('add', '.')
        git('commit', '--allow-empty', '-qm', 'develop baseline')
        git('branch', 'develop')
        git('checkout', '-q', '-b', 'candidate')
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (self._root / path).write_text('reviewed\n')
        git('add', '.')
        git('commit', '-qm', 'candidate head H')
        self._head = git('rev-parse', 'HEAD')
        git('checkout', '-q', 'develop')
        import hashlib
        files = dict(planner.BINDING_PR_1007_REVIEWED_FILES)
        hashes = {path: hashlib.sha256(
            subprocess.run(['git', '-C', str(self._root), 'cat-file',
                            'blob', f'{self._head}:{path}'],
                           capture_output=True).stdout).hexdigest()
                  for path in files}
        rec = {'repository': 'mcpdev80/baseharbor', 'target_branch': 'develop',
               'reviewed_head': self._head, 'files': files,
               'file_hashes': hashes,
               'required_checks': [c['id'] for c in planner.CI_CHECKS]}
        (self._root / self.RECORD_PATH).parent.mkdir(parents=True, exist_ok=True)
        (self._root / self.RECORD_PATH).write_text(
            '```json\n' + json.dumps(rec) + '\n```\n')
        git('add', '.')
        git('commit', '-qm', 'trusted base policy for #1007')
        self._target = git('rev-parse', 'develop')
        tree = git('merge-tree', '--write-tree', self._target, self._head).splitlines()[0]
        integrated = git('commit-tree', tree, '-p', self._target, '-p', self._head,
                         '-m', 'binding integration')
        self._actual = resolver.resolve_pr_diff(self._root, self._target, self._head, integrated)

    def _run_main(self, environ, *, extra_args=None):
        import json
        import io
        import contextlib
        diff_path = self._root / 'diff.json'
        diff_path.write_text(json.dumps(self._actual))
        previous = os.getcwd()
        os.chdir(str(self._root))
        saved = dict(os.environ)
        code = None
        msg = None
        buf = io.StringIO()
        argv_snapshot = sys.argv
        try:
            os.environ.pop('GITHUB_REPOSITORY', None)
            os.environ.pop('GITHUB_BASE_REF', None)
            os.environ.update(environ)
            sys.argv = ['pr-gate-plan.py',
                        '--diff', str(diff_path),
                        '--requirements', str(ROOT / 'release-requirements.json'),
                        '--mapping', str(ROOT / 'pr-gate-map-v1.json')]
            if extra_args:
                sys.argv.extend(extra_args)
            with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(buf):
                planner.main()
        except SystemExit as exit:
            code = exit.code
            if isinstance(exit.code, str):
                msg = exit.code
        finally:
            os.environ.clear()
            os.environ.update(saved)
            sys.argv = argv_snapshot
            os.chdir(previous)
        return code, buf.getvalue(), msg

    def test_matching_runner_context_resolves(self):
        # Context matches the record exactly -> plan resolves via the binding clause.
        code, out, msg = self._run_main({
            'GITHUB_REPOSITORY': 'mcpdev80/baseharbor',
            'GITHUB_BASE_REF': 'develop',
        })
        self.assertIsNone(code)
        self.assertIn('"coverage_status": "resolved"', out)

    def test_empty_context_fails_closed(self):
        # Missing runner context must be rejected before plan() runs: the
        # entrypoint refuses to qualify with an unknown authenticated context.
        code, out, msg = self._run_main({})
        self.assertNotEqual(0, code)
        self.assertIn('authenticated runner context', out or msg or '')
        self.assertNotIn('"coverage_status"', out)

    def test_empty_repository_fails_closed(self):
        code, out, msg = self._run_main({'GITHUB_BASE_REF': 'develop'})
        self.assertNotEqual(0, code)
        self.assertIn('authenticated runner context', out or msg or '')

    def test_empty_target_branch_fails_closed(self):
        code, out, msg = self._run_main({'GITHUB_REPOSITORY': 'mcpdev80/baseharbor'})
        self.assertNotEqual(0, code)
        self.assertIn('authenticated runner context', out or msg or '')

    def test_wrong_repository_fails_closed(self):
        # Non-empty but wrong repository: plan() runs and fails closed on the
        # repository mismatch rather than resolving.
        code, out, msg = self._run_main({
            'GITHUB_REPOSITORY': 'other-org/baseharbor',
            'GITHUB_BASE_REF': 'develop',
        })
        self.assertIsNone(code)
        self.assertNotIn('"coverage_status": "resolved"', out)
        self.assertIn('"coverage_status": "unresolved"', out)

class CopiedTrustedTreeReviewTests(unittest.TestCase):
    def test_conflict_resolution_needs_fresh_review_then_clean_merges_work(self):
        resolver_spec = importlib.util.spec_from_file_location(
            'copied_tree_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(resolver_spec)
        resolver_spec.loader.exec_module(resolver)
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            def git(*args):
                result = subprocess.run(['git', '-C', str(root), *args],
                                        text=True, capture_output=True)
                self.assertEqual(0, result.returncode, result.stderr)
                return result.stdout.strip()
            git('init', '-q')
            git('config', 'user.name', 'Review fixture')
            git('config', 'user.email', 'review@example.test')
            tracked = root / 'tracked.txt'
            tracked.write_text('baseline\n')
            git('add', '.')
            git('commit', '-qm', 'common base')
            base = git('rev-parse', 'HEAD')
            git('checkout', '-qb', 'trusted')
            tracked.write_text('trusted first\n')
            git('commit', '-qam', 'trusted advance')
            tracked.write_text('trusted second\n')
            git('commit', '-qam', 'trusted advances again')
            target = git('rev-parse', 'HEAD')
            git('checkout', '-qb', 'source', base)
            tracked.write_text('trusted first\n')
            candidate = root / planner.CORE_754_TEST_PATH
            candidate.parent.mkdir(parents=True)
            candidate.write_text('reviewed test source\n')
            git('add', '.')
            git('commit', '-qm', 'copy trusted tree without ancestry')
            reviewed = git('rev-parse', 'HEAD')
            conflict = subprocess.run(
                ['git', '-C', str(root), 'merge-tree', '--write-tree', reviewed, target],
                text=True, capture_output=True)
            self.assertEqual(1, conflict.returncode)
            tracked.write_text('trusted second\n')
            git('add', '.')
            resolved = git('commit-tree', git('write-tree'), '-p', reviewed,
                           '-p', target, '-m', 'explicit conflict resolution')
            self.assertFalse(planner.core752_reviewed_merge_chain(
                target, resolved, reviewed, root))
            integration = git('commit-tree', git('rev-parse', resolved + '^{tree}'),
                              '-p', target, '-p', resolved, '-m', 'PR integration')
            actual = resolver.resolve_pr_diff(root, target, resolved, integration)
            self.assertFalse(actual['broad_required'], actual['reasons'])
            self.assertEqual({planner.CORE_754_TEST_PATH: 'added'},
                             {c['path']: c['status'] for c in actual['changes']})
            proof = planner.core752_reviewed_merge_chain
            for anchor, expected in ((reviewed, 'unresolved'), (resolved, 'resolved')):
                with patch.object(planner, 'MCP_PRECONDITIONS_REVIEWED_HEAD', anchor), patch.object(
                    planner, 'core752_reviewed_merge_chain',
                    side_effect=lambda base, head, source=None: proof(base, head, source, root)):
                    self.assertEqual(expected, planner.plan(actual, REQ, MAP)['coverage_status'])
            git('checkout', '-q', 'trusted')
            (root / 'new-trusted.txt').write_text('independent target advance\n')
            git('add', '.')
            git('commit', '-qm', 'later target')
            later = git('rev-parse', 'HEAD')
            git('checkout', '-qB', 'source', resolved)
            git('merge', '--no-ff', '-qm', 'clean target integration after fresh review', 'trusted')
            self.assertTrue(proof(later, git('rev-parse', 'HEAD'), resolved, root))

class ReviewedUtilityExecutionTests(unittest.TestCase):
    def run_candidate_suite(self, filename, expected):
        path = ROOT / filename
        if not path.exists():
            self.skipTest('suite belongs only to its exact reviewed candidate')
        script = r"""
import importlib.util
from pathlib import Path
import sys
import unittest
path = Path(sys.argv[1])
expected = set(sys.argv[2:])
spec = importlib.util.spec_from_file_location('reviewed_utility_tests', path)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
suite = unittest.defaultTestLoader.loadTestsFromModule(module)
def flatten(node):
    for item in node:
        if isinstance(item, unittest.TestSuite):
            yield from flatten(item)
        else:
            yield item
actual = {case._testMethodName for case in flatten(suite)}
if actual != expected:
    raise SystemExit('missing or changed required utility test family: ' + repr(actual))
result = unittest.TextTestRunner(verbosity=2).run(suite)
if (not result.wasSuccessful() or result.skipped or
        result.testsRun != len(expected)):
    raise SystemExit('missing, skipped or failed reviewed utility regression')
"""
        result = subprocess.run([sys.executable, '-c', script, str(path), *sorted(expected)],
                                text=True, stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT, timeout=120)
        print(result.stdout, end='')
        self.assertEqual(0, result.returncode, result.stdout)

    def test_reviewed_scoped_reset_runs_all_five_sandbox_regressions(self):
        self.run_candidate_suite('test_ci_runtime_reset_scope.py', {
            'test_scoped_shared_state_preserved_resources_filtered',
            'test_unknown_scope_preserves_all_state_and_resources',
            'test_scoped_podman_quadlet_only_selected_and_retains_xdg',
            'test_even_isolated_scoped_root_has_no_implicit_delete_authority',
            'test_unscoped_dedicated_runner_full_reset_unchanged',
        })

    def test_reviewed_consent_runs_all_eight_actual_shell_regressions(self):
        self.run_candidate_suite('test_ci_podman_typed_consent.py', {
            'test_fresh_two_stage_consent_preserves_incomplete_inventory',
            'test_single_stage_only_if_required_by_plan',
            'test_operation_sources_bind_to_their_own_plan_shape',
            'test_missing_invalid_or_destructive_impact_blocks',
            'test_scope_output_parser_accepts_only_one_emitted_plan',
            'test_helper_runs_from_application_workdir_using_checkout_root',
            'test_actual_lifecycle_shell_fragment_uses_fresh_consent_per_operation',
            'test_workflow_uses_fresh_operation_bound_consent_for_full_lifecycle',
        })

    def test_reviewed_retention_runs_all_seven_gc_regressions(self):
        path = ROOT / 'test_artifact_gc.py'
        if not path.exists() or 'def test_pr_provenance_and_native_evidence_keep_review_window(' not in path.read_text():
            self.skipTest('retention suite belongs only to its exact reviewed candidate')
        self.run_candidate_suite('test_artifact_gc.py', {
            'test_qualification_outcomes_and_branches_survive_gc',
            'test_artifact_gc_retains_active_refs_and_deletes_inactive',
            'test_pr_evidence_artifacts_are_not_ephemeral',
            'test_gc_discovery_failure_never_deletes',
            'test_candidate_sha_and_second_page_fail_closed',
            'test_pr_event_is_read_only_and_never_enters_cleanup',
            'test_pr_provenance_and_native_evidence_keep_review_window',
        })

class TrustedBindingActivationTests(unittest.TestCase):
    CHECKS = planner.CI_CHECKS
    FILES = {path: 'modified' for path in (
        'scripts/pr-check-runner.py', 'scripts/pr-gate-plan.py',
        'scripts/test_pr_gate_plan.py')}

    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.git('init', '-q')
        self.git('config', 'user.name', 'Fixture')
        self.git('config', 'user.email', 'fixture@example.invalid')
        self.files = self.FILES.copy()
        for path in self.files:
            file = self.root / path
            file.parent.mkdir(parents=True, exist_ok=True)
            file.write_bytes(b'base\nline 2\nline 3\nline 4\nline 5\nstable target field\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'base')
        self.base = self.git('rev-parse', 'HEAD')
        for path in self.files:
            (self.root / path).write_bytes(b'reviewed\r\nline 2\nline 3\nline 4\nline 5\nstable target field\n')
        self.git('commit', '-qam', 'reviewed candidate')
        self.head = self.git('rev-parse', 'HEAD')
        self.record = {
            'repository': 'mcpdev80/baseharbor', 'target_branch': 'develop',
            'reviewed_head': self.head, 'files': self.files,
            'file_hashes': {p: hashlib.sha256(b'reviewed\r\nline 2\nline 3\nline 4\nline 5\nstable target field\n').hexdigest()
                            for p in self.files},
            'required_checks': [c['id'] for c in self.CHECKS],
        }
        spec = importlib.util.spec_from_file_location('binding_resolver', ROOT / 'pr-diff-base.py')
        self.resolver = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.resolver)

    def git(self, *args):
        return subprocess.run(['git', '-C', str(self.root), *args],
                              check=True, capture_output=True, text=True).stdout.strip()

    def resolved(self, record=None, raw=None, head=None):
        self.git('checkout', '-q', '--detach', self.base)
        if raw is None:
            raw = '```json\n' + json.dumps(self.record if record is None else record) + '\n```\n'
        path = self.root / 'docs/internal/ci-binding-qualifications.md'
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(raw)
        self.git('add', '.')
        self.git('commit', '-qm', 'trusted qualification')
        target = self.git('rev-parse', 'HEAD')
        head = head or self.head
        tree = self.git('merge-tree', '--write-tree', target, head).splitlines()[0]
        self.git('reset', '-q')
        integration = self.git('commit-tree', tree, '-p', target, '-p', head,
                               '-m', 'actual PR integration')
        return self.resolver.resolve_pr_diff(self.root, target, head, integration)

    def checks(self, diff, repository='mcpdev80/baseharbor', branch='develop'):
        return planner.trusted_binding_checks(diff, self.root, repository, branch)

    def test_real_git_exact_candidate_and_raw_crlf_blobs_select_all_checks(self):
        actual = self.resolved()
        self.assertEqual(self.CHECKS, self.checks(actual))
        old = Path.cwd()
        try:
            os.chdir(self.root)
            with patch.dict(os.environ, {'GITHUB_REPOSITORY': 'mcpdev80/baseharbor',
                                         'GITHUB_BASE_REF': 'develop'}):
                result = planner.plan(actual, REQ, MAP)
        finally:
            os.chdir(old)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['selected_gates'])
        self.assertEqual({c['id'] for c in self.CHECKS},
                         {c['id'] for c in result['required_validation_checks']})

    def integrated(self, target, head):
        tree = self.git('merge-tree', '--write-tree', target, head).splitlines()[0]
        self.git('reset', '-q')
        integration = self.git('commit-tree', tree, '-p', target, '-p', head,
                               '-m', 'current PR integration')
        return self.resolver.resolve_pr_diff(self.root, target, head, integration)

    def test_real_git_record_target_merge_keeps_frozen_bytes_and_current_target_ancestry(self):
        actual = self.resolved()
        target = actual['target_base_sha']
        self.git('checkout', '-q', '--detach', self.head)
        self.git('merge', '--no-ff', '-qm', 'merge trusted record target', target)
        merged = self.git('rev-parse', 'HEAD')
        self.git('merge-base', '--is-ancestor', target, merged)
        current = self.integrated(target, merged)
        self.assertNotEqual(self.head, merged)
        self.assertEqual(target, current['merge_base_sha'])
        self.assertEqual(self.CHECKS, self.checks(current))
        # Additional trusted target advances may be integrated by the same owner.
        self.git('checkout', '-q', '--detach', target)
        (self.root / 'accepted-target.md').write_text('accepted target advance\n')
        planner_path = self.root / 'scripts/pr-gate-plan.py'
        planner_path.write_bytes(planner_path.read_bytes().replace(
            b'stable target field', b'independently accepted target field'))
        self.git('add', '.')
        self.git('commit', '-qm', 'later trusted target')
        later = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-q', '--detach', merged)
        self.git('merge', '--no-ff', '-qm', 'merge later trusted target', later)
        advanced = self.git('rev-parse', 'HEAD')
        # The authenticated target changed a pinned file. Frozen hashes still bind
        # reviewed source; strict merge trees bind every actual inherited byte.
        actual_blob = subprocess.check_output(['git', '-C', str(self.root), 'cat-file',
            'blob', advanced + ':scripts/pr-gate-plan.py'])
        self.assertNotEqual(self.record['file_hashes']['scripts/pr-gate-plan.py'],
                            hashlib.sha256(actual_blob).hexdigest())
        current = self.integrated(later, advanced)
        self.assertEqual(len(self.files), len(current['changes']))
        self.git('merge-base', '--is-ancestor', later, advanced)
        self.assertEqual(self.CHECKS, self.checks(current))

    def test_real_git_untrusted_sideparent_dirty_merge_and_copied_tree_fail_closed(self):
        actual = self.resolved()
        target = actual['target_base_sha']
        # A descendant of the target is not itself an authenticated target ancestor.
        self.git('checkout', '-q', '--detach', target)
        self.git('commit', '--allow-empty', '-qm', 'untrusted sideparent')
        side = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-q', '--detach', self.head)
        self.git('merge', '--no-ff', '-qm', 'untrusted side merge', side)
        side_head = self.git('rev-parse', 'HEAD')
        self.assertEqual(len(self.files), len(self.integrated(target, side_head)['changes']))
        self.assertIsNone(self.checks(self.integrated(target, side_head)))
        # Matching source bytes copied into a new linear commit do not prove review ancestry.
        copied = self.git('commit-tree', self.git('rev-parse', side_head + '^{tree}'),
                          '-p', target, '-m', 'unreviewed copied tree')
        self.assertIsNone(self.checks(self.integrated(target, copied)))
        # Right parent identities cannot authorize extra merge-resolution contents.
        self.git('checkout', '-q', '--detach', self.head)
        self.git('merge', '--no-ff', '--no-commit', target)
        (self.root / 'scripts/pr-check-runner.py').write_bytes(b'unreviewed merge resolution\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'dirty merge')
        dirty = self.git('rev-parse', 'HEAD')
        self.assertIsNone(self.checks(self.integrated(target, dirty)))

    def test_untrusted_missing_and_wrong_context_fail_closed(self):
        actual = self.resolved()
        for repository, branch in (('', ''), ('other/repo', 'develop'),
                                    ('mcpdev80/baseharbor', 'main')):
            with self.subTest(repository=repository, branch=branch):
                self.assertIsNone(self.checks(actual, repository, branch))
        # Candidate adds its own record: target still has no trusted record.
        candidate_target = actual['target_base_sha']
        forged = self.resolver.resolve_pr_diff(self.root, self.base, candidate_target)
        self.assertIsNone(self.checks(forged))
        missing = copy.deepcopy(actual)
        missing['target_base_sha'] = self.base
        self.assertIsNone(self.checks(missing))

    def test_malformed_incomplete_or_weakened_records_fail_closed(self):
        variants = []
        for key, value in (
                ('repository', 'other/repo'), ('target_branch', 'main'),
                ('reviewed_head', '0' * 40), ('reviewed_head', 'A' * 40),
                ('reviewed_head', 'f' * 39), ('reviewed_head', []), ('files', {}),
                ('file_hashes', {}), ('required_checks', ['ci-policy']),
                ('required_checks', ['ci-policy'] * 3),
                ('required_checks', ['unknown', 'ci-policy', 'pr-planner-tests']),
                ('required_checks', [[], 'ci-policy', 'pr-planner-tests'])):
            record = copy.deepcopy(self.record)
            record[key] = value
            variants.append(record)
        for path in self.files:
            record = copy.deepcopy(self.record)
            record['file_hashes'][path] = '0' * 64
            variants.append(record)
        record = copy.deepcopy(self.record)
        record['files']['cmd/baha/mcp_tools.go'] = 'modified'
        record['file_hashes']['cmd/baha/mcp_tools.go'] = '0' * 64
        variants.append(record)
        for record in variants:
            with self.subTest(record=record):
                self.assertIsNone(self.checks(self.resolved(record)))
        raw = '```json\n' + json.dumps(self.record) + '\n```\n'
        for malformed in ('no record', raw + raw, raw.replace('"repository":',
                '"repository": "other/repo", "repository":', 1),
                '```json\n{invalid}\n```\n'):
            with self.subTest(raw=malformed):
                self.assertIsNone(self.checks(self.resolved(raw=malformed)))

    def test_forged_inventory_integration_and_followup_content_fail_closed(self):
        actual = self.resolved()
        variants = []
        for key, value in (('integration_sha', None), ('integration_sha', '0' * 40),
                           ('merge_base_sha', '0' * 40), ('head_sha', '0' * 40)):
            changed = copy.deepcopy(actual)
            changed[key] = value
            variants.append(changed)
        for changes in (actual['changes'][:1], actual['changes'] * 2,
                        actual['changes'] + paths('extra.py'),
                        [{'status': 'deleted', 'path': p} for p in self.files],
                        [{'status': 'renamed', 'old_path': p, 'new_path': p + '.new'}
                         for p in self.files]):
            changed = copy.deepcopy(actual)
            changed['changes'] = changes
            variants.append(changed)
        # Correct parent IDs with a forged tree must still block.
        bad_tree = self.git('rev-parse', self.base + '^{tree}')
        changed = copy.deepcopy(actual)
        changed['integration_sha'] = self.git('commit-tree', bad_tree,
            '-p', actual['target_base_sha'], '-p', self.head, '-m', 'forged tree')
        variants.append(changed)
        for changed in variants:
            with self.subTest(diff=changed):
                self.assertIsNone(self.checks(changed))
        self.git('checkout', '-q', '--detach', self.head)
        (self.root / 'scripts/pr-check-runner.py').write_text('unreviewed followup\n')
        self.git('commit', '-qam', 'unreviewed followup')
        followup = self.git('rev-parse', 'HEAD')
        self.assertIsNone(self.checks(self.resolved(head=followup)))


class WorkflowBindingActivationTests(TrustedBindingActivationTests):
    CHECKS = planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS
    FILES = {**TrustedBindingActivationTests.FILES, '.github/workflows/ci.yml': 'modified'}


class PinbumpBindingActivationTests(TrustedBindingActivationTests):
    CHECKS = planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS
    FILES = planner.PINBUMP_INVENTORY

    def test_pinbump_inventory_resolves_with_full_check_set(self):
        actual = self.resolved()
        self.assertEqual(len(self.FILES), len(actual['changes']))
        self.assertEqual(self.CHECKS, self.checks(actual))

    def test_pinbump_inventory_missing_file_fails_closed(self):
        for path in list(self.FILES):
            record = copy.deepcopy(self.record)
            del record['files'][path]
            del record['file_hashes'][path]
            with self.subTest(missing=path):
                self.assertIsNone(self.checks(self.resolved(record)))


class BootstrapExceptionTests(unittest.TestCase):
    """Fail-first: only PRs whose HEAD references the trusted Akte bootstrap.

    The fail-closed validator needs the first merge but its Core-921 register
    is stale, so it can never qualify the two bootstrap PRs. This bounded,
    time-boxed exception trusts only the frozen Akte read from the target base
    and requires the PR HEAD to reference it. It does NOT waive required checks
    or review; it only returns the CI check catalog so pr-required-final.py
    still enforces every green check.
    """

    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.git('init', '-q')
        self.git('config', 'user.name', 'Bootstrap fixture')
        self.git('config', 'user.email', 'fixture@example.test')

    def git(self, *args):
        result = subprocess.run(['git', '-C', str(self.root), *args],
                text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(0, result.returncode, result.stderr)
        return result.stdout.strip()

    def _bootstrap_repo(self, head_message):
        # Trusted develop baseline with the immutable Akte present.
        (self.root / 'docs' / 'internal').mkdir(parents=True, exist_ok=True)
        (self.root / 'scripts').mkdir(parents=True, exist_ok=True)
        akte = (
            '# Core-921 CI binding qualification (#1056)\n'
            'Owner-authorized immutable source inventory.\n'
            '```json\n'
            '{"repository": "mcpdev80/baseharbor", "target_branch": "develop", '
            '"reviewed_head": "0" * 40, '
            '"files": {"scripts/pr-check-runner.py": "modified"}, '
            '"file_hashes": {"scripts/pr-check-runner.py": "0" * 64}, '
            '"required_checks": ["pr-planner-tests"]}\n'
            '```\n'
        )
        (self.root / 'docs' / 'internal' / 'ci-binding-qualifications.md').write_text(akte)
        self.git('add', '.')
        self.git('commit', '-qm', 'trusted develop baseline with Akte')
        develop = self.git('rev-parse', 'HEAD')
        # PR head: touches the three CI files and references the Akte in the
        # commit message (bootstrap-eligible).
        (self.root / 'scripts' / 'pr-gate-plan.py').write_text('bootstrap planner\n')
        (self.root / 'scripts' / 'pr-check-runner.py').write_text('bootstrap runner\n')
        (self.root / 'scripts' / 'test_pr_gate_plan.py').write_text('bootstrap test\n')
        self.git('add', '.')
        self.git('commit', '-qm', head_message)
        head = self.git('rev-parse', 'HEAD')
        tree = self.git('merge-tree', '--write-tree', develop, head)
        integration = self.git('commit-tree', tree, '-p', develop, '-p', head,
                               '-m', 'bootstrap merge')
        rspec = importlib.util.spec_from_file_location(
            'bootstrap_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(resolver)
        actual = resolver.resolve_pr_diff(self.root, develop, head, integration)
        self.assertFalse(actual['broad_required'], actual['reasons'])
        return actual

    def test_bootstrap_pr_referencing_akte_stays_blocked(self):
        actual = self._bootstrap_repo(
            'ci(planner): bootstrap via '
            'docs/internal/ci-binding-qualifications.md Core-921 CI binding qualification')
        result = planner.plan(actual, REQ, MAP, repository=self.root,
                              context_repository='mcpdev80/baseharbor',
                              context_target_branch='develop')
        self.assertEqual('unresolved', result['coverage_status'])

    def test_bootstrap_without_akte_reference_stays_blocked(self):
        # Fail-first negative: same three CI files, no Akte reference anywhere.
        actual = self._bootstrap_repo('ci(planner): change the three CI files')
        result = planner.plan(actual, REQ, MAP, repository=self.root,
                              context_repository='mcpdev80/baseharbor',
                              context_target_branch='develop')
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])

    def test_bootstrap_tampered_akte_on_base_stays_blocked(self):
        # The immutable Akte on the target base is the sole authority. A base
        # whose Akte no longer parses as a JSON block must not bootstrap, even
        # though the CI-script paths alone would resolve via the planning lane.
        (self.root / 'docs' / 'internal').mkdir(parents=True, exist_ok=True)
        (self.root / 'scripts').mkdir(parents=True, exist_ok=True)
        (self.root / 'docs' / 'internal' / 'ci-binding-qualifications.md').write_text(
            '# not a qualification record\nno fenced json block here\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'develop with tampered Akte')
        develop = self.git('rev-parse', 'HEAD')
        # Adding a workflow file forces an unresolved area on top of the
        # planning scripts so the only resolution path is the bootstrap record.
        (self.root / '.github' / 'workflows').mkdir(parents=True, exist_ok=True)
        (self.root / '.github' / 'workflows' / 'extra.yml').write_text('name: x\n')
        self.git('add', '.')
        self.git('commit', '-qm',
                 'ci(planner): bootstrap via '
                 'docs/internal/ci-binding-qualifications.md Core-921 CI binding qualification')
        head = self.git('rev-parse', 'HEAD')
        tree = self.git('merge-tree', '--write-tree', develop, head)
        integration = self.git('commit-tree', tree, '-p', develop, '-p', head, '-m', 'merge')
        rspec = importlib.util.spec_from_file_location(
            'bootstrap_diff2', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(resolver)
        actual = resolver.resolve_pr_diff(self.root, develop, head, integration)
        result = planner.plan(actual, REQ, MAP, repository=self.root,
                              context_repository='mcpdev80/baseharbor',
                              context_target_branch='develop')
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])

    def test_bootstrap_wrong_repository_stays_blocked(self):
        actual = self._bootstrap_repo(
            'ci(planner): bootstrap via '
            'docs/internal/ci-binding-qualifications.md')
        result = planner.plan(actual, REQ, MAP, repository=self.root,
                              context_repository='other/repo',
                              context_target_branch='develop')
        self.assertEqual('unresolved', result['coverage_status'])


class RuntimeRetirementExecutionTests(unittest.TestCase):
    def test_exact_regression_and_safety_families_must_execute_and_pass(self):
        from types import SimpleNamespace
        spec = importlib.util.spec_from_file_location('retirement_runner', ROOT / 'pr-check-runner.py')
        runner = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(runner)
        patterns = runner.WIZARD_TEST_FAMILIES['wizard-runtime-retirement-tests']
        self.assertEqual('^TestRuntimeRetirementVerifiesPreviouslyUnboundCoreSQLForHTTPOnlyApplication$', patterns[0])
        for mode in ('pass', 'missing', 'skip', 'no-pass', 'failure'):
            def run(args, **kwargs):
                pattern = args[args.index('-list' if '-list' in args else '-run') + 1]
                name = pattern.removeprefix('^').removesuffix('$')
                if '-list' in args:
                    return SimpleNamespace(returncode=0, stdout='' if mode == 'missing' else name + '\n', stderr='')
                output = '=== RUN   ' + name + '\n'
                if mode != 'no-pass':
                    output += ('--- SKIP: ' if mode == 'skip' else '--- PASS: ') + name + ' (0.00s)\n'
                return SimpleNamespace(returncode=int(mode == 'failure'), stdout=output)
            with self.subTest(mode=mode), patch.object(runner.subprocess, 'run', side_effect=run):
                if mode == 'pass':
                    self.assertEqual(len(patterns), len(runner.execute_validation('wizard-runtime-retirement-tests', '.')))
                else:
                    with self.assertRaises(RuntimeError):
                        runner.execute_validation('wizard-runtime-retirement-tests', '.')


class OpenBaoExactRetryRouteTests(unittest.TestCase):
    def runner(self):
        spec = importlib.util.spec_from_file_location('openbao_runner', ROOT / 'pr-check-runner.py')
        runner = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(runner)
        return runner

    def test_actual_workflow_route_runs_trusted_helper_and_rejects_missing_or_skipped_tests(self):
        workflow = (ROOT.parent / '.github/workflows/ci.yml').read_text()
        start = workflow.index('            openbao-package-tests)\n') + len('            openbao-package-tests)\n')
        fragment = workflow[start:workflow.index('              ;;', start)]
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            trusted, candidate, temp = (root / name for name in ('trusted', 'candidate', 'temp'))
            for directory in (trusted, candidate, temp):
                directory.mkdir()
            def git(directory, *args):
                return subprocess.check_output(['git', '-C', str(directory), *args], text=True).strip()
            for directory in (trusted, candidate):
                git(directory, 'init', '-q')
                git(directory, 'config', 'user.name', 'Workflow fixture')
                git(directory, 'config', 'user.email', 'fixture@example.invalid')
            (trusted / 'scripts').mkdir()
            (trusted / 'scripts/pr-check-runner.py').write_text((ROOT / 'pr-check-runner.py').read_text())
            git(trusted, 'add', '.')
            git(trusted, 'commit', '-qm', 'trusted helper')
            git(candidate, 'remote', 'add', 'origin', str(trusted))
            (candidate / 'go.mod').write_text('module example.test/openbaoroute\n\ngo 1.25\n')
            package = candidate / 'internal/openbao'
            package.mkdir(parents=True)
            environment = dict(os.environ, BASE_SHA=git(trusted, 'rev-parse', 'HEAD'),
                               VALIDATION='openbao-package-tests', RUNNER_TEMP=str(temp),
                               GITHUB_WORKSPACE=str(candidate))
            names = self.runner().OPENBAO_REQUIRED_TESTS
            for mode in ('pass', 'missing', 'skip'):
                selected = names[:-1] if mode == 'missing' else names
                text = 'package openbao\nimport "testing"\n'
                text += '\n'.join('func %s(t *testing.T) { %s }' %
                                  (name, 't.Skip("not executed")' if mode == 'skip' and name == names[0] else '')
                                  for name in selected)
                (package / 'bootstrap_test.go').write_text(text)
                execution = subprocess.run(['bash', '-euo', 'pipefail', '-c', fragment],
                                           cwd=candidate, env=environment, capture_output=True, text=True)
                if mode == 'pass':
                    self.assertEqual(0, execution.returncode, execution.stderr)
                    self.assertEqual(list(names), json.loads(execution.stdout)['matched_tests'])
                else:
                    self.assertNotEqual(0, execution.returncode, execution.stdout)
                    self.assertIn('OpenBao', execution.stderr)

    def test_strict_discovery_execution_and_existing_safety_names(self):
        from types import SimpleNamespace
        runner = self.runner()
        names = runner.OPENBAO_REQUIRED_TESTS
        discovery = '\n'.join(names)
        execution = ''.join('=== RUN   ' + n + '\n--- PASS: ' + n + ' (0.00s)\n' for n in names)
        for listed, output, code, success in (
                (discovery, execution, 0, True), ('', execution, 0, False),
                ('\n'.join(names[:-1]), execution, 0, False),
                (discovery, '', 0, False), (discovery, execution, 1, False),
                (discovery, execution.replace('--- PASS: ' + names[0], '--- SKIP: ' + names[0]), 0, False)):
            with self.subTest(listed=listed, code=code, success=success):
                results = [SimpleNamespace(returncode=0, stdout=listed, stderr=''),
                           SimpleNamespace(returncode=code, stdout=output)]
                with patch.object(runner.subprocess, 'run', side_effect=results) as process:
                    if success:
                        self.assertEqual(list(names), runner.execute_validation('openbao-package-tests', '.'))
                        self.assertIn('./internal/openbao/...', process.call_args.args[0])
                        self.assertIn('-run', process.call_args.args[0])
                    else:
                        with self.assertRaises(RuntimeError):
                            runner.execute_validation('openbao-package-tests', '.')
        # A renamed replacement must not satisfy an existing exact SQL safety family.
        replacements = '\n'.join(p.removeprefix('^') + 'Replacement'
                                  for p in runner.WIZARD_TEST_FAMILIES['wizard-core918-native-sql-safety-tests'])
        with patch.object(runner.subprocess, 'run', return_value=SimpleNamespace(
                returncode=0, stdout=replacements, stderr='')):
            with self.assertRaises(RuntimeError):
                runner.execute_validation('wizard-core918-native-sql-safety-tests', '.')

    def test_real_git_frozen_source_forward_merge_and_rejections(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            def git(*args):
                return subprocess.check_output(['git', '-C', str(root), *args], text=True).strip()
            git('init', '-q')
            git('config', 'user.name', 'Retry fixture')
            git('config', 'user.email', 'fixture@example.invalid')
            for path in planner.OPENBAO_RETRY_FILES:
                item = root / path
                item.parent.mkdir(parents=True, exist_ok=True)
                item.write_text('base\n')
            git('add', '.')
            git('commit', '-qm', 'base')
            target = git('rev-parse', 'HEAD')
            for path in planner.OPENBAO_RETRY_FILES:
                (root / path).write_text('frozen retry\n')
            git('commit', '-qam', 'frozen retry')
            reviewed = git('rev-parse', 'HEAD')
            spec = importlib.util.spec_from_file_location('retry_diff', ROOT / 'pr-diff-base.py')
            resolver = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(resolver)
            def actual(base, head):
                tree = git('merge-tree', '--write-tree', base, head).splitlines()[0]
                integration = git('commit-tree', tree, '-p', base, '-p', head, '-m', 'integration')
                return resolver.resolve_pr_diff(root, base, head, integration)
            def checks(value, repo='mcpdev80/baseharbor', branch='develop'):
                return planner.openbao_retry_checks(value, root, repo, branch)
            with patch.object(planner, 'OPENBAO_RETRY_HEAD', reviewed):
                value = actual(target, reviewed)
                self.assertEqual(planner.CODE_CHECKS + planner.OPENBAO_CHECKS, checks(value))
                planned = planner.plan(value, REQ, MAP, repository=root,
                                       context_repository='mcpdev80/baseharbor', context_target_branch='develop')
                self.assertEqual('resolved', planned['coverage_status'], planned)
                self.assertIn('openbao-package-tests', [c['id'] for c in planned['required_validation_checks']])
                self.assertIsNone(checks(value, repo='other/repo'))
                self.assertIsNone(checks(value, branch='main'))
                for key, changed in (('integration_sha', None), ('merge_base_sha', 'f' * 40),
                                     ('changes', value['changes'][:1])):
                    forged = copy.deepcopy(value)
                    forged[key] = changed
                    self.assertIsNone(checks(forged))
                git('checkout', '-q', '--detach', target)
                (root / 'accepted-target.md').write_text('accepted target\n')
                git('add', '.')
                git('commit', '-qm', 'advance trusted target')
                target = git('rev-parse', 'HEAD')
                git('checkout', '-q', '--detach', reviewed)
                git('merge', '--no-ff', '-qm', 'forward target', target)
                head = git('rev-parse', 'HEAD')
                self.assertIsNotNone(checks(actual(target, head)))
                (root / 'internal/openbao/bootstrap.go').write_text('unreviewed followup\n')
                git('commit', '-qam', 'unreviewed followup')
                self.assertIsNone(checks(actual(target, git('rev-parse', 'HEAD'))))


class AktenRegisterQualificationTests(unittest.TestCase):
    # F40: the qualification register may accumulate one well-formed entry per
    # reviewed candidate. A PR that changes ONLY the three CI scripts resolves
    # exactly when the authenticated base register carries the entry binding
    # this candidate (head chain + raw blob hashes); without that entry the
    # conservative ci-executor-unresolved fallback must keep it unresolved.

    CI_PATHS = ('scripts/pr-check-runner.py', 'scripts/pr-gate-plan.py',
                'scripts/test_pr_gate_plan.py')
    RECORD_PATH = 'docs/internal/ci-binding-qualifications.md'
    CHECKS = planner.CI_CHECKS

    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.git('init', '-q')
        self.git('config', 'user.name', 'Register fixture')
        self.git('config', 'user.email', 'register@example.invalid')
        for path in self.CI_PATHS:
            file = self.root / path
            file.parent.mkdir(parents=True, exist_ok=True)
            file.write_text('baseline\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'base')
        self.base = self.git('rev-parse', 'HEAD')
        # Historical reviewed head H_old: same three paths, other content, on a
        # dead-end branch that is not an ancestor of the current candidate.
        self.git('checkout', '-q', '--detach', self.base)
        self.git('checkout', '-q', '-b', 'historical')
        for path in self.CI_PATHS:
            (self.root / path).write_text('historical reviewed\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'historical reviewed head')
        self.old_head = self.git('rev-parse', 'HEAD')
        # Current PR-only-CI-scripts candidate head H.
        self.git('checkout', '-q', '--detach', self.base)
        self.git('checkout', '-q', '-b', 'candidate')
        for path in self.CI_PATHS:
            (self.root / path).write_text('candidate reviewed\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'candidate head')
        self.head = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-q', '--detach', self.base)
        spec = importlib.util.spec_from_file_location(
            'register_resolver', ROOT / 'pr-diff-base.py')
        self.resolver = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.resolver)

    def git(self, *args):
        return subprocess.run(['git', '-C', str(self.root), *args],
                              check=True, capture_output=True, text=True).stdout.strip()

    def blob_sha(self, head, path):
        blob = subprocess.run(['git', '-C', str(self.root), 'cat-file', 'blob',
                               f'{head}:{path}'], check=True,
                              capture_output=True).stdout
        return hashlib.sha256(blob).hexdigest()

    def entry(self, head_sha):
        return {'repository': 'mcpdev80/baseharbor', 'target_branch': 'develop',
                'reviewed_head': head_sha,
                'files': {path: 'modified' for path in self.CI_PATHS},
                'file_hashes': {path: self.blob_sha(head_sha, path)
                                for path in self.CI_PATHS},
                'required_checks': [c['id'] for c in self.CHECKS]}

    def plan_with_register(self, entries):
        record = ''.join('```json\n' + json.dumps(entry) + '\n```\n'
                         for entry in entries)
        path = self.root / self.RECORD_PATH
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(record)
        self.git('add', '.')
        self.git('commit', '-qm', 'register entry on trusted base')
        target = self.git('rev-parse', 'HEAD')
        tree = self.git('merge-tree', '--write-tree', target, self.head).splitlines()[0]
        self.git('reset', '-q')
        integration = self.git('commit-tree', tree, '-p', target, '-p', self.head,
                               '-m', 'register integration')
        actual = self.resolver.resolve_pr_diff(self.root, target, self.head,
                                               integration)
        previous = os.getcwd()
        os.chdir(str(self.root))
        try:
            return planner.plan(actual, REQ, MAP,
                                context_repository='mcpdev80/baseharbor',
                                context_target_branch='develop')
        finally:
            os.chdir(previous)

    def test_pr_only_ci_scripts_without_entry_stays_unresolved(self):
        # Register carries only the historical entry; the current candidate has
        # no entry, so the exact pr-check-runner.py blocker must remain.
        planned = self.plan_with_register([self.entry(self.old_head)])
        self.assertEqual('unresolved', planned['coverage_status'], planned)
        self.assertIn('CI execution/evidence impact requires specific '
                      'qualification: scripts/pr-check-runner.py',
                      planned['unresolved_coverage'])

    def test_pr_only_ci_scripts_with_entry_resolves(self):
        # Same historical entry PLUS the current candidate's entry: the
        # register-driven proof must resolve with the CI check catalog only.
        planned = self.plan_with_register([self.entry(self.old_head),
                                           self.entry(self.head)])
        self.assertEqual('resolved', planned['coverage_status'], planned)
        self.assertEqual([], planned['unresolved_coverage'])
        self.assertEqual([], planned['selected_gates'])
        self.assertEqual({c['id'] for c in self.CHECKS},
                         {c['id'] for c in planned['required_validation_checks']})

    def test_duplicate_or_foreign_register_entries_fail_closed(self):
        # An identical entry twice is a malformed register and never silently
        # qualifies; a single current entry still resolves (record-per-file).
        duplicate = self.plan_with_register([self.entry(self.head),
                                             self.entry(self.head)])
        self.assertEqual('unresolved', duplicate['coverage_status'], duplicate)
        single = self.plan_with_register([self.entry(self.head)])
        self.assertEqual('resolved', single['coverage_status'], single)


class WorkflowRegisterQualificationTests(AktenRegisterQualificationTests):
    CI_PATHS = (*AktenRegisterQualificationTests.CI_PATHS, '.github/workflows/ci.yml')
    CHECKS = planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS

    def test_workflow_inventory_cannot_omit_syntax_validation(self):
        entry = self.entry(self.head)
        entry['required_checks'].remove('ci-actionlint')
        planned = self.plan_with_register([entry])
        self.assertEqual('unresolved', planned['coverage_status'], planned)


class BootstrapClaimRejectionTests(unittest.TestCase):
    def test_noncanonical_pins_paths_never_select_the_top_level_validator(self):
        for path in ('docs/releases/nested/v0.4.26.consumer-pins.json',
                     'docs/releases/unknown.consumer-pins.json',
                     'docs/releases/v0.4.27.consumer-pins.json'):
            with self.subTest(path=path):
                result = planner.plan(diff([{'status': 'added', 'path': path}]), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'], result)

    def test_commit_message_and_record_title_never_qualify_unreviewed_content(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            git_init(root)
            record = root / 'docs/internal/ci-binding-qualifications.md'
            record.parent.mkdir(parents=True)
            record.write_text('# Core-921 CI binding qualification\n```json\n{}\n```\n')
            subprocess.run(['git', '-C', str(root), 'add', '.'], check=True)
            subprocess.run(['git', '-C', str(root), 'commit', '-qm', 'target record'], check=True)
            target = git_rev_parse(root, 'HEAD')
            unreviewed = root / 'scripts/pr-check-runner.py'
            unreviewed.parent.mkdir()
            unreviewed.write_text('unreviewed executor\n')
            subprocess.run(['git', '-C', str(root), 'add', '.'], check=True)
            subprocess.run(['git', '-C', str(root), 'commit', '-qm',
                            'mention docs/internal/ci-binding-qualifications.md'], check=True)
            head = git_rev_parse(root, 'HEAD')
            spec = importlib.util.spec_from_file_location('bootstrap_diff', ROOT / 'pr-diff-base.py')
            resolver = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(resolver)
            actual = resolver.resolve_pr_diff(root, target, head, git_commit_tree(root, target, head))
            self.assertFalse(actual['broad_required'], actual['reasons'])
            result = planner.plan(actual, REQ, MAP, repository=root,
                                  context_repository='mcpdev80/baseharbor',
                                  context_target_branch='develop')
            self.assertEqual('unresolved', result['coverage_status'], result)


class PinsDataCanonicalExecutionTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.path = self.root / 'docs/releases/v0.4.26.consumer-pins.json'
        self.path.parent.mkdir(parents=True)
        sys.path.insert(0, str(ROOT))
        self.addCleanup(lambda: sys.path.remove(str(ROOT)))
        from ecosystem_release_pins import resolve_pins
        candidate = {'schema': 'baseharbor.ecosystem-candidate/v1', 'release': '0.4.26',
                     'core': 'a' * 40, 'demo': 'b' * 40, 'console': 'c' * 40}
        console = {k: candidate[k] for k in ('schema', 'release', 'core', 'demo')}
        self.pins = resolve_pins('a' * 40, 'd' * 40, candidate, console,
                                 'a' * 40, release='0.4.26')

    def execute(self, pins):
        self.path.write_text(json.dumps(pins))
        return subprocess.run([sys.executable, str(ROOT / 'pr-check-runner.py'),
                               'execute', '--check', 'pins-data-validation',
                               '--repository', str(self.root)],
                              capture_output=True, text=True)

    def test_real_cli_accepts_canonical_and_historical_metadata(self):
        execution = self.execute(self.pins)
        self.assertEqual(0, execution.returncode, execution.stderr)
        self.assertEqual(['v0.4.26.consumer-pins.json'],
                         json.loads(execution.stdout)['matched_tests'])
        historical = self.path.with_name('v0.4.23.consumer-pins.json')
        historical.write_text((ROOT.parent / 'docs/releases/v0.4.23.consumer-pins.json').read_text())
        execution = self.execute(self.pins)
        self.assertEqual(0, execution.returncode, execution.stderr)
        self.assertEqual(2, len(json.loads(execution.stdout)['matched_tests']))

    def test_real_cli_rejects_inconsistent_graph_gates_and_origins(self):
        mutations = [
            lambda p: p.pop('ecosystem'),
            lambda p: p['ecosystem']['demo']['consumes'].update(core='e' * 40),
            lambda p: p['ecosystem']['core']['consumes'].update(connector='d' * 40),
            lambda p: p['ecosystem']['console']['consumes'].pop('demo'),
            lambda p: p['ecosystem']['connector'].update(commit='e' * 40),
            lambda p: p['ecosystem']['console'].update(commit='e' * 40),
            lambda p: p['ecosystem'].update(core=[]),
            lambda p: p['gates'].pop('integration/podman/remote-target'),
            lambda p: p['gates'].update(unknown={}),
            lambda p: p['gates']['integration/docker/remote-target']['connector'].update(
                repository='mcpdev80/baseharbor-console'),
            lambda p: p['gates']['integration/podman/remote-target']['connector'].update(
                workflow='untrusted.yml'),
            lambda p: p['gates']['integration/static/live-console']['console'].update(commit='oops'),
            lambda p: p['gates']['integration/static/live-console']['console']['origin'].update(
                commit='e' * 40),
            lambda p: p['gates']['integration/static/live-console']['connector'].pop('origin'),
            lambda p: p['gates']['integration/static/live-console'].update(console=[]),
        ]
        for number, mutate in enumerate(mutations):
            with self.subTest(case=number):
                pins = copy.deepcopy(self.pins)
                mutate(pins)
                execution = self.execute(pins)
                self.assertNotEqual(0, execution.returncode, execution.stdout)
                self.assertIn('consumer-pins file differs', execution.stderr)

    def test_exact_workflow_route_uses_trusted_modules_and_propagates_failure(self):
        workflow = (ROOT.parent / '.github/workflows/ci.yml').read_text()
        start = workflow.index('            pins-data-validation)\n') + len('            pins-data-validation)\n')
        fragment = workflow[start:workflow.index('              ;;', start)]
        trusted = self.root / 'trusted'
        trusted.mkdir()
        git_init(trusted)
        (trusted / 'scripts').mkdir()
        for helper in ('pr-check-runner.py', 'ecosystem_release_pins.py', 'private_consumer_evidence.py'):
            (trusted / 'scripts' / helper).write_text((ROOT / helper).read_text())
        subprocess.run(['git', '-C', str(trusted), 'add', '.'], check=True, capture_output=True)
        subprocess.run(['git', '-C', str(trusted), 'commit', '-qm', 'trusted validators'],
                       check=True, capture_output=True)
        base = git_rev_parse(trusted, 'HEAD')
        candidate = self.root / 'candidate'
        candidate.mkdir()
        git_init(candidate)
        subprocess.run(['git', '-C', str(candidate), 'remote', 'add', 'origin', str(trusted)], check=True)
        # Candidate modules with the same names must never execute.
        for helper in ('pr-check-runner.py', 'ecosystem_release_pins.py', 'private_consumer_evidence.py'):
            (candidate / helper).write_text('raise RuntimeError("CANDIDATE MODULE EXECUTED")\n')
        path = candidate / 'docs/releases/v0.4.26.consumer-pins.json'
        path.parent.mkdir(parents=True)
        temp = self.root / 'runner-temp'
        temp.mkdir()
        environment = dict(os.environ, BASE_SHA=base, VALIDATION='pins-data-validation',
                           RUNNER_TEMP=str(temp), GITHUB_WORKSPACE=str(candidate))
        for valid in (True, False):
            payload = copy.deepcopy(self.pins)
            if not valid:
                payload['ecosystem']['demo']['consumes']['core'] = 'e' * 40
            path.write_text(json.dumps(payload))
            execution = subprocess.run(['bash', '-euo', 'pipefail', '-c', fragment],
                                       cwd=candidate, env=environment, capture_output=True, text=True)
            self.assertNotIn('CANDIDATE MODULE EXECUTED', execution.stderr)
            if valid:
                self.assertEqual(0, execution.returncode, execution.stderr)
                self.assertEqual(['v0.4.26.consumer-pins.json'],
                                 json.loads(execution.stdout)['matched_tests'])
            else:
                self.assertNotEqual(0, execution.returncode, execution.stdout)
                self.assertIn('ecosystem consumer pin differs', execution.stderr)




class ReleasePreflightSelectionTests(unittest.TestCase):
    def test_actual_git_blob_requires_complete_trusted_non_journey_catalog(self):
        path = 'docs/releases/v0.4.26.selected-gates.json'
        expected = sorted(g['id'] for g in REQ['gates']
                          if not g['id'].startswith('journey/'))
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            git_init(root)
            file = root / path
            file.parent.mkdir(parents=True)
            for content, status, accepted in (
                    (expected, 'added', True),
                    (expected[:-1], 'added', False),
                    (expected + expected[:1], 'added', False),
                    (expected + ['journey/docker'], 'added', False),
                    (expected, 'modified', False),
                    ('invalid-json', 'added', False)):
                with self.subTest(content=content, status=status):
                    file.write_text(json.dumps(content))
                    subprocess.run(['git', '-C', str(root), 'add', '.'], check=True)
                    subprocess.run(['git', '-C', str(root), 'commit', '--allow-empty',
                                    '-qm', 'selection fixture'], check=True)
                    value = diff([{'path': path, 'status': status}])
                    value['head_sha'] = git_rev_parse(root, 'HEAD')
                    result = planner.plan(value, REQ, MAP, repository=root)
                    self.assertEqual('resolved' if accepted else 'unresolved',
                                     result['coverage_status'], result)
                    if accepted:
                        self.assertEqual([], result['selected_gates'])
                        self.assertEqual({c['id'] for c in planner.CI_CHECKS},
                                         {c['id'] for c in result['required_validation_checks']})


class SQLPreviewQualificationTests(unittest.TestCase):
    def test_exact_source_clean_integration_and_authority_failures(self):
        spec = importlib.util.spec_from_file_location('sql_preview_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(resolver)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            def git(*args):
                result = subprocess.run(['git', '-C', str(root), *args], text=True, capture_output=True)
                self.assertEqual(0, result.returncode, result.stderr)
                return result.stdout.strip()
            git_init(root)
            # Bind all product/read-scope/authored-source regression paths.
            files = planner.EDIT_SQL_PREVIEW_REVIEWED_FILES
            for path in files:
                (root / path).parent.mkdir(parents=True, exist_ok=True)
                (root / path).write_text('baseline\n')
            git('add', '.')
            git('commit', '-qm', 'baseline')
            baseline = git('rev-parse', 'HEAD')
            git('checkout', '-qb', 'source')
            for path in files:
                (root / path).write_text('reviewed\n')
            git('commit', '-qam', 'reviewed SQL projection')
            reviewed = git('rev-parse', 'HEAD')
            git('checkout', '-qb', 'target', baseline)
            (root / 'README.md').write_text('trusted update\n')
            git('add', '.')
            git('commit', '-qm', 'trusted target')
            target = git('rev-parse', 'HEAD')
            git('checkout', '-q', 'source')
            git('merge', '--no-ff', '-qm', 'clean integration', target)
            head = git('rev-parse', 'HEAD')
            def actual(candidate):
                tree = git('merge-tree', '--write-tree', target, candidate).splitlines()[0]
                integration = git('commit-tree', tree, '-p', target, '-p', candidate, '-m', 'PR integration')
                return resolver.resolve_pr_diff(root, target, candidate, integration)
            def plan(value, requirements=REQ):
                return planner.plan(value, requirements, MAP, repository=root)
            def assert_rejected(value):
                result = plan(value)
                self.assertFalse(any(reason.startswith('Owner-qualified SQL Edit source:') for reason in result['reasons']), result)
                if any(c['path'] == 'cmd/baha/app_edit_apply_result.go' for c in value['changes']):
                    self.assertEqual('unresolved', result['coverage_status'], result)
            with patch.object(planner, 'EDIT_SQL_PREVIEW_REVIEWED_HEAD', reviewed):
                value = actual(head)
                result = plan(value)
                self.assertEqual('resolved', result['coverage_status'], result)
                self.assertEqual(sorted(planner.EDIT_SQL_PREVIEW_GATES), result['selected_gates'])
                required = planner.CODE_CHECKS + planner.CLI_CHECKS + planner.WIZARD_BASE_CHECKS + [wizard_check(name) for name in planner.EDIT_SQL_PREVIEW_CHECKS]
                self.assertEqual({c['id'] for c in required}, {c['id'] for c in result['required_validation_checks']})
                for changes in [value['changes'][:-1], value['changes'] + paths('unreviewed.go'),
                                [dict(c, status='deleted') if i == 0 else c for i, c in enumerate(value['changes'])]]:
                    assert_rejected(dict(value, changes=changes))
                assert_rejected(dict(value, integration_sha=None))
                assert_rejected(dict(value, broad_required=True))
                with patch.object(planner, 'EDIT_SQL_PREVIEW_REVIEWED_HEAD', 'f' * 40):
                    assert_rejected(value)
                missing = copy.deepcopy(REQ)
                missing['gates'] = [g for g in missing['gates'] if g['id'] != 'atomic/podman/data-capabilities']
                self.assertEqual('unresolved', plan(value, missing)['coverage_status'])
                (root / 'cmd/baha/app_edit_apply_result.go').write_text('unreviewed followup\n')
                git('commit', '-qam', 'unreviewed followup')
                followup = git('rev-parse', 'HEAD')
                assert_rejected(actual(followup))
                dirty = git('commit-tree', git('rev-parse', followup + '^{tree}'),
                            '-p', reviewed, '-p', target, '-m', 'injected merge')
                assert_rejected(actual(dirty))


class ReceiptControlPlaneExecutionTests(unittest.TestCase):
    """Bootstrap regressions: receipt-execution-tests coupling and actual suite execution."""

    def check_ids(self, result):
        return {check['id'] for check in result['required_validation_checks']}

    def test_complete_control_plane_inventory_resolves(self):
        names = (sorted(planner.RECEIPT_CONTROL_PLANE_SCRIPTS) +
                 sorted(planner.RECEIPT_CONTROL_PLANE_TEST_SCRIPTS) +
                 sorted(planner.RECEIPT_CONTROL_PLANE_WORKFLOWS))
        changes = [{'status': 'modified', 'path': path} for path in names]
        result = planner.plan(diff(changes), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['unresolved_coverage'])
        self.assertFalse(result['broad_required'])
        self.assertTrue(self.check_ids(result) <= set(runner_validations()))

    def test_unknown_receipt_like_path_stays_unresolved(self):
        for path in ('scripts/receipt_publisher.py', 'evidence/receipts/pilot.json'):
            with self.subTest(path=path):
                result = planner.plan(diff([{'status': 'added', 'path': path}]), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertIn(path, ' '.join(result['unresolved_coverage']))

    def test_active_native_workflow_still_requires_native(self):
        path = '.github/workflows/runtime-s3-acceptance.yml'
        result = planner.plan(diff([{'status': 'modified', 'path': path}]), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertIn('native qualification', ' '.join(result['unresolved_coverage']))

    def test_receipt_execution_check_actually_runs_receipt_suites(self):
        # Coupling guard: the receipt-execution-tests id the planner emits must
        # be whitelisted by the runner AND resolve to the real receipt suites,
        # so a receipt PR cannot pass CI with only a filename match.
        rspec = importlib.util.spec_from_file_location(
            'runner', Path(__file__).with_name('pr-check-runner.py'))
        runner = importlib.util.module_from_spec(rspec)
        rspec.loader.exec_module(runner)
        self.assertIn('receipt-execution-tests', runner.VALIDATIONS)
        emitted = {check['id'] for check in planner.CI_CHECKS}
        self.assertIn('receipt-execution-tests', emitted)
        executed = runner.execute_validation(
            'receipt-execution-tests', str(Path(__file__).resolve().parent.parent))
        self.assertEqual(
            ['test_receipt_writer.py', 'test_receipt_verifier.py',
             'test_receipt_chain.py', 'test_ci_workflow_policy_receipts.py',
             'test_receipt_tasklink.py', 'test_receipt_postrun.py'], executed)


class RuntimeProjectionPathMappingTests(unittest.TestCase):
    """Z7 coverage-mapping bootstrap: the PR #1096 projection source resolves."""

    def check_ids(self, result):
        return {check['id'] for check in result['required_validation_checks']}

    def test_exact_projection_source_resolves_under_source_contracts(self):
        path = 'internal/application/runtime_native_binding_projection.go'
        result = planner.plan(diff([{'status': 'modified', 'path': path}]), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['unresolved_coverage'])
        self.assertFalse(result['broad_required'])
        self.assertEqual({'source-contracts'}, self.check_ids(result))
        self.assertTrue(self.check_ids(result) <= set(runner_validations()))

    def test_unknown_application_source_stays_unresolved(self):
        result = planner.plan(
            diff([{'status': 'added', 'path': 'internal/application/unknown_impl.go'}]),
            REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertIn('application lifecycle/provider resolution impact needs targeted qualification',
                      ' '.join(result['unresolved_coverage']))


def runner_validations():
    import importlib.util
    spec_v = importlib.util.spec_from_file_location('check_runner', ROOT / 'pr-check-runner.py')
    mod = importlib.util.module_from_spec(spec_v)
    spec_v.loader.exec_module(mod)
    return mod.VALIDATIONS


if __name__ == '__main__':
    unittest.main()
