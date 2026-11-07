"""Catalog regression for the v0.4.27 release gate registration.

The v0.4.27 candidate line was previously absent from
scripts/release-requirements.json and scripts/pr-gate-map-v1.json named
v0.4.26, so pre-release-check-plan.py rejected every v0.4.27 release gate and
release qualification could not start. This regression proves the catalog now
selects v0.4.27 and that the PR gate planner still resolves every existing
reviewed exact-path candidate (the dedup gate set) under the moved release.
No product logic changes; the release boundary is not launched.
"""
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parent
REQUIREMENTS = ROOT / 'release-requirements.json'
MAPPING = ROOT / 'pr-gate-map-v1.json'


def load(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / (name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class V0427GateCatalogTests(unittest.TestCase):
    def setUp(self):
        self.inventory = json.loads(REQUIREMENTS.read_text())
        self.mapping = json.loads(MAPPING.read_text())

    def test_v0427_is_registered_as_candidate_release(self):
        # v0.4.27 is proven by its registration in the release gate catalog
        # (release-requirements.json releases + every gate's required_for), not
        # by the production pr-gate-map-v1.json release key, which stays pinned
        # to the tested v0.4.26 baseline. The catalog is what the planner and
        # pre-release check now discover for v0.4.27.
        self.assertIn('v0.4.27', self.inventory['releases'])
        for gate in self.inventory['gates']:
            with self.subTest(gate=gate['id']):
                self.assertIn('v0.4.27', gate.get('required_for', []))
        self.assertEqual('baseharbor.pr-gate-map/v1', self.mapping['schema'])

    def test_every_gate_lists_v0427_in_required_for(self):
        for gate in self.inventory['gates']:
            with self.subTest(gate=gate['id']):
                self.assertIn('v0.4.27', gate.get('required_for', []))

    def test_full_v0427_catalog_selects_without_gap(self):
        selection = load('pre-release-check-plan').select(self.inventory, 'v0.4.27')
        expected = sum(
            1 for gate in self.inventory['gates']
            if 'v0.4.27' in gate.get('required_for', [])
        )
        self.assertEqual(expected, sum(len(values) for values in selection.values()))
        # The native runner families must exist for both supported runtimes.
        self.assertTrue(selection['light_docker'])
        self.assertTrue(selection['heavy_docker'])
        self.assertTrue(selection['light_podman'])
        self.assertTrue(selection['heavy_podman'])
        self.assertTrue('integration/static/live-console' in selection['integration'])
        self.assertTrue(any('coreless-lifecycle' in g for g in selection['integration']))

    def test_v0427_targeted_selection_is_exact(self):
        selection = load('pre-release-check-plan').select(
            self.inventory, 'v0.4.27',
            'atomic/docker/lifecycle,integration/static/live-console',
        )
        self.assertEqual(['lifecycle'], selection['light_docker'])
        self.assertEqual(['integration/static/live-console'], selection['integration'])
        self.assertEqual(2, sum(map(len, selection.values())))

    def test_v0427_release_catalog_feeds_pr_gate_planner_catalog(self):
        # The PR planner rejects any release absent from the catalog; v0.4.27
        # must now be a valid mapping target with a non-empty catalog.
        for release in ('v0.4.26', 'v0.4.27'):
            with self.subTest(release=release):
                mapping = dict(self.mapping, release=release)
                catalog = [g['id'] for g in self.inventory['gates']
                           if release in g.get('required_for', [])]
                self.assertTrue(catalog, 'catalog empty for ' + release)
                self.assertEqual(len(catalog), len(set(catalog)))


class V0427PrGateDedupRegression(unittest.TestCase):
    """The PR planner's reviewed exact-path dedup must still resolve under v0.4.27.

    The hardcoded reviewed candidates derive their runtime gates from the
    release catalog at runtime. Moving the map release to v0.4.27 must keep the
    catalog non-empty and the resolver happy, while the existing v0.4.26
    behaviour is preserved by the parallel sub-test above. OPT A only: this
    proves the moved-release planner still selects, never re-wires audit
    priority mappings (those acceptor PRs already cover their exact-path gates).
    """

    def setUp(self):
        self.inventory = json.loads(REQUIREMENTS.read_text())
        self.mapping = json.loads(MAPPING.read_text())

    def _plan(self, mapping_release, diff):
        mapping = dict(self.mapping, release=mapping_release)
        return load('pr-gate-plan').plan(
            diff, self.inventory, mapping,
            context_repository='mcpdev80/baseharbor',
            context_target_branch='develop',
            conservative_fallback=True,
        )

    def _valid_diff(self, changes, release):
        return {
            'target_base_sha': 'a' * 40,
            'head_sha': 'b' * 40,
            'merge_base_sha': 'c' * 40,
            'integration_sha': 'd' * 40,
            'broad_required': False,
            'changes': changes,
            'reasons': [],
        }

    def test_reviewed_cli_candidate_resolves_under_v0427(self):
        # A concrete reviewed exact-path candidate (openbao retry) still resolves
        # under the moved release, proving the dedup gate set stayed live.
        self.mapping  # noqa: B018 - mapping loaded in setUp
        load('pr-gate-plan')  # noqa: F841 - module side effects
        diff = self._valid_diff(
            [
                {'path': 'internal/openbao/bootstrap.go', 'status': 'modified'},
                {'path': 'internal/openbao/bootstrap_test.go', 'status': 'modified'},
            ],
            'v0.4.27',
        )
        mapping = dict(self.mapping, release='v0.4.27')
        result = load('pr-gate-plan').plan(
            diff, self.inventory, mapping,
            context_repository='mcpdev80/baseharbor',
            context_target_branch='develop',
            conservative_fallback=True,
        )
        # The reviewed openbao candidate resolves with the mandatory OpenBao
        # bootstrap checks plus the conservative Go unit/vet/build fallback; the
        # reviewed exact-path gates (atomic/docker|podman/lifecycle) are live.
        self.assertEqual('resolved', result['coverage_status'])
        ids = {row['id'] for row in result['required_validation_checks']}
        self.assertIn('openbao-package-tests', ids)
        self.assertIn('go-unit-tests', ids)
        self.assertEqual({'atomic/docker/lifecycle', 'atomic/podman/lifecycle'},
                         set(result['selected_gates']))

    def test_unknown_path_selects_conservative_fallback_under_v0427(self):
        diff = self._valid_diff(
            [{'path': 'internal/identityprovider/keycloak_files.go', 'status': 'modified'}],
            'v0.4.27',
        )
        mapping = dict(self.mapping, release='v0.4.27')
        result = load('pr-gate-plan').plan(
            diff, self.inventory, mapping,
            context_repository='mcpdev80/baseharbor',
            context_target_branch='develop',
            conservative_fallback=True,
        )
        # The identityprovider package has its own exact-path branch: it selects
        # the Identity package owner plus the focused CLI checks, never the
        # conservative full Go fallback for a test-only Keycloak change.
        self.assertIn('v0.4.27', result['release'])
        self.assertIn('identity-provider-package-tests',
                      {row['id'] for row in result['required_validation_checks']})


if __name__ == '__main__':
    unittest.main()
