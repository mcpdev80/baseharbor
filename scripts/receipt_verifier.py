#!/usr/bin/env python3
"""Live GitHub verification and retained Git-history verification for one pilot gate."""
import argparse
import datetime as dt
import io
import json
import os
import re
import subprocess
import zipfile
from pathlib import Path

GATE = "public-contracts"
REPOSITORY = "mcpdev80/baseharbor"
REPOSITORY_ID = 1360667669
OWNER = "mcpdev80"
API_VERSION = "2026-03-10"
SHA = re.compile(r"[0-9a-f]{40}\Z")
PLAN_PATH = "evidence/receipt-plans/public-contracts.json"


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
    result = subprocess.run(["git", "-C", str(root), *args], capture_output=True)
    require(result.returncode == 0, "Git operation failed: " + args[0])
    return result.stdout


def blob(root, revision, path):
    require(full_sha(revision), "full Git revision required")
    entry = git(root, "ls-tree", revision, "--", path).decode().split()
    require(len(entry) == 4 and entry[0:2] == ["100644", "blob"] and entry[3] == path,
            "regular committed file required")
    require(int(git(root, "cat-file", "-s", entry[2])) < 4096, "committed file too large")
    return git(root, "show", f"{revision}:{path}")


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


def github(path, *, binary=False):
    require(path.startswith((f"repos/{REPOSITORY}/actions/runs/",
                             f"repos/{REPOSITORY}/actions/artifacts/")), "unsupported API path")
    result = subprocess.run(["gh", "api", "--method", "GET", "-H",
                             f"X-GitHub-Api-Version: {API_VERSION}", path], capture_output=True)
    require(result.returncode == 0, "GitHub witness unavailable")
    if binary:
        require(0 < len(result.stdout) <= 65536, "plan ZIP too large")
        return result.stdout
    return decode(result.stdout, 2_000_000)


def live(value, plan, api=github):
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


def approve(root, revision, path, candidate, plan_revision, api=None):
    require(os.environ.get("GITHUB_ACTOR") == OWNER
            and os.environ.get("GITHUB_TRIGGERING_ACTOR") == OWNER
            and os.environ.get("GITHUB_EVENT_NAME") == "workflow_dispatch"
            and os.environ.get("GITHUB_REPOSITORY") == REPOSITORY
            and os.environ.get("GITHUB_REF") == "refs/heads/main",
            "authenticated Owner action on protected main required")
    raw, value, plan = verify_file(root, revision, path, candidate, plan_revision)
    git(root, "merge-base", "--is-ancestor", revision,
        git(root, "rev-parse", "HEAD").decode().strip())
    require(git(root, "log", "--full-history", "--format=%H", revision, "--", path)
            .splitlines() == [revision.encode()], "receipt commit must introduce a new path")
    live(value, plan, **({"api": api} if api is not None else {}))
    from receipt_writer import commit_file
    value["approved"] = {"by": OWNER, "at": dt.datetime.now(dt.timezone.utc)
                         .strftime("%Y-%m-%dT%H:%M:%SZ"), "receipt_commit": revision}
    updated = canonical(value)
    receipt(updated)
    return commit_file(root, path, updated, expected=raw)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("live", "approve", "retained"))
    parser.add_argument("--planning", type=Path, required=True)
    parser.add_argument("--revision", required=True)
    parser.add_argument("--receipt", required=True)
    parser.add_argument("--candidate", required=True)
    parser.add_argument("--plan", required=True)
    args = parser.parse_args()
    try:
        if args.mode == "retained":
            retained(args.planning, args.revision, args.receipt, args.candidate, args.plan)
        elif args.mode == "approve":
            approve(args.planning, args.revision, args.receipt, args.candidate, args.plan)
        else:
            _, value, plan = verify_file(args.planning, args.revision, args.receipt,
                                         args.candidate, args.plan)
            live(value, plan)
        print("checked: " + GATE)
    except (ReceiptError, OSError, TypeError) as exc:
        parser.exit(1, "receipt blocked: " + str(exc) + "\n")


if __name__ == "__main__":
    main()
