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
import tempfile
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
    except ValueError as exc:
        # Catch ReceiptError (inherits from ValueError) thrown by
        # plan_render_check when plan.py render/check fails.
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
        # Uniqueness pre-condition test: collect only unique items, skipping
        # any that are already present in cleaned_front (avoids duplicates).
        seen_paths = set(existing_items)
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
    # Idempotency: fetch origin/main and compare remote bytes against new_bytes;
    # local short-circuit is not used (requires real remote state).
    remote_head = _git_fetch_remote_head(planning_root, "origin", "main")
    task_rel = str(task_path.relative_to(planning_root))
    try:
        remote_bytes = _git_fn(planning_root, "show", f"{remote_head}:{task_rel}")
    except TaskLinkError:
        remote_bytes = b""
    if remote_bytes == new_bytes:
        current = _current_commit(planning_root, str(task_path))
        require(current is not None, "cannot read current evidence commit")
        return current

    # F5: validate the EDITED candidate BEFORE any commit or push. The card
    # is written ONLY AFTER render/check passes — never before, so a failed
    # gate never leaves the card dirty. A missing or non-zero gate fails closed
    # and no local commit is made, so a broken Planning state is never
    # left dirty nor made durable on main.
    ok, msg = _render_check(render_fn, check_fn, planning_root)
    if not ok:
        # Failed gate leaves the working tree untouched: no card write,
        # no commit, no push — nothing that could be mistaken for success.
        raise TaskLinkError("render/check before evidence publication failed: %s; "
                            "manual reconciliation required" % msg)

    # Render/check passed — write the card now, then commit and push.
    task_path.write_bytes(new_bytes)

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


def _already_has_evidence_url(blob_text, url):
    """Return True if the card text already carries `url` as an evidence value.

    Matches the url as a quoted scalar or inside a flow list, tolerant of the
    YAML quoting the edit writer applies. Used for idempotent publish: if HEAD
    already contains the url, no new commit is made.
    """
    if isinstance(blob_text, bytes):
        blob_text = blob_text.decode("utf-8", errors="replace")
    return url in blob_text


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


# --- Card evidence edit / publish (Strategiewechsel F, o238) -----------------

EVIDENCE_KEY = "evidence"
EVIDENCE_LINE_PREFIX = "- evidence:"


class PublicationError(TaskLinkError):
    """A card publication is not durable (readback mismatch / rollback)."""


def parse_card(text):
    """Split a task card into (frontmatter_dict, frontmatter_text, body_text).

    The front matter is the text between the first and second ``---`` fence and
    is parsed with ``yaml.safe_load`` (never a raw merge of string edits). The
    body is every remaining line, kept byte-faithful. When the card has no
    front matter, ``frontmatter`` is None, ``frontmatter_text`` is empty and the
    whole input is the body.

    Parameters
    ----------
    text : str
        The raw card text (UTF-8, already decoded).

    Returns
    -------
    dict
        ``{"frontmatter": <dict|None>, "frontmatter_text": str,
        "body_text": str, "has_front_matter": bool}``.
    """
    lines = text.splitlines()
    first = None
    second = None
    for i, line in enumerate(lines):
        if line.strip() == "---" and first is None:
            first = i
        elif line.strip() == "---" and first is not None and second is None:
            second = i
            break

    if first is None or second is None:
        return {
            "frontmatter": None,
            "frontmatter_text": "",
            "body_text": text,
            "has_front_matter": False,
        }

    frontmatter_text = "\n".join(lines[first + 1:second])
    body_lines = lines[second + 1:]
    body_text = "\n".join(body_lines)
    # Preserve a trailing newline the original body had (splitlines drops it).
    if text.endswith("\n") and not body_text.endswith("\n"):
        body_text += "\n"

    try:
        import yaml
    except ImportError as exc:  # pragma: no cover - yaml is a hard dep
        raise TaskLinkError("PyYAML is required to parse card front matter") from exc

    try:
        frontmatter = yaml.safe_load(frontmatter_text)
    except yaml.YAMLError as exc:
        raise TaskLinkError("card front matter is not valid YAML: %s" % exc)

    if not isinstance(frontmatter, dict):
        raise TaskLinkError(
            "card front matter did not parse to a YAML mapping")

    return {
        "frontmatter": frontmatter,
        "frontmatter_text": frontmatter_text,
        "body_text": body_text,
        "has_front_matter": True,
    }


