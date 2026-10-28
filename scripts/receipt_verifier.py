#!/usr/bin/env python3
"""Live GitHub verification and retained Git-history verification for one pilot gate.

Approval identity (CODE-AUDIT-2026-10-27 F4/F5): the runner's GITHUB_* values are
claims only. The proof is the authenticated workflow-run object retrieved through
the API plus the pinned reviewed source. A caller-set environment string can
never stand in for identity, and a wrong actor, triggering actor, event,
repository, ref or producer workflow definition fails closed. Approval evidence
auto-resolves from the contributor PR reference; receipt path, candidate SHA and
plan revision are derived, never manual input. The sole approval edit publishes
through W2's receipt_writer interface; nothing here exports a patch. No push in
this chain carries its own credentials: every Planning push (receipt and card)
goes through the single writer mechanism with in-process push auth, and a
missing write token fails closed before any push (planning addendum cc379b1-1).
"""
import argparse
import datetime as dt
import io
import json
import os
import re
import subprocess
import sys
import zipfile
from pathlib import Path

GATE = "public-contracts"
REPOSITORY = "mcpdev80/baseharbor"
PLAN_REPOSITORY = "mcpdev80/baseharbor-plan"
REPOSITORY_ID = 1360667669
OWNER = "mcpdev80"
API_VERSION = "2026-03-10"
SHA = re.compile(r"[0-9a-f]{40}\Z")
PLAN_PATH = "evidence/receipt-plans/public-contracts.json"
RECEIPT_DIR = "evidence/receipts/public-contracts"
PRODUCT_WORKFLOW = ".github/workflows/ci.yml"
APPROVAL_WORKFLOW = ".github/workflows/receipt-approval.yml"
HELPER_SOURCE = "scripts/receipt_verifier.py"
# The ONLY Planning write credential the approval chain knows about; it is
# checked for presence before any push and consumed exclusively by the
# receipt_writer publication mechanism's in-process auth (planning addendum
# cc379b1-1). Its value is never printed, logged or interpolated anywhere.
PUSH_CREDENTIAL_ENV = "PLANNING_RECEIPT_WRITE_TOKEN"
# The product default branch is main. Only an authenticated run whose real ref
# is main may issue trust (CODE-AUDIT F7). This tuple is a policy allowlist
# cross-checked against the authenticated run proof; it is never an identity
# substitute (CODE-AUDIT F4). No other ref is permitted; a transitional or
# otherwise untrusted ref is rejected, not accepted (see
# test_approve_rejects_untrusted_ref under TrustedSourceTest).
POLICY_BRANCHES = ("main",)


class ReceiptError(ValueError):
    """Receipt cannot qualify the requested candidate."""


def require(condition, reason):
    if not condition:
        raise ReceiptError(reason)


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":"),
                       ensure_ascii=True, allow_nan=False) + "\n").encode()


def unique(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, "duplicate JSON key")
        result[key] = value
    return result


def decode(raw, limit):
    require(0 < len(raw) < limit, "invalid byte length")
    try:
        value = json.loads(raw, object_pairs_hook=unique,
                           parse_constant=lambda _: require(False, "nonfinite JSON"))
    except (ValueError, UnicodeError) as exc:
        raise ReceiptError("invalid JSON") from exc
    require(isinstance(value, dict), "JSON object required")
    return value


def full_sha(value):
    return isinstance(value, str) and SHA.fullmatch(value) is not None


