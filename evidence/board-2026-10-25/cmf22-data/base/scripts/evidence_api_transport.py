#!/usr/bin/env python3
"""Bounded read transport for the unchanged release evidence verifier."""
import importlib.util
import json
import pathlib
import re
import subprocess
import sys
import time
from urllib.parse import parse_qsl, urlencode


class GitHub:
    repository = 'mcpdev80/baseharbor'

    def __init__(self, repository):
        if repository != self.repository:
            raise ValueError('evidence repository is not trusted')

    def request(self, path):
        args = ['gh', 'api', '-H', 'X-GitHub-Api-Version: 2026-03-10',
                f'repos/{self.repository}/{path}']
        for attempt in range(4):
            try:
                return subprocess.check_output(args, stderr=subprocess.PIPE, timeout=60)
            except subprocess.CalledProcessError as error:
                # Authentication, authorization and missing evidence are never retried.
                transient = re.search(rb'HTTP (?:500|502|503|504)\b', error.stderr or b'')
                if not transient or attempt == 3:
                    raise
            except subprocess.TimeoutExpired:
                if attempt == 3:
                    raise
            print(f'Evidence API retry {attempt + 1}/3: {path}', file=sys.stderr)
            time.sleep(2 ** (attempt + 1))
        raise AssertionError('unreachable')

    def get(self, path):
        return json.loads(self.request(path))

    def page_rows(self, path, field):
        endpoint, _, query = path.partition('?')
        params = dict(parse_qsl(query))
        params['per_page'] = '10'
        rows, seen, total = [], set(), None
        for page in range(1, 1001):
            params['page'] = str(page)
            response = self.get(endpoint + '?' + urlencode(params))
            count = response.get('total_count')
            batch = response.get(field)
            if (type(count) is not int or count < 0 or count > 10000 or
                    not isinstance(batch, list) or len(batch) > 10 or
                    (total is not None and count != total)):
                raise ValueError('evidence API pagination is incomplete or unstable')
            total = count
            for row in batch:
                identity = row.get('id') if isinstance(row, dict) else None
                if type(identity) is not int or identity < 1 or identity in seen:
                    raise ValueError('evidence API duplicate or invalid row')
                seen.add(identity)
                rows.append(row)
            if len(rows) == total:
                return rows
            if not batch or len(rows) > total:
                raise ValueError('evidence API pagination ended before complete coverage')
        raise ValueError('evidence API pagination exceeds bound')

    def pages(self, path, field):
        match = re.fullmatch(r'actions/runs/([1-9][0-9]*)/jobs\?filter=all&per_page=(?:30|100)', path)
        if field != 'jobs' or match is None:
            return self.page_rows(path, field)
        run_id = int(match[1])
        run = self.get(f'actions/runs/{run_id}')
        attempts = run.get('run_attempt')
        if run.get('id') != run_id or type(attempts) is not int or not 1 <= attempts <= 100:
            raise ValueError('evidence API run attempt inventory is invalid')
        rows = []
        for attempt in range(1, attempts + 1):
            batch = self.page_rows(f'actions/runs/{run_id}/attempts/{attempt}/jobs', field)
            if any(job.get('run_id') != run_id or job.get('run_attempt') != attempt for job in batch):
                raise ValueError('evidence API job run or attempt differs')
            rows.extend(batch)
            print(f'Evidence API: run {run_id}, attempt {attempt}/{attempts}, {len(batch)} jobs', file=sys.stderr)
        if len({row['id'] for row in rows}) != len(rows):
            raise ValueError('evidence API job duplicated across attempts')
        return rows

    def archive(self, artifact):
        return self.request(f'actions/artifacts/{artifact["id"]}/zip')


def main():
    # Keep candidate-owned input fingerprints, coverage, origin, ZIP, secret and
    # private-consumer verification exactly as reviewed. Replace read transport only.
    path = pathlib.Path(__file__).with_name('pre-release-resume.py')
    spec = importlib.util.spec_from_file_location('evidence_verifier', path)
    verifier = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(verifier)
    verifier.GitHub = GitHub
    verifier.main()


if __name__ == '__main__':
    main()
