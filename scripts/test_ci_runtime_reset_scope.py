#!/usr/bin/env python3
"""Disposable sandbox safety regressions for ci-runtime-reset.sh."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().with_name("ci-runtime-reset.sh")


class RuntimeResetOwnershipTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.home = self.root / "home"
        self.home.mkdir()
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.log = self.root / "operations.log"
        engine = self.bin / "docker"
        engine.write_text("""#!/usr/bin/env bash
set -euo pipefail
printf '%s\\n' "$*" >> "$STUB_LOG"
case "$1 $2" in
  "container ls"|"network ls"|"volume ls")
    printf '%s\\n' selected foreign
    ;;
  "container inspect"|"network inspect"|"volume inspect")
    printf 'baseharbor-selected|baseharbor-selected|\\n'
    printf 'baseharbor-foreign|baseharbor-foreign|\\n'
    ;;
esac
""")
        engine.chmod(0o755)
        podman = self.bin / "podman"
        podman.symlink_to(engine)
        systemctl = self.bin / "systemctl"
        systemctl.write_text('#!/usr/bin/env bash\nprintf "systemctl %s\\n" "$*" >> "$STUB_LOG"\n')
        systemctl.chmod(0o755)
        self.env = os.environ.copy()
        self.env.update(
            HOME=str(self.home), XDG_DATA_HOME=str(self.root / "data"),
            XDG_CONFIG_HOME=str(self.root / "config"),
            XDG_CACHE_HOME=str(self.root / "cache"),
            STUB_LOG=str(self.log),
            PATH=str(self.bin) + os.pathsep + os.environ["PATH"],
        )
        for name in ("data", "config", "cache"):
            directory = self.root / name / "baseharbor"
            for scope in ("selected", "foreign", "unattributed"):
                entry = directory / scope / "sentinel"
                entry.parent.mkdir(parents=True, exist_ok=True)
                entry.write_text(scope)
        quadlet = self.root / "config" / "containers" / "systemd"
        quadlet.mkdir(parents=True)
        for name in ("baseharbor-selected.container", "baseharbor-foreign.container"):
            (quadlet / name).write_text("fixture")

    def run_reset(self, engine="docker", scope=None):
        args = ["bash", str(SCRIPT), engine]
        if scope is not None:
            args.append(scope)
        return subprocess.run(args, env=self.env, text=True, capture_output=True)

    def assert_state(self, exists):
        for name in ("data", "config", "cache"):
            for scope in ("selected", "foreign", "unattributed"):
                self.assertEqual(exists, (self.root / name / "baseharbor" / scope / "sentinel").exists())

    def test_scoped_shared_state_preserved_resources_filtered(self):
        result = self.run_reset(scope="selected")
        self.assertEqual(0, result.returncode, result.stderr)
        self.assert_state(True)
        ops = self.log.read_text()
        self.assertIn("container rm baseharbor-selected", ops)
        self.assertIn("network rm baseharbor-selected", ops)
        self.assertIn("volume rm -f baseharbor-selected", ops)
        self.assertNotIn("container rm baseharbor-foreign", ops)
        self.assertNotIn("network rm baseharbor-foreign", ops)
        self.assertNotIn("volume rm -f baseharbor-foreign", ops)

    def test_unknown_scope_preserves_all_state_and_resources(self):
        result = self.run_reset(scope="unknown")
        self.assertEqual(0, result.returncode, result.stderr)
        self.assert_state(True)
        self.assertNotIn("container rm ", self.log.read_text())
        self.assertNotIn("network rm ", self.log.read_text())
        self.assertNotIn("volume rm ", self.log.read_text())

    def test_scoped_podman_quadlet_only_selected_and_retains_xdg(self):
        result = self.run_reset(engine="podman", scope="selected")
        self.assertEqual(0, result.returncode, result.stderr)
        self.assert_state(True)
        quadlet = self.root / "config" / "containers" / "systemd"
        self.assertFalse((quadlet / "baseharbor-selected.container").exists())
        self.assertTrue((quadlet / "baseharbor-foreign.container").exists())

    def test_even_isolated_scoped_root_has_no_implicit_delete_authority(self):
        for name in ("data", "config", "cache"):
            base = self.root / name / "baseharbor"
            for other in ("foreign", "unattributed"):
                (base / other / "sentinel").unlink()
        result = self.run_reset(scope="selected")
        self.assertEqual(0, result.returncode, result.stderr)
        for name in ("data", "config", "cache"):
            self.assertTrue((self.root / name / "baseharbor" / "selected" / "sentinel").exists())

    def test_unscoped_dedicated_runner_full_reset_unchanged(self):
        result = self.run_reset()
        self.assertEqual(0, result.returncode, result.stderr)
        self.assert_state(False)
        ops = self.log.read_text()
        self.assertIn("container rm baseharbor-selected baseharbor-foreign", ops)
        self.assertIn("network rm baseharbor-selected baseharbor-foreign", ops)
        self.assertIn("volume rm -f baseharbor-selected baseharbor-foreign", ops)


if __name__ == "__main__":
    unittest.main()
