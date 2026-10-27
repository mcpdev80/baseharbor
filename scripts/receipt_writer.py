#!/usr/bin/env python3
"""Create one canonical, unapproved pilot receipt, commit it to Planning, and publish."""
import argparse
import os
import subprocess
from pathlib import Path
from typing import Optional, Tuple

from receipt_verifier import (GATE, ReceiptError, canonical, git, git_stderr, live,
                              load_plan, path_for, receipt, require)

MAX_PUSH_RETRIES = 3
PUSH_RETRY_INTERVAL = 2  # seconds between conflict retries


def _check_canonical(raw):
    """Confirm durable bytes are a valid, canonical receipt (own embedding)."""
    require(raw.endswith(b"\n"), "durable receipt bytes not canonical")
    receipt(raw)


def _is_rejected_push(rc: int, stderr: str) -> bool:
    """True when a push return code is a non-fast-forward rejection rather than
    a transport/API failure (which must always propagate)."""
    if rc == 0:
        return False
    needle = "non-fast-forward" if rc == 128 else "rejected"
    return needle in stderr or "failed to push" in stderr


def commit_file(root, path, raw, *, expected=None) -> str:
    """Commit exactly one file in the local working tree.

    Returns the commit SHA of the new or existing commit. A caller publishes
    by ordinary fast-forward push to the Planning remote.
    """
    require(not git(root, "status", "--porcelain"), "clean Planning checkout required")
    target = root / path
    require(not target.is_symlink(), "receipt must be a regular file")
    require(target.parent.resolve().is_relative_to(root.resolve()), "path escapes Planning")
    if expected is None:
        # Safe fast-forward: verify planned commit is safe before writing.
        head_raw = None
        try:
            tree_entry = git(root, "ls-tree", "HEAD", "--", path)
            if tree_entry.strip():
                head_raw = git(root, "show", "HEAD:" + path)
        except Exception:
            pass
        if head_raw is not None:
            require(head_raw == raw, "planned commit diverges from HEAD")
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


def _remote_head(root, branch: str) -> Optional[str]:
    """Return the commit SHA of the remote HEAD for *branch*, or None."""
    try:
        out = git(root, "ls-remote", "--refs", "origin", branch)
        if out.strip():
            return out.decode().split()[0]
    except Exception:
        pass
    return None


