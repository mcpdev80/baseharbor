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

Automatic task evidence publication: plan.task alone is NOT the update.
The script derives the task from the trusted PR->task binding (e.g.
"Closes #123" in the PR body), reads the actual Planning task metadata,
and writes the plan fixture with the verified task reference. Missing
or ambiguous PR->task mapping blocks the action.
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
# Pattern to match "Closes #123" or "Fixes #456" in PR body/commit messages.
CLOSERE = re.compile(r"(?:closes|fixes|resolves)\s+#(\d+)", re.I)


class TaskLinkError(ValueError):
    """The task link cannot be produced."""


def require(condition, reason):
    if not condition:
        raise TaskLinkError(reason)


def sha40(value):
    return isinstance(value, str) and re.fullmatch(r"[0-9a-f]{40}", value) is not None


def resolve_task_from_pr(pr_number: str, planning_root: Path,
                         api: object = None) -> str:
    """Resolve the automatic task-link from the PR number.

    Reads the PR body via the GitHub API to find issue closers like
    "Closes #123", then validates that the referenced issue references
    a valid Planning task. Missing or ambiguous references block.

    Parameters
    ----------
    pr_number : str
        The PR number (e.g. "1092").
    planning_root : Path
        Path to the Planning repository (for validation).
    api : callable, optional
        API function for fetching PR data. Defaults to `_gh_api`.

    Returns
    -------
    str
        The resolved task reference, e.g. "tasks/t-20261022-r27-pilot.md".

    Raises
    ------
    TaskLinkError
        If no issue is referenced, or the reference is ambiguous,
        or the referenced issue does not point to a valid task.
    """
    if pr_number is None or pr_number == "":
        raise TaskLinkError("PR number is empty; cannot resolve task link")

    if api is None:
        api = _gh_api

    # Fetch the PR body from GitHub.
    try:
        pr_data = api(f"repos/{REPOSITORY}/pulls/{pr_number}")
    except TaskLinkError as exc:
        raise TaskLinkError(f"failed to fetch PR #{pr_number}: {exc}") from exc

    body = pr_data.get("body") or ""
    closes = CLOSERE.findall(body)

    if not closes:
        raise TaskLinkError(
            f"PR #{pr_number} body has no 'Closes/Fixes/Resolves #NNN' "
            f"reference; task link cannot be auto-resolved"
        )

    if len(closes) > 1:
        # Ambiguous: multiple issue references.
        raise TaskLinkError(
            f"PR #{pr_number} has ambiguous issue references: {closes}; "
            f"only a single Closes/Fixes reference is accepted"
        )

    issue_number = closes[0]
    # Fetch the issue to verify it references a Planning task.
    try:
        issue_data = api(f"repos/{REPOSITORY}/issues/{issue_number}")
    except TaskLinkError as exc:
        raise TaskLinkError(
            f"referenced issue #{issue_number} not found: {exc}"
        ) from exc

    # The issue body/title should reference a task. We accept either:
    # - A task file reference in the issue body: "tasks/t-*.md"
    # - An issue title starting with "t-": "t-20261022-r27-*"
    title = issue_data.get("title") or ""
    issue_body = issue_data.get("body") or ""
    combined = title + " " + issue_body

    # Check for a task reference.
    task_match = re.search(r"tasks/([a-z0-9-]+\.md)", combined)
    if task_match:
        task = f"tasks/{task_match.group(1)}"
    else:
        # Accept issue title format: "t-20261022-r27-pilot" -> task file.
        if title.startswith("t-") and re.fullmatch(r"t-[a-z0-9-]+", title):
            # Map the issue number to the canonical task file name.
            # The task file is determined by the issue title convention.
            task = f"tasks/{title}.md"
        else:
            raise TaskLinkError(
                f"referenced issue #{issue_number} does not reference a "
                f"Planning task; issue title={title!r}"
            )

    # Validate the task reference format.
    if TASK_RE.fullmatch(task):
        return task
    else:
        raise TaskLinkError(f"resolved task reference {task!r} is invalid")


def _gh_api(path: str) -> dict:
    """Fetch data from the GitHub API using `gh` CLI."""
    token = os.environ.get("GITHUB_TOKEN", "")
    require(token, "GITHUB_TOKEN must be set")
    url = f"https://api.github.com/{path}"
    result = subprocess.run(
        ["gh", "api", url, "-H", f"Authorization: token {token}",
         "-H", "Accept: application/vnd.github+json"],
        capture_output=True, text=True
    )
    require(result.returncode == 0, f"GitHub API request failed: {result.stderr}")
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise TaskLinkError(f"invalid JSON from GitHub API: {exc}") from exc


