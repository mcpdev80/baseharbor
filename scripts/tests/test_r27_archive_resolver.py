import hashlib
import json
import pathlib
import tempfile
import unittest
from r27_archive_resolver import ArchiveError, resolve


class ResolverTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.directory = pathlib.Path(self.tmp.name) / 'evidence' / 'v0.4.27' / 'candidate' / 'run-1'
        (self.directory / 'payloads').mkdir(parents=True)
        self.payload = b'{"status":"ok"}'
        (self.directory / 'payloads' / 'check.json').write_bytes(self.payload)
        self.manifest = {'schema': 'baseharbor.ci-evidence-archive/v1', 'release': 'v0.4.27',
                         'candidate': {'id': 'candidate'},
                         'run': {'archive_id': 'run-1', 'id': 12345, 'attempt': 1},
                         'checks': [{'id': 'source-contracts', 'payload': 'payloads/check.json', 'bytes': len(self.payload),
                                     'sha256': hashlib.sha256(self.payload).hexdigest()}]}
        self.save()

    def save(self):
        data = json.dumps(self.manifest, sort_keys=True).encode()
        (self.directory / 'manifest.json').write_bytes(data)
        self.digest = hashlib.sha256(data).hexdigest()

    def resolve(self):
        return resolve(self.tmp.name, 'v0.4.27', 'candidate', 'run-1', self.digest)

    def test_valid_integrity_only(self):
        self.assertEqual(len(self.resolve()['checks']), 1)

    def test_tampered_payload(self):
        (self.directory / 'payloads' / 'check.json').write_bytes(b'bad')
        with self.assertRaises(ArchiveError): self.resolve()

    def test_missing_payload(self):
        (self.directory / 'payloads' / 'check.json').unlink()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_wrong_manifest_digest(self):
        with self.assertRaises(ArchiveError):
            resolve(self.tmp.name, 'v0.4.27', 'candidate', 'run-1', '0' * 64)

    def test_path_traversal(self):
        self.manifest['checks'][0]['payload'] = 'payloads/../manifest.json'
        self.save()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_duplicate_payload(self):
        self.manifest['checks'].append(dict(self.manifest['checks'][0]))
        self.save()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_symlink_payload(self):
        p = self.directory / 'payloads' / 'check.json'
        p.unlink()
        p.symlink_to(self.directory / 'manifest.json')
        with self.assertRaises(ArchiveError): self.resolve()

    def test_symlink_payload_directory(self):
        payload_dir = self.directory / 'payloads'
        (payload_dir / 'check.json').unlink()
        payload_dir.rmdir()
        payload_dir.symlink_to(self.directory, target_is_directory=True)
        with self.assertRaises(ArchiveError): self.resolve()

    def test_wrong_candidate_identity(self):
        self.manifest['candidate']['id'] = 'other'
        self.save()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_wrong_run_identity(self):
        self.manifest['run']['archive_id'] = 'run-2'
        self.save()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_wrong_attempt_type(self):
        self.manifest['run']['attempt'] = True
        self.save()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_empty_checks(self):
        self.manifest['checks'] = []
        self.save()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_duplicate_json_key(self):
        p = self.directory / 'manifest.json'
        raw = p.read_text()
        raw = raw.replace('"release": "v0.4.27"', '"release": "v0.4.27", "release": "v0.4.27"')
        p.write_text(raw)
        self.digest = hashlib.sha256(raw.encode()).hexdigest()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_duplicate_check_identity(self):
        other = dict(self.manifest['checks'][0])
        other['payload'] = 'payloads/other.json'
        (self.directory / 'payloads' / 'other.json').write_bytes(self.payload)
        self.manifest['checks'].append(other)
        self.save()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_missing_check_identity(self):
        del self.manifest['checks'][0]['id']
        self.save()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_manifest_directory_instead_of_file(self):
        p = self.directory / 'manifest.json'
        p.unlink()
        p.mkdir()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_payload_directory_instead_of_file(self):
        p = self.directory / 'payloads' / 'check.json'
        p.unlink()
        p.mkdir()
        with self.assertRaises(ArchiveError): self.resolve()

    def test_wrong_release(self):
        self.manifest['release'] = 'v0.4.26'
        self.save()
        with self.assertRaises(ArchiveError): self.resolve()


if __name__ == '__main__':
    unittest.main()
