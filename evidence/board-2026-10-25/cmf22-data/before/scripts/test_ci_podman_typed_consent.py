"""Fail-closed regressions for the existing Podman native acceptance helper."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest


ROOT = Path(__file__).resolve().parent
REPO = ROOT.parent
HELPER = ROOT / "ci-podman-typed-consent.py"
WORKFLOW = REPO / ".github/workflows/podman-acceptance.yml"
spec = importlib.util.spec_from_file_location("podman_consent", HELPER)
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


def impact(plan_id="fresh-plan-987", **changes):
    value = dict(version="baseharbor.provider-impact/v1", plan_id=plan_id,
                 confirmations_required=2, destructive=False, complete=False)
    value.update(changes)
    return value


def provider_plan(plan_id="fresh-plan-987", **changes):
    return {"provider_plan": {"impact": impact(plan_id, **changes)}}


def scope(plan_id="fresh-scope-987", **changes):
    value = dict(version="baseharbor.provider-impact-scope/v1", plan_id=plan_id,
                 confirmations_required=2, destructive=False, complete=False)
    value.update(changes)
    return {"scope": value}


class PodmanTypedConsentTests(unittest.TestCase):
    def test_fresh_two_stage_consent_preserves_incomplete_inventory(self):
        self.assertEqual(
            {"plan_id": "fresh-plan-987", "first": True, "second": True, "delete_data": False},
            helper.consent_from_plan(provider_plan()))

    def test_single_stage_only_if_required_by_plan(self):
        self.assertFalse(helper.consent_from_plan(
            provider_plan(confirmations_required=1))["second"])

    def test_operation_sources_bind_to_their_own_plan_shape(self):
        cases = [
            ("provider-plan", provider_plan("apply-or-up"), "apply-or-up"),
            ("impact", {"impact": impact("destroy")}, "destroy"),
            ("scope-output", scope("down"), "down"),
        ]
        for source, document, plan_id in cases:
            with self.subTest(source=source):
                consent = helper.consent_from_plan(document, source)
                self.assertEqual(plan_id, consent["plan_id"])
                self.assertFalse(consent["delete_data"])
        with self.assertRaises(ValueError):
            helper.consent_from_plan(provider_plan(), "impact")
        with self.assertRaises(ValueError):
            helper.consent_from_plan({"impact": impact()}, "provider-plan")

    def test_missing_invalid_or_destructive_impact_blocks(self):
        invalid = [
            None, {}, {"provider_plan": {}}, {"provider_plan": {"impact": None}},
            provider_plan(plan_id=""), provider_plan(plan_id=None),
            provider_plan(confirmations_required=0),
            provider_plan(confirmations_required=3),
            provider_plan(confirmations_required=True),
            provider_plan(destructive=None), provider_plan(destructive=True),
            provider_plan(version="unknown"), provider_plan(plan_id="x" * 513),
        ]
        for candidate in invalid:
            with self.subTest(candidate=candidate):
                with self.assertRaises(ValueError):
                    helper.consent_from_plan(candidate)

    def test_scope_output_parser_accepts_only_one_emitted_plan(self):
        document = scope("down-scope")
        raw = "Achtung: review current stop.\n" + json.dumps(document) + "\n"
        self.assertEqual(document, helper._document_from_text(raw, "scope-output"))
        with self.assertRaises(ValueError):
            helper._document_from_text(raw + "unexpected", "scope-output")

    def test_helper_runs_from_application_workdir_using_checkout_root(self):
        with tempfile.TemporaryDirectory() as tmp:
            app_dir = Path(tmp) / "app"
            app_dir.mkdir()
            plan_file = Path(tmp) / "plan.json"
            plan_file.write_text(json.dumps(provider_plan("cwd-plan")), encoding="utf-8")
            env = os.environ.copy()
            env["GITHUB_WORKSPACE"] = str(REPO)
            env["APP_DIR"] = str(app_dir)
            env["PLAN_FILE"] = str(plan_file)
            completed = subprocess.run(
                ["bash", "-eu", "-o", "pipefail", "-c",
                 'cd "$APP_DIR" && python3 "$GITHUB_WORKSPACE/scripts/ci-podman-typed-consent.py" provider-plan "$PLAN_FILE"'],
                env=env, text=True, capture_output=True, check=True)
            self.assertEqual("cwd-plan", json.loads(completed.stdout)["plan_id"])

    def test_actual_lifecycle_shell_fragment_uses_fresh_consent_per_operation(self):
        workflow = WORKFLOW.read_text(encoding="utf-8")
        start = workflow.index("          # Take the impact ID from a fresh, read-only plan immediately before")
        end_marker = "          unset destroy_consent"
        end = workflow.index(end_marker, start) + len(end_marker)
        fragment = textwrap.dedent(workflow[start:end]) + "\n"
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            app_dir = tmp_path / "app"
            runner = tmp_path / "runner"
            app_dir.mkdir()
            runner.mkdir()
            state = tmp_path / "state"
            log = tmp_path / "log"
            stub = runner / "baha"
            stub.write_text("""#!/usr/bin/env python3
