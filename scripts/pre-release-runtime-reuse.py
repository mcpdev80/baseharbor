#!/usr/bin/env python3
"""Fail-closed incremental reuse planner for Pre-Release runtime matrix gates.

Only successful, manifest-backed gates from this workflow, created in the last
7 days, can be reused. Core and Demo changes need proven ancestry and an
explicit impact-map classification. Unknown paths invalidate every matrix gate.
"""
import argparse
from datetime import datetime, timedelta, timezone
import hashlib
import http.client
import io
import json
import fnmatch
from pathlib import Path
import re
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile

SHA = re.compile(r"^[0-9a-f]{40}$")
TTL_SECONDS = 7 * 24 * 60 * 60
SCHEMA = "baseharbor.pre-release-runtime-gate-evidence/v1"
MAX_ARTIFACT_BYTES = 256 * 1024


def request_json(repo, path, token):
    request = urllib.request.Request(
        "https://api.github.com/repos/" + repo + "/" + path,
        headers={"Authorization": "Bearer " + token,
                 "Accept": "application/vnd.github+json",
                 "X-GitHub-Api-Version": "2022-11-28"})
    for attempt in range(3):
        try:
            with urllib.request.urlopen(request, timeout=15) as response:
                return json.load(response)
        except http.client.IncompleteRead as error:
            if attempt == 2:
                raise urllib.error.URLError("incomplete GitHub API response") from error
            time.sleep(0.25 * (2 ** attempt))
    raise urllib.error.URLError("GitHub API response retry exhausted")


def artifact_manifest(repo, artifact, token):
    """Fetch one small JSON evidence artifact using the signed redirect safely."""
    url = ("https://api.github.com/repos/" + repo + "/actions/artifacts/" +
           str(artifact["id"]) + "/zip")

    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, req, fp, code, msg, headers, newurl):
            return None

    opener = urllib.request.build_opener(NoRedirect)
    request = urllib.request.Request(url, headers={
        "Authorization": "Bearer " + token,
        "Accept": "application/vnd.github+json"})
    try:
        opener.open(request, timeout=15)
        raise ValueError("artifact endpoint did not redirect")
    except urllib.error.HTTPError as error:
        if error.code not in (301, 302, 303, 307):
            raise
        target = error.headers.get("Location", "")
    parsed = urllib.parse.urlparse(target)
    if parsed.scheme != "https" or not parsed.hostname or parsed.hostname == "api.github.com":
        raise ValueError("unsafe artifact redirect")
    with urllib.request.urlopen(target, timeout=20) as response:
        data = response.read(MAX_ARTIFACT_BYTES + 1)
    if len(data) > MAX_ARTIFACT_BYTES:
        raise ValueError("oversized gate evidence")
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        members = [item for item in archive.namelist()
                   if item == "manifest.json" or item.endswith("/manifest.json")]
        if len(members) != 1:
            raise ValueError("missing unique gate evidence manifest")
        manifest = json.loads(archive.read(members[0]))
    if not isinstance(manifest, dict):
        raise ValueError("invalid gate evidence manifest")
    return manifest


def parse_time(value):
    if not isinstance(value, str):
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None
    return parsed if parsed.tzinfo and parsed.utcoffset() == timedelta(0) else None


def within_7_days(previous_created_at, current_created_at):
    before = parse_time(previous_created_at)
    current = parse_time(current_created_at)
    if before is None or current is None:
        return False
    elapsed = (current - before).total_seconds()
    return 0 <= elapsed <= TTL_SECONDS


def canonical_hash(value):
    encoded = json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(encoded).hexdigest()


def sha256_bytes(value):
    return hashlib.sha256(value).hexdigest()


def git(*args, cwd=None):
    return subprocess.check_output(("git", *args), cwd=cwd, text=True,
                                  stderr=subprocess.DEVNULL, timeout=45).strip()


def blob_at(revision, path, cwd=None):
    try:
        data = subprocess.check_output(("git", "show", revision + ":" + path), cwd=cwd,
                                       stderr=subprocess.DEVNULL, timeout=15)
    except subprocess.CalledProcessError:
        return None
    return sha256_bytes(data)


