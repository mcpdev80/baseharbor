#!/usr/bin/env python3
"""Read actual GitHub Actions job conclusions; never trust PR result JSON.

This adapter does not authenticate native gate evidence: the existing evidence
verifier must supply those results separately before final acceptance.
"""
import argparse
import json
import os
from pathlib import Path
import urllib.request


def authenticated_results(plan, run, jobs, run_id, attempt, head_sha):
    if (str(run.get("id")) != str(run_id) or run.get("run_attempt") != attempt
            or run.get("event") != "pull_request" or run.get("head_sha") != head_sha
            or plan.get("head_sha") != head_sha):
        raise PermissionError("Actions run/head/attempt does not match immutable PR plan")
    expected = [c["id"] for c in plan["required_validation_checks"]]
    names = ["PR · Validation · " + name for name in expected]
    required = ["PR · Focused Python", "Workflow policy"]
    selected_gates = plan.get("selected_gates", [])
    if any(isinstance(gate, str) and (
            gate.startswith(("atomic/docker/", "atomic/podman/",
                             "integration/docker/", "integration/podman/")))
           for gate in selected_gates):
        required.append("PR · Exact Candidate")
    by_name = {}
    for job in jobs:
        if job.get("run_id") != run_id or job.get("run_attempt", attempt) != attempt:
            raise PermissionError("cross-run or stale Actions job")
        by_name.setdefault(job.get("name"), []).append(job)
    if any(len(by_name.get(name, [])) != 1 for name in names + required):
        raise PermissionError("missing or duplicate required Actions job")
    results = []
    for name, identifier in zip(names, expected):
        job = by_name[name][0]
        conclusion = job.get("conclusion")
        results.append({"kind": "validation", "id": identifier,
                        "conclusion": "success" if conclusion == "success" else "blocked"})
    for name in required:
        if by_name[name][0].get("conclusion") != "success":
            raise PermissionError("required workflow/focused job not successful: " + name)
    return {"schema": "baseharbor.pr-check-results/v1",
            **{k: plan.get(k) for k in ("target_base_sha", "head_sha",
                                        "merge_base_sha", "integration_sha")},
            "results": results}  # NO GATE PASS without verified runtime evidence


def api_json(url, token):
    if not token:
        raise PermissionError("read-only GitHub Actions token required")
    req = urllib.request.Request(url, headers={
        "Authorization": "Bearer " + token, "Accept": "application/vnd.github+json"})
    with urllib.request.urlopen(req, timeout=20) as response:
        return json.load(response)


def read_run(repository, run_id, attempt, token):
    if repository != "mcpdev80/baseharbor" or not str(run_id).isdecimal():
        raise PermissionError("unexpected repository or run")
    base = "https://api.github.com/repos/" + repository + "/actions/runs/" + str(run_id)
    run = api_json(base, token)
    jobs = []
    for page in range(1, 11):
        value = api_json(base + "/attempts/" + str(attempt) +
                         "/jobs?per_page=100&page=" + str(page), token)
        if not isinstance(value.get("jobs"), list):
            raise ValueError("incomplete Actions jobs response")
        jobs.extend(value["jobs"])
        if len(value["jobs"]) < 100:
            break
    else:
        raise ValueError("too many Actions jobs to prove complete pagination")
    return run, jobs


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--plan", type=Path, required=True)
    p.add_argument("--repository", required=True)
    p.add_argument("--run-id", type=int, required=True)
    p.add_argument("--attempt", type=int, required=True)
    p.add_argument("--head-sha", required=True)
    args = p.parse_args()
    plan = json.loads(args.plan.read_text())
    run, jobs = read_run(args.repository, args.run_id, args.attempt,
                         os.environ.get("GH_TOKEN", ""))
    print(json.dumps(authenticated_results(plan, run, jobs, args.run_id,
                                           args.attempt, args.head_sha), indent=2))


if __name__ == "__main__":
    main()
