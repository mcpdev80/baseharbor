#!/usr/bin/env python3
"""Exercise the published CI examples against a controlled baha fixture."""

from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import yaml


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts/ci-consumption-example.sh"


class CIConsumptionExamples(unittest.TestCase):
    def load_yaml(self, path: str) -> dict:
        return yaml.load((ROOT / path).read_text(), Loader=yaml.BaseLoader)

    def test_workflows_are_manual_and_share_the_tested_script(self) -> None:
        github = self.load_yaml("docs/example/github-actions/consume.yml")
        self.assertEqual(set(github["on"]), {"workflow_dispatch"})
        self.assertEqual(set(github["jobs"]), {"consume"})
        self.assertEqual(github["jobs"]["consume"]["steps"][-1]["run"],
                         "bash scripts/ci-consumption-example.sh")

        gitlab = self.load_yaml("docs/example/gitlab-ci/pipeline.yml")
        self.assertEqual(gitlab["workflow"]["rules"][0]["if"],
                         '$CI_PIPELINE_SOURCE == "web"')
        self.assertEqual(gitlab["workflow"]["rules"][-1]["when"], "never")
        self.assertEqual(gitlab["consume"]["when"], "manual")
        self.assertIn("bash scripts/ci-consumption-example.sh", gitlab["consume"]["script"][0])

    def run_script(self, *, fail_on: str = "", sleep_on: str = "",
                   version: str = "0.4.28") -> tuple[subprocess.CompletedProcess, Path]:
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        bindir = root / "bin"
        bindir.mkdir()
        log = root / "calls.log"
        fake = bindir / "baha"
        fake.write_text(
            "#!/usr/bin/env python3\n"
            "import json, os, pathlib, sys, time\n"
            "args = sys.argv[1:]\n"
            "call = ' '.join(args)\n"
            "with open(os.environ['BAHA_CALL_LOG'], 'a') as f: f.write(call + '\\n')\n"
            "if os.environ.get('BAHA_SLEEP_ON') and os.environ['BAHA_SLEEP_ON'] in call:\n"
            "    time.sleep(float(os.environ.get('BAHA_SLEEP_SECONDS', '5')))\n"
            "if os.environ.get('BAHA_FAIL_ON') and os.environ['BAHA_FAIL_ON'] in call:\n"
            "    print('fixture failure', file=sys.stderr); raise SystemExit(23)\n"
            "if ('-o' in args or '--format' in args) and 'json' in args: print(json.dumps({'fixture': True}))\n"
            "elif args and args[0] == 'version': print('baha fixture')\n"
        )
        fake.chmod(0o755)
        timeout = bindir / "timeout"
        timeout.write_text("#!/bin/sh\nshift\nexec /usr/bin/timeout 1 \"$@\"\n")
        timeout.chmod(0o755)
        env = os.environ.copy()
        env.update({
            "PATH": str(bindir) + os.pathsep + env["PATH"],
            "BAHA_CALL_LOG": str(log),
            "BAHA_FAIL_ON": fail_on,
            "BAHA_SLEEP_ON": sleep_on,
            "BAHA_SLEEP_SECONDS": "5",
            "BASEHARBOR_VERSION": version,
            "APP_ENVIRONMENT": "dev",
            "APP_API_KEY": "fixture-secret-must-not-be-printed",
            "APP_CHECK_POSTGRES_CREDS": "true",
        })
        result = subprocess.run(["bash", str(SCRIPT)], cwd=root, env=env,
                                text=True, capture_output=True, timeout=10)
        return result, log

    def test_normal_flow_passes_and_keeps_secret_off_output(self) -> None:
        result, log = self.run_script()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("fixture-secret-must-not-be-printed", result.stdout + result.stderr)
        calls = log.read_text().splitlines()
        self.assertTrue(any("up --control-plane-only --yes" in call for call in calls))
        self.assertTrue(any("app secret set API_KEY --stdin" in call for call in calls))
        self.assertTrue(any("up --environment dev --yes" in call for call in calls))
        self.assertTrue(any("app creds postgres -o json" in call for call in calls))

    def test_json_outputs_are_parseable(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            # Run in a directory that models the checked-out application.
            original = SCRIPT
            helper = Path(directory) / "consume.sh"
            helper.write_text(original.read_text())
            env = os.environ.copy()
            bindir = Path(directory) / "bin"
            bindir.mkdir()
            fake = bindir / "baha"
            fake.write_text("#!/bin/sh\nif [ \"$1\" = version ]; then exit 0; fi\n"
                            "for arg in \"$@\"; do [ \"$arg\" = json ] && echo '{\"ok\":true}' && exit 0; done\n"
                            "exit 0\n")
            fake.chmod(0o755)
            timeout = bindir / "timeout"
            timeout.write_text("#!/bin/sh\nshift\nexec /usr/bin/timeout 1 \"$@\"\n")
            timeout.chmod(0o755)
            env.update({"PATH": str(bindir) + os.pathsep + env["PATH"],
                        "BASEHARBOR_VERSION": "0.4.28", "APP_ENVIRONMENT": "dev",
                        "APP_CHECK_POSTGRES_CREDS": "true"})
            result = subprocess.run(["bash", str(helper)], cwd=directory, env=env,
                                    text=True, capture_output=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
            for name in ("inspection.json", "plan.json", "status.json",
                         "environment.json", "postgres-connection.json"):
                self.assertTrue(json.loads((Path(directory) / name).read_text())["ok"])

    def test_command_failure_propagates_and_cleanup_runs(self) -> None:
        result, log = self.run_script(fail_on="app preflight")
        self.assertEqual(result.returncode, 23)
        self.assertTrue(any(call.endswith("down --yes") for call in log.read_text().splitlines()))

    def test_timeout_is_enforced_and_cleanup_runs(self) -> None:
        result, log = self.run_script(sleep_on="up --environment")
        self.assertEqual(result.returncode, 124)
        self.assertTrue(any(call.endswith("down --yes") for call in log.read_text().splitlines()))

    def test_invalid_release_version_fails_before_cli_execution(self) -> None:
        result, log = self.run_script(version="latest; echo unsafe")
        self.assertEqual(result.returncode, 2)
        self.assertFalse(log.exists())


if __name__ == "__main__":
    unittest.main()