def tree_fingerprint(revision, paths, cwd=None):
    rows = []
    for path in sorted(set(paths)):
        digest = blob_at(revision, path, cwd=cwd)
        rows.append((path, digest))
    return rows


def execution_tree_fingerprint(revision, paths, cwd=None):
    """Fingerprint executable release contracts, excluding only the push hook."""
    rows = []
    for path in sorted(set(paths)):
        if path == ".github/workflows/pre-release.yml":
            data = subprocess.check_output(("git", "show", revision + ":" + path), cwd=cwd,
                                           stderr=subprocess.DEVNULL, timeout=15)
            # Adding the release-branch auto-dispatch hook changes how a run
            # starts, not what it qualifies. Preserve all manual/call inputs,
            # jobs and execution steps in the fingerprint.
            data = data.replace(b"    - release/v*\n", b"")
            digest = sha256_bytes(data)
        else:
            digest = blob_at(revision, path, cwd=cwd)
        rows.append((path, digest))
    return rows


def version_digests(core_sha, demo_sha, impact, *, core_cwd=None, demo_cwd=None):
    workflow = execution_tree_fingerprint(core_sha, impact["execution_inputs"], cwd=core_cwd)
    catalog = tree_fingerprint(demo_sha, impact["demo_catalog_inputs"], cwd=demo_cwd)
    return canonical_hash(workflow), canonical_hash(catalog)


def gate_demo_prefix(gate, impact):
    for prefix, root in impact["demo_gate_directories"].items():
        if gate.startswith(prefix):
            return root + gate[len(prefix):] + "/"
    return None


def gate_core_paths(gate, impact):
    paths = []
    for path, targets in impact["core"].items():
        if "*" in targets or gate in targets:
            paths.append(path)
    return paths


def gate_demo_paths(gate, demo_sha, impact, *, demo_cwd=None):
    prefix = gate_demo_prefix(gate, impact)
    if prefix is None:
        raise ValueError("no Demo directory mapping for gate")
    try:
        tree = git("ls-tree", "-r", "--name-only", demo_sha, "--", prefix, cwd=demo_cwd)
    except (OSError, subprocess.SubprocessError):
        raise ValueError("cannot enumerate gate Demo inputs")
    paths = [line for line in tree.splitlines() if line]
    paths.extend(path for path in impact["demo"] if path in impact["demo_catalog_inputs"])
    return paths


def gate_fingerprint(tag, gate, core_sha, demo_sha, workflow_version,
                     catalog_version, impact, *, core_cwd=None, demo_cwd=None):
    core_inputs = sorted(
        execution_tree_fingerprint(core_sha, impact["execution_inputs"], cwd=core_cwd) +
        tree_fingerprint(core_sha, gate_core_paths(gate, impact), cwd=core_cwd))
    demo_inputs = tree_fingerprint(
        demo_sha, gate_demo_paths(gate, demo_sha, impact, demo_cwd=demo_cwd), cwd=demo_cwd)
    return canonical_hash({
        "tag": tag,
        "gate": gate,
        "workflow_version": workflow_version,
        "catalog_version": catalog_version,
        "core_inputs": core_inputs,
        "demo_inputs": demo_inputs,
    })


