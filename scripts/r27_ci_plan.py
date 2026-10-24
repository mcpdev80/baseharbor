"""Trusted adapter over the existing diff, planner and native input owners.

Never import candidate modules. Candidate Git objects are read as data only.
The policy and this source are loaded from the protected executor checkout.
"""
import fnmatch
import hashlib
import importlib.util
import json
from pathlib import Path
from r27_ci_request import validate_request
from r27_commit_receipt import canonical
from r27_verified_coverage import DIGEST, EvidenceError

ROOT = Path(__file__).resolve().parents[1]


def owner(name):
    spec = importlib.util.spec_from_file_location(name.replace("-", "_"), ROOT / "scripts" / (name + ".py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def load_policy(path, expected_digest):
    from r27_verified_coverage import _unique, _reject_nonfinite
    raw = Path(path).read_bytes()
    if (len(raw) > 65536 or not isinstance(expected_digest, str)
            or not DIGEST.fullmatch(expected_digest)
            or hashlib.sha256(raw).hexdigest() != expected_digest):
        raise EvidenceError("CI_POLICY: independently pinned policy required")
    policy = json.loads(raw, object_pairs_hook=_unique, parse_constant=_reject_nonfinite)
    if canonical(policy) + b"\n" != raw:
        raise EvidenceError("CI_POLICY: policy must be canonical JSON with newline")
    if (not isinstance(policy, dict) or set(policy) != {"schema", "repository", "catalog_release", "demo_sha", "prototype_paths", "checks"}
            or policy["schema"] != "baseharbor.ci-policy/v1"
            or policy["repository"] != "mcpdev80/baseharbor"
            or policy["catalog_release"] != "v0.4.26"
            or not isinstance(policy["checks"], dict) or not policy["checks"]
            or not isinstance(policy["prototype_paths"], list)
            or any(not isinstance(p, str) or "*" in p or p.startswith("/") or ".." in p.split("/")
                   for p in policy["prototype_paths"])):
        raise EvidenceError("CI_POLICY: invalid protected policy schema")
    for check, value in policy["checks"].items():
        if (not isinstance(check, str) or not check or not isinstance(value, dict)
                or set(value) != {"command", "inputs"}
                or not isinstance(value["command"], str) or not value["command"]
                or not isinstance(value["inputs"], list) or not value["inputs"]
                or any(not isinstance(p, str) or not p or p.startswith("/") or ".." in p.split("/")
                       for p in value["inputs"])):
            raise EvidenceError("CI_POLICY: invalid protected check definition")
    return policy


def make_plan(request, policy, *, repository, demo_repository=None):
    r = validate_request(request)
    resolver = owner("pr-diff-base")
    diff = resolver.resolve_pr_diff(repository, r["base"], r["candidate"],
                                    r["integration"] if r["stage"] == "pr" else None)
    if diff["broad_required"] or not diff["changes"]:
        raise EvidenceError("CI_IMPACT: unverifiable or empty candidate diff")
    paths = policy["prototype_paths"]
    if all(c.get("path") in paths and c.get("status") in ("added", "modified")
           for c in diff["changes"]):
        checks, gates = ["r27-contracts"], []
    else:
        planner = owner("pr-gate-plan")
        legacy = planner.plan(
            diff, json.loads((ROOT / "scripts/release-requirements.json").read_text()),
            json.loads((ROOT / "scripts/pr-gate-map-v1.json").read_text()),
            repository=repository, context_repository=policy["repository"],
            context_target_branch="develop")
        if legacy["coverage_status"] != "resolved":
            raise EvidenceError("CI_IMPACT: " + "; ".join(legacy["unresolved_coverage"]))
        checks = [c["id"] for c in legacy["required_validation_checks"]]
        gates = legacy["selected_gates"]
    if r["stage"] == "release":
        if "release-smoke" not in policy["checks"]:
            raise EvidenceError("CI_SMOKE: exact-artifact release smoke owner is not yet configured")
        checks = sorted(set(checks) | {"release-smoke"})
    if not set(checks) <= set(policy["checks"]):
        raise EvidenceError("CI_IMPACT: selected validation lacks a protected executor")
    native = owner("pr-selected-runtime-matrix").select(
        {"selected_gates": gates}, json.loads((ROOT / "scripts/release-requirements.json").read_text()))
    if native["unsupported"] or "integration/static/public-contracts" in gates:
        raise EvidenceError("CI_IMPACT: selected non-atomic gate requires its accepted executor")
    inputs = owner("pre-release-resume").GitInputs(repository, demo_repository or repository)
    objects = inputs.objects(Path(repository), r["integration"])
    coverage = {}
    for check in checks:
        config = policy["checks"][check]
        selected = [(path, meta) for path, meta in objects
                    if any(fnmatch.fnmatchcase(path, pattern) for pattern in config["inputs"])]
        if not selected:
            raise EvidenceError("CI_INPUT: required validation has no matching source")
        identity = {"command": config["command"], "objects": selected}
        if check == "release-smoke":
            identity.update(candidate=r["candidate"], artifact=r["artifact"])
        coverage["release/smoke" if check == "release-smoke" else "validation/" + check] = hashlib.sha256(canonical(identity)).hexdigest()
    if native["required"]:
        if demo_repository is None:
            raise EvidenceError("CI_INPUT: immutable Demo checkout required for native gates")
        for gate in gates:
            coverage[gate] = inputs.fingerprint(r["integration"], policy["demo_sha"],
                                                policy["catalog_release"], gate)
    if not coverage:
        raise EvidenceError("CI_IMPACT: no qualified checks selected")
    return {"request": r, "coverage": coverage, "checks": sorted(checks),
            "native": native["include"] if native["required"] else [],
            "integration_sha": diff.get("integration_sha"),
            "merge_base_sha": diff["merge_base_sha"]}


def expected_jobs(plan):
    jobs = {"R27 · Plan", "R27 · Finalcheck"}
    jobs.update("R27 · Validation · " + check for check in plan["checks"])
    jobs.update("R27 · Native / " + item["runtime"] + " · " + item["gate"]
                for item in plan["native"])
    return jobs
