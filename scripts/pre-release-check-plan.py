#!/usr/bin/env python3
"""Select normal release checks from the candidate-owned catalog."""
import argparse
import json
from pathlib import Path

STATIC = ['config-matrix', 'init', 'mcp', 'agent', 'shell-ux']
LIGHT = ['guided', 'lifecycle', 'policy', 'connectivity', 'reconciliation', 'failure', 'full-destroy']
HEAVY = ['data-capabilities', 'observability', 'identity', 'security', 'backup-restore',
         'rabbitmq-runtime', 'mongodb-runtime', 'durable-key-value', 'control-plane-recovery',
         'loki-runtime', 'tempo-runtime']
ADOPTION = ['conformance', 'realworld-compose', 'realworld-quadlet', 'realworld-kubernetes']
HA = ['data', 'identity', 'observability', 'routing']
INTEGRATION = {'integration/static/public-contracts', 'integration/static/configuration-policy',
               'integration/static/browser-terminal', 'integration/static/live-console',
               *(f'integration/{runtime}/{name}' for runtime in ['docker', 'podman']
                 for name in ['core-bootstrap', 'coreless-lifecycle', 'remote-target'])}


def select(inventory, tag, requested=''):
    if not isinstance(inventory, dict) or tag not in inventory.get('releases', []):
        raise ValueError('release is absent from the candidate check catalog')
    rows = inventory.get('gates')
    if not isinstance(rows, list) or any(not isinstance(row, dict) for row in rows):
        raise ValueError('invalid check catalog')
    required = [row['id'] for row in rows if tag in row.get('required_for', [])]
    if not required or len(required) != len(set(required)):
        raise ValueError('empty or duplicate required checks')
    selected = required if not requested else requested.split(',')
    if (any(not key or key.strip() != key for key in selected) or
            len(selected) != len(set(selected)) or not set(selected) <= set(required)):
        raise ValueError('selected checks must be unique exact required check IDs')
    keys = set(selected)
    result = {'adoption': [g for g in ADOPTION if 'adoption/' + g in keys],
              'static': [g for g in STATIC if 'atomic/static/' + g in keys],
              'journey': [r for r in ['docker', 'podman'] if 'journey/' + r in keys],
              'integration': [key for key in selected if key in INTEGRATION]}
    handled = set(INTEGRATION)
    handled.update('adoption/' + g for g in ADOPTION)
    handled.update('atomic/static/' + g for g in STATIC)
    handled.update('journey/' + r for r in ['docker', 'podman'])
    for runtime in ['docker', 'podman']:
        for name, gates in [('light', LIGHT), ('heavy', HEAVY), ('ha', HA)]:
            prefix = 'ha' if name == 'ha' else 'atomic'
            result[name + '_' + runtime] = [g for g in gates if f'{prefix}/{runtime}/{g}' in keys]
            handled.update(f'{prefix}/{runtime}/{g}' for g in gates)
    if keys - handled:
        raise ValueError('selected check has no executor: ' + ', '.join(sorted(keys - handled)))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--only-gates', default='')
    args = parser.parse_args()
    inventory = json.loads(Path(__file__).with_name('release-requirements.json').read_text())
    for name, values in select(inventory, args.tag, args.only_gates).items():
        print(name + '=' + json.dumps(values, separators=(',', ':')))
        print(name + '_count=' + str(len(values)))


if __name__ == '__main__':
    main()
