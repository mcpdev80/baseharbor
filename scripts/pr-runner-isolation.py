#!/usr/bin/env python3
"""#896: prepare and clean isolated disposable PR runtime lanes.

This helper DOES NOT execute a gate or attest evidence. Trusted orchestration
must admit a request and invoke existing gate runners independently.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import tempfile
import fcntl
import subprocess
import urllib.request

SHA = re.compile(r"^[0-9a-f]{40}$")
SLUG = re.compile(r"^[a-zA-Z0-9_-]{1,60}$")
RUNTIMES = {"docker", "podman"}
FIELDS = ("target_base_sha", "head_sha", "merge_base_sha", "integration_sha")
PINS = ("demo_sha", "console_sha", "connector_sha")


def github_json(url, token):
    if not token:
        raise PermissionError("verified GitHub API token required")
    if not url.startswith("https://api.github.com/repos/"):
        raise ValueError("invalid GitHub API endpoint")
    request = urllib.request.Request(
        url, headers={"Authorization": "Bearer " + token,
                      "Accept": "application/vnd.github+json"})
    with urllib.request.urlopen(request, timeout=10) as response:
        return json.load(response)


def verify_github_admission(request, run_id, attempt, repository, pr_number, token):
    # This MUST run from immutable trusted source before untrusted code begins.
    # A username, head branch, environment variable or caller-supplied bool is
    # never accepted as proof of admission.
    if not re.fullmatch(r"[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+", repository):
        raise ValueError("invalid repository")
    if not run_id.isdecimal() or not isinstance(pr_number, int) or pr_number < 1:
        raise ValueError("invalid GitHub run or PR")
    prefix = "https://api.github.com/repos/" + repository
    run = github_json(prefix + "/actions/runs/" + run_id, token)
    pr = github_json(prefix + "/pulls/" + str(pr_number), token)
    repo_info = github_json(prefix, token)
    if (run.get("event") != "pull_request" or
            run.get("repository", {}).get("id") != repo_info.get("id") or
            run.get("run_attempt") != attempt or
            run.get("head_sha") != request.get("head_sha") or
            pr.get("base", {}).get("ref") != "develop" or
            pr.get("base", {}).get("sha") != request.get("target_base_sha") or
            pr.get("head", {}).get("sha") != request.get("head_sha") or
            pr.get("state") != "open" or
            pr.get("merge_commit_sha") != request.get("integration_sha")):
        raise PermissionError("GitHub run/PR/integration identity mismatch")
    # Only an authenticated repository-owner PR can enter this initial
    # privileged admission class. External PRs require separate reviewed
    # qualification and must not enter privileged runtime runners.
    if (pr.get("user", {}).get("id") != repo_info.get("owner", {}).get("id") or
            pr.get("head", {}).get("repo", {}).get("id") != repo_info.get("id")):
        raise PermissionError("untrusted PR cannot enter native qualification")
    return True


def validate(request, run_id, attempt, capacity):
    if not isinstance(request, dict) or request.get("schema") != "baseharbor.pr-runtime-request/v1":
        raise ValueError("unsupported request schema")
    if request.get("admission") != "requires-trusted-isolated-runner" or request.get("evidence") != "existing-verifier-only":
        raise ValueError("request cannot override runtime admission or evidence authority")
    runtime = request.get("runtime")
    gate = request.get("gate_id")
    if runtime not in RUNTIMES or not isinstance(gate, str) or not gate.startswith("atomic/" + runtime + "/"):
        raise ValueError("unsupported or mismatched runtime gate")
    if any(not isinstance(request.get(k), str) or not SHA.fullmatch(request[k]) for k in FIELDS):
        raise ValueError("invalid immutable candidate identity")
    pins = request.get("consumer_pins")
    if not isinstance(pins, dict) or any(not isinstance(pins.get(k), str) or not SHA.fullmatch(pins[k]) for k in PINS):
        raise ValueError("unverified consumer pins")
    if not isinstance(run_id, str) or not SLUG.fullmatch(run_id) or not isinstance(attempt, int) or attempt < 1:
        raise ValueError("invalid run identity")
    if not isinstance(capacity, int) or not 1 <= capacity <= 64:
        raise ValueError("invalid declared runner capacity")
    return runtime, gate


def identity(request, run_id, attempt):
    canonical = json.dumps({"request": request, "run_id": run_id, "attempt": attempt},
                           sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(canonical).hexdigest()


def prepare(request, root, run_id, attempt, capacity, repository, pr_number, token):
    runtime, gate = validate(request, run_id, attempt, capacity)
    verify_github_admission(request, run_id, attempt, repository, pr_number, token)
    original_root = Path(root)
    if original_root.is_symlink():
        raise ValueError("symlink isolation root forbidden")
    root = original_root.resolve(strict=True)
    if not root.is_dir():
        raise ValueError("invalid isolation root")
    lane_id = identity(request, run_id, attempt)
    # A lane is unique by all immutable request fields and the run/attempt.
    lane_name = "pr-" + lane_id[:24]
    lane = root / lane_name
    # No shared mutable workspace, recovery files, credentials or provider state.
    # Claiming slots is atomic; capacity overflow must block, never evict peers.
    lock_file = root / ".pr-runner-capacity.lock"
    with lock_file.open("a+") as lock:
        fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
        active = [entry for entry in root.glob("pr-*") if entry.is_dir()]
        if len(active) >= capacity:
            raise RuntimeError("runner capacity exhausted; no peer cleanup permitted")
        lane.mkdir(mode=0o700, exist_ok=False)
        fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
    try:
        for name in ("workspace", "target", "volumes", "provider", "secrets",
                     "recovery", "artifacts", "tmp"):
            (lane / name).mkdir(mode=0o700)
        metadata = {"schema": "baseharbor.pr-runtime-lease/v1",
                    "lane_id": lane_id, "run_id": run_id, "attempt": attempt,
                    "runtime": runtime, "gate_id": gate,
                    "request": request,
                    "network": "bh-" + lane_id[:20],
                    "volume_prefix": "bh-" + lane_id[:20],
                    "port_namespace": lane_id[:20],
                    "isolation_root": str(lane)}
        file = lane / "lease.json"
        with file.open("x") as handle:
            json.dump(metadata, handle, sort_keys=True)
        file.chmod(0o600)
        return metadata
    except BaseException:
        shutil.rmtree(lane)
        raise


def cleanup(root, lane_id, run_id, attempt):
    root = Path(root).resolve(strict=True)
    if not isinstance(lane_id, str) or not re.fullmatch("[0-9a-f]{64}", lane_id):
        raise ValueError("invalid lease")
    lane = root / ("pr-" + lane_id[:24])
    if lane.is_symlink():
        raise ValueError("refuse symlink lane")
    with (lane / "lease.json").open() as handle:
        lease = json.load(handle)
    if (lease.get("lane_id") != lane_id or lease.get("run_id") != run_id or
            lease.get("attempt") != attempt or lease.get("isolation_root") != str(lane)):
        raise PermissionError("cleanup ownership mismatch")
    # A helper cannot independently prove runtime resource deletion.
    # The admitted runner must first remove its OWN containers/networks/volumes.
    # Return this explicit required cleanup contract, not success evidence.
    return {"lane": str(lane), "cleanup_required": True,
            "network": lease["network"], "volume_prefix": lease["volume_prefix"],
            "state": "pending-runtime-cleanup"}


def finalize_cleanup(root, lane_id, run_id, attempt):
    """Release only our own verified-empty lease, never another PR's lane."""
    pending = cleanup(root, lane_id, run_id, attempt)
    root = Path(root).resolve(strict=True)
    lane = Path(pending["lane"])
    with (lane / "lease.json").open() as source:
        lease = json.load(source)
    runtime = lease["runtime"]
    if runtime not in RUNTIMES:
        raise ValueError("invalid lease runtime")
    for kind, name in (("network", lease["network"]),
                       ("volume", lease["volume_prefix"])):
        try:
            result = subprocess.run([runtime, kind, "inspect", name],
                                    capture_output=True, check=False, timeout=20)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise RuntimeError("runtime cleanup cannot be verified") from error
        if result.returncode == 0:
            raise RuntimeError("own runtime " + kind + " remains after cleanup")
        # The supported CLI returns 1 when the named resource is absent.
        # Unknown exit statuses are not absence and must not release a lease.
        if result.returncode != 1:
            raise RuntimeError("runtime cleanup inspector failed")
    lock_file = root / ".pr-runner-capacity.lock"
    with lock_file.open("a+") as lock:
        fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
        again = cleanup(root, lane_id, run_id, attempt)
        if again["lane"] != str(lane):
            raise PermissionError("lease changed during cleanup")
        shutil.rmtree(lane)
        if lane.exists():
            raise RuntimeError("lease workspace cleanup unverified")
    return {"schema": "baseharbor.pr-runtime-cleanup/v1",
            "lane_id": lane_id, "run_id": run_id, "attempt": attempt,
            "state": "verified-empty-and-released",
            "gate_evidence": False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=("prepare", "inspect-cleanup", "finalize-cleanup"))
    parser.add_argument("--request", type=Path, required=True)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--attempt", type=int, required=True)
    parser.add_argument("--capacity", type=int, default=1)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--pr-number", type=int, required=True)
    args = parser.parse_args()
    request = json.loads(args.request.read_text())
    if args.operation == "prepare":
        result = prepare(request, args.root, args.run_id, args.attempt,
                         args.capacity, args.repository, args.pr_number,
                         os.environ.get('GH_TOKEN', ''))
    elif args.operation == "inspect-cleanup":
        result = cleanup(args.root, identity(request, args.run_id, args.attempt),
                         args.run_id, args.attempt)
    else:
        result = finalize_cleanup(args.root, identity(request, args.run_id, args.attempt),
                                  args.run_id, args.attempt)
    print(json.dumps(result, sort_keys=True, indent=2))


if __name__ == "__main__":
    main()
