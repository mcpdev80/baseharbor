#!/usr/bin/env python3
r"""Produce the bounded plan fixture with an automatic task-link reference.

This is the missing building block from the p2-shadow-runbook section 3
("Automatic task link — missing building block"). The plan fixture at
`evidence/receipt-plans/<GATE>.json` must contain a correct `"task":
"tasks/<task-id>.md"` field that the receipt writer consumes via
`load_plan()`. Neither `receipt_writer.py` nor `receipt_verifier.py`
writes the plan fixture — the producer role is the trusted planner
inside the post-run workflow, exercised by this script.

Contract enforced by load_plan(receipt_verifier.py:101-112):
  - Exactly 7 fields: gate, repository, workflow_id, head_sha,
    candidate_sha, base_sha, task
  - 40-char full SHAs for head/candidate/base
  - task matches `tasks/[a-z0-9-]+\.md`
  - gate == GATE ("public-contracts"), repository == REPOSITORY
"""
import argparse
import json
import os
import re
import subprocess
from pathlib import Path

GATE = "public-contracts"
REPOSITORY = "mcpdev80/baseharbor"
REPOSITORY_ID = 1360667669
PLAN_PATH = "evidence/receipt-plans/public-contracts.json"
TASK_RE = re.compile(r"tasks/[a-z0-9-]+\.md$", re.ASCII)
CLOSERE = re.compile(r"(?:closes|fixes|resolves)\s+#(\d+)", re.I)


class TaskLinkError(ValueError):
    """The task link cannot be produced."""


def require(condition, reason):
    if not condition:
        raise TaskLinkError(reason)


def sha40(value):
    return isinstance(value, str) and re.fullmatch(r"[0-9a-f]{40}", value) is not None


def resolve_task_from_pr(pr_number, planning_root, api=None):
    """Resolve the automatic task link from the PR number.

    Reads the PR body via the GitHub API to find a single issue closer like
    "Closes #123", then validates that the referenced issue points to a valid
    Planning task. Missing or ambiguous references block.

    Parameters
    ----------
    pr_number : str
        The PR number (e.g. "1092").
    planning_root : Path
        Path to the Planning repository (validation context).
    api : callable, optional
        API function returning a dict for a GitHub API path. Defaults to
        ``_gh_api``.

    Returns
    -------
    str
        The resolved task reference, e.g. "tasks/t-20261022-r27-pilot.md".

    Raises
    ------
    TaskLinkError
        If no issue is referenced, the reference is ambiguous, or the
        referenced issue does not point to a valid task.
    """
    if pr_number is None or pr_number == "":
        raise TaskLinkError("PR number is empty; cannot resolve task link")

    if api is None:
        api = _gh_api

    try:
        pr_data = api("repos/%s/pulls/%s" % (REPOSITORY, pr_number))
    except TaskLinkError as exc:
        raise TaskLinkError("failed to fetch PR #%s: %s" % (pr_number, exc)) from exc

    body = pr_data.get("body") or ""
    closes = CLOSERE.findall(body)

    if not closes:
        raise TaskLinkError(
            "PR #%s body has no 'Closes/Fixes/Resolves #NNN' reference; "
            "task link cannot be auto-resolved" % pr_number
        )

    if len(closes) > 1:
        raise TaskLinkError(
            "PR #%s has ambiguous issue references: %s; only a single "
            "Closes/Fixes reference is accepted" % (pr_number, closes)
        )

    issue_number = closes[0]
    try:
        issue_data = api("repos/%s/issues/%s" % (REPOSITORY, issue_number))
    except TaskLinkError as exc:
        raise TaskLinkError(
            "referenced issue #%s not found: %s" % (issue_number, exc)
        ) from exc

    title = issue_data.get("title") or ""
    issue_body = issue_data.get("body") or ""
    combined = title + " " + issue_body

    task_match = re.search(r"tasks/([a-z0-9-]+\.md)", combined)
    if task_match:
        task = "tasks/%s" % task_match.group(1)
    elif title.startswith("t-") and re.fullmatch(r"t-[a-z0-9-]+", title):
        task = "tasks/%s.md" % title
    else:
        raise TaskLinkError(
            "referenced issue #%s does not reference a Planning task; "
            "issue title=%r" % (issue_number, title)
        )

    if TASK_RE.fullmatch(task):
        return task
    raise TaskLinkError("resolved task reference %r is invalid" % task)


def _gh_api(path):
    """Fetch data from the GitHub API using the `gh` CLI."""
    token = os.environ.get("GITHUB_TOKEN", "")
    require(token, "GITHUB_TOKEN must be set")
    url = "https://api.github.com/%s" % path
    result = subprocess.run(
        ["gh", "api", url, "-H", "Authorization: token %s" % token,
         "-H", "Accept: application/vnd.github+json"],
        capture_output=True, text=True,
    )
    require(result.returncode == 0, "GitHub API request failed: %s" % result.stderr)
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise TaskLinkError("invalid JSON from GitHub API: %s" % exc) from exc