def extract_facts(producer_plan, *, artifact_size, run_id=None, attempt=None, workflow_id=None):
    """Bounded, strict extraction of the authenticated producer facts (D3).

    Derives head/candidate/base SHAs from the real producer plan object and
    run id, attempt and workflow id from the caller-provided run context.
    Every input is validated for type, length and hexness BEFORE anything is
    returned, so a malformed/missing/null/duplicate field blocks at extraction
    and no consumer is invoked with spoofed context. Duplicate JSON keys are
    rejected by the canonical decoder that built *producer_plan*; this guards
    the resolved value.

    Parameters
    ----------
    producer_plan : dict
        The decoded `pr-plan.json` payload from the authenticated artifact.
    artifact_size : int
        The artifact size in bytes from the authenticated artifact object,
        already bounded by the caller against the documented maximum.
    run_id : int or None
        The authenticated producer run id (positive integer), from the
        API lookup, not from the producer plan itself.
    attempt : int or None
        The attempt number (must be 1 for pilot).
    workflow_id : int or None
        The authenticated producer workflow id (positive integer).

    Returns
    -------
    dict
        ``{"head_sha", "candidate_sha", "base_sha", "run_id", "attempt",
        "workflow_id"}`` with strict positive-int and 40-hex SHA types.

    Raises
    ------
    ReceiptError
        On any missing/null/malformed/duplicate field, wrong type, non-positive
        integer, non-hex/short SHA, or oversized artifact.
    """
    require(isinstance(producer_plan, dict), "producer plan is not an object")
    require(isinstance(artifact_size, int) and type(artifact_size) is not bool
            and 0 < artifact_size <= 65536, "artifact size must be a bounded positive int")
    require(producer_plan.get("schema") == "baseharbor.pr-gate-plan/v1",
            "unknown producer plan schema")

    def _sha40(field):
        raw = producer_plan.get(field)
        require(isinstance(raw, str) and SHA.fullmatch(raw) is not None,
                "missing or malformed %s (need 40 lowercase hex)" % field)
        return raw

    head_sha = _sha40("head_sha")
    integration_sha = _sha40("integration_sha")
    base_sha = _sha40("target_base_sha")

    run_id = producer_plan.get("run_id")
    require(run_id is not None and type(run_id) is int and run_id > 0,
            "run_id must be a positive integer")
    attempt = producer_plan.get("attempt")
    require(attempt is not None and type(attempt) is int and attempt == 1,
            "attempt must be the integer 1 (pilot restriction)")
    workflow_id = producer_plan.get("workflow_id")
    require(workflow_id is not None and type(workflow_id) is int and workflow_id > 0,
            "workflow_id must be a positive integer")

    return {"head_sha": head_sha, "candidate_sha": integration_sha,
            "base_sha": base_sha, "run_id": run_id, "attempt": attempt,
            "workflow_id": workflow_id}


def receipt(raw):
    value = decode(raw, 500)
    require(set(value) == {"gate", "run", "sha", "status", "plan", "approved"},
            "unexpected receipt fields")
    require(value["gate"] == GATE and value["status"] == "pass", "unsupported gate/status")
    run = value["run"]
    require(isinstance(run, dict) and set(run) == {"id", "attempt"}
            and all(type(n) is int and n > 0 for n in run.values()), "invalid run/attempt")
    require(full_sha(value["sha"]) and full_sha(value["plan"]), "full SHAs required")
    approval = value["approved"]
    if approval is not None:
        require(isinstance(approval, dict) and set(approval) == {"by", "at", "receipt_commit"}
                and approval["by"] == OWNER and full_sha(approval["receipt_commit"]),
                "invalid approval")
        try:
            parsed = dt.datetime.strptime(approval["at"], "%Y-%m-%dT%H:%M:%SZ")
            require(parsed.strftime("%Y-%m-%dT%H:%M:%SZ") == approval["at"], "invalid timestamp")
        except (ValueError, TypeError) as exc:
            raise ReceiptError("invalid approval timestamp") from exc
    require(raw == canonical(value), "receipt must use canonical bytes")
    return value


def path_for(value):
    return f"evidence/receipts/{GATE}/{value['run']['id']}-{value['run']['attempt']}.json"


def git(root, *args):
    env = dict(os.environ, GIT_NO_LAZY_FETCH="1")
    result = subprocess.run(["git", "-C", str(root), *args], capture_output=True, env=env)
    require(result.returncode == 0, "Git operation failed: " + args[0])
    return result.stdout


def git_stderr(root, *args):
    """Run a git command and return (returncode, stdout, stderr) without raising,
    so callers can inspect a rejection's diagnostics (e.g. non-fast-forward)."""
    env = dict(os.environ, GIT_NO_LAZY_FETCH="1")
    result = subprocess.run(["git", "-C", str(root), *args], capture_output=True, env=env)
    return result.returncode, result.stdout, result.stderr


def tree_entry(root, revision, path):
    require(full_sha(revision), "full Git revision required")
    entry = git(root, "ls-tree", revision, "--", path).decode().split()
    require(len(entry) == 4 and entry[0:2] == ["100644", "blob"] and entry[3] == path,
            "regular committed file required")
    return entry[2]


def blob(root, revision, path):
    oid = tree_entry(root, revision, path)
    require(int(git(root, "cat-file", "-s", oid)) < 4096, "committed file too large")
    return git(root, "show", oid)