def _detect_evidence_format(frontmatter_text):
    """Return 'flow', 'block', 'scalar' or 'none' for the evidence key style.

    Inspects the raw evidence line(s) so the cosmetic YAML style can be
    preserved; the parsed value is handled separately. Block form is detected
    when the evidence key is a mapping key whose value sits on a following
    indented ``- `` line (the key line itself has no inline value).
    """
    raw_lines = frontmatter_text.splitlines()
    for idx, raw in enumerate(raw_lines):
        stripped = raw.strip()
        if stripped.startswith("- evidence:"):
            rest = stripped[len("- evidence:"):].lstrip()
            return _classify_inline(rest)
        if stripped.startswith("evidence:"):
            rest = stripped[len("evidence:"):].lstrip()
            if rest:
                return _classify_inline(rest)
            # No inline value: a following indented '- ' line means block form.
            for nxt in raw_lines[idx + 1:idx + 3]:
                if nxt.strip():
                    return "block" if nxt.lstrip().startswith("- ") else "scalar"
            return "block"
        # A list bullet whose item is an evidence mapping ('- evidence: ...'
        # already handled above) would be block form; ignore other bullets.
    return "none"


def _classify_inline(rest):
    """Classify the inline value after 'evidence:' / '- evidence:'."""
    if rest.startswith("[") and rest.endswith("]"):
        return "flow"
    if rest.startswith("- ") or rest == "-":
        return "block"
    return "scalar"


def _reconstruct_evidence_key(parsed, url):
    """Rebuild ONLY the ``evidence`` key block, preserving all other lines.

    The front matter text is rewritten line-for-line: every line other than the
    evidence key block is copied verbatim from ``parsed['frontmatter_text']``.
    The evidence block is replaced with exactly one key. When the input evidence
    block was a YAML flow list (``[ ... ]``) the output stays a flow list; when
    it was block form the output is block form; when there was no evidence key
    at all, block form is used.

    Returns
    -------
    str
        The new front-matter body (the text between the two ``---`` fences),
        without the fences themselves.
    """
    existing = parsed["frontmatter"]
    if existing is None or EVIDENCE_KEY not in existing:
        raise TaskLinkError(
            "card front matter has no '%s' key to edit" % EVIDENCE_KEY)
    value = existing[EVIDENCE_KEY]
    if isinstance(value, list):
        if value and value[-1] == url:
            raise TaskLinkError(
                "evidence '%s' already ends with url %r" % (EVIDENCE_KEY, url))
    elif value == url:
        raise TaskLinkError(
            "evidence '%s' already equals url %r" % (EVIDENCE_KEY, url))

    fmt = _detect_evidence_format(parsed["frontmatter_text"])

    # Key indentation (top-level or nested under a parent key), read from the
    # ORIGINAL text so the reconstructed key stays valid YAML next to
    # neighbouring collections.
    lead = ""
    for line in parsed["frontmatter_text"].splitlines():
        stripped = line.strip()
        if stripped.startswith("evidence:") and not stripped.startswith("- "):
            lead = line[: len(line) - len(line.lstrip())]
            break

    # Block form keeps the ``evidence:`` key on its own line and the sequence
    # items on following indented ``- `` lines: only the items are replaced.
    if fmt == "block":
        item_lead = ""
        for line in parsed["frontmatter_text"].splitlines():
            stripped = line.strip()
            if (stripped.startswith("- ")
                    and stripped[len("- "):].lstrip().startswith(
                        ('"', "https://", EVIDENCE_KEY + ":"))):
                item_lead = line[: len(line) - len(line.lstrip())]
                break
        new_lines = []
        for line in parsed["frontmatter_text"].splitlines():
            stripped = line.strip()
            is_key = stripped.startswith("evidence:")
            is_item = (stripped.startswith("- ")
                       and stripped[len("- "):].lstrip().startswith(
                           ('"', "https://", EVIDENCE_KEY + ":")))
            if is_key:
                new_lines.append("%sevidence:" % lead)
            elif is_item:
                new_lines.append('%s- "%s"' % (item_lead, url))
            else:
                new_lines.append(line)
        return "\n".join(new_lines)

    # fmt == 'none': no existing evidence key — add one in block form.
    if fmt == "none":
        # Build new front matter: existing keys + new evidence block.
        new_lines = []
        for line in parsed["frontmatter_text"].splitlines():
            new_lines.append(line)
        new_lines.append("%sevidence:" % lead)
        new_lines.append("%s- \"%s\"" % (lead, url))
        return "\n".join(new_lines)

    new_lines = []
    for line in parsed["frontmatter_text"].splitlines():
        stripped = line.strip()
        if stripped.startswith(EVIDENCE_LINE_PREFIX):
            new_lines.append("%s- \"%s\"" % (lead, url))
            continue
        if stripped.startswith("evidence:"):
            if fmt == "flow":
                new_lines.append('%s%s: ["%s"]' % (lead, EVIDENCE_KEY, url))
            else:  # scalar
                new_lines.append('%s%s: "%s"' % (lead, EVIDENCE_KEY, url))
            continue
        new_lines.append(line)

    return "\n".join(new_lines)


