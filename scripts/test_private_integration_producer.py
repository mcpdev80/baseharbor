import copy
import importlib.util
import json
import pathlib
import tempfile
import unittest
from unittest.mock import patch

import private_consumer_evidence as private
from test_private_consumer_evidence import API, PIN, GATE, CORE, DEMO, resume

spec = importlib.util.spec_from_file_location('producer', pathlib.Path(__file__).with_name('pre-release-private-integration.py'))
producer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(producer)


class ProducerTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.output = pathlib.Path(self.directory.name)
        self.requirement = {'id': GATE, 'dependencies': ['core', 'connector'],
                            'schema': 'baseharbor.pre-release.integration-evidence/v1'}
        self.api = API()
        self.verifier = private.PrivateEvidenceVerifier({GATE: {'connector': copy.deepcopy(PIN)}},
                                                       self.api, resume.read_archive)

    def produce(self):
        return producer.produce(self.output, self.requirement, [self.requirement],
                                CORE, DEMO, '123', '1', self.verifier, resume)

    def test_success_reauthenticates_archive_and_exports_only_commitment(self):
        result = self.produce()
        self.assertEqual(result['consumer_commitments'], {'connector': private.commitment(PIN)})
        self.assertEqual(json.loads((self.output / 'manifest.json').read_text()), result)
        self.assertNotIn(PIN['repository'], (self.output / 'manifest.json').read_text())
        self.assertNotIn(PIN['commit'], (self.output / 'manifest.json').read_text())
        self.assertNotIn('job_id', result)

    def test_failed_rerun_deletes_stale_success_and_retains_safe_failure(self):
        self.produce()
        self.api.receipt['release_eligible'] = False
        with self.assertRaises(ValueError):
            self.produce()
        self.assertFalse((self.output / 'manifest.json').exists())
        failure = (self.output / 'failure.json').read_text()
        self.assertNotIn(PIN['repository'], failure)
        self.assertEqual(json.loads(failure)['result'], 'failure')

    def test_source_only_or_missing_private_configuration_cannot_pass(self):
        self.verifier = None
        with self.assertRaises(ValueError):
            self.produce()
        self.assertFalse((self.output / 'manifest.json').exists())

    def test_missing_live_console_role_cannot_borrow_single_consumer_proof(self):
        self.requirement['id'] = 'integration/static/live-console'
        self.requirement['dependencies'] = ['core', 'demo', 'console', 'connector']
        with self.assertRaises(ValueError):
            self.produce()

    def test_source_and_attempt_bindings_remain_mandatory(self):
        for candidate, attempt in [('branch', '1'), (CORE, '0')]:
            with self.assertRaises(ValueError):
                producer.produce(self.output, self.requirement, [self.requirement],
                                 candidate, DEMO, '123', attempt, self.verifier, resume)

    def test_secret_config_is_bounded_and_diagnostics_hide_private_content(self):
        for raw in ['private/repository', json.dumps({'schema': 'wrong'}), 'x' * (1024 * 1024 + 1)]:
            with patch.dict('os.environ', {'BASEHARBOR_PRIVATE_EVIDENCE_JSON': raw}, clear=True):
                with self.assertRaisesRegex(ValueError, '^private evidence configuration schema differs$'):
                    private.private_verifier_from_environment(resume.read_archive)

    def test_secret_config_requires_token_and_refuses_ambiguous_sources(self):
        config = json.dumps({'schema': 'baseharbor.private-evidence-pins/v1', 'gates': {GATE: {'connector': PIN}}})
        with patch.dict('os.environ', {'BASEHARBOR_PRIVATE_EVIDENCE_JSON': config}, clear=True):
            with self.assertRaisesRegex(ValueError, 'authenticated private evidence access is required'):
                private.private_verifier_from_environment(resume.read_archive)
        with patch.dict('os.environ', {'BASEHARBOR_PRIVATE_EVIDENCE_JSON': config,
                                      'BASEHARBOR_PRIVATE_EVIDENCE_FILE': '/private'}, clear=True):
            with self.assertRaisesRegex(ValueError, 'sources are ambiguous'):
                private.private_verifier_from_environment(resume.read_archive)

    def test_valid_secret_configuration_uses_separate_private_token(self):
        config = json.dumps({'schema': 'baseharbor.private-evidence-pins/v1', 'gates': {GATE: {'connector': PIN}}})
        with patch.dict('os.environ', {'BASEHARBOR_PRIVATE_EVIDENCE_JSON': config,
                                      'BASEHARBOR_PRIVATE_EVIDENCE_TOKEN': 'fixture-private-token',
                                      'GH_TOKEN': 'fixture-public-token'}, clear=True):
            verifier = private.private_verifier_from_environment(resume.read_archive)
            self.assertEqual(verifier.api.environment['GH_TOKEN'], 'fixture-private-token')
            self.assertEqual(verifier.pins[GATE]['connector'], PIN)

    def test_cli_early_rejection_removes_stale_success_before_config_or_git(self):
        (self.output / 'manifest.json').write_text('{"outcome":"success"}')
        args = ['producer', '--gate', GATE, '--candidate', 'branch', '--demo', DEMO,
                '--tag', 'v0.4.23', '--output', str(self.output)]
        with patch('sys.argv', args), self.assertRaises(ValueError):
            producer.main()
        self.assertFalse((self.output / 'manifest.json').exists())
        self.assertEqual(json.loads((self.output / 'failure.json').read_text())['result'], 'failure')

    def test_all_candidate_integration_gates_are_scheduled_and_join_final_evidence(self):
        import yaml
        root = pathlib.Path(__file__).resolve().parents[1]
        jobs = yaml.safe_load((root / '.github/workflows/pre-release.yml').read_text())['jobs']
        requirements = resume.local_requirements('v0.4.23')
        self.assertEqual(len(requirements), 63)
        for requirement in requirements:
            if not requirement['id'].startswith('integration/'):
                continue
            for job in requirement['jobs']:
                self.assertIn(job, jobs['evidence']['needs'])
                self.assertEqual(jobs[job]['name'], 'Integration · ' + requirement['id'])
                self.assertIn(requirement['id'], jobs[job]['if'])
        for job in ['source', 'evidence']:
            steps = [step for step in jobs[job]['steps'] if step.get('id') in ['resume', 'approval']]
            self.assertEqual(len(steps), 1)
            self.assertEqual(steps[0]['env']['GH_TOKEN'], '${{ github.token }}')
            self.assertEqual(steps[0]['env']['BASEHARBOR_PUBLIC_EVIDENCE_FILE'],
                             'docs/releases/v0.4.23.consumer-pins.json')


if __name__ == '__main__':
    unittest.main()
