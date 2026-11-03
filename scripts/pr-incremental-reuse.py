#!/usr/bin/env python3
"""Conservative incremental PR check selection; unavailable proof means full CI.

The normal trusted PR planner remains authoritative for the required check set.
Only exact successful jobs from a previous run of the same PR may be reused.
The previous and current synthetic merge commits must share an identical base,
and the previous PR head must be an ancestor of the new head.
"""
import argparse
from datetime import datetime
import io
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
import zipfile

SHA = re.compile(r"^[0-9a-f]{40}$")
RUNTIME = re.compile(r"^PR · Selected Runtime \((atomic/(?:docker|podman)/[^,]+),")
GLOBAL_TESTS = {"go-unit-tests", "go-vet"}
GLOBAL_BUILDS = {"go-build"}
DOC_CHECKS = {"documentation-audit", "mkdocs-en-strict", "mkdocs-de-strict",
              "pages-candidate-build"}
PYTHON_CHECKS = {"pr-planner-tests", "ci-executor-tests", "receipt-execution-tests",
                 "ci-policy", "ci-actionlint"}
GO_CHECK_TARGETS = {
    "source-contracts": ("./internal/application/...", "./contracts/..."),
    "deployment-package-tests": ("./internal/deployment/...",),
    "authority-package-tests": ("./internal/coreinstallation/...", "./internal/installauthority/...",
                                "./internal/stableid/...", "./internal/serviceaccess/..."),
    "delivery-package-tests": ("./internal/delivery/...",),
    "broker-package-tests": ("./internal/runtimebroker/...",),
    "openbao-package-tests": ("./internal/secretstore/...",),
    "cli-package-tests": ("./cmd/baha",),
    "cli-guided-tests": ("./cmd/baha",),
    "cli-build": ("./cmd/baha",),
    "cli-vet": ("./cmd/baha",),
}
# Check families executed using narrowly filtered cmd/baha tests.
CLI_TEST_CHECKS = {"cli-package-tests", "cli-guided-tests", "cli-vet"}
SENSITIVE_PREFIXES = ("scripts/", ".github/", "deploy/", "testdata/", "spec/", "contracts/")
SENSITIVE_EXACT = {"go.mod", "go.sum", "Makefile"}
DOC_PREFIXES = ("docs/", "docs/de/")
DOC_EXACT = {"mkdocs.yml", "mkdocs.de.yml", "requirements-docs.txt", "README.md",
             "CONTRIBUTING.md"}


def git(*args):
    return subprocess.check_output(("git", *args), text=True, stderr=subprocess.DEVNULL).strip()


def request_json(repo, path, token):
    req = urllib.request.Request("https://api.github.com/repos/" + repo + "/" + path,
                                 headers={"Authorization": "Bearer " + token,
                                          "Accept": "application/vnd.github+json",
                                          "X-GitHub-Api-Version": "2022-11-28"})
    with urllib.request.urlopen(req, timeout=12) as response:
        return json.load(response)


def artifact_zip(repo, artifact_id, token):
    """Follow GitHub's authenticated 302 to a short-lived signed blob WITHOUT the token."""
    url = "https://api.github.com/repos/" + repo + "/actions/artifacts/" + str(artifact_id) + "/zip"
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, req, fp, code, msg, headers, newurl):
            return None
    opener = urllib.request.build_opener(NoRedirect)
    req = urllib.request.Request(url, headers={
        "Authorization": "Bearer " + token, "Accept": "application/vnd.github+json"})
    try:
        opener.open(req, timeout=12)
        raise ValueError("artifact endpoint did not redirect")
    except urllib.error.HTTPError as err:
        if err.code not in (301, 302, 303, 307):
            raise
        target = err.headers.get("Location", "")
    parts = urllib.parse.urlparse(target)
    if parts.scheme != "https" or not parts.hostname or parts.hostname == "api.github.com":
        raise ValueError("unsafe artifact redirect")
    with urllib.request.urlopen(target, timeout=20) as response:
        data = response.read(1024 * 1024 + 1)
    if len(data) > 1024 * 1024:
        raise ValueError("oversized demo proof")
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        members = [name for name in archive.namelist() if name.endswith("/verified-demo.sha")
                   or name == "verified-demo.sha"]
        if len(members) != 1:
            raise ValueError("missing unique demo revision in prior proof")
        value = archive.read(members[0]).decode("ascii").strip()
        if not SHA.fullmatch(value):
            raise ValueError("invalid demo revision")
        return value


def prior_demo(repo, run_id, token):
    data = request_json(repo, "actions/runs/" + str(run_id) + "/artifacts?per_page=100", token)
    matches = [row for row in data.get("artifacts", [])
               if row.get("name") == "verified-pr-demo-pin" and not row.get("expired")]
    if len(matches) != 1:
        return None
    return artifact_zip(repo, matches[0]["id"], token)


