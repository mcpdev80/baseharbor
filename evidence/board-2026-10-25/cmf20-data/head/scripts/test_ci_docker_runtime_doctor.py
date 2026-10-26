"""Exercise the actual Docker workflow's structured Doctor prerequisite checks."""
import copy
import json
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest


WORKFLOW = Path(__file__).resolve().parent.parent / '.github/workflows/docker-runtime-acceptance.yml'
ENDPOINT = 'unix:///run/user/1001/docker.sock'


def report():
    return {
        'docker_engine': {'endpoint': ENDPOINT, 'mode': 'rootless',
                          'daemon_id': 'verified-daemon', 'verified': True},
        'ready': False,
        'checks': [{'name': name, 'ready': True}
                   for name in ['os', 'Docker engine', 'core-deployment']],
    }


def workflow_python(step):
    section = WORKFLOW.read_text().split('      - name: ' + step + '\n', 1)[1]
    code = section.split("<<'PYTHON'\n", 1)[1].split('\n          PYTHON', 1)[0]
    return textwrap.dedent(code)


class DockerRuntimeDoctorTests(unittest.TestCase):
    def execute(self, step, value, original=None):
        code = workflow_python(step)
        with tempfile.TemporaryDirectory() as tmp:
            preflight = Path(tmp) / 'preflight.json'
            doctor = Path(tmp) / 'doctor.json'
            preflight.write_text(json.dumps(value if original is None else original))
            doctor.write_text(json.dumps(value))
            code = code.replace('/tmp/docker-engine-preflight.json', str(preflight))
            code = code.replace('/tmp/docker-doctor.json', str(doctor))
            return subprocess.run(['python3', '-c', code], capture_output=True,
                                  text=True, env={**os.environ, 'DOCKER_HOST': ENDPOINT})

    def test_preflight_accepts_only_actual_verified_rootless_endpoint(self):
        step = 'Verify selected rootless Docker before cleanup'
        self.assertEqual(ENDPOINT, self.execute(step, report()).stdout.strip())
        invalid = [None, {}, {'verified': False}, {'mode': 'rootful'},
                   {'daemon_id': ''}, {'endpoint': 'tcp://localhost:2375'}, {'endpoint': 'unix:///other.sock'},
                   {'endpoint': ENDPOINT + '\nDOCKER_HOST=other'}]
        for changed in invalid:
            with self.subTest(changed=changed):
                value = report()
                if changed is None:
                    value.pop('docker_engine')
                elif changed == {}:
                    value['docker_engine'] = {}
                else:
                    value['docker_engine'].update(changed)
                result = self.execute(step, value)
                self.assertNotEqual(0, result.returncode)
                self.assertEqual('', result.stdout)

    def test_reset_doctor_accepts_current_not_deployed_contract(self):
        value = report()
        result = self.execute('Verify Docker runtime detection', value, copy.deepcopy(value))
        self.assertEqual(0, result.returncode, result.stderr)

    def test_reset_doctor_rejects_failures_missing_checks_and_engine_drift(self):
        baseline = report()
        cases = []
        for field, bad in [('verified', False), ('mode', 'rootful'),
                           ('endpoint', 'unix:///other.sock'), ('daemon_id', 'new-daemon')]:
            value = report()
            value['docker_engine'][field] = bad
            cases.append(value)
        for name in ['os', 'Docker engine', 'core-deployment']:
            value = report()
            value['checks'] = [check for check in value['checks'] if check['name'] != name]
            cases.append(value)
        value = report()
        value['checks'][0]['ready'] = False
        cases.append(value)
        value = report()
        value['checks'].append({'name': 'unknown-failure', 'ready': False})
        cases.append(value)
        value = report()
        value['checks'].append(value['checks'][0])
        cases.append(value)
        value = report()
        value['ready'] = True
        cases.append(value)
        for value in cases:
            with self.subTest(value=value):
                result = self.execute('Verify Docker runtime detection', value, baseline)
                self.assertNotEqual(0, result.returncode)


