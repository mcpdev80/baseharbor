import copy
import hashlib
import importlib.util
import io
import json
import pathlib
import subprocess
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location('resume', pathlib.Path(__file__).with_name('pre-release-resume.py'))
resume = importlib.util.module_from_spec(spec)
spec.loader.exec_module(resume)


def archive(manifest, entry=None):
    stream = io.BytesIO()
    with zipfile.ZipFile(stream, 'w') as output:
        output.writestr('manifest.json', json.dumps(manifest))
        if entry:
            output.writestr(entry, 'unsafe')
    data = stream.getvalue()
    return data, 'sha256:' + hashlib.sha256(data).hexdigest()


def manifest(key='atomic/static/mcp', run=1, attempt=1):
    kind, *parts = key.split('/')
    runtime = 'static' if kind == 'adoption' else parts[0]
    data = {'schema': resume.SCHEMAS[kind], 'candidate_sha': 'a' * 40, 'demo_ref': 'b' * 40,
            'workflow_run_id': run if kind == 'journey' else str(run), 'workflow_run_attempt': attempt,
            'outcome': 'success', 'cleanup_outcome': 'skipped' if runtime == 'static' else 'success'}
    if kind == 'atomic':
        data.update(id='/'.join(parts), runtime=runtime, gate=parts[1])
    elif kind == 'adoption':
        data.update(gate=parts[0])
    elif kind == 'ha':
        data.update(runtime=runtime, group=parts[1])
    else:
        data.update(runtime=runtime, duration_seconds=30)
    return data


class FakeInputs:
    def requirements(self, candidate, tag):
        return resume.local_requirements(tag)

    def ensure(self, *args):
        pass

    def pin(self, *args):
        return 'b' * 40

    def fingerprint(self, candidate, demo, tag, key):
        return resume.digest([key, tag, demo])


class FakeAPI:
    repository = 'mcpdev80/baseharbor'

    def __init__(self):
        self.jobs = {1: [self.job(1)], 2: [self.job(2)]}
        self.data = {}
        self.artifacts = {}
        for run in [1, 2]:
            data, checksum = archive(manifest(run=run))
            self.data[run] = data
            self.artifacts[run] = [{'id': run, 'name': resume.artifact_name('atomic/static/mcp', run, 1),
                                    'expired': False, 'digest': checksum,
                                    'workflow_run': {'id': run, 'head_sha': 'a' * 40}}]
        self.event = 'push'

    def job(self, run, attempt=1, conclusion='success'):
        return {'id': 100 + run, 'name': 'selected_gate / static · mcp', 'run_attempt': attempt,
                'head_sha': 'a' * 40, 'status': 'completed', 'conclusion': conclusion}

    def get(self, path):
        return {'path': '.github/workflows/pre-release.yml', 'event': self.event, 'status': 'completed',
                'conclusion': 'cancelled', 'head_sha': 'a' * 40,
                'repository': {'full_name': self.repository}, 'head_repository': {'full_name': self.repository}}

    def pages(self, path, field):
        run = int(path.split('/')[2])
        return (self.jobs if field == 'jobs' else self.artifacts)[run]

    def archive(self, artifact):
        return self.data[artifact['id']]


class EvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        self.api = FakeAPI()

    def collect(self, runs=(1, 2)):
        return resume.collect(self.api, FakeInputs(), 'a' * 40, 'b' * 40, 'v0.4.22', runs, self.root)

    def test_cancelled_parent_successful_child_is_preserved_with_origin(self):
        result = self.collect((1,))
        self.assertEqual(result['proofs'][0]['origin']['run_id'], 1)
        self.assertEqual(len(result['pending']), 54)
        self.assertEqual(json.loads(zipfile.ZipFile(self.root / '1.zip').read('manifest.json')), manifest())

    def test_newer_failure_or_cancellation_never_falls_back(self):
        for outcome in ['failure', 'cancelled', 'skipped', None]:
            with self.subTest(outcome=outcome):
                self.api.jobs[2][0]['conclusion'] = outcome
                result = self.collect()
                self.assertEqual(result['proofs'], [])
                self.assertIn('atomic/static/mcp', result['pending'])

    def test_newer_attempt_blocks_older_success_without_artifact(self):
        self.api.jobs[1].append(self.api.job(1, 2, 'failure'))
        self.assertEqual(self.collect((1,))['proofs'], [])

    def test_missing_or_expired_latest_artifact_does_not_fall_back(self):
        self.api.artifacts[2] = []
        self.assertEqual(self.collect()['proofs'], [])
        self.api = FakeAPI()
        self.api.artifacts[2][0]['expired'] = True
        self.assertEqual(self.collect()['proofs'], [])

    def test_dispatch_and_foreign_repository_are_rejected(self):
        self.api.event = 'workflow_dispatch'
        with self.assertRaises(ValueError):
            self.collect()
        with self.assertRaises(ValueError):
            resume.GitHub('foreign/fork')

    def test_archive_digest_traversal_symlink_and_duplicate_rejected(self):
        data, checksum = archive(manifest())
        with self.assertRaises(ValueError):
            resume.read_archive(data, 'sha256:' + '0' * 64)
        for path in ['../manifest.json', '/absolute', 'path\\escape']:
            data, checksum = archive(manifest(), path)
            with self.assertRaises(ValueError):
                resume.read_archive(data, checksum)
        stream = io.BytesIO()
        with zipfile.ZipFile(stream, 'w') as output:
            info = zipfile.ZipInfo('link')
            info.external_attr = 0o120777 << 16
            output.writestr(info, '../../outside')
            output.writestr('manifest.json', '{}')
        data = stream.getvalue()
        with self.assertRaises(ValueError):
            resume.read_archive(data, 'sha256:' + hashlib.sha256(data).hexdigest())

    def test_wrong_source_attempt_schema_identity_or_cleanup_rejected(self):
        for field, value in [('candidate_sha', 'c' * 40), ('demo_ref', 'c' * 40), ('workflow_run_id', '2'),
                             ('workflow_run_attempt', True), ('schema', 'invalid'), ('runtime', 'docker'),
                             ('id', 'static/init'), ('gate', 'init'), ('cleanup_outcome', 'failure')]:
            with self.subTest(field=field):
                with self.assertRaises(ValueError):
                    resume.validate_manifest(dict(manifest(), **{field: value}), 'atomic/static/mcp', 1,
                                             'a' * 40, 'b' * 40, 1)

    def test_all_real_manifest_families_and_exact_55_coverage(self):
        proofs = []
        for key in resume.EXPECTED:
            data = manifest(key)
            resume.validate_manifest(data, key, 1, 'a' * 40, 'b' * 40, 1)
            proofs.append({'gate': key})
        result = {'schema': 'baseharbor.pre-release.coverage/v2', 'candidate_sha': 'a' * 40,
                  'demo_ref': 'b' * 40, 'tag': 'v0.4.22', 'required': resume.EXPECTED,
                  'proofs': proofs, 'pending': {}, 'requirements_digest': resume.digest(resume.local_requirements('v0.4.22'))}
        resume.verify_coverage(result, 'a' * 40, 'b' * 40, 'v0.4.22')
        result['proofs'][-1] = result['proofs'][0]
        with self.assertRaises(ValueError):
            resume.verify_coverage(result, 'a' * 40, 'b' * 40, 'v0.4.22')

    def test_matrix_contains_only_missing_gates(self):
        result = self.collect()
        matrix = resume.matrices(result)
        self.assertNotIn('mcp', matrix['static'])
        self.assertEqual(matrix['journey'], ['docker', 'podman'])
        self.assertEqual(matrix['heavy_docker'], resume.HEAVY)

    def test_integration_matrix_preserves_required_order_and_excludes_proven_gates(self):
        required = [gate['id'] for gate in resume.local_requirements('v0.4.23')]
        missing = {key: 'unproven' for key in required}
        del missing['integration/static/public-contracts']
        matrix = resume.matrices({'required': required, 'pending': missing})
        self.assertEqual(matrix['integration'], [key for key in required
                                                if key.startswith('integration/') and key in missing])
        self.assertEqual(len(matrix['integration']), 5)