def source_blob(root, revision, path, limit=262_144):
    """Blob read for reviewed workflow/helper source files (not receipts)."""
    oid = tree_entry(root, revision, path)
    require(int(git(root, "cat-file", "-s", oid)) < limit, "reviewed source file too large")
    return git(root, "show", oid)


def load_plan(root, revision):
    plan = decode(blob(root, revision, PLAN_PATH), 4096)
    require(set(plan) == {"gate", "repository", "workflow_id", "head_sha", "candidate_sha",
                          "base_sha", "task"}, "unexpected plan fields")
    require(plan["gate"] == GATE and plan["repository"] == REPOSITORY
            and type(plan["workflow_id"]) is int and plan["workflow_id"] > 0,
            "untrusted plan identity")
    require(all(full_sha(plan[k]) for k in ("head_sha", "candidate_sha", "base_sha")),
            "invalid plan source inputs")
    require(isinstance(plan["task"], str)
            and re.fullmatch(r"tasks/[a-z0-9-]+\.md", plan["task"]), "invalid task reference")
    return plan


def witness_failure(result):
    """Classify a failed witness call. A transport failure is never reported as
    proof of a missing run; only an explicit HTTP 404 is (CODEMAN-START section 5)."""
    text = result.stderr.decode(errors="replace").lower()
    return "run or resource not found" if "404" in text else "witness transport failure"


def github(path, *, binary=False):
    require(path.startswith((f"repos/{REPOSITORY}/actions/runs/",
                              f"repos/{REPOSITORY}/actions/artifacts/",
                              f"repos/{REPOSITORY}/pulls/",
                              f"repos/{REPOSITORY}/issues/")), "unsupported API path")
    result = subprocess.run(["gh", "api", "--method", "GET", "-H",
                             f"X-GitHub-Api-Version: {API_VERSION}", path], capture_output=True)
    require(result.returncode == 0, "GitHub witness unavailable: " + witness_failure(result))
    if binary:
        require(0 < len(result.stdout) <= 65536, "plan ZIP too large")
        return result.stdout
    return decode(result.stdout, 2_000_000)


def identity(api=None):
    """Genuine Owner/ref validation: environment claims are cross-checked against
    the authenticated run object. No caller-set string substitutes for identity."""
    if api is None:
        api = github
    claim = "authenticated Owner action on the protected default branch required"
    require(os.environ.get("GITHUB_EVENT_NAME") == "workflow_dispatch", claim)
    require(os.environ.get("GITHUB_REPOSITORY") == REPOSITORY, claim)
    require(os.environ.get("GITHUB_ACTOR") == OWNER
            and os.environ.get("GITHUB_TRIGGERING_ACTOR") == OWNER, claim)
    run_id = os.environ.get("GITHUB_RUN_ID") or ""
    require(re.fullmatch(r"[0-9]+", run_id) is not None, claim)
    run = api(f"repos/{REPOSITORY}/actions/runs/{int(run_id)}")
    require(isinstance(run, dict), claim)
    require(run.get("id") == int(run_id), claim + ": run identity mismatch")
    require(run.get("event") == "workflow_dispatch", claim + ": event proof mismatch")
    require(run.get("path") == APPROVAL_WORKFLOW, claim + ": workflow definition proof mismatch")
    repository = run.get("repository") or {}
    require(repository.get("id") == REPOSITORY_ID
            and repository.get("full_name") == REPOSITORY, claim + ": repository proof mismatch")
    branch = run.get("head_branch")
    require(isinstance(branch, str) and branch in POLICY_BRANCHES, claim + ": untrusted ref")
    require(os.environ.get("GITHUB_REF") == "refs/heads/" + branch, claim + ": ref claim mismatch")
    head = run.get("head_sha")
    require(full_sha(head) and head == os.environ.get("GITHUB_SHA"),
            claim + ": source claim mismatch")
    actor = run.get("actor")
    if actor is not None:
        login = actor.get("login") if isinstance(actor, dict) else actor
        require(login == OWNER, claim + ": actor proof mismatch")
    return run


def producer_identity(product, plan):
    """Narrow pilot eligibility rule (CODE-AUDIT F8): the producer's ci.yml bytes
    must equal the reviewed protected-base definition. A workflow-changing PR,
    including any fork, cannot issue trust through matching job names."""
    head_definition = source_blob(product, plan["head_sha"], PRODUCT_WORKFLOW)
    base_definition = source_blob(product, plan["base_sha"], PRODUCT_WORKFLOW)
    require(head_definition == base_definition,
            "producer workflow definition differs from reviewed protected base")


