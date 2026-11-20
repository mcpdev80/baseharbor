"""Unknown valid source receives real checks; invalid source cannot be exempted."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import yaml

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('planner', ROOT / 'pr-gate-plan.py')
planner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(planner)
CATALOG = json.loads((ROOT / 'release-requirements.json').read_text())
GATE_MAPPING = json.loads((ROOT / 'pr-gate-map-v1.json').read_text())
advisory_spec = importlib.util.spec_from_file_location(
    'candidate_advisory', ROOT / 'pr-candidate-advisory.py')
candidate_advisory = importlib.util.module_from_spec(advisory_spec)
advisory_spec.loader.exec_module(candidate_advisory)
final_spec = importlib.util.spec_from_file_location(
    'required_final', ROOT / 'pr-required-final.py')
required_final = importlib.util.module_from_spec(final_spec)
final_spec.loader.exec_module(required_final)


def changes(path):
    return {'target_base_sha': 'a'*40, 'head_sha': 'b'*40, 'merge_base_sha': 'c'*40,
            'integration_sha': 'd'*40, 'broad_required': False, 'reasons': [],
            'changes': [{'status': 'modified', 'path': path}]}


class ConservativeFallbackTests(unittest.TestCase):
    def test_advisory_delta_runs_new_or_changed_commands_not_owner_text(self):
        base = {'schema': 'baseharbor.pr-gate-plan/v1',
                'required_validation_checks': [
                    {'id': 'existing', 'owner': 'old label', 'command': 'go test ./...'}]}
        candidate = {'schema': 'baseharbor.pr-gate-plan/v1',
                     'required_validation_checks': [
                         {'id': 'existing', 'owner': 'renamed label', 'command': 'go test ./...'},
                         {'id': 'changed', 'owner': 'same ID, new implementation',
                          'command': 'go test ./internal/application'},
                         {'id': 'new', 'owner': 'new validation', 'command': 'go vet ./...'}]}
        self.assertEqual(['changed', 'new'],
                         [row['id'] for row in candidate_advisory.added_checks(base, candidate)])

    def test_candidate_advisory_executes_and_reports_each_candidate_command(self):
        results = candidate_advisory.execute_checks([
            {'id': 'candidate-pass', 'command': 'printf candidate-pass'},
            {'id': 'candidate-fail', 'command': 'printf candidate-fail; exit 3'},
            {'id': 'candidate-setup-error'},
        ], ROOT, os.environ.copy())
        self.assertEqual(['candidate-pass', 'candidate-fail', 'candidate-setup-error'],
                         [row['id'] for row in results])
        self.assertEqual([0, 3, 2], [row['returncode'] for row in results])
        self.assertIn('candidate-pass', results[0]['output'])
        self.assertIn('candidate-fail', results[1]['output'])
        self.assertIn('no executable command', results[2]['output'])

    def test_candidate_advisory_main_replans_exact_diff_and_reports_empty_delta(self):
        source_diff = changes('docs/guide.md')
        base_plan = planner.plan(source_diff, CATALOG, GATE_MAPPING,
                                 context_repository='mcpdev80/baseharbor',
                                 context_target_branch='develop')
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            base_path = root / 'base-plan.json'
            diff_path = root / 'diff.json'
            summary_path = root / 'summary.md'
            base_path.write_text(json.dumps(base_plan))
            diff_path.write_text(json.dumps(source_diff))
            with patch.dict(os.environ, {
                    'GITHUB_REPOSITORY': 'mcpdev80/baseharbor',
                    'GITHUB_BASE_REF': 'develop'}):
                result = candidate_advisory.main([
                    '--base-plan', str(base_path), '--diff', str(diff_path),
                    '--repository', str(ROOT.parent), '--summary', str(summary_path)])
            self.assertEqual(0, result)
            self.assertIn('No candidate-added validation checks', summary_path.read_text())

    def test_legacy_base_fallback_passes_broad_checks_when_candidate_adds_new_vocabulary(self):
        # A stacked PR can carry a newer candidate catalog than its lane base.
        # The trusted legacy vocabulary does not recognize this source area,
        # so it selects broad Go proof rather than parsing candidate-only IDs.
        base_plan = planner.plan(
            changes('internal/new_plan_v2/feature.go'), CATALOG, GATE_MAPPING,
            conservative_fallback=True)
        self.assertEqual('resolved', base_plan['coverage_status'])
        self.assertTrue(base_plan['fallback_used'])
        base_checks = {row['id'] for row in base_plan['required_validation_checks']}
        self.assertTrue({'go-unit-tests', 'go-vet', 'go-build'} <= base_checks)
        self.assertEqual([], base_plan['selected_gates'])

        candidate_plan = {**base_plan,
                          'selected_gates': ['atomic/new-runtime/lifecycle'],
                          'required_validation_checks': [
                              *base_plan['required_validation_checks'],
                              {'id': 'candidate-new-check', 'owner': 'candidate'}]}
        self.assertEqual([{'id': 'candidate-new-check', 'owner': 'candidate'}],
                         candidate_advisory.added_checks(base_plan, candidate_plan))

        needs = {name: {'result': 'success'} for name in required_final.BASE_JOBS}
        needs['pr_unit_shards'] = {'result': 'success'}
        self.assertEqual([], required_final.verify(base_plan, needs))

    def test_unknown_product_executes_broad_checks_without_guessing_runtime_gates(self):
        result = planner.plan(changes('internal/newarea/feature.go'), CATALOG, GATE_MAPPING,
                              conservative_fallback=True)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertTrue(result['fallback_used'])
        self.assertTrue(result['fallback_reasons'])
        self.assertTrue({'go-unit-tests', 'go-vet', 'go-build'} <=
                        {row['id'] for row in result['required_validation_checks']})
        self.assertEqual([], result['selected_gates'])

    def test_broad_fallback_subsumes_only_identical_small_go_checks(self):
        # A new CLI test plus an unresolved product implementation requires
        # broad Go checks, but must not rerun the same CLI tests and build.
        mixed = {**changes('internal/unmapped/production.go'),
                 'changes': [
                     {'status': 'added', 'path': 'cmd/baha/new_regression_test.go'},
                     {'status': 'modified', 'path': 'internal/unmapped/production.go'},
                 ]}
        result = planner.plan(mixed, CATALOG, GATE_MAPPING, conservative_fallback=True)
        ids = [check['id'] for check in result['required_validation_checks']]
        self.assertEqual('resolved', result['coverage_status'])
        self.assertTrue(result['fallback_used'])
        self.assertTrue({'go-unit-tests', 'go-vet', 'go-build'} <= set(ids))
        self.assertFalse({'cli-package-tests', 'cli-guided-tests',
                          'cli-vet', 'cli-build', 'source-contracts',
                          'capability-package-tests'} & set(ids))
        # Formatting isn't provided by Go test/vet/build.
        self.assertIn('cli-gofmt', ids)
        self.assertEqual([], result['selected_gates'])

    def test_broad_fallback_keeps_specialized_runtime_provider_assertions(self):
        mixed = {**changes('internal/unmapped/production.go'),
                 'changes': [
                     {'status': 'modified', 'path': 'internal/runtime/stack_test.go'},
                     {'status': 'modified', 'path': 'internal/unmapped/production.go'},
                 ]}
        result = planner.plan(mixed, CATALOG, GATE_MAPPING, conservative_fallback=True)
        ids = {check['id'] for check in result['required_validation_checks']}
        self.assertTrue({'go-unit-tests', 'go-vet', 'go-build'} <= ids)
        # Specific checks may require explicit PASS lines, isolation or env.
        self.assertIn('runtime-provider-package-tests', ids)

    def test_known_docs_remain_targeted_and_unknown_metadata_does_not_boot_runtimes(self):
        known = planner.plan(changes('docs/guide.md'), CATALOG, GATE_MAPPING, conservative_fallback=True)
        self.assertNotIn('fallback_used', known)
        unknown = planner.plan(changes('NOTICE'), CATALOG, GATE_MAPPING, conservative_fallback=True)
        self.assertEqual('resolved', unknown['coverage_status'])
        self.assertTrue(unknown['fallback_used'])
        self.assertEqual([], unknown['selected_gates'])

    def test_v0427_demo_pin_uses_only_exact_source_validation(self):
        result = planner.plan(changes('docs/releases/v0.4.27.demo-ref'), CATALOG, GATE_MAPPING,
                              conservative_fallback=True)
        self.assertEqual('resolved', result['coverage_status'])
        self.assertNotIn('fallback_used', result)
        self.assertEqual([], result['selected_gates'])
        self.assertEqual(['demo-source-pin-validation'],
                         [row['id'] for row in result['required_validation_checks']])

    def test_v0427_demo_pin_rename_or_mixed_change_stays_conservative(self):
        source = changes('docs/releases/v0.4.27.demo-ref')
        source['changes'].append({'status': 'modified', 'path': 'NOTICE'})
        result = planner.plan(source, CATALOG, GATE_MAPPING, conservative_fallback=True)
        self.assertTrue(result['fallback_used'])

    def test_invalid_source_and_missing_catalog_checks_remain_errors(self):
        for edit in ({'broad_required': True}, {'head_sha': 'bad'},
                     {'changes': [{'status': 'modified', 'path': '../escape.go'}]}):
            result = planner.plan({**changes('internal/newarea/a.go'), **edit}, CATALOG,
                                  GATE_MAPPING, conservative_fallback=True)
            self.assertEqual('unresolved', result['coverage_status'])
        catalog = copy.deepcopy(CATALOG)
        catalog['gates'] = [g for g in catalog['gates'] if g['id'] != 'atomic/podman/lifecycle']
        with self.assertRaisesRegex(ValueError, 'invalid trusted PR gate coverage catalog'):
            planner.plan(changes('internal/newarea/a.go'), catalog, GATE_MAPPING,
                         conservative_fallback=True)

    def test_production_entrypoint_enables_the_fallback(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'diff.json'
            path.write_text(json.dumps(changes('internal/newarea/a.go')))
            out = subprocess.check_output(['python3', str(ROOT/'pr-gate-plan.py'), '--diff', str(path)],
                env={**os.environ, 'GITHUB_REPOSITORY': 'mcpdev80/baseharbor', 'GITHUB_BASE_REF': 'lane-old-base'}, text=True)
            self.assertTrue(json.loads(out)['fallback_used'])

    def test_format_checks_changed_files_without_blocking_on_unchanged_debt(self):
        if shutil.which('gofmt') is None:
            self.skipTest('gofmt is not installed in this environment')
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
        self.assertIn('draft == false', ci['jobs']['source_format']['if'])
        self.assertIn('draft == false', ci['jobs']['pr_native_impact']['if'])
        self.assertIn('draft == false', ci['jobs']['pr_native_selected']['if'])
        self.assertIn('pr_native_impact', ci['jobs']['pr_native_selected']['needs'])
        self.assertIn('fromJSON(needs.pr_native_impact.outputs.matrix)',
                      ci['jobs']['pr_native_selected']['strategy']['matrix'])
        self.assertIn('scripts/ci-native-pr-gate.sh',
                      str(ci['jobs']['pr_native_selected']['steps']))
        self.assertIn('pr_native_selected', ci['jobs']['pr_selected_final']['needs'])
        self.assertIn('source_format', ci['jobs']['pr_selected_final']['needs'])


if __name__ == '__main__':
    unittest.main()