class RequirementTests(unittest.TestCase):
    def test_new_release_preserves_baseline_and_adds_required_families(self):
        baseline = {gate['id'] for gate in resume.local_requirements('v0.4.22')}
        current = {gate['id'] for gate in resume.local_requirements('v0.4.23')}
        self.assertTrue(baseline < current)
        self.assertEqual(current - baseline, {
            'integration/static/public-contracts', 'integration/static/configuration-policy',
            'integration/static/browser-terminal', 'integration/docker/remote-target',
            'integration/podman/remote-target', 'integration/static/live-console'})
        coverage = {'schema': 'baseharbor.pre-release.coverage/v2', 'candidate_sha': 'a' * 40,
                    'demo_ref': 'b' * 40, 'tag': 'v0.4.23', 'required': sorted(baseline),
                    'pending': {}, 'proofs': [{'gate': key} for key in baseline]}
        with self.assertRaises(ValueError):
            resume.verify_coverage(coverage, 'a' * 40, 'b' * 40, 'v0.4.23')

    def test_duplicate_missing_and_unknown_requirements_fail_closed(self):
        original = json.loads(pathlib.Path(resume.__file__).with_name('release-requirements.json').read_text())
        for change in ['duplicate', 'schema', 'empty', 'dependencies']:
            data = copy.deepcopy(original)
            if change == 'duplicate':
                data['gates'].append(data['gates'][0])
            elif change == 'schema':
                data['schema'] = 'untrusted'
            elif change == 'empty':
                data['gates'] = []
            else:
                del data['gates'][0]['dependencies']
            with self.subTest(change=change), self.assertRaises(ValueError):
                resume.load_requirements(json.dumps(data), 'v0.4.23')
        with self.assertRaises(ValueError):
            resume.load_requirements(json.dumps(original), 'v9.0.0')


class GitFingerprintTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        self.product, self.demo = self.root / 'product', self.root / 'demo'
        for repo in [self.product, self.demo]:
            repo.mkdir()
            subprocess.run(['git', 'init', '-q', str(repo)], check=True)
            subprocess.run(['git', '-C', str(repo), 'config', 'user.name', 'Fixture'], check=True)
            subprocess.run(['git', '-C', str(repo), 'config', 'user.email', 'fixture@example.invalid'], check=True)
        workflow = self.product / '.github/workflows/pre-release.yml'
        workflow.parent.mkdir(parents=True)
        workflow.write_text('jobs:\n  runtime_candidate:\n    runs-on: ubuntu-latest\n    steps:\n      - name: Build\n        run: docker build .\n  reference_journey:\n    runs-on: ubuntu-latest\n    steps:\n      - name: Journey\n        run: bash scripts/acceptance.sh\n')
        (self.product / 'main.go').write_text('package main\n')
        path = self.demo / 'tests/mcp/run.sh'
        path.parent.mkdir(parents=True)
        path.write_text('snapshot old\n')
        (self.demo / 'compose.yaml').write_text('services: {}\n')
        self.p = self.commit(self.product)
        self.d = self.commit(self.demo)
        self.inputs = resume.GitInputs(self.product, self.demo)

    def commit(self, repo):
        subprocess.run(['git', 'add', '.'], cwd=repo, check=True)
        subprocess.run(['git', 'commit', '-qm', 'Fixture'], cwd=repo, check=True)
        return subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=repo).decode().strip()

    def fingerprint(self, p, d, gate):
        return self.inputs.fingerprint(p, d, 'v0.4.22', gate)

    def test_mcp_only_change_invalidates_mcp_and_journey_not_other_atomic(self):
        (self.demo / 'tests/mcp/run.sh').write_text('snapshot new\n')
        changed = self.commit(self.demo)
        for gate, same in [('atomic/static/mcp', False), ('journey/docker', False), ('atomic/docker/lifecycle', True)]:
            self.assertEqual(self.fingerprint(self.p, self.d, gate) == self.fingerprint(self.p, changed, gate), same)

    def test_shared_demo_or_product_change_invalidates_every_atomic(self):
        (self.demo / 'compose.yaml').write_text('services: {changed: {}}\n')
        changed = self.commit(self.demo)
        self.assertNotEqual(self.fingerprint(self.p, self.d, 'atomic/docker/lifecycle'),
                            self.fingerprint(self.p, changed, 'atomic/docker/lifecycle'))
        (self.product / 'main.go').write_text('package changed\n')
        changed_p = self.commit(self.product)
        self.assertNotEqual(self.fingerprint(self.p, self.d, 'atomic/static/mcp'),
                            self.fingerprint(changed_p, self.d, 'atomic/static/mcp'))

    def test_scheduling_change_does_not_invalidate_but_runner_or_build_does(self):
        path = self.product / '.github/workflows/pre-release.yml'
        original = path.read_text()
        path.write_text(original.replace('    runs-on:', '    needs: source\n    runs-on:'))
        schedule = self.commit(self.product)
        key = 'atomic/docker/lifecycle'
        self.assertEqual(self.fingerprint(self.p, self.d, key), self.fingerprint(schedule, self.d, key))
        path.write_text(original.replace('ubuntu-latest', 'ubuntu-26.04'))
        runner = self.commit(self.product)
        self.assertNotEqual(self.fingerprint(self.p, self.d, key), self.fingerprint(runner, self.d, key))

    def test_verifier_auth_schema_and_build_changes_invalidate_static_proof(self):
        key = 'atomic/static/mcp'
        original = self.fingerprint(self.p, self.d, key)
        for path in ['scripts/pre-release-resume.py', 'scripts/test_pre_release_resume.py',
                     'contracts/machine/v1/operation.schema.json', 'internal/auth/auth.go',
                     '.github/workflows/release.yml', '.goreleaser.yaml']:
            file = self.product / path
            file.parent.mkdir(parents=True, exist_ok=True)
            file.write_text('changed execution input\n')
            changed = self.commit(self.product)
            with self.subTest(path=path):
                self.assertNotEqual(original, self.fingerprint(changed, self.d, key))

    def test_static_call_arguments_and_evidence_steps_are_execution_inputs(self):
        path = self.product / '.github/workflows/pre-release.yml'
        original = path.read_text()
        key = 'atomic/static/mcp'
        for suffix in [
            '  static_gates:\n    uses: ./.github/workflows/gate.yml\n    with: {auth_profile: strict}\n',
            '  static_gates:\n    steps:\n      - name: Generate adoption evidence\n        run: rewrite source identity\n',
        ]:
            path.write_text(original + suffix)
            changed = self.commit(self.product)
            self.assertNotEqual(self.fingerprint(self.p, self.d, key), self.fingerprint(changed, self.d, key))

    def test_missing_new_candidate_inventory_is_rejected(self):
        with self.assertRaises(ValueError):
            self.inputs.requirements(self.p, 'v0.4.23')

    def test_publication_handoff_allows_prose_and_rejects_workflow_auth_and_inventory(self):
        spec = importlib.util.spec_from_file_location('handoff', pathlib.Path(resume.__file__).with_name('release-handoff.py'))
        handoff = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(handoff)
        (self.product / 'CHANGELOG.md').write_text('final release date\n')
        prose = self.commit(self.product)
        self.assertEqual(handoff.validate_handoff(self.product, self.p, prose, 'v0.4.23'), ['CHANGELOG.md'])
        for path in ['.github/workflows/pre-release.yml', 'internal/auth/verifier.go',
                     'scripts/release-requirements.json', 'scripts/pre-release-resume.py']:
            file = self.product / path
            file.parent.mkdir(parents=True, exist_ok=True)
            file.write_text('unvalidated execution change\n')
            changed = self.commit(self.product)
            with self.subTest(path=path), self.assertRaises(ValueError):
                handoff.validate_handoff(self.product, self.p, changed, 'v0.4.23')