class HostedRootlessDockerReceipts(unittest.TestCase):
    required = {
        'main': [
            ('cmd/baha', 'TestRepositoryWorkflowRealLifecycle'),
            ('cmd/baha', 'TestRepositoryComposeWorkloadUsesBaseHarborBackendsInCI'),
            ('cmd/baha', 'TestRepositoryComposeWorkloadOnlyLifecycleInCI'),
            ('cmd/baha', 'TestRepositoryBuildWorkloadRebuildsSourceChangesInCI'),
            ('internal/application', 'TestMultiInstanceComposeLifecycleInCI'),
        ],
        'recovery': [('cmd/baha', 'TestExistingControlPlaneRestartRequiresAndUsesRecoveryFile')],
        'logs': [('internal/logs', 'TestManagedLokiIngestsRealComposeWorkloadLogs')],
        'traces': [('internal/traces', 'TestManagedTempoReceivesVerificationTraceThroughCollector')],
    }

    def receipts(self):
        result = {}
        for name, expected in self.required.items():
            rows = [{'Package': 'github.com/mcpdev80/baseharbor/' + package,
                     'Test': test, 'Action': 'pass'} for package, test in expected]
            rows += [{'Package': 'github.com/mcpdev80/baseharbor/' + package,
                      'Action': 'pass'} for package in sorted({p for p, _ in expected})]
            result[name] = rows
        return result

    def execute(self, receipts):
        code = workflow_python('Verify all eight executed runtime tests')
        with tempfile.TemporaryDirectory() as tmp:
            folder = Path(tmp) / 'docker-runtime-evidence/tests'
            folder.mkdir(parents=True)
            for name, events in receipts.items():
                (folder / (name + '.jsonl')).write_text('\n'.join(json.dumps(e) for e in events))
            return subprocess.run(['python3', '-c', code], capture_output=True,
                                  text=True, env={**os.environ, 'RUNNER_TEMP': tmp})

    def test_actual_parser_requires_all_eight_test_and_package_passes(self):
        self.assertEqual(8, sum(len(tests) for tests in self.required.values()))
        passed = self.execute(self.receipts())
        self.assertEqual(0, passed.returncode, passed.stderr)
        for name, required in self.required.items():
            for _, test in required:
                for mutation in ['missing', 'skip', 'fail', 'wrong-package']:
                    with self.subTest(name=name, test=test, mutation=mutation):
                        value = self.receipts()
                        for row in list(value[name]):
                            if row.get('Test') != test:
                                continue
                            if mutation == 'missing':
                                value[name].remove(row)
                            elif mutation == 'wrong-package':
                                row['Package'] = 'example.test/other'
                            else:
                                row['Action'] = mutation
                        self.assertNotEqual(0, self.execute(value).returncode)
            value = self.receipts()
            value[name] = [row for row in value[name] if 'Test' in row]
            self.assertNotEqual(0, self.execute(value).returncode)
            value = self.receipts()
            del value[name]
            self.assertNotEqual(0, self.execute(value).returncode)

    def test_actual_parser_rejects_skipped_nested_cleanup_even_with_parent_pass(self):
        value = self.receipts()
        value['main'].append({'Package': 'github.com/mcpdev80/baseharbor/cmd/baha',
                              'Test': 'TestRepositoryWorkflowRealLifecycle/cleanup', 'Action': 'skip'})
        self.assertNotEqual(0, self.execute(value).returncode)

    def test_supported_hosted_setup_precedes_every_reset_and_doctor(self):
        workflow = WORKFLOW.read_text().split('  provider-ha:', 1)[0]
        self.assertIn('runs-on: ubuntu-latest', workflow)
        self.assertNotIn('self-hosted', workflow)
        self.assertNotIn('Persistent', workflow)
        self.assertIn("go-version: '1.26.9'", workflow)
        setup = workflow.index('run: bash scripts/ci-core-rootless-runtime.sh')
        for match in __import__('re').finditer('ci-runtime-reset.sh|baha\" doctor', workflow):
            self.assertLess(setup, match.start())
        self.assertIn('if: always()', workflow.split('      - name: Verify all eight executed runtime tests', 1)[1])
        self.assertIn('retention-days: 30', workflow)
        self.assertIn('if-no-files-found: error', workflow)
        for name in self.required:
            self.assertIn('/tests/' + name + '.jsonl', workflow)
        for name in ['source-sha.txt', 'source-tree.txt', 'source-repository.txt',
                     'go-version.txt', 'runtime-version.json']:
            self.assertIn(name, workflow)


