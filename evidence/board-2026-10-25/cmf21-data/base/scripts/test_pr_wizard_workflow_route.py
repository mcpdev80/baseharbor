"""Exercise the exact wizard case from ci.yml with a real local Git trusted base."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
CI = ROOT / '.github/workflows/ci.yml'
HELPER = ROOT / 'scripts/pr-check-runner.py'


def run(*args, cwd=None, env=None):
    return subprocess.run(args, cwd=cwd, env=env, text=True,
                          stdout=subprocess.PIPE, stderr=subprocess.STDOUT)


class WizardWorkflowRoute(unittest.TestCase):
    def test_real_workflow_route_trusted_base_positive_and_missing_family(self):
        workflow = CI.read_text()
        start = workflow.index('            wizard-*-tests)') + len('            wizard-*-tests)\n')
        end = workflow.index('              ;;', start)
        fragment = workflow[start:end]
        self.assertIn('git show "$BASE_SHA":scripts/pr-check-runner.py', fragment)
        self.assertIn('git fetch --no-tags origin "$BASE_SHA"', fragment)
        self.assertIn('execute --check "$VALIDATION"', fragment)
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            trusted = root / 'trusted'
            candidate = root / 'candidate'
            temp = root / 'temp'
            for directory in (trusted, candidate, temp):
                directory.mkdir()
            self.assertEqual(0, run('git', 'init', '-q', str(trusted)).returncode)
            (trusted / 'scripts').mkdir()
            (trusted / 'scripts/pr-check-runner.py').write_text(HELPER.read_text())
            self.assertEqual(0, run('git', 'add', '.', cwd=trusted).returncode)
            envgit = dict(os.environ, GIT_AUTHOR_NAME='CI fixture',
                          GIT_AUTHOR_EMAIL='fixture@example.test',
                          GIT_COMMITTER_NAME='CI fixture',
                          GIT_COMMITTER_EMAIL='fixture@example.test')
            commit = run('git', 'commit', '-qm', 'trusted reviewed helper', cwd=trusted, env=envgit)
            self.assertEqual(0, commit.returncode, commit.stdout)
            base_sha = run('git', 'rev-parse', 'HEAD', cwd=trusted).stdout.strip()
            self.assertEqual(0, run('git', 'init', '-q', str(candidate)).returncode)
            self.assertEqual(0, run('git', 'remote', 'add', 'origin', str(trusted),
                                    cwd=candidate).returncode)
            (candidate / 'go.mod').write_text('module example.com/workflowroute\n\ngo 1.25\n')
            pkg = candidate / 'cmd/baha'
            pkg.mkdir(parents=True)
            environment = dict(os.environ, BASE_SHA=base_sha, VALIDATION='wizard-change-presentation-tests',
                               RUNNER_TEMP=str(temp), GITHUB_WORKSPACE=str(candidate))
            for missing in (False, True):
                names = ['TestAppChangePreviewPositive']
                if not missing:
                    names += ['TestAppChangeResultPresenterPositive']
                (pkg / 'wizard_test.go').write_text('package main\nimport "testing"\n' +
                    '\n'.join('func %s(t *testing.T) {}' % name for name in names) + '\n')
                execution = run('bash', '-euo', 'pipefail', '-c', fragment,
                                cwd=candidate, env=environment)
                if missing:
                    self.assertNotEqual(0, execution.returncode, execution.stdout)
                    self.assertIn('zero matching tests', execution.stdout)
                else:
                    self.assertEqual(0, execution.returncode, execution.stdout)
                    self.assertIn('TestAppChangeResultPresenterPositive', execution.stdout)


if __name__ == '__main__':
    unittest.main()
