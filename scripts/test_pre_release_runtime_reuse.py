"""Fail-closed unit tests for Pre-Release runtime evidence reuse."""
from datetime import datetime, timedelta, timezone
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location(
    "pre_release_runtime_reuse", Path(__file__).with_name("pre-release-runtime-reuse.py"))
reuse = importlib.util.module_from_spec(spec)
spec.loader.exec_module(reuse)


NOW = "2026-11-15T12:00:00Z"
RUN_AT = "2026-11-15T10:00:00Z"
ARTIFACT_AT = "2026-11-15T11:59:59Z"
MANIFEST = {
    "schema": reuse.SCHEMA,
    "status": "success",
    "run_id": "123",
    "source_run_id": "123",
    "run_created_at": RUN_AT,
    "source_run_created_at": RUN_AT,
    "tag": "v0.4.27",
    "runtime": "docker",
    "gate": "atomic/docker/data-capabilities",
    "candidate_sha": "a" * 40,
    "demo_ref": "b" * 40,
    "input_fingerprint": "fingerprint",
    "workflow_version": "workflow",
    "catalog_version": "catalog",
    "artifact_name": "pre-release-runtime-evidence-docker-data-capabilities",
    "qualified_at": RUN_AT,
}


class RuntimeReuseTests(unittest.TestCase):
    def decide(self, **overrides):
        values = dict(
            job_ok=True, manifest=MANIFEST, run_id=123, tag="v0.4.27",
            runtime="docker", gate="atomic/docker/data-capabilities",
            fingerprint="fingerprint", workflow_version="workflow",
            catalog_version="catalog", now=NOW, run_created_at=RUN_AT,
            current_run_created_at=NOW, evidence_created_at=ARTIFACT_AT,
        )
        values.update(overrides)
        return reuse.decide_reuse(**values)

    def artifact(self, **overrides):
        value = {
            "id": 99,
            "name": "pre-release-runtime-evidence-docker-data-capabilities",
            "expired": False,
            "created_at": ARTIFACT_AT,
            "expires_at": "2026-11-17T11:59:59Z",
            "size_in_bytes": 123,
            "workflow_run": {"id": 123},
        }
        value.update(overrides)
        return value

    def test_24_hour_ttl_has_inclusive_edge_and_expires_after(self):
        current = datetime(2026, 11, 15, 12, tzinfo=timezone.utc)
        exact = (current - timedelta(hours=24)).isoformat()
        stale = (current - timedelta(hours=24, microseconds=1)).isoformat()
        self.assertTrue(reuse.within_24_hours(exact, current.isoformat()))
        self.assertFalse(reuse.within_24_hours(stale, current.isoformat()))
        self.assertFalse(reuse.within_24_hours(None, current.isoformat()))
        self.assertFalse(reuse.within_24_hours("2026-11-14T12:00:00", current.isoformat()))
        self.assertFalse(reuse.within_24_hours("not-a-time", current.isoformat()))

    def test_only_successful_exact_evidence_is_reused(self):
        self.assertTrue(self.decide())
        self.assertFalse(self.decide(job_ok=False))
        self.assertFalse(self.decide(manifest=None))
        self.assertFalse(self.decide(manifest={**MANIFEST, "status": "failure"}))

    def test_exact_gate_input_fingerprint_is_required(self):
        self.assertFalse(self.decide(fingerprint="other"))

    def test_workflow_and_catalog_version_must_match(self):
        self.assertFalse(self.decide(workflow_version="changed"))
        self.assertFalse(self.decide(catalog_version="changed"))

    def test_stale_artifact_cannot_be_rescued_by_fresh_manifest_timestamp(self):
        fresh_manifest = {**MANIFEST, "qualified_at": NOW}
        self.assertFalse(self.decide(manifest=fresh_manifest,
                                     evidence_created_at="2026-11-14T11:59:59Z"))

    def test_missing_and_unparseable_timestamps_fail_closed(self):
        self.assertFalse(self.decide(evidence_created_at=""))
        self.assertFalse(self.decide(evidence_created_at="yesterday"))
        self.assertFalse(self.decide(manifest={**MANIFEST, "qualified_at": "bad"}))
        self.assertFalse(self.decide(run_created_at=""))
        self.assertFalse(self.decide(current_run_created_at=""))

    def test_artifact_identity_expiration_and_order_are_verified(self):
        expected = "pre-release-runtime-evidence-docker-data-capabilities"
        self.assertTrue(reuse.artifact_matches_run(self.artifact(), 123, expected,
                                                   NOW, NOW))
        self.assertFalse(reuse.artifact_matches_run(self.artifact(workflow_run={"id": 124}),
                                                    123, expected, NOW, NOW))
        self.assertFalse(reuse.artifact_matches_run(self.artifact(name="wrong"),
                                                    123, expected, NOW, NOW))
        self.assertFalse(reuse.artifact_matches_run(self.artifact(expires_at=None),
                                                    123, expected, NOW, NOW))
        self.assertFalse(reuse.artifact_matches_run(self.artifact(created_at="bad"),
                                                    123, expected, NOW, NOW))
        self.assertFalse(reuse.artifact_matches_run(self.artifact(created_at="2026-11-15T12:00:01Z"),
                                                    123, expected, NOW, NOW))

    def test_known_changed_path_invalidates_only_mapped_gates(self):
        impact = {
            "cmd/baha/app_machine_adapters.go": [
                "atomic/docker/data-capabilities", "atomic/podman/data-capabilities"],
        }
        affected = reuse.classify_paths(["cmd/baha/app_machine_adapters.go"], impact, [])
        self.assertEqual({"atomic/docker/data-capabilities",
                          "atomic/podman/data-capabilities"}, affected)
        self.assertNotIn("atomic/docker/guided", affected)

    def test_unknown_changed_path_invalidates_all(self):
        self.assertIsNone(reuse.classify_paths(["internal/new_unknown.go"], {}, []))
        self.assertEqual(set(), reuse.classify_paths(["docs/new.md"], {}, ["docs/**"]))

    def test_wrong_manifest_run_and_wrong_artifact_name_are_rejected(self):
        self.assertFalse(self.decide(manifest={**MANIFEST, "run_id": "999"}))
        self.assertFalse(self.decide(manifest={**MANIFEST, "artifact_name": "wrong"}))

    def test_copy_forward_manifest_can_qualify_a_later_run(self):
        # The reused gate keeps its original qualification provenance, while
        # the copied artifact is bound to the current workflow run for chaining.
        propagated = {
            **MANIFEST,
            "run_id": "456",
            "source_run_id": "123",
            "run_created_at": "2026-11-15T12:00:00Z",
            "source_run_created_at": RUN_AT,
            "source_artifact_created_at": ARTIFACT_AT,
        }
        self.assertTrue(reuse.decide_reuse(
            job_ok=True, manifest=propagated, run_id=456, tag="v0.4.27",
            runtime="docker", gate="atomic/docker/data-capabilities",
            fingerprint="fingerprint", workflow_version="workflow",
            catalog_version="catalog", now="2026-11-15T14:00:00Z",
            run_created_at="2026-11-15T12:00:00Z",
            current_run_created_at="2026-11-15T14:00:00Z",
            evidence_created_at="2026-11-15T13:59:59Z"))

    def test_run_and_artifact_ttl_are_measured_against_actual_planner_time(self):
        # A recent current run does not make an old prior artifact reusable;
        # both its workflow and artifact timestamps must still be within TTL.
        self.assertFalse(self.decide(
            now="2026-11-16T12:00:01Z",
            current_run_created_at="2026-11-16T12:00:00Z"))


if __name__ == "__main__":
    unittest.main()
