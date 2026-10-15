#!/usr/bin/env python3
"""Narrow, fail-closed PR runtime request contract for #896.

This produces requests, not authority to execute untrusted code or accept evidence.
"""
import json
import re

SHA = re.compile(r'^[0-9a-f]{40}$')
RUNTIMES = {'docker', 'podman'}


def requests(plan, requirements, pins):
    if plan.get('schema') != 'baseharbor.pr-gate-plan/v1':
        raise ValueError('invalid trusted plan')
    sha_fields = ('target_base_sha', 'head_sha', 'merge_base_sha')
    if any(not SHA.fullmatch(str(plan.get(field, ''))) for field in sha_fields):
        raise ValueError('immutable plan SHA required')
    integration = plan.get('integration_sha')
    if not isinstance(integration, str) or not SHA.fullmatch(integration):
        raise ValueError('exact integration commit required for runtime admission')
    if not isinstance(pins, dict) or not all(
            SHA.fullmatch(str(pins.get(name, '')))
            for name in ('demo_sha', 'console_sha', 'connector_sha')):
        raise ValueError('exact verified consumer pins required')
    selected = plan.get('selected_gates')
    if not isinstance(selected, list) or not selected or len(selected) != len(set(selected)):
        raise ValueError('nonempty unique gate selection required')
    catalog = {entry['id'] for entry in requirements.get('gates', [])
               if 'v0.4.26' in entry.get('required_for', [])}
    if not set(selected) <= catalog:
        raise ValueError('unrecognized gate in selection')
    result = []
    for gate in sorted(selected):
        parts = gate.split('/')
        if len(parts) < 3 or parts[1] not in RUNTIMES:
            continue
        result.append({
            'schema': 'baseharbor.pr-runtime-request/v1',
            'gate_id': gate,
            'runtime': parts[1],
            'target_base_sha': plan['target_base_sha'],
            'head_sha': plan['head_sha'],
            'merge_base_sha': plan['merge_base_sha'],
            'integration_sha': integration,
            'consumer_pins': {key: pins[key] for key in ('demo_sha', 'console_sha', 'connector_sha')},
            'admission': 'requires-trusted-isolated-runner',
            'evidence': 'existing-verifier-only',
        })
    return result


def main():
    import argparse
    from pathlib import Path
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--plan', type=Path, required=True)
    p.add_argument('--requirements', type=Path, required=True)
    p.add_argument('--pins', type=Path, required=True)
    args = p.parse_args()
    print(json.dumps(requests(*[json.loads(x.read_text()) for x in
                                (args.plan, args.requirements, args.pins)]), indent=2))


if __name__ == '__main__':
    main()
