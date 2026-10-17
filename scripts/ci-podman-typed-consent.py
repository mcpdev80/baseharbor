#!/usr/bin/env python3
"""Issue bounded CLI consent from the current Podman acceptance operation plan.

Only this local CI journey supplies acknowledgement of a freshly observed,
non-destructive provider impact. Runtime revalidation remains authoritative.
"""
import json
import sys
from pathlib import Path


SOURCES = {
    "provider-plan": ("provider_plan", "impact", "baseharbor.provider-impact/v1"),
    "impact": ("impact", None, "baseharbor.provider-impact/v1"),
    "scope-output": ("scope", None, "baseharbor.provider-impact-scope/v1"),
}


def _document_from_text(text, source):
    if source != "scope-output":
        return json.loads(text)
    start = text.find("{")
    if start < 0:
        raise ValueError("application stop did not emit a consequence scope")
    document, offset = json.JSONDecoder().raw_decode(text[start:])
    if text[start + offset:].strip():
        raise ValueError("unexpected output after application stop consequence scope")
    return document


def consent_from_plan(document, source="provider-plan"):
    path = SOURCES.get(source)
    if path is None:
        raise ValueError("unsupported operation plan source")
    if not isinstance(document, dict):
        raise ValueError("operation plan must be an object")
    first, second, version = path
    impact = document.get(first)
    if second is not None:
        if not isinstance(impact, dict):
            raise ValueError(first + " is missing")
        impact = impact.get(second)
    if not isinstance(impact, dict):
        raise ValueError("provider impact is missing")
    if impact.get("version") != version:
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
        raise ValueError("destructive provider impact requires separate authorization")
    return {"plan_id": plan_id, "first": True,
            "second": stages == 2, "delete_data": False}


def main(argv):
    if len(argv) != 3:
        raise ValueError("expected SOURCE and one freshly generated operation plan file")
    source = argv[1]
    if source not in SOURCES:
        raise ValueError("unsupported operation plan source")
    text = Path(argv[2]).read_text(encoding="utf-8")
    data = _document_from_text(text, source)
    print(json.dumps(consent_from_plan(data, source), separators=(",", ":"), sort_keys=True))


if __name__ == "__main__":
    try:
        main(sys.argv)
    except (ValueError, OSError, json.JSONDecodeError) as error:
        print("podman consent: " + str(error), file=sys.stderr)
        sys.exit(1)
