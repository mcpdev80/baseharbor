#!/usr/bin/env python3
"""Check protected private source/Actions read access without issuing approval."""
import json
import re
import hashlib

from private_consumer_evidence import (QUALIFICATIONS, REPOSITORIES, WORKFLOW,
                                      private_verifier_from_environment, source_origin)


def check_access(pins, api):
    if set(pins) != set(QUALIFICATIONS):
        raise ValueError('private_configuration_roles_differ')
    sources = {}
    for gate, binding in pins.items():
        roles = {'connector', 'console'} if gate.endswith('/live-console') else {'connector'}
        if not isinstance(binding, dict) or set(binding) != roles:
            raise ValueError('private_configuration_roles_differ')
        for role, pin in binding.items():
            try:
                origin_repository, origin_commit, _, _ = source_origin(role, gate, pin)
            except (ValueError, KeyError, TypeError):
                raise ValueError('private_configuration_source_binding_invalid')
            key = (pin['repository'], pin['commit'])
            sources[key] = sources.get(key, False) or 'origin' not in pin
            sources[(origin_repository, origin_commit)] = True
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
            if getattr(api, 'public', False) and sources[(repository, commit)]:
                artifacts = api.pages(repository, 'actions/artifacts?per_page=100', 'artifacts')
                exact = [item for item in artifacts if item.get('expired') is False and
                         item.get('workflow_run', {}).get('head_sha') == commit]
                if not exact:
                    raise ValueError()
                artifact = max(exact, key=lambda item: item['id'])
                archive = api.archive(repository, artifact)
                if (not archive.startswith(b'PK') or
                        artifact.get('digest') != 'sha256:' + hashlib.sha256(archive).hexdigest()):
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