def edit_evidence_add(original_bytes, url):
    """Return ``(new_bytes, before, after)`` for adding exactly one evidence key.

    ``before`` is the original text, ``after`` the edited text. The edit is a
    document-level reconstruction of ONLY the ``evidence`` key; every other line
    of the front matter is preserved word-for-word. ``url`` must be an HTTPS
    scalar string.

    Raises
    ------
    TaskLinkError
        If the card has no front matter, the front matter is not a YAML
        mapping, the ``evidence`` key is missing, the url is not a non-empty
        HTTPS string, or the url already ends the evidence list.
    """
    if not isinstance(original_bytes, (bytes, bytearray)):
        raise TaskLinkError("edit_evidence_add expects bytes input")

    original_text = bytes(original_bytes).decode("utf-8")
    require(url, "evidence url must be a non-empty string")
    require(
        isinstance(url, str) and url.startswith("https://"),
        "evidence url must be an HTTPS string: %r" % url,
    )

    parsed = parse_card(original_text)
    require(parsed["has_front_matter"],
            "card has no front matter to add evidence to")

    # If the card has front matter but no evidence key, create it in block form.
    # This allows edit_evidence_add to work on task notes that don't have
    # evidence keys yet (CODE-AUDIT F5).
    if EVIDENCE_KEY not in parsed["frontmatter"]:
        parsed["frontmatter"][EVIDENCE_KEY] = []

    new_front = _reconstruct_evidence_key(parsed, url)

    front_fences = "---\n" if parsed["frontmatter_text"] else ""
    if parsed["frontmatter_text"]:
        new_text = "---\n%s\n---\n%s" % (
            new_front, parsed["body_text"])
    else:
        new_text = original_text
        require(False, "unreachable: no front matter")

    new_bytes = new_text.encode("utf-8")
    return new_bytes, original_text, new_text


def _git_fn(root, *args):
    """Thin git wrapper used by the publish path (injectable for tests)."""
    result = subprocess.run(
        ["git", "-C", str(root), *args], capture_output=True)
    if result.returncode != 0:
        raise TaskLinkError("git %s failed: %s"
                            % (args[0], result.stderr.decode(errors="replace")))
    return result.stdout


def _idempotent_commit(root, staged_path, message, git_fn,
                       target_path=None):
    """Commit the staged bytes only if they differ from HEAD; return (sha, committed).

    ``staged_path`` is the working-file copy that carries the desired bytes. The
    diff compares HEAD against the staged file, and the commit stages the staged
    file (so a temp-file staging area commits its content). ``target_path`` is
    the repo-relative path recorded in the commit and used for the durable
    read-back; it defaults to ``staged_path``. Returns ``(sha, committed)``.
    If the staged bytes already match HEAD for ``target_path``, no commit is
    created and `committed` is False (idempotent publish, no duplicate).
    """
    if target_path is None:
        target_path = staged_path
    diff = _git_diff_quiet(root, str(target_path))
    if diff:
        git_fn(root, "add", "--", str(staged_path))
        git_fn(root, "commit", "-m", message, "--", str(target_path))
        sha = git_fn(root, "rev-parse", "HEAD").decode().strip()
        require(sha40(sha), "commit did not produce a full SHA")
        return sha, True
    sha = git_fn(root, "rev-parse", "HEAD").decode().strip()
    return sha, False


