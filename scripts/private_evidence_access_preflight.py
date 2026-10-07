#!/usr/bin/env python3
"""Check protected private source/Actions read access without issuing approval."""
import json
import re

from private_consumer_evidence import (QUALIFICATIONS, REPOSITORIES, WORKFLOW,
                                      private_verifier_from_environment)


def check_access(pins, api):
    if set(pins) != set(QUALIFICATIONS):
        raise ValueError('private_configuration_roles_differ')
    sources = set()
    for gate, binding in pins.items():
        roles = {'connector', 'console'} if gate.endswith('/live-console') else {'connector'}
        if not isinstance(binding, dict) or set(binding) != roles:
            raise ValueError('private_configuration_roles_differ')
        for role, pin in binding.items():
            if (not isinstance(pin, dict) or set(pin) != {'repository', 'commit', 'workflow'} or
                    pin.get('repository') != REPOSITORIES[role] or pin.get('workflow') != WORKFLOW or
                    not isinstance(pin.get('commit'), str) or not re.fullmatch('[0-9a-f]{40}', pin['commit'])):
                raise ValueError('private_configuration_source_binding_invalid')
            sources.add((pin['repository'], pin['commit']))
    # Validate every binding before contacting any origin. Diagnostics never
    # contain the private source identity, API response or authentication token.
    try:
        for repository, commit in sorted(sources):
            source = json.loads(api.command(repository, 'commits/' + commit))
            if source.get('sha') != commit:
                raise ValueError()
            runs = json.loads(api.command(repository, 'actions/runs?per_page=1'))
            if not isinstance(runs.get('workflow_runs'), list):
                raise ValueError()
    except Exception:
        raise ValueError('private_source_or_actions_access_unavailable') from None
    return {'schema': 'baseharbor.private-access-preflight/v1',
            'source_access': True, 'actions_access': True, 'release_approved': False}


def main():
    try:
        verifier = private_verifier_from_environment(lambda *_: None)
        if verifier is None:
            raise ValueError('private_configuration_missing')
        result = check_access(verifier.pins, verifier.api)
    except Exception:
        # The safe result is deliberately not a qualification/approval receipt.
        raise SystemExit('Private evidence access preflight failed; protected configuration and source/Actions read access are required.') from None
    print(json.dumps(result, sort_keys=True))


if __name__ == '__main__':
    main()
