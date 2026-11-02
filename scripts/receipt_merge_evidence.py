#!/usr/bin/env python3
"""Build post-merge machine evidence without impersonating Owner approval.

This module only builds a verified record; it does not publish an approval or
replace a pre-merge check. API and product checkout must be supplied by a
trusted caller.
"""
import datetime as dt
import io
import argparse
import json
import re
import subprocess
import zipfile
from pathlib import Path

import receipt_verifier as verifier

OWNER_ID = 22537373
SCHEMA = "baseharbor.post-merge-evidence/v1"
PREFIX = f"repos/{verifier.REPOSITORY}"


def github(path, *, binary=False):
    """Use the verifier adapter, with three narrowly scoped read-only lookups.

    These extra endpoints are needed only for post-merge provenance; do not
    enlarge the approval controller's own API surface.
    """
    prefix = re.escape(PREFIX)
    extra = (
        rf"{prefix}/commits/[0-9a-f]{{40}}/pulls\?per_page=100&page=(?:[1-9]|10)",
        rf"{prefix}/git/commits/[0-9a-f]{{40}}",
        rf"{prefix}/actions/workflows/ci\.yml/runs\?head_sha=[0-9a-f]{{40}}&event=pull_request&per_page=100&page=(?:[1-9]|10)",
    )
    if not any(re.fullmatch(pattern, path) for pattern in extra):
        return verifier.github(path, binary=binary)
    verifier.require(not binary, "binary response not allowed for this endpoint")
    result = subprocess.run(["gh", "api", "--method", "GET", "-H",
                             f"X-GitHub-Api-Version: {verifier.API_VERSION}", path],
                            capture_output=True)
    verifier.require(result.returncode == 0,
                     "GitHub witness unavailable: " + verifier.witness_failure(result))
    if '/commits/' not in path or not '/pulls?' in path:
        return verifier.decode(result.stdout, 2_000_000)
    verifier.require(0 < len(result.stdout) < 2_000_000, "invalid byte length")
    try:
        value = json.loads(result.stdout, object_pairs_hook=verifier.unique,
                           parse_constant=lambda _: verifier.require(False, "nonfinite JSON"))
    except (ValueError, UnicodeError) as exc:
        raise verifier.ReceiptError("invalid JSON") from exc
    verifier.require(isinstance(value, list), "JSON array required")
    return value


def _time(value):
    verifier.require(isinstance(value, str), "missing GitHub timestamp")
    try:
        return dt.datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=dt.timezone.utc)
    except ValueError as exc:
        raise verifier.ReceiptError("invalid GitHub timestamp") from exc


def _sha(value, message):
    verifier.require(verifier.full_sha(value or ""), message)
    return value


def merge_parents_match(parents, base, head):
    """Accept the protected squash policy and an exact ordinary merge."""
    return (isinstance(parents, list) and all(isinstance(p, dict) for p in parents)
            and [p.get('sha') for p in parents] in ([base], [base, head]))


def _inventory(api, path, key):
    rows, total = [], None
    for page in range(1, 11):
        separator = '&' if '?' in path else '?'
        result = api(f"{path}{separator}per_page=100&page={page}")
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


def wakeup_pr(run_id, *, api):
    """Resolve a PR/run-completion or develop-push wakeup from GitHub data."""
    verifier.require(type(run_id) is int and run_id > 0, 'invalid wakeup run')
    run = api(f'{PREFIX}/actions/runs/{run_id}')
    verifier.require(run.get('id') == run_id and run.get('status') == 'completed'
                     and run.get('path') == '.github/workflows/ci.yml'
                     and (run.get('repository') or {}).get('id') == verifier.REPOSITORY_ID,
                     'untrusted wakeup run')
    event = run.get('event')
    verifier.require(event in ('pull_request', 'push'), 'unsupported wakeup event')
    if event == 'push' and run.get('head_branch') != 'develop':
        return None
    head = _sha(run.get('head_sha'), 'missing wakeup head')
    prs = _head_pulls(api, head)
    relevant = [pr for pr in prs if pr.get('merged_at') and pr.get('state') == 'closed'
                and (pr.get('base') or {}).get('ref') == 'develop'
                and ((pr.get('base') or {}).get('repo') or {}).get('id') == verifier.REPOSITORY_ID
                and ((pr.get('head') or {}).get('sha') == head if event == 'pull_request'
                     else pr.get('merge_commit_sha') == head)]
    verifier.require(len(relevant) <= 1, 'ambiguous merged PR wakeup')
    if not relevant:
        return None
    number = relevant[0].get('number')
    verifier.require(type(number) is int and number > 0, 'invalid wakeup PR')
    return number


