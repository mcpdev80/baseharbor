"""Dispatch-gate selftest: bare/short SHA refs must never reach actions/checkout unresolved.

Regression proof for Atomic Acceptance Gate run 39856259550 (analyses o33-atomic-gate-c2.md,
o26-selected-checks.md): actions/checkout v7 fetches any value that is not a full 40/64-hex
SHA via the refspec +refs/heads/<value>*, so a bare short SHA such as 2adae47a fails
deterministically after silent retries. Verifies the resolver classification (mocked gh,
no network) and the workflow wiring in targeted-demo-acceptance.yml and
pre-release-runtime-suite.yml, executing the suite's inline fail-fast guard with a fake gh.
"""
import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest

import yaml

ROOT = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('gate_ref', ROOT / 'targeted-gate-ref.py')
gate_ref = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate_ref)

FULL_A = 'a' * 40
FULL_B = 'b' * 40
SHORT = '2adae47a'
SHORT_EXPANDED = SHORT + '0123456789abcdef0123456789abcdef'


class FixtureResolver:
    """Data-driven RefResolver stand-in; records nothing that a network would see."""

    def __init__(self, branches=(), tags=(), commits=(), expandable=None, repo='mcpdev80/baseharbor'):
        self.branches = set(branches)
        self.tags = set(tags)
        self.commits = set(commits)
        self.expandable = dict(expandable or {})
        self.repo = repo

    def branch(self, name):
        return name in self.branches

    def tag(self, name):
        return name in self.tags

    def commit_object(self, sha):
        return sha.lower() in {c.lower() for c in self.commits}

    def expand(self, abbreviated):
        return self.expandable.get(abbreviated.lower())


def resolver(**kwargs):
    fixture = FixtureResolver(**kwargs)
    resolver = gate_ref.RefResolver.__new__(gate_ref.RefResolver)
    resolver.gh = 'gh'
    resolver.repo = fixture.repo
    resolver.branch = fixture.branch
    resolver.tag = fixture.tag
    resolver.commit_object = fixture.commit_object
    resolver.expand = fixture.expand
    return resolver


class ResolverClassification(unittest.TestCase):
    def test_bare_short_sha_expands_to_full_commit(self):
        # o33 regression: dispatch ref=2adae47a, no branch/tag of that name.
        resolved = gate_ref.resolve(SHORT, resolver(expandable={SHORT: SHORT_EXPANDED}))
        self.assertTrue(gate_ref.FULL_SHA.match(resolved))
        self.assertNotEqual(resolved, SHORT)

    def test_short_hex_matching_branch_name_stays_a_branch(self):
        self.assertEqual(gate_ref.resolve(SHORT, resolver(branches=[SHORT])), f'refs/heads/{SHORT}')
        self.assertEqual(gate_ref.resolve(SHORT, resolver(tags=[SHORT])), f'refs/tags/{SHORT}')

    def test_unresolvable_short_sha_fails_fast_with_recipe_message(self):
        with self.assertRaises(ValueError) as caught:
            gate_ref.resolve(SHORT, resolver())
        self.assertIn('is not a branch/tag; resolve to a ref', str(caught.exception))

    def test_existing_full_sha_passes_through(self):
        self.assertEqual(gate_ref.resolve(FULL_A, resolver(commits=[FULL_A])), FULL_A)

    def test_nonexistent_full_sha_fails_fast(self):
        # o26 regression: a malformed/absent SHA must not fall into the branch-prefix fetch.
        with self.assertRaises(ValueError) as caught:
            gate_ref.resolve(FULL_B, resolver(commits=[]))
        self.assertIn('does not exist', str(caught.exception))

    def test_branch_and_tag_names_are_qualified(self):
        self.assertEqual(gate_ref.resolve('release/v0.4.21', resolver(branches=['release/v0.4.21'])),
                         'refs/heads/release/v0.4.21')
        self.assertEqual(gate_ref.resolve('v0.4.26', resolver(tags=['v0.4.26'])), 'refs/tags/v0.4.26')

    def test_unknown_name_fails_fast(self):
        with self.assertRaises(ValueError) as caught:
            gate_ref.resolve('no/such-branch', resolver())
        self.assertIn('is not a branch/tag; resolve to a ref', str(caught.exception))

    def test_explicit_ref_path_passes_through_without_api(self):
        self.assertEqual(gate_ref.resolve('refs/pull/1041/merge', resolver()), 'refs/pull/1041/merge')

    def test_empty_ref_fails_fast(self):
        for value in ('', '   '):
            with self.subTest(value=value), self.assertRaises(ValueError):
                gate_ref.resolve(value, resolver())

    def test_cli_fail_fast_exit_code_and_stderr(self):
        stderr = sys.stderr
        try:
            sys.stderr = io = __import__('io').StringIO()
            code = gate_ref.main(['--ref', '  ', '--repo', 'mcpdev80/baseharbor', '--gh', 'gh'])
        finally:
            sys.stderr = stderr
        self.assertEqual(code, 2)
        self.assertIn('::error::', io.getvalue())


