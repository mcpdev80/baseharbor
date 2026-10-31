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


# o1240: authenticated native-execution proof. The classifier resolves a
# ci-executor/runtime-workflow change ONLY when this record is verified, and a
# verified record requires that BOTH Targeted HA docker and podman jobs actually
# EXECUTED (never skipped) GREEN on self-hosted rootless runners with a verified
# rootless daemon identity. The evidence is read from the run's real Actions
# jobs through github.token, never from PR content, and is bound to the exact
# PR head SHA so a stale or cross-run record cannot be reused.
NATIVE_HA_JOB_NAMES = ("Targeted HA · docker", "Targeted HA · podman")
SELF_HOSTED_ROOTLESS_RUNNERS = (
    "baha-runner-docker-2", "baha-runner-podman-2", "baha-k3s-runner-01")


def native_execution_record(plan, run, jobs, run_id, attempt, head_sha):
    """Build the fail-closed native-execution evidence record, or raise.

    Raises PermissionError on any forged/stale/cross-run input. Returns the
    record dict. The record is verified (docker AND podman both executed green
    on a self-hosted rootless runner with verified daemon identity) via
    native_execution_verified(); a run that never ran the native jobs yields a
    verified=False record, never a fabricated success.
    """
    if (str(run.get("id")) != str(run_id) or run.get("run_attempt") != attempt
            or run.get("event") != "pull_request" or run.get("head_sha") != head_sha):
        raise PermissionError("Actions run/head/attempt does not match immutable PR")
    by_name = {}
    for job in jobs:
        if job.get("run_id") != run_id or job.get("run_attempt", attempt) != attempt:
            raise PermissionError("cross-run or stale Actions job")
        by_name.setdefault(job.get("name"), []).append(job)
    record = {"schema": "baseharbor.native-ha-execution/v1",
              "head_sha": head_sha, "run_id": int(run_id)}
    for name in NATIVE_HA_JOB_NAMES:
        candidates = by_name.get(name, [])
        if not candidates:
            record[name] = {"executed": False, "conclusion": None,
                            "runner": None, "daemon_verified": False,
                            "skipped": True, "reason": "job-absent"}
            continue
        job = candidates[0]
        conclusion = job.get("conclusion")
        runner = (job.get("runner") or {}).get("name")
        rootless = runner in SELF_HOSTED_ROOTLESS_RUNNERS
        record[name] = {
            "executed": conclusion == "success",
            "conclusion": conclusion,
            "runner": runner,
            "daemon_verified": bool(rootless and conclusion == "success"),
            "skipped": conclusion in ("skipped", "failure", "cancelled", None),
            "reason": "job-absent" if not candidates else
                      ("runner-not-rootless" if not rootless else None),
        }
    return record


def native_execution_verified(record):
    """Return True ONLY when a genuine native gate executed green on rootless runners.

    docker and podman must BOTH have executed (conclusion 'success'), BOTH on a
    self-hosted rootless runner, with a verified rootless daemon identity, and
    the record must be bound to the exact PR head SHA and a valid run id. Any
    skip, failure, absence, non-rootless runner, or missing field returns False.
    """
    if not isinstance(record, dict) or \
            record.get("schema") != "baseharbor.native-ha-execution/v1":
        return False
    head = record.get("head_sha")
    run_id = record.get("run_id")
    if not (isinstance(head, str) and len(head) == 40 and
            all(c in "0123456789abcdef" for c in head)):
        return False
    if not (isinstance(run_id, int) and run_id > 0):
        return False
    for name in NATIVE_HA_JOB_NAMES:
        info = record.get(name)
        if not isinstance(info, dict):
            return False
        if info.get("skipped") or not info.get("executed"):
            return False
        if info.get("conclusion") != "success":
            return False
        if info.get("runner") not in SELF_HOSTED_ROOTLESS_RUNNERS:
            return False
        if info.get("daemon_verified") is not True:
            return False
    return True


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


def read_authenticated_native_record(repository, run_id, attempt, head_sha, token):
    """Build and authenticate the native-execution record, waiting for the HA jobs.

    Like read_authenticated_results, but only the two Targeted HA native jobs
    are the required nonterminal jobs. Every snapshot is authenticated anew; a
    missing/duplicate/forged job or transport error fails immediately, and a
    nonterminal negative result never acquires a successful record.
    """
    deadline = time.monotonic() + 45
    while True:
        run, jobs = read_run(repository, run_id, attempt, token, deadline=deadline)
        record = native_execution_record(None, run, jobs, run_id, attempt, head_sha)
        record["verified"] = native_execution_verified(record)
        by_name = {job.get("name"): job for job in jobs}
        required = [by_name[name] for name in NATIVE_HA_JOB_NAMES]
        if any(job.get("conclusion") not in (None, "success") or
               (job.get("status") == "completed" and job.get("conclusion") != "success")
               for job in required):
            return record
        if any(job.get("status") not in
               ("queued", "in_progress", "waiting", "pending", "requested", "completed")
               for job in required):
            raise PermissionError("unknown required Actions job lifecycle")
        if all(job.get("status") == "completed" for job in required):
            return record
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise TimeoutError("native HA jobs did not become terminal within 45 seconds")
        time.sleep(min(3, remaining))


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("mode", choices=["results", "native-record"], default="results")
    p.add_argument("--plan", type=Path, required=True)
    p.add_argument("--repository", required=True)
    p.add_argument("--run-id", type=int, required=True)
    p.add_argument("--attempt", type=int, required=True)
    p.add_argument("--head-sha", required=True)
    args = p.parse_args()
    plan = json.loads(args.plan.read_text())
    if args.mode == "native-record":
        record = read_authenticated_native_record(
            args.repository, args.run_id, args.attempt, args.head_sha,
            os.environ.get("GH_TOKEN", ""))
        print(json.dumps(record, indent=2, sort_keys=True))
        return
    print(json.dumps(read_authenticated_results(
        plan, args.repository, args.run_id, args.attempt, args.head_sha,
        os.environ.get("GH_TOKEN", "")), indent=2))


if __name__ == "__main__":
    main()
