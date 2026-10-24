"""Bind a dispatched candidate to a pinned executor and fresh GitHub state.

The run name is emitted by the protected workflow, never consumed as a shell
command. Credentials and executor pin are supplied only by its protected caller.
"""
from r27_verified_coverage import EvidenceError, SHA, DIGEST

PREFIX = "r27"
WORKFLOW = ".github/workflows/r27-executor.yml"
FIELDS = {"stage", "number", "candidate", "base", "integration", "artifact"}


def validate_request(request):
    if not isinstance(request, dict) or set(request) != FIELDS:
        raise EvidenceError("CI_REQUEST: invalid candidate fields")
    stage = request["stage"]
    if stage not in ("pr", "integration", "release"):
        raise EvidenceError("CI_REQUEST: unsupported stage")
    if any(not isinstance(request[k], str) or not SHA.fullmatch(request[k])
           for k in ("candidate", "base", "integration")):
        raise EvidenceError("CI_REQUEST: exact source and base SHAs required")
    number = request["number"]
    if type(number) is not int or not 0 <= number <= 2147483647:
        raise EvidenceError("CI_REQUEST: invalid PR number")
    if (stage == "pr") != (number > 0):
        raise EvidenceError("CI_REQUEST: PR scope mismatch")
    if stage != "pr" and request["integration"] != request["candidate"]:
        raise EvidenceError("CI_REQUEST: non-PR execution must use exact candidate")
    artifact = request["artifact"]
    if (stage == "release" and (not isinstance(artifact, str) or not DIGEST.fullmatch(artifact))
            or stage != "release" and artifact != "none"):
        raise EvidenceError("CI_REQUEST: exact release artifact digest required")
    return dict(request)


def run_name(request):
    r = validate_request(request)
    return "|".join((PREFIX, r["stage"], str(r["number"]), r["candidate"], r["base"], r["integration"], r["artifact"]))


def request_from_name(name):
    if not isinstance(name, str) or len(name) > 255:
        raise EvidenceError("CI_REQUEST: invalid run name")
    parts = name.split("|")
    if len(parts) != 7 or parts[0] != PREFIX or not parts[2].isascii() or not parts[2].isdigit():
        raise EvidenceError("CI_REQUEST: missing protected request binding")
    request = validate_request(dict(stage=parts[1], number=int(parts[2]),
                                    candidate=parts[3], base=parts[4], integration=parts[5], artifact=parts[6]))
    if run_name(request) != name:
        raise EvidenceError("CI_REQUEST: noncanonical run name")
    return request


def check_current(request, get_json, repository):
    """Recheck immediately before dispatch and after completed executor jobs."""
    r = validate_request(request)
    if r["stage"] == "pr":
        pr = get_json(f"repos/{repository}/pulls/{r['number']}")
        if (not isinstance(pr, dict) or type(pr.get("number")) is not int
                or pr["number"] != r["number"] or pr.get("state") != "open"
                or pr.get("draft") is not False):
            raise EvidenceError("CI_STALE: PR is not open and ready")
        for field, expected in (("head", r["candidate"]), ("base", r["base"])):
            obj = pr.get(field)
            if (not isinstance(obj, dict) or obj.get("sha") != expected
                    or not isinstance(obj.get("repo"), dict)
                    or obj["repo"].get("full_name") != repository):
                raise EvidenceError("CI_STALE: PR head/base identity changed")
        if pr["base"].get("ref") != "develop":
            raise EvidenceError("CI_REQUEST: PR target must be develop")
        if pr.get("merge_commit_sha") != r["integration"]:
            raise EvidenceError("CI_STALE: PR integration tree changed")
    else:
        ref = get_json(f"repos/{repository}/git/ref/heads/develop")
        if (not isinstance(ref, dict) or ref.get("ref") != "refs/heads/develop"
                or not isinstance(ref.get("object"), dict)
                or ref["object"].get("type") != "commit"
                or ref["object"].get("sha") != r["candidate"]):
            raise EvidenceError("CI_STALE: integration/release candidate changed")
    return r


def verify_executor_run(run, *, executor_sha, repository, run_id, attempt):
    if (not isinstance(executor_sha, str) or not SHA.fullmatch(executor_sha)
            or any(type(n) is not int or n < 1 for n in (run_id, attempt))):
        raise EvidenceError("CI_TRUST: invalid protected executor identity")
    if (not isinstance(run, dict) or type(run.get("id")) is not int
            or run["id"] != run_id or type(run.get("run_attempt")) is not int
            or run["run_attempt"] != attempt or run.get("head_sha") != executor_sha
            or run.get("path") != WORKFLOW or run.get("event") != "workflow_dispatch"
            or run.get("status") != "completed" or run.get("conclusion") != "success"
            or not isinstance(run.get("repository"), dict)
            or run["repository"].get("full_name") != repository):
        raise EvidenceError("CI_TRUST: executor source, identity or outcome mismatch")
    return request_from_name(run.get("display_title"))
