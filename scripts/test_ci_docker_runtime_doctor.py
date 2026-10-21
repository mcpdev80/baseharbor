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
                   {'daemon_id': ''}, {'endpoint': 'tcp://localhost:2375'},
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


if __name__ == '__main__':
    unittest.main()