class GhEndpointWiring(unittest.TestCase):
    def test_resolver_uses_expected_api_endpoints(self):
        with tempfile.TemporaryDirectory() as tmp:
            calls = pathlib.Path(tmp) / 'calls.jsonl'
            fake = pathlib.Path(tmp) / 'gh'
            fake.write_text('#!/usr/bin/env python3\n'
                            'import json, os, sys\n'
                            'with open(os.environ["GATE_REF_CALLS"], "a") as log:\n'
                            '    log.write(json.dumps(sys.argv) + "\\n")\n'
                            'endpoint = sys.argv[2]\n'
                            'table = json.loads(os.environ["GATE_REF_FIXTURES"])\n'
                            'if endpoint in table:\n'
                            '    print(json.dumps(table[endpoint])); sys.exit(0)\n'
                            'sys.exit(1)\n')
            fake.chmod(0o755)
            env = dict(os.environ, GATE_REF_CALLS=str(calls),
                       GATE_REF_FIXTURES=json.dumps({
                           f'repos/o/r/git/ref/heads/{SHORT}': {'object': {'sha': FULL_A}},
                           f'repos/o/r/git/commits/{FULL_B}': {'sha': FULL_B},
                           f'repos/o/r/commits/{SHORT}': {'sha': SHORT_EXPANDED},
                       }))
            for ref, expected in ((SHORT, f'refs/heads/{SHORT}'), (FULL_B, FULL_B)):
                with self.subTest(ref=ref):
                    result = subprocess.run([sys.executable, str(ROOT / 'targeted-gate-ref.py'),
                                             '--ref', ref, '--repo', 'o/r', '--gh', str(fake)],
                                            capture_output=True, text=True, env=env, check=False)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(result.stdout.strip(), expected)
            result = subprocess.run([sys.executable, str(ROOT / 'targeted-gate-ref.py'),
                                     '--ref', 'deadbeefcafe', '--repo', 'o/r', '--gh', str(fake)],
                                    capture_output=True, text=True, env=env, check=False)
            self.assertEqual(result.returncode, 2)
            self.assertIn('::error::', result.stderr)
            logged = [json.loads(line)[2] for line in calls.read_text().splitlines()]
            self.assertIn(f'repos/o/r/commits/deadbeefcafe', logged)


