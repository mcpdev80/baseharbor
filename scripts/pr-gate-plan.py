#!/usr/bin/env python3
"""Conservative v0.4.26 PR area-to-existing-gate selection (#892)."""
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent

DOC_CHECKS = [
    {'id': 'documentation-audit', 'owner': 'scripts/documentation-audit.sh',
     'command': 'bash scripts/documentation-audit.sh'},
    {'id': 'mkdocs-en-strict', 'owner': '.github/workflows/pages.yml:build',
     'command': 'mkdocs build --strict --config-file mkdocs.yml'},
    {'id': 'mkdocs-de-strict', 'owner': '.github/workflows/pages.yml:build',
     'command': 'mkdocs build --strict --config-file mkdocs.de.yml'},
    {'id': 'pages-candidate-build', 'owner': '.github/workflows/pages.yml:build',
     'command': 'build candidate documentation without deploying Pages'},
]


def validated_diff(diff):
    if not isinstance(diff, dict):
        raise ValueError('diff-base result must be an object')
    problems = []
    if type(diff.get('broad_required')) is not bool:
        problems.append('broad_required must be an explicit boolean')
    changes = diff.get('changes')
    if not isinstance(changes, list) or any(not isinstance(c, dict) for c in changes):
        problems.append('changes must be an array of objects')
    reasons = diff.get('reasons')
    if not isinstance(reasons, list) or any(
            not isinstance(r, dict) or not isinstance(r.get('code'), str) or
            not isinstance(r.get('message'), str) for r in reasons):
        problems.append('reasons must be an array of code/message objects')
    for key in ('target_base_sha', 'head_sha', 'merge_base_sha'):
        value = diff.get(key)
        if not (isinstance(value, str) and len(value) == 40 and
                all(c in '0123456789abcdef' for c in value)):
            problems.append(key + ' must be a full lowercase commit SHA')
    integration = diff.get('integration_sha')
    if integration is not None and not (
            isinstance(integration, str) and len(integration) == 40 and
            all(c in '0123456789abcdef' for c in integration)):
        problems.append('integration_sha must be null or a full lowercase commit SHA')
    # Resolver fallbacks may legitimately omit an unavailable SHA. Never
    # reject those or turn them into an eligible narrow plan.
    if problems:
        return False, problems
    return True, []



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
    valid, problems = validated_diff(diff)
    reasons = ['invalid diff-base input: ' + problem for problem in problems]
    broad = bool(diff.get('broad_required')) or not valid
    paths = []
    if broad:
        reasons.append('diff-base resolver requires broad checks: ' + json.dumps(diff.get('reasons', []), sort_keys=True))
    for change in (diff.get('changes', []) if valid else []):
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
    documentation_required = broad or 'prose' in areas
    return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
            'mapping_schema': mapping['schema'], 'release': release,
            'broad_required': broad, 'selected_gates': sorted(selected),
            'required_validation_checks': DOC_CHECKS if documentation_required else [],
            'validation_contract': 'required_validation_checks are mandatory named checks; #893 must enforce successful results independently of release gate IDs',
            'reasons': reasons}


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
