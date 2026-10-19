"""Trusted diff-to-check planner contract and fail-closed regression fixtures."""
import importlib.util
import json
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
MAP = json.loads((ROOT / 'pr-gate-map-v1.json').read_text())
GATES = ['integration/static/public-contracts', 'atomic/docker/lifecycle',
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


class MappingTests(unittest.TestCase):
    def test_documentation_requires_validation_but_zero_release_gates(self):
        for name in ['docs/how-to/example.md', 'docs/internal/reference.md',
                     'README.md', 'mkdocs.yml']:
            with self.subTest(path=name):
                result = planner.plan(diff(paths(name)), REQ, MAP)
                self.assertEqual('resolved', result['coverage_status'])
                self.assertFalse(result['broad_required'])
                self.assertEqual([], result['selected_gates'])
                self.assertEqual(4, len(result['required_validation_checks']))
                self.assertFalse(result['unresolved_coverage'])

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
                self.assertEqual({'ci-policy', 'pr-planner-tests', 'ci-executor-verifier-tests'},
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
        self.assertEqual({'pr-planner-tests', 'ci-policy', 'ci-executor-verifier-tests'},
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
                self.assertEqual({'pr-planner-tests','ci-policy','ci-executor-verifier-tests'},
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
            ('EVIDENCE_RETENTION_REVIEWED_HEAD', planner.EVIDENCE_RETENTION_REVIEWED_FILES,
             {c['id'] for c in planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS}),
            ('RUNTIME_RESET_REVIEWED_HEAD', planner.RUNTIME_RESET_REVIEWED_FILES,
             {c['id'] for c in planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS}),
            ('PODMAN_CONSENT_REVIEWED_HEAD', planner.PODMAN_CONSENT_REVIEWED_FILES,
             {c['id'] for c in planner.CI_CHECKS + planner.WORKFLOW_SYNTAX_CHECKS}),
            ('MCP_PRECONDITIONS_REVIEWED_HEAD', planner.MCP_PRECONDITIONS_REVIEWED_FILES,
             {c['id'] for c in planner.WIZARD_BASE_CHECKS} | {planner.CORE_754_TEST_CHECK}),
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

if __name__ == '__main__':
    unittest.main()
