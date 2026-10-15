"""Exercise the deployed GC script against retained and disposable origins."""
import contextlib
import io
import json
import os
import pathlib
import unittest
import urllib.request
import urllib.error
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

    def test_artifact_gc_retains_active_refs_and_deletes_inactive(self):
        workflow = yaml.safe_load((pathlib.Path(__file__).resolve().parents[1] /
                                   '.github/workflows/artifact-gc.yml').read_text())
        shell = workflow['jobs']['artifacts']['steps'][0]['run']
        source = shell.split("python3 - <<'PY'\n", 1)[1].rsplit('\nPY', 1)[0]
        prefix = 'https://api.github.com/repos/example/core/'
        old = '2020-01-01T00:00:00Z'
        artifacts = [
            dict(id=i, name='targeted-fixture', created_at=old,
                 workflow_run={'id': i}) for i in range(1, 5)
        ]
        branches = ['work/active-pr', 'candidate/v0.4.26', 'diag/idle']
        run_branches = {1: 'work/active-pr', 2: 'candidate/v0.4.26',
                        3: 'diag/idle', 4: 'diag/idle'}
        deleted = []

        def request(req):
            path = req.full_url.removeprefix(prefix)
            if req.get_method() == 'DELETE':
                deleted.append(path)
                return Response(status=204)
            if path.startswith('actions/artifacts?'):
                return Response({'artifacts': artifacts if path.endswith('page=1') else []})
            if path.startswith('pulls?'):
                return Response([{'head': {'ref': branches[0],
                                          'repo': {'full_name': 'example/core'}}}])
            if path.startswith('branches?'):
                return Response([{'name': name, 'protected': False} for name in branches])
            if path.startswith('actions/runs/'):
                return Response({'status': 'completed',
                                 'head_branch': run_branches[int(path.rsplit('/', 1)[-1])]})
            raise AssertionError(path)

        with patch.dict(os.environ, {'REPOSITORY': 'example/core', 'GH_TOKEN': 'test-only'}), \
                patch.object(urllib.request, 'urlopen', request), \
                contextlib.redirect_stdout(io.StringIO()):
            exec(compile(source, 'artifact-gc-artifacts.yml', 'exec'), {})
        self.assertEqual(deleted, ['actions/artifacts/3', 'actions/artifacts/4'])

    def test_gc_discovery_failure_never_deletes(self):
        workflow = yaml.safe_load((pathlib.Path(__file__).resolve().parents[1] /
                                   '.github/workflows/artifact-gc.yml').read_text())
        for job, step in [('artifacts', 0), ('repository-hygiene', 0)]:
            shell = workflow['jobs'][job]['steps'][step]['run']
            source = shell.split("python3 - <<'PY'\n", 1)[1].rsplit('\nPY', 1)[0]
            deleted = []

            def request(req):
                path = req.full_url.split('/example/core/', 1)[-1]
                if req.get_method() == 'DELETE':
                    deleted.append(path)
                    return Response(status=204)
                if path.startswith('actions/artifacts?') and not path.endswith('page=1'):
                    return Response({'artifacts': []})
                if path.startswith('actions/artifacts?'):
                    return Response({'artifacts': [{'id': 1, 'name': 'targeted-old',
                        'created_at': '2020-01-01T00:00:00Z', 'workflow_run': {'id': 1}}]})
                if path.startswith('actions/runs?'):
                    return Response({'workflow_runs': []})
                if path.startswith('pulls?'):
                    raise urllib.error.HTTPError(req.full_url, 503, 'unavailable', {}, None)
                raise AssertionError(path)

            with self.subTest(job=job), patch.dict(os.environ, {
                    'REPOSITORY': 'example/core', 'GH_TOKEN': 'test-only',
                    'CURRENT_RUN_ID': '99'}), patch.object(urllib.request, 'urlopen', request), \
                    contextlib.redirect_stdout(io.StringIO()):
                with self.assertRaises(urllib.error.HTTPError):
                    exec(compile(source, 'artifact-gc.yml', 'exec'), {})
            self.assertFalse(deleted)


    def test_candidate_sha_and_second_page_fail_closed(self):
        workflow = yaml.safe_load((pathlib.Path(__file__).resolve().parents[1] /
                                   '.github/workflows/artifact-gc.yml').read_text())
        old = '2020-01-01T00:00:00Z'
        same = 'a' * 40
        advanced = 'b' * 40
        for job in ('artifacts', 'repository-hygiene'):
            shell = workflow['jobs'][job]['steps'][0]['run']
            source = shell.split("python3 - <<'PY'\n", 1)[1].rsplit('\nPY', 1)[0]
            for case in ('reused', 'merged', 'page2-active', 'page2-failure'):
                deleted = []
                branch_name = 'candidate/v0.4.26'
                current_sha = same if case == 'merged' else advanced
                prs = ([{'merged_at': old, 'head': {'ref': branch_name,
                        'sha': same, 'repo': {'full_name': 'example/core'}}}])
                def request(req):
                    url = req.full_url.split('/example/core/', 1)[-1]
                    if req.get_method() == 'DELETE':
                        deleted.append(url)
                        return Response(status=204)
                    page2 = url.endswith('page=2')
                    if url.startswith('actions/artifacts?'):
                        return Response({'artifacts': [{'id': 9, 'name': 'targeted-fixture',
                            'created_at': old, 'workflow_run': {'id': 9}}]
                            if url.endswith('page=1') else []})
                    if url.startswith('actions/runs?'):
                        return Response({'workflow_runs': [{'id': 9, 'head_branch': branch_name,
                            'path': '.github/workflows/ci.yml', 'status': 'completed',
                            'updated_at': old}] if url.endswith('page=1') else []})
                    if url.startswith('actions/runs/'):
                        return Response({'status': 'completed', 'head_branch': branch_name})
                    if url.startswith('pulls?state=open'):
                        if case.startswith('page2'):
                            if page2 and case == 'page2-failure':
                                raise urllib.error.HTTPError(req.full_url, 503, 'failed', {}, None)
                            rows = [{'head': {'ref': branch_name, 'repo': {
                                'full_name': 'example/core'}}}] if page2 else [{}] * 100
                            return Response(rows)
                        return Response([])
                    if url.startswith('pulls?state=closed'):
                        return Response(prs if url.endswith('page=1') else [])
                    if url.startswith('branches?'):
                        rows = [{'name': branch_name, 'protected': False,
                                 'commit': {'sha': current_sha}}]
                        if case.startswith('page2'):
                            return Response(rows if page2 else [{'name': 'filler/' + str(i),
                                'protected': False, 'commit': {'sha': same}}
                                for i in range(100)])
                        return Response(rows)
                    if url.startswith('commits/'):
                        return Response({'commit': {'committer': {'date': old}}})
                    raise AssertionError(url)
                with self.subTest(job=job, case=case), patch.dict(os.environ, {
                        'REPOSITORY': 'example/core', 'GH_TOKEN': 'test-only',
                        'CURRENT_RUN_ID': '99'}), patch.object(urllib.request, 'urlopen', request), \
                        contextlib.redirect_stdout(io.StringIO()):
                    if case == 'page2-failure':
                        with self.assertRaises(urllib.error.HTTPError):
                            exec(compile(source, 'gc.yml', 'exec'), {})
                        self.assertEqual(deleted, [])
                    else:
                        exec(compile(source, 'gc.yml', 'exec'), {})
                        if case == 'merged':
                            self.assertTrue(deleted)
                        else:
                            self.assertEqual(deleted, [])


if __name__ == '__main__':
    unittest.main()