def legacy_evidence_identity(manifest, tag, gate, impact, *, current_core, current_demo,
                             core_cwd, demo_cwd):
    """Verify pre-execution-input-schema evidence against its original source.

    Older manifests fingerprinted the reuse planner and impact map together
    with the actual runtime executors. Recompute that historical fingerprint
    from the evidence's own immutable commits, then separately require that all
    current executor/catalog inputs still match those commits.
    """
    old_core, old_demo = manifest.get("candidate_sha", ""), manifest.get("demo_ref", "")
    if not SHA.fullmatch(old_core) or not SHA.fullmatch(old_demo):
        return None
    try:
        old_impact = json.loads(subprocess.check_output(
            ("git", "show", old_core + ":scripts/pre-release-runtime-impact.json"),
            cwd=core_cwd, text=True, stderr=subprocess.DEVNULL, timeout=15))
        old_workflow_inputs = old_impact["workflow_inputs"]
        old_execution_inputs = [path for path in old_workflow_inputs if path not in (
            "scripts/pre-release-runtime-impact.json", "scripts/pre-release-runtime-reuse.py")]
        if old_execution_inputs != impact["execution_inputs"]:
            return None
        old_execution = canonical_hash(execution_tree_fingerprint(
            old_core, old_execution_inputs, cwd=core_cwd))
        current_execution = canonical_hash(execution_tree_fingerprint(
            current_core, impact["execution_inputs"], cwd=core_cwd))
        if old_execution != current_execution:
            return None
        subprocess.run(("git", "-C", str(demo_cwd), "fetch", "--quiet", "--no-tags",
                        "origin", old_demo), check=True, stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, timeout=45)
        old_catalog = canonical_hash(tree_fingerprint(
            old_demo, old_impact["demo_catalog_inputs"], cwd=demo_cwd))
        current_catalog = canonical_hash(tree_fingerprint(
            current_demo, impact["demo_catalog_inputs"], cwd=demo_cwd))
        if old_catalog != current_catalog:
            return None
        workflow_version = canonical_hash(tree_fingerprint(
            old_core, old_workflow_inputs, cwd=core_cwd))
        if workflow_version != manifest.get("workflow_version") or old_catalog != manifest.get("catalog_version"):
            return None
        core_inputs = tree_fingerprint(
            old_core, old_workflow_inputs + gate_core_paths(gate, old_impact), cwd=core_cwd)
        demo_inputs = tree_fingerprint(
            old_demo, gate_demo_paths(gate, old_demo, old_impact, demo_cwd=demo_cwd), cwd=demo_cwd)
        fingerprint = canonical_hash({
            "tag": tag, "gate": gate, "workflow_version": workflow_version,
            "catalog_version": old_catalog, "core_inputs": core_inputs,
            "demo_inputs": demo_inputs,
        })
        if fingerprint != manifest.get("input_fingerprint"):
            return None
        return fingerprint, workflow_version, old_catalog
    except (KeyError, OSError, TypeError, ValueError, subprocess.SubprocessError,
            json.JSONDecodeError):
        return None


def classify_paths(paths, mapping, safe_patterns):
    affected = set()
    for path in paths:
        if path in mapping:
            targets = mapping[path]
            if "*" in targets:
                return {"*"}
            affected.update(targets)
            continue
        if any(fnmatch.fnmatchcase(path, pattern) for pattern in safe_patterns):
            continue
        return None
    return affected


def paths_between(old_sha, new_sha, *, cwd=None):
    if not SHA.fullmatch(old_sha or "") or not SHA.fullmatch(new_sha or ""):
        return None
    try:
        subprocess.run(("git", "fetch", "--no-tags", "origin", old_sha), cwd=cwd,
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                       timeout=45)
        if subprocess.run(("git", "merge-base", "--is-ancestor", old_sha, new_sha), cwd=cwd,
                          stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                          timeout=20).returncode:
            return None
        raw = subprocess.check_output(("git", "diff", "--name-only", "-z", old_sha, new_sha),
                                      cwd=cwd, timeout=30)
        return [item.decode("utf-8") for item in raw.split(b"\0") if item]
    except (OSError, subprocess.SubprocessError):
        return None


def demo_paths_between(old_sha, new_sha, repo_url, token, temp_dir):
    if not SHA.fullmatch(old_sha or "") or not SHA.fullmatch(new_sha or ""):
        return None
    clone = Path(temp_dir) / ("demo-" + old_sha[:12] + "-" + new_sha[:12])
    try:
        if not clone.exists():
            subprocess.run(("git", "clone", "--quiet", "--filter=blob:none", "--no-checkout",
                            repo_url, str(clone)), check=True, stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL, timeout=90)
        for revision in (old_sha, new_sha):
            subprocess.run(("git", "-C", str(clone), "fetch", "--quiet", "--no-tags",
                            "origin", revision), check=True, stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL, timeout=45)
        return paths_between(old_sha, new_sha, cwd=clone)
    except (OSError, subprocess.SubprocessError):
        return None


def job_succeeded(jobs, runtime, gate):
    suffixes = ("/ Executed " + runtime + " · " + gate.rsplit("/", 1)[-1],
                "/ Reused " + runtime + " · " + gate.rsplit("/", 1)[-1])
    return any(job.get("status") == "completed" and job.get("conclusion") == "success"
               and job.get("name", "").endswith(suffixes) for job in jobs)


