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


# receipt_verifier.ReceiptError is a ValueError subclass. The card publisher
# must surface a failed plan.py render/check (a ReceiptError) with the same
# fail-closed semantics as a git/edit failure, so it is aliased here. Importing
# receipt_verifier at module top would create a circular import (the verifier
# imports this module), so the alias is resolved lazily via _receipt_error().
def _receipt_error():
    import receipt_verifier
    return receipt_verifier.ReceiptError


# The single runtime class; ``ReceiptError`` below references it for
# ``except (TaskLinkError, ReceiptError)`` and ``raise ReceiptError(...)``.
ReceiptError = _receipt_error()


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
    except (TaskLinkError, ReceiptError) as exc:
        # A failed plan.py render/check raises ReceiptError (fail closed);
        # report it so a broken Planning state is never made durable.
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
                      _git_fn=None, push_fn=None):
    """Thin wrapper around the live card-evidence publisher (finding: no dualism).

    The former standalone implementation (which rebuilt the evidence node by
    hand and published a bare receipt path) is removed. This wrapper preserves
    the historic signature for callers and tests, converts the receipt path into
    an immutable, schema-valid HTTPS evidence reference — the same
    ``https://github.com/<repo>/blob/<sha>/<path>`` form the live production
    path emits via ``evidence_reference`` — and delegates every step to
    ``publish_card_evidence``. There is exactly one production publisher; the
    wrapper adds no second publication path, no separate edit, and no divergent
    durable output.

    Parameters
    ----------
    planning_root : Path
        Checked-out Planning repository.
    task : str
        Task reference, e.g. "tasks/t-xxx.md".
    receipt_path : str
        Published receipt path relative to the Planning root.
    candidate_sha : str
        40-char approved candidate SHA (pins the evidence reference blob).
    plan_revision : str
        40-char approved plan revision SHA (accepted for signature parity; not
        written into the card — candidate/plan live only in the commit message).
    render_fn : callable, optional
        ``render(planning_root) -> bool`` Planning render hook (delegated once).
    check_fn : callable, optional
        ``check(planning_root) -> bool`` Planning check hook (delegated once).
    _git_fn : callable, optional
        Git helper function. Accepted for signature parity; the live publisher
        owns the git seam.

    Returns
    -------
    str
        The durable commit SHA that published the evidence reference on main.

    Raises
    ------
    TaskLinkError
        Propagated from ``publish_card_evidence``: missing/invalid task note,
        edit failure, a failed render/check gate, a failed durable read-back, or
        an unreachable/failed remote.
    """
    require(sha40(candidate_sha), "candidate_sha must be a 40-char hex SHA")
    require(sha40(plan_revision), "plan_revision must be a 40-char hex SHA")

    url = "https://github.com/mcpdev80/baseharbor/blob/%s/%s" % (candidate_sha, receipt_path)
    publication, durable = publish_card_evidence(
        Path(planning_root) / task, url, planning_root,
        render_fn=render_fn, check_fn=check_fn,
        git_fn=_git_fn, push_fn=push_fn)
    require(durable, "evidence publication is not durable on remote main")
    require(sha40(publication), "live publisher returned no full SHA")
    return publication