def pinned_source(product, revision):
    """Honest trusted-source check: the running verifier and the approval
    workflow bytes must equal the reviewed pinned definition; the executing
    checkout is the authenticated run head."""
    require(full_sha(revision or ""), "reviewed helper source pin must be a full SHA")
    require(source_blob(product, revision, HELPER_SOURCE)
            == Path(__file__).resolve().read_bytes(),
            "running verifier differs from reviewed pinned source")
    source = os.environ.get("GITHUB_SHA") or ""
    require(full_sha(source), "authenticated source SHA required")
    require(source_blob(product, revision, APPROVAL_WORKFLOW)
            == source_blob(product, source, APPROVAL_WORKFLOW),
            "running approval workflow differs from reviewed pinned definition")


def live(value, plan, api=None, product=None):
    if api is None:
        api = github
    require(value["approved"] is None, "already approved; use retained verification")
    require(value["run"]["attempt"] == 1, "pilot requires a fresh run, not an Actions rerun")
    require(value["sha"] == plan["candidate_sha"], "wrong candidate SHA")
    prefix = f"repos/{REPOSITORY}/actions/runs/{value['run']['id']}"
    run = api(prefix)
    require(run.get("id") == value["run"]["id"]
            and run.get("run_attempt") == value["run"]["attempt"], "wrong run/attempt")
    run = api(f"{prefix}/attempts/{value['run']['attempt']}")
    require(run.get("id") == value["run"]["id"]
            and run.get("run_attempt") == value["run"]["attempt"]
            and run.get("repository", {}).get("id") == REPOSITORY_ID
            and run.get("repository", {}).get("full_name") == REPOSITORY
            and run.get("workflow_id") == plan["workflow_id"]
            and run.get("path") == ".github/workflows/ci.yml"
            and run.get("event") == "pull_request", "wrong run provenance")
    require(run.get("head_sha") == plan["head_sha"], "wrong run source SHA")
    require(run.get("status") == "completed" and run.get("conclusion") == "success",
            "run is not successful")
    jobs, total = [], None
    for page in range(1, 11):
        inventory = api(f"{prefix}/attempts/{value['run']['attempt']}/jobs?per_page=100&page={page}")
        count, rows = inventory.get("total_count"), inventory.get("jobs")
        require(type(count) is int and 0 < count <= 1000 and isinstance(rows, list)
                and (total is None or count == total) and 0 < len(rows) <= 100,
                "invalid or changing job inventory")
        total = count
        jobs.extend(rows)
        if len(jobs) >= total:
            break
    require(len(jobs) == total and all(isinstance(j, dict) for j in jobs), "incomplete jobs")
    ids = [j.get("id") for j in jobs]
    require(all(type(i) is int and i > 0 for i in ids) and len(set(ids)) == len(ids),
            "ambiguous job inventory")
    selected = [j for j in jobs if j.get("name") == "PR · Public Contracts"]
    require(len(selected) == 1, "required gate job missing or ambiguous")
    job = selected[0]
    require(job.get("run_id") == value["run"]["id"]
            and job.get("head_sha") == plan["head_sha"]
            and job.get("status") == "completed" and job.get("conclusion") == "success",
            "required gate job is not successful")
    producer = [j for j in jobs if j.get("name") == "PR · Trusted Plan"]
    require(len(producer) == 1 and producer[0].get("status") == "completed"
            and producer[0].get("conclusion") == "success"
            and producer[0].get("run_id") == value["run"]["id"]
            and producer[0].get("head_sha") == plan["head_sha"], "trusted plan job missing")
    inventory = api(prefix + "/artifacts?per_page=100")
    artifacts = inventory.get("artifacts")
    require(isinstance(artifacts, list) and len(artifacts) <= 100
            and inventory.get("total_count") == len(artifacts)
            and all(isinstance(a, dict) for a in artifacts), "incomplete artifact inventory")
    matches = [a for a in artifacts if a.get("name") == "trusted-pr-plan"]
    require(len(matches) == 1, "trusted plan artifact missing or ambiguous")
    artifact = matches[0]
    require(type(artifact.get("id")) is int and artifact["id"] > 0
            and artifact.get("expired") is False
            and type(artifact.get("size_in_bytes")) is int
            and 0 < artifact["size_in_bytes"] <= 65536
            and artifact.get("workflow_run", {}).get("id") == value["run"]["id"]
            and artifact.get("workflow_run", {}).get("head_sha") == plan["head_sha"],
            "invalid trusted plan artifact")
    raw = api(f"repos/{REPOSITORY}/actions/artifacts/{artifact['id']}/zip", binary=True)
    require(type(raw) is bytes and len(raw) <= 65536, "invalid plan ZIP")
    try:
        with zipfile.ZipFile(io.BytesIO(raw)) as archive:
            entries = archive.infolist()
            require(len(entries) == 1 and entries[0].filename == "pr-plan.json"
                    and 0 < entries[0].file_size < 65536, "unexpected plan ZIP contents")
            witnessed = decode(archive.read(entries[0]), 65536)
    except (zipfile.BadZipFile, RuntimeError, NotImplementedError) as exc:
        raise ReceiptError("unreadable plan ZIP") from exc
    require(witnessed.get("schema") == "baseharbor.pr-gate-plan/v1"
            and witnessed.get("coverage_status") == "resolved"
            and "integration/static/public-contracts" in witnessed.get("selected_gates", [])
            and witnessed.get("head_sha") == plan["head_sha"]
            and witnessed.get("target_base_sha") == plan["base_sha"]
            and witnessed.get("integration_sha") == value["sha"], "producer plan/source mismatch")
    if product is not None:
        producer_identity(product, plan)


