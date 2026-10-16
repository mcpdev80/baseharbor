"""Trusted diff-to-check planner contract and fail-closed regression fixtures."""
import importlib.util
import json
import subprocess
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
         'journey/docker', 'journey/podman', 'ha/docker/data']
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
            with patch.object(planner, 'CORE_752_CI_HEAD', reviewed), \\
                 patch.object(planner, 'CORE_752_CI_MERGE_BASE', shared), \\
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


if __name__ == '__main__':
    unittest.main()