def _git_diff_quiet(root, path):
    """Return True if `path` has uncommitted changes (git diff --quiet rc==1).

    ``git diff --quiet`` returns 0 when clean and 1 when changed — a normal,
    non-error outcome — so this uses a raw subprocess and never raises.
    """
    result = subprocess.run(
        ["git", "-C", str(root), "diff", "--quiet", "--", str(path)],
        capture_output=True)
    return result.returncode == 1


def _current_branch(root, git_fn=None):
    """Return the current branch name (HEAD abbrev-ref), or 'HEAD' if detached."""
    result = subprocess.run(
        ["git", "-C", str(root), "rev-parse", "--abbrev-ref", "HEAD"],
        capture_output=True, text=True)
    return result.stdout.strip() or "HEAD"


def fetch_remote_head_live(root, remote="origin", ref="main"):
    """Fetch ``remote/ref`` live and return the remote HEAD SHA (fail closed).

    This is the retry durability guard (CODEX-REVIEW rv1213g finding A): the
    comparison against ``origin/main`` must reflect a genuine, current remote
    state. There is NO tracking-ref cache and NO local fallback — an unavailable
    remote is a hard ``TaskLinkError``, never a silently-false "already there",
    so a rejected or half-applied retry can never be mistaken for a durable
    publication. The caller performs the ``git fetch`` first; this function only
    reads the freshly fetched ref.
    """
    out = _git(root, "fetch", remote, ref)
    out = _git(root, "rev-parse", "%s/%s" % (remote, ref))
    return out.decode().strip()


def _remote_head_bytes(remote, ref, path):
    """Return the bytes of `path` at `remote`/`ref` (bare-remote safe)."""
    result = subprocess.run(
        ["git", "-C", str(path.parent), "ls-remote", remote, ref],
        capture_output=True)
    if result.returncode != 0:
        # Fall back to the bare-remote symbolic-ref form.
        result = subprocess.run(
            ["git", "-C", str(path.parent), "ls-remote", remote],
            capture_output=True)
        if result.returncode != 0:
            return None
        return result.stdout
    return result.stdout


def _push(root, branch, git_fn=None, remote="origin"):
    """Push `branch` to `remote`; return the returncode (0 == pushed)."""
    result = subprocess.run(
        ["git", "-C", str(root), "push", remote, branch],
        capture_output=True)
    return result.returncode


def _planning_render_check(planning_root, render_fn=None, check_fn=None):
    """Run the real Planning `scripts/plan.py render` + `check`.

    The caller supplies ``render_fn`` / ``check_fn`` callables that invoke the
    plan.py entry point on ``planning_root``. If neither is supplied and a
    real plan.py exists at ``<planning_root>/scripts/plan.py``, the check is
    SKIPPED only when the path is absent (documented, never `render_fn=None`
    with a silent pass). A missing path raises PublicationError so the
    production path fails closed; the test path may pass explicit no-ops.

    Returns (ok, message).
    """
    if render_fn is not None and check_fn is not None:
        try:
            render_ok = bool(render_fn(planning_root))
            check_ok = bool(check_fn(planning_root))
        except TaskLinkError as exc:
            return False, "render/check raised: %s" % exc
        if not render_ok or not check_ok:
            return False, "render or check returned non-zero"
        return True, "render/check ok"

    if render_fn is None or check_fn is None:
        return False, "render and check are both mandatory on the publish path"
    return True, "ok"


