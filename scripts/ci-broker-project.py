#!/usr/bin/env python3
"""Read the actual isolated fixture project from secret-safe app status."""
import json
import re
import sys


def project_from_status(status, application):
    if not isinstance(status, dict):
        raise ValueError("invalid fixture status")
    project = status.get("project")
    if (status.get("target") != "ci" or status.get("application") != application
            or status.get("environment") != "dev" or status.get("ready") is not True
            or not isinstance(project, str)
            or re.fullmatch(r"[a-z0-9][a-z0-9_-]*", project) is None):
        raise ValueError("unexpected fixture status")
    return project


if __name__ == "__main__":
    try:
        if len(sys.argv) != 2 or sys.argv[1] not in {"broker-demo", "other-demo", "perf-demo"}:
            raise ValueError("unexpected fixture application")
        print(project_from_status(json.load(sys.stdin), sys.argv[1]))
    except (ValueError, OSError):
        print("broker project: invalid fixture status", file=sys.stderr)
        sys.exit(1)
