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
    changes = plan.get("changes")
    if not isinstance(changes, list):
        raise ValueError("provider changes are missing")
    if "impact" in plan:
        return native_consent.consent_from_plan(document, "provider-plan")
    if plan.get("complete") is not True:
        raise ValueError("incomplete provider plan without impact")
    if any(not isinstance(item, dict) or item.get("action") not in {"reuse", "preserve"}
           for item in changes):
        raise ValueError("provider changes require an impact")
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