import json
import os
from pathlib import Path
import sys

args = sys.argv[1:]
state = Path(os.environ["STUB_STATE"])
log = Path(os.environ["STUB_LOG"])

def impact(plan_id):
    return {"version": "baseharbor.provider-impact/v1", "plan_id": plan_id,
            "confirmations_required": 2, "destructive": False, "complete": False}

def require(plan_id):
    value = json.loads(os.environ.get("BASEHARBOR_PROVIDER_CHANGE_CONSENT", ""))
    assert value == {"delete_data": False, "first": True, "plan_id": plan_id, "second": True}, (args, value)
    with log.open("a", encoding="utf-8") as handle:
        handle.write(plan_id + "\\n")

if args[:2] == ["app", "plan"]:
    count = int(state.read_text(encoding="utf-8") if state.exists() else "0")
    plan_id = "apply-plan" if count == 0 else "up-plan"
    state.write_text(str(count + 1), encoding="utf-8")
    print(json.dumps({"provider_plan": {"impact": impact(plan_id)}}))
elif args[:3] == ["--no-input", "app", "down"]:
    print("Achtung: review the current application stop.")
    print(json.dumps({"scope": {"version": "baseharbor.provider-impact-scope/v1",
        "plan_id": "down-plan", "confirmations_required": 2,
        "destructive": False, "complete": False}}))
    sys.exit(23)
elif args[:2] == ["app", "apply"]:
    require("apply-plan")
elif args[:2] == ["app", "status"]:
    print("authenticated SELECT 1 succeeded")
    print("semantic Valkey verification passed")
elif args[:2] == ["app", "doctor"]:
    print("READY")
elif args[:2] == ["app", "down"]:
    require("down-plan")
elif args[:2] == ["app", "up"]:
    require("up-plan")
elif args[:2] == ["app", "destroy"] and "-o" in args:
    print(json.dumps({"impact": impact("destroy-plan")}))
elif args[:2] == ["app", "destroy"]:
    require("destroy-plan")
else:
    raise SystemExit("unexpected stub command: " + repr(args))
""", encoding="utf-8")
            stub.chmod(0o755)
            env = os.environ.copy()
            env.update({"GITHUB_WORKSPACE": str(REPO), "RUNNER_TEMP": str(runner),
                        "STUB_STATE": str(state), "STUB_LOG": str(log)})
            completed = subprocess.run(
                ["bash", "-eu", "-o", "pipefail", "-c",
                 'cd "$1"; shift; eval "$1"', "test", str(app_dir), fragment],
                env=env, text=True, capture_output=True)
            self.assertEqual(0, completed.returncode, completed.stderr + completed.stdout)
            self.assertEqual(["apply-plan", "down-plan", "up-plan", "destroy-plan"],
                             log.read_text(encoding="utf-8").splitlines())

    def test_workflow_uses_fresh_operation_bound_consent_for_full_lifecycle(self):
        workflow = WORKFLOW.read_text(encoding="utf-8")
        helper_path = '"$GITHUB_WORKSPACE/scripts/ci-podman-typed-consent.py"'
        self.assertEqual(4, workflow.count(helper_path))
        self.assertIn('app plan podman-ci -o json > "$apply_plan_file"', workflow)
        self.assertIn('provider-plan "$apply_plan_file"', workflow)
        self.assertIn('BASEHARBOR_PROVIDER_CHANGE_CONSENT="$apply_consent" timeout 180s', workflow)
        self.assertIn('baha --no-input app down podman-ci > "$down_plan_file"', workflow)
        self.assertIn('scope-output "$down_plan_file"', workflow)
        self.assertIn('BASEHARBOR_PROVIDER_CHANGE_CONSENT="$down_consent"', workflow)
        self.assertIn('app plan podman-ci -o json > "$up_plan_file"', workflow)
        self.assertIn('provider-plan "$up_plan_file"', workflow)
        self.assertIn('BASEHARBOR_PROVIDER_CHANGE_CONSENT="$up_consent"', workflow)
        self.assertIn('app destroy podman-ci -o json > "$destroy_plan_file"', workflow)
        self.assertIn('impact "$destroy_plan_file"', workflow)
        self.assertIn('BASEHARBOR_PROVIDER_CHANGE_CONSENT="$destroy_consent"', workflow)
        self.assertIn('app destroy podman-ci --yes', workflow)
        self.assertNotIn("--skip-consent", workflow)
        self.assertNotIn('"delete_data": true', workflow)


if __name__ == "__main__":
    unittest.main()
