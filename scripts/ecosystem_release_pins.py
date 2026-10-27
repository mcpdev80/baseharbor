#!/usr/bin/env python3
"""Resolve immutable release consumers without circular repository SHA pins."""
import argparse
import base64
import json
import pathlib
import re
import subprocess

from private_consumer_evidence import REPOSITORIES, WORKFLOW


def exact(value):
    return isinstance(value, str) and re.fullmatch('[0-9a-f]{40}', value) is not None


def resolve_pins(core, connector, candidate, console_candidate, demo_core, requested_demo='', release='0.4.24'):
    if (not exact(core) or not exact(connector) or not isinstance(candidate, dict) or
            set(candidate) != {'schema', 'release', 'core', 'console', 'demo'} or
            candidate['schema'] != 'baseharbor.ecosystem-candidate/v1' or
            release not in {'0.4.24', '0.4.25', '0.4.26'} or candidate['release'] != release or candidate['core'] != core or
            not all(exact(candidate[key]) for key in ['core', 'console', 'demo']) or
            not isinstance(console_candidate, dict) or
            set(console_candidate) != {'schema', 'release', 'core', 'demo'} or
            console_candidate != {key: candidate[key] for key in ['schema', 'release', 'core', 'demo']} or
            demo_core.strip() != core or (requested_demo and requested_demo != candidate['demo'])):
        raise ValueError('immutable Core/Connector/Console/Demo candidate differs')
    connector_pin = {'repository': REPOSITORIES['connector'], 'commit': connector, 'workflow': WORKFLOW}
    origin = {'repository': REPOSITORIES['connector'], 'commit': connector,
              'gate': 'integration/docker/remote-target'}
    pins = {'schema': 'baseharbor.consumer-evidence-pins/v1', 'gates': {
        'integration/docker/remote-target': {'connector': connector_pin},
        'integration/podman/remote-target': {'connector': connector_pin},
        'integration/static/live-console': {
            'connector': {**connector_pin, 'origin': origin},
            'console': {'repository': REPOSITORIES['console'], 'commit': candidate['console'],
                        'workflow': WORKFLOW, 'origin': origin},
        },
    }}
    if release == '0.4.26':
        # Acyclic, source-owned dependency graph. The published candidate is a
        # resolved immutable snapshot, not a reference to moving branches.
        pins['ecosystem'] = {
            'core': {'repository': 'mcpdev80/baseharbor', 'commit': core, 'consumes': {}},
            'demo': {'repository': 'mcpdev80/baseharbor-demo', 'commit': candidate['demo'],
                     'consumes': {'core': core}},
            'console': {'repository': REPOSITORIES['console'], 'commit': candidate['console'],
                        'consumes': {'core': core, 'demo': candidate['demo']}},
            'connector': {'repository': REPOSITORIES['connector'], 'commit': connector,
                          'consumes': {'core': core, 'demo': candidate['demo'],
                                       'console': candidate['console']}},
        }
        validate_ecosystem_graph(pins['ecosystem'])
    return pins


def validate_ecosystem_graph(ecosystem):
    """Require an exact four-repository DAG with immutable matching references."""
    expected = {'core': set(), 'demo': {'core'},
                'console': {'core', 'demo'},
                'connector': {'core', 'demo', 'console'}}
    repositories = {'core': 'mcpdev80/baseharbor', 'demo': 'mcpdev80/baseharbor-demo',
                    'console': REPOSITORIES['console'], 'connector': REPOSITORIES['connector']}
    if not isinstance(ecosystem, dict) or set(ecosystem) != set(expected):
        raise ValueError('ecosystem consumer graph differs')
    for role, dependencies in expected.items():
        node = ecosystem[role]
        if (not isinstance(node, dict) or set(node) != {'repository', 'commit', 'consumes'} or
                node['repository'] != repositories[role] or not exact(node['commit']) or
                not isinstance(node['consumes'], dict) or set(node['consumes']) != dependencies):
            raise ValueError('ecosystem consumer graph differs')
        for dependency, commit in node['consumes'].items():
            if not exact(commit) or commit != ecosystem[dependency]['commit']:
                raise ValueError('ecosystem consumer pin differs')
    visiting, visited = set(), set()

    def visit(role):
        if role in visiting:
            raise ValueError('cyclic ecosystem consumer dependency')
        if role in visited:
            return
        visiting.add(role)
        for dependency in ecosystem[role]['consumes']:
            visit(dependency)
        visiting.remove(role)
        visited.add(role)

    for role in expected:
        visit(role)


def api(path):
    return json.loads(subprocess.check_output(['gh', 'api', path], text=True))


def source_file(repository, commit, path):
    if repository not in REPOSITORIES.values() and repository != 'mcpdev80/baseharbor-demo':
        raise ValueError('unreviewed source repository')
    if not exact(commit):
        raise ValueError('immutable source commit required')
    reply = api(f'repos/{repository}/contents/{path}?ref={commit}')
    if reply.get('type') != 'file' or reply.get('encoding') != 'base64' or reply.get('size', 0) > 65536:
        raise ValueError('candidate source is not a bounded file')
    return base64.b64decode(reply['content']).decode()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--core', required=True)
    parser.add_argument('--tag', default='v0.4.24', choices=['v0.4.24', 'v0.4.25', 'v0.4.26'])
    parser.add_argument('--connector', default='')
    parser.add_argument('--demo', default='')
    parser.add_argument('--output', required=True, type=pathlib.Path)
    args = parser.parse_args()
    if not exact(args.core):
        raise ValueError('immutable Core commit required')
    connector = args.connector
    if not connector:
        branch = 'work/' + args.tag + '-integrations-docs'
        ref = api(f'repos/{REPOSITORIES["connector"]}/git/ref/heads/{branch}')
        if ref.get('ref') != 'refs/heads/' + branch or ref.get('object', {}).get('type') != 'commit':
            raise ValueError('reviewed Connector candidate branch differs')
        connector = ref['object']['sha']
    candidate = json.loads(source_file(REPOSITORIES['connector'], connector, 'integration-candidate.json'))
    console_candidate = json.loads(source_file(REPOSITORIES['console'], candidate['console'], 'integration-candidate.json'))
    demo_core = source_file('mcpdev80/baseharbor-demo', candidate['demo'], 'baseharbor-core.ref')
    pins = resolve_pins(args.core, connector, candidate, console_candidate, demo_core, args.demo, args.tag[1:])
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(pins, indent=2) + '\n')
    args.output.chmod(0o644)
    print(json.dumps({'connector_ref': connector, 'console_ref': candidate['console'],
                      'demo_ref': candidate['demo'], 'core_ref': args.core}))


if __name__ == '__main__':
    main()
