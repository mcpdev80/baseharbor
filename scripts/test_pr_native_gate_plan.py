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

    def test_unknown_path_does_not_select_every_native_runtime_gate(self):
        matrix = planner.resolve_matrix(
            ['cmd/baha/future_unclassified.go'], coverage)
        self.assertEqual(matrix, {'include': []})


if __name__ == '__main__':
    unittest.main()