class WorkflowWiring(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.workflows = pathlib.Path(ROOT).parent / '.github' / 'workflows'
        cls.targeted = yaml.safe_load((cls.workflows / 'targeted-demo-acceptance.yml').read_text())
        cls.suite = yaml.safe_load((cls.workflows / 'pre-release-runtime-suite.yml').read_text())

    def test_resolve_job_normalizes_dispatch_ref_and_has_tooling_on_all_events(self):
        steps = self.targeted['jobs']['resolve']['steps']
        checkout = next(s for s in steps if s.get('name', '').startswith('Checkout'))
        self.assertNotIn('if', checkout, 'gate tooling checkout must run for dispatch events too')
        self.assertEqual(checkout['with']['ref'], '${{ github.sha }}')
        resolve = next(s for s in steps if s.get('id') == 'resolve')
        self.assertIn('scripts/targeted-gate-ref.py --ref "$DISPATCH_REF"', resolve['run'])
        self.assertIn('ref="$GITHUB_SHA"', resolve['run'])
        triggers = self.targeted.get('on') or self.targeted.get(True)
        description = triggers['workflow_dispatch']['inputs']['ref']['description']
        self.assertIn('short SHAs are resolved', description)

    def test_suite_validates_ref_before_checkout_without_evidence_uploads(self):
        steps = self.suite['jobs']['gate']['steps']
        names = [s.get('name') for s in steps]
        self.assertLess(names.index('Validate candidate ref before checkout'),
                        names.index('Checkout BaseHarbor candidate'))
        self.assertFalse(any('evidence' in str(s.get('with', {}).get('name', '')).lower()
                             for s in steps if s.get('uses', '').startswith('actions/upload-artifact@')))

    def test_observability_gate_executes_fullstack_and_rejects_skip_or_failure(self):
        steps = self.suite['jobs']['gate']['steps']
        ci = yaml.safe_load((self.workflows / 'ci.yml').read_text())
        plan = ci['jobs']['pr_plan']
        self.assertEqual('${{ steps.plan.outputs.trusted_base_sha }}', plan['outputs']['trusted_base_sha'])
        self.assertIn('["target_base_sha"]', next(s['run'] for s in plan['steps'] if s.get('id') == 'plan'))
        self.assertEqual('${{ needs.pr_plan.outputs.trusted_base_sha }}',
                         ci['jobs']['pr_runtime_selected']['with']['trusted_base_sha'])
        setup = next(s for s in steps if s.get('name') == 'Set up Go')
        self.assertIn("inputs.gate == 'observability'", setup['if'])
        gate = next(s for s in steps if s.get('id') == 'runtime_gate')
        self.assertIn("inputs.gate == 'observability' && 25", gate['timeout-minutes'])
        condition = 'if [ "$GATE" = observability ] && [ "$GITHUB_EVENT_NAME" = pull_request ]; then'
        block = 'set -euo pipefail\n' + condition + gate['run'].split(condition, 1)[1]
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            (root / 'scripts').mkdir()
            (root / 'scripts/ci-runtime-reset.sh').write_text('printf "%s\\n" "$1" > "$RUNNER_TEMP/reset"\n')
            bindir = root / 'bin'
            bindir.mkdir()
            git = bindir / 'git'
            git.write_text('#!/bin/sh\ncase "$1" in\n'
                           '  rev-parse) printf "%s\\n" "${GIT_ACTUAL_SHA:-$CANDIDATE_SHA}";;\n'
                           '  fetch) exit "${FETCH_STATUS:-0}";;\n'
                           '  diff) exit "${DIFF_STATUS:-1}";;\n'
                           '  *) exit 2;;\nesac\n')
            git.chmod(0o755)
            go = bindir / 'go'
            go.write_text('#!/bin/sh\nprintf "%s %s %s\\n" "$BASEHARBOR_OBSERVABILITY_ACCEPTANCE" "$BASEHARBOR_PROVIDER_POSTGRESQL_SCOPE" "$BASEHARBOR_PROVIDER_VALKEY_SCOPE" > "$RUNNER_TEMP/test-env"\n'
                          'case "$FAKE_RESULT" in\n'
                          '  pass) printf "=== RUN   TestObservabilityFullStackAcceptanceInCI\\n    acceptance_test.go:100: [VERIFIED] logs 1 workload stream(s), 4 provider stream(s) ingested\\n--- PASS: TestObservabilityFullStackAcceptanceInCI (1s)\\n";;\n'
                          '  zero) printf "=== RUN   TestObservabilityFullStackAcceptanceInCI\\n    acceptance_test.go:100: [VERIFIED] logs 1 workload stream(s), 0 provider stream(s) ingested\\n--- PASS: TestObservabilityFullStackAcceptanceInCI (1s)\\n";;\n'
                          '  missing) printf "=== RUN   TestObservabilityFullStackAcceptanceInCI\\n--- PASS: TestObservabilityFullStackAcceptanceInCI (1s)\\n";;\n'
                          '  skip) printf "=== RUN   TestObservabilityFullStackAcceptanceInCI\\n--- SKIP: TestObservabilityFullStackAcceptanceInCI (0s)\\n";;\n'
                          '  empty) printf "testing: warning: no tests to run\\n";;\n'
                          '  fail) exit 1;;\n'
                          'esac\n')
            go.chmod(0o755)
            env = dict(os.environ, PATH=f'{bindir}{os.pathsep}{os.environ["PATH"]}',
                       GITHUB_WORKSPACE=tmp, RUNNER_TEMP=tmp, CANDIDATE_SHA='a' * 40,
                       RUNTIME='podman', GATE='observability', GITHUB_EVENT_NAME='pull_request',
                       PR_BASE_SHA='c' * 40, EVENT_BASE_SHA='c' * 40)
            for result, expected in (('pass', 0), ('zero', 1), ('missing', 1), ('skip', 1), ('empty', 1), ('fail', 1)):
                with self.subTest(result=result):
                    execution = subprocess.run(['bash', '-c', block], env={**env, 'FAKE_RESULT': result},
                                               capture_output=True, text=True)
                    self.assertEqual(expected, execution.returncode, execution.stderr)
            self.assertEqual('podman', (root / 'reset').read_text().strip())
            self.assertEqual('1 application application', (root / 'test-env').read_text().strip())
            wrong_sha = subprocess.run(['bash', '-c', block],
                                       env={**env, 'FAKE_RESULT': 'pass', 'GIT_ACTUAL_SHA': 'b' * 40},
                                       capture_output=True, text=True)
            self.assertNotEqual(0, wrong_sha.returncode)
            (root / 'reset').unlink()
            other_gate = subprocess.run(['bash', '-c', block],
                                        env={**env, 'GATE': 'lifecycle', 'FAKE_RESULT': 'pass'},
                                        capture_output=True, text=True)
            self.assertEqual(0, other_gate.returncode, other_gate.stderr)
            self.assertFalse((root / 'reset').exists())
            unrelated = subprocess.run(['bash', '-c', block],
                                       env={**env, 'FAKE_RESULT': 'pass', 'DIFF_STATUS': '0'},
                                       capture_output=True, text=True)
            self.assertEqual(0, unrelated.returncode, unrelated.stderr)
            self.assertFalse((root / 'reset').exists())
            for override in ({'PR_BASE_SHA': ''}, {'EVENT_BASE_SHA': 'd' * 40},
                             {'FETCH_STATUS': '2'}, {'DIFF_STATUS': '2'}):
                with self.subTest(override=override):
                    failed = subprocess.run(['bash', '-c', block],
                                            env={**env, **override, 'FAKE_RESULT': 'pass'},
                                            capture_output=True, text=True)
                    self.assertNotEqual(0, failed.returncode)

    def test_lifecycle_native_test_source_changes_execute_real_tests(self):
        gate = next(s for s in self.suite['jobs']['gate']['steps'] if s.get('id') == 'runtime_gate')
        condition = 'if [ "$GATE" = lifecycle ] && [ "$GITHUB_EVENT_NAME" = pull_request ]; then'
        next_block = 'if [ "$GATE" = lifecycle ] && [ "$RUNTIME" = podman ]'
        block = 'set -euo pipefail\n' + condition + gate['run'].split(condition, 1)[1].split(next_block, 1)[0]
        mapping = {
            'cmd/baha/app_repository_integration_test.go': ['TestRepositoryWorkflowRealLifecycle'],
            'cmd/baha/app_workload_integration_test.go': ['TestRepositoryComposeWorkloadUsesBaseHarborBackendsInCI', 'TestRepositoryComposeWorkloadOnlyLifecycleInCI', 'TestRepositoryBuildWorkloadRebuildsSourceChangesInCI'],
            'internal/application/runtime_multi_integration_test.go': ['TestMultiInstanceComposeLifecycleInCI'],
        }
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            (root / 'scripts').mkdir()
            (root / 'scripts/ci-runtime-reset.sh').write_text('exit 0\n')
            bindir = root / 'bin'
            bindir.mkdir()
            git = bindir / 'git'
            git.write_text('#!/bin/sh\ncase "$1" in\n'
                           'rev-parse) echo "$CANDIDATE_SHA";;\n'
                           'fetch) exit 0;;\n'
                           'diff) test "${DIFF_ERROR:-0}" = 0 || exit 2; printf "%s\n" "$CHANGED_PATH";;\n'
                           '*) exit 2;; esac\n')
            git.chmod(0o755)
            go = bindir / 'go'
            go.write_text('#!/usr/bin/env python3\nimport os, sys\n'
                          'name=sys.argv[sys.argv.index("-run")+1].strip("^$")\n'
                          'assert os.environ.get("BASEHARBOR_CI_RUNTIME_INTEGRATION") == "1"\n'
                          'open(os.path.join(os.environ["RUNNER_TEMP"], "calls"), "a").write(name+"\\n")\n'
                          'print("=== RUN   "+name)\n'
                          'print(("--- PASS: " if os.environ.get("FAKE_OUTCOME") == "pass" else "--- SKIP: ")+name+" (1s)")\n')
            go.chmod(0o755)
            env = dict(os.environ, PATH=f'{bindir}{os.pathsep}{os.environ["PATH"]}',
                       GITHUB_WORKSPACE=tmp, RUNNER_TEMP=tmp, CANDIDATE_SHA='a' * 40,
                       RUNTIME='podman', GATE='lifecycle', GITHUB_EVENT_NAME='pull_request',
                       PR_BASE_SHA='c' * 40, EVENT_BASE_SHA='c' * 40, FAKE_OUTCOME='pass')
            for path, expected in mapping.items():
                with self.subTest(path=path):
                    calls = root / 'calls'
                    calls.unlink(missing_ok=True)
                    run = subprocess.run(['bash', '-c', block], env={**env, 'CHANGED_PATH': path}, capture_output=True, text=True)
                    self.assertEqual(0, run.returncode, run.stderr)
                    self.assertEqual(expected, calls.read_text().splitlines())
            bad = {**env, 'CHANGED_PATH': next(iter(mapping)), 'FAKE_OUTCOME': 'skip'}
            self.assertNotEqual(0, subprocess.run(['bash', '-c', block], env=bad, capture_output=True).returncode)
            bad = {**env, 'CHANGED_PATH': next(iter(mapping)), 'DIFF_ERROR': '1'}
            self.assertNotEqual(0, subprocess.run(['bash', '-c', block], env=bad, capture_output=True).returncode)

    def test_podman_doctor_smoke_is_fail_closed(self):
        gate = next(s for s in self.suite['jobs']['gate']['steps'] if s.get('id') == 'runtime_gate')
        start = 'if [ "$GATE" = lifecycle ] && [ "$RUNTIME" = podman ] && [ "$GITHUB_EVENT_NAME" = pull_request ]; then'
        end = 'if [ "$GATE" = observability ] && [ "$GITHUB_EVENT_NAME" = pull_request ]; then'
        block = 'set -euo pipefail\n' + start + gate['run'].split(start, 1)[1].split(end, 1)[0]
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            (root / 'demo/.tools/bin').mkdir(parents=True)
            bindir = root / 'bin'
            bindir.mkdir()
            git = bindir / 'git'
            git.write_text('#!/bin/sh\ncase "$1" in rev-parse) echo "$CANDIDATE_SHA";; fetch) exit 0;; diff) exit "${DIFF_STATUS:-1}";; esac\n')
            git.chmod(0o755)
            baha = root / 'demo/.tools/bin/baha'
            baha.write_text('#!/bin/sh\ncase "$1" in version) echo "$CANDIDATE_SHA";; target) exit 0;; doctor) test ! -e baseharbor.yaml || exit 1; printf "%s\\n" "$DOCTOR_JSON";; *) exit 1;; esac\n')
            baha.chmod(0o755)
            env = dict(os.environ, PATH=f'{bindir}{os.pathsep}{os.environ["PATH"]}',
                       GITHUB_WORKSPACE=tmp, RUNNER_TEMP=tmp, CANDIDATE_SHA='a' * 40,
                       PR_BASE_SHA='b' * 40, EVENT_BASE_SHA='b' * 40,
                       GATE='lifecycle', RUNTIME='podman', GITHUB_EVENT_NAME='pull_request',
                       workdir=str(root / 'demo'))
            for checks, expected in (([{'name': 'podman', 'ready': True}, {'name': 'quadlet', 'ready': True}], 0),
                                     ([{'name': 'podman', 'ready': True}], 1),
                                     ([{'name': 'podman', 'ready': False}, {'name': 'quadlet', 'ready': True}], 1)):
                with self.subTest(checks=checks):
                    execution = subprocess.run(['bash', '-c', block], env={**env, 'DOCTOR_JSON': json.dumps({'checks': checks})},
                                               capture_output=True, text=True)
                    self.assertEqual(expected, execution.returncode, execution.stderr)

    def test_observability_scope_detects_git_rename_and_delete(self):
        gate = next(s for s in self.suite['jobs']['gate']['steps'] if s.get('id') == 'runtime_gate')
        self.assertIn('git diff --no-renames --quiet "$PR_BASE_SHA" "$CANDIDATE_SHA" -- internal/logs/render.go', gate['run'])
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            def git(*args):
                return subprocess.run(['git', '-C', tmp, *args], check=True,
                                      capture_output=True, text=True).stdout.strip()
            git('init', '-q')
            git('config', 'user.name', 'fixture')
            git('config', 'user.email', 'fixture@example.invalid')
            (root / 'internal/logs').mkdir(parents=True)
            (root / 'internal/logs/render.go').write_text('package logs\n')
            git('add', '.')
            git('commit', '-qm', 'base')
            base = git('rev-parse', 'HEAD')
            for action in ('rename', 'delete'):
                with self.subTest(action=action):
                    git('reset', '--hard', '-q', base)
                    if action == 'rename':
                        git('mv', 'internal/logs/render.go', 'internal/logs/renamed.go')
                    else:
                        git('rm', '-q', 'internal/logs/render.go')
                    git('commit', '-qm', action)
                    candidate = git('rev-parse', 'HEAD')
                    diff = subprocess.run(['git', '-C', tmp, 'diff', '--no-renames', '--quiet',
                                           base, candidate, '--', 'internal/logs/render.go'],
                                          capture_output=True, text=True)
                    self.assertEqual(1, diff.returncode)

    def test_suite_inline_guard_fails_fast_on_bare_short_sha(self):
        guard = next(s for s in self.suite['jobs']['gate']['steps']
                     if s.get('name') == 'Validate candidate ref before checkout')
        fixtures = {
            f'repos/o/r/git/commits/{FULL_A}': 0,
            f'repos/o/r/git/ref/heads/release/v0.4.21': 0,
            f'repos/o/r/git/ref/tags/v0.4.26': 0,
        }
        with tempfile.TemporaryDirectory() as tmp:
            fake = pathlib.Path(tmp) / 'gh'
            fake.write_text('#!/usr/bin/env python3\n'
                            'import json, os, sys\n'
                            'table = json.loads(os.environ["GATE_REF_FIXTURES"])\n'
                            'sys.exit(0 if sys.argv[2] in table else 1)\n')
            fake.chmod(0o755)
            env = dict(os.environ, PATH=f'{tmp}{os.pathsep}{os.environ["PATH"]}',
                       GITHUB_REPOSITORY='o/r', GH_TOKEN='x',
                       GATE_REF_FIXTURES=json.dumps(fixtures))
            cases = {
                SHORT: 2, 'nonexistent-branch': 2, '': 2,
                FULL_A: 0, f'{SHORT}{"0" * 32}': 2, 'refs/heads/anything': 0,
                'release/v0.4.21': 0, 'v0.4.26': 0,
            }
            for ref, expected in cases.items():
                with self.subTest(ref=ref):
                    result = subprocess.run(['bash', '-c', guard['run']],
                                            capture_output=True, text=True, check=False,
                                            env=dict(env, CANDIDATE_REF=ref))
                    self.assertEqual(result.returncode, expected, result.stderr)
                    if expected:
                        self.assertIn('::error::', result.stderr)


if __name__ == '__main__':
    unittest.main(verbosity=2)