def verify_file(root, revision, path, candidate, plan_revision):
    raw = blob(root, revision, path)
    value = receipt(raw)
    require(path == path_for(value), "wrong receipt path")
    require(value["sha"] == candidate and value["plan"] == plan_revision,
            "receipt does not match required candidate/plan")
    plan = load_plan(root, plan_revision)
    require(plan["candidate_sha"] == candidate, "plan candidate mismatch")
    return raw, value, plan


def retained(root, revision, path, candidate, plan_revision):
    raw, value, _ = verify_file(root, revision, path, candidate, plan_revision)
    require(value["approved"] is not None, "Owner approval missing")
    original = value["approved"]["receipt_commit"]
    git(root, "merge-base", "--is-ancestor", original, revision)
    before = dict(value, approved=None)
    require(blob(root, original, path) == canonical(before), "original receipt was edited")
    introduction = git(root, "log", "--full-history", "--format=%H", original,
                       "--", path).splitlines()
    require(introduction == [original.encode()], "receipt commit must introduce a new path")
    history = git(root, "log", "--full-history", "--format=%H", f"{original}..{revision}",
                  "--", path).splitlines()
    require(len(history) == 1, "receipt must have exactly one approval edit and no later edits")
    require(blob(root, history[0].decode(), path) == raw, "approval commit mismatch")
    return value


def approve(root, revision, path, candidate, plan_revision, api=None, *, product=None, pin=None):
    """Re-verify live, then create the single allowed approval edit at the
    original introduction commit. Idempotent when already approved and retained-
    verified; never a second edit."""
    if api is None:
        api = github
    identity(api)
    require(product is not None, "reviewed product checkout required for approval")
    pinned_source(product, pin)
    raw, value, plan = verify_file(root, revision, path, candidate, plan_revision)
    head = git(root, "rev-parse", "HEAD").decode().strip()
    git(root, "merge-base", "--is-ancestor", revision, head)
    require(git(root, "log", "--full-history", "--format=%H", revision, "--", path)
            .splitlines() == [revision.encode()], "receipt commit must introduce a new path")
    blob(root, plan_revision, plan["task"])
    if receipt(blob(root, head, path))["approved"] is not None:
        retained(root, head, path, candidate, plan_revision)
        return head
    live(value, plan, api=api, product=product)
    value["approved"] = {"by": OWNER, "at": dt.datetime.now(dt.timezone.utc)
                         .strftime("%Y-%m-%dT%H:%M:%SZ"), "receipt_commit": revision}
    updated = canonical(value)
    receipt(updated)
    from receipt_writer import commit_file
    return commit_file(root, path, updated, expected=raw)


def resolve_pull(pr_number, api=None):
    """The human selects the contributor PR; its authenticated head/base tuple is
    the binding. Branch names are never used (CODEMAN-START section 4)."""
    if api is None:
        api = github
    pr = api(f"repos/{REPOSITORY}/pulls/{pr_number}")
    require(isinstance(pr, dict) and pr.get("number") == pr_number
            and pr.get("state") == "open", "contributor PR reference is stale or closed")
    head, base = pr.get("head") or {}, pr.get("base") or {}
    require((base.get("repo") or {}).get("full_name") == REPOSITORY,
            "PR base repository is not the protected repository")
    require(full_sha(head.get("sha")) and full_sha(base.get("sha")), "PR source tuple incomplete")
    return {"number": pr_number, "head_sha": head["sha"], "base_sha": base["sha"]}