def valid_manifest(manifest, *, run_id, tag, runtime, gate, fingerprint,
                   workflow_version, catalog_version, now, run_created_at,
                   current_run_created_at, evidence_created_at):
    source_run_created = manifest.get("source_run_created_at") or run_created_at
    source_artifact_created = manifest.get("source_artifact_created_at") or evidence_created_at
    qualified = manifest.get("qualified_at")
    return (manifest.get("schema") == SCHEMA and
            manifest.get("status") == "success" and
            str(manifest.get("run_id")) == str(run_id) and
            str(manifest.get("source_run_id", "" )).isdigit() and
            manifest.get("tag") == tag and
            manifest.get("runtime") == runtime and
            manifest.get("gate") == gate and
            manifest.get("input_fingerprint") == fingerprint and
            manifest.get("workflow_version") == workflow_version and
            manifest.get("catalog_version") == catalog_version and
            SHA.fullmatch(manifest.get("candidate_sha", "")) is not None and
            SHA.fullmatch(manifest.get("demo_ref", "")) is not None and
            manifest.get("artifact_name") == artifact_name(runtime, gate.rsplit("/", 1)[-1]) and
            within_7_days(run_created_at, now) and
            within_7_days(evidence_created_at, now) and
            within_7_days(source_run_created, now) and
            within_7_days(source_artifact_created, now) and
            within_7_days(qualified, now) and
            timestamps_ordered(run_created_at, evidence_created_at, current_run_created_at) and
            timestamps_ordered(source_run_created, qualified, source_artifact_created))


def timestamps_ordered(start, middle, end):
    first, center, last = parse_time(start), parse_time(middle), parse_time(end)
    return first is not None and center is not None and last is not None and first <= center <= last


def decide_reuse(*, job_ok, manifest, run_id, tag, runtime, gate, fingerprint,
                 workflow_version, catalog_version, now, run_created_at,
                 current_run_created_at, evidence_created_at):
    if not job_ok or not isinstance(manifest, dict) or not evidence_created_at:
        return False
    return valid_manifest(manifest, run_id=run_id, tag=tag, runtime=runtime, gate=gate,
                          fingerprint=fingerprint, workflow_version=workflow_version,
                          catalog_version=catalog_version, now=now,
                          run_created_at=run_created_at,
                          current_run_created_at=current_run_created_at,
                          evidence_created_at=evidence_created_at)


def artifact_matches_run(artifact, run_id, expected_name, now, current_run_created_at):
    workflow_run = artifact.get("workflow_run")
    if not isinstance(workflow_run, dict) or str(workflow_run.get("id")) != str(run_id):
        return False
    if artifact.get("name") != expected_name or artifact.get("expired") is not False:
        return False
    created = artifact.get("created_at")
    created_time = parse_time(created)
    current_run_time = parse_time(current_run_created_at)
    if created_time is None or current_run_time is None or created_time > current_run_time:
        return False
    expires_time = parse_time(artifact.get("expires_at"))
    now_time = parse_time(now)
    if expires_time is None or now_time is None or expires_time <= now_time:
        return False
    try:
        if int(artifact.get("size_in_bytes", 0)) <= 0 or int(artifact.get("id", 0)) <= 0:
            return False
    except (TypeError, ValueError):
        return False
    return True


def list_runs(repo, workflow_id, token):
    rows = []
    for page in range(1, 6):
        data = request_json(repo, "actions/workflows/" + str(workflow_id) +
                            "/runs?per_page=100&page=" + str(page), token)
        batch = data.get("workflow_runs", [])
        rows.extend(batch)
        if len(batch) < 100:
            break
    else:
        raise ValueError("pre-release run history exceeded bounded inventory")
    return rows


def list_jobs(repo, run_id, token):
    jobs = []
    for page in range(1, 6):
        batch = request_json(repo, "actions/runs/" + str(run_id) +
                             "/jobs?per_page=100&page=" + str(page), token).get("jobs", [])
        jobs.extend(batch)
        if len(batch) < 100:
            break
    else:
        raise ValueError("job inventory exceeded bounded limit")
    return jobs


def list_artifacts(repo, run_id, token):
    data = request_json(repo, "actions/runs/" + str(run_id) +
                        "/artifacts?per_page=100", token)
    return [row for row in data.get("artifacts", []) if not row.get("expired")]


