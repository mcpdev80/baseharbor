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
            "referenced issue #%s does not point to a Planning task; "
            "issue title=%r body=%r" % (issue_number, title, issue_body)
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
        # The auto-resolved PR->task binding is authoritative. If the caller
        # supplied a task, it is NOT used: a divergent caller task means the
        # caller is not going through the trusted resolver, and the resolved
        # task is the only valid evidence link.
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


def _git(root, *args, check=True):
    """Run a git command inside `root`, raising on failure.

    Parameters
    ----------
    check : bool
        When True and the command is "push", a non-zero return code
        raises TaskLinkError rather than returning raw output. This
        ensures callers cannot silently misinterpret a rejected push.
    """
    env = dict(os.environ, GIT_NO_LAZY_FETCH="1")
    result = subprocess.run(["git", "-C", str(root), *args],
                            capture_output=True, env=env)
    if result.returncode != 0:
        stderr = result.stderr.decode()
        if check and any(n in stderr for n in ("non-fast-forward", "rejected", "failed to push")):
            raise TaskLinkError(f"git {args[0]} rejected: {stderr.strip()}")
        raise TaskLinkError(f"git {args[0]} failed: {stderr}")
    return result.stdout


def _git_fetch_remote_head(root, remote="origin", ref="main"):
    """Fetch the remote tracking ref and return the remote HEAD SHA.

    This is the F5 remote-read-back: after committing locally we fetch
    from the remote to verify that the publication is durable on the
    remote side, not just a local-only commit.
    """
    try:
        _git(root, "fetch", remote, ref)
    except TaskLinkError:
        raise TaskLinkError(
            "publication blocked: remote fetch of %s/%s failed — "
            "evidence was not durably published" % (remote, ref))
    try:
        out = _git(root, "rev-parse", "%s/%s" % (remote, ref))
        return out.decode().strip()
    except TaskLinkError:
        raise TaskLinkError(
            "publication blocked: cannot determine remote HEAD for %s/%s"
            % (remote, ref))


def _render_check(render_fn, check_fn, planning_root):
    """Run mandatory Planning render + check before any evidence publication.

    Returns (ok, message). Both hooks are REQUIRED (fail-closed): a missing
    render/check entry point fails the publication, never a silent "ok", so a
    broken Planning state can never be made durable on main (CODE-AUDIT F5). A
    non-zero render/check is likewise a hard failure.
    """
    if render_fn is None or check_fn is None:
        return False, "mandatory render or check callback not provided"
    try:
        render_ok = render_fn(planning_root)
        check_ok = check_fn(planning_root)
    except TaskLinkError as exc:
        return False, "render/check raised: %s" % exc
    if render_ok is False or check_ok is False:
        return False, "render or check returned non-zero"
    return True, "ok"


def _front_matter_has_mapping_keys(front_lines):
    """True when front-matter lines already contain mapping keys (id:, prs:).

    A bare list item appended at the top level alongside such keys is invalid
    YAML, so evidence entries must be nested under an `evidence:` key instead.
    """
    return any(re.fullmatch(r"[a-z_][a-z0-9_]*:.*", ln) for ln in front_lines
               if ln.strip())


def _existing_inline_evidence(lines):
    """Return the existing inline evidence line(s) in YAML list form, if any.

    Preserves any pre-existing evidence entry so this update is additive and
    never discards prior evidence (CODE-AUDIT F5).
    """
    found = []
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("- evidence:") or stripped.startswith("evidence:"):
            found.append(stripped)
    return found


