"""Independent table-driven expected gates and conservative failure fixtures."""
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('planner', ROOT / 'pr-gate-plan.py')
planner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(planner)
MAP = json.loads((ROOT / 'pr-gate-map-v1.json').read_text())
STATIC = ['atomic/static/config-matrix', 'atomic/static/init', 'atomic/static/mcp',
          'atomic/static/agent', 'atomic/static/shell-ux', 'integration/static/public-contracts',
          'integration/static/configuration-policy', 'integration/static/browser-terminal',
          'integration/static/live-console']
DOCKER = STATIC + ['atomic/docker/lifecycle', 'ha/docker/data', 'integration/docker/remote-target',
                   'adoption/conformance', 'adoption/realworld-compose', 'journey/docker']
ALL = sorted(set(DOCKER + ['atomic/podman/guided', 'journey/podman', 'integration/podman/core-bootstrap']))
REQ = {'releases': ['v0.4.26'], 'gates': [{'id': g, 'required_for': ['v0.4.26']} for g in ALL]}


def diff(changes, broad=False):
    return {'target_base_sha': 'a'*40, 'head_sha': 'b'*40, 'merge_base_sha': 'c'*40,
            'integration_sha': 'd'*40, 'broad_required': broad,
            'reasons': [{'code': 'conflict'}] if broad else [], 'changes': changes}


class MappingTests(unittest.TestCase):
    def test_independently_expected_table(self):
        cases = [
            ('prose', [{'status': 'modified', 'path': 'docs/how-to/x.md'}], STATIC, False),
            ('docker', [{'status': 'added', 'path': 'deploy/docker/service.yaml'}], DOCKER, False),
            ('cli', [{'status': 'modified', 'path': 'cmd/baha/root.go'}], ALL, True),
            ('shared-security', [{'status': 'modified', 'path': 'internal/operatorauth/policy.go'}], ALL, True),
            ('toolchain', [{'status': 'modified', 'path': 'go.mod'}], ALL, True),
            ('image', [{'status': 'modified', 'path': 'Dockerfile'}], ALL, True),
            ('workflow-policy', [{'status': 'modified', 'path': '.github/workflows/ci.yml'}], ALL, True),
            ('build-tags', [{'status': 'modified', 'path': 'internal/providers/docker/tagged_linux.go'}], ALL, True),
            ('test-imports', [{'status': 'modified', 'path': 'internal/runtime/core_test.go'}], ALL, True),
            ('embedded', [{'status': 'modified', 'path': 'internal/assets/embed.json'}], ALL, True),
            ('generated', [{'status': 'modified', 'path': 'spec/generated.pb.go'}], ALL, True),
            ('renamed-boundary', [{'status': 'renamed', 'old_path': 'docs/a.md', 'new_path': 'go.mod'}], ALL, True),
            ('deleted-unknown', [{'status': 'deleted', 'path': 'config/old.yaml'}], ALL, True),
            ('empty', [], ALL, True),
        ]
        for name, changes, expected, broad in cases:
            with self.subTest(name=name):
                result = planner.plan(diff(changes), REQ, MAP)
                self.assertEqual(sorted(expected), result['selected_gates'])
                self.assertEqual(broad, result['broad_required'])
                self.assertTrue(result['reasons'])
                self.assertEqual('a'*40, result['target_base_sha'])

    def test_diff_fallback_propagates(self):
        result = planner.plan(diff([{'status': 'modified', 'path': 'docs/a.md'}], True), REQ, MAP)
        self.assertTrue(result['broad_required'])
        self.assertEqual(ALL, result['selected_gates'])
        self.assertIn('conflict', str(result['reasons']))

    def test_unknown_status_and_bad_path(self):
        for change in ({'status': 'unmerged', 'path': 'docs/a.md'},
                       {'status': 'added', 'path': '../docs/a.md'}):
            with self.subTest(change=change):
                result = planner.plan(diff([change]), REQ, MAP)
                self.assertTrue(result['broad_required'])
                self.assertEqual(ALL, result['selected_gates'])

    def test_incomplete_diff_cannot_narrow(self):
        malformed = {'changes': [{'status': 'modified', 'path': 'docs/how-to/x.md'}]}
        result = planner.plan(malformed, REQ, MAP)
        self.assertTrue(result['broad_required'])
        self.assertEqual(ALL, result['selected_gates'])
        self.assertIn('broad_required', str(result['reasons']))

    def test_malformed_input_table(self):
        for mutation in [
            {'head_sha': 'abc'}, {'target_base_sha': 15},
            {'merge_base_sha': None}, {'integration_sha': False},
            {'broad_required': 'false'}, {'changes': 'not-an-array'},
            {'changes': [None]}, {'reasons': 'not-an-array'},
        ]:
            with self.subTest(mutation=mutation):
                payload = diff([{'status': 'modified', 'path': 'docs/a.md'}])
                payload.update(mutation)
                result = planner.plan(payload, REQ, MAP)
                self.assertTrue(result['broad_required'])
                self.assertEqual(ALL, result['selected_gates'])

    def test_legitimate_fallback_and_optional_integration(self):
        payload = diff([{'status': 'modified', 'path': 'docs/a.md'}], True)
        payload['target_base_sha'] = None
        payload['merge_base_sha'] = None
        payload['integration_sha'] = None
        self.assertTrue(planner.plan(payload, REQ, MAP)['broad_required'])
        payload = diff([{'status': 'modified', 'path': 'docs/a.md'}])
        payload['integration_sha'] = None
        self.assertFalse(planner.plan(payload, REQ, MAP)['broad_required'])

    def test_documentation_requires_existing_validation_owners(self):
        result = planner.plan(diff([{'status': 'modified', 'path': 'docs/how-to/x.md'}]), REQ, MAP)
        self.assertEqual({'documentation-audit', 'mkdocs-en-strict', 'mkdocs-de-strict',
                          'pages-candidate-build'},
                         {check['id'] for check in result['required_validation_checks']})
        self.assertTrue(all(check['owner'] and check['command'] for check in result['required_validation_checks']))
        self.assertIn('#893', result['validation_contract'])

    def test_actual_release_requirements_inventory(self):
        requirements = json.loads((ROOT / 'release-requirements.json').read_text())
        self.assertEqual('baseharbor.release-requirements/v1', requirements['schema'])
        self.assertEqual(65, len([g for g in requirements['gates'] if 'v0.4.26' in g['required_for']]))
        docs = planner.plan(diff([{'status': 'modified', 'path': 'docs/how-to/x.md'}]),
                            requirements, MAP)
        self.assertEqual(9, len(docs['selected_gates']))
        self.assertEqual(4, len(docs['required_validation_checks']))
        fallback = planner.plan(diff([{'status': 'modified', 'path': 'go.mod'}]),
                                requirements, MAP)
        self.assertTrue(fallback['broad_required'])
        self.assertEqual(65, len(fallback['selected_gates']))

    def test_catalog_mismatch_is_not_silent(self):
        mapping = json.loads(json.dumps(MAP))
        mapping['areas']['prose']['exact'].append('invented/gate')
        result = planner.plan(diff([{'status': 'added', 'path': 'docs/a.md'}]), REQ, mapping)
        self.assertTrue(result['broad_required'])
        self.assertEqual(ALL, result['selected_gates'])


if __name__ == '__main__':
    unittest.main()
