#!/usr/bin/env python3
"""Bind every selected root manifest to its exact artifact/run/attempt."""
import argparse
import json
import pathlib
import re


def verify(root, run_id, candidate, demo, current_attempt):
    families = {"gate-evidence": 41, "adoption-evidence": 4, "v0421-ha-evidence": 8, "reference-journey-evidence": 2}
    for family, count in families.items():
        manifests = sorted((root / family).glob("*/manifest.json"))
        if len(manifests) != count:
            raise ValueError(f"{family}: expected {count} root manifests, found {len(manifests)}")
        for path in manifests:
            if path.is_symlink() or path.parent.is_symlink():
                raise ValueError("evidence symlink is forbidden")
            manifest = json.loads(path.read_text())
            match = re.fullmatch(r"(.+)-([0-9]+)-([1-9][0-9]*)", path.parent.name)
            attempt = manifest.get("workflow_run_attempt")
            if not match or match[2] != run_id or type(attempt) is not int or not 1 <= attempt <= current_attempt or match[3] != str(attempt):
                raise ValueError(f"wrong artifact attempt: {path.parent.name}")
            if manifest.get("workflow_run_id") != run_id or manifest.get("candidate_sha") != candidate or manifest.get("demo_ref") != demo:
                raise ValueError(f"wrong manifest binding: {path.parent.name}")
            runtime = manifest.get("runtime", "static")
            if family == "gate-evidence":
                key = f'pre-release-gate-{runtime}-{manifest.get("gate")}-evidence'
                if manifest.get("id") != f'{runtime}/{manifest.get("gate")}':
                    raise ValueError("atomic runtime/gate identity differs from its logical id")
            elif family == "adoption-evidence":
                key = f'pre-release-adoption-{manifest.get("gate")}-evidence'
            elif family == "v0421-ha-evidence":
                key = f'pre-release-v0421-ha-{runtime}-{manifest.get("group")}-evidence'
            else:
                key = f'pre-release-reference-journey-evidence-{runtime}'
            if match[1] != key:
                raise ValueError(f"logical identity differs from artifact: {path.parent.name}")
            cleanup = manifest.get("cleanup_outcome")
            if family != "adoption-evidence" and cleanup != ("skipped" if runtime == "static" else "success"):
                raise ValueError(f"cleanup was not successful: {path.parent.name}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=pathlib.Path)
    parser.add_argument("run_id")
    parser.add_argument("candidate")
    parser.add_argument("demo")
    parser.add_argument("current_attempt", type=int)
    args = parser.parse_args()
    try:
        verify(args.root, args.run_id, args.candidate, args.demo, args.current_attempt)
    except (ValueError, OSError) as error:
        parser.exit(1, f"attempt binding verification failed: {error}\n")
    print("All 55 gate manifests match candidate, demo pin, source run, artifact attempt and cleanup.")


if __name__ == "__main__":
    main()