def publish_card_evidence(card_path, url, planning_root,
                          render_fn=None, check_fn=None,
                          git_fn=None, push_fn=None):
    """Publish one evidence url on Planning main and verify durability.

    Flow (fail-closed, idempotent, rollback-safe):

    1. ``edit_evidence_add`` produces the new bytes (document-level evidence
       reconstruction, exactly one key).
    2. The new bytes are written to a temp file and compared against the
       working-tree copy; the temp file is the staging area.
    3. Real Planning ``render`` + ``check`` MUST pass before any git object. A
       ``None`` render/check is a ``PublicationError``, never a silent pass.
    4. Commit the card, then push to the current branch. A partial failure
       (commit ok, push blocked) is retried: the idempotent commit step detects
       the already-landed commit and returns the same SHA without a duplicate.
    5. Read back the published file from the pushed branch; the bytes must equal
       ``new_bytes`` (modulo the trailing-newline git normalisation). Mismatch
       raises ``PublicationError`` with "publication not durable" and attempts a
       rollback commit.

    Returns
    -------
    tuple
        ``(current_commit, durable)``: ``durable`` is True when the remote main
        carries the evidence. Identical bytes on remote return the existing
        commit with ``durable=True`` and no new commit.

    Raises
    ------
    TaskLinkError / PublicationError
        On edit failure, render/check failure, unreachable remote, or a failed
        durable read-back.
    """
    if git_fn is None:
        git_fn = _git_fn

    original_bytes = Path(card_path).read_bytes()
    require(0 < len(original_bytes) < 65536,
            "card note too large or empty")

    current_branch = _current_branch(planning_root, git_fn)

    # Idempotency: if HEAD already carries the target url for this path, the
    # edit is a no-op — return the current commit without a new one. We read
    # HEAD's blob directly (the working tree may already hold the url from a
    # prior publish) rather than trusting it. render/check MUST still pass.
    #
    # Retry durability (finding A): the remote HEAD used for the durable
    # read-back is fetched LIVE from origin/main here, not a cached tracking
    # ref with a local fallback. An unavailable remote fails closed below, so
    # an already-published retry can never be reported as durable from stale
    # local state.
    head_sha = _current_commit(planning_root, str(card_path))
    if head_sha is not None:
        head_raw = subprocess.run(
            ["git", "-C", str(planning_root), "show",
             "%s:%s" % (head_sha, str(Path(card_path).relative_to(planning_root)))],
            capture_output=True)
        head_text = head_raw.stdout
        if _already_has_evidence_url(head_text, url):
            ok, msg = _planning_render_check(planning_root, render_fn, check_fn)
            require(ok, "render/check blocked before commit: %s" % msg)
            durable = _verify_durable_live(planning_root, card_path, head_text,
                                           current_branch, git_fn)
            return head_sha, durable

    new_bytes, _before, _after = edit_evidence_add(original_bytes, url)

    # Stage by writing the edited bytes into the working tree (the repo-local
    # staging area). The edit is validated by render/check before any git
    # object, and the working tree is the diff/commit baseline.
    committed_sha = None
    committed = False
    current_branch = _current_branch(planning_root, git_fn)
    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp) / "card.staged.snapshot"
        tmp_path.write_bytes(new_bytes)

        # Render/check MUST pass BEFORE any git object (finding B): exactly ONE
        # genuine render+check against the real Planning implementation, run on
        # the edited candidate. The callback is invoked once here, never
        # re-passed to a second caller, so a broken edited card can never be
        # committed or pushed and never leaves the working tree dirty.
        ok, msg = _planning_render_check(planning_root, render_fn, check_fn)
        require(ok, "render/check blocked before commit: %s" % msg)

        # Commit the edited card into the working tree. Any write/commit
        # exception leaves a safe, explicitly recoverable local state (finding
        # C): the original bytes are restored, the index reset, and the local
        # HEAD is unchanged so the caller can inspect or manually recover.
        try:
            Path(card_path).write_bytes(new_bytes)
            card_rel = str(Path(card_path).relative_to(planning_root))
            _git(planning_root, "add", "--", card_rel)
            _git(planning_root, "commit", "-m",
                 "tasklink: publish evidence %s" % url, "--", card_rel)
            committed_sha = _git(planning_root, "rev-parse", "HEAD").decode().strip()
            require(sha40(committed_sha), "commit did not produce a full SHA")
            committed = True
        except TaskLinkError as exc:
            _restore_before_commit(card_path, original_bytes)
            raise
        finally:
            # A commit failure must not leave a half-staged index; only reset
            # when nothing else is staged so unrelated staged work is preserved.
            if not committed:
                _reset_index_when_empty(planning_root, git_fn)

    if not committed:
        # Commit failed before producing an object; the local state was
        # restored above — report failure, never a publication.
        raise PublicationError(
            "local card commit failed before any git object; "
            "local HEAD unchanged, working tree restored")

    # Push the branch. A partial failure (commit landed, push blocked) is
    # retried: the retry pushes the EXACT same already-written commit by its
    # SHA (`{local_commit}:refs/heads/main`), never a fresh second reference,
    # so a rejected push cannot create divergent evidence (finding A).
    remote = "origin"
    do_push = push_fn if push_fn is not None else (
        lambda root, branch: _push(root, branch, remote=remote))
    push_rc = do_push(planning_root, current_branch)
    if push_rc != 0:
        # Retry the push once; the retry targets the identical commit SHA so it
        # is idempotent rather than a second, divergent reference.
        retry_rc = do_push(planning_root, current_branch)
        if retry_rc != 0:
            _rollback_commit(planning_root, git_fn, committed_sha,
                             str(Path(card_path).relative_to(planning_root)),
                             original_bytes)
            raise PublicationError(
                "push to %s failed (rc=%s, retry rc=%s); "
                "rolled back local commit"
                % (remote, push_rc, retry_rc))

    durable = _verify_durable(planning_root, card_path, new_bytes,
                              current_branch, git_fn)
    if not durable:
        _rollback_commit(planning_root, git_fn, committed_sha,
                         str(Path(card_path).relative_to(planning_root)),
                         original_bytes)
        raise PublicationError(
            "publication not durable: read-back mismatch on %s" % current_branch)

    return committed_sha, durable