def resolve_evidence(planning, pull, api=None):
    """Find exactly one published receipt whose plan is bound to this PR tuple."""
    if api is None:
        api = github
    head = git(planning, "rev-parse", "HEAD").decode().strip()
    listing = git(planning, "ls-tree", "-r", "--name-only", head, "--", RECEIPT_DIR).decode()
    matches = []
    for entry in listing.splitlines():
        value = receipt(blob(planning, head, entry))
        plan = load_plan(planning, value["plan"])
        if (plan["head_sha"] == pull["head_sha"] and plan["base_sha"] == pull["base_sha"]
                and plan["candidate_sha"] == value["sha"]):
            introduction = git(planning, "log", "--full-history", "--reverse", "--format=%H",
                               value["plan"] + ".." + head, "--", entry).decode().splitlines()
            require(introduction, "receipt is not reachable in Planning history: " + entry)
            matches.append({"path": entry, "revision": introduction[0],
                            "candidate": value["sha"], "plan_revision": value["plan"],
                            "approved": value["approved"] is not None, "task": plan["task"]})
    require(len(matches) == 1,
            "PR-to-receipt binding missing or ambiguous; approve nothing and re-resolve")
    return matches[0]


def front_matter(text):
    """Parse the YAML front matter of a Planning task note (CODE-AUDIT F4).

    Returns the metadata mapping, or None when the note has no front-matter
    block. Malformed front matter is a hard ReceiptError, never a silent skip:
    the mapping authority must not fall back to loose text search.
    """
    try:
        import yaml
    except ImportError as exc:
        raise ReceiptError("Planning schema resolver requires PyYAML") from exc
    lines = text.splitlines()
    if not lines or lines[0].strip() != "---":
        return {}
    for i in range(1, len(lines)):
        if lines[i].strip() == "---":
            try:
                meta = yaml.safe_load("\n".join(lines[1:i]))
            except yaml.YAMLError as exc:
                raise ReceiptError("task note front matter is invalid YAML") from exc
            if meta is None:
                return {}
            require(isinstance(meta, dict), "task note front matter must be a mapping")
            return meta
    raise ReceiptError("task note front matter block is unterminated")


def task_pr_associations(planning, revision, name):
    """PR markers declared in one task note's schema front matter at `revision`.

    Quoted inline lists, block lists and block metadata all resolve identically
    because the Planning YAML parser is the single authority (CODE-AUDIT F4);
    body prose is never a binding source.
    """
    meta = front_matter(git(planning, "show", f"{revision}:{name}")
                        .decode(errors="replace"))
    entries = meta.get("prs")
    if entries is None:
        return []
    if isinstance(entries, str):
        entries = [entries]
    require(isinstance(entries, list), "invalid prs metadata in task note: " + name)
    return [str(entry).strip() for entry in entries]


def task_binding(planning, plan_revision, task, pr_number, api=None):
    """Deterministic trusted PR-to-task mapping: exactly one Planning task note
    lists this repository PR in its schema front matter, it must be the plan's
    task, and the forward PR→task resolver must resolve the same task
    (CODE-AUDIT F4). One Planning-schema-aware resolver serves the backward
    task→PR binding at the bound revision; the shared tasklink
    resolve_task_from_pr() serves the forward lookup. Both must agree.

    A caught resolver error is NEVER authorization: any resolution failure
    blocks. The api parameter is ALWAYS forwarded to resolve_task_from_pr — in
    tests an injected API keeps the resolver hermetic; a real network call is
    never a fallback path in this lane's tests.
    """
    import receipt_tasklink
    expected_task = receipt_tasklink.resolve_task_from_pr(pr_number, planning, api)
    names = git(planning, "ls-tree", "-r", "--name-only", plan_revision, "--",
                "tasks/").decode(errors="replace").splitlines()
    marker = f"{REPOSITORY}#{pr_number}"
    bound = [name for name in names
             if marker in task_pr_associations(planning, plan_revision, name)]
    # Exactly one task note must claim the PR, and it must be the plan's task.
    # When the PR body resolves, that task must equal the bound task too.
    require(bound == [task],
            "deterministic PR-to-task association missing or ambiguous")
    require(task == expected_task,
            "PR-resolved task does not match the bound task note")


