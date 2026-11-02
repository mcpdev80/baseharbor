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


def closed_run(run_id, product, api=gh):
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
    require(run.get("pull_requests") == [], "run is not missing its PR association")
    pages = api(f"{prefix}/attempts/1/jobs?per_page=100", pages=True)
    jobs = [j for page in pages for j in page.get("jobs", [])]
    pr_pages = api(f"repos/{REPOSITORY}/commits/{head}/pulls?per_page=100", pages=True)
    prs = [p for page in pr_pages for p in page
           if p.get("head", {}).get("sha") == head
           and p.get("head", {}).get("ref") == branch
           and p.get("base", {}).get("repo", {}).get("id") == REPOSITORY_ID]
    require(len(prs) == 1, "missing or ambiguous PR for producer head")
    pr = prs[0]
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
    require("integration/static/public-contracts" not in plan["selected_gates"],
            "public-contracts producer requires receipt handling")
    require(pr.get("state") == "closed" and pr.get("base", {}).get("ref") == "develop"
            and pr.get("base", {}).get("sha") == facts["base_sha"]
            and pr.get("head", {}).get("repo", {}).get("id") == run.get("head_repository", {}).get("id"),
            "closed PR source tuple mismatch")
    subprocess.run(["git", "-C", str(product), "fetch", "--no-tags", "--depth=2", "origin",
                    head, facts["base_sha"], facts["candidate_sha"]], check=True,
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    parents = subprocess.run(["git", "-C", str(product), "rev-list", "--parents", "-n", "1",
                              facts["candidate_sha"]], capture_output=True, check=True, text=True).stdout.split()
    require(len(parents) == 3 and parents[0] == facts["candidate_sha"]
            and set(parents[1:]) == {head, facts["base_sha"]}, "candidate parent mismatch")
    rv.producer_identity(product, {"head_sha": head, "base_sha": facts["base_sha"]})
    return pr["number"]


if __name__ == "__main__":
    try:
        require(len(sys.argv) == 3 and sys.argv[1].isdigit(), "invalid arguments")
        number = closed_run(int(sys.argv[1]), Path(sys.argv[2]))
        print(f"Authenticated closed PR #{number}; no receipt required")
    except (ValueError, KeyError, TypeError, OSError, zipfile.BadZipFile, subprocess.CalledProcessError, rv.ReceiptError):
        print("Unable to authenticate a unique closed PR; failing closed", file=sys.stderr)
        sys.exit(1)
