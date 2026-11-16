"""Native follow-ups accept advanced target history and reject foreign sources."""
import importlib.util
import subprocess
import tempfile
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location('runtime_source', Path(__file__).with_name('pr-runtime-source.py'))
source = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(source)


class RuntimeSourceTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.git('init', '-q')
        self.git('config', 'user.name', 'source fixture')
        self.git('config', 'user.email', 'fixture@example.test')
        self.git('config', 'commit.gpgsign', 'false')
        self.event = self.commit('event')
        self.base = self.commit('advanced base')
        self.head = self.commit('PR head')
        self.tip = self.commit('later target tip')
        self.git('update-ref', 'refs/heads/develop', self.tip)
        self.git('remote', 'add', 'origin', str(self.root))
        tree = self.git('rev-parse', 'HEAD^{tree}')
        self.candidate = self.git('commit-tree', tree, '-p', self.base, '-p', self.head, '-m', 'integration')
        self.git('checkout', '-q', '--detach', self.candidate)

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.root), *args], text=True, stderr=subprocess.DEVNULL).strip()

    def commit(self, text):
        self.git('commit', '-q', '--allow-empty', '-m', text)
        return self.git('rev-parse', 'HEAD')

    def verify(self, **overrides):
        values = dict(repository=self.root, event_base=self.event, base=self.base,
                      head=self.head, candidate=self.candidate)
        source.verify(**{**values, **overrides})

    def test_advanced_base_and_later_target_tip_are_valid(self):
        self.verify()
        self.verify(event_base=self.base)

    def test_foreign_stale_or_incomplete_inputs_reject(self):
        for values in ({'base': self.event}, {'head': self.tip}, {'candidate': self.head},
                       {'event_base': self.tip}, {'head': ''}):
            with self.subTest(values=values), self.assertRaises(ValueError):
                self.verify(**values)
        unrelated = self.git('commit-tree', self.git('rev-parse', 'HEAD^{tree}'), '-m', 'unrelated')
        with self.assertRaises(ValueError):
            self.verify(event_base=unrelated)

    def test_reversed_merge_parents_and_unknown_target_reject(self):
        candidate = self.git('commit-tree', self.git('rev-parse', 'HEAD^{tree}'),
                             '-p', self.head, '-p', self.base, '-m', 'reversed')
        self.git('checkout', '-q', '--detach', candidate)
        with self.assertRaises(ValueError):
            self.verify(candidate=candidate)
        self.git('update-ref', 'refs/heads/develop', self.event)
        self.git('checkout', '-q', '--detach', self.candidate)
        with self.assertRaises(ValueError):
            self.verify()

    def test_linear_candidate_is_bound_to_actual_head(self):
        self.git('checkout', '-q', '--detach', self.head)
        self.verify(candidate=self.head)
        with self.assertRaises(ValueError):
            self.verify(candidate=self.head, head=self.tip)


if __name__ == '__main__':
    unittest.main()
