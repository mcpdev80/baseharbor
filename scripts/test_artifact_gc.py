"""Exercise the deployed GC script against retained and disposable origins."""
import contextlib
import io
import json
import os
import pathlib
import unittest
import urllib.request
from unittest.mock import patch

import yaml


class Response(io.BytesIO):
    def __init__(self, data=None, status=200):
        super().__init__(json.dumps(data).encode())
        self.status = status


class ArtifactGC(unittest.TestCase):
    def test_qualification_outcomes_and_branches_survive_gc(self):
        workflow = yaml.safe_load((pathlib.Path(__file__).resolve().parents[1] /
                                   '.github/workflows/artifact-gc.yml').read_text())
        shell = workflow['jobs']['repository-hygiene']['steps'][0]['run']
        source = shell.split("python3 - <<'PY'\n", 1)[1].rsplit('\nPY', 1)[0]
        prefix = 'https://api.github.com/repos/example/core/'
        core = 'runtime-validation/ha/core-bootstrap/release/podman'
        templates = 'runtime-validation/ha/bug-hunt/release-templates'
        ordinary = 'diag/disposable'
        runs = [dict(id=i, path='.github/workflows/targeted-integration.yml',
                     head_branch=branch, status='completed', conclusion=outcome,
                     updated_at='2020-01-01T00:00:00Z')
                for i, branch, outcome in [(1, core, 'success'), (2, core, 'failure'),
                                            (3, templates, 'cancelled'),
                                            (4, ordinary, 'failure')]]
        deleted = []

        def request(req):
            url = req.full_url.removeprefix(prefix)
            if req.get_method() == 'DELETE':
                deleted.append(url)
                return Response(status=204)
            if url.startswith('actions/runs?'):
                return Response({'workflow_runs': runs})
            if url.startswith('pulls?'):
                return Response([])
            if url.startswith('branches?'):
                return Response([dict(name=branch, protected=False, commit={'sha': 'a' * 40})
                                 for branch in [core, templates, ordinary]])
            if url.startswith('commits/'):
                return Response({'commit': {'committer': {'date': '2020-01-01T00:00:00Z'}}})
            raise AssertionError('Unexpected API request: ' + url)

        with patch.dict(os.environ, {'REPOSITORY': 'example/core', 'GH_TOKEN': 'test-only',
                                     'CURRENT_RUN_ID': '99'}), \
                patch.object(urllib.request, 'urlopen', request), \
                contextlib.redirect_stdout(io.StringIO()):
            exec(compile(source, 'artifact-gc.yml', 'exec'), {})
        self.assertEqual(deleted, ['actions/runs/4', 'git/refs/heads/diag%2Fdisposable'])


if __name__ == '__main__':
    unittest.main()
