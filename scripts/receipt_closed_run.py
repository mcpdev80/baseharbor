#!/usr/bin/env python3
"""Recognize an authenticated completed run for one already-closed PR."""
import io
import json
import subprocess
import sys
import zipfile
from pathlib import Path

import receipt_verifier as rv

REPOSITORY = "mcpdev80/baseharbor"
REPOSITORY_ID = 1360667669


def require(condition, message):
    if not condition:
        raise ValueError(message)


def gh(path, *, pages=False, binary=False):
    command = ["gh", "api"]
    if pages:
        command += ["--paginate", "--slurp"]
    result = subprocess.run(command + [path], capture_output=True, check=True)
    if binary:
        require(0 < len(result.stdout) <= 65536, "invalid artifact size")
        return result.stdout
    return json.loads(result.stdout)


def closed_run(run_id, product, api=gh, *, planning=None, classify_only=False):
    prefix = f"repos/{REPOSITORY}/actions/runs/{run_id}"
    run = api(prefix)
    head = run.get("head_sha")
    branch = run.get("head_branch")
    require(run.get("id") == run_id and run.get("event") == "pull_request"
            and run.get("status") == "completed" and run.get("conclusion") == "success"
            and run.get("run_attempt") == 1 and run.get("path") == ".github/workflows/ci.yml"
            and run.get("repository", {}).get("id") == REPOSITORY_ID
            and isinstance(head, str) and rv.full_sha(head)
            and isinstance(branch, str) and branch, "invalid producer run")
    associations = run.get("pull_requests")
    require(isinstance(associations, list) and len(associations) <= 1,
            "run PR association is invalid or ambiguous")
    pages = api(f"{prefix}/attempts/1/jobs?per_page=100", pages=True)
    jobs = [j for page in pages for j in page.get("jobs", [])]
    pr_pages = api(f"repos/{REPOSITORY}/commits/{head}/pulls?per_page=100", pages=True)
    prs = [p for page in pr_pages for p in page
           if p.get("head", {}).get("sha") == head
           and p.get("head", {}).get("ref") == branch
           and p.get("base", {}).get("repo", {}).get("id") == REPOSITORY_ID]
    require(len(prs) == 1, "missing or ambiguous PR for producer head")
    pr = prs[0]
    if associations:
        association = associations[0]
        require(isinstance(association, dict) and association.get("number") == pr.get("number")
                and association.get("head", {}).get("sha") == head
                and association.get("base", {}).get("sha") == pr.get("base", {}).get("sha")
                and association.get("base", {}).get("ref") == pr.get("base", {}).get("ref"),
                "run PR association mismatch")
    require(pr.get("state") == "closed"
            and pr.get("head", {}).get("repo", {}).get("id") == run.get("head_repository", {}).get("id"),
            "closed PR identity mismatch")
    if pr.get("base", {}).get("ref") == "main":
        # Main PRs do not run the develop-only planner. Accept that case only
        # with authenticated job outcomes, no plan, and an unchanged producer.
        for name, conclusion in (("Workflow policy", "success"),
                                 ("PR · Trusted Plan", "skipped"),
                                 ("PR Selected Checks", "skipped")):
            found = [j for j in jobs if j.get("name") == name]
            require(len(found) == 1 and found[0].get("run_id") == run_id
                    and found[0].get("run_attempt") == 1 and found[0].get("head_sha") == head
                    and found[0].get("status") == "completed"
                    and found[0].get("conclusion") == conclusion, "invalid main PR jobs")
        inventories = api(f"{prefix}/artifacts?per_page=100", pages=True)
        require(not any(a.get("name") == "trusted-pr-plan"
                        for page in inventories for a in page.get("artifacts", [])),
                "main PR unexpectedly has a trusted plan")
        base = pr.get("base", {}).get("sha")
        require(isinstance(base, str) and rv.full_sha(base), "invalid main PR base")
        subprocess.run(["git", "-C", str(product), "fetch", "--no-tags", "--depth=1",
                        "origin", head, base], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        rv.producer_identity(product, {"head_sha": head, "base_sha": base})
        print('decision: nonpilot')
        return pr["number"]
    for name in ("PR · Trusted Plan", "PR Selected Checks"):
        found = [j for j in jobs if j.get("name") == name]
        require(len(found) == 1 and found[0].get("run_id") == run_id
                and found[0].get("run_attempt") == 1 and found[0].get("head_sha") == head
                and found[0].get("status") == "completed"
                and found[0].get("conclusion") == "success", "required producer job missing")
    inventories = api(f"{prefix}/artifacts?per_page=100", pages=True)
    artifacts = [a for page in inventories for a in page.get("artifacts", [])
                 if a.get("name") == "trusted-pr-plan"]
    require(len(artifacts) == 1 and not artifacts[0].get("expired")
            and artifacts[0].get("workflow_run", {}).get("id") == run_id
            and artifacts[0].get("workflow_run", {}).get("head_sha") == head
            and type(artifacts[0].get("id")) is int, "invalid producer artifact")
    data = api(f"repos/{REPOSITORY}/actions/artifacts/{artifacts[0]['id']}/zip", binary=True)
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        entries = archive.infolist()
        require(len(entries) == 1 and entries[0].filename == "pr-plan.json"
                and 0 < entries[0].file_size <= 65536, "invalid producer archive")
        plan = rv.decode(archive.read(entries[0]), 65536)
    facts = rv.extract_facts(plan, artifact_size=len(data), run_id=run_id,
                             attempt=1, workflow_id=run.get("workflow_id"))
    require(facts["head_sha"] == head and plan.get("coverage_status") == "resolved"
            and isinstance(plan.get("selected_gates"), list)
            and all(isinstance(x, str) for x in plan["selected_gates"]), "invalid producer plan")
    pilot = "integration/static/public-contracts" in plan["selected_gates"]
    require(not pilot or planning is not None or classify_only,
            "public-contracts producer requires receipt handling")
    require(pr.get("state") == "closed" and pr.get("base", {}).get("ref") == "develop"
            and (pilot or pr.get("base", {}).get("sha") == facts["base_sha"])
            and pr.get("head", {}).get("repo", {}).get("id") == run.get("head_repository", {}).get("id"),
            "closed PR source tuple mismatch")
    sources = [head, facts["base_sha"], facts["candidate_sha"]]
    if pilot:
        merge = pr.get("merge_commit_sha") or ""
        require(rv.full_sha(merge), "missing merged commit")
        sources.append(merge)
    subprocess.run(["git", "-C", str(product), "fetch", "--no-tags", "--depth=2", "origin",
                    *sources], check=True,
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    parents = subprocess.run(["git", "-C", str(product), "rev-list", "--parents", "-n", "1",
                              facts["candidate_sha"]], capture_output=True, check=True, text=True).stdout.split()
    require(len(parents) == 3 and parents[0] == facts["candidate_sha"]
            and set(parents[1:]) == {head, facts["base_sha"]}, "candidate parent mismatch")
    rv.producer_identity(product, {"head_sha": head, "base_sha": facts["base_sha"]})
    if pilot:
        writer_plan = dict(head_sha=head, base_sha=facts['base_sha'],
                           candidate_sha=facts['candidate_sha'], workflow_id=run['workflow_id'])
        if classify_only:
            rv.merge_evidence(pr['number'], run_id, writer_plan, product=product, api=api)
        else:
            from receipt_writer import write_merge
            path, _, publication = write_merge(Path(planning), pr['number'], run_id, writer_plan,
                                               product=product, api=api)
            rv.retained_merge(Path(planning), publication, path, facts['candidate_sha'],
                              product=product, api=api)
            print('decision: receipt')
            print(f"receipt: {publication}:{path}")
            print(f"candidate: {facts['candidate_sha']}")
    else:
        print('decision: nonpilot')
    return pr["number"]


def push_run(push_id, product, planning, api=gh):
    """Wake after develop push when the PR run finished before its merge."""
    push = api(f'repos/{REPOSITORY}/actions/runs/{push_id}')
    merge = push.get('head_sha') or ''
    require(push.get('id') == push_id and push.get('event') == 'push'
            and push.get('status') == 'completed' and push.get('head_branch') == 'develop'
            and push.get('path') == '.github/workflows/ci.yml'
            and push.get('repository', {}).get('id') == REPOSITORY_ID
            and rv.full_sha(merge), 'invalid develop push wakeup')
    return reconcile_commit(merge, product, planning, api=api)


def reconcile_commit(merge, product, planning, *, api=gh, strict=False):
    """Resolve one immutable develop commit; strict mode cannot drop a commit."""
    require(rv.full_sha(merge), 'invalid develop commit')
    pages = api(f'repos/{REPOSITORY}/commits/{merge}/pulls?per_page=100', pages=True)
    require(isinstance(pages, list) and all(isinstance(p, list) for p in pages),
            'invalid push PR inventory')
    prs = [p for page in pages for p in page if p.get('state') == 'closed'
           and p.get('merge_commit_sha') == merge and p.get('merged_at')
           and p.get('base', {}).get('ref') == 'develop'
           and p.get('base', {}).get('repo', {}).get('id') == REPOSITORY_ID]
    require(len(prs) <= 1, 'ambiguous push PR')
    if not prs:
        require(not strict, 'develop commit has no authenticated merged PR')
        print('decision: no-pr')
        return None
    if strict:
        detail = api(f"repos/{REPOSITORY}/pulls/{prs[0]['number']}")
        merger = detail.get('merged_by') or {}
        base = (detail.get('base') or {}).get('sha') or ''
        source = (detail.get('head') or {}).get('sha') or ''
        require(detail.get('number') == prs[0]['number'] and detail.get('merged') is True
                and detail.get('merge_commit_sha') == merge and merger.get('id') == rv.OWNER_ID
                and merger.get('login') == rv.OWNER and rv.full_sha(base) and rv.full_sha(source),
                'develop commit is not an authenticated Owner merge')
        parents = rv.git(product, 'show', '-s', '--format=%P', merge).decode().split()
        require(parents in ([base], [base, source]), 'develop merge parents do not match PR')
    head = prs[0].get('head', {}).get('sha') or ''
    require(rv.full_sha(head), 'missing merged PR head')
    pages = api(f'repos/{REPOSITORY}/actions/workflows/ci.yml/runs?head_sha={head}'
                '&event=pull_request&per_page=100', pages=True)
    require(isinstance(pages, list) and all(isinstance(p, dict) for p in pages),
            'invalid PR run inventory')
    runs = [r for page in pages for r in page.get('workflow_runs', [])
            if r.get('head_sha') == head and r.get('event') == 'pull_request'
            and r.get('head_branch') == prs[0].get('head', {}).get('ref')
            and r.get('head_repository', {}).get('id') == prs[0].get('head', {}).get('repo', {}).get('id')
            and type(r.get('id')) is int and r['id'] > 0]
    require(runs, 'merged PR has no candidate run')
    expected_base = prs[0].get('base', {}).get('sha') or ''
    require(rv.full_sha(expected_base), 'missing merged PR base')
    for latest in sorted(runs, key=lambda r: r['id'], reverse=True):
        pending = latest.get('status') != 'completed'
        inventories = api(f"repos/{REPOSITORY}/actions/runs/{latest['id']}/artifacts?per_page=100", pages=True)
        require(isinstance(inventories, list) and all(isinstance(p, dict) for p in inventories),
                'invalid candidate artifact inventory')
        artifacts = [a for page in inventories for a in page.get('artifacts', [])
                     if a.get('name') == 'trusted-pr-plan']
        if pending and not artifacts:
            require(not strict, 'candidate run still pending without classifiable plan')
            print('Candidate run still pending; completion event will retry')
            print('decision: pending')
            return None
        require(len(artifacts) == 1 and artifacts[0].get('expired') is False
                and artifacts[0].get('workflow_run', {}).get('id') == latest['id']
                and artifacts[0].get('workflow_run', {}).get('head_sha') == head
                and type(artifacts[0].get('id')) is int, 'candidate artifact missing or untrusted')
        raw = api(f"repos/{REPOSITORY}/actions/artifacts/{artifacts[0]['id']}/zip", binary=True)
        require(type(raw) is bytes and 0 < len(raw) <= 65536, 'invalid candidate archive size')
        with zipfile.ZipFile(io.BytesIO(raw)) as archive:
            entries = archive.infolist()
            require(len(entries) == 1 and entries[0].filename == 'pr-plan.json'
                    and 0 < entries[0].file_size < 65536, 'invalid candidate archive')
            plan = rv.decode(archive.read(entries[0]), 65536)
        base = plan.get('target_base_sha') or ''
        require(plan.get('schema') == 'baseharbor.pr-gate-plan/v1'
                and plan.get('head_sha') == head and rv.full_sha(base), 'invalid candidate plan')
        subprocess.run(['git', '-C', str(product), 'fetch', '--no-tags', '--depth=2', 'origin',
                        head, base], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        rv.producer_identity(product, {'head_sha': head, 'base_sha': base})
        if base != expected_base:
            continue
        if pending:
            require(not strict, 'matching candidate run still pending')
            print('Candidate run still pending; completion event will retry')
            print('decision: pending')
            return None
        require(latest.get('conclusion') == 'success' and latest.get('run_attempt') == 1,
                'latest matching candidate run did not succeed')
        return closed_run(latest['id'], product, api, planning=planning)
    raise ValueError('merged candidate has no matching run')


def reconcile_history(initial, product, planning, *, api=gh):
    """Replay develop commits, advancing only after downstream verification."""
    require(rv.full_sha(initial), 'invalid reconciliation anchor')
    product, planning = Path(product), Path(planning)
    rv.git(planning, 'fetch', 'origin', 'main')
    rv.git(planning, 'merge', '--ff-only', 'origin/main')
    path = 'evidence/receipts/merge-reconciliation-cursor.json'
    cursor_file = planning / path
    previous = cursor_file.read_bytes() if cursor_file.exists() else None
    if previous is None:
        cursor = initial
    else:
        state = rv.decode(previous, 4096)
        require(previous == rv.canonical(state) and set(state) == {'schema', 'commit'}
                and state['schema'] == 'baseharbor.merge-cursor/v1'
                and rv.full_sha(state['commit']), 'invalid reconciliation cursor')
        cursor = state['commit']
    rv.git(product, 'fetch', '--no-tags', 'origin',
           '+refs/heads/develop:refs/remotes/origin/develop')
    tip = rv.git(product, 'rev-parse', 'refs/remotes/origin/develop').decode().strip()
    history = rv.git(product, 'rev-list', '--first-parent', tip).decode().splitlines()
    require(rv.full_sha(tip) and initial in history and cursor in history
            and history.index(cursor) <= history.index(initial), 'cursor is not on protected develop history')
    for commit in reversed(history[:history.index(cursor)]):
        parents = rv.git(product, 'show', '-s', '--format=%P', commit).decode().split()
        require(len(parents) in (1, 2), 'unsupported develop commit parent count')
        # Pilot publication calls retained_merge synchronously. Any failure
        # propagates before the cursor is written.
        reconcile_commit(commit, product, planning, api=api, strict=True)
    if cursor == tip:
        return tip
    current = cursor_file.read_bytes() if cursor_file.exists() else None
    require(current == previous, 'reconciliation cursor changed concurrently')
    from receipt_writer import _push_authed, _scrub_secret, commit_file
    raw = rv.canonical({'schema': 'baseharbor.merge-cursor/v1', 'commit': tip})
    commit_file(planning, path, raw, expected=previous)
    rc, _, stderr = _push_authed(planning, ['push', 'origin', 'HEAD:refs/heads/main'])
    require(rc == 0, 'cursor push failed: ' + _scrub_secret(stderr.decode(errors='replace')))
    return tip


if __name__ == "__main__":
    try:
        push = len(sys.argv) == 5 and sys.argv[1] == '--push'
        classify = len(sys.argv) == 4 and sys.argv[1] == '--classify'
        reconcile = len(sys.argv) == 5 and sys.argv[1] == '--reconcile'
        args = sys.argv[2:] if push or classify or reconcile else sys.argv[1:]
        if reconcile:
            print('reconciled through: ' + reconcile_history(args[0], Path(args[1]), Path(args[2])))
            sys.exit(0)
        require(len(args) in (2, 3) and args[0].isdigit() and (not push or len(args) == 3),
                "invalid arguments")
        planning = Path(args[2]) if len(args) == 3 else None
        number = (push_run(int(args[0]), Path(args[1]), planning) if push
                  else closed_run(int(args[0]), Path(args[1]), planning=planning, classify_only=classify))
        if number is not None:
            suffix = '; classification only' if classify else ('; no receipt required' if planning is None else '')
            print(f"Authenticated closed PR #{number}" + suffix)
    except (ValueError, KeyError, TypeError, OSError, zipfile.BadZipFile, subprocess.CalledProcessError, rv.ReceiptError):
        print("Unable to authenticate a unique closed PR; failing closed", file=sys.stderr)
        sys.exit(1)
