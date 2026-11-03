"""Immutable consumer pins remain source data, without CI origin lookup."""
import json
import os
from pathlib import Path
import tempfile
import unittest

from consumer_source_pins import REPOSITORIES, WORKFLOW, load_source_pins, source_origin


class ConsumerSourcePinsTests(unittest.TestCase):
    def test_exact_source_and_shared_origin_binding(self):
        pin = {'repository': REPOSITORIES['connector'], 'commit': 'a' * 40,
               'workflow': WORKFLOW}
        self.assertEqual((REPOSITORIES['connector'], 'a' * 40,
                          'integration/docker/remote-target', 'connector'),
                         source_origin('connector', 'integration/docker/remote-target', pin))
        pin['origin'] = {'repository': REPOSITORIES['connector'], 'commit': 'a' * 40,
                         'gate': 'integration/docker/remote-target'}
        self.assertEqual('connector', source_origin('connector', 'integration/static/live-console', pin)[3])
        pin['origin']['commit'] = 'b' * 40
        with self.assertRaises(ValueError):
            source_origin('connector', 'integration/static/live-console', pin)

    def test_mutable_foreign_or_unknown_source_is_rejected(self):
        pin = {'repository': REPOSITORIES['console'], 'commit': 'a' * 40, 'workflow': WORKFLOW}
        for role, field, value in [('unknown', 'commit', 'a' * 40),
                                   ('console', 'commit', 'main'),
                                   ('console', 'repository', 'someone/else'),
                                   ('console', 'workflow', 'unrelated.yml')]:
            with self.subTest(field=field, role=role), self.assertRaises(ValueError):
                source_origin(role, 'integration/static/live-console', dict(pin, **{field: value}))

    def test_file_permissions_symlinks_and_legacy_source_schema(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / 'pins.json'
            path.write_text(json.dumps({'schema': 'baseharbor.consumer-evidence-pins/v1', 'gates': {}}))
            os.chmod(path, 0o644)
            self.assertEqual({}, load_source_pins(path, public=True))
            with self.assertRaises(ValueError):
                load_source_pins(path)
            link = Path(temp) / 'link.json'
            link.symlink_to(path)
            with self.assertRaises(OSError):
                load_source_pins(link, public=True)
            path.write_text(json.dumps({'schema': 'baseharbor.private-evidence-pins/v1', 'gates': {}}))
            os.chmod(path, 0o600)
            self.assertEqual({}, load_source_pins(path))


if __name__ == '__main__':
    unittest.main()