def evidence_reference(publication, path):
    """Immutable public reference to one published receipt at its durable
    publication SHA (CODEX-REVIEW F5): the reference is content-addressed, so
    later Planning main movement can never redirect it."""
    require(full_sha(publication), "evidence reference requires a full publication SHA")
    require(str(path).startswith("evidence/receipts/"),
            "evidence reference requires a published receipt path")
    return f"https://github.com/{PLAN_REPOSITORY}/blob/{publication}/{path}"


def writer_push_git(planning, path):
    """Git adapter that routes every push through receipt_writer.publish().

    The card publication reuses the SAME writer mechanism as the receipt
    publication (in-process push authentication, bounded fast-forward re-apply,
    durable read-back) instead of a second, self-credentialled push path
    (planning addendum cc379b1-1). Non-push git operations are ordinary
    read/commit calls against the checked-out repository. No credential value
    is ever attached to an argument or included in an error message.
    """
    def git_fn(root, *args, check=True):
        if args and args[0] == "push":
            import receipt_writer
            local = ""
            for token in args[1:]:
                left, sep, right = str(token).partition(":")
                if sep and full_sha(left) and right.startswith("refs/"):
                    local = left
            require(full_sha(local), "card publication push carried no local commit")
            published = receipt_writer.publish(Path(root), path, local)
            require(isinstance(published, (tuple, list)) and len(published) == 3,
                    "card publication push returned no durable tuple")
            return b""
        env = dict(os.environ, GIT_NO_LAZY_FETCH="1")
        result = subprocess.run(["git", "-C", str(root), *args], capture_output=True, env=env)
        require(result.returncode == 0, "Git operation failed: " + str(args[0]))
        return result.stdout
    return git_fn


def publish_approval(planning, path, approval_commit, task=None, candidate_sha=None,
                     plan_revision=None):
    """Publish the sole approval edit through W2's idempotent publication helper.

    Interface: receipt_writer.publish(root, path, local_commit) ->
    (path, introduction_commit, publication_commit) as present on W2's branch.
    Finding BF-W3-1: the frozen develop base this PR is built on exposes only
    commit_file/write, so until W2's branch merges this call fails closed here
    instead of re-implementing push logic or exporting a patch (CODE-AUDIT F5).

    Returns the FINAL durable publication revision: when the card step runs,
    that is tasklink's card-publication SHA (which contains both the receipt
    edit and the evidence reference); otherwise the receipt publication. The
    caller verifies retained history at exactly this revision.

    Credential policy (planning addendum cc379b1-1): this path never invents or
    attaches its own push credentials. Every Planning push is delegated to the
    single receipt_writer publication mechanism, whose in-process push auth is
    configured by the workflow. A missing write token fails closed BEFORE any
    push is attempted, and no token value ever enters a message or log.
    """
    import receipt_writer
    helper = getattr(receipt_writer, "publish_approval_edit", None)
    if helper is None or not callable(helper):
        require(False,
                "publication blocked: W2 receipt_writer.publish_approval_edit interface missing (finding BF-W3-1)")
    require(os.environ.get(PUSH_CREDENTIAL_ENV, "") != "",
            "publication blocked: Planning write credential " + PUSH_CREDENTIAL_ENV
            + " is not configured; failing closed before any push")
    # The approval edit must modify exactly the unapproved base bytes that were
    # verified live; any other local content means the working tree diverged.
    base_bytes = git(planning, "show", f"{approval_commit}^:{path}")
    try:
        publication = helper(planning, path, approval_commit, expected_base=base_bytes)
    except Exception as exc:
        err_msg = "publication blocked: W2 publish_approval_edit failed (%s; finding BF-W3-1)" % exc
        raise ReceiptError(err_msg) from exc
    require(full_sha(publication), "publication returned no durable commit")
    # Automatic evidence publication (CODEX-REVIEW F5 verifier side): the card
    # edit goes through tasklink and its durable publication SHA is captured and
    # returned — never the older receipt publication alone. Failures propagate
    # (fail closed); a local-only card write must never report complete
    # publication. The evidence entry is an immutable public reference bound to
    # the durable receipt edit, not a mutable relative path, and the card push
    # reuses the same writer mechanism through the _git_fn seam (addendum).
    if task is not None and candidate_sha is not None and plan_revision is not None:
        import receipt_tasklink
        receipt_commit = git(planning, "log", "--format=%H", "-n", "1",
                             publication, "--", path).decode().split()
        require(len(receipt_commit) == 1 and full_sha(receipt_commit[0]),
                "receipt edit is not reachable in the durable publication history")
        reference = evidence_reference(receipt_commit[0], path)
        card_publication = receipt_tasklink.publish_evidence(
            planning, task, reference, candidate_sha, plan_revision,
            _git_fn=writer_push_git(planning, task))
        require(full_sha(card_publication),
                "tasklink returned no durable card publication SHA")
        durable = git(planning, "show",
                      f"{card_publication}:{task}").decode(errors="replace")
        require(reference in durable,
                "immutable evidence reference is not present at the card publication")
        return card_publication
    return publication