def build(pr_number, run_id, *, api, product, fetch_sources=None):
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
    if fetch_sources is not None:
        fetch_sources(product, pr_head, base_sha)
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
                     and [p.get("sha") for p in parents] == [base_sha, pr_head]
                     and base_sha != pr_head, "candidate parent mismatch")
    merged_parents = merge_commit.get("parents")
    verifier.require(merge_parents_match(merged_parents, base_sha, pr_head),
                     "merged commit does not preserve tested base and head")
    tree = _sha((candidate_commit.get("tree") or {}).get("sha"), "missing candidate tree")
    verifier.require((merge_commit.get("tree") or {}).get("sha") == tree,
                     "merged tree differs from the tested candidate")
    return dict(schema=SCHEMA, repository=verifier.REPOSITORY, pr=pr_number,
                head_sha=pr_head, base_sha=base_sha, candidate_sha=candidate,
                candidate_tree=tree, merge_commit=merge_sha, run_id=run_id,
                run_attempt=1, workflow_id=workflow_id, merged_by_id=OWNER_ID,
                merged_at=pr["merged_at"])


def validate_record(raw):
    value = verifier.decode(raw, 65536)
    verifier.require(raw == verifier.canonical(value), "noncanonical machine evidence")
    verifier.require(set(value) == {"schema", "repository", "pr", "head_sha", "base_sha",
                                   "candidate_sha", "candidate_tree", "merge_commit", "run_id",
                                   "run_attempt", "workflow_id", "merged_by_id", "merged_at"}
                     and value["schema"] == SCHEMA and value["repository"] == verifier.REPOSITORY
                     and value["merged_by_id"] == OWNER_ID and value["run_attempt"] == 1
                     and all(type(value[k]) is int and value[k] > 0
                             for k in ("pr", "run_id", "workflow_id"))
                     and all(verifier.full_sha(value[k]) for k in
                             ("head_sha", "base_sha", "candidate_sha", "candidate_tree", "merge_commit")),
                     "invalid machine evidence record")
    _time(value["merged_at"])
    return value


def verify_record(raw, *, api, product, fetch_sources=None):
    """Authenticate persisted evidence live; a canonical JSON file is not proof.

    This deliberately does not claim offline verification after GitHub expires
    the source artifact. Missing or changed evidence fails closed.
    """
    recorded = validate_record(raw)
    actual = build(recorded['pr'], recorded['run_id'], api=api, product=product,
                   fetch_sources=fetch_sources)
    verifier.require(recorded == actual, 'stored evidence differs from live provenance')
    return actual


def publish(pr, run, *, api, product, planning, fetch_sources=None):
    # Verification is repeated on every invocation, including idempotent retries.
    import receipt_writer
    record = build(pr, run, api=api, product=product, fetch_sources=fetch_sources)
    raw = verifier.canonical(record)
    validate_record(raw)
    path = f"evidence/ci-runs/pr-{pr}/run-{run}.json"
    root = Path(planning)
    commit = receipt_writer.commit_file(root, path, raw)
    return receipt_writer.publish(root, path, commit, canonical_validator=validate_record)


def fetch_sources(product, head, base):
    # Inputs have already passed full-SHA validation. No PR-controlled code is run.
    subprocess.run(["git", "-C", str(product), "fetch", "--no-tags", "origin", head, base],
                   check=True, stdout=subprocess.DEVNULL)


def select_run(runs, head, matches):
    """Select by authenticated candidate before considering run conclusion."""
    candidates = [r for r in runs if r.get('head_sha') == head
                  and r.get('event') == 'pull_request'
                  and type(r.get('id')) is int and r['id'] > 0]
    verifier.require(candidates, 'no candidate run available')
    relevant = [r for r in candidates if matches(r)]
    verifier.require(relevant, 'no run matches the merged candidate')
    latest = max(relevant, key=lambda r: r['id'])
    verifier.require(latest.get('run_attempt') == 1 and latest.get('status') == 'completed'
                     and latest.get('conclusion') == 'success',
                     'latest candidate run has not succeeded')
    return latest['id']


