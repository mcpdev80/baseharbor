#!/usr/bin/env python3
"""Create one canonical, unapproved pilot receipt and commit it to Planning."""
import argparse
import os
from pathlib import Path

from receipt_verifier import (GATE, ReceiptError, canonical, git, live, load_plan,
                              path_for, receipt, require)


def commit_file(root, path, raw, *, expected=None):
    """Commit exactly one file; a caller publishes by ordinary fast-forward push."""
    require(not git(root, "status", "--porcelain"), "clean Planning checkout required")
    target = root / path
    require(not target.is_symlink(), "receipt must be a regular file")
    require(target.parent.resolve().is_relative_to(root.resolve()), "path escapes Planning")
    if expected is None:
        if target.exists():
            require(target.read_bytes() == raw, "conflicting existing receipt")
            history = git(root, "log", "--full-history", "--format=%H", "HEAD",
                          "--", path).decode().splitlines()
            require(len(history) == 1, "existing receipt has unexpected edit history")
            return history[0]
        target.parent.mkdir(parents=True, exist_ok=True)
        with target.open("xb") as stream:
            stream.write(raw)
    else:
        require(target.is_file() and target.read_bytes() == expected, "receipt changed before approval")
        target.write_bytes(raw)
    git(root, "add", "--", path)
    git(root, "commit", "-m", f"receipt: {GATE} {'approve' if expected else 'record'}", "--", path)
    return git(root, "rev-parse", "HEAD").decode().strip()


def write(root, plan_revision, run_id, attempt, api=None):
    plan = load_plan(root, plan_revision)
    value = {"gate": GATE, "run": {"id": run_id, "attempt": attempt},
             "sha": plan["candidate_sha"], "status": "pass", "plan": plan_revision,
             "approved": None}
    raw = canonical(value)
    receipt(raw)
    live(value, plan, **({"api": api} if api is not None else {}))
    path = path_for(value)
    commit = commit_file(root, path, raw)
    return path, commit


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--planning", type=Path, required=True)
    parser.add_argument("--plan", required=True)
    parser.add_argument("--run", type=int, required=True)
    parser.add_argument("--attempt", type=int, required=True)
    args = parser.parse_args()
    try:
        require(os.environ.get("GITHUB_EVENT_NAME") == "workflow_run"
                and os.environ.get("GITHUB_REPOSITORY") == "mcpdev80/baseharbor"
                and os.environ.get("GITHUB_REF") == "refs/heads/main",
                "trusted post-run action on protected main required")
        path, commit = write(args.planning, args.plan, args.run, args.attempt)
        print(f"receipt: {commit}:{path}")
    except (ReceiptError, OSError, TypeError) as exc:
        parser.exit(1, "receipt blocked: " + str(exc) + "\n")


if __name__ == "__main__":
    main()
