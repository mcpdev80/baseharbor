#!/usr/bin/env python3
"""Authenticate original atomic gate artifacts with the EXISTING evidence owner.

Reads trusted GitHub Actions jobs and archived gate evidence. No new schema,
runtime verifier, or PR-supplied PASS payload is accepted.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys

sys.path.insert(0, str(Path.cwd() / "scripts"))
spec=importlib.util.spec_from_file_location("canonical_gate_evidence",Path.cwd()/"scripts/pre-release-resume.py")
canonical=importlib.util.module_from_spec(spec)
spec.loader.exec_module(canonical)


def execute(argv):
    result=subprocess.run(argv,capture_output=True,check=True,timeout=30)
    return result.stdout


def authenticate(plan,report,requirements,run_id,attempt,head_sha,integrated_sha,demo_sha,jobs,artifacts,fetch_archive):
    if not all(re.fullmatch("[0-9a-f]{40}",sha or "") for sha in (head_sha,integrated_sha,demo_sha)):
        raise PermissionError("missing exact source or demo pin")
    if plan.get("head_sha") != head_sha or plan.get("integration_sha") != integrated_sha:
        raise PermissionError("stale selected PR plan")
    inventory={g["id"]:g for g in requirements["gates"] if "v0.4.26" in g.get("required_for",[])}
    selected=plan.get("selected_gates",[])
    if not isinstance(selected,list) or not set(selected)<=set(inventory):
        raise PermissionError("untrusted selected gate inventory")
    evidence=[]
    for key in selected:
        parts=key.split("/")
        if len(parts)!=3 or parts[0]!="atomic" or parts[1] not in ("docker","podman"):
            continue
        runtime,gate=parts[1:]
        matches=[j for j in jobs if j.get("name","").endswith(runtime+" · "+gate)
                 and j.get("run_id")==run_id and j.get("run_attempt")==attempt
                 and j.get("head_sha")==integrated_sha]
        if len(matches)!=1 or matches[0].get("status")!="completed" or matches[0].get("conclusion")!="success":
            continue
        name=f"pre-release-gate-{runtime}-{gate}-evidence-{run_id}-{attempt}"
        origins=[a for a in artifacts if a.get("name")==name and
                 not a.get("expired") and a.get("workflow_run",{}).get("id")==run_id and
                 a.get("workflow_run",{}).get("head_sha")==integrated_sha]
        if len(origins)!=1:
            continue
        origin=origins[0]
        if not isinstance(origin.get("id"),int) or not origin.get("digest"):
            continue
        raw=fetch_archive(origin["id"])
        manifest,manifest_digest=canonical.read_archive(raw,origin["digest"])
        canonical.validate_manifest(manifest,key,run_id,integrated_sha,demo_sha,attempt)
        evidence.append({"kind":"gate","id":key,"conclusion":"success"})
    report["results"].extend(evidence)
    return report


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan",type=Path,required=True)
    parser.add_argument("--results",type=Path,required=True)
    parser.add_argument("--requirements",type=Path,required=True)
    parser.add_argument("--run-id",type=int,required=True)
    parser.add_argument("--attempt",type=int,required=True)
    parser.add_argument("--head-sha",required=True)
    parser.add_argument("--integration-sha",required=True)
    parser.add_argument("--demo-sha",required=True)
    args=parser.parse_args()
    repo="mcpdev80/baseharbor"
    prefix=f"repos/{repo}"
    def api(path):
        return json.loads(execute(["gh","api",prefix+"/"+path]))
    jobs=[]
    artifacts=[]
    for page in range(1,11):
        batch=api(f"actions/runs/{args.run_id}/attempts/{args.attempt}/jobs?per_page=100&page={page}")["jobs"]
        jobs.extend(batch)
        if len(batch)<100:
            break
    else:
        raise RuntimeError("Actions jobs pagination incomplete")
    for page in range(1,11):
        batch=api(f"actions/runs/{args.run_id}/artifacts?per_page=100&page={page}")["artifacts"]
        artifacts.extend(batch)
        if len(batch)<100:
            break
    else:
        raise RuntimeError("Actions artifacts pagination incomplete")
    report=authenticate(json.loads(args.plan.read_text()),json.loads(args.results.read_text()),
                        json.loads(args.requirements.read_text()),args.run_id,args.attempt,
                        args.head_sha,args.integration_sha,args.demo_sha,jobs,artifacts,
                        lambda artifact_id:execute(["gh","api",f"{prefix}/actions/artifacts/{artifact_id}/zip"]))
    args.results.write_text(json.dumps(report,sort_keys=True)+"\n")


if __name__=="__main__":
    main()
