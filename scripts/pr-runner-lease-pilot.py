#!/usr/bin/env python3
"""Exact GitHub PR admission plus Docker/Podman lease cleanup pilot.

Synthetic consumer hashes are ONLY isolation fixtures, never valid gate evidence.
"""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

spec = importlib.util.spec_from_file_location("isolation", Path(__file__).with_name("pr-runner-isolation.py"))
isolation = importlib.util.module_from_spec(spec)
spec.loader.exec_module(isolation)


def call(runtime, *args):
    return subprocess.run([runtime, *args], capture_output=True, text=True,
                          check=True, timeout=90)


def resources(runtime, lease):
    prefix = lease["volume_prefix"]
    network, volume, container = prefix + "-net", prefix + "-vol", prefix + "-container"
    call(runtime, "network", "create", network)
    call(runtime, "volume", "create", volume)
    call(runtime, "create", "--name", container, "--network", network,
         "--mount", "type=volume,src=" + volume + ",dst=/data",
         "--security-opt", "no-new-privileges", "--cap-drop", "ALL",
         "docker.io/library/alpine:3.21", "/bin/sh", "-c",
         "printf ok > /data/probe")
    call(runtime, "start", "-a", container)
    return network, volume, container


def remove_own(runtime, names):
    net, vol, container = names
    for args in (("rm", "-f", container), ("volume", "rm", "-f", vol),
                 ("network", "rm", net)):
        call(runtime, *args)


def execute(runtime):
    event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
    pr = event["pull_request"]
    run_id = os.environ["GITHUB_RUN_ID"]
    attempt = int(os.environ["GITHUB_RUN_ATTEMPT"])
    repository = os.environ["GITHUB_REPOSITORY"]
    verified_pr = isolation.github_json("https://api.github.com/repos/" + repository + "/pulls/" + str(pr["number"]), "")
    pins = {"demo_sha": "1" * 40, "console_sha": "2" * 40, "connector_sha": "3" * 40}
    common = {"schema": "baseharbor.pr-runtime-request/v1",
              "runtime": runtime, "target_base_sha": pr["base"]["sha"],
              "head_sha": pr["head"]["sha"], "merge_base_sha": pr["base"]["sha"],
              "integration_sha": verified_pr["merge_commit_sha"], "consumer_pins": pins,
              "admission": "requires-trusted-isolated-runner",
              "evidence": "existing-verifier-only"}
    main_request = dict(common, gate_id="atomic/" + runtime + "/backup-restore")
    peer_request = dict(common, gate_id="atomic/" + runtime + "/lifecycle")
    call(runtime, "pull", "docker.io/library/alpine:3.21")
    with tempfile.TemporaryDirectory(prefix="bh-pilot-", dir=os.environ["RUNNER_TEMP"]) as root:
        peer = isolation.prepare(peer_request, root, run_id, attempt, 2,
                                 repository, pr["number"], "")
        peer_names = None
        try:
            peer_names = resources(runtime, peer)
            for outcome in ("success", "failure", "cancellation"):
                main = isolation.prepare(main_request, root, run_id, attempt, 2,
                                         repository, pr["number"], "")
                names = None
                try:
                    names = resources(runtime, main)
                    try:
                        isolation.finalize_cleanup(root, main["lane_id"], run_id, attempt)
                    except RuntimeError:
                        pass  # Required: existing objects block lease release
                    else:
                        raise AssertionError("existing native objects released lease")
                    if outcome == "failure":
                        raise RuntimeError("injected failure")
                    if outcome == "cancellation":
                        raise KeyboardInterrupt("injected cancellation")
                except (RuntimeError, KeyboardInterrupt) as error:
                    if str(error) not in ("injected failure", "injected cancellation"):
                        raise
                finally:
                    if names is not None:
                        remove_own(runtime, names)
                    released = isolation.finalize_cleanup(root, main["lane_id"], run_id, attempt)
                    assert released["state"] == "verified-empty-and-released"
                    assert released["gate_evidence"] is False
                assert Path(peer["isolation_root"]).exists()
                assert call(runtime, "network", "inspect", peer_names[0]).returncode == 0
                assert call(runtime, "volume", "inspect", peer_names[1]).returncode == 0
                print("PASS", runtime, outcome, "own cleanup verified, peer preserved")
        finally:
            if peer_names is not None:
                remove_own(runtime, peer_names)
                released = isolation.finalize_cleanup(root, peer["lane_id"], run_id, attempt)
                assert released["state"] == "verified-empty-and-released"


if __name__ == "__main__":
    if len(sys.argv) != 2 or sys.argv[1] not in ("docker", "podman"):
        raise SystemExit(2)
    execute(sys.argv[1])
