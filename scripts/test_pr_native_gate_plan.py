#!/usr/bin/env python3
"""Regression tests for exact catalog-driven native PR gate selection."""
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('pr_native_gate_plan', ROOT / 'pr-native-gate-plan.py')
planner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(planner)
coverage = json.loads((ROOT / 'pr-gate-coverage.json').read_text())


class NativeGatePlanTests(unittest.TestCase):
    def test_workflow_loads_trusted_catalog_and_requirements_with_the_planner(self):
        workflow = (ROOT.parent / '.github/workflows/ci.yml').read_text()
        selector = workflow.split('      - name: Select exact PR native impact', 1)[1].split(
            '\n  pr_native_selected:', 1)[0]
        self.assertIn('git show "$BASE_SHA:scripts/pr-gate-coverage.json"', selector)
        self.assertIn('git show "$BASE_SHA:scripts/validate-pr-gate-coverage.py"', selector)
        self.assertIn('git show "$BASE_SHA:scripts/release-requirements.json"', selector)
        self.assertIn('--coverage "$RUNNER_TEMP/pr-gate-coverage.json"', selector)
        self.assertIn('--requirements "$RUNNER_TEMP/release-requirements.json"', selector)

    def test_container_identity_does_not_select_native_destroy_or_greenfield(self):
        matrix = planner.resolve_matrix(
            ['internal/runtime/container_identity.go'], coverage)
        self.assertEqual(matrix, {'include': []})

    def test_destroy_source_selects_only_its_podman_gate(self):
        matrix = planner.resolve_matrix(
            ['cmd/baha/management_remote_installation_destroy.go'], coverage)
        self.assertEqual(matrix, {'include': [
            {'id': 'native/podman/installation-destroy', 'runtime': 'podman'},
        ]})

    def test_greenfield_change_does_not_select_unrelated_native_gates(self):
        matrix = planner.resolve_matrix(
            ['cmd/baha/greenfield_isolated_core_native_test.go'], coverage)
        self.assertEqual(matrix, {'include': [
            {'id': 'native/docker/greenfield', 'runtime': 'docker'},
        ]})

    def test_second_target_conflict_is_independent_of_destroy(self):
        matrix = planner.resolve_matrix(
            ['cmd/baha/second_target_conflict_native_test.go'], coverage)
        self.assertEqual(matrix, {'include': [
            {'id': 'native/podman/second-target-conflict', 'runtime': 'podman'},
        ]})

    def test_onboarding_contract_and_document_changes_select_both_native_runtimes(self):
        expected = {'include': [
            {'id': 'native/docker/onboarding', 'runtime': 'docker'},
            {'id': 'native/podman/workstation-onboarding', 'runtime': 'podman'},
        ]}
        for path in (
                'cmd/baha/workstation_onboarding_remote_native_test.go',
                'docs/how-to/operator-api.md',
                'docs/how-to/remote-workstation.md',
                'docs/spec/zero-friction-workstation-onboarding-v0427.md'):
            with self.subTest(path=path):
                self.assertEqual(expected, planner.resolve_matrix([path], coverage))

    def test_runtime_validation_branch_runs_only_the_two_onboarding_native_gates(self):
        workflow = (ROOT.parent / '.github/workflows/ci.yml').read_text()
        push = workflow.split('  push:\n', 1)[1].split('  pull_request:', 1)[0]
        self.assertIn("      - 'runtime-validation/onboarding/**'", push)
        job = workflow.split('  targeted_workstation_onboarding:', 1)[1].split('\n  validate:', 1)[0]
        self.assertIn("startsWith(github.ref_name, 'runtime-validation/onboarding/')", job)
        self.assertIn("matrix.runtime == 'docker' && 'native/docker/onboarding'", job)
        self.assertIn("'native/podman/workstation-onboarding'", job)
        self.assertIn('scripts/ci-native-pr-gate.sh "$NATIVE_GATE_ID"', job)
        self.assertIn('if: always()', job)

    def test_unknown_path_does_not_select_every_native_runtime_gate(self):
        matrix = planner.resolve_matrix(
            ['cmd/baha/future_unclassified.go'], coverage)
        self.assertEqual(matrix, {'include': []})

    def test_ci_native_pr_gate_script_selects_both_data_capabilities_gates(self):
        # Card 0428-242 follow-up: scripts/ci-native-pr-gate.sh is the CI runner
        # that executes the native gate. A change to it is a ci-executor change
        # and must select the native data-capabilities gates for both runtimes,
        # otherwise card 0428-237 (#1770) leaves its native matrix SKIPPED.
        matrix = planner.resolve_matrix(
            ['scripts/ci-native-pr-gate.sh'], coverage)
        self.assertEqual(matrix, {'include': [
            {'id': 'native/docker/data-capabilities', 'runtime': 'docker'},
            {'id': 'native/podman/data-capabilities', 'runtime': 'podman'},
        ]})

    def test_ci_native_pr_gate_script_selects_only_data_capabilities(self):
        # The ci-executor change must not leak unrelated native runtime gates.
        matrix = planner.resolve_matrix(
            ['scripts/ci-native-pr-gate.sh', 'cmd/baha/greenfield_isolated_core_native_test.go'],
            coverage)
        self.assertEqual(matrix, {'include': [
            {'id': 'native/docker/data-capabilities', 'runtime': 'docker'},
            {'id': 'native/docker/greenfield', 'runtime': 'docker'},
            {'id': 'native/podman/data-capabilities', 'runtime': 'podman'},
        ]})


if __name__ == '__main__':
    unittest.main()
