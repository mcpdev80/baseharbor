#!/usr/bin/env python3
"""Issue bounded CLI consent from the current Podman acceptance operation plan.

Only this local CI journey supplies acknowledgement of a freshly observed,
non-destructive provider impact. Runtime revalidation remains authoritative.
"""
import json
import sys
from pathlib import Path


def consent_from_plan(document):
    if not isinstance(document, dict):
        raise ValueError("operation plan must be an object")
    provider = document.get("provider_plan")
    if not isinstance(provider, dict):
        raise ValueError("provider_plan is missing")
    impact = provider.get("impact")
    if not isinstance(impact, dict):
        raise ValueError("provider impact is missing")
    if impact.get("version") != "baseharbor.provider-impact/v1":
        raise ValueError("unsupported provider-impact version")
    plan_id = impact.get("plan_id")
    if not isinstance(plan_id, str) or not plan_id.strip() or len(plan_id) > 512:
        raise ValueError("provider impact lacks a valid plan_id")
    stages = impact.get("confirmations_required")
    if type(stages) is not int or stages not in (1, 2):
        raise ValueError("invalid required confirmation stages")
    if type(impact.get("destructive")) is not bool:
        raise ValueError("destructive impact must be explicit")
    if impact["destructive"]:
        # This clean-state acceptance must never grant deletion by implication.
        # Separate destructive authorization belongs to its own reviewed test.
        raise ValueError("destructive provider impact requires separate authorization")
    return {"plan_id": plan_id, "first": True,
            "second": stages == 2, "delete_data": False}


def main(argv):
    if len(argv) != 2:
        raise ValueError("expected one freshly generated operation plan JSON file")
    data = json.loads(Path(argv[1]).read_text(encoding="utf-8"))
    print(json.dumps(consent_from_plan(data), separators=(",", ":"), sort_keys=True))


if __name__ == "__main__":
    try:
        main(sys.argv)
    except (ValueError, OSError, json.JSONDecodeError) as error:
        print("podman consent: " + str(error), file=sys.stderr)
        sys.exit(1)
