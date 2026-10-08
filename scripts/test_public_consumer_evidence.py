import json
import pathlib
import tempfile
import unittest
import hashlib
from unittest.mock import patch

import private_consumer_evidence as evidence
import private_evidence_access_preflight as preflight
from test_private_evidence_access_preflight import API, pins


class PublicEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.path = pathlib.Path(self.temp.name) / 'pins.json'
        self.path.write_text(json.dumps({'schema': 'baseharbor.consumer-evidence-pins/v1', 'gates': {}}))
        self.path.chmod(0o644)

    def test_public_config_uses_standard_token_and_still_requires_authentication(self):
        env = {'BASEHARBOR_PUBLIC_EVIDENCE_FILE': str(self.path), 'GH_TOKEN': 'fixture-standard-token'}
        with patch.dict('os.environ', env, clear=True):
            verifier = evidence.private_verifier_from_environment(None)
            self.assertTrue(verifier.api.public)
            self.assertEqual(verifier.api.environment['GH_TOKEN'], env['GH_TOKEN'])
        env.pop('GH_TOKEN')
        with patch.dict('os.environ', env, clear=True), self.assertRaises(ValueError):
            evidence.private_verifier_from_environment(None)

    def test_writable_symlink_and_ambiguous_public_config_are_denied(self):
        self.path.chmod(0o666)
        with self.assertRaises(ValueError):
            evidence.load_private_pins(self.path, public=True)
        self.path.chmod(0o644)
        link = self.path.with_name('link.json')
        link.symlink_to(self.path)
        with self.assertRaises(OSError):
            evidence.load_private_pins(link, public=True)
        with patch.dict('os.environ', {'BASEHARBOR_PUBLIC_EVIDENCE_FILE': str(self.path),
                                     'BASEHARBOR_PRIVATE_EVIDENCE_JSON': '{}'}, clear=True):
            with self.assertRaisesRegex(ValueError, 'sources are ambiguous'):
                evidence.private_verifier_from_environment(None)

    def test_visibility_is_checked_before_artifact_access_and_never_grants_foreign_access(self):
        repo = evidence.REPOSITORIES['connector']
        with patch.dict('os.environ', {'GH_TOKEN': 'fixture-standard-token'}, clear=True):
            for metadata in [{'full_name': repo, 'private': True, 'visibility': 'private'},
                             {'full_name': 'foreign/repo', 'private': False, 'visibility': 'public'},
                             {'full_name': repo, 'private': False}]:
                api = evidence.PrivateGitHub(public=True)
                with patch('subprocess.check_output', return_value=json.dumps(metadata).encode()) as command:
                    with self.assertRaises(ValueError):
                        api.archive(repo, {'id': 1})
                    self.assertEqual(command.call_count, 1)
            api = evidence.PrivateGitHub(public=True)
            with patch('subprocess.check_output') as command:
                with self.assertRaises(ValueError):
                    api.archive('foreign/repo', {'id': 1})
                command.assert_not_called()

    def test_verified_public_archive_keeps_exact_repository_path_and_token(self):
        repo = evidence.REPOSITORIES['connector']
        with patch.dict('os.environ', {'GH_TOKEN': 'fixture-standard-token'}, clear=True):
            api = evidence.PrivateGitHub(public=True)
            metadata = json.dumps({'full_name': repo, 'private': False, 'visibility': 'public'}).encode()
            with patch('subprocess.check_output', side_effect=[metadata, b'archive', b'again']) as command:
                self.assertEqual(api.archive(repo, {'id': 123}), b'archive')
                self.assertEqual(api.archive(repo, {'id': 124}), b'again')
                self.assertEqual(command.call_count, 3)
                self.assertEqual(command.call_args.args[0][-1], 'repos/' + repo + '/actions/artifacts/124/zip')
                self.assertEqual(command.call_args.kwargs['env']['GH_TOKEN'], 'fixture-standard-token')

    def test_public_access_requires_exact_source_archive_and_matching_digest(self):
        class PublicAPI(API):
            public = True
            bad = False

            def pages(self, repository, path, field):
                role = 'connector' if repository.endswith('node-connector') else 'console'
                return [{'id': 123, 'expired': False,
                         'workflow_run': {'head_sha': ('a' if role == 'connector' else 'b') * 40},
                         'digest': 'sha256:' + hashlib.sha256(b'PK-fixture').hexdigest()}]

            def archive(self, repository, artifact):
                return b'changed' if self.bad else b'PK-fixture'

        api = PublicAPI()
        self.assertFalse(preflight.check_access(pins(), api)['release_approved'])
        api.bad = True
        with self.assertRaises(ValueError):
            preflight.check_access(pins(), api)


if __name__ == '__main__':
    unittest.main()
