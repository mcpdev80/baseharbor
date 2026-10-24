import hashlib
import tempfile
import unittest
import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
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

    def test_invalid_run_does_not_stage_files(self):
        self.manifest["run"]["attempt"] = True
        with self.assertRaises(ArchiveError):
            self.publish()
        self.assertFalse((Path(self.temp.name) / "evidence").exists())

    def test_conflicting_committed_manifest_does_not_stage_new_payload(self):
        self.publish()
        self.manifest["checks"][0]["id"] = "renamed"
        self.manifest["checks"][0]["payload"] = "payloads/new.json"
        with self.assertRaises(ArchiveError):
            publish(self.temp.name, "v0.4.27", "candidate", "run-1",
                    self.manifest, {"payloads/new.json": self.payload})
        directory = Path(self.temp.name) / "evidence" / "v0.4.27" / "candidate" / "run-1"
        self.assertFalse((directory / "payloads" / "new.json").exists())

    def test_boolean_byte_count_rejected_before_staging(self):
        self.payload = b"x"
        self.manifest["checks"][0]["bytes"] = True
        self.manifest["checks"][0]["sha256"] = hashlib.sha256(self.payload).hexdigest()
        with self.assertRaisesRegex(ArchiveError, "payload size or digest mismatch"):
            self.publish()
        self.assertFalse((Path(self.temp.name) / "evidence").exists())

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

    def test_symlink_root_is_rejected_before_staging(self):
        alias = Path(self.temp.name) / "root-alias"
        alias.symlink_to(Path(self.temp.name), target_is_directory=True)
        with self.assertRaises(ArchiveError):
            publish(alias, "v0.4.27", "candidate", "run-1",
                    self.manifest, {self.name: self.payload})

    def test_symlink_ancestor_is_rejected_before_staging(self):
        parent = Path(self.temp.name) / "evidence"
        external = Path(self.temp.name) / "outside"
        external.mkdir()
        parent.symlink_to(external, target_is_directory=True)
        with self.assertRaises(ArchiveError):
            self.publish()
        self.assertEqual(list(external.iterdir()), [])

    def test_symlink_payload_directory_is_rejected(self):
        self.publish()
        directory = Path(self.temp.name) / "evidence" / "v0.4.27" / "candidate" / "run-1"
        payload_dir = directory / "payloads"
        payload_dir.rename(directory / "saved-payloads")
        payload_dir.symlink_to(directory / "saved-payloads", target_is_directory=True)
        with self.assertRaises(ArchiveError):
            self.publish()

    def test_failed_fsync_does_not_unlink_by_name(self):
        from unittest.mock import patch
        with patch("r27_archive_publisher.os.fsync",
                   side_effect=OSError("injected write durability failure")):
            with self.assertRaisesRegex(OSError, "injected write durability failure"):
                self.publish()
        staged = (Path(self.temp.name) / "evidence" / "v0.4.27" /
                  "candidate" / "run-1" / "payloads" / "check.json")
        self.assertTrue(staged.exists(), "failed write must not unlink a replaced entry")
        self.assertFalse((staged.parent.parent / "manifest.json").exists(),
                         "failed staging must never publish the manifest")

    def test_extra_payload(self):
        with self.assertRaises(ArchiveError):
            publish(self.temp.name, "v0.4.27", "candidate", "run-1",
                    self.manifest, {self.name: self.payload, "payloads/extra.json": b"x"})


if __name__ == "__main__":
    unittest.main()
