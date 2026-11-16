#!/usr/bin/env python3
"""Fail closed on any missing, failed, cancelled or skipped develop integration job."""
import json
import os
import sys

REQUIRED = ("resolve", "ecosystem_source", "selected_gate", "consumer_integration")


def require_success(jobs):
    if not isinstance(jobs, dict):
        raise ValueError("develop acceptance results missing")
    for name in REQUIRED:
        job = jobs.get(name)
        result = job.get("result") if isinstance(job, dict) else None
        if result != "success":
            raise ValueError(f"develop acceptance incomplete: {name}={result}")
    return True


if __name__ == "__main__":
    try:
        require_success(json.loads(os.environ["JOB_RESULTS"]))
    except (KeyError, ValueError, TypeError, json.JSONDecodeError) as error:
        sys.exit(str(error))
    print("Four-repo source, Docker/Podman lifecycle and native Console/Connector integration passed.")
