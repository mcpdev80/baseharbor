"""Check the complete normal planner/runner/workflow routing contract."""
import ast
import fnmatch
import importlib.util
import json
from pathlib import Path
import re
import shlex
import unittest

import yaml

ROOT = Path(__file__).resolve().parent.parent


def load(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'scripts' / (name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def declared_checks(source, runner):
    checks = set(runner.WIZARD_TEST_FAMILIES)
    for node in ast.walk(ast.parse(source)):
        if isinstance(node, ast.Dict):
            values = {key.value: value.value for key, value in zip(node.keys, node.values)
                      if isinstance(key, ast.Constant) and isinstance(value, ast.Constant)}
            if isinstance(values.get('id'), str):
                checks.add(values['id'])
    return checks


def workflow_check_patterns(script):
    return [pattern for line in script.splitlines()
            if (match := re.match(r'^  ([^\s]+)\)', line)) and match[1] != '*'
            for pattern in match[1].split('|')]


def test_functions(package):
    folder = ROOT / package.removeprefix('./').removesuffix('/...')
    files = folder.rglob('*_test.go') if package.endswith('/...') else folder.glob('*_test.go')
    return {name for path in files
            for name in re.findall(r'^func (Test\w+)\(', path.read_text(), re.M)}


class CheckRoutingTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.runner = load('pr-check-runner')
        cls.ci = yaml.safe_load((ROOT / '.github/workflows/ci.yml').read_text())
        cls.script = next(step['run'] for step in cls.ci['jobs']['pr_validations']['steps']
                          if step.get('name') == 'Execute existing validation owner without publication')

    def test_migration_controller_is_an_immutable_commit(self):
        env = next(step['env'] for step in self.ci['jobs']['pr_plan']['steps']
                   if 'MIGRATION_CONTROLLER' in step.get('env', {}))
        self.assertRegex(env['MIGRATION_CONTROLLER'], r'^[0-9a-f]{40}$')

    def test_stacked_lane_prs_trigger_the_full_pr_check_graph(self):
        workflow = (ROOT / '.github/workflows/ci.yml').read_text()
        self.assertIn("      - 'lane-*'", workflow)
        self.assertIn("startsWith(github.base_ref, 'lane-')", workflow)
        self.assertIn('BASE_REF: ${{ github.base_ref }}', workflow)
        self.assertIn('target_tip="$BASE_SHA"', workflow)

    def test_named_cli_and_worker_stack_bases_trigger_and_select_pr_checks(self):
        workflow = (ROOT / '.github/workflows/ci.yml').read_text()
        pull_request = workflow.split('  pull_request:\n', 1)[1].split('\n\nconcurrency:', 1)[0]
        patterns = re.findall(r"^      - '?([^'\n]+)'?$", pull_request, re.M)
        for base in ('cli2-env-policy-head', 'cli2-identity-secrets-head',
                     'w083-perses-datasources', 'w091-0428-091'):
            with self.subTest(base=base):
                self.assertTrue(any(fnmatch.fnmatchcase(base, pattern) for pattern in patterns),
                                f'pull_request.branches does not match {base}: {patterns}')
        base_conditions = [line for line in workflow.splitlines()
                           if 'github.base_ref' in line and 'startsWith(github.base_ref, \'lane-\')' in line]
        self.assertGreater(len(base_conditions), 10)
        for line in base_conditions:
            with self.subTest(condition=line[:100]):
                self.assertIn("startsWith(github.base_ref, 'cli2-')", line)
                self.assertIn("startsWith(github.base_ref, 'w0')", line)

    def test_verified_oras_precedes_runtime_bootstrap_tests(self):
        docker_runtime = yaml.safe_load(
            (ROOT / '.github/workflows/docker-runtime-acceptance.yml').read_text())
        steps = docker_runtime['jobs']['docker-runtime']['steps']
        oras = next(i for i, step in enumerate(steps)
                    if step.get('run') == 'bash scripts/ci-install-oras.sh')
        native = next(i for i, step in enumerate(steps)
                      if step.get('id') == 'runtime_integration')
        self.assertLess(oras, native)

        observability = yaml.safe_load(
            (ROOT / '.github/workflows/observability-acceptance.yml').read_text())
        for job_name in ('docker', 'podman'):
            job_steps = observability['jobs'][job_name]['steps']
            with self.subTest(job=job_name):
                oras = next(i for i, step in enumerate(job_steps)
                            if step.get('run') == 'bash scripts/ci-install-oras.sh')
                acceptance = next(i for i, step in enumerate(job_steps)
                                  if 'TestObservabilityFullStackAcceptanceInCI' in step.get('run', ''))
                self.assertLess(oras, acceptance)

        dispatch_inputs = (observability.get('on') or observability.get(True))['workflow_dispatch']['inputs']
        self.assertEqual({'description': 'Run both Docker and rootless Podman acceptance jobs',
                          'required': False, 'default': True, 'type': 'boolean'},
                         dispatch_inputs['all_runtimes'])
        self.assertIn('inputs.all_runtimes', observability['jobs']['docker']['if'])
        self.assertIn('inputs.all_runtimes', observability['jobs']['podman']['if'])
        self.assertIn('github.ref_name, \'/podman/\'', observability['jobs']['podman']['if'])

        native_script = (ROOT / 'scripts/ci-native-pr-gate.sh').read_text()
        self.assertRegex(native_script,
                         r'native/docker/perses-runtime-smoke\)\s+export BASEHARBOR_PERSES_RUNTIME_SMOKE=1\s+run_exact_test TestPersesRuntimeSmokeDockerIsolated 5m ./internal/application')

    def test_docker_dispatch_uses_selected_ref_and_receipt_check_only_runs_after_tests(self):
        workflow = (ROOT / '.github/workflows/docker-runtime-acceptance.yml').read_text()
        dispatch = workflow.split('  workflow_dispatch:\n', 1)[1].split('\n  push:\n', 1)[0]
        self.assertIn('required: false', dispatch)
        self.assertNotIn('default:', dispatch)
        self.assertNotIn('release/v0.4.21', dispatch)
        self.assertIn('ref: ${{ inputs.ref || github.sha }}', workflow)
        check = workflow.split('      - name: Verify all eight executed runtime tests\n', 1)[1]
        for step in ['runtime_preflight', 'runtime_integration', 'recovery',
                     'log_ingestion', 'trace_ingestion']:
            self.assertIn(f"steps.{step}.outcome == 'success'", check)
        podman = (ROOT / '.github/workflows/podman-acceptance.yml').read_text()
        podman_dispatch = podman.split('  workflow_dispatch:\n', 1)[1].split('\n  push:\n', 1)[0]
        self.assertIn('required: false', podman_dispatch)
        self.assertNotIn('default:', podman_dispatch)
        self.assertIn('ref: ${{ inputs.ref || github.sha }}', podman)

    def test_candidate_checks_are_advisory_and_final_uses_only_trusted_base_plan(self):
        workflow = (ROOT / '.github/workflows/ci.yml').read_text()
        advisory = self.ci['jobs']['pr_candidate_advisory']
        final = self.ci['jobs']['pr_selected_final']
        required_final = load('pr-required-final')
        self.assertTrue(advisory['continue-on-error'])
        self.assertEqual({'contents': 'read'}, advisory['permissions'])
        self.assertEqual(list(required_final.BASE_WORKFLOW_JOBS), final['needs'])
        self.assertNotIn('pr_candidate_advisory', final['needs'])
        advisory_run = next(step['run'] for step in advisory['steps']
                            if 'pr-candidate-advisory.py' in step.get('run', ''))
        self.assertIn('--base-plan pr-plan.json', advisory_run)
        self.assertIn('--diff pr-diff.json', advisory_run)
        self.assertIn('candidate_advisory_required', self.ci['jobs']['pr_plan']['outputs'])

        final_run = next(step['run'] for step in final['steps']
                         if 'pr-required-final.py' in step.get('run', ''))
        self.assertIn('git show "$CONTROLLER_SHA:scripts/pr-required-final.py"', final_run)
        self.assertIn('--plan pr-plan.json', final_run)
        self.assertNotIn('--base-plan pr-plan.json', final_run)
        plan_job = workflow[workflow.index('  pr_plan:'):workflow.index('  pr_candidate_advisory:')]
        self.assertIn('for policy_file in pr-gate-plan.py', plan_job)
        self.assertIn('git show "$controller:scripts/$policy_file"', plan_job)
        self.assertIn('controller="$effective_base"', workflow)
        self.assertIn('controller="$MIGRATION_CONTROLLER"', workflow)

    def test_versionless_release_qualification_job_names(self):
        workflow = (ROOT / '.github/workflows/pre-release.yml').read_text()
        self.assertIn('name: Validate release readiness', workflow)
        self.assertIn('name: HA groups · All runtime results', workflow)
        self.assertNotIn('v0421_ha_groups', workflow)
        self.assertNotIn('v0.4.21 HA ·', workflow)

    def test_every_declared_check_has_registry_and_workflow_executor(self):
        checks = declared_checks((ROOT / 'scripts/pr-gate-plan.py').read_text(), self.runner)
        self.assertEqual(checks, self.runner.VALIDATIONS)
        patterns = workflow_check_patterns(self.script)
        self.assertEqual([], sorted(check for check in checks
                                   if not any(fnmatch.fnmatchcase(check, pattern) for pattern in patterns)))
        self.assertTrue(any('exit 2' in line for line in self.script.splitlines()))

    def test_planner_validation_executes_runtime_reuse_regressions(self):
        self.assertIn('pr-planner-tests)', self.script)
        self.assertIn('scripts/test_pre_release_runtime_reuse.py', self.script)

    def test_named_go_profiles_match_real_test_functions(self):
        names = test_functions('./cmd/baha')
        for check, patterns in self.runner.WIZARD_TEST_FAMILIES.items():
            for pattern in patterns:
                with self.subTest(check=check, pattern=pattern):
                    self.assertTrue(any(re.search(pattern, name) for name in names),
                                    'required test family has no source test')
        # Inspect literal filters in the normal validation handlers, including
        # Guided, Inspect/Reconcile and the provider/doctor checks.
        runtime = yaml.safe_load((ROOT / '.github/workflows/pre-release-runtime-suite.yml').read_text())
        scripts = [self.script] + [step['run'] for step in runtime['jobs']['gate']['steps']
                                 if step.get('id') in {'runtime_gate', 'integration_gate'}]
        lines = [line for script in scripts for line in script.replace('\\\n', ' ').splitlines()]
        for line in lines:
            if 'go test ' not in line or '-run ' not in line:
                continue
            words = shlex.split(line.split('| tee ', 1)[0])
            pattern = words[words.index('-run') + 1]
            if '$' in pattern.removesuffix('$'):
                continue
            packages = [word for word in words if word.startswith('./')]
            with self.subTest(command=line.strip()):
                if not packages:
                    continue  # Variable package/name profiles are checked by their route tests.
                self.assertTrue(any(re.search(pattern, name) for package in packages
                                    for name in test_functions(package)))
        for check in load('pre-release-integration').CHECKS.values():
            for arguments in check:
                if '-run' in arguments:
                    pattern = arguments[arguments.index('-run') + 1]
                    packages = [word for word in arguments if word.startswith('./')]
                    with self.subTest(packages=packages, pattern=pattern):
                        self.assertTrue(any(re.search(pattern, name) for package in packages
                                            for name in test_functions(package)))

    def test_selective_identity_source_routes_to_exact_pr_runtime_test(self):
        runtime = yaml.safe_load((ROOT / '.github/workflows/pre-release-runtime-suite.yml').read_text())
        script = next(step['run'] for step in runtime['jobs']['gate']['steps']
                      if step.get('id') == 'runtime_gate')
        self.assertIn('cmd/baha/app_identity_selective_runtime_test.go', script)
        self.assertIn('BASEHARBOR_IDENTITY_LIFECYCLE_ACCEPTANCE=1', script)
        self.assertIn('run_native_test ./cmd/baha TestSelectiveIdentityRuntimeAddRemoveReAdd', script)
        self.assertIn('=== RUN', script)
        self.assertIn('--- PASS:', script)

    def test_pr_selected_gates_have_native_or_contract_jobs(self):
        source = ast.parse((ROOT / 'scripts/pr-gate-plan.py').read_text())
        gates = {node.value for node in ast.walk(source)
                 if isinstance(node, ast.Constant) and isinstance(node.value, str)
                 and re.fullmatch(r'(?:atomic|integration)/(?:docker|podman|static)/[\w-]+', node.value)}
        inventory = json.loads((ROOT / 'scripts/release-requirements.json').read_text())
        selection = load('pr-selected-runtime-matrix').select({'selected_gates': sorted(gates)}, inventory)
        self.assertEqual([], selection['unsupported'])
        runtime = yaml.safe_load((ROOT / '.github/workflows/pre-release-runtime-suite.yml').read_text())
        step = next(step for step in runtime['jobs']['gate']['steps'] if step.get('id') == 'runtime_gate')
        self.assertEqual('${{ github.event.pull_request.head.sha }}', step['env']['PR_HEAD_SHA'])
        self.assertIn('--head "$PR_HEAD_SHA"', step['run'])
        plan = {'schema': 'baseharbor.pr-gate-plan/v1', 'coverage_status': 'resolved',
                'unresolved_coverage': [], 'selected_gates': sorted(gates),
                'required_validation_checks': [{'id': 'documentation-audit'}],
                'required_consumer_dependencies': ['core', 'demo']}
        final = load('pr-required-final')
        needs = {name: {'result': 'success'} for name in self.ci['jobs']['pr_selected_final']['needs']}
        self.assertEqual([], final.verify(plan, needs))
        for name in ('pr_runtime_selected', 'pr_public_contracts'):
            with self.subTest(job=name):
                self.assertTrue(final.verify(plan, {**needs, name: {'result': 'skipped'}}))

    def test_complete_release_catalog_has_executor_and_consumer_classification(self):
        inventory = json.loads((ROOT / 'scripts/release-requirements.json').read_text())
        catalog = [gate['id'] for gate in inventory['gates'] if 'v0.4.26' in gate['required_for']]
        self.assertEqual(len(catalog), len(set(catalog)))
        selection = load('pre-release-check-plan').select(inventory, 'v0.4.26')
        self.assertEqual(len(catalog), sum(len(values) for values in selection.values()))
        dependency = load('pr-gate-dependencies').dependency_mode({'selected_gates': catalog}, inventory)
        self.assertEqual('ecosystem', dependency['mode'])
        self.assertEqual(['connector', 'console', 'core', 'demo'], dependency['dependencies'])


if __name__ == '__main__':
    unittest.main()