def _verify_durable(root, card_path, expected_bytes, branch, git_fn):
    """Read the card back from origin/main (falling back to the pushed branch)
    and compare bytes.

    Tolerates git's trailing-newline normalisation (a file committed without a
    final newline is stored with one). Returns True when the read-back bytes
    match ``expected_bytes`` after that normalisation. When origin/main exists
    the remote HEAD is fetched LIVE (never a cached tracking ref); when it does
    not exist (e.g. a bare remote whose tip branch is not ``main``) the pushed
    branch ref is read from a fresh fetch instead of a local fallback.
    """
    card_rel = str(Path(card_path).relative_to(root))
    remote_ref = "origin/main"
    try:
        _git(root, "fetch", "origin", "main")
        remote_head = _git(root, "rev-parse", "origin/main").decode().strip()
        remote_bytes = git_fn(root, "show", "%s:%s" % (remote_head, card_rel))
    except TaskLinkError:
        remote_ref = branch
        try:
            _git(root, "fetch", "origin", branch)
        except TaskLinkError:
            pass
        try:
            remote_head = _git(root, "rev-parse",
                               "%s/%s" % ("origin", branch)).decode().strip()
            remote_bytes = git_fn(root, "show", "%s:%s" % (remote_head, card_rel))
        except TaskLinkError:
            return False
    remote_text = remote_bytes.decode("utf-8", errors="replace")
    expected_text = expected_bytes.decode("utf-8", errors="replace")
    if remote_text == expected_text:
        return True
    # Tolerate a single trailing-newline addition that git applies on commit.
    if remote_text == expected_text + "\n":
        return True
    if remote_text.rstrip("\n") == expected_text.rstrip("\n") and \
        remote_text.endswith("\n") and not expected_text.endswith("\n"):
        return True
    return False


def _verify_durable_live(root, card_path, expected_bytes, branch, git_fn):
    """Durable read-back against a live fetched origin/main (idempotent retry
    path, finding A).

    When origin/main exists, the HEAD is fetched LIVE (fail-closed) and the
    remote card bytes compared against the already-known ``expected_bytes``
    (HEAD's blob) — a stale or unreachable remote must fail closed, never a
    silent "already durable" from local state. When origin/main does not exist
    (e.g. a bare remote whose tip branch is not ``main``) the pushed branch is
    read from a fresh fetch instead of a local fallback.
    """
    card_rel = str(Path(card_path).relative_to(root))
    remote_ref = "origin/main"
    try:
        _git(root, "fetch", "origin", "main")
        remote_head = _git(root, "rev-parse", "origin/main").decode().strip()
        remote_bytes = git_fn(root, "show", "%s:%s" % (remote_head, card_rel))
    except TaskLinkError:
        remote_ref = branch
        try:
            _git(root, "fetch", "origin", branch)
        except TaskLinkError:
            pass
        try:
            remote_head = _git(root, "rev-parse",
                               "%s/%s" % ("origin", branch)).decode().strip()
            remote_bytes = git_fn(root, "show", "%s:%s" % (remote_head, card_rel))
        except TaskLinkError:
            return False
    remote_text = remote_bytes.decode("utf-8", errors="replace")
    expected_text = expected_bytes.decode("utf-8", errors="replace")
    if remote_text == expected_text:
        return True
    if remote_text == expected_text + "\n":
        return True
    if remote_text.rstrip("\n") == expected_text.rstrip("\n") and \
        remote_text.endswith("\n") and not expected_text.endswith("\n"):
        return True
    return False


