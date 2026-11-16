import importlib.util
import json
import os
from unittest.mock import patch
import pathlib
import subprocess
import sys
import tempfile
import unittest


spec = importlib.util.spec_from_file_location('integration', pathlib.Path(__file__).with_name('pre-release-integration.py'))
integration = importlib.util.module_from_spec(spec)
spec.loader.exec_module(integration)


class ContractCheckTests(unittest.TestCase):
    def test_coreless_baseline_test_anchor_is_bound_to_both_runtime_executors(self):
        anchors = {
            'cmd/baha/app_backend_selective_runtime_test.go':
                'TestSelectiveBackendCleanupPreservesRuntimeBaseline',
        }
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            for source_path, name in anchors.items():
                source = root / source_path
                source.parent.mkdir(parents=True, exist_ok=True)
                source.write_text(f'func {name}(t *testing.T) {{}}\n')
            for runtime in ('docker', 'podman'):
                gate = f'integration/{runtime}/coreless-lifecycle'
                self.assertEqual([
                    ['./cmd/baha', '-run', '^TestSelectiveBackendCleanupPreservesRuntimeBaseline$', '-timeout', '20m'],
                ],
                                 integration.coreless_anchor_checks(gate, root))
            (root / next(iter(anchors))).write_text('package main\n')
            with self.assertRaisesRegex(ValueError, 'mandatory Coreless test anchor'):
                integration.coreless_anchor_checks('integration/docker/coreless-lifecycle', root)
        self.assertEqual([], integration.coreless_anchor_checks('integration/static/public-contracts'))

    def test_public_contracts_require_aggregate_provider_transport_authority(self):
        selections = integration.CHECKS['integration/static/public-contracts']
        self.assertIn(['./cmd/baha', '-run', '^TestExternalProviderRemoval'], selections)
        self.assertIn(['./internal/application', '-run', '^TestExternalRemoval'], selections)

    def test_candidate_label_cannot_qualify_changed_or_untracked_source(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            def git(*args):
                return subprocess.check_output(['git', '-C', str(root), *args], stderr=subprocess.DEVNULL)
            git('init', '--quiet')
            source = root / 'fixture.go'
            source.write_text('original source\n')
            git('add', 'fixture.go')
            git('-c', 'user.name=Test', '-c', 'user.email=test@example.invalid',
                'commit', '--quiet', '-m', 'candidate')
            candidate = git('rev-parse', 'HEAD').decode().strip()
            integration.verify_candidate_source(candidate, root)
            with self.assertRaises(ValueError):
                integration.verify_candidate_source('f' * 40, root)
            source.write_text('altered source\n')
            with self.assertRaises(ValueError):
                integration.verify_candidate_source(candidate, root)
            git('add', 'fixture.go')
            with self.assertRaises(ValueError):
                integration.verify_candidate_source(candidate, root)
            git('restore', '--staged', '--worktree', 'fixture.go')
            (root / 'unexpected_test.go').write_text('extra test input\n')
            with self.assertRaises(ValueError):
                integration.verify_candidate_source(candidate, root)
            (root / 'unexpected_test.go').unlink()
            integration.verify_candidate_source(candidate, root)

    def check(self, events, code=0):
        with tempfile.TemporaryDirectory() as root:
            log = pathlib.Path(root) / 'go-test.jsonl'
            output = '\n'.join(json.dumps(event) for event in events)
            command = [sys.executable, '-c', f'import sys; print({output!r}); sys.exit({code})']
            return integration.checked_tests(command, log)


    def test_native_gate_rejects_foreign_or_unprepared_runtime(self):
        for environment in [{}, {'BASEHARBOR_TEST_RUNTIME': 'podman'}]:
            with patch.dict(os.environ, environment, clear=True):
                with self.assertRaises(ValueError):
                    integration.configure_runtime_environment('integration/docker/core-bootstrap')

    def test_native_gate_enables_all_required_runtime_regressions(self):
        with patch.dict(os.environ, {'BASEHARBOR_TEST_RUNTIME': 'docker'}, clear=True):
            self.assertTrue(integration.configure_runtime_environment('integration/docker/core-bootstrap'))
            for key in ['BASEHARBOR_CORE_BOOTSTRAP_ACCEPTANCE',
                        'BASEHARBOR_BUG_HUNT_LIFECYCLE_ACCEPTANCE',
                        'BASEHARBOR_TEMPLATE_BUILD_ACCEPTANCE']:
                self.assertEqual(os.environ[key], '1')
        with patch.dict(os.environ, {}, clear=True):
            self.assertFalse(integration.configure_runtime_environment('integration/static/public-contracts'))
            self.assertEqual(dict(os.environ), {})

    def test_core_bootstrap_gate_keeps_both_machine_roles_on_both_runtimes(self):
        source = pathlib.Path(__file__).resolve().parents[1] / 'cmd/baha/core_installation_runtime_test.go'
        text = source.read_text()
        self.assertIn('roles := []coreinstallation.MachineRole{coreinstallation.Development, coreinstallation.Deployment}', text)
        self.assertIn('approvedNativeFixtureCleanup(t, cleanupCtx, io.Discard)', text)
        for runtime in ['docker', 'podman']:
            gate = 'integration/' + runtime + '/core-bootstrap'
            selections = integration.CHECKS[gate]
            acceptance = [args for args in selections if '^TestCoreOnlyBootstrapRuntimeAcceptance$' in args]
            self.assertEqual(acceptance, [['./cmd/baha', '-run', '^TestCoreOnlyBootstrapRuntimeAcceptance$', '-timeout', '28m']])

    def test_coreless_gates_require_selected_runtime_and_enable_real_tests(self):
        for runtime in ['docker', 'podman']:
            gate = 'integration/' + runtime + '/coreless-lifecycle'
            with patch.dict(os.environ, {'BASEHARBOR_TEST_RUNTIME': runtime}, clear=True):
                self.assertTrue(integration.configure_runtime_environment(gate))
                self.assertEqual(os.environ['BASEHARBOR_CORELESS_ACCEPTANCE'], '1')
                self.assertEqual(os.environ['BASEHARBOR_IDENTITY_LIFECYCLE_ACCEPTANCE'], '1')
                self.assertEqual(os.environ['BASEHARBOR_BACKEND_LIFECYCLE_ACCEPTANCE'], '1')
                self.assertEqual(os.environ['BASEHARBOR_HTTPS_LIFECYCLE_ACCEPTANCE'], '1')
                self.assertEqual(os.environ['BASEHARBOR_SECRETS_LIFECYCLE_ACCEPTANCE'], '1')
                self.assertEqual(os.environ['BASEHARBOR_RENOVATE_RECOVERY_ACCEPTANCE'], '1')
                self.assertEqual(os.environ['BASEHARBOR_APP_STOP_ACCEPTANCE'], '1')
            with patch.dict(os.environ, {}, clear=True):
                with self.assertRaises(ValueError):
                    integration.configure_runtime_environment(gate)

    def test_release_proof_requires_each_selective_fixture_on_both_engines(self):
        required = {'^TestCorelessFreshWorkloadRuntimeLifecycle$',
                    '^TestCorelessLocalProdRuntimeLifecycle$',
                    '^TestSelectiveIdentityRuntimeAddRemoveReAdd$',
                    '^TestSelectiveBackendRuntimeAddRemoveReAdd$',
                    '^TestSelectiveApplicationBackendRuntimeAddRemoveReAdd$',
                    '^TestSelectiveHTTPSRuntimeAddRemoveReAdd$',
                    '^TestManagedRegisteredOTLPNativeConsumerHandoff$',
                    '^TestRenovateVolumeRecoveryRuntimeAcceptance$',
                    '^TestApplicationCapabilityLifecycleExistingCore$'}
        for runtime in ['docker', 'podman']:
            selections = integration.CHECKS['integration/' + runtime + '/coreless-lifecycle']
            expressions = [args[args.index('-run') + 1] for args in selections]
            self.assertEqual(set(expressions), required)
            self.assertEqual(len(expressions), len(required))
            for expression in expressions:
                name = expression.removeprefix('^').removesuffix('$')
                package = selections[expressions.index(expression)][0].removeprefix('./')
                sources = pathlib.Path(__file__).resolve().parents[1].joinpath(package)
                self.assertTrue(any('func ' + name + '(' in p.read_text()
                                    for p in sources.glob('*_test.go')), name)

    def test_coreless_shards_partition_every_runtime_test_exactly_once(self):
        expected = {
            'TestCorelessFreshWorkloadRuntimeLifecycle',
            'TestCorelessLocalProdRuntimeLifecycle',
            'TestSelectiveIdentityRuntimeAddRemoveReAdd',
            'TestSelectiveBackendRuntimeAddRemoveReAdd',
            'TestSelectiveApplicationBackendRuntimeAddRemoveReAdd',
            'TestSelectiveHTTPSRuntimeAddRemoveReAdd',
            'TestManagedRegisteredOTLPNativeConsumerHandoff',
            'TestRenovateVolumeRecoveryRuntimeAcceptance',
            'TestApplicationCapabilityLifecycleExistingCore',
        }
        backend_source = pathlib.Path(__file__).resolve().parents[1] / 'cmd/baha/app_backend_selective_runtime_test.go'
        expected.add('TestSelectiveBackendCleanupPreservesRuntimeBaseline')
        for runtime in ['docker', 'podman']:
            gate = f'integration/{runtime}/coreless-lifecycle'
            integration.validate_coreless_shard_partition(gate)
            assigned = []
            for shard in integration.CORELESS_SHARD_NAMES:
                commands = integration.coreless_shard_checks(gate, shard)
                self.assertTrue(commands)
                for command in commands:
                    self.assertIn('-run', command)
                    go_command = integration.go_test_command(command)
                    self.assertIn('-race', go_command)
                    self.assertIn('-count=1', go_command)
                    assigned.append(command[command.index('-run') + 1][1:-1])
            self.assertEqual(len(assigned), len(set(assigned)))
            self.assertEqual(set(assigned), expected)

    def test_registry_is_an_explicit_owned_resource_and_not_an_inventory_filter(self):
        root = pathlib.Path(__file__).resolve().parents[1]
        script = (root / 'scripts/ci-local-oci-registry.sh').read_text()
        self.assertIn('name="baseharbor-pre-release-registry"', script)
        self.assertIn('--label baseharbor.pre-release.registry=true', script)
        self.assertIn('runtime_cmd rm --force "$name"', script)
        runtime_tests = '\n'.join(path.read_text() for path in (root / 'cmd/baha').glob('*runtime_test.go'))
        self.assertNotIn('baseharbor.pre-release.registry', runtime_tests)
        self.assertIn('runtimeContainersNotInBaseline', runtime_tests)

    def test_coreless_shard_catalog_rejects_unknown_or_misapplied_shards(self):
        with self.assertRaises(ValueError):
            integration.coreless_shard_checks(
                'integration/static/public-contracts', 'foundation')
        with self.assertRaises(ValueError):
            integration.coreless_shard_checks(
                'integration/docker/coreless-lifecycle', 'unknown')

    def test_release_coreless_shards_preserve_aggregate_gate_jobs(self):
        root = pathlib.Path(__file__).resolve().parents[1]
        workflow = (root / '.github/workflows/pre-release.yml').read_text()
        requirements = json.loads((root / 'scripts/release-requirements.json').read_text())
        for runtime in ('docker', 'podman'):
            aggregate_id = f'integration_coreless_{runtime}'
            shard_id = aggregate_id + '_shards'
            gate = f'integration/{runtime}/coreless-lifecycle'
            shard = workflow.split(f'  {shard_id}:\n', 1)[1].split('\n  integration_coreless_', 1)[0]
            aggregate = workflow.split(f'  {aggregate_id}:\n', 1)[1].split('\n  integration_coreless_', 1)[0]
            self.assertIn('fail-fast: false', shard)
            self.assertIn('shard: [foundation, identity-backend, application, core-capability]', shard)
            self.assertIn('--shard "${{ matrix.shard }}"', shard)
            self.assertIn(f'needs: [source, {shard_id}]', aggregate)
            self.assertIn(f'needs.{shard_id}.result', aggregate)
            requirement = next(row for row in requirements['gates'] if row['id'] == gate)
            self.assertEqual([aggregate_id], requirement['jobs'])

    def test_backend_subcase_skip_rejects_otherwise_passing_native_proof(self):
        with self.assertRaises(ValueError):
            self.check([{'Action': 'pass', 'Package': 'core',
                         'Test': 'TestSelectiveIdentityRuntimeAddRemoveReAdd'},
                        {'Action': 'pass', 'Package': 'core',
                         'Test': 'TestSelectiveBackendRuntimeAddRemoveReAdd/sql'},
                        {'Action': 'skip', 'Package': 'core',
                         'Test': 'TestSelectiveBackendRuntimeAddRemoveReAdd/cache'},
                        {'Action': 'pass', 'Package': 'core',
                         'Test': 'TestSelectiveBackendRuntimeAddRemoveReAdd'}])

    def test_zero_exit_without_tests_is_rejected(self):
        with self.assertRaises(ValueError):
            self.check([{'Action': 'pass', 'Package': 'empty/package'}])

    def test_skipped_required_checks_cannot_produce_success(self):
        with self.assertRaises(ValueError):
            self.check([{'Action': 'pass', 'Package': 'core', 'Test': 'TestPassed'},
                        {'Action': 'skip', 'Package': 'core', 'Test': 'TestNotRun'}])

    def test_partial_pass_before_process_failure_is_rejected(self):
        with self.assertRaises(ValueError):
            self.check([{'Action': 'pass', 'Package': 'core', 'Test': 'TestPassed'}], code=1)

    def test_passed_checks_retain_names_and_original_log_binding(self):
        result = self.check([{'Action': 'pass', 'Package': 'core', 'Test': 'TestRequired'}])
        self.assertEqual(result['passed'], ['core/TestRequired'])
        self.assertEqual(result['log'], 'go-test.jsonl')


if __name__ == '__main__':
    unittest.main()
