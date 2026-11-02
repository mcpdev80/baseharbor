#!/usr/bin/env python3
"""Build post-merge machine evidence without impersonating Owner approval.

This module only builds a verified record; it does not publish an approval or
replace a pre-merge check. API and product checkout must be supplied by a
trusted caller.
"""
import datetime as dt
import io
import re
import zipfile

import receipt_verifier as verifier

OWNER_ID = 22537373
SCHEMA = "baseharbor.post-merge-evidence/v1"
PREFIX = f"repos/{verifier.REPOSITORY}"


def _time(value):
    verifier.require(isinstance(value, str), "missing GitHub timestamp")
    try:
        return dt.datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=dt.timezone.utc)
    except ValueError as exc:
        raise verifier.ReceiptError("invalid GitHub timestamp") from exc


def _sha(value, message):
    verifier.require(verifier.full_sha(value or ""), message)
    return value


def _inventory(api, path, key):
    rows, total = [], None
    for page in range(1, 11):
        result = api(f"{path}?per_page=100&page={page}")
        page_rows, count = result.get(key), result.get("total_count")
        verifier.require(isinstance(page_rows, list) and type(count) is int
                         and 0 <= count <= 1000 and (total is None or total == count)
                         and len(page_rows) <= 100, "invalid or changing inventory")
        total = count
        rows.extend(page_rows)
        if len(rows) >= total:
            break
        verifier.require(page_rows, "incomplete inventory")
    verifier.require(len(rows) == total and all(isinstance(row, dict) for row in rows),
                     "incomplete inventory")
    return rows


def _head_pulls(api, head):
    rows = []
    for page in range(1, 11):
        batch = api(f"{PREFIX}/commits/{head}/pulls?per_page=100&page={page}")
        verifier.require(isinstance(batch, list) and len(batch) <= 100
                         and all(isinstance(item, dict) for item in batch),
                         "invalid PR association inventory")
        rows.extend(batch)
        if len(batch) < 100:
            return rows
    raise verifier.ReceiptError("PR association inventory exceeds limit")


