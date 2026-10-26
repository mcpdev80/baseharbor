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

    def test_suite_validates_ref_before_checkout_and_hardens_uploads(self):
        steps = self.suite['jobs']['gate']['steps']
        names = [s.get('name') for s in steps]
        self.assertLess(names.index('Validate candidate ref before checkout'),
                        names.index('Checkout BaseHarbor candidate'))
        for name in ('Upload demo runtime diagnostics', 'Upload atomic failure diagnostics'):
            upload = next(s for s in steps if s.get('name') == name)
            self.assertEqual(upload['with']['if-no-files-found'], 'error')
            self.assertIn("steps.checkout.outcome == 'success'", upload['if'])
        self.assertEqual([s['with']['if-no-files-found'] for s in steps
                          if s.get('uses', '').startswith('actions/upload-artifact@')
                          if isinstance(s.get('with'), dict) and 'if-no-files-found' in s['with']].count('warn'), 0)
        preserve = next(s for s in steps if s.get('name') == 'Preserve demo runtime diagnostics')
        self.assertIn('preserve-summary.txt', preserve['run'])

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
