#!/usr/bin/env python3
"""Fail-closed PR unit-test shards: full Go coverage or targeted CLI coverage."""
import argparse
import hashlib
import json
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CLI_PACKAGE = "./cmd/baha"
CLI_SHARDS = 3
TEST_NAME = re.compile(r"^(?:Test|Example|Fuzz)[A-Za-z0-9_]*$")
REQUIRED_CLIENT_TESTS = {
    "TestClientCommandsTwoHomesSameDeploymentPrecedence",
    "TestClientCommandsDenialOverTwoHomes",
    "TestClientCommandsServerLossOverTwoHomes",
    "TestClientCommandsStaleConsentOverTwoHomes",
    "TestClientCommandsErrorParityOverTwoHomes",
}


def run(args):
    return subprocess.run(args, cwd=ROOT, text=True, capture_output=True, check=False)


def discover_cli_tests():
    result = run(["go", "test", CLI_PACKAGE, "-list", "."])
    if result.returncode:
        raise RuntimeError("CLI test discovery failed: " + result.stdout + result.stderr)
    names = [line.strip() for line in result.stdout.splitlines()
             if TEST_NAME.fullmatch(line.strip())]
    if not names or len(names) != len(set(names)):
        raise RuntimeError("CLI test discovery is empty or has duplicate names")
    if (ROOT / "cmd/baha/client_commands_behaviors_test.go").is_file():
        missing = REQUIRED_CLIENT_TESTS - set(names)
        if missing:
            raise RuntimeError("required CLI contract tests missing: " + ", ".join(sorted(missing)))
    return sorted(names)


def selected_tests(all_names, shard):
    if not re.fullmatch(r"cli-[0-2]", shard):
        raise ValueError("unknown CLI shard")
    index = int(shard[-1])
    return [name for name in all_names
            if int.from_bytes(hashlib.sha256(name.encode()).digest()[:8], "big") % CLI_SHARDS == index]


def cli(shard):
    names = discover_cli_tests()
    selected = selected_tests(names, shard)
    if not selected:
        raise RuntimeError("empty CLI test shard (fail closed)")
    regex = "^(" + "|".join(re.escape(x) for x in selected) + ")$"
    result = run(["go", "test", CLI_PACKAGE, "-count=1", "-json", "-run", regex])
    events = []
    try:
        events = [json.loads(line) for line in result.stdout.splitlines() if line.strip()]
    except ValueError as exc:
        raise RuntimeError("invalid Go JSON test output") from exc
    if result.returncode or any(event.get("Action") == "fail" for event in events):
        raise RuntimeError("CLI shard failed:\n" + result.stdout[-18000:] + result.stderr[-3000:])
    executed = {event["Test"] for event in events
                if event.get("Action") == "run" and event.get("Test") in selected}
    passed = {event["Test"] for event in events
              if event.get("Action") == "pass" and event.get("Test") in selected}
    skipped = {event["Test"] for event in events
               if event.get("Action") == "skip" and event.get("Test") in selected}
    missing = set(selected) - executed
    incomplete = executed - passed - skipped
    required = REQUIRED_CLIENT_TESTS & set(selected)
    if missing or incomplete or required - passed or not passed:
        raise RuntimeError("CLI shard incomplete: missing=" + str(sorted(missing)) +
                           " incomplete=" + str(sorted(incomplete)) +
                           " required-not-passed=" + str(sorted(required-passed)))
    print(f"PASS {shard}: {len(passed)} passed / {len(skipped)} intentionally skipped "
          f"/ {len(selected)} discovered; CLI inventory total={len(names)}")


def packages():
    result = run(["go", "list", "./..."])
    if result.returncode:
        raise RuntimeError("Go package discovery failed: " + result.stdout + result.stderr)
    packages = [line.strip() for line in result.stdout.splitlines() if line.strip()]
    if not packages or len(packages) != len(set(packages)):
        raise RuntimeError("Go package inventory empty or duplicated")
    cli_package = next((p for p in packages if p.endswith("/cmd/baha")), None)
    if not cli_package:
        raise RuntimeError("CLI package missing from go list ./...")
    others = [p for p in packages if p != cli_package]
    if not others:
        raise RuntimeError("non-CLI package inventory empty")
    print(f"Running every non-CLI Go package ({len(others)}), no test filters", flush=True)
    completed = subprocess.run(["go", "test", "-count=1", *others], cwd=ROOT, check=False)
    if completed.returncode:
        raise RuntimeError("non-CLI Go unit package tests failed")
    print("PASS all non-CLI Go package tests")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=["cli-0", "cli-1", "cli-2", "packages"], required=True)
    mode = parser.parse_args().mode
    if mode == "packages":
        packages()
    else:
        cli(mode)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, ValueError) as error:
        sys.exit(str(error))
