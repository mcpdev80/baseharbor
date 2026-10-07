import copy
import json
import unittest

import private_evidence_access_preflight as preflight
from private_consumer_evidence import QUALIFICATIONS, REPOSITORIES, WORKFLOW


def pins():
    result = {}
    for gate in QUALIFICATIONS:
        roles = ['connector', 'console'] if gate.endswith('/live-console') else ['connector']
        result[gate] = {role: {'repository': REPOSITORIES[role],
                              'commit': ('a' if role == 'connector' else 'b') * 40,
                              'workflow': WORKFLOW} for role in roles}
    return result


class API:
    def __init__(self):
        self.calls = []
        self.fail = False

    def command(self, repository, path):
        self.calls.append((repository, path))
        if self.fail:
            raise ValueError('private-fixture-do-not-disclose')
        if path.startswith('commits/'):
            return json.dumps({'sha': path.split('/')[1]})
        return json.dumps({'workflow_runs': []})


class PrivateAccessTests(unittest.TestCase):
    def test_source_and_actions_read_access_do_not_approve_release(self):
        api = API()
        result = preflight.check_access(pins(), api)
        self.assertFalse(result['release_approved'])
        self.assertTrue(result['source_access'])
        self.assertEqual(len(api.calls), 4)
        self.assertNotIn('mcpdev80', json.dumps(result))

    def test_missing_foreign_or_invalid_bindings_fail_before_api_access(self):
        for kind in ['missing-gate', 'missing-role', 'foreign', 'mutable-ref', 'workflow']:
            with self.subTest(kind=kind):
                config = copy.deepcopy(pins())
                gate = 'integration/static/live-console'
                if kind == 'missing-gate':
                    config.pop(gate)
                elif kind == 'missing-role':
                    config[gate].pop('console')
                else:
                    field, value = {'foreign': ('repository', 'foreign/private'),
                                    'mutable-ref': ('commit', 'main'),
                                    'workflow': ('workflow', '.github/workflows/source.yml')}[kind]
                    config[gate]['console'][field] = value
                api = API()
                with self.assertRaises(ValueError):
                    preflight.check_access(config, api)
                self.assertEqual(api.calls, [])

    def test_access_denial_does_not_disclose_origin_diagnostics(self):
        api = API()
        api.fail = True
        with self.assertRaisesRegex(ValueError, '^private_source_or_actions_access_unavailable$'):
            preflight.check_access(pins(), api)


if __name__ == '__main__':
    unittest.main()