class WorkflowIntegrationTests(unittest.TestCase):
    def test_core_integration_jobs_use_exact_requirements_names_and_retained_origins(self):
        import yaml
        jobs = yaml.safe_load(pathlib.Path('.github/workflows/pre-release.yml').read_text())['jobs']
        for gate in resume.local_requirements('v0.4.23'):
            if not gate['id'].startswith('integration/') or gate['dependencies'] != ['core']:
                continue
            job_id = gate['jobs'][0]
            job = jobs[job_id]
            with self.subTest(gate=gate['id']):
                self.assertEqual(resume.key_from_job(job['name'], [gate['id']]), gate['id'])
                self.assertIn("contains(fromJSON(needs.source.outputs.integration), '" + gate['id'] + "')", job['if'])
                checkout = next(s for s in job['steps'] if s.get('uses', '').startswith('actions/checkout@'))
                self.assertEqual(checkout['with']['ref'], '${{ needs.source.outputs.candidate_sha }}')
                upload = next(s for s in job['steps'] if s.get('uses', '').startswith('actions/upload-artifact@'))
                expected = resume.artifact_name(gate['id'], '${{ github.run_id }}', '${{ github.run_attempt }}')
                self.assertEqual(upload['with']['name'], expected)
                self.assertEqual(upload['with']['retention-days'], 90)
                evidence_job = next(j for j in jobs.values() if j.get('name') == 'Evidence · Candidate Manifest')
                self.assertIn(job_id, evidence_job['needs'])

    def test_every_matrix_is_supplied_by_the_authenticated_plan(self):
        import yaml
        jobs = yaml.safe_load(pathlib.Path('.github/workflows/pre-release.yml').read_text())['jobs']
        names = {'adoption': ('adoption', 'gate'), 'static_gates': ('static', 'gate'),
                 'runtime_light_gates_docker': ('light_docker', 'gate'),
                 'runtime_light_gates_podman': ('light_podman', 'gate'),
                 'runtime_heavy_gates_docker': ('heavy_docker', 'gate'),
                 'runtime_heavy_gates_podman': ('heavy_podman', 'gate'),
                 'v0421_ha_groups_docker': ('ha_docker', 'group'),
                 'v0421_ha_groups_podman': ('ha_podman', 'group'),
                 'reference_journey': ('journey', 'runtime')}
        for job, (output, field) in names.items():
            with self.subTest(job=job):
                self.assertEqual(jobs[job]['strategy']['matrix'][field],
                                 '${{ fromJSON(needs.source.outputs.' + output + ') }}')
                self.assertIn("needs.source.outputs." + output + "_count != '0'", jobs[job]['if'])

    def test_journey_manifest_expression_actually_executes(self):
        import yaml
        jobs = yaml.safe_load(pathlib.Path('.github/workflows/pre-release.yml').read_text())['jobs']
        step = next(s for s in jobs['reference_journey']['steps'] if s['name'] == 'Generate reference journey evidence')
        import re
        values = {'needs.source.outputs.candidate_sha': 'a' * 40,
                  'needs.source.outputs.demo_ref': 'b' * 40, 'steps.journey.outcome': 'success',
                  'steps.cleanup.outcome': 'success', 'matrix.runtime': 'docker',
                  'steps.journey.outputs.duration_seconds': '42'}
        script = re.sub(r'\$\{\{\s*(.*?)\s*\}\}', lambda m: values[m[1]], step['run'])
        with tempfile.TemporaryDirectory() as root:
            import os
            env = dict(os.environ, RUNNER_TEMP=root, GITHUB_RUN_ID='1', GITHUB_RUN_ATTEMPT='1')
            subprocess.run(['bash', '-euo', 'pipefail', '-c', script], env=env, check=True)
            data = json.loads((pathlib.Path(root) / 'reference-journey-evidence/manifest.json').read_text())
            resume.validate_manifest(data, 'journey/docker', 1, 'a' * 40, 'b' * 40, 1)


if __name__ == '__main__':
    unittest.main()
