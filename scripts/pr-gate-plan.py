#!/usr/bin/env python3
"""Conservative v0.4.26 PR area-to-existing-gate selection (#892)."""
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent


def plan(diff, requirements, mapping):
    if mapping.get('schema') != 'baseharbor.pr-gate-map/v1':
        raise ValueError('unsupported mapping schema')
    release = mapping['release']
    if release not in requirements.get('releases', []):
        raise ValueError('release absent from gate catalog')
    gates = [g['id'] for g in requirements['gates'] if release in g['required_for']]
    if len(set(gates)) != len(gates) or not gates:
        raise ValueError('duplicate or empty gate catalog')
    all_gates = set(gates)
    identifiers = {key: diff.get(key) for key in ('target_base_sha', 'head_sha', 'merge_base_sha', 'integration_sha')}
    reasons = []
    broad = bool(diff.get('broad_required'))
    paths = []
    if broad:
        reasons.append('diff-base resolver requires broad checks: ' + json.dumps(diff.get('reasons', []), sort_keys=True))
    for change in diff.get('changes', []):
        status = change.get('status')
        if status in ('renamed', 'copied'):
            names = [change.get('old_path'), change.get('new_path')]
        elif status in ('added', 'deleted', 'modified', 'type_changed'):
            names = [change.get('path')]
        else:
            broad = True
            reasons.append('unrecognized change status: ' + repr(status))
            names = []
        if any(not isinstance(n, str) or not n or n.startswith('/') or '..' in n.split('/') for n in names):
            broad = True
            reasons.append('invalid changed path')
        paths.extend(n for n in names if isinstance(n, str))
    if not paths and not broad:
        broad = True
        reasons.append('empty or unverified changes cannot exempt tests')
    areas = set()
    for path in paths:
        # These exact prefixes are deliberately limited; anything with uncertain
        # transitive effects, build tags, test imports, generated/embed input,
        # OS/arch or downstream consumers needs the full existing catalog.
        if path.startswith('docs/') and path.endswith('.md') and not path.startswith(('docs/spec/', 'docs/internal/')):
            area = 'prose'
        elif path.startswith('deploy/docker/') and path.endswith(('.yaml', '.yml', '.sh')):
            area = 'docker'
        else:
            area = 'full'
        areas.add(area)
        reasons.append(path + ': ' + area)
        if area == 'full':
            broad = True
    if broad:
        selected = all_gates
        reasons.append('conservative full-catalog fallback: unknown/shared/toolchain/security/consumer impact')
    else:
        selected = set()
        for area in areas:
            definition = mapping['areas'][area]
            selected.update(g for g in gates if any(g.startswith(p) for p in definition['prefixes']))
            selected.update(definition['exact'])
        if not selected or not selected <= all_gates:
            selected = all_gates
            broad = True
            reasons.append('empty or unrecognized mapping: full-catalog fallback')
    return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
            'mapping_schema': mapping['schema'], 'release': release,
            'broad_required': broad, 'selected_gates': sorted(selected), 'reasons': reasons}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--diff', type=Path, required=True, help='JSON output from scripts/pr-diff-base.py')
    parser.add_argument('--requirements', type=Path, default=ROOT / 'release-requirements.json')
    parser.add_argument('--mapping', type=Path, default=ROOT / 'pr-gate-map-v1.json')
    args = parser.parse_args()
    result = plan(json.loads(args.diff.read_text()), json.loads(args.requirements.read_text()),
                  json.loads(args.mapping.read_text()))
    print(json.dumps(result, indent=2, sort_keys=True))


if __name__ == '__main__':
    main()
