import copy
import unittest

from ecosystem_release_pins import resolve_pins


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


if __name__ == '__main__':
    unittest.main()