def artifact_name(runtime, gate):
    return "pre-release-runtime-evidence-" + runtime + "-" + gate.rsplit("/", 1)[-1]


def build_outputs(gates, tag, candidate_sha, demo_sha, current_run, token,
                  workflow_version, catalog_version, impact, demo_repo,
                  *, core_cwd=None, temp_dir=None):
    rows = []
    all_gates = [row["id"] for row in gates]
    current_time = current_run.get("created_at")
    actual_now = datetime.now(timezone.utc).isoformat()
    previous_runs = []
    job_cache, artifact_cache, core_diff_cache, demo_diff_cache = {}, {}, {}, {}
    manifest_cache = {}
    for run in list_runs(current_run["repository"], current_run["workflow_id"], token):
        if run.get("id") == current_run.get("id") or run.get("status") != "completed":
            continue
        if run.get("conclusion") not in ("success", "failure"):
            continue
        if run.get("event") not in ("workflow_dispatch", "workflow_call"):
            continue
        if within_7_days(run.get("created_at"), actual_now):
            previous_runs.append(run)

    for row in gates:
        gate_id, runtime, gate = row["id"], row["runtime"], row["gate"]
        fingerprint = gate_fingerprint(tag, gate_id, candidate_sha, demo_sha,
                                        workflow_version, catalog_version, impact,
                                        core_cwd=core_cwd, demo_cwd=Path(temp_dir) / "demo-current")
        reusable = ""
        for previous in previous_runs:
            try:
                run_id = previous["id"]
                if run_id not in job_cache:
                    job_cache[run_id] = list_jobs(current_run["repository"], run_id, token)
                jobs = job_cache[run_id]
                if not job_succeeded(jobs, runtime, gate):
                    continue
                expected_name = artifact_name(runtime, gate)
                if run_id not in artifact_cache:
                    artifact_cache[run_id] = list_artifacts(current_run["repository"], run_id, token)
                artifacts = [a for a in artifact_cache[run_id]
                             if artifact_matches_run(a, previous["id"], expected_name,
                                                     actual_now, current_time)]
                if len(artifacts) != 1:
                    continue
                artifact = artifacts[0]
                manifest_key = (run_id, artifact["id"])
                if manifest_key not in manifest_cache:
                    manifest_cache[manifest_key] = artifact_manifest(
                        current_run["repository"], artifact, token)
                manifest = manifest_cache[manifest_key]
                old_core = manifest.get("candidate_sha", "")
                old_demo = manifest.get("demo_ref", "")
                core_key = (old_core, candidate_sha)
                if core_key not in core_diff_cache:
                    core_diff_cache[core_key] = paths_between(old_core, candidate_sha, cwd=core_cwd)
                core_diff = core_diff_cache[core_key]
                if core_diff is None:
                    continue
                core_affected = classify_paths(core_diff, impact["core"], impact["safe_core_paths"])
                demo_key = (old_demo, demo_sha)
                if demo_key not in demo_diff_cache:
                    demo_diff_cache[demo_key] = demo_paths_between(
                        old_demo, demo_sha, demo_repo, token, temp_dir)
                demo_diff = demo_diff_cache[demo_key]
                if demo_diff is None:
                    continue
                demo_affected = classify_paths(demo_diff, impact["demo"], impact["safe_demo_paths"])
                if core_affected is None or demo_affected is None:
                    continue
                if "*" in core_affected or "*" in demo_affected:
                    continue
                if gate_id in core_affected or gate_id in demo_affected:
                    continue
                reusable_evidence = decide_reuse(
                    job_ok=True, manifest=manifest, run_id=previous["id"], tag=tag,
                    runtime=runtime, gate=gate_id, fingerprint=fingerprint,
                    workflow_version=workflow_version, catalog_version=catalog_version,
                    now=actual_now, run_created_at=previous.get("created_at"),
                    current_run_created_at=current_time,
                    evidence_created_at=artifact.get("created_at"))
                if not reusable_evidence:
                    legacy = legacy_evidence_identity(
                        manifest, tag, gate_id, impact, current_core=candidate_sha,
                        current_demo=demo_sha, core_cwd=core_cwd,
                        demo_cwd=Path(temp_dir) / "demo-current")
                    if legacy is not None:
                        old_fingerprint, old_workflow, old_catalog = legacy
                        reusable_evidence = decide_reuse(
                            job_ok=True, manifest=manifest, run_id=previous["id"], tag=tag,
                            runtime=runtime, gate=gate_id, fingerprint=old_fingerprint,
                            workflow_version=old_workflow, catalog_version=old_catalog,
                            now=actual_now, run_created_at=previous.get("created_at"),
                            current_run_created_at=current_time,
                            evidence_created_at=artifact.get("created_at"))
                if reusable_evidence:
                    reusable = str(previous["id"])
                    row["evidence_artifact_id"] = str(artifact["id"])
                    row["evidence_created_at"] = artifact["created_at"]
                    row["evidence_run_created_at"] = previous.get("created_at", "")
                    break
            except (KeyError, OSError, TypeError, ValueError, urllib.error.URLError,
                    subprocess.SubprocessError, zipfile.BadZipFile, json.JSONDecodeError):
                continue
        rows.append({**row, "reuse_from": reusable,
                     "input_fingerprint": fingerprint,
                     "workflow_version": workflow_version,
                     "catalog_version": catalog_version,
                     "evidence_reuse": True,
                     "evidence_artifact_id": row.get("evidence_artifact_id", ""),
                     "evidence_created_at": row.get("evidence_created_at", ""),
                     "evidence_run_created_at": row.get("evidence_run_created_at", ""),
                     "current_run_created_at": current_time})
    return rows


