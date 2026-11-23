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
validator_spec = importlib.util.spec_from_file_location(
    'pr_gate_coverage_validator', ROOT / 'validate-pr-gate-coverage.py')
validator = importlib.util.module_from_spec(validator_spec)
validator_spec.loader.exec_module(validator)
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
        # and must select the native data-capabilities gates for both runtimes.
        matrix = planner.resolve_matrix(
            ['scripts/ci-native-pr-gate.sh'], coverage)
        self.assertEqual(matrix, {'include': [
            {'id': 'native/docker/data-capabilities', 'runtime': 'docker'},
            {'id': 'native/podman/data-capabilities', 'runtime': 'podman'},
        ]})

    def test_app_broker_diagnostics_test_selects_both_data_capabilities_gates(self):
        # Card 0428-248 follow-up: cmd/baha/app_broker_diagnostics_test.go is
        # the broker diagnostics test source (#1763's file). The catalog maps it
        # like cmd/baha/app_broker.go, so a change to it selects the native
        # data-capabilities gates for both runtimes instead of the fail-loud
        # uncataloged abort.
        matrix = planner.resolve_matrix(
            ['cmd/baha/app_broker_diagnostics_test.go'], coverage)
        self.assertEqual(matrix, {'include': [
            {'id': 'native/docker/data-capabilities', 'runtime': 'docker'},
            {'id': 'native/podman/data-capabilities', 'runtime': 'podman'},
        ]})

    def test_workload_port_sources_select_both_data_capabilities_gates(self):
        # Card 0428-248 round-2: cmd/baha/app_workload_ports.go and
        # cmd/baha/workload_port_local_regression_test.go are the #1723 files
        # on the local --workload-port path. The catalog maps both like the
        # broker entries, so a change to either selects the native
        # data-capabilities gates for both runtimes instead of the fail-loud
        # uncataloged abort.
        for path in ('cmd/baha/app_workload_ports.go',
                     'cmd/baha/workload_port_local_regression_test.go'):
            with self.subTest(path=path):
                matrix = planner.resolve_matrix([path], coverage)
                self.assertEqual(matrix, {'include': [
                    {'id': 'native/docker/data-capabilities', 'runtime': 'docker'},
                    {'id': 'native/podman/data-capabilities', 'runtime': 'podman'},
                ]})

    def test_core_installation_runtime_test_selects_both_data_capabilities_gates(self):
        # Card 0428-248 coreinst-slice: cmd/baha/core_installation_runtime_test.go
        # is the #1738 file (installation destroy-stamp through the real
        # boundary). The catalog maps it in the #1768/#1777 pattern onto the
        # existing data-capabilities surfaces for both runtimes, so a change
        # selects the native gates instead of the fail-loud uncataloged abort.
        matrix = planner.resolve_matrix(
            ['cmd/baha/core_installation_runtime_test.go'], coverage)
        self.assertEqual(matrix, {'include': [
            {'id': 'native/docker/data-capabilities', 'runtime': 'docker'},
            {'id': 'native/podman/data-capabilities', 'runtime': 'podman'},
        ]})

    def test_perses_cluster_sources_are_cataloged_on_both_observability_surfaces(self):
        # Card 0428-248 perses round: the #1597 sources (the perses dashboards
        # cluster in internal/metrics plus the CLI/status surfaces in
        # cmd/baha) must be cataloged so the trusted-planner guardian resolves
        # them instead of aborting with an uncataloged source change. They map
        # onto the existing source.gate.atomic observability surfaces, which
        # are owned by the gate-owned atomic observability gates (no native
        # check covers them, so the native matrix stays empty - exactly like
        # the existing cmd/baha/app_observability_acceptance_test.go entry).
        paths = (
            'cmd/baha/app_status_result.go',
            'cmd/baha/app_perses_cli_test.go',
            'internal/metrics/perses_dashboards.go',
            'internal/metrics/perses_dashboards_helpers.go',
            'internal/metrics/perses_dashboards_test.go',
            'internal/metrics/perses_dashboards_0428-076_native_test.go',
        )
        surfaces = {'source.gate.atomic.docker.observability',
                    'source.gate.atomic.podman.observability'}
        catalog = {component['path']: component
                   for component in coverage['components']
                   if 'path' in component}
        for path in paths:
            with self.subTest(path=path):
                self.assertIn(path, catalog)
                self.assertEqual(set(catalog[path]['surfaces']), surfaces)
                # Guardian: the path resolves to declared owners, never to the
                # fail-loud uncataloged abort. runtime_specific sources select
                # both the docker and podman atomic observability gates.
                covered, unowned = validator.resolve_covered_gates([path], coverage)
                self.assertEqual(unowned, [])
                self.assertEqual(
                    set(covered),
                    {'atomic/docker/observability', 'atomic/podman/observability'})
                # No native check exists for these surfaces, so the native
                # matrix must stay empty rather than invent gates.
                self.assertEqual(planner.resolve_matrix([path], coverage),
                                 {'include': []})
        # The #1597 testdata JSON files ride the same observability ownership
        # through the internal/metrics/testdata package row (shared, not
        # runtime-specific: the representative docker gate). None of them may
        # hit the fail-loud uncataloged abort.
        covered, unowned = validator.resolve_covered_gates([
            'internal/metrics/testdata/perses-dashboards/loki-logs.json',
            'internal/metrics/testdata/perses-native/v0.55.1/loki-app-errors.json',
        ], coverage)
        self.assertEqual(unowned, [])
        self.assertEqual(set(covered), {'atomic/docker/observability'})


if __name__ == '__main__':
    unittest.main()