def produce_plan_fixture(planning_root, workflow_id, head_sha, candidate_sha,
                         base_sha, task, pr_number=None, *, dry_run=False,
                         _api=None):
    """Write the plan fixture with automatic task link and return the commit SHA.

    Parameters
    ----------
    planning_root : Path
        Checked-out Planning repository (protected main).
    workflow_id : int
        GitHub workflow ID of the producer CI run.
    head_sha : str
        40-char commit SHA of the PR head.
    candidate_sha : str
        40-char integration/candidate SHA (tested merge base).
    base_sha : str
        40-char commit SHA of the PR base branch.
    task : str
        Automatic task-link reference, e.g. "tasks/t-20261026-r27-pilot.md".
    pr_number : str, optional
        The PR number that triggered the producer CI. If provided, the task is
        auto-resolved from the trusted PR->task binding. Without it, the task
        must be caller-supplied and will NOT produce a real evidence link.
    dry_run : bool
        If True, emit the fixture to stdout instead of committing.
    _api : callable, optional
        API function for PR data resolution. Defaults to ``_gh_api``.

    Returns
    -------
    str
        The commit SHA of the new plan-fixture commit (or the last plan
        commit on dry_run).

    Raises
    ------
    TaskLinkError
        If auto-resolution fails (missing/ambiguous PR->task binding).
    """
    require(sha40(head_sha), "head_sha must be a 40-char hex SHA")
    require(sha40(candidate_sha), "candidate_sha must be a 40-char hex SHA")
    require(sha40(base_sha), "base_sha must be a 40-char hex SHA")
    require(isinstance(workflow_id, int) and workflow_id > 0,
            "workflow_id must be a positive integer")

    if pr_number is not None and pr_number != "":
        if _api is None:
            _api = _gh_api
        resolved = resolve_task_from_pr(pr_number, planning_root, _api)
        # If caller-supplied task differs from resolved, the caller is not
        # using the trusted PR->task binding. We proceed with the resolved
        # task — that's the evidence link.
        task = resolved
        require(isinstance(task, str) and TASK_RE.fullmatch(task),
                "resolved task must match tasks/[a-z0-9-]+\\.md")
        print("tasklink: auto-resolved task from PR %s: %s" % (pr_number, task),
              flush=True)
    else:
        # No PR number — task is caller-supplied but NOT auto-linked.
        # This means plan.task is set, but there is NO real evidence link.
        require(isinstance(task, str) and TASK_RE.fullmatch(task),
                "task must be a valid task reference matching tasks/[a-z0-9-]+\\.md")
        print("tasklink: no PR number; task is caller-supplied without "
              "auto-resolution — plan.task alone is NOT evidence publication",
              flush=True)

    plan = {
        "gate": GATE,
        "repository": REPOSITORY,
        "workflow_id": workflow_id,
        "head_sha": head_sha,
        "candidate_sha": candidate_sha,
        "base_sha": base_sha,
        "task": task,
    }

    target = Path(planning_root) / PLAN_PATH

    # Idempotency: if the fixture already exists with the same content,
    # return the existing commit rather than creating a new one.
    if target.exists() and not dry_run:
        existing_raw = target.read_bytes()
        try:
            existing_value = json.loads(existing_raw)
            if (isinstance(existing_value, dict) and
                    set(existing_value) == set(plan) and
                    all(existing_value.get(k) == plan.get(k) for k in plan)):
                # Content matches — return the current commit for the path.
                return _current_commit(planning_root, str(target))
        except (json.JSONDecodeError, ValueError, TypeError):
            pass

    raw = (json.dumps(plan, sort_keys=True, separators=(",", ":")) + "\n").encode()
    target.parent.mkdir(parents=True, exist_ok=True)

    if dry_run:
        print(raw.decode(), end="")
        return _current_commit(planning_root, str(target))

    target.write_bytes(raw)

    # Commit the plan fixture. The file may already be tracked with the
    # same content (idempotent overwrite), so force a re-hash by
    # removing and re-adding before committing.
    target.unlink()
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(raw)
    _git(planning_root, "add", "-f", str(target))
    _git(planning_root, "commit", "-m",
         f"plan: {GATE} fixture with task link {task}", str(target))

    return _git(planning_root, "rev-parse", "HEAD").decode().strip()


def _git(root, *args):
    """Run a git command inside `root`, raising on failure."""
    result = subprocess.run(["git", "-C", str(root), *args],
                            capture_output=True)
    if result.returncode != 0:
        raise TaskLinkError(f"git {args[0]} failed: {result.stderr.decode()}")
    return result.stdout