class ExactDockerRuntimeImage(unittest.TestCase):
    def test_actual_packaging_step_verifies_revision_binary_hash_and_commit(self):
        import hashlib
        section = WORKFLOW.read_text().split('      - name: Package and verify exact candidate runtime image\n', 1)[1]
        shell = textwrap.dedent(section.split('        run: |\n', 1)[1].split('\n      - name:', 1)[0])
        recipe = (WORKFLOW.parent.parent.parent / 'deploy/control-plane/Dockerfile.binary').read_text()
        sha = 'a' * 40
        binary = b'fixture exact static candidate binary'
        digest = hashlib.sha256(binary).hexdigest()
        for bad in ['', 'revision', 'binary', 'commit', 'version']:
            with self.subTest(bad=bad), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                build_source = root / 'source'
                recipe_path = build_source / 'deploy/control-plane/Dockerfile.binary'
                recipe_path.parent.mkdir(parents=True)
                recipe_path.write_text(recipe)
                runner_temp = root / 'runner'
                runner_temp.mkdir()
                (runner_temp / 'docker-runtime-evidence').mkdir()
                (runner_temp / 'baha').write_bytes(binary)
                tools = root / 'tools'
                tools.mkdir()
                git = tools / 'git'
                git.write_text('#!/bin/sh\ntest "$*" = "rev-parse HEAD" || exit 98\nprintf "%s\\n" "$FIXTURE_SHA"\n')
                git.chmod(0o755)
                docker = tools / 'docker'
                docker.write_text('''#!/usr/bin/env python3
import hashlib, json, os
from pathlib import Path
import sys
args = sys.argv[1:]
with Path(os.environ['FIXTURE_CALLS']).open('a') as output:
    output.write(json.dumps(args) + '\\n')
assert args[:2] == ['--host', os.environ['DOCKER_HOST']], args
args = args[2:]
image = 'localhost/baseharbor-runtime:docker-' + os.environ['FIXTURE_SHA']
assert 'push' not in args
bad = os.environ['FIXTURE_BAD']
if args[0] == 'build':
    assert '--pull' in args
    assert args[args.index('-t') + 1] == image
    assert args[args.index('--label') + 1] == 'org.opencontainers.image.revision=' + os.environ['FIXTURE_SHA']
    context = Path(args[-1])
    assert (context / 'Dockerfile').read_text() == Path('deploy/control-plane/Dockerfile.binary').read_text()
    assert hashlib.sha256((context / 'baha').read_bytes()).hexdigest() == os.environ['FIXTURE_DIGEST']
elif args[:2] == ['image', 'inspect']:
    assert args[-1] == image
    print('sha256:' + 'b' * 64 if args[3] == '{{.Id}}' else ('other' if bad == 'revision' else os.environ['FIXTURE_SHA']))
elif args[0] == 'run':
    assert '--rm' in args and '--pull=never' in args and args[args.index('--network') + 1] == 'none'
    command = args[args.index('--entrypoint') + 1]
    assert image in args
    if command == 'sha256sum':
        print(('0' * 64 if bad == 'binary' else os.environ['FIXTURE_DIGEST']) + '  /usr/local/bin/baha')
    else:
        assert command == '/usr/local/bin/baha' and args[-2:] == ['version', '--json']
        print(json.dumps({'commit': 'other' if bad == 'commit' else os.environ['FIXTURE_SHA'],
                          'version': 'v0.4.26-candidate' if bad == 'version' else 'dev'}))
else:
    raise SystemExit(99)
''')
                docker.chmod(0o755)
                env_file = root / 'github-env'
                calls_file = root / 'docker-calls'
                env = {**os.environ, 'PATH': str(tools) + os.pathsep + os.environ['PATH'],
                       'RUNNER_TEMP': str(runner_temp), 'DOCKER_HOST': ENDPOINT,
                       'GITHUB_ENV': str(env_file), 'FIXTURE_SHA': sha,
                       'FIXTURE_DIGEST': digest, 'FIXTURE_BAD': bad,
                       'FIXTURE_CALLS': str(calls_file)}
                result = subprocess.run(['bash', '-euo', 'pipefail', '-c', shell], cwd=build_source,
                                        env=env, capture_output=True, text=True)
                self.assertEqual(not bad, result.returncode == 0, result.stdout + result.stderr)
                if bad:
                    self.assertFalse(env_file.exists(), 'unverified image must not be selected')
                else:
                    self.assertEqual('BASEHARBOR_RUNTIME_IMAGE=localhost/baseharbor-runtime:docker-' + sha,
                                     env_file.read_text().strip())
                    self.assertEqual(binary, (runner_temp / 'baha').read_bytes())

    def test_candidate_image_and_host_binary_survive_existing_reset_contract(self):
        workflow = WORKFLOW.read_text().split('  provider-ha:', 1)[0]
        binary = workflow.index('CGO_ENABLED=0 go build')
        preflight = workflow.index('      - name: Verify selected rootless Docker before cleanup')
        image = workflow.index('      - name: Package and verify exact candidate runtime image')
        reset = workflow.index('run: bash scripts/ci-runtime-reset.sh docker')
        first_test = workflow.index('go test -p 1')
        self.assertLess(binary, preflight)
        self.assertLess(preflight, image)
        self.assertLess(image, reset)
        self.assertLess(reset, first_test)
        self.assertIn('-X main.commit=$candidate_sha', workflow)
        self.assertIn('-X main.version=dev ', workflow)
        self.assertNotIn('-X main.version=v0.4.26-candidate', workflow)
        self.assertIn('-o "$RUNNER_TEMP/baha"', workflow)
        self.assertNotIn('/tmp/baha', workflow)
        self.assertIn('cp deploy/control-plane/Dockerfile.binary', workflow)
        self.assertNotIn('-f deploy/control-plane/Dockerfile ', workflow)
        reset_source = (WORKFLOW.parent.parent.parent / 'scripts/ci-runtime-reset.sh').read_text()
        self.assertNotRegex(reset_source, r'(?m)^\s*(?:docker|\$engine|"\$engine")\s+(?:image|system)\s+(?:prune|rm)')


if __name__ == '__main__':
    unittest.main()