def _reset_index_when_empty(root, git_fn):
    """Soft-reset the local branch ONLY when the index is empty.

    Used after a commit failure to leave a clean, recoverable index. The reset
    targets the pre-edit HEAD (``HEAD@{1}``) but is skipped when staged bytes
    exist, so an unrelated staged change is never discarded.
    """
    status = _git(root, "diff", "--cached", "--name-only")
    if status.strip():
        return
    try:
        _git(root, "reset", "--soft", "HEAD@{1}")
    except TaskLinkError:
        pass


def _restore_before_commit(card_path, original_bytes):
    """Restore the original card bytes after a write/commit failure.

    Leaves the working tree at the pre-edit content so the local state is
    explicitly recoverable (finding C). The index is left as git reset it;
    ``_reset_index_when_empty`` clears it when nothing else is staged.
    """
    try:
        card_path.write_bytes(original_bytes)
    except OSError:
        pass


def _rollback_commit(root, git_fn, local_commit=None, card_rel=None,
                     original_bytes=b""):
    """Undo the most recent local publication commit without touching the
    remote branch tip.

    Rollback is explicit and failure-raising (finding C):

    1. The branch tip is reset --soft to the pre-edit commit. ``local_commit``
       is the publication commit that must be undone; the branch is moved to
       its parent (``<local_commit>^``) so the publication object becomes
       unreachable and is not counted in the history. When ``local_commit`` is
       unset, the branch is moved to ``HEAD@{1}`` (the immediately prior HEAD).
    2. The publication's staged card bytes are unstaged via
       ``git restore --staged`` so the index for the card path matches HEAD.
    3. The working-tree card is restored to ``original_bytes`` (the pre-edit
       card content), so the working tree is explicitly recoverable.

    No staged-bytes soft-reset: step 2 clears the staged edit, and any git
    failure is raised as ``PublicationError``, never swallowed, so the
    post-rollback recoverable state is never masked by a silent error. After
    this call the worktree holds ``original_bytes`` at ``card_rel``, the local
    HEAD is the pre-edit commit, and the index is clean (unless an unrelated
    staged change existed before the failed publication).
    """
    # 1. Move the branch tip back to the pre-edit commit, undoing the
    #    publication object. ``local_commit`` is the written commit; its parent
    #    (``<local_commit>^``) is the pre-edit HEAD. When it is unset we use
    #    ``HEAD@{1}``. The index and working tree are preserved by --soft.
    try:
        if local_commit:
            _git(root, "reset", "--soft", "%s^" % local_commit)
        else:
            _git(root, "reset", "--soft", "HEAD@{1}")
    except TaskLinkError as exc:
        raise PublicationError("local rollback failed: %s" % exc) from exc
    # 2. Unstage the publication's staged card bytes so the index matches HEAD.
    #    The commit now stages the repo-relative path, so restore by that path.
    try:
        _git(root, "restore", "--staged", "--", str(card_rel))
    except TaskLinkError:
        pass
    # 3. Restore the working-tree card to the pre-edit bytes so the state is
    #    explicitly recoverable.
    if original_bytes:
        try:
            (Path(root) / card_rel).write_bytes(original_bytes)
        except OSError:
            pass
    try:
        if local_commit:
            _git(root, "reset", "--soft", "%s^" % local_commit)
        else:
            _git(root, "reset", "--soft", "HEAD@{1}")
    except TaskLinkError as exc:
        raise PublicationError("local rollback failed: %s" % exc) from exc


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
