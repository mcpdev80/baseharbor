#!/usr/bin/env python3
"""Plan and verify exact gate coverage with immutable, input-equivalent origins."""
import argparse
import functools
import hashlib
import io
import math
import json
import os
import pathlib
import re
import stat
import subprocess
import zipfile

import yaml

STATIC = ['config-matrix', 'init', 'mcp', 'agent', 'shell-ux']
LIGHT = ['guided', 'lifecycle', 'policy', 'connectivity', 'reconciliation', 'failure', 'full-destroy']
HEAVY = ['data-capabilities', 'observability', 'identity', 'security', 'backup-restore',
         'rabbitmq-runtime', 'mongodb-runtime', 'durable-key-value', 'control-plane-recovery',
         'loki-runtime', 'tempo-runtime']
ADOPTION = ['conformance', 'realworld-compose', 'realworld-quadlet', 'realworld-kubernetes']
HA = ['data', 'identity', 'observability', 'routing']
REQUIREMENTS_PATH = 'scripts/release-requirements.json'


def load_requirements(raw, tag):
    inventory = json.loads(raw)
    if (inventory.get('schema') != 'baseharbor.release-requirements/v1' or
            tag not in inventory.get('releases', [])):
        raise ValueError('release requirements schema or release differs')
    gates, seen = [], set()
    for gate in inventory.get('gates', []):
        key = gate.get('id', '')
        if (not re.fullmatch(r'(atomic|adoption|ha|journey|integration)/[a-z0-9/-]+', key) or
                key in seen or not gate.get('jobs') or not gate.get('dependencies') or
                not gate.get('schema') or not gate.get('required_for')):
            raise ValueError('invalid or duplicate release requirement')
        seen.add(key)
        if tag in gate['required_for']:
            gates.append(gate)
    if not gates:
        raise ValueError('empty release requirements')
    return gates


def local_requirements(tag):
    return load_requirements(pathlib.Path(__file__).with_name('release-requirements.json').read_text(), tag)


# Compatibility callers use the explicit released baseline; CLI selects the candidate's set.
EXPECTED = [gate['id'] for gate in local_requirements('v0.4.22')]
TRUSTED = {'.github/workflows/pre-release.yml', '.github/workflows/targeted-demo-acceptance.yml'}
# Only published prose and origin lists are metadata. Workflow execution, evidence
# tooling, schemas, policy/auth and build inputs always invalidate prior evidence.
METADATA = {'CHANGELOG.md', 'docs/roadmap.md',
            'docs/development/v0.4.22-evidence-resume.md',
            'docs/development/v0.4.20-release-runbook.md'}


def metadata_path(path):
    return path in METADATA or (path.startswith('docs/releases/') and
                                path.endswith(('.md', '.evidence-runs')))


SCHEMAS = {'atomic': 'baseharbor.pre-release.gate-evidence/v1',
           'adoption': 'baseharbor.pre-release.adoption-evidence/v1',
           'ha': 'baseharbor.pre-release.v0.4.21-ha-evidence/v1',
           'journey': 'baseharbor.pre-release.reference-journey/v1',
           'integration': 'baseharbor.pre-release.integration-evidence/v1'}


def command(args, cwd=None):
    return subprocess.check_output(args, cwd=cwd)


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