def matches_merged_candidate(run, pr, *, api, product):
    """Classify via validated source and producer artifact, never by branch name."""
    head = _sha((pr.get('head') or {}).get('sha'), 'missing PR head')
    run_id = run['id']
    detail = api(f'{PREFIX}/actions/runs/{run_id}')
    verifier.require(detail.get('id') == run_id and detail.get('head_sha') == head
                     and detail.get('event') == 'pull_request'
                     and detail.get('path') == '.github/workflows/ci.yml'
                     and (detail.get('repository') or {}).get('id') == verifier.REPOSITORY_ID,
                     'untrusted candidate run')
    artifacts = _inventory(api, f'{PREFIX}/actions/runs/{run_id}/artifacts', 'artifacts')
    found = [a for a in artifacts if a.get('name') == 'trusted-pr-plan']
    verifier.require(len(found) == 1, 'candidate run plan missing or ambiguous')
    artifact = found[0]
    verifier.require(type(artifact.get('id')) is int and artifact['id'] > 0
                     and artifact.get('expired') is False
                     and (artifact.get('workflow_run') or {}).get('id') == run_id
                     and (artifact.get('workflow_run') or {}).get('head_sha') == head,
                     'untrusted candidate plan')
    raw = api(f"{PREFIX}/actions/artifacts/{artifact['id']}/zip", binary=True)
    verifier.require(type(raw) is bytes and 0 < len(raw) <= 65536, 'invalid candidate archive')
    try:
        with zipfile.ZipFile(io.BytesIO(raw)) as archive:
            entries = archive.infolist()
            verifier.require(len(entries) == 1 and entries[0].filename == 'pr-plan.json'
                             and 0 < entries[0].file_size < 65536, 'invalid candidate archive')
            plan = verifier.decode(archive.read(entries[0]), 65536)
    except (zipfile.BadZipFile, RuntimeError, NotImplementedError) as exc:
        raise verifier.ReceiptError('invalid candidate archive') from exc
    base = _sha(plan.get('target_base_sha'), 'missing candidate base')
    candidate = _sha(plan.get('integration_sha'), 'missing candidate identity')
    verifier.require(plan.get('schema') == 'baseharbor.pr-gate-plan/v1'
                     and plan.get('head_sha') == head, 'candidate plan source mismatch')
    fetch_sources(product, head, base)
    verifier.producer_identity(product, {'head_sha': head, 'base_sha': base})
    candidate_commit = api(f'{PREFIX}/git/commits/{candidate}')
    merge_sha = _sha(pr.get('merge_commit_sha'), 'missing merge commit')
    merge = api(f'{PREFIX}/git/commits/{merge_sha}')
    verifier.require(candidate_commit.get('sha') == candidate and merge.get('sha') == merge_sha,
                     'candidate commit identity mismatch')
    parents = candidate_commit.get('parents')
    verifier.require(isinstance(parents, list) and len(parents) == 2
                     and all(isinstance(p, dict) for p in parents)
                     and [p.get('sha') for p in parents] == [base, head] and base != head,
                     'candidate commit parents invalid')
    merged_parents = merge.get('parents')
    verifier.require(isinstance(merged_parents, list) and len(merged_parents) in (1, 2)
                     and all(isinstance(p, dict) for p in merged_parents),
                     'merge commit parents invalid')
    tree = _sha((candidate_commit.get('tree') or {}).get('sha'), 'missing candidate tree')
    merged_tree = _sha((merge.get('tree') or {}).get('sha'), 'missing merge tree')
    return merge_parents_match(merged_parents, base, head) and tree == merged_tree


def main():
    parser = argparse.ArgumentParser()
    selection = parser.add_mutually_exclusive_group(required=True)
    selection.add_argument('--pr', type=int)
    selection.add_argument('--wakeup-run', type=int)
    parser.add_argument('--product', type=Path, required=True)
    parser.add_argument('--planning', type=Path, required=True)
    args = parser.parse_args()
    pr_number = args.pr if args.pr is not None else wakeup_pr(args.wakeup_run, api=github)
    if pr_number is None:
        print('no merged develop PR for authenticated wakeup')
        return
    pr = github(f"{PREFIX}/pulls/{pr_number}")
    head = _sha((pr.get('head') or {}).get('sha'), 'missing PR head')
    runs = _inventory(github,
                      f"{PREFIX}/actions/workflows/ci.yml/runs?head_sha={head}&event=pull_request",
                      'workflow_runs')
    run = select_run(runs, head, lambda run: matches_merged_candidate(
        run, pr, api=github, product=args.product))
    path, introduction, publication = publish(pr_number, run, api=github,
                                                  product=args.product, planning=args.planning,
                                                  fetch_sources=fetch_sources)
    print(f'publication: {publication}\nevidence: {path}\nintroduction: {introduction}')


if __name__ == '__main__':
    main()
