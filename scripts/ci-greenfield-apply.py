#!/usr/bin/env python3
"""Native Greenfield CI: bind only an explicitly reviewed, non-destructive plan.

This runs the real baha CLI. It does not impersonate a PASS, preseed data, or
turn generic --yes into consent. Every retry is bound to the same concrete
plan ID or immutable candidate digest returned by the product.
"""
import json
import os
import re
import subprocess
import sys


def impact_plan(output):
    decoder = json.JSONDecoder()
    for match in re.finditer(r"\{", output):
        try:
            candidate, _ = decoder.raw_decode(output[match.start():])
        except ValueError:
            continue
        if isinstance(candidate, dict) and candidate.get("version") == "baseharbor.provider-impact/v1":
            return candidate
    raise ValueError("required current typed provider impact plan missing")


def scoped_approval(output, target):
    plan = impact_plan(output)
    if plan.get("target") != target:
        raise ValueError("provider impact target differs from the isolated test target")
    if plan.get("destructive") is not False or plan.get("confirmations_required") != 2:
        raise ValueError("provider plan is destructive or lacks the required two acknowledgements")
    if not plan.get("impacts") or any(item.get("operation") != "add" for item in plan["impacts"]):
        raise ValueError("provider plan contains a non-add or missing operation")
    plan_id = plan.get("plan_id", "")
    if not re.fullmatch(r"[0-9a-f]{64}", plan_id):
        raise ValueError("provider plan has no valid immutable identifier")
    return json.dumps({"plan_id": plan_id, "first": True, "second": True, "delete_data": False})


def apply(cli, target):
    env = dict(os.environ)
    args = [cli, "up", "--yes"]
    approved, candidate_confirmed = False, False
    for attempt in range(3):
        completed = subprocess.run(args, env=env, text=True, capture_output=True, check=False)
        sys.stdout.write(completed.stdout)
        sys.stderr.write(completed.stderr)
        if completed.returncode == 0:
            return
        transcript = completed.stdout + "\n" + completed.stderr
        if "impact-confirmation-required" in transcript and not approved:
            env["BASEHARBOR_PROVIDER_CHANGE_CONSENT"] = scoped_approval(transcript, target)
            approved = True
            print("Greenfield CI: explicitly acknowledged the current non-destructive add-only provider plan")
            continue
        digest = re.search(r"(?m)^\s*confirm-hash\s+(\S+)\s*$", transcript)
        if digest and not candidate_confirmed:
            args += ["--confirm-candidate", digest.group(1)]
            candidate_confirmed = True
            print("Greenfield CI: confirming exactly the immutable candidate digest printed by baha")
            continue
        raise SystemExit("native baha up refused; no unscoped approval or fallback permitted")
    raise SystemExit("native baha up exceeded the bounded consent/candidate handshake")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: ci-greenfield-apply.py /absolute/path/to/baha")
    target_name = os.environ.get("BASEHARBOR_TARGET", "")
    if not re.fullmatch(r"gf-(docker|podman)-(go|nextjs|python|quarkus)", target_name):
        raise SystemExit("native Greenfield test target not explicitly scoped")
    apply(sys.argv[1], target_name)
