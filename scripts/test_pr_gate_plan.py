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
         'atomic/podman/lifecycle', 'journey/docker', 'journey/podman', 'ha/docker/data',
          'atomic/docker/observability', 'atomic/podman/observability',
          'atomic/docker/control-plane-recovery', 'atomic/podman/control-plane-recovery',
          'atomic/docker/backup-restore', 'atomic/podman/backup-restore']
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
        # A changed workload integration test is now covered by both native
        # lifecycle gates independently of this historical reviewed bundle.
        with patch.object(planner, 'core752_reviewed_merge_chain', return_value=True):
            native = planner.plan(diff(paths('cmd/baha/app_workload_integration_test.go')), REQ, MAP)
        self.assertEqual('resolved', native['coverage_status'])
        self.assertTrue({'atomic/docker/lifecycle', 'atomic/podman/lifecycle'}
                        .issubset(set(native['selected_gates'])))
        variants = [reviewed + paths('cmd/baha/mcp_tools.go'),
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


class Pr1272RegistrationMixedExactQualificationTests(unittest.TestCase):
    """PR #1272 registration-mixed candidate: exact three-file reviewed source.

    The cmd/baha authoring fix plus its regression test and CHANGELOG note must
    resolve only for the exact reviewed HEAD with a clean trusted-target merge and
    must require both native lifecycle gates; any other head, path set or status,
    or missing lifecycle gate stays unresolved.
    """

    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.repository = Path(temporary.name)
        self.git('init', '-q')
        self.git('config', 'user.name', 'pr1272 fixture')
        self.git('config', 'user.email', 'fixture@example.test')
        for name in planner.PR1272_REGISTRATION_MIXED_FILES:
            path = self.repository / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text('baseline\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'trusted base')
        self.git('branch', 'trusted')
        self.git('checkout', '-qb', 'source')
        for name in planner.PR1272_REGISTRATION_MIXED_FILES:
            (self.repository / name).write_text('reviewed source\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'reviewed three-file registration-mixed source')
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
        with patch.object(planner, 'PR1272_REGISTRATION_MIXED_HEAD', self.reviewed), \
             patch.object(planner, 'core752_reviewed_merge_chain', side_effect=proof):
            return planner.plan(actual, requirements, MAP)

    def test_exact_source_and_clean_trusted_merge_select_both_lifecycle_gates(self):
        for head in (self.reviewed, self.head):
            with self.subTest(head=head):
                actual = self.resolved_diff(head)
                self.assertEqual(set(planner.PR1272_REGISTRATION_MIXED_FILES),
                                 {change['path'] for change in actual['changes']})
                self.assertEqual('resolved', self.qualify(actual)['coverage_status'])
                self.assertEqual(planner.PR1272_REGISTRATION_MIXED_GATES,
                                 set(self.qualify(actual)['selected_gates']))
        self.assertEqual(self.target, self.resolved_diff(self.head)['merge_base_sha'])

    def test_wrong_head_stays_unresolved(self):
        # Same three files but a different, unreviewed merge into the target.
        other = self.git('commit-tree', self.git('merge-tree', '--write-tree',
                                                 self.target, self.reviewed).splitlines()[0],
                         '-p', self.target, '-p', self.reviewed, '-m', 'unreviewed followup')
        actual = self.resolved_diff(other)
        self.assertEqual('unresolved', self.qualify(actual)['coverage_status'])

    def test_extra_renamed_deleted_paths_and_status_mismatch_stay_unresolved(self):
        actual = self.resolved_diff(self.head)
        added = json.loads(json.dumps(actual))
        added['changes'].append({'path': 'internal/application/extra.go', 'status': 'modified'})
        variants = [added]
        for change in ({'path': 'internal/application/manifest.go', 'status': 'deleted'},
                       {'path': 'internal/application/manifest.go', 'status': 'added'},
                       {'old_path': 'internal/application/manifest.go',
                        'new_path': 'internal/application/manifest_renamed.go', 'status': 'renamed'}):
            bad = json.loads(json.dumps(actual))
            index = next(i for i, row in enumerate(bad['changes'])
                         if row.get('path') == 'internal/application/manifest.go')
            bad['changes'][index] = change
            variants.append(bad)
        for bad in variants:
            with self.subTest(changes=bad['changes']):
                self.assertEqual('unresolved', self.qualify(bad)['coverage_status'])

    def test_missing_or_wrong_lifecycle_gate_fails_closed(self):
        actual = self.resolved_diff(self.head)
        for missing in ('atomic/docker/lifecycle', 'atomic/podman/lifecycle'):
            with self.subTest(missing=missing):
                catalog = json.loads(json.dumps(REQ))
                catalog['gates'] = [gate for gate in catalog['gates']
                                    if gate['id'] != missing]
                self.assertEqual('unresolved', self.qualify(actual, catalog)['coverage_status'])






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

    def test_capability_implementation_reviewed_files_resolve(self):
        # gatemap-r3 / #1229 (core-390 service-binding enforcement): the reviewed
        # lifecycle/service_binding implementation + its paired test resolve via
        # their package's own capability-package-tests check plus the existing
        # static public-contracts gate (#1260 pattern: reference existing gates,
        # NO new gate). The reviewed files bind to EXISTING gates only.
        for name in ['internal/capability/lifecycle.go',
                     'internal/capability/service_binding.go',
                     'internal/capability/service_binding_test.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertFalse(result['broad_required'])
                self.assertEqual(['integration/static/public-contracts'],
                                 result['selected_gates'])
                ids = {c['id'] for c in result['required_validation_checks']}
                self.assertIn('capability-package-tests', ids)
                self.assertIn('source-contracts', ids)

    def test_unreviewed_capability_implementation_stays_fail_closed(self):
        # Fail-closed regression: any OTHER internal/capability/* production file
        # that is not the reviewed core-390 set stays unresolved, exactly like
        # the registry/provider_placement surface.
        for name in ['internal/capability/registry.go',
                     'internal/capability/provider_placement.go',
                     'internal/capability/manifest.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])

    def test_testdata_adoption_parity_fixtures_resolve(self):
        # gatemap-r3 / #1222: static adoption fixtures (README/compose/k8s/quadlet)
        # bind to the same focused inspect checks that already run the
        # repositoryinspect parity test (#1259 observability-binding pattern) —
        # no new gate, no widening.
        for name in ['testdata/adoption-parity/README.md',
                     'testdata/adoption-parity/compose/compose.yaml',
                     'testdata/adoption-parity/kubernetes/app.yaml',
                     'testdata/adoption-parity/quadlet/storage.container']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertFalse(result['broad_required'])
                ids = {c['id'] for c in result['required_validation_checks']}
                self.assertEqual({'inspect-gofmt', 'inspect-package-tests',
                                  'inspect-vet', 'inspect-consumer-tests',
                                  'inspect-consumer-build'}, ids)

    def test_other_testdata_stays_unresolved(self):
        # Fail-closed regression: a testdata file OUTSIDE adoption-parity still
        # falls to the catch-all unresolved branch (only the adopted prefix is
        # resolved).
        result = planner.plan(diff(paths('testdata/something/else.txt')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])

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

    def test_runtimebroker_package_binds_existing_lifecycle_gates_resolved(self):
        # #1265 / broker-ready4 PR B: the runtime-broker container realization
        # (compose_render.go readiness grace, broker/retirement lifecycle) runs
        # on BOTH runners and must resolve to the EXISTING atomic lifecycle
        # gates — no new gate, no widening.  Without this binding the files fell
        # to the catch-all and blocked PR Selected checks with 'unknown impact'.
        for name in ['internal/runtimebroker/broker.go',
                     'internal/runtimebroker/compose_render.go',
                     'internal/runtimebroker/retirement.go',
                     'internal/runtimebroker/broker_test.go',
                     'internal/runtimebroker/retirement_test.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertFalse(result['broad_required'])
                expected = [] if name.endswith('_test.go') else ['atomic/docker/lifecycle', 'atomic/podman/lifecycle']
                self.assertEqual(expected, sorted(result['selected_gates']))
                ids = {c['id'] for c in result['required_validation_checks']}
                self.assertIn('broker-package-tests', ids)
                if not name.endswith('_test.go'):
                    self.assertTrue({'cli-vet', 'cli-build'} <= ids)

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
        for name in ['internal/application/footprint.go',
                     'internal/application/provider_union.go',
                     'internal/capability/registry.go',
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
                self.assertEqual({'ci-policy', 'pr-planner-tests', 'ci-executor-tests',
                                  'receipt-execution-tests'},
                                 {c['id'] for c in result['required_validation_checks']})

    def test_unknown_never_auto_selects_full_release_catalog(self):
        for name in ['Dockerfile', 'internal/operatorauth/policy.go',
                     'config/deleted.yaml', 'spec/generated.pb.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                self.assertNotEqual(set(GATES), set(result['selected_gates']))

    def test_rename_and_delete_paths_are_not_lost(self):
        # go.mod now resolves via the Go build/test checks (#1160); the renamed
        # source is not lost and a deleted doc routes to the docs checks. The
        # go.mod no longer appears as an 'unknown impact' unresolved entry.
        result = planner.plan(diff([
            {'status': 'renamed', 'old_path': 'docs/a.md', 'new_path': 'go.mod'},
            {'status': 'deleted', 'path': 'docs/deleted.md'}]), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['unresolved_coverage'])
        self.assertEqual([], result['selected_gates'])
        checks = {c['id'] for c in result['required_validation_checks']}
        self.assertIn('cli-build', checks)
        self.assertIn('source-contracts', checks)
        self.assertNotIn('go.mod', str(result['unresolved_coverage']))

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



    def test_actual_repository_release_catalog(self):
        requirements = json.loads((ROOT / 'release-requirements.json').read_text())
        self.assertEqual(65, len([g for g in requirements['gates']
                                  if 'v0.4.26' in g['required_for']]))
        docs = planner.plan(diff(paths('docs/how-to/guide.md')), requirements, MAP)
        self.assertEqual('resolved', docs['coverage_status'])
        self.assertEqual([], docs['selected_gates'])
        self.assertEqual(4, len(docs['required_validation_checks']))
        unknown = planner.plan(diff(paths('config/deleted.yaml')), requirements, MAP)
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

class PodmanHealthPackageBindingTests(unittest.TestCase):
    def test_direct_package_paths_require_fixed_tests_and_lifecycles(self):
        for name in ('internal/health/podman_doctor.go',
                     'internal/providers/runtime/podman/doctor.go',
                     'internal/providers/runtime/podman/doctor_test.go',
                     'internal/providers/runtime/podman/new.go'):
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertIn('runtime-provider-package-tests',
                              {c['id'] for c in result['required_validation_checks']})
                self.assertEqual({'atomic/docker/lifecycle', 'atomic/podman/lifecycle'},
                                 set(result['selected_gates']))

    def test_unreviewed_package_scope_and_type_change_remain_blocked(self):
        for change in ([{'status': 'modified', 'path': 'internal/health/doctor.go'}],
                       [{'status': 'modified', 'path': 'internal/providers/runtime/podman/sub/doctor.go'}],
                       [{'status': 'type_changed', 'path': 'internal/health/podman_doctor.go'}],
                       [{'status': 'renamed', 'old_path': 'internal/providers/runtime/podman/doctor.go',
                         'new_path': 'internal/unknown/doctor.go'}]):
            with self.subTest(change=change):
                self.assertEqual('unresolved', planner.plan(diff(change), REQ, MAP)['coverage_status'])
class NativeIntegrationSourceTests(unittest.TestCase):
    def test_native_sources_select_real_lifecycle_gates(self):
        for name in ('cmd/baha/app_repository_integration_test.go',
                     'cmd/baha/app_workload_integration_test.go',
                     'internal/application/runtime_multi_integration_test.go'):
            with self.subTest(name=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertEqual({'atomic/docker/lifecycle', 'atomic/podman/lifecycle'},
                                 set(result['selected_gates']))
                deleted = planner.plan(diff([{'status': 'deleted', 'path': name}]), REQ, MAP)
                self.assertEqual('unresolved', deleted['coverage_status'])


class LogsRenderingBindingTests(unittest.TestCase):
    def test_render_requires_package_and_native_observability(self):
        result = planner.plan(diff(paths('internal/logs/render.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertIn('logs-package-tests', {c['id'] for c in result['required_validation_checks']})
        self.assertEqual({'atomic/docker/observability', 'atomic/podman/observability'},
                         set(result['selected_gates']))

    def test_provider_journal_test_runs_existing_package_and_native_checks(self):
        path = 'internal/logs/journal_provider_render_test.go'
        for status in ('added', 'modified'):
            with self.subTest(status=status):
                result = planner.plan(diff([{'status': status, 'path': path}]), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertIn('logs-package-tests', {c['id'] for c in result['required_validation_checks']})
                self.assertEqual({'atomic/docker/observability', 'atomic/podman/observability'},
                                 set(result['selected_gates']))
        result = planner.plan(diff([{'status': 'deleted', 'path': path}]), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])

    def test_other_logs_files_remain_unresolved(self):
        for path in ('internal/logs/sub/render.go', 'internal/logs/other.go',
                     'internal/logs/sub/journal_provider_render_test.go'):
            with self.subTest(path=path):
                result = planner.plan(diff(paths(path)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])

    def test_executor_rejects_failure_and_empty_test_run(self):
        from types import SimpleNamespace
        spec = importlib.util.spec_from_file_location('logs_runner', ROOT / 'pr-check-runner.py')
        runner = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(runner)
        for result in (SimpleNamespace(returncode=1, stdout='', stderr='failed'),
                       SimpleNamespace(returncode=0, stdout='', stderr='[no test files]')):
            with self.subTest(result=result), patch.object(runner.subprocess, 'run', return_value=result):
                with self.assertRaises(RuntimeError):
                    runner.execute_validation('logs-package-tests', '.')
        with patch.object(runner.subprocess, 'run', return_value=SimpleNamespace(
                returncode=0, stderr='', stdout='\n'.join(json.dumps(row) for row in [
                    {'Action': 'run', 'Package': 'example/internal/logs', 'Test': 'TestRender'},
                    {'Action': 'pass', 'Package': 'example/internal/logs', 'Test': 'TestRender'},
                    {'Action': 'pass', 'Package': 'example/internal/logs'}]) + '\n')):
            self.assertEqual(['./internal/logs'], runner.execute_validation('logs-package-tests', '.'))


class CliGofmtTestBindingTests(unittest.TestCase):
    """gatemap-r6: cmd/baha/*_test.go bound to EXISTING cli-gofmt gate.

    Pure cmd/baha test-only source exercises existing CLI behaviour and must
    resolve to the existing cli-gofmt formatting gate that the CI matrix already
    runs for every cmd/baha change (WIZARD_BASE_CHECKS['cli-gofmt'],
    .github/workflows/ci.yml cli-gofmt handler). No new gate, no widening.
    """

    def test_generic_cmd_baha_test_file_selects_cli_gofmt(self):
        for name in ['cmd/baha/main_test.go',
                     'cmd/baha/app_show_test.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertFalse(result['broad_required'])
                self.assertEqual([], result['selected_gates'])
                ids = {c['id'] for c in result['required_validation_checks']}
                self.assertIn('cli-gofmt', ids)
                self.assertIn('cli-vet', ids)
                self.assertIn('cli-build', ids)
                self.assertIn('source-contracts', ids)

    def test_generic_cmd_baha_test_file_not_bundled_with_unreviewed(self):
        # fail-closed: resolved test file + an unreviewed production file keeps
        # the whole PR unresolved.
        result = planner.plan(diff(paths('cmd/baha/main_test.go',
                                          'cmd/baha/app_apply.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])

    def test_mcp_and_registration_source_stays_unresolved(self):
        # The cmd/baha/*_test.go binding must NOT sweep in mcp_ / registration
        # mixed source — those stay on their own unresolved clauses.
        for name in ['cmd/baha/mcp_workspace_mutation_test.go',
                     'cmd/baha/mcp_preconditions_test.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])


class RuntimeDockerEngineMappingTests(unittest.TestCase):
    def test_docker_engine_path_classifies_to_lifecycle_gate(self):
        # core-1240 permanent lane: changing ONLY internal/runtime/docker_engine.go
        # must resolve with the real atomic docker lifecycle gate.  This is the exact
        # regression that was broken by the unknown-impact fallback — the path
        # previously fell to `area='unresolved'` at line ~1526 and produced
        # `unknown impact for internal/runtime/docker_engine.go`, which kept
        # every PR that touched docker_engine ROT despite correct code.
        result = planner.plan(diff(paths('internal/runtime/docker_engine.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'], result['selected_gates'])
        self.assertFalse(result['unresolved_coverage'])
        self.assertFalse(result['broad_required'])
        checks = {c['id'] for c in result['required_validation_checks']}
        self.assertIn('source-contracts', checks)
        self.assertIn('cli-build', checks)
        self.assertNotIn('unknown impact', str(result['unresolved_coverage']))

    def test_openbao_runtime_prepare_selects_same_lifecycle_gate(self):
        # openbao_runtime_prepare.go provisions TLS and PostgreSQL storage for the
        # OpenBao stack — same mutation surface as the CORE_917/CORE_918 lifecycle
        # files.  Verify it maps to the same gate.
        result = planner.plan(diff(paths('internal/runtime/openbao_runtime_prepare.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'], result['selected_gates'])
        self.assertFalse(result['unresolved_coverage'])

    def test_docker_engine_test_file_executes_its_package(self):
        result = planner.plan(diff(paths('internal/runtime/docker_engine_test.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['selected_gates'])
        self.assertIn('runtime-provider-package-tests',
                      {row['id'] for row in result['required_validation_checks']})

    def test_docker_engine_bundled_with_unreviewed_stays_unresolved(self):
        # Even if docker_engine is resolved, a second unreviewed file must
        # keep the whole PR unresolved — fail-closed.
        result = planner.plan(diff(paths('internal/runtime/docker_engine.go',
                                          'internal/application/footprint.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])

    def test_resolver_files_remain_separate_and_unresolved(self):
        # internal/runtime/resolver/*.go must NOT match the runtime-docker-engine
        # classification — the resolver branch must stay unresolved.
        result = planner.plan(diff(paths('internal/runtime/resolver/extra.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])


class CliEngineMappingTests(unittest.TestCase):
    def test_cli_docker_engine_selects_both_lifecycle_gates(self):
        # round2 A: cmd/baha/docker_engine.go is the CLI endpoint that calls the
        # runtime and binds Docker/Podman engine observations to the Compose backend.
        # Changing ONLY this file must resolve with BOTH atomic lifecycle gates —
        # the engine code affects both runners (Docker and Podman/Quadlet).
        result = planner.plan(diff(paths('cmd/baha/docker_engine.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'],
                         result['selected_gates'])
        self.assertFalse(result['unresolved_coverage'])
        self.assertFalse(result['broad_required'])
        checks = {c['id'] for c in result['required_validation_checks']}
        self.assertIn('source-contracts', checks)
        self.assertIn('cli-build', checks)
        self.assertIn('cli-vet', checks)

    def test_cli_docker_engine_podman_test_runs_actual_unit_tests(self):
        result = planner.plan(diff(paths('cmd/baha/docker_engine_podman_test.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['selected_gates'])
        self.assertIn('cli-package-tests', {row['id'] for row in result['required_validation_checks']})
        self.assertFalse(result['unresolved_coverage'])

    def test_cli_docker_engine_bundled_with_unreviewed_stays_unresolved(self):
        # round2 fail-closed: even if docker_engine.go is resolved, a second
        # unreviewed file must keep the whole PR unresolved.
        result = planner.plan(diff(paths('cmd/baha/docker_engine.go',
                                          'internal/application/footprint.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])

    def test_cli_engine_other_files_not_glob_resolved(self):
        # Only *_engine.go files are matched — other cmd/baha/*.go that are NOT
        # in the wizard dict (like main.go) remain unresolved.
        # The wizard dict maps specific known files; anything else falls to the
        # catch-all unresolved branch.
        result = planner.plan(diff(paths('cmd/baha/main.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])

    def test_cli_engine_missing_lifecycle_gate_fails_closed(self):
        # If the catalog is missing one lifecycle gate (e.g. podman), the result
        # must stay unresolved — fail-closed on incomplete gate availability.
        incomplete = {'releases': ['v0.4.26'],
                       'gates': [{'id': 'atomic/docker/lifecycle',
                                  'required_for': ['v0.4.26']}]}
        result = planner.plan(diff(paths('cmd/baha/docker_engine.go')), incomplete, MAP)
        self.assertEqual('unresolved', result['coverage_status'])


class RestartIntegrationMappingTests(unittest.TestCase):
    def test_runtime_restart_integration_test_maps_both_recovery_gates(self):
        # gatemap-r2 Gap 1 (blocks #1245).  cmd/baha/runtime_restart_integration_test.go
        # is the control-plane restart proof the Lifecycle-Gates drive (targeted
        # integration.yml dispatches its TestExistingControlPlaneRestartRequiresAndUses
        # RecoveryFile / TestSingleControlPlaneRestartRotationAndCleanup to the
        # control-plane / control-plane-single cases on BOTH docker and podman).
        # It must resolve to BOTH atomic lifecycle gates, exactly like the other
        # cmd/baha runtime-mutation files — not the cli-unresolved-runtime-impact
        # catch-all that previously blocked PR Selected checks.
        result = planner.plan(diff(paths('cmd/baha/runtime_restart_integration_test.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(['atomic/docker/control-plane-recovery', 'atomic/podman/control-plane-recovery'],
                         result['selected_gates'])
        self.assertFalse(result['unresolved_coverage'])
        self.assertFalse(result['broad_required'])
        checks = {c['id'] for c in result['required_validation_checks']}
        self.assertIn('source-contracts', checks)
        self.assertIn('cli-build', checks)
        self.assertIn('cli-vet', checks)

    def test_restart_integration_bundled_with_unreviewed_stays_unresolved(self):
        # fail-closed: resolved restart test + an unreviewed second file keeps
        # the whole PR unresolved.
        result = planner.plan(diff(paths('cmd/baha/runtime_restart_integration_test.go',
                                          'cmd/baha/main.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])


class SharedBackendResolutionMappingTests(unittest.TestCase):
    def test_shared_backends_maps_to_existing_lifecycle_gate(self):
        # gatemap-r2 Gap 2 (blocks #1255).  internal/application/shared_backends.go
        # is genuine Provider-Resolution-Impact; its qualification gate is the
        # hosted runtime-validation/ha/shared-postgres targeted run.  The
        # aggregator only recognises the eight catalog tokens and the mission
        # forbids inventing an `integration/shared-postgres` token, so this file
        # maps to the existing atomic/podman/lifecycle lifecycle gate — a real
        # catalog token covering this shared Postgres service-identity/credential
        # path — NOT the application-implementation-unresolved catch-all that
        # previously blocked PR Selected checks.
        result = planner.plan(diff(paths('internal/application/shared_backends.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(['atomic/podman/lifecycle'], result['selected_gates'])
        self.assertFalse(result['unresolved_coverage'])
        self.assertFalse(result['broad_required'])
        checks = {c['id'] for c in result['required_validation_checks']}
        self.assertIn('source-contracts', checks)

    def test_shared_backend_dependency_sister_maps_to_lifecycle_gate(self):
        # The sister resolution file named in the mission (shared_backend_dependency.go)
        # must resolve via the same classification.
        result = planner.plan(diff(paths('internal/application/shared_backend_dependency.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(['atomic/podman/lifecycle'], result['selected_gates'])

    def test_shared_backend_state_sister_maps_to_lifecycle_gate(self):
        result = planner.plan(diff(paths('internal/application/shared_backends_state.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(['atomic/podman/lifecycle'], result['selected_gates'])

    def test_shared_backends_bundled_with_unreviewed_stays_unresolved(self):
        # fail-closed: resolved shared-backend file + an unreviewed second file
        # keeps the whole PR unresolved.
        result = planner.plan(diff(paths('internal/application/shared_backends.go',
                                          'internal/application/footprint.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])

    def test_unrelated_application_file_stays_unresolved(self):
        # A non-manifest application file must NOT pick up the r4 manifest
        # classification; it stays in application-implementation-unresolved.
        result = planner.plan(diff(paths('internal/application/footprint.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertIn('application lifecycle', result['unresolved_coverage'][0])
        self.assertNotIn('atomic/podman/lifecycle', result['selected_gates'])
        self.assertNotIn('atomic/docker/lifecycle', result['selected_gates'])

    def test_gatemap_r4_manifest_paths_bind_lifecycle_gates(self):
        # gatemap-r4: the three reviewed manifest-parser paths bind to the two
        # EXISTING native lifecycle gates (atomic/docker/lifecycle,
        # atomic/podman/lifecycle) -> coverage resolved, selected_gates exactly
        # those two catalog tokens (no new gate).
        for name in ['internal/application/manifest.go',
                     'internal/application/manifest_parse.go',
                     'internal/application/v0425_contract_test.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertEqual([], result['unresolved_coverage'])
                self.assertEqual({'atomic/docker/lifecycle',
                                  'atomic/podman/lifecycle'},
                                 set(result['selected_gates']))
                commands = {c['command'] for c in result['required_validation_checks']}
                self.assertIn('go test ./internal/application/... ./contracts/...',
                              commands)

    def test_gatemap_r4_other_application_file_stays_unresolved(self):
        # Next application file that is NOT the reviewed manifest parser stays
        # fail-closed unresolved; the three-file binding does NOT widen.
        for name in ['internal/application/footprint.go',
                     'internal/application/provider_union.go']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertTrue(result['unresolved_coverage'])
                self.assertNotEqual({'atomic/docker/lifecycle',
                                     'atomic/podman/lifecycle'},
                                    set(result['selected_gates']))

    def test_shared_backend_missing_lifecycle_gate_fails_closed(self):
        # Fail-closed on incomplete catalog: no atomic/podman/lifecycle => unresolved.
        incomplete = {'releases': ['v0.4.26'],
                        'gates': [{'id': 'integration/static/public-contracts',
                                   'required_for': ['v0.4.26']}]}
        result = planner.plan(diff(paths('internal/application/shared_backends.go')), incomplete, MAP)
        self.assertEqual('unresolved', result['coverage_status'])

    def test_app_observability_acceptance_maps_both_observability_gates(self):
        # p1242diag gate-map follow-up (blocks #1242).  cmd/baha/app_observability
        # _acceptance_test.go is the full-stack observability acceptance proof the
        # observability acceptance gates drive (observability-acceptance.yml
        # dispatches TestObservabilityFullStackAcceptanceInCI to the docker and
        # podman blocks).  It must resolve to BOTH observability gates, exactly
        # like the other cmd/baha runtime-acceptance files — not the
        # cli-unresolved-runtime-impact catch-all that previously blocked PR
        # Selected checks.
        result = planner.plan(diff(paths('cmd/baha/app_observability_acceptance_test.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(['atomic/docker/observability', 'atomic/podman/observability'],
                         result['selected_gates'])
        self.assertFalse(result['unresolved_coverage'])
        self.assertFalse(result['broad_required'])
        checks = {c['id'] for c in result['required_validation_checks']}
        self.assertIn('source-contracts', checks)
        self.assertIn('cli-build', checks)
        self.assertIn('cli-vet', checks)

    def test_app_observability_acceptance_bundled_with_unreviewed_stays_unresolved(self):
        # fail-closed: resolved observability test + an unreviewed second file
        # keeps the whole PR unresolved.
        result = planner.plan(diff(paths('cmd/baha/app_observability_acceptance_test.go',
                                          'cmd/baha/main.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])

    def test_app_observability_acceptance_missing_gate_fails_closed(self):
        # Fail-closed on incomplete catalog: only atomic/docker/observability => unresolved.
        incomplete = {'releases': ['v0.4.26'],
                        'gates': [{'id': 'atomic/docker/observability',
                                   'required_for': ['v0.4.26']}]}
        result = planner.plan(diff(paths('cmd/baha/app_observability_acceptance_test.go')), incomplete, MAP)
        self.assertEqual('unresolved', result['coverage_status'])

    def test_unrelated_cli_test_file_stays_unresolved(self):
        # A non-observability cmd/baha/*.go test must NOT pick up the new
        # classification; it stays in cli-unresolved-runtime-impact.
        result = planner.plan(diff(paths('cmd/baha/main.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertIn('CLI runtime impact', result['unresolved_coverage'][0])
        self.assertNotIn('atomic/docker/observability', result['selected_gates'])
    def test_dependabot_only_go_mod_go_sum_resolves_with_go_build_test_checks(self):
        # deps-gatemap / #1160: a Dependabot-only go.mod/go.sum diff must NOT fall
        # into the 'unknown impact' catch-all. It resolves and requires the Go
        # build/test validation checks the CI matrix runs for a Go change.
        result = planner.plan(diff(paths('go.mod', 'go.sum')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertFalse(result['unresolved_coverage'])
        self.assertFalse(result['broad_required'])
        self.assertEqual([], result['selected_gates'])
        checks = {c['id'] for c in result['required_validation_checks']}
        self.assertIn('source-contracts', checks)
        self.assertIn('cli-build', checks)
        self.assertIn('cli-vet', checks)

    def test_go_mod_go_sum_bundled_with_unreviewed_stays_unresolved(self):
        # fail-closed: resolved go.mod/go.sum + an unreviewed product file keeps
        # the whole PR unresolved.
        result = planner.plan(diff(paths('go.mod', 'go.sum',
                                        'internal/application/thing.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])

    def test_go_mod_alone_still_resolves(self):
        # A single go.mod modify resolves the same way (dependabot bump).
        result = planner.plan(diff(paths('go.mod')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['unresolved_coverage'])

    def test_go_sum_alone_still_resolves(self):
        result = planner.plan(diff(paths('go.sum')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['unresolved_coverage'])

    def test_dependabot_go_mod_go_sum_missing_gate_fails_closed(self):
        # Fail-closed on an incomplete catalog is not required here: Go build/test
        # checks are validations only, but the module file must still route to the
        # go-module-dependency area rather than a broad unresolved catch-all.
        result = planner.plan(diff(paths('go.mod', 'go.sum')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        reasons = ' '.join(result['reasons'])
        self.assertIn('go-module-dependency', reasons)


class ReceiptControlPlaneMappingTests(unittest.TestCase):
    def test_receipt_control_plane_script_maps_to_ci_class(self):
        for path in ['scripts/receipt_writer.py']:
            with self.subTest(path=path):
                result = planner.plan(diff(paths(path)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                checks = {c['id'] for c in result['required_validation_checks']}
                self.assertEqual({'pr-planner-tests', 'ci-policy',
                                  'ci-executor-tests',
                                  'receipt-execution-tests'}, checks)

    def test_receipt_control_plane_test_script_maps_to_ci_class(self):
        for path in ['scripts/test_receipt_writer.py', 'scripts/test_receipt_postrun.py']:
            with self.subTest(path=path):
                result = planner.plan(diff(paths(path)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                checks = {c['id'] for c in result['required_validation_checks']}
                self.assertEqual({'pr-planner-tests', 'ci-policy',
                                  'ci-executor-tests',
                                  'receipt-execution-tests'}, checks)

    def test_receipt_control_plane_workflow_maps_ci_and_actionlint(self):
        for path in ['.github/workflows/receipt-post-run.yml']:
            with self.subTest(path=path):
                result = planner.plan(diff(paths(path)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['unresolved_coverage'])
                self.assertEqual([], result['selected_gates'])
                checks = {c['id'] for c in result['required_validation_checks']}
                self.assertEqual({'ci-policy', 'ci-actionlint', 'pr-planner-tests',
                                  'ci-executor-tests',
                                  'receipt-execution-tests'}, checks)

    def test_ci_workflow_policy_script_maps_to_ci_class(self):
        path = 'scripts/ci-workflow-policy.sh'
        result = planner.plan(diff(paths(path)), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['unresolved_coverage'])
        self.assertEqual([], result['selected_gates'])
        checks = {c['id'] for c in result['required_validation_checks']}
        self.assertEqual({'pr-planner-tests', 'ci-policy',
                          'ci-executor-tests',
                          'receipt-execution-tests'}, checks)


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
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self._root = Path(temporary.name)
        self._actual = diff(paths('scripts/pr-gate-plan.py'))

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
        # The expected repository and target select the normal checks.
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
        # Wrong repository is rejected before any check selection.
        code, out, msg = self._run_main({
            'GITHUB_REPOSITORY': 'other-org/baseharbor',
            'GITHUB_BASE_REF': 'develop',
        })
        self.assertIsNotNone(code)
        self.assertNotIn('"coverage_status"', out)
        self.assertIn('authenticated runner context', msg)











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
        def stream(skip=None):
            events = []
            for name in names:
                events.extend([{'Action': 'run', 'Package': 'example/internal/openbao', 'Test': name},
                               {'Action': 'skip' if name == skip else 'pass',
                                'Package': 'example/internal/openbao', 'Test': name}])
            events.append({'Action': 'pass', 'Package': 'example/internal/openbao'})
            return '\n'.join(json.dumps(row) for row in events) + '\n'
        execution = stream()
        for listed, output, code, success in (
                (discovery, execution, 0, True), ('', execution, 0, False),
                ('\n'.join(names[:-1]), execution, 0, False),
                (discovery, '', 0, False), (discovery, execution, 1, False),
                (discovery, stream(skip=names[0]), 0, False)):
            with self.subTest(listed=listed, code=code, success=success):
                results = [SimpleNamespace(returncode=0, stdout=listed, stderr=''),
                           SimpleNamespace(returncode=code, stdout=output, stderr='')]
                with patch.object(runner.subprocess, 'run', side_effect=results) as process:
                    if success:
                        self.assertEqual(list(names), runner.execute_validation('openbao-package-tests', '.'))
                        self.assertIn('./internal/openbao', process.call_args.args[0])
                        self.assertNotIn('-run', process.call_args.args[0])
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
        for helper in ('pr-check-runner.py', 'ecosystem_release_pins.py', 'consumer_source_pins.py'):
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
        for helper in ('pr-check-runner.py', 'ecosystem_release_pins.py', 'consumer_source_pins.py'):
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

    def test_unknown_product_path_stays_unresolved(self):
        for path in ('internal/unmapped/publisher.go', 'spec/unmapped.json'):
            with self.subTest(path=path):
                result = planner.plan(diff([{'status': 'added', 'path': path}]), REQ, MAP)
                self.assertEqual('unresolved', result['coverage_status'])
                self.assertIn(path, ' '.join(result['unresolved_coverage']))

    def test_native_workflow_changes_select_normal_ci_checks(self):
        path = '.github/workflows/runtime-s3-acceptance.yml'
        result = planner.plan(diff([{'status': 'modified', 'path': path}]), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertTrue({'ci-actionlint', 'ci-policy', 'ci-executor-tests'} <= self.check_ids(result))

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
            ['test_receipt_writer.py', 'test_receipt_postrun.py'], executed)


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


class PodmanDoctorRuntimeImpactMappingTests(unittest.TestCase):
    """gatemap-r5: the three PR #1267 podman-doctor files bind to the existing
    atomic lifecycle gates; a genuinely unrelated new cmd/baha file stays
    unresolved (the catch-all is unchanged)."""

    def test_three_podman_doctor_files_resolve_on_both_lifecycle_gates(self):
        reviewed = [
            {'status': 'modified', 'path': 'cmd/baha/control_plane_operations.go'},
            {'status': 'added', 'path': 'cmd/baha/podman_doctor_checks.go'},
            {'status': 'added', 'path': 'cmd/baha/podman_doctor_checks_test.go'},
        ]
        result = planner.plan(diff(reviewed), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'], result)
        self.assertEqual([], result['unresolved_coverage'], result)
        self.assertFalse(result['broad_required'])
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'],
                         result['selected_gates'])
        required = planner.CODE_CHECKS + planner.CLI_CHECKS
        self.assertEqual({c['id'] for c in result['required_validation_checks']},
                         {c['id'] for c in required})

    def test_unrelated_new_cmd_baha_file_stays_unresolved(self):
        result = planner.plan(
            diff([{'status': 'added', 'path': 'cmd/baha/brand_new_command.go'}]),
            REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertIn('CLI runtime impact needs qualified targeted native coverage',
                      ' '.join(result['unresolved_coverage']))


def runner_validations():
    import importlib.util
    spec_v = importlib.util.spec_from_file_location('check_runner', ROOT / 'pr-check-runner.py')
    mod = importlib.util.module_from_spec(spec_v)
    spec_v.loader.exec_module(mod)
    return mod.VALIDATIONS




class NormalCISelectionTests(unittest.TestCase):
    def test_ci_source_add_modify_delete_rename_run_normal_checks(self):
        for changes in [
            paths('.github/workflows/ci.yml', 'scripts/pr-required-final.py'),
            [{'status': 'deleted', 'path': '.github/workflows/receipt-approval.yml'},
             {'status': 'deleted', 'path': 'scripts/receipt_verifier.py'}],
            [{'status': 'added', 'path': 'scripts/pre-release-check-plan.py'},
             {'status': 'added', 'path': 'scripts/test_pre_release_check_plan.py'}],
            [{'status': 'renamed', 'old_path': 'scripts/consumer_source_pins.py',
              'new_path': 'scripts/consumer_source_pins.py'}],
        ]:
            with self.subTest(changes=changes):
                result = planner.plan(diff(changes), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                ids = {row['id'] for row in result['required_validation_checks']}
                self.assertTrue({'ci-policy', 'ci-executor-tests', 'pr-planner-tests'} <= ids)
                self.assertEqual([], result['selected_gates'])

    def test_ci_source_cannot_hide_unknown_product_impact(self):
        result = planner.plan(diff(paths('scripts/pr-required-final.py',
                                         'internal/unmapped/production.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(any('internal/unmapped/production.go' in reason
                            for reason in result['unresolved_coverage']))

    def test_changed_cli_unit_tests_run_package_tests(self):
        result = planner.plan(diff(paths('cmd/baha/new_regression_test.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertIn('cli-package-tests', {row['id'] for row in result['required_validation_checks']})

    def test_delivery_package_source_and_tests_use_normal_package_executor(self):
        for path in ('internal/delivery/contract.go', 'internal/delivery/contract_test.go'):
            for status in ('added', 'modified', 'deleted'):
                with self.subTest(path=path, status=status):
                    result = planner.plan(diff([{'status': status, 'path': path}]), REQ, MAP)
                    self.assertEqual('resolved', result['coverage_status'])
                    self.assertIn('delivery-package-tests', {row['id'] for row in result['required_validation_checks']})
                    self.assertEqual([], result['selected_gates'])
        unknown = planner.plan(diff(paths('internal/delivery/newpkg/untested.go')), REQ, MAP)
        self.assertEqual('unresolved', unknown['coverage_status'])

    def test_authority_package_routes_to_tests_without_native_matrix(self):
        result = planner.plan(diff(paths('internal/coreinstallation/authority.go',
                                         'internal/stableid/id.go',
                                         'internal/serviceaccess/local_issuer.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertIn('authority-package-tests', {row['id'] for row in result['required_validation_checks']})
        self.assertEqual([], result['selected_gates'])

if __name__ == '__main__':
    unittest.main()


class DeploymentPackageRoutingTests(unittest.TestCase):
    def test_state_changes_run_package_consumers_and_both_lifecycles(self):
        for change in [{'status': kind, 'path': 'internal/deployment/deployment_store.go'}
                       for kind in ('added', 'modified', 'deleted')]:
            with self.subTest(change=change):
                result = planner.plan(diff([change]), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'],
                                 result['selected_gates'])
                self.assertTrue({'deployment-package-tests', 'source-contracts', 'cli-build'} <=
                                {row['id'] for row in result['required_validation_checks']})

    def test_test_only_changes_execute_package_without_native_matrix(self):
        result = planner.plan(diff(paths('internal/deployment/one_writer_guard_test.go',
                                         'internal/deployment/preferences_retention_test.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual([], result['selected_gates'])
        self.assertEqual(['deployment-package-tests'],
                         [row['id'] for row in result['required_validation_checks']])

    def test_rename_cannot_hide_production_or_unknown_nested_package(self):
        result = planner.plan(diff([{'status': 'renamed',
            'old_path': 'internal/deployment/deployment_store.go',
            'new_path': 'internal/deployment/nested/state.go'}]), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'],
                         result['selected_gates'])


class AuthorityPackageCompletenessTests(unittest.TestCase):
    def test_each_known_authority_package_covers_new_direct_source_and_tests(self):
        for package in planner.AUTHORITY_PACKAGES:
            for filename in ('new_source.go', 'new_source_test.go'):
                result = planner.plan(diff(paths(package + '/' + filename)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'], result)
                self.assertTrue({'authority-package-tests', 'source-contracts', 'cli-build'} <=
                                {row['id'] for row in result['required_validation_checks']})
                self.assertEqual([], result['selected_gates'])

    def test_unknown_nested_authority_package_stays_unresolved(self):
        result = planner.plan(diff(paths('internal/serviceaccess/new_package/auth.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])


class RuntimePackageCompletenessTests(unittest.TestCase):
    def test_nested_runtime_tests_are_actually_selected_without_native_matrix(self):
        for path in ('internal/runtime/remoteprojection/quadlet_test.go',
                     'internal/runtime/model/plan_test.go',
                     'internal/runtime/terminal/process_linux_test.go'):
            result = planner.plan(diff(paths(path)), REQ, MAP)
            self.assertEqual('resolved', result['coverage_status'])
            self.assertEqual(['runtime-provider-package-tests'],
                             [row['id'] for row in result['required_validation_checks']])
            self.assertEqual([], result['selected_gates'])

    def test_shared_quadlet_production_requires_both_runtime_checks(self):
        result = planner.plan(diff(paths('internal/runtime/remoteprojection/quadlet.go')), REQ, MAP)
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'],
                         result['selected_gates'])
        self.assertIn('runtime-provider-package-tests',
                      {row['id'] for row in result['required_validation_checks']})


class ConcreteUnitTestExecutionTests(unittest.TestCase):
    def test_broker_unit_changes_execute_the_broker_package(self):
        result = planner.plan(diff(paths('internal/runtimebroker/broker_test.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(['broker-package-tests'], [row['id'] for row in result['required_validation_checks']])
        self.assertEqual([], result['selected_gates'])

    def test_cli_engine_name_regression_executes_instead_of_guided_only(self):
        result = planner.plan(diff(paths('cmd/baha/docker_engine_podman_test.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertIn('cli-package-tests', {row['id'] for row in result['required_validation_checks']})
        self.assertEqual([], result['selected_gates'])


class CurrentProductSeamRoutingTests(unittest.TestCase):
    def test_mixed_cli_tests_run_with_deployment_owner_implementation(self):
        result = planner.plan(diff(paths('cmd/baha/deployment_record.go',
                                         'cmd/baha/app_context_incomplete_test.go',
                                         'internal/deployment/deployment_store.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertTrue({'cli-package-tests', 'deployment-package-tests', 'source-contracts'} <=
                        {row['id'] for row in result['required_validation_checks']})
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'], result['selected_gates'])

    def test_issuer_bridge_runs_authority_openbao_and_cli_consumers(self):
        for name in ('cmd/baha/application_service_issuer.go', 'internal/openbao/service_issuer.go',
                     'internal/openbao/service_pki.go', 'internal/testsupport/serviceissuer/issuer.go'):
            result = planner.plan(diff(paths(name)), REQ, MAP)
            self.assertEqual('resolved', result['coverage_status'])
            self.assertTrue({'cli-package-tests', 'authority-package-tests', 'openbao-package-tests', 'source-contracts'} <=
                            {row['id'] for row in result['required_validation_checks']})

    def test_native_backup_source_runs_its_own_existing_gates(self):
        result = planner.plan(diff(paths('cmd/baha/app_backup_restore_integration_test.go')), REQ, MAP)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertEqual(['atomic/docker/backup-restore', 'atomic/podman/backup-restore'], result['selected_gates'])


class CLISelectedCheckDeduplicationTests(unittest.TestCase):
    def test_mixed_cli_unit_and_source_runs_guided_only_through_package(self):
        changes = paths('cmd/baha/deployment_record.go',
                        'cmd/baha/app_context_incomplete_test.go',
                        'internal/deployment/deployment_store.go')
        result = planner.plan(diff(changes), REQ, MAP)
        ids = [check['id'] for check in result['required_validation_checks']]
        self.assertEqual('resolved', result['coverage_status'])
        self.assertIn('cli-package-tests', ids)
        self.assertNotIn('cli-guided-tests', ids)
        self.assertEqual(1, ids.count('cli-package-tests'))
        self.assertTrue({'cli-vet', 'cli-build', 'source-contracts'} <= set(ids))
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'],
                         result['selected_gates'])

    def test_test_only_change_keeps_one_package_owner_without_native_suite(self):
        result = planner.plan(diff(paths('cmd/baha/new_regression_test.go')), REQ, MAP)
        ids = [check['id'] for check in result['required_validation_checks']]
        self.assertEqual('resolved', result['coverage_status'])
        self.assertIn('cli-package-tests', ids)
        self.assertNotIn('cli-guided-tests', ids)
        self.assertEqual([], result['selected_gates'])

    def test_guided_check_is_retained_when_no_full_package_selected(self):
        result = planner.plan(diff(paths('cmd/baha/docker_engine.go')), REQ, MAP)
        ids = [check['id'] for check in result['required_validation_checks']]
        self.assertEqual('resolved', result['coverage_status'])
        self.assertNotIn('cli-package-tests', ids)
        self.assertIn('cli-guided-tests', ids)
        self.assertTrue({'cli-vet', 'cli-build'} <= set(ids))
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'],
                         result['selected_gates'])

    def test_deduplication_does_not_change_unresolved_coverage_or_gate_scope(self):
        result = planner.plan(
            diff(paths('cmd/baha/new_regression_test.go',
                       'internal/unmapped/production.go')), REQ, MAP)
        self.assertEqual('unresolved', result['coverage_status'])
        self.assertTrue(result['unresolved_coverage'])
        ids = [check['id'] for check in result['required_validation_checks']]
        self.assertIn('cli-package-tests', ids)
        self.assertNotIn('cli-guided-tests', ids)


class IdentityProviderPackageRoutingTests(unittest.TestCase):
    def test_keycloak_test_only_selects_one_package_not_broad_fallback(self):
        for filename in ('internal/identityprovider/keycloak_single_native_ci_test.go',
                         'internal/identityprovider/keycloak_files_test.go'):
            result = planner.plan(diff(paths(filename)), REQ, MAP,
                                  conservative_fallback=True)
            checks = {row['id'] for row in result['required_validation_checks']}
            self.assertEqual('resolved', result['coverage_status'])
            self.assertFalse(result.get('fallback_used', False))
            self.assertEqual([], result['selected_gates'])
            self.assertIn('identity-provider-package-tests', checks)
            self.assertFalse({'go-unit-tests', 'go-build', 'go-vet'} & checks)

    def test_identity_implementation_requires_native_lifecycle_both_runtimes(self):
        result = planner.plan(
            diff(paths('internal/identityprovider/keycloak_core_sql.go')),
            REQ, MAP, conservative_fallback=True,
        )
        self.assertEqual('resolved', result['coverage_status'])
        checks = {row['id'] for row in result['required_validation_checks']}
        self.assertTrue({'identity-provider-package-tests', 'source-contracts',
                         'cli-vet', 'cli-build'} <= checks)
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'],
                         result['selected_gates'])

    def test_unrelated_unknown_code_remains_conservative_fallback(self):
        result = planner.plan(diff(paths('internal/newly_unknown/feature.go')), REQ, MAP,
                              conservative_fallback=True)
        self.assertTrue(result['fallback_used'])
        checks = {row['id'] for row in result['required_validation_checks']}
        self.assertTrue({'go-unit-tests', 'go-build', 'go-vet'} <= checks)
        self.assertNotIn('identity-provider-package-tests', checks)