def completed_previous(repo, workflow_id, branch, pr_number, run_id, token):
    path = "actions/workflows/" + str(workflow_id) + "/runs?event=pull_request&per_page=100&branch=" + urllib.parse.quote(branch, safe="")
    runs = request_json(repo, path, token).get("workflow_runs", [])
    for run in runs:
        if run.get("id") == run_id or run.get("status") != "completed":
            continue
        if run.get("conclusion") not in ("failure", "success"):
            continue
        if run.get("event") != "pull_request" or run.get("workflow_id") != workflow_id:
            continue
        if not any(row.get("number") == pr_number for row in run.get("pull_requests", [])):
            continue
        if not SHA.fullmatch(run.get("head_sha") or ""):
            continue
        return run
    return None


def within_24_hours(previous, current):
    """Compare authenticated GitHub server timestamps, never local wall clocks."""
    try:
        now = datetime.fromisoformat(current["created_at"].replace("Z", "+00:00"))
        before = datetime.fromisoformat(previous["created_at"].replace("Z", "+00:00"))
        if now.tzinfo is None or before.tzinfo is None:
            return False
        elapsed = (now - before).total_seconds()
        return 0 <= elapsed <= 24 * 60 * 60
    except (TypeError, ValueError, AttributeError, KeyError):
        return False