def no_reuse_outputs(gates, tag, candidate_sha, demo_sha, workflow_version,
                     catalog_version, impact, *, core_cwd=None, demo_cwd=None):
    rows = []
    for row in gates:
        try:
            fingerprint = gate_fingerprint(tag, row["id"], candidate_sha, demo_sha,
                                           workflow_version, catalog_version, impact,
                                           core_cwd=core_cwd, demo_cwd=demo_cwd)
        except (OSError, ValueError, subprocess.SubprocessError):
            fingerprint = ""
        rows.append({**row, "reuse_from": "", "input_fingerprint": fingerprint,
                     "workflow_version": workflow_version,
                     "catalog_version": catalog_version,
                     "evidence_reuse": True, "evidence_artifact_id": "",
                     "evidence_created_at": "", "evidence_run_created_at": "",
                     "current_run_created_at": ""})
    return rows


def selected_runtime_gates(tag, only_gates, requirements):
    path = Path(__file__).with_name("pre-release-check-plan.py")
    import importlib.util
    spec = importlib.util.spec_from_file_location("pre_release_check_plan", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    selected = module.select(requirements, tag, only_gates)
    rows = []
    for gate in selected["static"]:
        rows.append({"id": "atomic/static/" + gate, "runtime": "static", "gate": gate})
    for runtime in ("docker", "podman"):
        for group in ("light", "heavy"):
            for gate in selected[group + "_" + runtime]:
                rows.append({"id": "atomic/" + runtime + "/" + gate,
                             "runtime": runtime, "gate": gate})
    return rows


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True)
    parser.add_argument("--run-id", required=True, type=int)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--candidate-sha", required=True)
    parser.add_argument("--demo-ref", required=True)
    parser.add_argument("--only-gates", default="")
    parser.add_argument("--token", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--temp-dir", required=True)
    args = parser.parse_args()
    base = Path(__file__).resolve().parent.parent
    impact = json.loads((base / "scripts/pre-release-runtime-impact.json").read_text())
    requirements = json.loads((base / "scripts/release-requirements.json").read_text())
    if not SHA.fullmatch(args.candidate_sha) or not SHA.fullmatch(args.demo_ref):
        raise SystemExit("candidate and Demo refs must be exact commit SHAs")
    Path(args.temp_dir).mkdir(parents=True, exist_ok=True)
    # Develop mode uses the existing v0.4.26 inventory baseline while keeping
    # the release identity itself as "develop" in evidence/fingerprints.
    plan_tag = "v0.4.26" if args.tag == "develop" else args.tag
    gates = selected_runtime_gates(plan_tag, args.only_gates, requirements)
    demo_repo = "https://github.com/mcpdev80/baseharbor-demo.git"
    demo_cwd = Path(args.temp_dir) / "demo-current"
    try:
        if not demo_cwd.exists():
            subprocess.run(("git", "clone", "--quiet", "--filter=blob:none", "--no-checkout",
                            demo_repo, str(demo_cwd)), check=True, timeout=90)
        subprocess.run(("git", "-C", str(demo_cwd), "fetch", "--quiet", "--no-tags",
                        "origin", args.demo_ref), check=True, timeout=45)
        workflow_version, catalog_version = version_digests(
            args.candidate_sha, args.demo_ref, impact, core_cwd=base, demo_cwd=demo_cwd)
        current = request_json(args.repo, "actions/runs/" + str(args.run_id), args.token)
        if current.get("event") not in ("workflow_dispatch", "workflow_call"):
            raise ValueError("current run is not a pre-release dispatch/call")
        current["repository"] = args.repo
        if not isinstance(current.get("workflow_id"), int):
            raise ValueError("current run has no workflow identity")
    except (KeyError, OSError, TypeError, ValueError, urllib.error.URLError,
            subprocess.SubprocessError, json.JSONDecodeError):
        workflow_version = canonical_hash(execution_tree_fingerprint(
            args.candidate_sha, impact["execution_inputs"], cwd=base))
        catalog_version = "unavailable"
        rows = no_reuse_outputs(gates, args.tag, args.candidate_sha, args.demo_ref,
                                workflow_version, catalog_version, impact,
                                core_cwd=base, demo_cwd=demo_cwd if demo_cwd.exists() else None)
        current = None
    if current is None:
        output = {
            "static": [r for r in rows if r["runtime"] == "static"],
            **{name + "_" + runtime: [r for r in rows if r["runtime"] == runtime and
               ((r["gate"] in ("guided", "lifecycle", "policy", "connectivity", "reconciliation", "failure", "full-destroy")) == (name == "light"))]
               for runtime in ("docker", "podman") for name in ("light", "heavy")},
        }
        for key, value in list(output.items()):
            output[key + "_execute_count"] = sum(not row["reuse_from"] for row in value)
        with open(args.output, "a", encoding="utf-8") as stream:
            for key, value in output.items():
                stream.write(key + "=" + json.dumps(value, separators=(",", ":")) + "\n")
        print(json.dumps(output, sort_keys=True))
        return
    try:
        rows = build_outputs(gates, args.tag, args.candidate_sha, args.demo_ref,
                             current, args.token, workflow_version, catalog_version,
                             impact, demo_repo, core_cwd=base, temp_dir=args.temp_dir)
    except (KeyError, OSError, TypeError, ValueError, urllib.error.URLError,
            subprocess.SubprocessError, zipfile.BadZipFile, json.JSONDecodeError):
        rows = no_reuse_outputs(gates, args.tag, args.candidate_sha, args.demo_ref,
                                workflow_version, catalog_version, impact,
                                core_cwd=base, demo_cwd=demo_cwd)
    output = {
        "static": [r for r in rows if r["runtime"] == "static"],
        "light_docker": [r for r in rows if r["runtime"] == "docker" and r["gate"] in
                         ("guided", "lifecycle", "policy", "connectivity", "reconciliation", "failure", "full-destroy")],
        "heavy_docker": [r for r in rows if r["runtime"] == "docker" and r["gate"] not in
                         ("guided", "lifecycle", "policy", "connectivity", "reconciliation", "failure", "full-destroy")],
        "light_podman": [r for r in rows if r["runtime"] == "podman" and r["gate"] in
                         ("guided", "lifecycle", "policy", "connectivity", "reconciliation", "failure", "full-destroy")],
        "heavy_podman": [r for r in rows if r["runtime"] == "podman" and r["gate"] not in
                         ("guided", "lifecycle", "policy", "connectivity", "reconciliation", "failure", "full-destroy")],
    }
    for key, value in list(output.items()):
        output[key + "_execute_count"] = sum(not row["reuse_from"] for row in value)
    with open(args.output, "a", encoding="utf-8") as stream:
        for key, value in output.items():
            stream.write(key + "=" + json.dumps(value, separators=(",", ":")) + "\n")
    print(json.dumps(output, sort_keys=True))


if __name__ == "__main__":
    main()
