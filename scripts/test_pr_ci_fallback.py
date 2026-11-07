"""Unknown valid source receives real checks; invalid source cannot be exempted."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import yaml

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('planner', ROOT / 'pr-gate-plan.py')
planner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(planner)
CATALOG = json.loads((ROOT / 'release-requirements.json').read_text())
MAPPING = json.loads((ROOT / 'pr-gate-map-v1.json').read_text())


def changes(path):
    return {'target_base_sha': 'a'*40, 'head_sha': 'b'*40, 'merge_base_sha': 'c'*40,
            'integration_sha': 'd'*40, 'broad_required': False, 'reasons': [],
            'changes': [{'status': 'modified', 'path': path}]}


class ConservativeFallbackTests(unittest.TestCase):
    def test_unknown_product_executes_broad_checks_and_both_lifecycles(self):
        result = planner.plan(changes('internal/newarea/feature.go'), CATALOG, MAPPING,
                              conservative_fallback=True)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertTrue(result['fallback_used'])
        self.assertTrue(result['fallback_reasons'])
        self.assertTrue({'go-unit-tests', 'go-vet', 'go-build'} <=
                        {row['id'] for row in result['required_validation_checks']})
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'], result['selected_gates'])

    def test_broad_fallback_subsumes_only_identical_small_go_checks(self):
        # A new CLI test plus an unresolved product implementation requires
        # broad Go checks, but must not rerun the same CLI tests and build.
        mixed = {**changes('internal/unmapped/production.go'),
                 'changes': [
                     {'status': 'added', 'path': 'cmd/baha/new_regression_test.go'},
                     {'status': 'modified', 'path': 'internal/unmapped/production.go'},
                 ]}
        result = planner.plan(mixed, CATALOG, MAPPING, conservative_fallback=True)
        ids = [check['id'] for check in result['required_validation_checks']]
        self.assertEqual('resolved', result['coverage_status'])
        self.assertTrue(result['fallback_used'])
        self.assertTrue({'go-unit-tests', 'go-vet', 'go-build'} <= set(ids))
        self.assertFalse({'cli-package-tests', 'cli-guided-tests',
                          'cli-vet', 'cli-build', 'source-contracts',
                          'capability-package-tests'} & set(ids))
        # Formatting isn't provided by Go test/vet/build.
        self.assertIn('cli-gofmt', ids)
        self.assertEqual(['atomic/docker/lifecycle', 'atomic/podman/lifecycle'],
                         result['selected_gates'])

    def test_broad_fallback_keeps_specialized_runtime_provider_assertions(self):
        mixed = {**changes('internal/unmapped/production.go'),
                 'changes': [
                     {'status': 'modified', 'path': 'internal/runtime/stack_test.go'},
                     {'status': 'modified', 'path': 'internal/unmapped/production.go'},
                 ]}
        result = planner.plan(mixed, CATALOG, MAPPING, conservative_fallback=True)
        ids = {check['id'] for check in result['required_validation_checks']}
        self.assertTrue({'go-unit-tests', 'go-vet', 'go-build'} <= ids)
        # Specific checks may require explicit PASS lines, isolation or env.
        self.assertIn('runtime-provider-package-tests', ids)

    def test_known_docs_remain_targeted_and_unknown_metadata_does_not_boot_runtimes(self):
        known = planner.plan(changes('docs/guide.md'), CATALOG, MAPPING, conservative_fallback=True)
        self.assertNotIn('fallback_used', known)
        unknown = planner.plan(changes('NOTICE'), CATALOG, MAPPING, conservative_fallback=True)
        self.assertEqual('resolved', unknown['coverage_status'])
        self.assertTrue(unknown['fallback_used'])
        self.assertEqual([], unknown['selected_gates'])

    def test_invalid_source_and_missing_catalog_checks_remain_errors(self):
        for edit in ({'broad_required': True}, {'head_sha': 'bad'},
                     {'changes': [{'status': 'modified', 'path': '../escape.go'}]}):
            result = planner.plan({**changes('internal/newarea/a.go'), **edit}, CATALOG,
                                  MAPPING, conservative_fallback=True)
            self.assertEqual('unresolved', result['coverage_status'])
        catalog = copy.deepcopy(CATALOG)
        catalog['gates'] = [g for g in catalog['gates'] if g['id'] != 'atomic/podman/lifecycle']
        result = planner.plan(changes('internal/newarea/a.go'), catalog, MAPPING, conservative_fallback=True)
        self.assertEqual('unresolved', result['coverage_status'])

    def test_production_entrypoint_enables_the_fallback(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'diff.json'
            path.write_text(json.dumps(changes('internal/newarea/a.go')))
            out = subprocess.check_output(['python3', str(ROOT/'pr-gate-plan.py'), '--diff', str(path)],
                env={**os.environ, 'GITHUB_REPOSITORY': 'mcpdev80/baseharbor', 'GITHUB_BASE_REF': 'develop'}, text=True)
            self.assertTrue(json.loads(out)['fallback_used'])

    def test_format_checks_changed_files_without_blocking_on_unchanged_debt(self):
        ci = yaml.safe_load((ROOT.parent/'.github/workflows/ci.yml').read_text())
        steps = ci['jobs']['source_format']['steps']
        self.assertEqual(0, steps[0]['with']['fetch-depth'])
        command = steps[-1]['run']
        with tempfile.TemporaryDirectory() as directory:
            repo = Path(directory)
            def git(*args):
                return subprocess.check_output(['git', '-C', str(repo), *args], text=True).strip()
            git('init', '-q')
            git('config', 'user.email', 'test@example.invalid')
            git('config', 'user.name', 'CI test')
            (repo/'unchanged.go').write_text('package sample;func Bad( ){ }\n')
            (repo/'changed.go').write_text('package sample\n')
            git('add', '.')
            git('commit', '-qm', 'base')
            base = git('rev-parse', 'HEAD')
            (repo/'changed.go').write_text('package sample\n\nfunc Good() {}\n')
            git('add', '.')
            git('commit', '-qm', 'candidate')
            head = git('rev-parse', 'HEAD')
            # The event base can predate newer target commits. Formatting must
            # compare the actual merge parent, excluding new target-only debt.
            git('checkout', '--detach', '-q', base)
            (repo/'target-only.go').write_text('package sample;func TargetBad( ){ }\n')
            git('add', '.')
            git('commit', '-qm', 'advance target')
            target = git('rev-parse', 'HEAD')
            tree = git('merge-tree', '--write-tree', target, head).splitlines()[0]
            candidate = git('commit-tree', tree, '-p', target, '-p', head, '-m', 'PR integration')
            git('checkout', '--detach', '-q', candidate)
            # Readability has its own regression suite; exercise this real Git/gofmt step.
            (repo/'scripts').mkdir()
            (repo/'scripts/source-readability-audit.sh').write_text('exit 0\n')
            env = {**os.environ, 'BASE_SHA': base, 'INTEGRATION_SHA': candidate,
                   'RUNNER_TEMP': directory, 'HEAD_SHA': head}
            result = subprocess.run(['bash', '-euo', 'pipefail', '-c', command], cwd=repo,
                                    env=env, capture_output=True, text=True)
            self.assertEqual(0, result.returncode, result.stdout + result.stderr)
            git('checkout', '--detach', '-q', head)
            env['INTEGRATION_SHA'] = head
            result = subprocess.run(['bash', '-euo', 'pipefail', '-c', command], cwd=repo,
                                    env=env, capture_output=True, text=True)
            self.assertEqual(0, result.returncode, result.stdout + result.stderr)
            git('checkout', '--detach', '-q', candidate)
            env['INTEGRATION_SHA'] = candidate
            (repo/'changed.go').write_text('package sample;func Bad( ){ }\n')
            result = subprocess.run(['bash', '-euo', 'pipefail', '-c', command], cwd=repo,
                                    env=env, capture_output=True, text=True)
            self.assertNotEqual(0, result.returncode)
            self.assertIn('changed.go', result.stdout)
            env['BASE_SHA'] = 'missing-ref'
            result = subprocess.run(['bash', '-euo', 'pipefail', '-c', command], cwd=repo,
                                    env=env, capture_output=True, text=True)
            self.assertNotEqual(0, result.returncode)

    def test_workflow_tests_ready_updates_without_duplicate_develop_push(self):
        ci = yaml.safe_load((ROOT.parent/'.github/workflows/ci.yml').read_text())
        trigger = ci.get('on', ci.get(True))
        self.assertNotIn('develop', trigger['push']['branches'])
        self.assertTrue({'synchronize', 'ready_for_review'} <= set(trigger['pull_request']['types']))
        for name in ('pr_plan', 'pr_validations', 'pr_focused', 'pr_selected_final'):
            self.assertIn('draft == false', ci['jobs'][name]['if'])
        self.assertNotIn('draft == false', ci['jobs']['source_format']['if'])
        self.assertIn('source_format', ci['jobs']['pr_selected_final']['needs'])


if __name__ == '__main__':
    unittest.main()
