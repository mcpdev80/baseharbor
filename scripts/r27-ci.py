#!/usr/bin/env python3
"""Dormant trusted R27 CI entrypoint. Never invoke from candidate source."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

from r27_ci_plan import ROOT, load_policy, make_plan
from r27_ci_request import check_current, validate_request, verify_executor_run, WORKFLOW
from r27_ci_issuer import issue_completed_run
from r27_commit_receipt import verify
from r27_github_transport import github_getter
from r27_verified_coverage import EvidenceError, SHA


def git(*args):
    return subprocess.check_output(["git", "-C", str(ROOT), *args], text=True).strip()


def gh(method, path, data=None, *, missing_ok=False):
    command = ["gh", "api", "--method", method, path]
    if data is not None:
        command += ["--input", "-"]
    result = subprocess.run(command, input=json.dumps(data) if data is not None else None,
                            text=True, capture_output=True, check=False)
    if result.returncode:
        if missing_ok and "(HTTP 404)" in result.stderr:
            return None
        raise EvidenceError("CI_API: GitHub request failed; inspect scoped permissions")
    return json.loads(result.stdout) if result.stdout.strip() else None


def protected_context():
    if os.environ.get("R27_CI_ENABLED") != "true":
        raise EvidenceError("CI_LOCKED: R27 rollout is disabled")
    executor = os.environ.get("R27_TRUSTED_EXECUTOR_SHA", "")
    if not SHA.fullmatch(executor) or git("rev-parse", "HEAD") != executor:
        raise EvidenceError("CI_TRUST: this checkout is not the pinned executor")
    digest = os.environ.get("R27_POLICY_DIGEST", "")
    policy = load_policy(ROOT / "deploy/ci/r27/policy.json", digest)
    return executor, digest, policy


def fetch_sources(request):
    request = validate_request(request)
    # Fetch objects only; never checkout, import, source or run candidate code.
    for sha in sorted({request["base"], request["candidate"], request["integration"]}):
        git("fetch", "--no-tags", "origin", sha)


def input_request():
    number = os.environ.get("R27_PR", "0")
    if not number.isascii() or not number.isdigit() or str(int(number)) != number:
        raise EvidenceError("CI_REQUEST: PR number must be canonical decimal")
    return validate_request(dict(stage=os.environ.get("R27_STAGE"), number=int(number),
                                 candidate=os.environ.get("R27_CANDIDATE"),
                                 base=os.environ.get("R27_BASE"),
                                 integration=os.environ.get("R27_INTEGRATION"),
                                 artifact=os.environ.get("R27_ARTIFACT", "none")))


def candidate_plan(request, policy):
    demo = os.environ.get("R27_DEMO_REPO")
    try:
        return make_plan(request, policy, repository=ROOT, demo_repository=demo), demo
    except EvidenceError as exc:
        if str(exc) != "CI_INPUT: immutable Demo checkout required for native gates":
            raise
    pin = policy["demo_sha"]
    if not isinstance(pin, str) or not SHA.fullmatch(pin):
        raise EvidenceError("CI_POLICY: accepted immutable Demo pin is missing")
    demo = Path(os.environ["RUNNER_TEMP"]) / ("r27-demo-" + pin)
    if not demo.exists():
        subprocess.run(["git", "clone", "--bare", "--filter=blob:none", "--no-tags",
                        "https://github.com/mcpdev80/baseharbor-demo.git", str(demo)], check=True)
    subprocess.run(["git", "-C", str(demo), "fetch", "--no-tags", "origin", pin], check=True)
    return make_plan(request, policy, repository=ROOT, demo_repository=demo), str(demo)


def dispatch(policy, executor):
    event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
    repo = policy["repository"]
    getter = github_getter(os.environ.get("GH_TOKEN"))
    if os.environ.get("GITHUB_EVENT_NAME") == "push" and event.get("ref") == "refs/heads/develop":
        request = validate_request(dict(stage="integration", number=0,
                                        candidate=event.get("after"), base=event.get("before"),
                                        integration=event.get("after"), artifact="none"))
    else:
        if (os.environ.get("GITHUB_EVENT_NAME") != "pull_request_target"
                or event.get("action") not in ("opened", "ready_for_review", "labeled")
                or event.get("action") == "labeled" and event.get("label", {}).get("name") != "ci:validate"):
            raise EvidenceError("CI_REQUEST: no explicit candidate-ready event")
        number = event.get("number")
        if type(number) is not int or number < 1:
            raise EvidenceError("CI_REQUEST: invalid candidate event number")
        pr = getter(f"repos/{repo}/pulls/{number}")
        request = validate_request(dict(stage="pr", number=number, candidate=pr["head"]["sha"],
                                        base=pr["base"]["sha"], integration=pr["merge_commit_sha"], artifact="none"))
    check_current(request, getter, repo)
    ref = getter(f"repos/{repo}/git/ref/heads/ci-policy/r27")
    if ref.get("object", {}).get("sha") != executor:
        raise EvidenceError("CI_TRUST: protected dispatch branch moved; repin after review")
    gh("POST", f"repos/{repo}/actions/workflows/r27-executor.yml/dispatches", {
        "ref": "ci-policy/r27", "inputs": {key: str(value) for key, value in request.items()}})
    print(json.dumps({"state": "WAITING", "code": "CI_DISPATCHED", "request": request}))


def plan(policy):
    request = input_request()
    getter = github_getter(os.environ.get("GH_TOKEN"))
    check_current(request, getter, policy["repository"])
    fetch_sources(request)
    result, _ = candidate_plan(request, policy)
    # Only this trusted job writes matrix outputs. Signer recomputes them later.
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a") as output:
            output.write("checks=" + json.dumps({"check": result["checks"] or ["not-required"]}, separators=(",", ":")) + "\n")
            output.write("native=" + json.dumps({"include": result["native"] or [{"runtime": "static", "gate": "not-required"}]}, separators=(",", ":")) + "\n")
            output.write("native_required=" + str(bool(result["native"])).lower() + "\n")
            output.write("demo_sha=" + policy["demo_sha"] + "\n")
    print(json.dumps(result, sort_keys=True))


def issue(policy, executor, digest, output, seed):
    event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text())
    if os.environ.get("GITHUB_EVENT_NAME") != "workflow_run" or event.get("action") != "completed":
        raise EvidenceError("CI_TRUST: completed workflow_run context required")
    run_id, attempt = event["workflow_run"]["id"], event["workflow_run"]["run_attempt"]
    getter = github_getter(os.environ.get("GH_TOKEN"))
    run = getter(f"repos/{policy['repository']}/actions/runs/{run_id}")
    request = verify_executor_run(run, executor_sha=executor, repository=policy["repository"],
                                  run_id=run_id, attempt=attempt)
    # Remove signing material before any child process can inherit it.
    if len(seed) != 64 or any(c not in "0123456789abcdef" for c in seed):
        raise EvidenceError("CI_TRUST: protected Ed25519 signing seed unavailable")
    fetch_sources(request)
    _, demo = candidate_plan(request, policy)
    raw = issue_completed_run(token=os.environ.get("GH_TOKEN"), key=bytes.fromhex(seed),
                              run_id=run_id, attempt=attempt, executor_sha=executor,
                              policy=policy, policy_digest=digest, repository_path=ROOT,
                              demo_repository=demo)
    verify(raw, key=bytes.fromhex(os.environ["R27_PUBLIC_KEY_HEX"]),
           policy_digest=digest, executor_sha=executor)
    Path(output).write_bytes(raw)
    print(json.dumps({"state": "PASS", "code": "CI_RECEIPT_ISSUED", "request": request}))


def publish(policy, executor, digest, path):
    raw = Path(path).read_bytes()
    record = verify(raw, key=bytes.fromhex(os.environ["R27_PUBLIC_KEY_HEX"]),
                    policy_digest=digest, executor_sha=executor)
    r = record["request"]
    # A current PR/base change also blocks delayed publication of an old proof.
    check_current(r, github_getter(os.environ.get("R27_READ_TOKEN")), policy["repository"])
    destination = (f"evidence/r27/{r['stage']}/{r['candidate']}/"
                   f"{r['integration']}.{digest}.json")
    endpoint = "repos/mcpdev80/baseharbor-plan/contents/" + destination
    existing = gh("GET", endpoint + "?ref=main", missing_ok=True)
    if existing is not None:
        if base64.b64decode(existing["content"]) != raw:
            raise EvidenceError("CI_COLLISION: conflicting evidence already published")
    else:
        gh("PUT", endpoint, {"message": "evidence(r27): qualified exact candidate [skip ci]",
                              "branch": "main", "content": base64.b64encode(raw).decode()})
    print(json.dumps({"state": "PASS", "code": "CI_RECEIPT_PUBLISHED", "path": destination,
                      "sha256": hashlib.sha256(raw).hexdigest()}))


def execute(policy, check, candidate):
    if check not in policy["checks"]:
        raise EvidenceError("CI_POLICY: unknown validation executor")
    if any(os.environ.get(name) for name in ("R27_SIGNING_KEY_HEX", "GH_TOKEN", "GITHUB_TOKEN")):
        raise EvidenceError("CI_TRUST: candidate validation must not receive credentials")
    env = dict(os.environ)
    env.pop("BASH_ENV", None)
    env["R27_TRUSTED_RUNNER"] = str(ROOT / "scripts/pr-check-runner.py")
    try:
        subprocess.run(["bash", "--noprofile", "--norc", "-eo", "pipefail", "-c",
                        policy["checks"][check]["command"]], cwd=candidate, env=env, check=True)
    except subprocess.CalledProcessError as exc:
        raise EvidenceError("CI_VALIDATION_FAILED: selected check failed: " + check) from exc


def main():
    seed = os.environ.pop("R27_SIGNING_KEY_HEX", "")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("dispatch", "plan", "execute", "issue", "publish", "verify"))
    parser.add_argument("--receipt", default="r27-receipt.json")
    parser.add_argument("--check")
    parser.add_argument("--candidate", default=".")
    parser.add_argument("--expected-commit")
    parser.add_argument("--expected-stage", choices=("pr", "integration", "release"))
    parser.add_argument("--policy-digest")
    parser.add_argument("--executor-sha")
    parser.add_argument("--public-key")
    args = parser.parse_args()
    try:
        if args.mode == "verify":
            record = verify(Path(args.receipt).read_bytes(), key=bytes.fromhex(args.public_key or ""),
                            policy_digest=args.policy_digest, executor_sha=args.executor_sha)
            if (record["request"]["candidate"] != args.expected_commit
                    or record["request"]["stage"] != args.expected_stage):
                raise EvidenceError("CI_TARGET: receipt is for another candidate or stage")
            print(json.dumps({"state": "PASS", "code": "CI_OFFLINE_VERIFIED", "request": record["request"]}))
            return 0
        executor, digest, policy = protected_context()
        if args.mode == "dispatch":
            dispatch(policy, executor)
        elif args.mode == "plan":
            plan(policy)
        elif args.mode == "issue":
            issue(policy, executor, digest, args.receipt, seed)
        elif args.mode == "publish":
            publish(policy, executor, digest, args.receipt)
        else:
            if seed:
                raise EvidenceError("CI_TRUST: signer secret supplied to candidate executor")
            execute(policy, args.check, args.candidate)
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as exc:
        code = str(exc).split(":", 1)[0]
        if not code.startswith("CI_"):
            code = "CI_INPUT"
        # Never echo credential-bearing child-process output or raw API bodies.
        actions = {"CI_STALE": "Submit the current head/base using ci:validate",
                   "CI_POLICY": "Independently review the protected policy and repin its digest",
                   "CI_TRUST": "Restore protected executor source, scoped credentials and trust anchor",
                   "CI_LOCKED": "Keep disabled until v0.4.26 closeout and explicit rollout approval",
                   "CI_IMPACT": "Add independently qualified targeted coverage for the reported paths",
                   "CI_SMOKE": "Bind an accepted smoke owner to the exact release artifacts",
                   "CI_VALIDATION_FAILED": "Fix the named validation and resubmit the finished candidate"}
        print(json.dumps({"state": "ACTION_REQUIRED" if code == "CI_VALIDATION_FAILED" else "BLOCKED",
                          "code": code, "reason": str(exc) if isinstance(exc, EvidenceError)
                          else "Invalid input, unavailable file or failed trusted subprocess",
                          "action": actions.get(code, "Inspect the reported input or coverage error")}), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
