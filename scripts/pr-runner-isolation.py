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

SHA = re.compile(r"^[0-9a-f]{40}$")
SLUG = re.compile(r"^[a-zA-Z0-9_-]{1,60}$")
RUNTIMES = {"docker", "podman"}
FIELDS = ("target_base_sha", "head_sha", "merge_base_sha", "integration_sha")
PINS = ("demo_sha", "console_sha", "connector_sha")


def validate(request, run_id, attempt, capacity, trusted=False):
    if not trusted:
        raise PermissionError("runtime admission requires verified trusted orchestration")
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


def prepare(request, root, run_id, attempt, capacity, trusted=False):
    runtime, gate = validate(request, run_id, attempt, capacity, trusted)
    root = Path(root).resolve(strict=True)
    if root.is_symlink() or not root.is_dir():
        raise ValueError("invalid isolation root")
    lane_id = identity(request, run_id, attempt)
    # A lane is unique by all immutable request fields and the run/attempt.
    lane_name = "pr-" + lane_id[:24]
    lane = root / lane_name
    slot = root / ("slot-" + lane_id[:16])
    # No shared mutable workspace, recovery files, credentials or provider state.
    # Claiming slots is atomic; capacity overflow must block, never evict peers.
    active = list(root.glob("pr-*/lease.json"))
    if len(active) >= capacity:
        raise RuntimeError("runner capacity exhausted; no peer cleanup permitted")
    lane.mkdir(mode=0o700, exist_ok=False)
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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=("prepare", "inspect-cleanup"))
    parser.add_argument("--request", type=Path, required=True)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--attempt", type=int, required=True)
    parser.add_argument("--capacity", type=int, default=1)
    parser.add_argument("--admitted", action="store_true")
    args = parser.parse_args()
    request = json.loads(args.request.read_text())
    if args.operation == "prepare":
        result = prepare(request, args.root, args.run_id, args.attempt,
                         args.capacity, args.admitted)
    else:
        result = cleanup(args.root, identity(request, args.run_id, args.attempt),
                         args.run_id, args.attempt)
    print(json.dumps(result, sort_keys=True, indent=2))


if __name__ == "__main__":
    main()
