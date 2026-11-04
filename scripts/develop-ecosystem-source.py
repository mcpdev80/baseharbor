#!/usr/bin/env python3
"""Verify exact Core/Demo/Console/Connector source relationships before runtime acceptance.

This is a source preflight, never a replacement for Docker/Podman gates.
The caller supplies four immutable SHAs; no moving refs are trusted within a run.
"""
import argparse
import base64
import json
import re
import subprocess

REPOS = {
    "demo": "mcpdev80/baseharbor-demo",
    "console": "mcpdev80/baseharbor-console",
    "connector": "mcpdev80/baseharbor-node-connector",
}
SHA = re.compile(r"^[0-9a-f]{40}$")


def exact(value):
    return isinstance(value, str) and SHA.fullmatch(value) is not None


def validate(core, demo, console, connector, demo_core, console_candidate, connector_candidate):
    pins = {"core": core, "demo": demo, "console": console, "connector": connector}
    if not all(exact(sha) for sha in pins.values()):
        raise ValueError("all four source identities must be full immutable commit SHAs")
    if demo_core.strip() != core:
        raise ValueError("Demo's Core source identity does not match the Core candidate")
    if not isinstance(console_candidate, dict) or not isinstance(connector_candidate, dict):
        raise ValueError("Console/Connector source manifests missing")
    if set(console_candidate) != {"schema", "release", "core", "demo"}:
        raise ValueError("Console source manifest fields differ")
    if set(connector_candidate) != {"schema", "release", "core", "demo", "console"}:
        raise ValueError("Connector source manifest fields differ")
    if console_candidate["schema"] != "baseharbor.ecosystem-candidate/v1" or (
        connector_candidate["schema"] != console_candidate["schema"]
    ):
        raise ValueError("ecosystem source manifest schema differs")
    release = console_candidate["release"]
    if not isinstance(release, str) or not re.fullmatch(r"0\\.[0-9]+\\.[0-9]+", release):
        raise ValueError("ecosystem source manifest release label is invalid")
    if connector_candidate["release"] != release:
        raise ValueError("Console/Connector source manifest release labels differ")
    for role in ("core", "demo"):
        if console_candidate[role] != pins[role] or connector_candidate[role] != pins[role]:
            raise ValueError(role + " source identity differs across consumers")
    if connector_candidate["console"] != console:
        raise ValueError("Connector's Console source identity does not match the Console candidate")
    return {"schema": "baseharbor.develop-ecosystem-source/v1",
            "sources": pins, "consumer_label": release,
            "result": "source-compatible", "runtime_qualified": False}


def read_source(repo, sha, path):
    if repo not in REPOS.values() or not exact(sha) or path not in (
        "baseharbor-core.ref", "integration-candidate.json"
    ):
        raise ValueError("unreviewed immutable source request")
    reply = json.loads(subprocess.check_output([
        "gh", "api", f"repos/{repo}/contents/{path}?ref={sha}"
    ], text=True))
    if reply.get("type") != "file" or reply.get("encoding") != "base64" or not (
        0 < reply.get("size", 0) <= 65536
    ):
        raise ValueError("source file metadata differs")
    return base64.b64decode(reply["content"], validate=True).decode("utf-8")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for role in ("core", "demo", "console", "connector"):
        parser.add_argument("--" + role, required=True)
    args = parser.parse_args()
    try:
        report = validate(
            args.core, args.demo, args.console, args.connector,
            read_source(REPOS["demo"], args.demo, "baseharbor-core.ref"),
            json.loads(read_source(REPOS["console"], args.console, "integration-candidate.json")),
            json.loads(read_source(REPOS["connector"], args.connector, "integration-candidate.json")),
        )
    except (ValueError, KeyError, TypeError, json.JSONDecodeError, subprocess.CalledProcessError) as exc:
        parser.exit(1, "develop ecosystem source UNKNOWN/INCOMPATIBLE: " + str(exc) + "\n")
    print(json.dumps(report, sort_keys=True))


if __name__ == "__main__":
    main()
