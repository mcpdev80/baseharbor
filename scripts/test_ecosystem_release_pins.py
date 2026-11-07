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
from consumer_source_pins import load_source_pins


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
    def setUp(self):
        self.source = pathlib.Path(__file__).resolve().parents[1]
        self.jobs = yaml.safe_load((self.source / '.github/workflows/pre-release.yml').read_text())['jobs']

    def test_final_check_waits_for_every_required_test_job(self):
        requirements = json.loads((self.source / 'scripts/release-requirements.json').read_text())['gates']
        producers = {job for gate in requirements if 'v0.4.26' in gate['required_for'] for job in gate['jobs']}
        pending, ancestors = list(self.jobs['checks']['needs']), set()
        while pending:
            job = pending.pop()
            if job in ancestors:
                continue
            ancestors.add(job)
            needs = self.jobs[job].get('needs', [])
            pending.extend([needs] if isinstance(needs, str) else needs)
        self.assertTrue(producers <= ancestors, sorted(producers - ancestors))

    def test_current_scheduler_results_fail_on_missing_failed_or_skipped_required_job(self):
        finalizer = self.jobs['checks']['steps'][0]['run']
        outputs = {key: '0' for key in ['adoption_count', 'static_count', 'light_docker_count',
                   'light_podman_count', 'heavy_docker_count', 'heavy_podman_count',
                   'ha_docker_count', 'ha_podman_count', 'journey_count']}
        outputs['light_docker_count'] = '1'
        outputs['tag'] = 'v0.4.26'
        outputs['integration'] = '[]'
        required = {'source', 'runtime_candidate', 'runtime_light_gates', 'runtime_heavy_gates',
                    'v0421_ha_groups', 'runtime_light_gates_docker'}
        jobs = {key: {'result': 'success' if key in required else 'skipped'}
                for key in self.jobs['checks']['needs']}
        jobs['source']['outputs'] = outputs
        for result in ['success', 'failure', 'cancelled', 'skipped', None]:
            candidate = copy.deepcopy(jobs)
            if result is None:
                candidate.pop('runtime_light_gates_docker')
            else:
                candidate['runtime_light_gates_docker']['result'] = result
            completed = subprocess.run(['bash', '-euo', 'pipefail', '-c', finalizer],
                env={**os.environ, 'JOB_RESULTS':json.dumps(candidate)}, capture_output=True, text=True)
            self.assertEqual(completed.returncode == 0, result == 'success', completed.stderr)

    def test_v0427_native_greenfield_matrix_has_eight_real_cli_cells(self):
        workflow = yaml.safe_load((self.source / '.github/workflows/nextjs-greenfield-e2e.yml').read_text())
        matrix = workflow['jobs']['greenfield-native']['strategy']['matrix']
        self.assertEqual(set(matrix['runtime']), {'docker', 'podman'})
        self.assertEqual(set(matrix['stack']), {'go', 'nextjs', 'python', 'quarkus'})
        self.assertEqual(len(matrix['runtime']) * len(matrix['stack']), 8)
        commands = '\n'.join(step.get('run', '') for step in workflow['jobs']['greenfield-native']['steps'])
        for command in ('app new ', 'ci-greenfield-apply.py', ' app status', ' app doctor',
                        'test -s baseharbor.yaml', 'grep -qi'):
            self.assertIn(command, commands)
        apply_script = (self.source / 'scripts/ci-greenfield-apply.py').read_text()
        for guard in ('"impact-confirmation-required"', '"destructive"', '"confirmations_required"',
                      '"plan_id"', '"--confirm-candidate"', '"up", "--yes"'):
            self.assertIn(guard, apply_script)
        pre_release = self.jobs['greenfield_native']
        self.assertEqual(pre_release['uses'], './.github/workflows/nextjs-greenfield-e2e.yml')
        self.assertIn('needs.source.outputs.candidate_sha', pre_release['with']['candidate_ref'])
        self.assertIn('greenfield_native', self.jobs['checks']['needs'])

    def test_v0427_greenfield_is_required_and_never_skip_green(self):
        finalizer = self.jobs['checks']['steps'][0]['run']
        outputs = {key: '0' for key in ['adoption_count', 'static_count', 'light_docker_count',
                   'light_podman_count', 'heavy_docker_count', 'heavy_podman_count',
                   'ha_docker_count', 'ha_podman_count', 'journey_count']}
        outputs['integration'] = '[]'
        outputs['tag'] = 'v0.4.27'
        required = {'source', 'runtime_light_gates', 'runtime_heavy_gates',
                    'v0421_ha_groups', 'greenfield_native'}
        jobs = {key: {'result': 'success' if key in required else 'skipped'}
                for key in self.jobs['checks']['needs']}
        jobs['source']['outputs'] = outputs
        for result in ['success', 'failure', 'cancelled', 'skipped', None]:
            candidate = copy.deepcopy(jobs)
            if result is None:
                candidate.pop('greenfield_native')
            else:
                candidate['greenfield_native']['result'] = result
            completed = subprocess.run(['bash', '-euo', 'pipefail', '-c', finalizer],
                env={**os.environ, 'JOB_RESULTS': json.dumps(candidate), 'ONLY_GATES': ''},
                capture_output=True, text=True)
            self.assertEqual(completed.returncode == 0, result == 'success', completed.stderr)

    def test_release_executes_checks_on_the_exact_tag_instead_of_reusing_old_runs(self):
        release = yaml.safe_load((self.source / '.github/workflows/release.yml').read_text())
        jobs = release['jobs']
        self.assertEqual(jobs['checks']['uses'], './.github/workflows/pre-release.yml')
        self.assertEqual(jobs['checks']['with']['ref'], '${{ needs.tag.outputs.tag_sha }}')
        for job in ['runtime-image', 'release']:
            self.assertIn('checks', jobs[job]['needs'])
        self.assertNotIn('approval', jobs)
        self.assertNotIn('evidence', jobs)
        events = release.get('on', release.get(True))
        self.assertEqual(events['push'], {'tags': ['v*']})
        self.assertNotIn('pre_release_run_id', events['workflow_dispatch']['inputs'])


if __name__ == '__main__':
    unittest.main()