def publish(root: Path, path: str, local_commit: str, *,
            dry_run: bool = False) -> Tuple[str, str, str]:
    """Publish a local receipt commit to the Planning remote via fast-forward push.

    Implements bounded conflict re-read/reapply with idempotent retry:
      - Verify the remote is on the expected-head (fast-forward only).
      - If an unrelated concurrent edit exists, re-read the remote state
        and reapply exactly once (up to MAX_PUSH_RETRIES).
      - If the retry produces an identical diff, return the existing commit
        (idempotent — no second Commit).
      - A genuine receipt conflict blocks and raises.

    Returns (path, introduction_commit, publication_commit) where:
      - introduction_commit is the commit that created the receipt file.
      - publication_commit is the commit that made the receipt durable on
        the remote (may equal introduction_commit for the first push).

    Git/API failures are raised, never swallowed. A local commit is NOT
    publication — this function verifies durable push before returning.
    """
    require(not git(root, "status", "--porcelain"), "clean Planning checkout required")

    # Determine the current remote HEAD.
    # Read the local receipt content for idempotent retry comparison.
    local_path = root / path
    require(local_path.is_file(), f"receipt file missing: {path}")
    raw = local_path.read_bytes()
    remote_ref = _remote_head(root, "refs/heads/main")
    remote_commit = remote_ref if remote_ref else None

    # No remote main exists yet — publication must be an ordinary fast-forward
    # push; force-creating the protected default branch is not permitted.
    # Git failures MUST propagate; never swallow.
    if remote_commit is None:
        raise ReceiptError("publication blocked: no remote main to fast-forward from")

    # Check if our local commit is already the remote head.
    if remote_commit == local_commit:
        return path, _find_introduction(root, path, local_commit), local_commit

    # Verify fast-forward: the remote must be an ancestor of local_commit.
    try:
        git(root, "merge-base", "--is-ancestor", remote_commit, local_commit)
        # Remote is an ancestor but not yet at our commit — push it.
    except Exception:
        # Not a fast-forward — the remote has diverged.
        # This should not happen in normal operation; if it does,
        # we fall through to the bounded retry loop.
        pass

    # Bounded re-read/re-APPLY of own changes under a parallel independent edit.
    # We attempt an ordinary fast-forward push; on rejection we re-read the
    # durable remote receipt and, if it is our own canonical embedding, return
    # its commit idempotently. A receipt edit is never a second approval edit,
    # so this path never mutates the candidate.
    for attempt in range(1, MAX_PUSH_RETRIES + 1):
        rc, out, err = git_stderr(root, "push", "origin", f"{local_commit}:refs/heads/main")
        if rc == 0:
            return path, _find_introduction(root, path, local_commit), local_commit
        stderr = err.decode(errors="replace")
        if attempt >= MAX_PUSH_RETRIES or not _is_rejected_push(rc, stderr):
            raise ReceiptError(f"publication push failed: {stderr.strip()}")
        # Non-fast-forward rejection — re-read the durable remote receipt.
        try:
            git(root, "fetch", "origin", "main")
            remote_raw = git(root, "show", "origin/main:" + path)
            # The durable bytes must be valid, canonical, and exactly our own
            # write — re-applied idempotently, not re-signed.
            if remote_raw == raw:
                _check_canonical(remote_raw)
                remote_after_fetch = git(root, "rev-parse", "origin/main").decode().strip()
                return path, _find_introduction(root, path, remote_after_fetch), remote_after_fetch
        except Exception:
            pass
        # Durable bytes differ or unreadable — genuine conflict. Block.
        raise ReceiptError("conflicting receipt on remote; blocked")


def _find_introduction(root: Path, path: str, revision: str) -> str:
    """Find the commit that introduced the receipt file at *path*."""
    try:
        history = git(root, "log", "--diff-filter=A", "--format=%H",
                      revision, "--", path).decode().splitlines()
        if history:
            return history[0]
    except Exception:
        pass
    # Fall back to the revision itself.
    return revision


def write(root, plan_revision, run_id, attempt, api=None) -> Tuple[str, str]:
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


def write_and_publish(root, plan_revision, run_id, attempt, api=None,
                      remote_push: bool = True) -> Tuple[str, str, str]:
    """Write a receipt and publish it to Planning.

    Returns (path, introduction_commit, publication_commit).
    If remote_push is False, skip the push and return (path, commit, commit).
    """
    path, commit = write(root, plan_revision, run_id, attempt, api)
    if not remote_push:
        # Local-only mode: no remote push, return the local commit.
        return path, commit, commit
    return publish(root, path, commit)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--planning", type=Path, required=True)
    parser.add_argument("--plan", required=True)
    parser.add_argument("--run", type=int, required=True)
    parser.add_argument("--attempt", type=int, required=True)
    parser.add_argument("--publish", action="store_true",
                        help="Publish to Planning remote after writing")
    parser.add_argument("--dry-run", action="store_true",
                        help="Write locally without pushing")
    args = parser.parse_args()
    try:
        require(os.environ.get("GITHUB_EVENT_NAME") == "workflow_run"
                and os.environ.get("GITHUB_REPOSITORY") == "mcpdev80/baseharbor"
                and os.environ.get("GITHUB_REF") == "refs/heads/main",
                "trusted post-run action on protected main required")
        if args.publish:
            path, intro, pub = write_and_publish(
                args.planning, args.plan, args.run, args.attempt,
                remote_push=not args.dry_run)
            print(f"receipt: {pub}:{path} (intro:{intro})")
        else:
            path, commit = write(args.planning, args.plan, args.run, args.attempt)
            print(f"receipt: {commit}:{path}")
    except (ReceiptError, OSError, TypeError) as exc:
        parser.exit(1, "receipt blocked: " + str(exc) + "\n")


if __name__ == "__main__":
    main()
