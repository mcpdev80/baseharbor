#!/usr/bin/env python3
"""Validate the selected Docker and Podman journey evidence for one candidate."""
import argparse
import json
import pathlib


def verify(root, candidate, demo, run):
    manifests = sorted(root.glob('*/manifest.json'))
    if len(manifests) != 2:
        raise ValueError('exactly two runtime journey manifests are required')
    results = {}
    for path in manifests:
        if path.is_symlink() or path.parent.is_symlink():
            raise ValueError('journey evidence must not be a symlink')
        value = json.loads(path.read_text())
        runtime = value.get('runtime')
        if runtime not in ('docker', 'podman') or runtime in results:
            raise ValueError('each runtime must have exactly one journey manifest')
        expected = {'schema': 'baseharbor.pre-release.reference-journey/v1',
                    'candidate_sha': candidate, 'demo_ref': demo,
                    'workflow_run_id': run, 'outcome': 'success', 'cleanup_outcome': 'success'}
        if any(value.get(key) != item for key, item in expected.items()):
            raise ValueError(f'{runtime} journey failed or belongs to another candidate/demo/run')
        attempt = value.get('workflow_run_attempt')
        if type(attempt) is not int or attempt < 1:
            raise ValueError('journey attempt must be a positive integer')
        if path.parent.name != f'pre-release-reference-journey-evidence-{runtime}-{run}-{attempt}':
            raise ValueError('journey manifest attempt does not match its artifact identity')
        results[runtime] = value
    return [results[runtime] for runtime in ('docker', 'podman')]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', type=pathlib.Path)
    parser.add_argument('candidate')
    parser.add_argument('demo')
    parser.add_argument('run', type=int)
    parser.add_argument('output', type=pathlib.Path)
    args = parser.parse_args()
    try:
        values = verify(args.root, args.candidate, args.demo, args.run)
        args.output.write_text(json.dumps(values, indent=2) + '\n')
    except (ValueError, OSError, KeyError) as error:
        parser.exit(1, f'journey evidence verification failed: {error}\n')


if __name__ == '__main__':
    main()