def qualified_ancestry(previous_merge, current_merge, current_head):
    if not all(SHA.fullmatch(x) for x in (previous_merge, current_merge, current_head)):
        return None
    # Never rely on the GitHub runs API's mutable pull_requests.head.sha field.
    subprocess.run(("git", "fetch", "--no-tags", "origin", previous_merge),
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                   timeout=40)
    old = git("rev-list", "--parents", "-n", "1", previous_merge).split()
    new = git("rev-list", "--parents", "-n", "1", current_merge).split()
    if len(old) != 3 or len(new) != 3 or old[1] != new[1] or new[2] != current_head:
        return None
    if subprocess.run(("git", "merge-base", "--is-ancestor", old[2], current_head),
                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode:
        return None
    diff = subprocess.check_output(("git", "diff", "--name-only", "-z",
                                    old[2], current_head), timeout=20)
    paths = [name.decode("utf-8") for name in diff.split(b"\0") if name]
    return paths


def all_jobs(repo, run_id, token):
    jobs = []
    for page in range(1, 6):
        batch = request_json(repo, "actions/runs/" + str(run_id) +
                             "/jobs?per_page=100&page=" + str(page), token)
        jobs.extend(batch.get("jobs", []))
        if len(batch.get("jobs", [])) < 100:
            break
    else:
        raise ValueError("previous run exceeded bounded job inventory")
    if not jobs or len({job.get("id") for job in jobs}) != len(jobs):
        raise ValueError("invalid previous job inventory")
    return jobs


def path_classification(paths):
    """Only known Go and documentation changes can reuse old green results."""
    if any(not p or p.startswith("/") or "/../" in "/" + p or
           p in SENSITIVE_EXACT or p.startswith(SENSITIVE_PREFIXES) for p in paths):
        return None
    for path in paths:
        if path.endswith(".go") or path.startswith(DOC_PREFIXES) or path in DOC_EXACT:
            continue
        return None
    return True


def go_dependency_dirs(targets):
    try:
        out = subprocess.check_output(
            ("go", "list", "-deps", "-test", "-f", "{{.Dir}}", *targets),
            text=True, stderr=subprocess.DEVNULL, timeout=100)
    except (OSError, subprocess.CalledProcessError, subprocess.TimeoutExpired):
        return None
    dirs = set()
    root = Path.cwd().resolve()
    for entry in out.splitlines():
        path = Path(entry).resolve()
        try:
            relative = path.relative_to(root)
        except ValueError:
            continue
        dirs.add(relative.as_posix())
    return dirs or None


def impacts(check, paths, dependencies):
    """True means test again. Unrecognized scopes always run again."""
    changed_go = [path for path in paths if path.endswith(".go")]
    changed_source_go = [path for path in changed_go if not path.endswith("_test.go")]
    changed_docs = any(p.startswith(DOC_PREFIXES) or p in DOC_EXACT for p in paths)
    if check in DOC_CHECKS:
        return changed_docs
    if check in PYTHON_CHECKS:
        return False
    if check in GLOBAL_TESTS:
        return bool(changed_go)
    if check in GLOBAL_BUILDS:
        return bool(changed_source_go)
    if check == "cli-gofmt":
        return any(p.startswith("cmd/baha/") for p in changed_go)
    if check.startswith("wizard-"):
        check = "cli-package-tests"
    targets = GO_CHECK_TARGETS.get(check)
    if targets is None:
        return True
    dirs = dependencies.get(targets)
    if dirs is None:
        return True
    for p in changed_go:
        if p.rsplit("/", 1)[0] in dirs:
            if p.endswith("_test.go") and check == "cli-build":
                continue
            return True
    return False


def filter_selected(matrix, runtime_matrix, jobs, paths, *, previous_run, demo_matches,
                    dependencies):
    succeeded = {job.get("name") for job in jobs if job.get("status") == "completed"
                 and job.get("conclusion") == "success"}
    checks = matrix["include"]
    gates = runtime_matrix["include"]
    retained_checks = []
    retained_gates = []
    reused_checks = []
    reused_gates = []
    for row in checks:
        name = "PR · Validation · " + row["check"]
        if row["check"] != "not-required" and name in succeeded and not impacts(row["check"], paths, dependencies):
            reused_checks.append(row["check"])
        else:
            retained_checks.append(row)
    for row in gates:
        gate = row["id"]
        success = any(name and name.startswith("PR · Selected Runtime (" + gate + ",")
                      for name in succeeded)
        # Runtime provenance includes the immutable demo revision, not merely source ancestry.
        if gate != "not-required" and success and demo_matches and not any(p.endswith(".go") for p in paths):
            retained_gates.append({**row, "reuse_from": str(previous_run)})
            reused_gates.append(gate)
        else:
            retained_gates.append({**row, "reuse_from": ""})
    return {
        "matrix": {"include": retained_checks or [{"check": "not-required"}]},
        "runtime_matrix": {"include": retained_gates},
        "reused_checks": sorted(reused_checks),
        "reused_gates": sorted(reused_gates),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for arg in ("repo", "branch", "head", "integration", "workflow-id", "run-id",
                "pr-number", "token", "demo-ref", "plan", "runtime-matrix", "output"):
        parser.add_argument("--" + arg, required=True)
    a = parser.parse_args()
    plan = json.loads(Path(a.plan).read_text())
    checks = plan["required_validation_checks"]
    matrix = {"include": [{"check": c["id"]} for c in checks] or [{"check": "not-required"}]}
    runtime_matrix = json.loads(Path(a.runtime_matrix).read_text())
    result = {"matrix": matrix, "runtime_matrix": runtime_matrix,
              "reused_checks": [], "reused_gates": [], "previous_run": None,
              "reason": "no qualified previous run"}
    try:
        if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", a.repo):
            raise ValueError("invalid repository")
        current = request_json(a.repo, "actions/runs/" + str(int(a.run_id)), a.token)
        workflow_id = current.get("workflow_id")
        if (current.get("event") != "pull_request" or
                not isinstance(workflow_id, int) or workflow_id <= 0):
            raise ValueError("current run is not an authenticated PR workflow")
        previous = completed_previous(a.repo, workflow_id, a.branch,
                                      int(a.pr_number), int(a.run_id), a.token)
        if previous and not within_24_hours(previous, current):
            result["reason"] = "previous run is outside the 24-hour window"
        elif previous:
            paths = qualified_ancestry(previous["head_sha"], a.integration, a.head)
            if paths is None:
                result["reason"] = "previous source/base ancestry not proven"
            elif path_classification(paths) is None:
                result["reason"] = "unknown or sensitive changed path: full checks"
            else:
                jobs = all_jobs(a.repo, previous["id"], a.token)
                native_preconditions = {
                    "PR · Demo Pin", "PR · Exact Candidate", "PR · Runner Interface"
                }
                native_ready = native_preconditions <= {
                    job.get("name") for job in jobs
                    if job.get("conclusion") == "success" and job.get("status") == "completed"
                }
                # Ecosystem inputs may include independent Console/Connector revisions.
                deps = plan.get("required_consumer_dependencies", [])
                native_ready = native_ready and deps in (["core", "demo"], ["core"])
                previous_demo = prior_demo(a.repo, previous["id"], a.token) if native_ready else None
                demo_matches = bool(previous_demo and previous_demo == a.demo_ref)
                # Resolve dependency closures only for the selected scoped checks.
                targets = {GO_CHECK_TARGETS.get(c["id"], GO_CHECK_TARGETS.get("cli-package-tests")
                           if c["id"].startswith("wizard-") else None) for c in checks}
                dependencies = {target: go_dependency_dirs(target)
                                for target in targets if target is not None}
                decided = filter_selected(matrix, runtime_matrix, jobs, paths,
                                          previous_run=previous["id"],
                                          demo_matches=demo_matches,
                                          dependencies=dependencies)
                result.update(decided)
                result["previous_run"] = previous["id"]
                result["reason"] = "prior successful checks reused only where sources and inputs unchanged"
                result["changed_files"] = paths
                result["demo_matches"] = demo_matches
    except (OSError, ValueError, TypeError, KeyError, urllib.error.URLError,
            subprocess.CalledProcessError, subprocess.TimeoutExpired,
            json.JSONDecodeError, zipfile.BadZipFile) as error:
        # A missing rate limit, stale run, unavailable commit or malformed API
        # response is never a license to skip a required test.
        result["reason"] = "reuse unavailable, full checks: " + type(error).__name__
    output = Path(a.output)
    with output.open("a") as stream:
        for key in ("matrix", "runtime_matrix"):
            stream.write(key + "=" + json.dumps(result[key], separators=(",", ":")) + "\n")
    print(json.dumps(result, sort_keys=True))
    print("::notice::Incremental PR checks: " + result["reason"] +
          "; reused validations=" + str(len(result["reused_checks"])) +
          "; reused runtime gates=" + str(len(result["reused_gates"])))


if __name__ == "__main__":
    main()
