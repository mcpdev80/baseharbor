#!/usr/bin/env python3
"""Read actual GitHub Actions job conclusions; never trust PR result JSON.

This adapter does not authenticate native gate evidence: the existing evidence
verifier must supply those results separately before final acceptance.
"""
import argparse
import json
import os
import time
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
        required.extend(("PR · Exact Candidate", "PR · Runner Interface"))
        # The trusted authoritative inventory owns consumer dependency edges.
        # No private Console/Connector prerequisite for Core+Demo backup.
        dependencies = plan.get("required_consumer_dependencies")
        if not isinstance(dependencies, list):
            raise PermissionError("unqualified selected-gate dependencies")
        if not set(dependencies) <= {"core", "demo", "console", "connector"}:
            raise PermissionError("unknown consumer dependency")
        if set(dependencies) & {"console", "connector"}:
            required.append("PR · Consumer Pins")
        elif "demo" in dependencies:
            required.append("PR · Demo Pin")
    if "integration/static/public-contracts" in plan.get("selected_gates", []):
        required.append("PR · Public Contracts")
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
    preconditions = [{"name": name, "conclusion": by_name[name][0].get("conclusion")}
                     for name in required]
    return {"schema": "baseharbor.pr-check-results/v1",
            "required_job_conclusions": preconditions,
            **{k: plan.get(k) for k in ("target_base_sha", "head_sha",
                                        "merge_base_sha", "integration_sha")},
            "results": results}  # NO GATE PASS without verified runtime evidence


def api_json(url, token, timeout=20):
    if not token:
        raise PermissionError("read-only GitHub Actions token required")
    req = urllib.request.Request(url, headers={
        "Authorization": "Bearer " + token, "Accept": "application/vnd.github+json"})
    with urllib.request.urlopen(req, timeout=timeout) as response:
        return json.load(response)


def read_run(repository, run_id, attempt, token, deadline=None):
    if repository != "mcpdev80/baseharbor" or not str(run_id).isdecimal():
        raise PermissionError("unexpected repository or run")
    base = "https://api.github.com/repos/" + repository + "/actions/runs/" + str(run_id)
    def read_api(url):
        timeout = 20 if deadline is None else min(20, deadline - time.monotonic())
        if timeout <= 0:
            raise TimeoutError("required Actions jobs did not become terminal within 45 seconds")
        return api_json(url, token, timeout=timeout)
    run = read_api(base)
    jobs = []
    for page in range(1, 11):
        value = read_api(base + "/attempts/" + str(attempt) +
                         "/jobs?per_page=100&page=" + str(page))
        if not isinstance(value.get("jobs"), list):
            raise ValueError("incomplete Actions jobs response")
        jobs.extend(value["jobs"])
        if len(value["jobs"]) < 100:
            break
    else:
        raise ValueError("too many Actions jobs to prove complete pagination")
    return run, jobs


def read_authenticated_results(plan, repository, run_id, attempt, head_sha, token):
    # Job lifecycle publication can lag the dependency scheduler. Only genuine
    # required nonterminal jobs may wait; every snapshot is authenticated anew.
    # Missing/duplicate/forged jobs and transport errors still fail immediately.
    deadline = time.monotonic() + 45
    while True:
        run, jobs = read_run(repository, run_id, attempt, token, deadline=deadline)
        report = authenticated_results(plan, run, jobs, run_id, attempt, head_sha)
        if time.monotonic() > deadline:
            raise TimeoutError("required Actions jobs did not become terminal within 45 seconds")
        names = ["PR · Validation · " + row["id"] for row in report["results"]]
        names += [row["name"] for row in report["required_job_conclusions"]]
        by_name = {job.get("name"): job for job in jobs}
        required = [by_name[name] for name in names]
        # Terminal negative results never wait or acquire a successful receipt.
        if any(job.get("conclusion") not in (None, "success") or
               (job.get("status") == "completed" and job.get("conclusion") != "success")
               for job in required):
            return report
        if any(job.get("status") not in
               ("queued", "in_progress", "waiting", "pending", "requested", "completed")
               for job in required):
            raise PermissionError("unknown required Actions job lifecycle")
        if all(job.get("status") == "completed" for job in required):
            return report
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise TimeoutError("required Actions jobs did not become terminal within 45 seconds")
        time.sleep(min(3, remaining))


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--plan", type=Path, required=True)
    p.add_argument("--repository", required=True)
    p.add_argument("--run-id", type=int, required=True)
    p.add_argument("--attempt", type=int, required=True)
    p.add_argument("--head-sha", required=True)
    args = p.parse_args()
    plan = json.loads(args.plan.read_text())
    print(json.dumps(read_authenticated_results(
        plan, args.repository, args.run_id, args.attempt, args.head_sha,
        os.environ.get("GH_TOKEN", "")), indent=2))


if __name__ == "__main__":
    main()
