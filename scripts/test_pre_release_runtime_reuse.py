"""Fail-closed unit tests for Pre-Release runtime evidence reuse."""
from datetime import datetime, timedelta, timezone
import http.client
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

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

    def test_seven_day_ttl_has_inclusive_edge_and_expires_after(self):
        current = datetime(2026, 11, 15, 12, tzinfo=timezone.utc)
        exact = (current - timedelta(days=7)).isoformat()
        stale = (current - timedelta(days=7, microseconds=1)).isoformat()
        self.assertTrue(reuse.within_7_days(exact, current.isoformat()))
        self.assertFalse(reuse.within_7_days(stale, current.isoformat()))
        self.assertFalse(reuse.within_7_days(None, current.isoformat()))
        self.assertFalse(reuse.within_7_days("2026-11-14T12:00:00", current.isoformat()))
        self.assertFalse(reuse.within_7_days("not-a-time", current.isoformat()))

    def test_incomplete_api_response_retries_before_parsing(self):
        with mock.patch.object(reuse.urllib.request, "urlopen", side_effect=[
                http.client.IncompleteRead(b"", 274661), io.BytesIO(b'{"ok": true}')]) as urlopen, \
                mock.patch.object(reuse.time, "sleep") as sleep:
            self.assertEqual({"ok": True}, reuse.request_json("owner/repo", "test", "token"))
        self.assertEqual(2, urlopen.call_count)
        sleep.assert_called_once_with(0.25)

    def test_exhausted_incomplete_api_retries_fail_closed_as_url_error(self):
        error = http.client.IncompleteRead(b"", 274661)
        with mock.patch.object(reuse.urllib.request, "urlopen", side_effect=[error, error, error]) as urlopen, \
                mock.patch.object(reuse.time, "sleep") as sleep:
            with self.assertRaises(reuse.urllib.error.URLError):
                reuse.request_json("owner/repo", "test", "token")
        self.assertEqual(3, urlopen.call_count)
        self.assertEqual(2, sleep.call_count)

    def test_only_successful_exact_evidence_is_reused(self):
        self.assertTrue(self.decide())
        self.assertFalse(self.decide(job_ok=False))
        self.assertFalse(self.decide(manifest=None))
        self.assertFalse(self.decide(manifest={**MANIFEST, "status": "failure"}))

    def test_release_push_is_eligible_only_for_exact_tag_branch(self):
        self.assertTrue(reuse.run_event_eligible(
            {"event": "push", "head_branch": "release/v0.4.27-current"}, "v0.4.27"))
        self.assertFalse(reuse.run_event_eligible(
            {"event": "push", "head_branch": "release/v0.4.26-current"}, "v0.4.27"))
        self.assertFalse(reuse.run_event_eligible(
            {"event": "push", "head_branch": "release/v0.4.27"}, "v0.4.27"))
        self.assertFalse(reuse.run_event_eligible(
            {"event": "push", "head_branch": "release/develop-current"}, "develop"))
        self.assertTrue(reuse.run_event_eligible(
            {"event": "workflow_dispatch", "head_branch": "main"}, "v0.4.27"))

    def test_unavailable_or_missing_catalog_evidence_is_never_reused(self):
        self.assertFalse(self.decide(
            manifest={**MANIFEST, "catalog_version": "unavailable"},
            catalog_version="unavailable"))
        self.assertFalse(self.decide(
            manifest={key: value for key, value in MANIFEST.items()
                      if key != "catalog_version"}))

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

    def test_current_app_planning_changes_map_to_their_runtime_gates(self):
        impact = json.loads(Path(__file__).with_name("pre-release-runtime-impact.json").read_text())
        affected = reuse.classify_paths([
            "cmd/baha/app_core_requirement.go",
            "cmd/baha/app_new_wizard.go",
            "cmd/baha/guided_application_read_context.go",
            "cmd/baha/app_edit_apply_test.go",
        ], impact["core"], impact["safe_core_paths"])
        self.assertEqual({
            "atomic/docker/guided", "atomic/podman/guided",
            "atomic/docker/data-capabilities", "atomic/podman/data-capabilities",
        }, affected)

    def test_unknown_changed_path_invalidates_all(self):
        self.assertIsNone(reuse.classify_paths(["internal/new_unknown.go"], {}, []))
        self.assertEqual(set(), reuse.classify_paths(["docs/new.md"], {}, ["docs/**"]))

    def test_main_pr_ci_and_reuse_policy_edits_are_safe_for_runtime_evidence(self):
        impact = json.loads(Path(__file__).with_name("pre-release-runtime-impact.json").read_text())
        changed = [
            ".github/workflows/ci.yml",
            "scripts/pre-release-runtime-impact.json",
            "scripts/pre-release-runtime-reuse.py",
        ]
        self.assertEqual(set(), reuse.classify_paths(
            changed, impact["core"], impact["safe_core_paths"]))

    def test_coreless_runtime_acceptance_fixes_invalidate_both_engine_gates(self):
        impact = json.loads(Path(__file__).with_name("pre-release-runtime-impact.json").read_text())
        expected = {"integration/docker/coreless-lifecycle",
                    "integration/podman/coreless-lifecycle"}
        for path in ("cmd/baha/app_backend_selective_runtime_test.go",
                     "cmd/baha/app_backend_shared_runtime_test.go",
                     "cmd/baha/app_application_backend_runtime_test.go",
                     "cmd/baha/app_capability_runtime_test.go",
                     "cmd/baha/app_https_selective_runtime_test.go",
                     "cmd/baha/application_otlp_handoff_runtime_test.go",
                     "cmd/baha/app_coreless_runtime_test.go",
                     "cmd/baha/app_identity_selective_runtime_test.go",
                     "cmd/baha/app_local_stdio_runtime_test.go"):
            self.assertEqual(expected, set(impact["core"][path]))

    def test_openbao_manager_login_changes_invalidate_recovery_on_both_engines(self):
        impact = json.loads(Path(__file__).with_name("pre-release-runtime-impact.json").read_text())
        expected = {"atomic/docker/control-plane-recovery",
                    "atomic/podman/control-plane-recovery"}
        for path in ("internal/openbao/bootstrap_auth.go",
                     "internal/openbao/bootstrap_auth_test.go"):
            self.assertEqual(expected, set(impact["core"][path]))

    def test_demo_consent_fix_invalidates_affected_gate_families_only(self):
        impact = json.loads(Path(__file__).with_name("pre-release-runtime-impact.json").read_text())
        for path in ("tests/guided/drive.py", "tests/guided/new_edit.py"):
            self.assertEqual({"atomic/docker/guided", "atomic/podman/guided",
                              "atomic/docker/data-capabilities",
                              "atomic/podman/data-capabilities"},
                             set(impact["demo"][path]))
        self.assertEqual([], impact["demo"]["tests/guided/new_edit_helpers_test.py"])

    def test_legacy_green_evidence_survives_policy_only_changes_but_not_executor_edits(self):
        impact = {
            "execution_inputs": [".github/workflows/pre-release.yml",
                                 ".github/workflows/pre-release-runtime-suite.yml",
                                 "scripts/pre-release-check-plan.py",
                                 "scripts/release-requirements.json"],
            "core": {}, "demo": {}, "demo_gate_directories": {"atomic/docker/": "tests/"},
            "safe_core_paths": [".github/workflows/ci.yml",
                                "scripts/pre-release-runtime-impact.json",
                                "scripts/pre-release-runtime-reuse.py"],
            "demo_catalog_inputs": ["tests/gate-resources.json"],
            "safe_demo_paths": [],
        }
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            core = root / "core"
            core.mkdir()
            subprocess.run(["git", "init", "-q", str(core)], check=True)
            subprocess.run(["git", "-C", str(core), "config", "user.email", "test@example.invalid"], check=True)
            subprocess.run(["git", "-C", str(core), "config", "user.name", "Test"], check=True)
            demo_bare = root / "demo.git"
            subprocess.run(["git", "init", "--bare", "-q", str(demo_bare)], check=True)
            demo_work = root / "demo-work"
            subprocess.run(["git", "init", "-q", str(demo_work)], check=True)
            subprocess.run(["git", "-C", str(demo_work), "remote", "add", "origin", str(demo_bare)], check=True)
            subprocess.run(["git", "-C", str(demo_work), "config", "user.email", "test@example.invalid"], check=True)
            subprocess.run(["git", "-C", str(demo_work), "config", "user.name", "Test"], check=True)
            (demo_work / "tests/gate").mkdir(parents=True)
            (demo_work / "tests/gate/gate.py").write_text("gate\n")
            (demo_work / "tests/gate-resources.json").write_text("{}\n")
            subprocess.run(["git", "-C", str(demo_work), "add", "."], check=True)
            subprocess.run(["git", "-C", str(demo_work), "commit", "-qm", "demo"], check=True)
            subprocess.run(["git", "-C", str(demo_work), "push", "-q", "origin", "HEAD:main"], check=True)
            demo_sha = subprocess.check_output(["git", "-C", str(demo_work), "rev-parse", "HEAD"], text=True).strip()
            legacy_impact = {
                "workflow_inputs": impact["execution_inputs"][:3] + [
                    "scripts/pre-release-runtime-impact.json",
                    "scripts/pre-release-runtime-reuse.py",
                    impact["execution_inputs"][3]],
                "demo_catalog_inputs": ["tests/gate-resources.json"],
                "demo_gate_directories": impact["demo_gate_directories"],
            "core": {}, "demo": {"tests/gate-resources.json": []},
            }
            (core / ".github/workflows").mkdir(parents=True)
            (core / "scripts").mkdir()
            (core / "scripts/pre-release-runtime-impact.json").write_text(json.dumps(legacy_impact))
            for path in impact["execution_inputs"]:
                destination = core / path
                destination.parent.mkdir(parents=True, exist_ok=True)
                if path == ".github/workflows/pre-release.yml":
                    destination.write_text("'on':\n  push:\n    branches:\n    - release-candidate/**\n    - validation/pre-release/**\n")
                elif path == ".github/workflows/pre-release-runtime-suite.yml":
                    destination.write_text("name: runtime suite\nretention-days: 1\n")
                else:
                    destination.write_text(path + "\n")
            (core / "scripts/pre-release-runtime-reuse.py").write_text("old policy\n")
            subprocess.run(["git", "-C", str(core), "add", "."], check=True)
            subprocess.run(["git", "-C", str(core), "commit", "-qm", "old candidate"], check=True)
            old_core = subprocess.check_output(["git", "-C", str(core), "rev-parse", "HEAD"], text=True).strip()
            old_workflow = reuse.canonical_hash(reuse.tree_fingerprint(old_core, legacy_impact["workflow_inputs"], cwd=core))
            old_catalog = reuse.canonical_hash(reuse.tree_fingerprint(demo_sha, legacy_impact["demo_catalog_inputs"], cwd=demo_work))
            gate = "atomic/docker/gate"
            old_fingerprint = reuse.canonical_hash({
                "tag": "v0.4.27", "gate": gate, "workflow_version": old_workflow,
                "catalog_version": old_catalog,
                "core_inputs": reuse.tree_fingerprint(old_core, legacy_impact["workflow_inputs"], cwd=core),
                "demo_inputs": reuse.tree_fingerprint(demo_sha, ["tests/gate/gate.py", "tests/gate-resources.json"], cwd=demo_work),
            })
            manifest = {"candidate_sha": old_core, "demo_ref": demo_sha,
                        "workflow_version": old_workflow, "catalog_version": old_catalog,
                        "input_fingerprint": old_fingerprint}
            (core / ".github/workflows/ci.yml").write_text("main PR route\n")
            (core / "scripts/pre-release-runtime-impact.json").write_text("new impact policy\n")
            (core / "scripts/pre-release-runtime-reuse.py").write_text("new reuse policy\n")
            (core / ".github/workflows/pre-release-runtime-suite.yml").write_text(
                "name: runtime suite\nretention-days: 7\n")
            workflow_path = core / ".github/workflows/pre-release.yml"
            workflow_path.write_text(workflow_path.read_text().replace(
                "    - release-candidate/**\n", "    - release-candidate/**\n    - release/v*\n"))
            subprocess.run(["git", "-C", str(core), "add", "."], check=True)
            subprocess.run(["git", "-C", str(core), "commit", "-qm", "policy changes"], check=True)
            current_core = subprocess.check_output(["git", "-C", str(core), "rev-parse", "HEAD"], text=True).strip()
            manifest.update({"current_candidate_sha": current_core, "current_demo_sha": demo_sha})
            self.assertEqual((old_fingerprint, old_workflow, old_catalog),
                             reuse.legacy_evidence_identity(
                                 manifest, "v0.4.27", gate, impact, current_core=current_core,
                                 current_demo=demo_sha, core_cwd=core, demo_cwd=demo_work))
            (core / ".github/workflows/pre-release-runtime-suite.yml").write_text("execution changed\n")
            subprocess.run(["git", "-C", str(core), "add", "."], check=True)
            subprocess.run(["git", "-C", str(core), "commit", "-qm", "executor change"], check=True)
            changed_core = subprocess.check_output(["git", "-C", str(core), "rev-parse", "HEAD"], text=True).strip()
            self.assertIsNone(reuse.legacy_evidence_identity(
                manifest, "v0.4.27", gate, impact, current_core=changed_core,
                current_demo=demo_sha, core_cwd=core, demo_cwd=demo_work))

    def test_execution_contract_digest_excludes_reuse_policy_files(self):
        impact = json.loads(Path(__file__).with_name("pre-release-runtime-impact.json").read_text())
        self.assertIn(".github/workflows/pre-release.yml", impact["execution_inputs"])
        self.assertIn(".github/workflows/pre-release-runtime-suite.yml", impact["execution_inputs"])
        self.assertIn("scripts/pre-release-check-plan.py", impact["execution_inputs"])
        self.assertEqual(["atomic/static/ci-fast-check"], impact["core"]["scripts/ci-fast-check.sh"])
        self.assertEqual(["atomic/static/ci-fast-check"], impact["core"]["scripts/test_pre_release_check_plan.py"])
        self.assertEqual(["atomic/static/ci-fast-check"], impact["core"]["scripts/test_ci_v0427_gate_catalog.py"])
        self.assertNotIn(".github/workflows/ci.yml", impact["execution_inputs"])
        self.assertNotIn("scripts/pre-release-runtime-impact.json", impact["execution_inputs"])
        self.assertNotIn("scripts/pre-release-runtime-reuse.py", impact["execution_inputs"])
        self.assertNotIn("workflow_inputs", impact)

    def test_main_target_release_ci_uses_its_own_lane_and_requires_full_push_run(self):
        root = Path(__file__).resolve().parent.parent
        ci = (root / ".github/workflows/ci.yml").read_text()
        prerelease = (root / ".github/workflows/pre-release.yml").read_text()
        self.assertIn("name: PR · Main release source", ci)
        self.assertIn("name: PR · Main release Go · ${{ matrix.shard }}", ci)
        self.assertIn("github.base_ref == 'main'", ci)
        self.assertIn("github.base_ref == 'develop'", ci)
        self.assertIn(".event == \"push\" and .head_sha == $sha", ci)
        self.assertIn("Greenfield · Docker + Podman · All built-in stacks", ci)
        self.assertIn("- release/v*", prerelease)
        self.assertIn("ref: ${{ inputs.ref || github.sha }}", prerelease)
        self.assertIn("ONLY_GATES: ${{ inputs.only_gates }}", prerelease)

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
            now="2026-11-22T12:00:01Z",
            current_run_created_at="2026-11-22T12:00:00Z"))


if __name__ == "__main__":
    unittest.main()