def publish_evidence(planning_root, task, receipt_path, candidate_sha,
                     plan_revision, _git_fn=None):
    """Write evidence reference to the Planning task card via W2 publication.

    Updates the task note file (e.g. tasks/t-xxx.md) with an 'evidence:'
    field that points to the published receipt path. This provides automatic
    evidence-Aktualisierung of the Planning card — no manual maintenance needed.

    Parameters
    ----------
    planning_root : Path
        Checked-out Planning repository.
    task : str
        Task reference, e.g. "tasks/t-xxx.md".
    receipt_path : str
        Published receipt path relative to the Planning root.
    candidate_sha : str
        40-char approved candidate SHA.
    plan_revision : str
        40-char approved plan revision SHA.
    _git_fn : callable, optional
        Git helper function. Defaults to ``_git``.

    Returns
    -------
    str
        The commit SHA that wrote the evidence reference.

    Raises
    ------
    TaskLinkError
        If the task note cannot be found, modified, or committed.
    """
    if _git_fn is None:
        _git_fn = _git

    task_path = Path(planning_root) / task
    require(task_path.exists(), "task note missing for evidence publication: %s" % task)

    existing = task_path.read_bytes()
    require(isinstance(existing, bytes) and 0 < len(existing) < 65536,
            "task note too large or empty")

    # Parse YAML front matter to find/update the evidence field.
    # Format: ---\nkey: value\n---\nbody
    text = existing.decode(errors="replace")
    lines = text.splitlines()

    # Find YAML front matter boundaries
    start = None
    end = None
    for i, line in enumerate(lines):
        if line.strip() == "---" and start is None:
            start = i
        elif line.strip() == "---" and start is not None:
            end = i
            break

    if start is None or end is None:
        # No YAML front matter — create one.
        evidence_line = "evidence: %s" % receipt_path
        new_text = "---\n%s\n---\n%s" % (evidence_line, "\n".join(lines))
    else:
        # Extract existing front matter and body
        front_lines = lines[start + 1:end]
        body_lines = lines[end + 1:]

        # Check if evidence field already exists
        evidence_found = False
        new_front = []
        for fl in front_lines:
            if fl.strip().startswith("evidence:"):
                new_front.append("evidence: %s" % receipt_path)
                evidence_found = True
            else:
                new_front.append(fl)

        if not evidence_found:
            new_front.append("evidence: %s" % receipt_path)

        new_text = "---\n" + "\n".join(new_front) + "\n---\n" + "\n".join(body_lines)

    new_bytes = new_text.encode()
    task_path.write_bytes(new_bytes)

    # Commit the evidence update. Force re-hash for idempotent overwrite.
    _git_fn(planning_root, "add", "-f", "--", str(task_path))
    _git_fn(planning_root, "commit", "-m",
            "tasklink: publish evidence %s for task %s (candidate: %s)" % (
                receipt_path, task, candidate_sha[:12]),
            "--", str(task_path))

    return _git_fn(planning_root, "rev-parse", "HEAD").decode().strip()


def _current_commit(root, path):
    """Return the commit that last touched `path`, or None."""
    try:
        lines = _git(root, "log", "-n", "1", "--format=%H", "--", path).decode().splitlines()
        if lines and lines[0]:
            return lines[0]
    except TaskLinkError:
        pass
    return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--planning", type=Path, required=True,
                        help="Path to the Planning repository checkout")
    parser.add_argument("--workflow-id", type=int, required=True,
                        help="GitHub workflow ID of the producer CI run")
    parser.add_argument("--head", required=True,
                        help="40-char commit SHA of the PR head")
    parser.add_argument("--candidate", required=True,
                        help="40-char integration/candidate SHA")
    parser.add_argument("--base", required=True,
                        help="40-char commit SHA of the PR base branch")
    parser.add_argument("--task", default=None,
                        help="Automatic task-link reference, e.g. 'tasks/t-20261026-r27-pilot.md'. "
                             "Omit to auto-resolve from --pr-number.")
    parser.add_argument("--pr-number", default=None,
                        help="PR number for auto-resolution of the task link. "
                             "If provided, the task is resolved from the trusted "
                             "PR->task binding (e.g. 'Closes #123' in PR body).")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print the fixture to stdout without committing")
    args = parser.parse_args()

    if args.task is None and args.pr_number is None:
        parser.error("one of --task or --pr-number is required")
    if args.task is not None and args.pr_number is not None:
        # Both provided: pr_number takes precedence (auto-resolution).
        print("tasklink: both --task and --pr-number provided; "
              "auto-resolving from PR takes precedence", flush=True)

    try:
        commit = produce_plan_fixture(
            args.planning,
            args.workflow_id,
            args.head,
            args.candidate,
            args.base,
            args.task,
            pr_number=args.pr_number,
            dry_run=args.dry_run,
        )
        print(commit)
    except (TaskLinkError, OSError, TypeError) as exc:
        parser.exit(1, "tasklink blocked: " + str(exc) + "\n")


if __name__ == "__main__":
    main()
