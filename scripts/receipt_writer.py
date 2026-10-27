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


def _current_remote_bytes(root, path: str) -> Optional[bytes]:
    """Return the receipt bytes at the current remote HEAD, or None (absent)."""
    try:
        return git(root, "show", "origin/main:" + path)
    except Exception:
        return None


def _reapply_onto(root, path: str, raw, *, head: str) -> bool:
    """Re-apply exactly our own receipt edit onto the fresh remote HEAD.

    Preserves unrelated concurrent changes by fast-forwarding onto *head* and
    committing identical bytes only when the prior receipt is still absent.
    Returns True on a successful re-apply commit, False when a receipt already
    exists on the fresh remote (caller must not overwrite it) — genuine
    conflicts are left for the caller to block.
    """
    current = _current_remote_bytes(root, path)
    if current is not None:
        # A receipt already exists on the fresh remote — do not touch it.
        return False
    try:
        git(root, "reset", "--hard", head)
    except Exception:
        return False
    if _current_remote_bytes(root, path) is not None:
        # An unrelated edit added its own receipt here; never overwrite it.
        return False
    if git(root, "status", "--porcelain").decode().strip():
        # Preserve unrelated concurrent working-tree state; do not commit it.
        return False
    target = root / path
    target.parent.mkdir(parents=True, exist_ok=True)
    with target.open("wb") as stream:
        stream.write(raw)
    git(root, "add", "--", path)
    git(root, "commit", "-m", f"receipt: {GATE} record (re-applied)", "--", path)
    return True


def publish(root: Path, path: str, local_commit: str, *,
            dry_run: bool = False) -> Tuple[str, str, str]:
    """Publish a local receipt commit to the Planning remote via fast-forward push.

    Implements bounded re-apply with idempotent retry under an unrelated
    concurrent edit:
      - Verify the remote is on the expected-head (fast-forward only).
      - On a non-fast-forward push rejection, re-read the fresh remote HEAD,
        re-apply exactly our own receipt edit onto that HEAD (preserving the
        unrelated concurrent change), and ordinary-push again (up to
        MAX_PUSH_RETRIES).
      - If the fresh remote already carries our own canonical receipt bytes,
        return that commit idempotently (no second Commit).
      - A genuine receipt conflict (durable receipt with different bytes, or a
        receipt added by the unrelated edit) blocks and raises.

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

    # Bounded re-apply of own changes under a parallel independent edit.
    #
    # We attempt an ordinary fast-forward push; on a non-fast-forward rejection
    # we re-read the fresh remote HEAD, re-apply exactly our own receipt edit
    # onto that HEAD (preserving the unrelated concurrent change), and ordinary-
    # push again. This recovers when the receipt is still absent on the fresh
    # remote even though the local commit was rejected by the unrelated edit.
    #
    # The same edit is never re-signed: re-application commits identical bytes.
    # A genuine receipt conflict (the durable receipt exists with different
    # bytes) blocks and raises.
    for attempt in range(1, MAX_PUSH_RETRIES + 1):
        rc, out, err = git_stderr(root, "push", "origin", f"{local_commit}:refs/heads/main")
        if rc == 0:
            return path, _find_introduction(root, path, local_commit), local_commit
        stderr = err.decode(errors="replace")
        if attempt >= MAX_PUSH_RETRIES or not _is_rejected_push(rc, stderr):
            raise ReceiptError(f"publication push failed: {stderr.strip()}")
        # Non-fast-forward rejection — fresh remote diverged.
        try:
            git(root, "fetch", "origin", "main")
        except Exception:
            raise ReceiptError("publication failed: fetch after rejection")
        remote_after_fetch = git(root, "rev-parse", "origin/main").decode().strip()
        remote_bytes = _current_remote_bytes(root, path)
        # Durable receipt exists on the fresh remote.
        if remote_bytes is not None:
            _check_canonical(remote_bytes)
            if remote_bytes == raw:
                # Identical canonical bytes — return idempotently.
                after = git(root, "rev-parse", "origin/main").decode().strip()
                return path, _find_introduction(root, path, after), after
            # Genuine conflict: durable receipt differs. Block.
            raise ReceiptError("conflicting receipt on remote; blocked")
        # The receipt is still absent on the fresh remote: re-apply our own
        # edit onto the fresh HEAD and ordinary-push.
        if _reapply_onto(root, path, raw, head=remote_after_fetch):
            rc2, out2, err2 = git_stderr(root, "push", "origin", f"HEAD:refs/heads/main")
            if rc2 == 0:
                reapplied = git(root, "rev-parse", "HEAD").decode().strip()
                return path, _find_introduction(root, path, reapplied), reapplied
            reapplied_stderr = err2.decode(errors="replace")
            if not _is_rejected_push(rc2, reapplied_stderr):
                raise ReceiptError(f"publication re-apply push failed: {reapplied_stderr.strip()}")
            # Still diverged after re-apply (fast-forward exhausted): fall back to
            # idempotent return only if the fresh remote now carries our own bytes.
            remote_bytes2 = _current_remote_bytes(root, path)
            if remote_bytes2 is not None and remote_bytes2 == raw:
                _check_canonical(remote_bytes2)
                after2 = git(root, "rev-parse", "origin/main").decode().strip()
                return path, _find_introduction(root, path, after2), after2
            raise ReceiptError("conflicting receipt on remote after re-apply; blocked")
        # Receipt present on remote with different bytes — genuine conflict.
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
