import hashlib
import tempfile
import unittest
from r27_archive_publisher import publish
from r27_archive_resolver import ArchiveError, resolve


class PublisherTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.payload = b'{"result":"passed"}'
        self.name = "payloads/check.json"
        self.manifest = {
            "schema": "baseharbor.ci-evidence-archive/v1",
            "release": "v0.4.27",
            "candidate": {"id": "candidate"},
            "run": {"archive_id": "run-1", "id": 7, "attempt": 1},
            "checks": [{"id": "test", "payload": self.name, "bytes": len(self.payload),
                        "sha256": hashlib.sha256(self.payload).hexdigest()}],
        }

    def publish(self):
        return publish(self.temp.name, "v0.4.27", "candidate", "run-1",
                       self.manifest, {self.name: self.payload})

    def test_roundtrip(self):
        digest = self.publish()
        self.assertEqual(resolve(self.temp.name, "v0.4.27", "candidate", "run-1", digest), self.manifest)

    def test_idempotent_retry(self):
        self.assertEqual(self.publish(), self.publish())

    def test_conflicting_retry(self):
        self.publish()
        self.payload = b"changed"
        self.manifest["checks"][0].update(bytes=len(self.payload), sha256=hashlib.sha256(self.payload).hexdigest())
        with self.assertRaises(ArchiveError):
            self.publish()

    def test_missing_payload(self):
        with self.assertRaises(ArchiveError):
            publish(self.temp.name, "v0.4.27", "candidate", "run-1", self.manifest, {})

    def test_wrong_candidate(self):
        self.manifest["candidate"]["id"] = "other"
        with self.assertRaises(ArchiveError):
            self.publish()

    def test_wrong_digest(self):
        self.manifest["checks"][0]["sha256"] = "0" * 64
        with self.assertRaises(ArchiveError):
            self.publish()

    def test_traversal(self):
        self.manifest["checks"][0]["payload"] = "payloads/../check.json"
        with self.assertRaises(ArchiveError):
            publish(self.temp.name, "v0.4.27", "candidate", "run-1",
                    self.manifest, {"payloads/../check.json": self.payload})

    def test_duplicate_check_id(self):
        self.manifest["checks"].append(dict(self.manifest["checks"][0]))
        with self.assertRaises(ArchiveError):
            self.publish()

    def test_missing_check_id(self):
        del self.manifest["checks"][0]["id"]
        with self.assertRaises(ArchiveError):
            self.publish()

    def test_existing_manifest_symlink(self):
        self.publish()
        from pathlib import Path
        path = Path(self.temp.name) / "evidence" / "v0.4.27" / "candidate" / "run-1" / "manifest.json"
        path.unlink()
        path.symlink_to(Path(self.temp.name) / "external.json")
        with self.assertRaises(ArchiveError):
            self.publish()

    def test_extra_payload(self):
        with self.assertRaises(ArchiveError):
            publish(self.temp.name, "v0.4.27", "candidate", "run-1",
                    self.manifest, {self.name: self.payload, "payloads/extra.json": b"x"})


if __name__ == "__main__":
    unittest.main()