def publish_evidence(planning_root, task, receipt_path, candidate_sha,
                     plan_revision, render_fn=None, check_fn=None,
                     _git_fn=None):
    """Durably publish the task evidence reference on Planning main via W2.

    Appends an immutable, canonical evidence reference line to the task note
    while preserving every pre-existing evidence entry (additive boundaries,
    CODE-AUDIT F5). It never replaces, truncates or leaves stale continuation
    lines. The edit is committed and the durable revision read back; the
    returned publication must contain both the receipt AND the valid task
    evidence on remote main. Failure is propagated (fail closed), never
    swallowed, and an unchanged update returns idempotently without a second
    commit.

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
        40-char approved plan revision SHA (immutable evidence link input).
    render_fn : callable, optional
        ``render(planning_root) -> bool`` Planning render hook.
    check_fn : callable, optional
        ``check(planning_root) -> bool`` Planning check hook.
    _git_fn : callable, optional
        Git helper function. Defaults to ``_git``.

    Returns
    -------
    str
        The durable commit SHA that published the evidence reference on main.

    Raises
    ------
    TaskLinkError
        If the task note is missing/invalid, the edit fails, the durable read-back
        does not contain the evidence, or remote main is not updated.
    """
    if _git_fn is None:
        _git_fn = _git

    task_path = Path(planning_root) / task
    require(task_path.exists(), "task note missing for evidence publication: %s" % task)

    # F5: validate the immutable reference inputs before any export. A malformed
    # candidate SHA must fail closed and leave the remote unchanged — string
    # presence alone is insufficient.
    require(sha40(candidate_sha), "candidate_sha must be a 40-char hex SHA")
    require(sha40(plan_revision), "plan_revision must be a 40-char hex SHA")

    existing = task_path.read_bytes()
    require(isinstance(existing, bytes) and 0 < len(existing) < 65536,
            "task note too large or empty")

    text = existing.decode(errors="replace")
    lines = text.splitlines()

    # Canonical evidence entry: an immutable, schema-valid HTTPS receipt URL
    # (receipt path). candidate/plan revisions move into the commit message,
    # never into the card front matter. The Planning validator (plan.py)
    # requires each evidence item to be a plain https string, so the entry is
    # emitted as a nested YAML block list item under a single 'evidence:' key.
    # The 2-space indentation keeps the block sequence valid under PyYAML and
    # alongside the other id:/evidence: mapping keys (CODE-AUDIT F5).
    evidence_entry = "  - %s" % receipt_path

    # Determine front matter boundaries (--- delimited) if present.
    start = None
    end = None
    for i, line in enumerate(lines):
        if line.strip() == "---" and start is None:
            start = i
        elif line.strip() == "---" and start is not None:
            end = i
            break

    # Split front matter into its body lines and the trailing body; a missing
    # front-matter block is synthesised below so the result is always a
    # schema-valid, single-document YAML block (F5).
    has_front = start is not None and end is not None
    front_lines = list(lines[start + 1:end]) if has_front else []
    body_lines = list(lines[end + 1:]) if has_front else ["", text.rstrip()]

    # Recognise EVERY evidence shape by real YAML parsing rather than line
    # scraping (BF-W3-2 / CODE-AUDIT F5): an empty 'evidence: []' scalar, a
    # block sequence ('evidence:' followed by '  - ...' items) and an inline
    # flow 'evidence: ["https://...", ...]' value all resolve to the same
    # Python list so nothing is dropped. A corrupt front matter fails closed
    # instead of silently skipping prior evidence. ``existing_items`` holds the
    # raw evidence strings (the receipt paths) so idempotency and re-emission
    # compare apples to apples (CODE-AUDIT F5).
    try:
        import yaml
    except ImportError as exc:
        raise TaskLinkError("evidence preservation requires PyYAML") from exc
    existing_items = []
    try:
        parsed = yaml.safe_load("\n".join(front_lines)) if front_lines else {}
    except yaml.YAMLError as exc:
        raise TaskLinkError("evidence front matter is invalid YAML: %s" % exc) from exc
    if isinstance(parsed, dict) and isinstance(parsed.get("evidence"), list):
        for item in parsed["evidence"]:
            if isinstance(item, str) and item.strip():
                existing_items.append(item.strip())

    # Replace the ENTIRE evidence node in one shot: strip every pre-existing
    # evidence artefact — the stale 'evidence: []' scalar, an 'evidence:'
    # mapping key together with its nested block items, and any prior inline-
    # flow or block list item — while keeping every other key and its order
    # byte-for-byte. Re-emitting a SINGLE 'evidence:' key with all preserved
    # items plus the new scalar guarantees exactly one evidence key in the
    # result (finding 1), never orphaned '  - ...' lines, never a duplicate
    # key, and never loss of prior evidence (CODE-AUDIT F5).
    cleaned_front = []
    pending_evidence_key = False
    for line in front_lines:
        stripped = line.strip()
        # Drop the 'evidence:' mapping key itself and every line until its
        # nested block ends (a following indented '- <url>' item, an inline
        # flow value, or the '[]' scalar).
        if pending_evidence_key:
            if stripped.startswith("- ") or stripped.startswith("  -") or stripped.startswith("["):
                continue
            pending_evidence_key = False
        if stripped == "evidence: []" or stripped == "evidence:" or stripped.startswith("evidence: ["):
            pending_evidence_key = True
            continue
        if stripped.startswith("- evidence:"):
            continue
        cleaned_front.append(line)

    # Idempotency: an identical evidence entry is already present -> the card
    # on disk already holds the canonical form, so re-emit that exact canonical
    # form: a single 'evidence:' key with each preserved item once plus the new
    # scalar exactly once. Re-emitting the key here — not skipping it — keeps
    # exactly one evidence key and never drops prior evidence (CODE-AUDIT F5).
    already_present = any(item == receipt_path for item in existing_items)
    if already_present:
        if not cleaned_front:
            cleaned_front = ["evidence:"]
        elif not cleaned_front[-1].strip().endswith("evidence:"):
            cleaned_front.append("evidence:")
        for item in existing_items:
            cleaned_front.append("  - %s" % item)
        # The new scalar is already one of existing_items (idempotent re-emission),
        # so append it only when it is not already the last preserved item.
        if existing_items[-1] != receipt_path:
            cleaned_front.append(evidence_entry)
        new_text = "---\n" + "\n".join(cleaned_front) + "\n---\n" + "\n".join(body_lines)
    else:
        # Ensure the entries live under a single 'evidence:' key so the block
        # sequence parses alongside the other front-matter keys. With no
        # pre-existing front matter, prepend the key so the block sequence is
        # not emitted at the top level (F5). Re-add preserved items first so the
        # existing evidence is never lost (CODE-AUDIT F5).
        if not cleaned_front:
            cleaned_front = ["evidence:"]
        elif not cleaned_front[-1].strip().endswith("evidence:"):
            cleaned_front.append("evidence:")
        for item in existing_items:
            cleaned_front.append("  - %s" % item)
        cleaned_front.append(evidence_entry)
        new_text = "---\n" + "\n".join(cleaned_front) + "\n---\n" + "\n".join(body_lines)

    # Preserve the exact trailing newline of the original file: splitlines()
    # drops it, so restore one trailing "\n" when the original ended in exactly
    # one newline. This keeps idempotency (re-emission compares equal to the
    # on-disk bytes) and avoids silently rewriting trailing-newline state.
    if text.endswith("\n") and not text.endswith("\n\n"):
        new_bytes = (new_text + "\n").encode()
    elif text.endswith("\n\n"):
        new_bytes = (new_text + "\n").encode()
    else:
        new_bytes = new_text.encode()
    # Idempotency: identical bytes on disk already -> return current commit.
    if task_path.read_bytes() == new_bytes:
        current = _current_commit(planning_root, str(task_path))
        require(current is not None, "cannot read current evidence commit")
        return current

    # F5: validate the EDITED candidate BEFORE any commit or push. The edited
    # bytes are written to the real card first, so the mandatory render/check
    # runs ONCE against the exact state that will be published — never against
    # the old card (finding 2 / CODE-AUDIT F5). A missing or non-zero gate
    # fails closed and the candidate bytes are rolled back, so a broken
    # Planning state is never left dirty nor made durable on main. Generated
    # staging outputs produced by render are handled under the Planning
    # contract and never advance the remote on their own.
    task_path.write_bytes(new_bytes)
    ok, msg = _render_check(render_fn, check_fn, planning_root)
    if not ok:
        # Roll the candidate back so a failed gate leaves the working tree
        # exactly as it was: the edited state never survives a blocked publish.
        task_path.write_bytes(existing)
        raise TaskLinkError("render/check before evidence publication failed: %s; "
                            "manual reconciliation required" % msg)

    # Commit, push to remote main, then read back the durable revision
    # from the remote to verify both the receipt and the valid task
    # evidence are present there (F5: publication is NOT local-only).
    _git_fn(planning_root, "add", "--", str(task_path))
    _git_fn(planning_root, "commit", "-m",
            "tasklink: publish evidence %s for task %s (candidate %s, plan %s)" % (
                receipt_path, task, candidate_sha, plan_revision),
            "--", str(task_path))
    local_commit = _git_fn(planning_root, "rev-parse", "HEAD").decode().strip()
    require(sha40(local_commit), "local publication is not a full SHA")

    # F5 remote-read-back: push to origin/main and fetch back to verify
    # the publication is durable on the remote, not a local-only commit.
    _git_fn(planning_root, "push", "origin", f"{local_commit}:refs/heads/main")
    remote_head = _git_fetch_remote_head(planning_root, "origin", "main")
    require(remote_head == local_commit,
            "remote HEAD does not match local commit — publication was not durably pushed")

    # Read back from the remote HEAD to assert the evidence is present
    # (fail closed if the edit did not land on the remote).
    durable_text = _git_fn(planning_root, "show",
                            f"origin/main:{task}").decode(errors="replace")
    require(evidence_entry.strip() in [l.strip() for l in durable_text.splitlines()],
            "evidence reference not present in the durable publication revision on remote")

    return remote_head


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
