"""Publication handoff accepts reviewed prose, never changed runtime inputs."""
import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest


def module():
    spec = importlib.util.spec_from_file_location('handoff', Path(__file__).with_name('release-handoff.py'))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


class WorkflowBinding(unittest.TestCase):
    def test_recovery_mapping_requires_exact_release_run_and_workflow(self):
        h = module()
        args = ('v0.4.24', h.V024_RECOVERY_WORKFLOW, h.V024_RECOVERY_RUN)
        self.assertEqual(h.candidate_for_workflow(*args), h.V024_CANDIDATE)
        for tag, head, run in [('v0.4.23', args[1], args[2]),
                               (args[0], 'f' * 40, args[2]),
                               (args[0], args[1], args[2] + 1)]:
            self.assertEqual(h.candidate_for_workflow(tag, head, run), head)


class PublicationInputs(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git('init', '-q')
        self.git('config', 'user.name', 'Test')
        self.git('config', 'user.email', 'test@example.invalid')
        self.h = module()
        for name in self.h.V024_VERIFIED_COLLECTOR:
            self.write(name, 'verified collector\n')
        self.write('internal/auth/verifier.go', 'approved product\n')
        self.candidate = self.commit()
        self.h.V024_CANDIDATE = self.candidate
        self.h.V024_VERIFIED_COLLECTOR = {
            name: self.git('rev-parse', self.candidate + ':' + name)
            for name in self.h.V024_VERIFIED_COLLECTOR}

    def git(self, *args):
        return subprocess.check_output(['git', *args], cwd=self.root, stderr=subprocess.DEVNULL).decode().strip()

    def write(self, path, content):
        p = self.root / path
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content)

    def commit(self):
        self.git('add', '.')
        self.git('commit', '-qm', 'fixture')
        return self.git('rev-parse', 'HEAD')

    def test_reviewed_english_and_german_prose_is_accepted(self):
        self.write('docs/de/releases/v0.4.24.md', 'translated highlights\n')
        self.write('docs/cli/core.md', 'published command guidance\n')
        self.write('CHANGELOG.md', 'release date\n')
        changed = self.h.validate_handoff(self.root, self.candidate, self.commit(), 'v0.4.24')
        self.assertEqual(set(changed), {'CHANGELOG.md', 'docs/cli/core.md', 'docs/de/releases/v0.4.24.md'})

    def test_runtime_auth_inventory_and_unreviewed_docs_are_rejected(self):
        for name in ['internal/auth/verifier.go', 'scripts/release-requirements.json',
                     '.github/workflows/pre-release.yml', 'docs/unreviewed.md']:
            with self.subTest(path=name):
                self.git('reset', '--hard', self.candidate)
                self.git('clean', '-fdq')
                self.write(name, 'changed input\n')
                with self.assertRaisesRegex(ValueError, 'execution inputs'):
                    self.h.validate_handoff(self.root, self.candidate, self.commit(), 'v0.4.24')

    def test_changed_verified_collector_is_rejected(self):
        self.write('scripts/pre-release-resume.py', 'unreviewed collector\n')
        with self.assertRaisesRegex(ValueError, 'verified recovery'):
            self.h.validate_handoff(self.root, self.candidate, self.commit(), 'v0.4.24')

    def test_changed_collector_mode_is_rejected(self):
        (self.root / 'scripts/pre-release-resume.py').chmod(0o755)
        with self.assertRaisesRegex(ValueError, 'verified recovery'):
            self.h.validate_handoff(self.root, self.candidate, self.commit(), 'v0.4.24')

    def test_deleted_collector_is_rejected(self):
        (self.root / 'scripts/pre-release-resume.py').unlink()
        with self.assertRaisesRegex(ValueError, 'verified recovery'):
            self.h.validate_handoff(self.root, self.candidate, self.commit(), 'v0.4.24')


if __name__ == '__main__':
    unittest.main()