def approve_pr(planning, product, pr_number, api=None, pin=None):
    """Owner decision path: only the PR number is input; everything else resolves,
    re-verifies live, edits once, publishes and reads the durable approval back."""
    if api is None:
        api = github
    identity(api)
    pull = resolve_pull(pr_number, api)
    evidence = resolve_evidence(planning, pull, api)
    task_binding(planning, evidence["plan_revision"], evidence["task"], pr_number, api)
    approval = approve(planning, evidence["revision"], evidence["path"], evidence["candidate"],
                       evidence["plan_revision"], api=api, product=product, pin=pin)
    published = publish_approval(planning, evidence["path"], approval,
                                  task=evidence["task"],
                                  candidate_sha=evidence["candidate"],
                                  plan_revision=evidence["plan_revision"])
    retained(planning, published, evidence["path"], evidence["candidate"],
             evidence["plan_revision"])
    return {"path": evidence["path"], "introduction": evidence["revision"],
            "approval": approval, "publication": published,
            "plan": evidence["plan_revision"], "candidate": evidence["candidate"]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("live", "approve", "retained"))
    parser.add_argument("--planning", type=Path, required=True)
    parser.add_argument("--product", type=Path)
    parser.add_argument("--pin", default=os.environ.get("TRUSTED_SOURCE", ""))
    parser.add_argument("--pr", type=int)
    parser.add_argument("--revision")
    parser.add_argument("--receipt")
    parser.add_argument("--candidate")
    parser.add_argument("--plan")
    args = parser.parse_args()
    try:
        if args.mode == "retained":
            missing = [n for n in ("revision", "receipt", "candidate", "plan")
                       if getattr(args, n) in (None, "")]
            if missing:
                parser.exit(2, "missing arguments: " + ", ".join("--" + n for n in missing) + "\n")
            retained(args.planning, args.revision, args.receipt, args.candidate, args.plan)
        elif args.mode == "approve":
            if args.pr is None or args.product is None:
                parser.exit(2, "approve requires --pr and --product; receipt path, candidate "
                               "SHA and plan revision are auto-resolved, never manual input\n")
            result = approve_pr(args.planning, args.product, args.pr, pin=args.pin)
            # Output contract (CODEX-REVIEW F6): every emitted value is the
            # actual returned field of the verified chain — plan is the plan
            # revision, candidate is the approved candidate, publication is the
            # final durable (card) publication. A missing or malformed value is
            # an explicit failure, never a silently skipped output line.
            for name in ("introduction", "approval", "publication", "plan"):
                require(full_sha(result.get(name)),
                        "approval result field is not a full SHA: " + name)
            require(full_sha(result.get("candidate")),
                    "approval result field is not a full SHA: candidate")
            require(isinstance(result.get("path"), str) and result["path"],
                    "approval result has no receipt path")
            print("approved: " + result["path"])
            print("introduction: " + result["introduction"])
            print("approval: " + result["approval"])
            print("publication: " + result["publication"])
            # Auto-resolved durable outputs for the shadow consumer — never
            # manual inputs. Receipt path, plan revision and candidate come from
            # the verified publication, not from dispatch fields.
            print("receipt: " + result["path"])
            print("plan: " + result["plan"])
            print("candidate: " + result["candidate"])
            return
        else:
            missing = [n for n in ("revision", "receipt", "candidate", "plan")
                       if getattr(args, n) in (None, "")]
            if missing:
                parser.exit(2, "missing arguments: " + ", ".join("--" + n for n in missing) + "\n")
            _, value, plan = verify_file(args.planning, args.revision, args.receipt,
                                         args.candidate, args.plan)
            live(value, plan, product=args.product)
        print("checked: " + GATE)
    except (ReceiptError, OSError, TypeError) as exc:
        parser.exit(1, "receipt blocked: " + str(exc) + "\n")


if __name__ == "__main__":
    main()
