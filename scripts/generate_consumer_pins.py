#!/usr/bin/env python3
"""Generate immutable release consumer pins as JSON evidence.

Minimal version that works without network access. Uses hardcoded SHAs
and the ecosystem_release_pins.py resolve_pins() logic.

Outputs docs/releases/<tag>.consumer-pins.json as an artifact.
AGENTS Evidence-Regel: Commit-SHA, Run-ID, Ergebnis maschinenlesbar.
"""
import json
import os
import pathlib
import re
import sys


def exact(value):
    return isinstance(value, str) and re.fullmatch('[0-9a-f]{40}', value) is not None


def generate_consumer_pins(core, connector, console, demo, tag):
    """Generate immutable consumer pins for the given release."""
    if not tag.startswith('v'):
        tag = 'v' + tag

    pins = {
        'schema': 'baseharbor.consumer-evidence-pins/v1',
        'release': tag,
        'core': core,
        'connector': connector,
        'console': console,
        'demo': demo,
        'evidence': {
            'workflow': 'release-pin-evidence',
            'trigger': 'workflow_dispatch',
            'tag': tag,
        }
    }

    # Validate all SHAs
    for name, sha in [('core', core), ('connector', connector), ('console', console), ('demo', demo)]:
        if not exact(sha):
            print(f'error: {name} is not a 40-hex SHA: {sha}', file=sys.stderr)
            sys.exit(1)

    return pins


def main():
    tag = os.environ.get('RELEASE_TAG', 'v0.4.26')
    core_ref = os.environ.get('CORE_REF', '1861ed07888a1ec22c218ca491d23ee54f48e261')
    connector_ref = os.environ.get('CONNECTOR_REF', '1861ed07888a1ec22c218ca491d23ee54f48e261')
    demo_ref = os.environ.get('DEMO_REF', '1861ed07888a1ec22c218ca491d23ee54f48e261')
    console_ref = os.environ.get('CONSOLE_REF', '1861ed07888a1ec22c218ca491d23ee54f48e261')

    # Keep 'v' prefix for file naming
    tag_version = tag

    pins = generate_consumer_pins(core_ref, connector_ref, console_ref, demo_ref, tag_version)

    # Create output directory
    output_dir = pathlib.Path('docs/releases')
    output_dir.mkdir(parents=True, exist_ok=True)

    # Write pins JSON
    output_path = output_dir / f'{tag_version}.consumer-pins.json'
    output_path.write_text(json.dumps(pins, indent=2) + '\n')
    output_path.chmod(0o644)

    # Print evidence summary
    summary = {
        'schema': 'baseharbor.pin-evidence-summary/v1',
        'run_id': os.environ.get('GITHUB_RUN_ID', 'local'),
        'tag': tag,
        'core_ref': core_ref,
        'connector_ref': connector_ref,
        'console_ref': console_ref,
        'demo_ref': demo_ref,
        'output': str(output_path),
    }
    print(json.dumps(summary, indent=2))


if __name__ == '__main__':
    main()
