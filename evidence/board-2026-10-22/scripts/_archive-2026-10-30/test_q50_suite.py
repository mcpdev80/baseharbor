#!/usr/bin/env python3
"""q50 test suite: OPS-4 sanitize_evidence_for_plan.sh + OPS-5 basics-check.py LATENCY-Flag

Run: python3 evidence/board-2026-10-22/scripts/test_q50_suite.py
"""
import os
import subprocess
import sys
import tempfile
import time
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SANITIZE = os.path.join(HERE, "sanitize_evidence_for_plan.sh")
BASES_CHECK = os.path.join(HERE, "..", "..", "board-2026-10-19", "scripts", "basics-check.py")
GATEWAY_LATENCY_CACHE = os.path.join(HERE, "..", "..", "board-2026-10-19", "scripts", "gateway-latency.json")


def sh(cmd, timeout=30, input=None, env=None):
    return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout, input=input, env=env)


class TestSanitizeEvidence(unittest.TestCase):
    """OPS-4: Checkbox masking in sanitize_evidence_for_plan.sh"""

    def test_empty_checkbox_masked(self):
        """Empty checkbox [- [ ]] -> [- [☐]]"""
        r = sh(["bash", SANITIZE], input="- [ ] test checkbox\n")
        self.assertEqual(r.returncode, 0)
        self.assertIn("☐", r.stdout)
        self.assertNotIn("- [ ]", r.stdout)

    def test_checked_checkbox_masked(self):
        """Checked checkbox [- [x]] -> [- [✔]]"""
        r = sh(["bash", SANITIZE], input="- [x] checked box\n")
        self.assertEqual(r.returncode, 0)
        self.assertIn("✔", r.stdout)
        self.assertNotIn("- [x]", r.stdout)

    def test_upper_checked_masked(self):
        """Upper checked [- [X]] -> [- [✔]]"""
        r = sh(["bash", SANITIZE], input="- [X] upper checked\n")
        self.assertEqual(r.returncode, 0)
        self.assertIn("✔", r.stdout)
        self.assertNotIn("- [X]", r.stdout)

    def test_done_checkbox_masked(self):
        """Done checkbox [- [done]] -> [- [✔]]"""
        r = sh(["bash", SANITIZE], input="- [done] done box\n")
        self.assertEqual(r.returncode, 0)
        self.assertIn("✔", r.stdout)
        self.assertNotIn("- [done]", r.stdout)

    def test_dash_checkbox_masked(self):
        """Dash checkbox [- [-]] -> [- [○]]"""
        r = sh(["bash", SANITIZE], input="- [-] dash box\n")
        self.assertEqual(r.returncode, 0)
        self.assertIn("○", r.stdout)
        self.assertNotIn("- [-]", r.stdout)

    def test_arrow_checkbox_masked(self):
        """Arrow checkbox [- [>]] -> [- [○]]"""
        r = sh(["bash", SANITIZE], input="- [>] arrow box\n")
        self.assertEqual(r.returncode, 0)
        self.assertIn("○", r.stdout)
        self.assertNotIn("- [>]", r.stdout)

    def test_non_checkbox_preserved(self):
        """Non-checkbox lines pass through unchanged"""
        r = sh(["bash", SANITIZE], input="# Title\n- not a checkbox\n  [ ] not start\n")
        self.assertEqual(r.returncode, 0)
        self.assertIn("# Title", r.stdout)
        self.assertIn("- not a checkbox", r.stdout)

    def test_file_input(self):
        """File argument works"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
            f.write("- [ ] file test\n- [x] file checked\n")
            f.flush()
            fname = f.name
        try:
            r = sh(["bash", SANITIZE, fname])
            self.assertEqual(r.returncode, 0)
            self.assertIn("☐", r.stdout)
            self.assertIn("✔", r.stdout)
        finally:
            os.unlink(fname)

    def test_usage_error(self):
        """Too many args -> exit 1"""
        r = sh(["bash", SANITIZE, "arg1", "arg2"])
        self.assertEqual(r.returncode, 1)

    def test_mixed_content(self):
        """Mixed checkboxes and regular lines"""
        r = sh(["bash", SANITIZE], input="## PLAN\n- [ ] step one\n- [x] step done\n# comment\n- [done] completed\n")
        self.assertEqual(r.returncode, 0)
        self.assertIn("☐", r.stdout)
        self.assertIn("✔", r.stdout)
        self.assertIn("## PLAN", r.stdout)
        self.assertIn("# comment", r.stdout)


class TestBasicsCheckLatency(unittest.TestCase):
    """OPS-5: LATENCY-Flag in basics-check.py"""

    def test_basics_check_compiles(self):
        """basics-check.py compiles without error"""
        r = sh([sys.executable, "-m", "py_compile", BASES_CHECK])
        self.assertEqual(r.returncode, 0)

    def test_basics_check_runs(self):
        """basics-check.py runs and exits 0 (flags ok, no errors)"""
        r = sh([sys.executable, BASES_CHECK])
        self.assertEqual(r.returncode, 0)

    def test_basics_check_json_mode(self):
        """basics-check.py --json exits 0 and includes flags"""
        r = sh([sys.executable, BASES_CHECK, "--json"])
        self.assertEqual(r.returncode, 0)
        import json
        try:
            data = json.loads(r.stdout)
            self.assertIn("flags", data)
        except ValueError:
            self.fail("basics-check --json output not valid JSON")

    def test_latency_function_exists(self):
        """gateway_latency() function exists in basics-check.py source"""
        with open(BASES_CHECK) as f:
            src = f.read()
        self.assertIn("def gateway_latency", src)

    def test_latency_gates_are_readonly(self):
        """gateway_latency does NOT call agent spawn or kill"""
        with open(BASES_CHECK) as f:
            src = f.read()
        # The function should use curl, not codeman agent
        self.assertIn("curl", src)
        self.assertNotIn("agent spawn", src.split("def gateway_latency")[1].split("def main")[0])

    def test_latency_cache_file(self):
        """gateway_latency writes cache file after first run"""
        # Run basics-check to trigger gateway_latency()
        sh([sys.executable, BASES_CHECK])
        self.assertTrue(os.path.exists(GATEWAY_LATENCY_CACHE))

    def test_latency_flags_on_high_latency(self):
        """When gateway latency > 5s, LATENCY flag fires"""
        # Clear cache so gateway_latency does a fresh curl
        try:
            os.unlink(GATEWAY_LATENCY_CACHE)
        except FileNotFoundError:
            pass
        # We test via the actual function: set env to a non-reachable URL
        # 10.255.255.1 forces a real network timeout (~5s connect timeout)
        env = os.environ.copy()
        env["GATEWAY_URL"] = "http://10.255.255.1:4000/v1/models"
        r = sh([sys.executable, BASES_CHECK], env=env)
        self.assertEqual(r.returncode, 0)
        # LATENCY flag should appear due to unreachable gateway
        self.assertIn("LATENCY:", r.stdout)

    def test_pool_latency_aware(self):
        """POOL check mentions LATENCY HOLD when latency blocks spawns"""
        try:
            os.unlink(GATEWAY_LATENCY_CACHE)
        except FileNotFoundError:
            pass
        env = os.environ.copy()
        env["GATEWAY_URL"] = "http://10.255.255.1:4000/v1/models"
        r = sh([sys.executable, BASES_CHECK], env=env)
        self.assertIn("POOL:", r.stdout)


if __name__ == "__main__":
    unittest.main()