def build(pr_number, run_id, *, api, product):
    """Authenticate a completed PR run and actual Owner merge, then return evidence.

    The API must be an authenticated GitHub API client, and `product` an
    independently fetched product Git repository. Never use this output as an
    approval or as evidence that the merge was permitted before it occurred.
    """
    verifier.require(type(pr_number) is int and pr_number > 0
                     and type(run_id) is int and run_id > 0, "invalid PR/run identity")
    verifier.require(product is not None, "reviewed product checkout required")
    pr = api(f"{PREFIX}/pulls/{pr_number}")
    merger = pr.get("merged_by") or {}
    base = pr.get("base") or {}
    verifier.require(pr.get("number") == pr_number and pr.get("state") == "closed"
                     and pr.get("merged") is True and merger.get("id") == OWNER_ID
                     and merger.get("login") == verifier.OWNER
                     and base.get("ref") == "develop"
                     and (base.get("repo") or {}).get("id") == verifier.REPOSITORY_ID,
                     "PR was not merged by the authorized Owner into develop")
    merge_sha = _sha(pr.get("merge_commit_sha"), "missing merge commit")
    pr_head = _sha((pr.get("head") or {}).get("sha"), "missing merged PR head")
    merged_at = _time(pr.get("merged_at"))
    run = api(f"{PREFIX}/actions/runs/{run_id}")
    verifier.require(run.get("id") == run_id and run.get("run_attempt") == 1
                     and run.get("event") == "pull_request"
                     and run.get("head_sha") == pr_head
                     and run.get("status") == "completed" and run.get("conclusion") == "success"
                     and run.get("path") == ".github/workflows/ci.yml"
                     and (run.get("repository") or {}).get("id") == verifier.REPOSITORY_ID,
                     "producer run provenance or status mismatch")
    associations = run.get("pull_requests")
    verifier.require(isinstance(associations, list) and len(associations) <= 1,
                     "producer run PR association ambiguous")
    head_ref = (pr.get("head") or {}).get("ref")
    head_repo = ((pr.get("head") or {}).get("repo") or {}).get("id")
    verifier.require(isinstance(head_ref, str) and head_ref
                     and run.get("head_branch") == head_ref
                     and type(head_repo) is int and head_repo > 0
                     and (run.get("head_repository") or {}).get("id") == head_repo,
                     "producer PR head repository or branch mismatch")
    pulls = [item for item in _head_pulls(api, pr_head)
             if (item.get("head") or {}).get("sha") == pr_head
             and (item.get("head") or {}).get("ref") == head_ref
             and ((item.get("head") or {}).get("repo") or {}).get("id") == head_repo
             and (item.get("base") or {}).get("ref") == "develop"
             and ((item.get("base") or {}).get("repo") or {}).get("id") == verifier.REPOSITORY_ID]
    verifier.require(len(pulls) == 1 and pulls[0].get("number") == pr_number,
                     "producer PR association missing or ambiguous")
    workflow_id = run.get("workflow_id")
    verifier.require(type(workflow_id) is int and workflow_id > 0, "invalid workflow identity")
    jobs = _inventory(api, f"{PREFIX}/actions/runs/{run_id}/attempts/1/jobs", "jobs")
    selected = [job for job in jobs if job.get("name") == "PR Selected Checks"]
    verifier.require(len(selected) == 1 and selected[0].get("run_id") == run_id
                     and selected[0].get("run_attempt", 1) == 1
                     and selected[0].get("head_sha") == pr_head
                     and selected[0].get("status") == "completed"
                     and selected[0].get("conclusion") == "success"
                     and _time(selected[0].get("completed_at")) <= merged_at,
                     "required checks not successful before merge")
    artifacts = _inventory(api, f"{PREFIX}/actions/runs/{run_id}/artifacts", "artifacts")
    matches = [item for item in artifacts if item.get("name") == "trusted-pr-plan"]
    verifier.require(len(matches) == 1, "trusted plan artifact missing or ambiguous")
    artifact = matches[0]
    verifier.require(type(artifact.get("id")) is int and artifact["id"] > 0
                     and artifact.get("expired") is False
                     and (artifact.get("workflow_run") or {}).get("id") == run_id
                     and (artifact.get("workflow_run") or {}).get("head_sha") == pr_head,
                     "untrusted producer artifact")
    raw = api(f"{PREFIX}/actions/artifacts/{artifact['id']}/zip", binary=True)
    verifier.require(type(raw) is bytes and 0 < len(raw) <= 65536, "invalid plan archive")
    try:
        with zipfile.ZipFile(io.BytesIO(raw)) as archive:
            entries = archive.infolist()
            verifier.require(len(entries) == 1 and entries[0].filename == "pr-plan.json"
                             and 0 < entries[0].file_size < 65536, "invalid plan archive")
            plan_data = verifier.decode(archive.read(entries[0]), 65536)
    except (zipfile.BadZipFile, RuntimeError, NotImplementedError) as exc:
        raise verifier.ReceiptError("invalid plan archive") from exc
    candidate = _sha(plan_data.get("integration_sha"), "missing candidate SHA")
    base_sha = _sha(plan_data.get("target_base_sha"), "missing run base")
    if associations:
        association = associations[0]
        verifier.require(isinstance(association, dict) and association.get("number") == pr_number
                         and (association.get("head") or {}).get("sha") == pr_head
                         and (association.get("base") or {}).get("ref") == "develop"
                         and (association.get("base") or {}).get("sha") == base_sha
                         and ((association.get("base") or {}).get("repo") or {}).get("id")
                             == verifier.REPOSITORY_ID, "producer PR source mismatch")
    plan = dict(candidate_sha=candidate, head_sha=pr_head, base_sha=base_sha,
                workflow_id=workflow_id)
    # Reuse live's producer workflow, artifact, plan and selected pilot gate
    # verification. No Owner approval object is created here.
    verifier.live(dict(approved=None, run=dict(id=run_id, attempt=1), sha=candidate),
                  plan, api=api, product=product)
    candidate_commit = api(f"{PREFIX}/git/commits/{candidate}")
    merge_commit = api(f"{PREFIX}/git/commits/{merge_sha}")
    verifier.require(candidate_commit.get("sha") == candidate and merge_commit.get("sha") == merge_sha,
                     "commit identity mismatch")
    parents = candidate_commit.get("parents")
    verifier.require(isinstance(parents, list) and len(parents) == 2
                     and all(isinstance(p, dict) for p in parents)
                     and {p.get("sha") for p in parents} == {base_sha, pr_head}
                     and base_sha != pr_head, "candidate parent mismatch")
    tree = _sha((candidate_commit.get("tree") or {}).get("sha"), "missing candidate tree")
    verifier.require((merge_commit.get("tree") or {}).get("sha") == tree,
                     "merged tree differs from the tested candidate")
    return dict(schema=SCHEMA, repository=verifier.REPOSITORY, pr=pr_number,
                head_sha=pr_head, base_sha=base_sha, candidate_sha=candidate,
                candidate_tree=tree, merge_commit=merge_sha, run_id=run_id,
                run_attempt=1, workflow_id=workflow_id, merged_by_id=OWNER_ID,
                merged_at=pr["merged_at"])