class GitInputs:
    def __init__(self, product, demo):
        self.product, self.demo = pathlib.Path(product), pathlib.Path(demo)

    @functools.lru_cache(maxsize=None)
    def objects(self, repo, commit):
        if not re.fullmatch('[0-9a-f]{40}', commit):
            raise ValueError('input commit must be an exact SHA')
        command(['git', 'cat-file', '-e', commit + '^{commit}'], repo)
        entries = command(['git', 'ls-tree', '-rz', commit], repo).split(b'\0')
        return [(path.decode(), meta.decode()) for entry in entries if entry
                for meta, path in [entry.split(b'\t', 1)]]

    @functools.lru_cache(maxsize=None)
    def workflow(self, commit):
        raw = command(['git', 'show', commit + ':.github/workflows/pre-release.yml'], self.product)
        return yaml.safe_load(raw)['jobs']

    @functools.lru_cache(maxsize=None)
    def requirements(self, candidate, tag):
        paths = {path for path, _ in self.objects(self.product, candidate)}
        if REQUIREMENTS_PATH not in paths:
            if tag == 'v0.4.22':
                return local_requirements(tag)
            raise ValueError('candidate release requirements are missing')
        raw = command(['git', 'show', candidate + ':' + REQUIREMENTS_PATH], self.product)
        return load_requirements(raw, tag)

    def fingerprint(self, candidate, demo, tag, key):
        product = [(path, obj) for path, obj in self.objects(self.product, candidate)
                   if not metadata_path(path) and path != '.github/workflows/pre-release.yml']
        demo_objects = [(path, obj) for path, obj in self.objects(self.demo, demo)
                        if not (path == 'tests/mcp/run.sh' and key != 'atomic/static/mcp'
                                and not key.startswith('journey/'))]
        requirements = self.requirements(candidate, tag)
        requirement = next((gate for gate in requirements if gate['id'] == key), None)
        if requirement is None:
            raise ValueError('gate is outside candidate release requirements')
        jobs = self.workflow(candidate)
        # Only scheduling edges and fan-out may change without new evidence.
        # Static/reusable call arguments, all build steps and evidence generation
        # are execution inputs. Keep unrelated job contracts conservatively too.
        execution = {}
        for name, job in jobs.items():
            execution[name] = {field: value for field, value in job.items()
                               if field not in {'needs', 'if', 'strategy'}}
            strategy = {field: value for field, value in job.get('strategy', {}).items()
                        if field != 'matrix'}
            if strategy:
                execution[name]['strategy'] = strategy
        return digest({'policy': 'baseharbor.gate-inputs/v2', 'tag': tag, 'gate': key,
                       'requirement': requirement, 'product': product, 'demo': demo_objects,
                       'execution': execution})

    def ensure(self, candidate, demo):
        for repo, sha in [(self.product, candidate), (self.demo, demo)]:
            if not re.fullmatch('[0-9a-f]{40}', sha):
                raise ValueError('invalid source commit')
            if subprocess.run(['git', 'cat-file', '-e', sha + '^{commit}'], cwd=repo,
                              stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode:
                command(['git', 'fetch', '--no-tags', 'origin', sha], repo)

    def pin(self, candidate, tag):
        return command(['git', 'show', candidate + ':docs/releases/' + tag + '.demo-ref'],
                       self.product).decode().strip()


def key_from_job(name, expected=None):
    for key in EXPECTED if expected is None else expected:
        parts = key.split('/')
        kind = parts[0]
        if kind == 'adoption' and name == 'Adoption · ' + parts[1]:
            return key
        if kind == 'ha' and name == f'v0.4.21 HA · {parts[2]} · {parts[1]}':
            return key
        if kind == 'journey' and name == f'Reference Journey · {parts[1]} · Full E2E':
            return key
        if kind == 'integration' and name == 'Integration · ' + '/'.join(parts):
            return key
        if kind == 'atomic':
            runtime, gate = parts[1:]
            suffix = f'/ {runtime} · {gate}'
            if name.endswith(suffix) and (name.startswith(('Static · ', 'Light · ', 'Heavy · '))
                                          or name.startswith('selected_gate ')):
                return key
    return None


def artifact_name(key, run, attempt):
    parts = key.split('/')
    if parts[0] == 'atomic':
        prefix = f'pre-release-gate-{parts[1]}-{parts[2]}-evidence'
    elif parts[0] == 'adoption':
        prefix = f'pre-release-adoption-{parts[1]}-evidence'
    elif parts[0] == 'ha':
        prefix = f'pre-release-v0421-ha-{parts[1]}-{parts[2]}-evidence'
    elif parts[0] == 'integration':
        prefix = f'pre-release-integration-{parts[1]}-{parts[2]}-evidence'
    else:
        prefix = f'pre-release-reference-journey-evidence-{parts[1]}'
    return f'{prefix}-{run}-{attempt}'


def validate_manifest(manifest, key, run, candidate, demo, attempt):
    kind, *parts = key.split('/')
    runtime = parts[0] if kind != 'adoption' else 'static'
    if (manifest.get('schema') != SCHEMAS[kind] or manifest.get('candidate_sha') != candidate or
            manifest.get('demo_ref') != demo or
            manifest.get('workflow_run_id') != (run if kind == 'journey' else str(run)) or
            type(manifest.get('workflow_run_attempt')) is not int or
            manifest['workflow_run_attempt'] != attempt or manifest.get('outcome') != 'success' or
            manifest.get('cleanup_outcome') != ('skipped' if runtime == 'static' else 'success')):
        raise ValueError('manifest outcome, schema, source, attempt or cleanup differs')
    if kind == 'integration' and (manifest.get('id') != '/'.join(parts) or
                                  manifest.get('runtime') != runtime or manifest.get('gate') != parts[1]):
        raise ValueError('integration logical identity differs')
    if kind == 'atomic' and (manifest.get('id') != '/'.join(parts) or
                            manifest.get('runtime') != runtime or manifest.get('gate') != parts[1]):
        raise ValueError('atomic logical identity differs')
    if kind == 'adoption' and manifest.get('gate') != parts[0]:
        raise ValueError('adoption logical identity differs')
    if kind == 'ha' and (manifest.get('runtime') != runtime or manifest.get('group') != parts[1]):
        raise ValueError('HA logical identity differs')
    if kind == 'journey' and (manifest.get('runtime') != runtime or
                             type(manifest.get('duration_seconds')) not in [int, float] or
                             not math.isfinite(manifest['duration_seconds']) or manifest['duration_seconds'] < 0):
        raise ValueError('journey runtime or duration differs')


def read_archive(data, expected_digest):
    if len(data) > 128 * 1024 * 1024 or 'sha256:' + hashlib.sha256(data).hexdigest() != expected_digest:
        raise ValueError('artifact archive digest or size differs')
    archive = zipfile.ZipFile(io.BytesIO(data))
    infos = archive.infolist()
    if len(infos) > 8000 or sum(info.file_size for info in infos) > 256 * 1024 * 1024:
        raise ValueError('artifact archive expansion exceeds limit')
    names = set()
    for info in infos:
        path = pathlib.PurePosixPath(info.filename)
        if (path.is_absolute() or '..' in path.parts or '\\' in info.filename or
                stat.S_ISLNK(info.external_attr >> 16) or info.filename in names):
            raise ValueError('unsafe artifact archive entry')
        names.add(info.filename)
    if 'manifest.json' not in names:
        raise ValueError('artifact root manifest is missing')
    raw = archive.read('manifest.json')
    if len(raw) > 1024 * 1024:
        raise ValueError('manifest exceeds limit')
    return json.loads(raw), hashlib.sha256(raw).hexdigest()


class GitHub:
    def __init__(self, repository):
        if repository != 'mcpdev80/baseharbor':
            raise ValueError('evidence repository is not trusted')
        self.repository = repository

    def get(self, path):
        return json.loads(command(['gh', 'api', f'repos/{self.repository}/{path}']))

    def pages(self, path, field):
        pages = json.loads(command(['gh', 'api', '--paginate', '--slurp',
                                    f'repos/{self.repository}/{path}']))
        return [row for page in pages for row in page[field]]

    def archive(self, artifact):
        return command(['gh', 'api', f'repos/{self.repository}/actions/artifacts/{artifact["id"]}/zip'])


def collect(api, inputs, candidate, demo, tag, run_ids, output, current_run=None):
    inputs.ensure(candidate, demo)
    if inputs.pin(candidate, tag) != demo:
        raise ValueError('target demo pin differs')
    requirements = inputs.requirements(candidate, tag)
    expected = [gate['id'] for gate in requirements]
    requirements_digest = digest(requirements)
    target = {key: inputs.fingerprint(candidate, demo, tag, key) for key in expected}
    latest, pending = {}, {}
    for run_id in sorted(set(run_ids)):
        run = api.get(f'actions/runs/{run_id}')
        if (run.get('path') not in TRUSTED or run.get('event') != 'push' or
                run.get('head_repository', {}).get('full_name') != api.repository or
                run.get('repository', {}).get('full_name') != api.repository or
                (run_id != current_run and run.get('status') != 'completed')):
            raise ValueError(f'run {run_id} is not an immutable trusted push origin')
        origin_candidate = run['head_sha']
        inputs.ensure(origin_candidate, demo)
        origin_demo = inputs.pin(origin_candidate, tag)
        inputs.ensure(origin_candidate, origin_demo)
        artifacts = api.pages(f'actions/runs/{run_id}/artifacts?per_page=100', 'artifacts')
        jobs = api.pages(f'actions/runs/{run_id}/jobs?filter=all&per_page=100', 'jobs')
        for job in jobs:
            key = key_from_job(job['name'], expected)
            if key is None or inputs.fingerprint(origin_candidate, origin_demo, tag, key) != target[key]:
                continue
            attempt = job.get('run_attempt')
            if type(attempt) is not int or attempt < 1 or job.get('head_sha') != origin_candidate:
                raise ValueError('job attempt or source binding differs')
            order = (run_id, attempt, job['id'])
            if key in latest and order <= latest[key][0]:
                continue
            # Record failed/missing/cancelled observations before considering success.
            latest[key] = (order, None)
            pending[key] = f'run {run_id} attempt {attempt}: {job.get("conclusion") or job.get("status")}'
            if job.get('status') != 'completed' or job.get('conclusion') != 'success':
                continue
            matches = [a for a in artifacts if a['name'] == artifact_name(key, run_id, attempt)]
            if len(matches) != 1 or matches[0].get('expired'):
                pending[key] = f'run {run_id}: missing, duplicate or expired artifact'
                continue
            artifact = matches[0]
            if (artifact.get('workflow_run', {}).get('id') != run_id or
                    artifact.get('workflow_run', {}).get('head_sha') != origin_candidate):
                raise ValueError('artifact source run or commit differs')
            data = api.archive(artifact)
            manifest, manifest_digest = read_archive(data, artifact.get('digest'))
            validate_manifest(manifest, key, run_id, origin_candidate, origin_demo, attempt)
            requirement = next(gate for gate in requirements if gate['id'] == key)
            if key.startswith('integration/'):
                if manifest.get('requirements_digest') != requirements_digest:
                    raise ValueError('integration manifest requirement set differs')
                if set(requirement['dependencies']) & {'console', 'connector'}:
                    raise ValueError('authenticated private consumer evidence verifier is not implemented')
            origin = {'repository': api.repository, 'workflow': run['path'], 'run_id': run_id,
                      'attempt': attempt, 'job_id': job['id'], 'artifact_id': artifact['id'],
                      'artifact_name': artifact['name'], 'archive_digest': artifact['digest'],
                      'manifest_digest': manifest_digest, 'candidate_sha': origin_candidate,
                      'demo_ref': origin_demo}
            output.mkdir(parents=True, exist_ok=True)
            # Store original ZIP bytes, never rewrite source manifests as new candidates.
            (output / f'{artifact["id"]}.zip').write_bytes(data)
            zipfile.ZipFile(io.BytesIO(data)).extractall(output / str(artifact['id']))
            latest[key] = (order, {'gate': key, 'input_digest': target[key],
                                   'origin': origin, 'manifest': manifest})
            pending.pop(key, None)
    proofs = [latest[key][1] for key in expected if key in latest and latest[key][1] is not None]
    missing = [key for key in expected if key not in latest or latest[key][1] is None]
    return {'schema': 'baseharbor.pre-release.coverage/v2', 'candidate_sha': candidate,
            'demo_ref': demo, 'tag': tag, 'required': expected, 'requirements_digest': requirements_digest,
            'proofs': proofs,
            'pending': {key: pending.get(key, 'no compatible completed proof') for key in missing}}


def matrices(coverage):
    missing = set(coverage['pending'])
    result = {'adoption': [g for g in ADOPTION if f'adoption/{g}' in missing],
              'static': [g for g in STATIC if f'atomic/static/{g}' in missing],
              'journey': [r for r in ['docker', 'podman'] if f'journey/{r}' in missing]}
    for runtime in ['docker', 'podman']:
        for name, gates in [('light', LIGHT), ('heavy', HEAVY), ('ha', HA)]:
            prefix = 'ha' if name == 'ha' else 'atomic'
            result[name + '_' + runtime] = [g for g in gates if f'{prefix}/{runtime}/{g}' in missing]
    return result


def verify_coverage(coverage, candidate, demo, tag, requirements=None):
    requirements = local_requirements(tag) if requirements is None else requirements
    expected = [gate['id'] for gate in requirements]
    if (coverage.get('schema') != 'baseharbor.pre-release.coverage/v2' or
            coverage.get('candidate_sha') != candidate or coverage.get('demo_ref') != demo or
            coverage.get('tag') != tag or coverage.get('required') != expected or coverage.get('pending') or
            (coverage.get('requirements_digest') != digest(requirements) and
             not (tag == 'v0.4.22' and 'requirements_digest' not in coverage))):
        raise ValueError('coverage target or required set differs, or pending gates remain')
    proofs = coverage.get('proofs', [])
    if len(proofs) != len(expected) or sorted(p['gate'] for p in proofs) != sorted(expected):
        raise ValueError('coverage requires every unique requirement proof')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['plan', 'approve', 'check-approval'])
    parser.add_argument('--candidate', required=True)
    parser.add_argument('--demo', required=True)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--demo-repo', required=True, type=pathlib.Path)
    parser.add_argument('--output', required=True, type=pathlib.Path)
    parser.add_argument('--runs')
    parser.add_argument('--repository', default=os.getenv('GITHUB_REPOSITORY', 'mcpdev80/baseharbor'))
    args = parser.parse_args()
    api = GitHub(args.repository)
    inputs = GitInputs(pathlib.Path.cwd(), args.demo_repo)
    requirements = inputs.requirements(args.candidate, args.tag)
    current = int(os.environ['GITHUB_RUN_ID'])
    if args.mode == 'check-approval':
        approved = json.loads((args.output / 'evidence-coverage.json').read_text())
        verify_coverage(approved, args.candidate, args.demo, args.tag, requirements)
        runs = sorted({p['origin']['run_id'] for p in approved['proofs']})
    else:
        path = pathlib.Path(args.runs or f'docs/releases/{args.tag}.evidence-runs')
        runs = json.loads(path.read_text()) if path.exists() else []
        if not isinstance(runs, list) or any(type(r) is not int or r < 1 for r in runs) or len(runs) > 20:
            raise ValueError('resume origins must be at most 20 positive run ids')
        if args.mode == 'approve':
            runs.append(current)
    coverage = collect(api, inputs, args.candidate, args.demo, args.tag, runs,
                       args.output / 'origins', current_run=current)
    if args.mode == 'check-approval':
        verify_coverage(coverage, args.candidate, args.demo, args.tag, requirements)
        if coverage != approved:
            raise ValueError('approved evidence differs from authenticated immutable origins')
        print(f'All {len(requirements)} approved proofs match immutable authenticated origins and target inputs.')
        return
    args.output.mkdir(parents=True, exist_ok=True)
    (args.output / 'evidence-coverage.json').write_text(json.dumps(coverage, indent=2) + '\n')
    if args.mode == 'approve':
        verify_coverage(coverage, args.candidate, args.demo, args.tag, requirements)
        approval = {'schema': 'baseharbor.pre-release.approval/v2', 'tag': args.tag,
                    'candidate_sha': args.candidate, 'tested_sha': args.candidate, 'demo_sha': args.demo,
                    'workflow': 'pre-release.yml', 'workflow_run_id': current,
                    'workflow_run_attempt': int(os.environ['GITHUB_RUN_ATTEMPT']),
                    'gate_count': len(requirements), 'requirements_digest': digest(requirements),
                    'gate_evidence_schema': 'baseharbor.pre-release.coverage/v2', 'result': 'success',
                    'coverage_digest': digest(coverage)}
        (args.output / 'release-approved.json').write_text(json.dumps(approval, indent=2) + '\n')
    else:
        unscheduled = [key for key in coverage['pending'] if key.startswith('integration/')]
        if unscheduled:
            raise ValueError('required integration gates are not wired for scheduling: ' + ', '.join(unscheduled))
        with open(os.environ['GITHUB_OUTPUT'], 'a') as stream:
            for name, gates in matrices(coverage).items():
                stream.write(f'{name}={json.dumps(gates, separators=(",", ":"))}\n{name}_count={len(gates)}\n')
    print(f'{len(coverage["proofs"])} authenticated proofs; {len(coverage["pending"])} gates pending.')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError, zipfile.BadZipFile) as error:
        raise SystemExit(f'evidence resume rejected: {error}')
