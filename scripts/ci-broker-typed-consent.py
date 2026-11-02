#!/usr/bin/env python3
"""Resolve narrowly scoped consent for an isolated broker acceptance plan."""
import importlib.util
import json
import sys
from pathlib import Path


spec = importlib.util.spec_from_file_location(
    "native_consent", Path(__file__).with_name("ci-podman-typed-consent.py"))
native_consent = importlib.util.module_from_spec(spec)
spec.loader.exec_module(native_consent)


def consent_from_plan(document):
    if not isinstance(document, dict) or not isinstance(document.get("provider_plan"), dict):
        raise ValueError("provider plan is missing")
    plan = document["provider_plan"]
    if plan.get("version") != "baseharbor.provider-plan/v1":
        raise ValueError("unsupported provider-plan version")
    if not isinstance(plan.get("target"), str) or not plan["target"].strip():
        raise ValueError("provider target is missing")
    if type(plan.get("complete")) is not bool:
        raise ValueError("provider completeness is missing")
    if not isinstance(plan.get("consumers"), dict):
        raise ValueError("provider consumers are missing")
    if not isinstance(plan.get("required"), list) or not all(isinstance(x, str) for x in plan["required"]):
        raise ValueError("provider requirements are invalid")
    if not isinstance(plan.get("released_bindings"), list):
        raise ValueError("provider released bindings are missing")
    changes = plan.get("changes")
    if not isinstance(changes, list):
        raise ValueError("provider changes are missing")
    if any(not isinstance(item, dict) or not isinstance(item.get("instance_id"), str)
           or not item["instance_id"] or item.get("action") not in
           {"reuse", "preserve", "add", "restart", "stop", "remove-runtime"}
           or not isinstance(item.get("reason"), str)
           or not isinstance(item.get("ownership"), str)
           or not isinstance(item.get("consumers"), list)
           or not all(isinstance(x, str) for x in item["consumers"])
           or type(item.get("preserve_data")) is not bool for item in changes):
        raise ValueError("provider change is invalid")
    if plan.get("impact") is not None:
        return native_consent.consent_from_plan(document, "provider-plan")
    return None


def main():
    if len(sys.argv) != 2:
        raise ValueError("expected a fresh provider plan file")
    document = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
    consent = consent_from_plan(document)
    print("none" if consent is None else json.dumps(consent, separators=(",", ":"), sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, json.JSONDecodeError) as error:
        print("broker consent: " + str(error), file=sys.stderr)
        sys.exit(1)
