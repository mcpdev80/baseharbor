import importlib.util
import json
import pathlib
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('journey', pathlib.Path(__file__).with_name('verify-journey-evidence.py'))
journey = importlib.util.module_from_spec(spec)
spec.loader.exec_module(journey)


class JourneyEvidenceTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = pathlib.Path(self.directory.name)
        self.paths = {}
        for runtime in ('docker', 'podman'):
            path = self.root / f'pre-release-reference-journey-evidence-{runtime}-123-1' / 'manifest.json'
            path.parent.mkdir()
            path.write_text(json.dumps(dict(schema='baseharbor.pre-release.reference-journey/v1', candidate_sha='a'*40,
                demo_ref='b'*40, runtime=runtime, workflow_run_id=123, workflow_run_attempt=1,
                outcome='success', cleanup_outcome='success')))
            self.paths[runtime] = path

    def verify(self):
        return journey.verify(self.root, 'a'*40, 'b'*40, 123)

    def test_both_runtimes_required_and_nested_demo_manifest_ignored(self):
        nested = self.paths['docker'].parent / 'demo' / 'manifest.json'
        nested.parent.mkdir()
        nested.write_text('{}')
        self.assertEqual([x['runtime'] for x in self.verify()], ['docker', 'podman'])
        self.paths['podman'].unlink()
        with self.assertRaises(ValueError):
            self.verify()

    def test_wrong_identity_failure_cleanup_and_duplicate_rejected(self):
        for field, value in [('candidate_sha', 'c'*40), ('demo_ref', 'c'*40), ('workflow_run_id', 124),
                             ('outcome', 'failure'), ('cleanup_outcome', 'failure'), ('runtime', 'docker'),
                             ('workflow_run_attempt', 2), ('workflow_run_attempt', True)]:
            with self.subTest(field=field, value=value):
                path = self.paths['podman']
                original = path.read_text()
                mutated = json.loads(original)
                mutated[field] = value
                path.write_text(json.dumps(mutated))
                with self.assertRaises(ValueError):
                    self.verify()
                path.write_text(original)

    def test_symlink_rejected(self):
        path = self.paths['podman']
        original = self.root / 'original.json'
        path.rename(original)
        path.symlink_to(original)
        with self.assertRaises(ValueError):
            self.verify()


if __name__ == '__main__':
    unittest.main()
