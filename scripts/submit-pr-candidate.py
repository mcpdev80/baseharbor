#!/usr/bin/env python3
"""Emit an explicit PR candidate event via an independently authenticated gh CLI.

Call only after all candidate changes have been pushed. A GitHub Actions
GITHUB_TOKEN cannot trigger another workflow through its label events.
"""
import argparse
import json
import os
import subprocess
import sys


def gh(method, path, *, data=None):
    args = ["gh", "api", "--method", method, path]
    if data is not None:
        args += ["--input", "-"]
    result = subprocess.run(
        args, input=json.dumps(data) if data is not None else None,
        text=True, capture_output=True, check=False,
    )
    if result.returncode:
        raise RuntimeError(f"gh api {method} {path}: {result.stderr.strip()}")
    return json.loads(result.stdout) if result.stdout.strip() else None


def publish(repo, number, expected_sha):
    if os.environ.get("BASEHARBOR_CANDIDATE_CI_ROLLOUT") != "enabled":
        raise RuntimeError("candidate CI rollout locked until v0.4.26 completion and explicit owner approval")
    if os.environ.get("GITHUB_ACTIONS") == "true":
        raise RuntimeError(
            "Run candidate submission outside GitHub Actions using independent "
            "GitHub App/PAT credentials, never a workflow GITHUB_TOKEN."
        )
    pr = gh("GET", f"repos/{repo}/pulls/{number}")
    if pr.get("draft"):
        raise RuntimeError("draft PR cannot trigger candidate CI")
    if pr["state"] != "open":
        raise RuntimeError("PR is not open")
    if pr["head"]["sha"] != expected_sha:
        raise RuntimeError(
            f"stale candidate: expected {expected_sha}, got {pr['head']['sha']}"
        )
    if pr["base"]["ref"] != "develop":
        raise RuntimeError("candidate PR does not target develop")
    labels = {label["name"] for label in pr.get("labels", [])}
    if "ci:validate" in labels:
        gh("DELETE", f"repos/{repo}/issues/{number}/labels/ci%3Avalidate")
    # Close the race between removing the old label and re-adding it.
    latest = gh("GET", f"repos/{repo}/pulls/{number}")
    if (latest["head"]["sha"] != expected_sha or latest["state"] != "open"
            or latest.get("draft") or latest["base"]["ref"] != "develop"):
        raise RuntimeError("candidate changed while preparing event; retry")
    gh("POST", f"repos/{repo}/issues/{number}/labels",
       data={"labels": ["ci:validate"]})
    print(f"candidate submitted: {repo}#{number} {expected_sha}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True)
    parser.add_argument("--pr", type=int, required=True)
    parser.add_argument("--head-sha", required=True)
    args = parser.parse_args()
    if not args.repo.count("/") == 1:
        parser.error("--repo must be owner/repository")
    if len(args.head_sha) != 40 or any(c not in "0123456789abcdef" for c in args.head_sha):
        parser.error("--head-sha must be full lowercase Git commit SHA")
    if not os.environ.get("GH_TOKEN"):
        parser.error("independent GH_TOKEN required; GITHUB_TOKEN alone is not accepted")
    try:
        publish(args.repo, args.pr, args.head_sha)
    except (RuntimeError, KeyError, ValueError) as error:
        print(f"candidate submission failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