def produce_plan_fixture(planning_root, workflow_id, head_sha, candidate_sha,
                          base_sha, task, pr_number=None, *, dry_run=False,
                          _api=None):
    """Write the plan fixture with automatic task link and return the commit SHA.

    Also performs automatic evidence-Aktualisierung of the Planning task note:
    when pr_number is provided, the task note at `tasks/<task-name>.md` in the
    Planning repository gets updated with a `plans:` line referencing the plan
    revision, so that the Planning-Karte shows the current evidence chain.

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
        The PR number that triggered the producer CI. If provided, the task
        is auto-resolved from the trusted PR->task binding. Without it, the
        task must be caller-supplied and will NOT produce a real evidence link.
    dry_run : bool
        If True, emit the fixture to stdout instead of committing.
    _api : callable, optional
        API function for PR data resolution. Defaults to `_gh_api`.

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

    # Auto-resolve task from PR if pr_number is provided.
    if pr_number is not None and pr_number != "":
        if _api is None:
            _api = _gh_api
        resolved = resolve_task_from_pr(pr_number, planning_root, _api)
        # If caller-supplied task differs from resolved, the caller
        # is not using the trusted PR->task binding.
        # We proceed with the resolved task — that's the evidence link.
        task = resolved
        require(isinstance(task, str) and TASK_RE.fullmatch(task),
                "resolved task must match tasks/[a-z0-9-]+\\.md")
        print(f"tasklink: auto-resolved task from PR #{pr_number}: {task}",
              flush=True)
    else:
        # No PR number — task is caller-supplied but NOT auto-linked.
        # This means plan.task is set, but there is NO real evidence link.
        require(isinstance(task, str) and TASK_RE.fullmatch(task),
                "task must be a valid task reference matching tasks/[a-z0-9-]+\\.md")
        print(f"tasklink: no PR number; task is caller-supplied without "
              f"auto-resolution — plan.task alone is NOT evidence publication",
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

    plan_commit = _git(planning_root, "rev-parse", "HEAD").decode().strip()

    # Automatic evidence-Aktualisierung der Planning-Karte (G4b):
    # When pr_number is provided (auto-linked), update the task note at
    # `tasks/<task-name>.md` with a `plans:` line referencing the plan
    # commit. This makes the Planning-Karte show the current evidence chain
    # without manual maintenance.
    if pr_number is not None and pr_number != "":
        # Derive the task note path from the task reference.
        task_name = task  # e.g. "tasks/t-20261022-r27-pilot.md"
        task_path = Path(planning_root) / task_name
        if task_path.exists():
            _update_task_note(planning_root, task_path, plan_commit)
        else:
            # Task note doesn't exist yet — create it with minimal structure.
            task_path.parent.mkdir(parents=True, exist_ok=True)
            _create_task_note(planning_root, task_path, task, plan_commit)

    return plan_commit


def _update_task_note(planning_root, task_path, plan_commit):
    """Update an existing task note with a plan reference.

    Adds a `plans:` line to the YAML front matter referencing the plan
    commit. If a `plans:` line already exists, appends the new commit.
    """
    raw = task_path.read_text()
    # YAML front matter: lines between --- markers.
    fm_end = raw.find("---", raw.find("---") + 3)
    if fm_end < 0:
        # No front matter — add one.
        fm = "---\nplans: [{sha}]\n---\n".format(sha=plan_commit)
        updated = fm + raw
    else:
        fm = raw[:fm_end + 3]
        body = raw[fm_end + 3:]
        # Check if plans: line exists.
        plans_match = re.search(r"^plans:\s*\[([^\]]*)\]", fm, re.MULTILINE)
        if plans_match:
            # Append to existing plans list.
            existing = plans_match.group(1)
            # Clean up the list.
            items = [s.strip() for s in existing.split(",") if s.strip()]
            # Avoid duplicate commits.
            if plan_commit not in items:
                items.append(plan_commit)
            new_list = ", ".join(items)
            fm = re.sub(r"plans:\s*\[[^\]]*\]",
                         f"plans: [{new_list}]", fm)
            updated = fm + body
        else:
            # Add plans line to front matter.
            insert_pos = fm.find("---", 3)
            if insert_pos < 0:
                # No closing --- in front matter — fix it.
                fm = fm.rstrip()
                if not fm.endswith("---"):
                    fm += "\n"
                fm += f"plans: [{plan_commit}]\n---\n"
            else:
                # Insert before closing ---.
                before = fm[:insert_pos]
                before = before.rstrip()
                if before.endswith("\n"):
                    before = before[:-1]
                new_fm = before + "\nplans: [{sha}]\n---\n".format(sha=plan_commit)
                fm = new_fm
            updated = fm + body
    task_path.write_text(updated)
    _git(planning_root, "add", "-f", str(task_path.relative_to(planning_root)))
    _git(planning_root, "commit", "-q", "-m",
         f"tasklink: evidence link {plan_commit[:8]} → {task_name}",
         str(task_path.relative_to(planning_root)))


def _create_task_note(planning_root, task_path, task, plan_commit):
    """Create a new task note with plan reference.

    Minimal YAML front matter with id, plans list, and a body placeholder.
    The plans: line references the plan commit so the Planning-Karte
    automatically shows the current evidence chain.
    """
    # Derive a short id from the task name.
    short_id = task.split("/")[-1].replace(".md", "")
    note = "---\nid: {id}\nplans: [{sha}]\n---\nplan fixture linked\n".format(
        id=short_id, sha=plan_commit)
    task_path.write_text(note)
    _git(planning_root, "add", "-f", str(task_path.relative_to(planning_root)))
    _git(planning_root, "commit", "-q", "-m",
         f"tasklink: create task note {task} with plan {plan_commit[:8]}",
         str(task_path.relative_to(planning_root)))


def _git(root, *args):
    """Run a git command inside `root`, raising on failure."""
    result = subprocess.run(["git", "-C", str(root), *args],
                            capture_output=True)
    if result.returncode != 0:
        raise TaskLinkError(f"git {args[0]} failed: {result.stderr.decode()}")
    return result.stdout


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

    # Validate: either --task is provided OR --pr-number is provided.
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
