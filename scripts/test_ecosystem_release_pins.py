import copy
import unittest

from ecosystem_release_pins import resolve_pins, validate_ecosystem_graph


class EcosystemPinsTests(unittest.TestCase):
    def setUp(self):
        self.core, self.connector, self.console, self.demo = (letter * 40 for letter in 'abcd')
        self.candidate = {'schema': 'baseharbor.ecosystem-candidate/v1', 'release': '0.4.24',
                          'core': self.core, 'console': self.console, 'demo': self.demo}
        self.console_candidate = {key: self.candidate[key] for key in ['schema', 'release', 'core', 'demo']}

    def resolve(self, candidate=None, console=None, demo_core=None, requested=''):
        return resolve_pins(self.core, self.connector, candidate or self.candidate,
                            console or self.console_candidate, demo_core or self.core, requested)

    def test_one_candidate_binds_all_consumers_and_reuses_the_complete_browser_origin(self):
        pins = self.resolve()
        self.assertEqual(len(pins['gates']), 3)
        joint = pins['gates']['integration/static/live-console']
        self.assertEqual(joint['console']['commit'], self.console)
        self.assertEqual(joint['connector']['commit'], self.connector)
        self.assertEqual(joint['console']['origin'], joint['connector']['origin'])
        self.assertEqual(joint['console']['origin']['gate'], 'integration/docker/remote-target')

    def test_v0425_requires_matching_consumer_release_and_exact_core(self):
        candidate = {**self.candidate, 'release': '0.4.25'}
        console = {**self.console_candidate, 'release': '0.4.25'}
        pins = resolve_pins(self.core, self.connector, candidate, console,
                            self.core, release='0.4.25')
        self.assertEqual(len(pins['gates']), 3)
        with self.assertRaises(ValueError):
            resolve_pins(self.core, self.connector, candidate,
                         self.console_candidate, self.core, release='0.4.25')
        with self.assertRaises(ValueError):
            resolve_pins(self.core, self.connector, candidate, console,
                         self.core, release='0.4.26')

    def test_mixed_core_demo_console_or_release_cannot_generate_source_pins(self):
        for field in ['core', 'console', 'demo', 'release', 'schema']:
            candidate = copy.deepcopy(self.candidate)
            candidate[field] = 'work/mutable-branch' if field == 'console' else 'e' * 40
            with self.subTest(candidate_field=field), self.assertRaises(ValueError):
                self.resolve(candidate=candidate)
        for field in ['core', 'demo', 'release', 'schema']:
            console = copy.deepcopy(self.console_candidate)
            console[field] = 'e' * 40
            with self.subTest(console_field=field), self.assertRaises(ValueError):
                self.resolve(console=console)
        with self.assertRaises(ValueError):
            self.resolve(demo_core='e' * 40)
        with self.assertRaises(ValueError):
            self.resolve(requested='e' * 40)

    def test_v0426_complete_immutable_consumer_graph(self):
        candidate = {**self.candidate, 'release': '0.4.26'}
        console = {**self.console_candidate, 'release': '0.4.26'}
        pins = resolve_pins(self.core, self.connector, candidate, console,
                            self.core, release='0.4.26')
        graph = pins['ecosystem']
        self.assertEqual(set(graph), {'core', 'demo', 'console', 'connector'})
        for role, sha in [('core', self.core), ('demo', self.demo),
                          ('console', self.console), ('connector', self.connector)]:
            self.assertEqual(graph[role]['commit'], sha)
        self.assertEqual(graph['connector']['consumes'],
                         {'core': self.core, 'demo': self.demo, 'console': self.console})
        self.assertEqual(graph['console']['consumes'],
                         {'core': self.core, 'demo': self.demo})
        self.assertEqual(graph['demo']['consumes'], {'core': self.core})
        self.assertEqual(graph['core']['consumes'], {})
        self.assertEqual(pins['gates']['integration/static/live-console']['console']['commit'],
                         self.console)

    def test_v0426_rejects_mutable_missing_mismatched_and_cyclic_consumer_pins(self):
        candidate = {**self.candidate, 'release': '0.4.26'}
        console = {**self.console_candidate, 'release': '0.4.26'}
        valid = resolve_pins(self.core, self.connector, candidate, console,
                             self.core, release='0.4.26')['ecosystem']
        for role in valid:
            for field in ('commit', 'repository'):
                corrupted = copy.deepcopy(valid)
                corrupted[role][field] = '' if field == 'commit' else 'wrong/repo'
                with self.subTest(role=role, field=field), self.assertRaises(ValueError):
                    validate_ecosystem_graph(corrupted)
        for role, dependency in [('demo', 'core'), ('console', 'demo'),
                                 ('connector', 'console')]:
            for broken in ('main', 'f' * 40, ''):
                corrupted = copy.deepcopy(valid)
                corrupted[role]['consumes'][dependency] = broken
                with self.subTest(role=role, dependency=dependency, broken=broken), self.assertRaises(ValueError):
                    validate_ecosystem_graph(corrupted)
        cyclic = copy.deepcopy(valid)
        cyclic['core']['consumes'] = {'connector': self.connector}
        with self.assertRaises(ValueError):
            validate_ecosystem_graph(cyclic)
        for broken in ({**candidate, 'demo': 'main'}, {**candidate, 'console': ''},
                       {**candidate, 'core': 'f' * 40}, {**candidate, 'release': '0.4.25'}):
            with self.assertRaises(ValueError):
                resolve_pins(self.core, self.connector, broken, console,
                             self.core, release='0.4.26')
        with self.assertRaises(ValueError):
            resolve_pins(self.core, 'main', candidate, console, self.core, release='0.4.26')
        with self.assertRaises(ValueError):
            resolve_pins(self.core, self.connector, candidate, console,
                         'f' * 40, release='0.4.26')

    def test_historical_release_output_is_unchanged(self):
        self.assertNotIn('ecosystem', self.resolve())
        candidate = {**self.candidate, 'release': '0.4.25'}
        console = {**self.console_candidate, 'release': '0.4.25'}
        self.assertNotIn('ecosystem', resolve_pins(self.core, self.connector, candidate,
                                                   console, self.core, release='0.4.25'))


if __name__ == '__main__':
    unittest.main()
