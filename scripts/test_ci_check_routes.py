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

    def test_every_declared_check_has_registry_and_workflow_executor(self):
        checks = declared_checks((ROOT / 'scripts/pr-gate-plan.py').read_text(), self.runner)
        self.assertEqual(checks, self.runner.VALIDATIONS)
        patterns = workflow_check_patterns(self.script)
        self.assertEqual([], sorted(check for check in checks
                                   if not any(fnmatch.fnmatchcase(check, pattern) for pattern in patterns)))
        self.assertTrue(any('exit 2' in line for line in self.script.splitlines()))

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
