#!/usr/bin/env python3
"""Run independent local checks on an isolated source snapshot, not live work."""

import argparse
import concurrent.futures
import json
import os
import pathlib
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time


def git(root, *args, input_data=None):
    return subprocess.run(["git", "-C", str(root), *args], input=input_data,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True).stdout


def snapshot_source(root, destination):
    head = git(root, "rev-parse", "HEAD").decode().strip()
    patch = git(root, "diff", "--binary", "HEAD")
    untracked = git(root, "ls-files", "--others", "--exclude-standard", "-z").split(b"\0")
    subprocess.run(["git", "clone", "--quiet", "--shared", "--no-checkout", str(root), str(destination)], check=True)
    git(destination, "checkout", "--quiet", "--detach", head)
    if patch:
        git(destination, "apply", "--binary", "-", input_data=patch)
    for entry in filter(None, untracked):
        relative = pathlib.Path(os.fsdecode(entry))
        source, target = root / relative, destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target, follow_symlinks=False)
    git(destination, "add", "-A")
    tree = git(destination, "write-tree").decode().strip()
    # A concurrent edit during snapshot construction must not be called the
    # captured working tree. Later edits cannot alter this independent copy.
    if (git(root, "rev-parse", "HEAD").decode().strip() != head or
            git(root, "diff", "--binary", "HEAD") != patch or
            git(root, "ls-files", "--others", "--exclude-standard", "-z").split(b"\0") != untracked):
        raise RuntimeError("Source changed while constructing snapshot; retry qualification")
    for entry in filter(None, untracked):
        source, target = root / os.fsdecode(entry), destination / os.fsdecode(entry)
        if source.is_symlink():
            equal = target.is_symlink() and os.readlink(source) == os.readlink(target)
        else:
            equal = source.read_bytes() == target.read_bytes()
        if not equal:
            raise RuntimeError("Untracked source changed while constructing snapshot; retry qualification")
    return {"commit": head, "tree": tree, "working_tree_changes": tree != git(root, "rev-parse", f"{head}^{{tree}}").decode().strip()}


def run_check(name, command, root, output, env, timeout, cancelled):
    started = time.monotonic()
    status, code = "cancelled", 130
    with (output / f"{name}.log").open("wb") as log:
        if not cancelled.is_set():
            try:
                process = subprocess.Popen(command, cwd=root, env=env, stdout=log,
                                           stderr=subprocess.STDOUT, start_new_session=True)
                while process.poll() is None:
                    if cancelled.is_set() or time.monotonic() - started >= timeout:
                        status = "cancelled" if cancelled.is_set() else "timeout"
                        code = 130 if cancelled.is_set() else 124
                        os.killpg(process.pid, signal.SIGKILL)
                        process.wait()
                        break
                    cancelled.wait(0.1)
                else:
                    code = process.returncode
                    status = "pass" if code == 0 else "fail"
            except OSError as error:
                log.write(str(error).encode())
                status, code = "fail", 127
    result = {"name": name, "status": status, "exit_code": code,
              "seconds": round(time.monotonic() - started, 3), "log": f"{name}.log"}
    print(f"{name}: {status.upper()} ({result['seconds']}s)", flush=True)
    return result


def run_checks(checks, root, output, jobs, env, timeout, cancelled):
    with concurrent.futures.ThreadPoolExecutor(max_workers=jobs) as executor:
        futures = [executor.submit(run_check, name, command, root, output, env, timeout, cancelled)
                   for name, command in checks]
        return [future.result() for future in concurrent.futures.as_completed(futures)]


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--jobs", type=int, default=4, help="maximum simultaneous checks (default: 4)")
    parser.add_argument("--go", default="go", help="Go executable")
    parser.add_argument("--race-package", action="append", default=[], help="affected package; repeat as needed")
    parser.add_argument("--timeout", type=int, default=1200, help="maximum seconds per check")
    args = parser.parse_args(argv)
    if args.jobs < 1 or args.timeout < 1:
        parser.error("jobs and timeout must be positive")
    root = pathlib.Path(__file__).resolve().parent.parent
    output = pathlib.Path(tempfile.mkdtemp(prefix="baseharbor-local-qualification-"))
    cancelled = threading.Event()
    for signum in (signal.SIGINT, signal.SIGTERM):
        signal.signal(signum, lambda *_: cancelled.set())
    env = os.environ.copy()
    # Native acceptance uses its separate isolated Docker/Podman authorities.
    # A caller's acceptance flags must not trigger them in this local runner.
    env = {key: value for key, value in env.items() if not key.startswith("BASEHARBOR_")}
    # Share compilation caches, but bound each Go worker's CPU parallelism.
    env.setdefault("GOMAXPROCS", str(max(1, (os.cpu_count() or 1) // args.jobs)))
    started = time.monotonic()
    print(f"Checks: {args.jobs} parallel workers; output: {output}", flush=True)
    try:
        with tempfile.TemporaryDirectory(prefix="baseharbor-source-snapshot-") as temporary:
            snapshot = pathlib.Path(temporary) / "repo"
            source = snapshot_source(root, snapshot)
            print(f"Source: {source['commit']}; tree: {source['tree']}", flush=True)
            checks = [
                ("go-tests", [args.go, "test", "./..."]),
                ("go-vet", [args.go, "vet", "./..."]),
                ("cli-build", [args.go, "build", "-o", str(output / "baha"), "./cmd/baha"]),
                ("docs-en", [sys.executable, "-m", "mkdocs", "build", "--strict", "--site-dir", str(output / "site-en")]),
                ("docs-de", [sys.executable, "-m", "mkdocs", "build", "--strict", "--config-file", "mkdocs.de.yml", "--site-dir", str(output / "site-de")]),
                ("docs-audit", ["bash", "scripts/documentation-audit.sh"]),
                ("readability", ["bash", "scripts/source-readability-audit.sh"]),
                ("script-tests", [sys.executable, "-m", "unittest", "discover", "-s", "scripts", "-p", "test_*.py"]),
            ]
            if args.race_package:
                checks.insert(1, ("go-race", [args.go, "test", "-race", *args.race_package]))
            results = run_checks(checks, snapshot, output, args.jobs, env, args.timeout, cancelled)
        receipt = {"source": source, "jobs": args.jobs, "seconds": round(time.monotonic() - started, 3),
                   "race_packages": args.race_package, "checks": sorted(results, key=lambda item: item["name"]),
                   "native_runtime_qualified": False, "pre_release_ready": False}
        (output / "result.json").write_text(json.dumps(receipt, indent=2) + "\n")
        print(f"Result: {output / 'result.json'}", flush=True)
        return 0 if all(result["status"] == "pass" for result in results) else 1
    except (OSError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"Qualification failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
