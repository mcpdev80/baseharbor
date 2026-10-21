import copy
import json
import os
import pathlib
import re
import shutil
import subprocess
import tempfile
import unittest

import yaml

from ecosystem_release_pins import resolve_pins, validate_ecosystem_graph
from private_consumer_evidence import load_private_pins


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


class ReleaseWorkflowPinsTests(unittest.TestCase):
    def test_final_evidence_waits_for_every_required_producer(self):
        source = pathlib.Path(__file__).resolve().parents[1]
        jobs = yaml.safe_load((source / '.github/workflows/pre-release.yml').read_text())['jobs']
        requirements = json.loads((source / 'scripts/release-requirements.json').read_text())['gates']
        producers = {job for gate in requirements if 'v0.4.26' in gate['required_for']
                     for job in gate['jobs']}
        self.assertTrue(producers <= jobs.keys())
        pending, ancestors = list(jobs['evidence']['needs']), set()
        while pending:
            job = pending.pop()
            if job in ancestors:
                continue
            ancestors.add(job)
            needs = jobs[job].get('needs', [])
            pending.extend([needs] if isinstance(needs, str) else needs)
        self.assertTrue(producers <= ancestors, sorted(producers - ancestors))

    def test_actual_candidate_resolution_binds_consumers_and_rejects_invalid_sources(self):
        source = pathlib.Path(__file__).resolve().parents[1]
        workflow = (source / '.github/workflows/pre-release.yml').read_text()
        jobs = yaml.safe_load(workflow)['jobs']
        source_step = next(step for step in jobs['source']['steps'] if step.get('id') == 'release')
        routes = {'source': source_step}
        for job in ('integration_remote_target_docker', 'integration_remote_target_podman',
                    'integration_live_console', 'evidence'):
            routes[job] = next(step for step in jobs[job]['steps']
                               if 'scripts/ecosystem_release_pins.py' in step.get('run', ''))
        release_workflow = (source / '.github/workflows/release.yml').read_text()
        start = release_workflow.index('              if [ "$tag" = v0.4.24 ] ||')
        end = release_workflow.index('\n              fi', start) + len('\n              fi')
        routes['publication'] = {'run': release_workflow[start:end]}
        targeted = yaml.safe_load((source / '.github/workflows/targeted-demo-acceptance.yml').read_text())
        routes['targeted'] = next(step for step in targeted['jobs']['resolve']['steps']
                                  if step.get('id') == 'resolve')
        for job, step in routes.items():
            if job in {'source', 'publication', 'targeted'}:
                continue
            for tag, outcome, selected in [('v0.4.24', 'success', True),
                                           ('v0.4.25', 'success', True),
                                           ('v0.4.26', 'success', True),
                                           ('v0.4.26', 'failure', False),
                                           ('v0.4.23', 'success', False)]:
                expression = step['if'].replace('needs.source.outputs.tag', repr(tag))
                expression = expression.replace('needs.source.result', repr(outcome))
                expression = expression.replace('&&', ' and ').replace('||', ' or ')
                with self.subTest(job=job, tag=tag, source_result=outcome):
                    self.assertEqual(eval(expression, {'__builtins__': {}}, {}), selected)
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            repo, origin, tools = root / 'repo', root / 'origin', root / 'bin'
            repo.mkdir()
            tools.mkdir()
            subprocess.run(['git', 'init', '--bare', str(origin)], check=True, capture_output=True)
            subprocess.run(['git', 'init', '-b', 'develop', str(repo)], check=True, capture_output=True)

            def git(*args):
                return subprocess.check_output(['git', '-C', str(repo), *args],
                                               stderr=subprocess.PIPE, text=True).strip()

            git('config', 'user.name', 'Release route fixture')
            git('config', 'user.email', 'fixture@example.invalid')
            (repo / 'scripts').mkdir()
            shutil.copy(source / 'scripts/ecosystem_release_pins.py', repo / 'scripts')
            notes = repo / 'docs/releases'
            notes.mkdir(parents=True)
            for version in ('0.4.24', '0.4.25', '0.4.26'):
                (notes / f'v{version}.md').write_text(f'Release {version}\n')
            (repo / 'CHANGELOG.md').write_text('\n'.join(
                f'## [{version}]' for version in ('0.4.24', '0.4.25', '0.4.26')))
            git('add', '.')
            git('commit', '-m', 'Immutable source fixture')
            core = git('rev-parse', 'HEAD')
            git('branch', 'main')
            git('remote', 'add', 'origin', str(origin))
            git('push', 'origin', 'develop', 'main')
            # Only remote source transport is substituted; the production shell and resolver run.
            gh = tools / 'gh'
            gh.write_text('''#!/usr/bin/env python3
import base64, json, os, sys
path = sys.argv[2]
if path == 'repos/mcpdev80/baseharbor-node-connector/git/ref/heads/work/v' + os.environ['FIXTURE_RELEASE'] + '-integrations-docs':
    print(json.dumps({'ref': 'refs/heads/work/v' + os.environ['FIXTURE_RELEASE'] + '-integrations-docs',
                      'object': {'type': 'commit', 'sha': 'b' * 40}}))
    raise SystemExit(0)
candidate = {'schema': 'baseharbor.ecosystem-candidate/v1',
             'release': os.environ['FIXTURE_RELEASE'], 'core': os.environ['FIXTURE_CORE'],
             'console': 'c' * 40, 'demo': 'd' * 40}
if 'baseharbor-node-connector/contents/integration-candidate.json?' in path:
    content = json.dumps(candidate)
elif 'baseharbor-console/contents/integration-candidate.json?' in path:
    content = json.dumps({key: candidate[key] for key in ('schema', 'release', 'core', 'demo')})
elif 'baseharbor-demo/contents/baseharbor-core.ref?' in path:
    content = os.environ['FIXTURE_DEMO_CORE']
else:
    raise SystemExit('unexpected source request: ' + path)
print(json.dumps({'type': 'file', 'encoding': 'base64', 'size': len(content),
                  'content': base64.b64encode(content.encode()).decode()}))
''')
            gh.chmod(0o755)
            for job, step in routes.items():
                scenarios = [('0.4.24', ''), ('0.4.25', ''), ('0.4.26', ''),
                             ('0.4.26', 'demo-core'), ('0.4.26', 'requested-demo')]
                if job == 'publication':
                    scenarios.append(('0.4.26', 'consumer-ref'))
                if job == 'targeted':
                    scenarios = [('0.4.25', ''), ('0.4.26', ''), ('0.4.26', 'demo-core')]
                for version, invalid in scenarios:
                    with self.subTest(job=job, version=version, invalid=invalid):
                        output = root / 'outputs'
                        output.write_text('')
                        pins_path = (root / 'targeted-consumer-pins.json' if job == 'targeted'
                                     else notes / f'v{version}.consumer-pins.json')
                        pins_path.unlink(missing_ok=True)
                        (repo / 'CHANGELOG.md').write_text(f'## [{version}]\n')
                        approval_json = root / 'release-approved.json'
                        approval_json.write_text(json.dumps({'consumer_ref': 'main' if invalid == 'consumer-ref' else 'b' * 40,
                            'demo_sha': ('e' if invalid == 'requested-demo' else 'd') * 40}))
                        env = {**os.environ, 'PATH': str(tools) + os.pathsep + os.environ['PATH'],
                               'PYTHONPATH': str(source / 'scripts'),
                               'EVENT_NAME': 'workflow_dispatch', 'INPUT_TAG': 'v' + version,
                               'INPUT_CONSUMER_REF': 'b' * 40,
                               'INPUT_DEMO_REF': ('e' if invalid == 'requested-demo' else 'd') * 40,
                               'FIXTURE_RELEASE': version, 'FIXTURE_CORE': core,
                               'FIXTURE_DEMO_CORE': 'e' * 40 if invalid == 'demo-core' else core,
                               'CANDIDATE_SHA': core, 'BASEHARBOR_CONSUMER_REF': 'b' * 40,
                               'DEMO_REF': ('e' if invalid == 'requested-demo' else 'd') * 40,
                               'RUNNER_TEMP': str(root), 'GITHUB_OUTPUT': str(output),
                               'GITHUB_REF_NAME': 'runtime-validation/targeted/docker/lifecycle/fixture',
                               'GITHUB_SHA': core, 'tag': 'v' + version, 'tested_sha': core,
                               'approval_json': str(approval_json)}
                        if job == 'targeted':
                            env['EVENT_NAME'] = 'push'
                        script = step['run'].replace('${{ needs.source.outputs.tag }}', 'v' + version)
                        result = subprocess.run(['bash', '-euo', 'pipefail', '-c', script], cwd=repo,
                                                env=env, capture_output=True, text=True, timeout=30)
                        if invalid:
                            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                            if invalid != 'consumer-ref':
                                self.assertIn('immutable Core/Connector/Console/Demo candidate differs',
                                              result.stderr)
                            self.assertFalse(pins_path.exists())
                            self.assertEqual(output.read_text(), '')
                        else:
                            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                            if job == 'source':
                                self.assertEqual(output.read_text().splitlines(), [
                                    'consumer_ref=' + 'b' * 40, 'tag=v' + version,
                                    'candidate_sha=' + core, 'demo_ref=' + 'd' * 40])
                            elif job == 'targeted':
                                actual = dict(line.split('=', 1) for line in output.read_text().splitlines())
                                self.assertEqual(actual['ref'], core)
                                self.assertEqual(actual['demo_ref'], 'd' * 40)
                                self.assertEqual(actual['runtime'], 'docker')
                                self.assertEqual(actual['gate'], 'lifecycle')
                            elif job == 'publication':
                                self.assertEqual(output.read_text(), '')
                            else:
                                consumer = next(s for s in jobs[job]['steps']
                                                if 'BASEHARBOR_PUBLIC_EVIDENCE_FILE' in s.get('env', {}))
                                public_path = re.sub(
                                    r"\$\{\{ format\('([^']+)', needs.source.outputs.tag\) \}\}",
                                    lambda m: m[1].format('v' + version),
                                    consumer['env']['BASEHARBOR_PUBLIC_EVIDENCE_FILE'])
                                self.assertEqual(repo / public_path, pins_path)
                                self.assertEqual(load_private_pins(repo / public_path, public=True),
                                                 json.loads(pins_path.read_text())['gates'])
                            pins = json.loads(pins_path.read_text())
                            self.assertEqual(len(pins['gates']), 3)
                            if version == '0.4.26':
                                validate_ecosystem_graph(pins['ecosystem'])


if __name__ == '__main__':
    unittest.main()
