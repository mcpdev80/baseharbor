import copy
import hashlib
import importlib.util
import io
import json
import pathlib
import tempfile
import unittest
import zipfile

import private_consumer_evidence as private

spec = importlib.util.spec_from_file_location('resume', pathlib.Path(__file__).with_name('pre-release-resume.py'))
resume = importlib.util.module_from_spec(spec)
spec.loader.exec_module(resume)

GATE = 'integration/docker/remote-target'
REPO = private.REPOSITORIES['connector']
SOURCE, CORE, DEMO = 'a' * 40, 'b' * 40, 'c' * 40
PIN = {'repository': REPO, 'commit': SOURCE, 'workflow': private.WORKFLOW}


class API:
    def __init__(self):
        self.runs = [{'id': 10, 'head_sha': SOURCE, 'event': 'push', 'path': private.WORKFLOW,
                      'repository': {'full_name': REPO}, 'head_repository': {'full_name': REPO},
                      'run_attempt': 1, 'status': 'completed', 'conclusion': 'failure'}]
        self.jobs = {10: [{'id': 20, 'head_sha': SOURCE, 'run_attempt': 1, 'status': 'completed',
                          'conclusion': 'success', 'name': 'Integration · ' + GATE}]}
        self.receipt = {'schema': private.SCHEMA, 'repository': REPO, 'consumer_commit': SOURCE,
                        'core_commit': CORE, 'demo_commit': DEMO, 'role': 'connector', 'gate': GATE,
                        'workflow_run_id': '10', 'workflow_run_attempt': 1, 'job_id': 20,
                        'result': 'success', 'cleanup_result': 'success', 'release_eligible': True,
                        'production_authority': True, 'build_sha256': 'd' * 64,
                        'qualifications': dict.fromkeys(private.QUALIFICATIONS[GATE], True)}
        self.artifact_changes = {}
        self.bytes = b''

    def pages(self, repository, path, field):
        if repository != REPO:
            raise AssertionError('untrusted repository contacted')
        if field == 'workflow_runs':
            return self.runs
        run = int(path.split('/')[2])
        if field == 'jobs':
            return self.jobs.get(run, [])
        stream = io.BytesIO()
        with zipfile.ZipFile(stream, 'w') as archive:
            archive.writestr('manifest.json', json.dumps(self.receipt))
        self.bytes = stream.getvalue()
        return [dict({'id': 30, 'name': 'private-integration-' + GATE.replace('/', '-') + '-10-1',
                      'expired': False, 'digest': 'sha256:' + hashlib.sha256(self.bytes).hexdigest(),
                      'workflow_run': {'id': 10, 'head_sha': SOURCE}}, **self.artifact_changes)]

    def archive(self, repository, artifact):
        return self.bytes


class PrivateEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.api = API()
        self.pins = {GATE: {'connector': copy.deepcopy(PIN)}}
        self.manifest = {'consumer_commitments': {'connector': private.commitment(PIN)}}
        self.requirement = {'id': GATE, 'dependencies': ['core', 'connector']}

    def verify(self):
        return private.PrivateEvidenceVerifier(self.pins, self.api, resume.read_archive).verify(
            self.manifest, self.requirement, CORE, DEMO)

    def test_authenticated_child_succeeds_without_exporting_private_origins(self):
        before = copy.deepcopy(self.manifest)
        self.assertIsNone(self.verify())
        self.assertEqual(self.manifest, before)
        public = json.dumps(self.manifest)
        self.assertNotIn(REPO, public)
        self.assertNotIn(SOURCE, public)

    def test_latest_failure_cancel_pending_or_missing_job_blocks_old_success(self):
        for status in ['failure', 'cancelled', 'skipped', None]:
            with self.subTest(status=status):
                self.api = API()
                newer = dict(self.api.runs[0], id=11, conclusion=status)
                self.api.runs.append(newer)
                self.api.jobs[11] = [dict(self.api.jobs[10][0], id=21, conclusion=status)]
                with self.assertRaises(ValueError):
                    self.verify()
        self.api = API()
        self.api.runs.append(dict(self.api.runs[0], id=11, status='in_progress'))
        with self.assertRaises(ValueError):
            self.verify()

    def test_newest_run_attempt_cannot_borrow_an_older_job_or_artifact(self):
        self.api.runs[0]['run_attempt'] = 2
        with self.assertRaises(ValueError):
            self.verify()
        self.api.jobs[10][0]['run_attempt'] = 2
        with self.assertRaises(ValueError):
            self.verify()

    def test_dispatch_fork_workflow_and_source_cannot_supply_trusted_origin(self):
        changes = [{'event': 'workflow_dispatch'}, {'head_sha': 'e' * 40},
                   {'path': '.github/workflows/ci.yml'},
                   {'head_repository': {'full_name': 'foreign/fork'}}]
        for change in changes:
            with self.subTest(change=change):
                self.api = API()
                self.api.runs[0].update(change)
                with self.assertRaises(ValueError):
                    self.verify()

    def test_receipt_partial_scope_test_ca_wrong_pins_and_cleanup_fail_closed(self):
        changes = {'release_eligible': False, 'production_authority': False,
                   'core_commit': 'e' * 40, 'demo_commit': 'e' * 40,
                   'consumer_commit': 'e' * 40, 'cleanup_result': 'failure',
                   'workflow_run_attempt': True, 'job_id': 21, 'result': 'failure',
                   'qualifications': {}, 'build_sha256': None}
        for field, value in changes.items():
            with self.subTest(field=field):
                self.api = API()
                self.api.receipt[field] = value
                with self.assertRaises(ValueError):
                    self.verify()

    def test_archive_digest_expiry_and_source_are_authenticated(self):
        for change in [{'digest': 'sha256:' + '0' * 64}, {'expired': True},
                       {'workflow_run': {'id': 11, 'head_sha': SOURCE}}]:
            with self.subTest(change=change):
                self.api = API()
                self.api.artifact_changes = change
                with self.assertRaises(ValueError):
                    self.verify()

    def test_role_and_source_commitment_cannot_be_substituted(self):
        self.manifest['consumer_commitments']['connector'] = 'sha256:' + '0' * 64
        with self.assertRaises(ValueError):
            self.verify()
        self.manifest = {'consumer_commitments': {'connector': private.commitment(PIN)}}
        self.pins[GATE]['connector']['repository'] = 'foreign/fork'
        with self.assertRaises(ValueError):
            self.verify()
        self.pins[GATE] = {'connector': PIN}
        self.requirement['dependencies'].append('console')
        with self.assertRaises(ValueError):
            self.verify()

    def test_private_configuration_rejects_symlink_and_broad_permissions(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / 'pins.json'
            path.write_text(json.dumps({'schema': 'baseharbor.private-evidence-pins/v1', 'gates': self.pins}))
            path.chmod(0o600)
            self.assertEqual(private.load_private_pins(path), self.pins)
            link = path.with_suffix('.link')
            link.symlink_to(path)
            with self.assertRaises(OSError):
                private.load_private_pins(link)
            path.chmod(0o644)
            with self.assertRaises(ValueError):
                private.load_private_pins(path)

    def test_public_collection_reauthenticates_private_origin_and_requires_configuration(self):
        requirement = dict(self.requirement, schema=resume.SCHEMAS['integration'],
                           jobs=['integration_remote_target_docker'], required_for=['v0.4.23'])

        class Inputs:
            def ensure(self, *args):
                pass

            def pin(self, *args):
                return DEMO

            def requirements(self, *args):
                return [requirement]

            def fingerprint(self, *args):
                return 'same-inputs'

        manifest = {**self.manifest, 'schema': requirement['schema'], 'candidate_sha': CORE,
                    'demo_ref': DEMO, 'workflow_run_id': '10', 'workflow_run_attempt': 1,
                    'outcome': 'success', 'cleanup_outcome': 'success', 'id': 'docker/remote-target',
                    'runtime': 'docker', 'gate': 'remote-target',
                    'requirements_digest': resume.digest([requirement])}
        stream = io.BytesIO()
        with zipfile.ZipFile(stream, 'w') as archive:
            archive.writestr('manifest.json', json.dumps(manifest))
        data = stream.getvalue()

        class PublicAPI:
            repository = 'mcpdev80/baseharbor'

            def get(self, path):
                return {'path': '.github/workflows/pre-release.yml', 'event': 'push',
                        'head_sha': CORE, 'status': 'completed',
                        'repository': {'full_name': self.repository},
                        'head_repository': {'full_name': self.repository}}

            def pages(self, path, field):
                if field == 'jobs':
                    return [{'id': 40, 'name': 'Integration · ' + GATE, 'head_sha': CORE,
                             'run_attempt': 1, 'status': 'completed', 'conclusion': 'success'}]
                return [{'id': 50, 'name': resume.artifact_name(GATE, 10, 1), 'expired': False,
                         'digest': 'sha256:' + hashlib.sha256(data).hexdigest(),
                         'workflow_run': {'id': 10, 'head_sha': CORE}}]

            def archive(self, artifact):
                return data

        with tempfile.TemporaryDirectory() as directory:
            output = pathlib.Path(directory)
            with self.assertRaisesRegex(ValueError, 'configuration is required'):
                resume.collect(PublicAPI(), Inputs(), CORE, DEMO, 'v0.4.23', [10], output)
            verifier = private.PrivateEvidenceVerifier(self.pins, self.api, resume.read_archive)
            coverage = resume.collect(PublicAPI(), Inputs(), CORE, DEMO, 'v0.4.23', [10], output,
                                      private_verifier=verifier)
            self.assertEqual(coverage['pending'], {})
            self.assertEqual(len(coverage['proofs']), 1)
            self.assertNotIn(REPO, json.dumps(coverage))
            self.assertNotIn(SOURCE, json.dumps(coverage))
            self.assertEqual({path.name for path in output.iterdir()}, {'50.zip', '50'})
            self.api.receipt['release_eligible'] = False
            with self.assertRaises(ValueError):
                resume.collect(PublicAPI(), Inputs(), CORE, DEMO, 'v0.4.23', [10], output,
                               private_verifier=verifier)


if __name__ == '__main__':
    unittest.main()