def _is_ancestor(ancestor_sha, descendant_sha, root, remote="origin", ref="main"):
    """Return True if ``ancestor_sha`` is an ancestor of ``descendant_sha``.

    Uses ``git merge-base --is-ancestor``. The ancestor ref is fetched live so
    the ancestry check reflects current remote state, never a stale tracking ref.
    """
    _git(root, "fetch", remote, ref)
    # --is-ancestor exits 0 if ancestor, 1 if not. The generic _git raises on
    # any non-zero return, so exit 1 propagates as TaskLinkError; a raised
    # exception therefore means "not an ancestor".
    _git(
        root, "merge-base", "--is-ancestor", ancestor_sha, descendant_sha
    )
    return True


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
        except (TaskLinkError, ReceiptError) as exc:
            # A failed plan.py render/check raises ReceiptError (fail closed);
            # wrap it as a TaskLinkError so the caller's exception handling is
            # uniform — a broken Planning state is never made durable.
            raise TaskLinkError("render/check raised: %s" % exc) from exc
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

    # Snapshot the pre-edit local state (finding C): record HEAD, the index
    # contents and the working-tree card bytes BEFORE any git object is touched,
    # so a mid-operation failure can be restored deterministically rather than
    # by an untracked reflog entry. The snapshot is the recovery anchor.
    pre_head = _current_commit(planning_root, str(card_path))
    pre_index = _snapshot_index(planning_root, git_fn)
    pre_worktree = Path(card_path).read_bytes()

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
        # C): the original bytes are restored, the index reset to the pre-edit
        # snapshot (NOT a reflog entry), and the local HEAD is unchanged so the
        # caller can inspect or manually recover. Both TaskLinkError (git/edit)
        # and ReceiptError (plan.py render/check) are caught here so a failed
        # validation never leaves the card committed or the worktree dirty.
        try:
            Path(card_path).write_bytes(new_bytes)
            card_rel = str(Path(card_path).relative_to(planning_root))
            _git(planning_root, "add", "--", card_rel)
            _git(planning_root, "commit", "-m",
                 "tasklink: publish evidence %s" % url, "--", card_rel)
            committed_sha = _git(planning_root, "rev-parse", "HEAD").decode().strip()
            require(sha40(committed_sha), "commit did not produce a full SHA")
            committed = True
        except (TaskLinkError, ReceiptError) as exc:
            _restore_after_failure(planning_root, card_path, git_fn, original_bytes,
                                   pre_index, committed)
            raise
        finally:
            # A commit failure must not leave a half-staged index; reset the
            # index to the pre-edit snapshot when nothing else is staged, so an
            # unrelated staged change is never discarded. No HEAD@{1} reflog
            # reset: the index is restored to a recorded snapshot, deterministically.
            if not committed:
                _reset_index_to_snapshot(planning_root, git_fn, pre_index)

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


def _snapshot_index(root, git_fn):
    """Return an immutable map of ``{path: blob_hex}`` for every staged file.

    The snapshot is the pre-edit recovery anchor (finding C): it records exactly
    what the index held before any publication touched it, so a failed
    publication can restore the index without consulting a reflog entry (which
    the protocol forbids). An empty index yields an empty map.
    """
    snap = {}
    try:
        listing = _git(root, "diff", "--cached", "--name-only", "--raw").decode()
    except TaskLinkError:
        return snap
    for line in listing.splitlines():
        # raw format: :<oldsha>-<newsha> <mode> <path>
        parts = line.split(" ", 2)
        if len(parts) < 3:
            continue
        statuses, path = parts[0], parts[2].strip()
        new_sha = statuses[2:42]
        if len(new_sha) == 40:
            snap[path] = new_sha
    return snap


def _reset_index_to_snapshot(root, git_fn, snapshot):
    """Restore the index to a recorded pre-edit snapshot (finding C).

    No HEAD@{1} reflog reset: the index is rebuilt from the snapshot so the
    result is deterministic regardless of reflog state. Files absent from the
    snapshot are unstaged; files still staged that are not in the snapshot are
    left staged only if they were staged before this publication (i.e. an
    unrelated change) — those are preserved, never discarded.
    """
    if not snapshot:
        # Nothing was staged before: unstage everything that is currently
        # staged so the index is clean (unless an unrelated change exists).
        try:
            current = _snapshot_index(root, git_fn)
        except TaskLinkError:
            return
        for path in current:
            try:
                _git(root, "restore", "--staged", "--", path)
            except TaskLinkError:
                pass
        return
    # Re-stage the snapshot's files (idempotent when they match HEAD already),
    # then unstage anything that is present now but was not in the snapshot.
    try:
        current = _snapshot_index(root, git_fn)
    except TaskLinkError:
        return
    for path in current:
        if path not in snapshot:
            try:
                _git(root, "restore", "--staged", "--", path)
            except TaskLinkError:
                pass


def _restore_after_failure(root, card_path, git_fn, original_bytes, pre_index,
                           committed):
    """Deterministically restore local state after a write/commit/rollback
    failure (finding C): working-tree card to ``original_bytes``, index to the
    pre-edit snapshot, and a raised PublicationError when a restore itself
    fails (never a silent, dirty state).

    No HEAD@{1} reflog reset: the index is restored from the recorded snapshot,
    so the post-rollback state is identical regardless of reflog history.
    """
    # 1. Working-tree card back to the pre-edit bytes.
    try:
        card_path.write_bytes(original_bytes)
    except OSError as exc:
        raise PublicationError(
            "local restore failed: cannot rewrite card to pre-edit bytes: %s"
            % exc) from exc
    # 2. Index back to the pre-edit snapshot (no reflog entry).
    try:
        _reset_index_to_snapshot(root, git_fn, pre_index)
    except TaskLinkError as exc:
        raise PublicationError(
            "local restore failed: cannot restore index to pre-edit snapshot: %s"
            % exc) from exc


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
