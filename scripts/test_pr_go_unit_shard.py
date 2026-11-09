"""Regression tests for the deterministic, fail-closed Go PR unit shards."""
import importlib.util
import json
from pathlib import Path
import re
import subprocess
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("go_shards", Path(__file__).with_name("pr-go-unit-shard.py"))
shards = importlib.util.module_from_spec(spec)
spec.loader.exec_module(shards)


class GoUnitShardTests(unittest.TestCase):
    def test_every_discovered_cli_test_is_assigned_exactly_once(self):
        names = ["TestA", "TestB", "TestC", "TestD", "TestE", "ExampleFoo", "FuzzBar"]
        partitions = [shards.selected_tests(names, f"cli-{index}") for index in range(3)]
        self.assertEqual(set(names), set().union(*map(set, partitions)))
        self.assertEqual(len(names), sum(map(len, partitions)))
        self.assertEqual(partitions, [shards.selected_tests(names, f"cli-{index}")
                                      for index in range(3)])
        with self.assertRaises(ValueError):
            shards.selected_tests(names, "cli-9")

    def test_missing_go_test_inventory_rejected(self):
        response = subprocess.CompletedProcess(["go"], 0, stdout="ok\n")
        with patch.object(shards, "run", return_value=response):
            with self.assertRaisesRegex(RuntimeError, "empty"):
                shards.discover_cli_tests()

    def test_cli_shard_requires_actual_test_execution_not_just_exit_zero(self):
        names = sorted(shards.REQUIRED_CLIENT_TESTS | {f"TestExtra{i}" for i in range(20)})
        listed = subprocess.CompletedProcess(["go"], 0, stdout="\n".join(names))
        # The Go command can exit successfully when its -run regex matches no tests.
        fake_green = subprocess.CompletedProcess(["go"], 0, stdout=json.dumps({
            "Action": "pass", "Package": "github.com/mcpdev80/baseharbor/cmd/baha"
        }))
        with patch.object(shards, "run", side_effect=[listed, fake_green]):
            with self.assertRaisesRegex(RuntimeError, "incomplete"):
                shards.cli("cli-0")

    def test_cli_shard_rejects_skipped_mandatory_client_tests(self):
        names = sorted(shards.REQUIRED_CLIENT_TESTS | {f"TestExtra{i}" for i in range(40)})
        chosen = shards.selected_tests(names, "cli-1")
        required = shards.REQUIRED_CLIENT_TESTS & set(chosen)
        self.assertTrue(required)
        events = []
        for name in chosen:
            events.extend((
                {"Action": "run", "Test": name},
                {"Action": "skip" if name in required else "pass", "Test": name},
            ))
        listed = subprocess.CompletedProcess(["go"], 0, stdout="\n".join(names))
        execution = subprocess.CompletedProcess(["go"], 0, stdout="\n".join(map(json.dumps, events)))
        with patch.object(shards, "run", side_effect=[listed, execution]):
            with self.assertRaisesRegex(RuntimeError, "required-not-passed"):
                shards.cli("cli-1")

    def test_non_cli_inventory_excludes_cli_but_keeps_all_other_packages(self):
        listed = subprocess.CompletedProcess(["go"], 0, stdout="\n".join((
            "github.com/mcpdev80/baseharbor/cmd/baha",
            "github.com/mcpdev80/baseharbor/internal/deployment",
            "github.com/mcpdev80/baseharbor/internal/stableid",
        )))
        with patch.object(shards, "run", return_value=listed), patch.object(
                shards.subprocess, "run", return_value=subprocess.CompletedProcess(["go"], 0)) as execute:
            shards.packages()
        cmd = execute.call_args.args[0]
        self.assertEqual(["go", "test", "-count=1"], cmd[:3])
        self.assertEqual(2, len(cmd[3:]))
        self.assertNotIn("github.com/mcpdev80/baseharbor/cmd/baha", cmd)


if __name__ == "__main__":
    unittest.main()
