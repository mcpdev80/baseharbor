"""Re-authenticate private integration origins without publishing their identities."""
import hashlib
import json
import os
import re
import stat
import subprocess


REPOSITORIES = {'console': 'mcpdev80/baseharbor-console',
                'connector': 'mcpdev80/baseharbor-node-connector'}
WORKFLOW = '.github/workflows/release-integration.yml'
SCHEMA = 'baseharbor.private-integration-evidence/v1'
QUALIFICATIONS = {
    'integration/docker/remote-target': {
        'production-enrollment', 'ca-overlap-renewal-revocation', 'outbound-mtls',
        'application-plan-apply-status-doctor-repair-destroy', 'secret-tls',
        'foreign-preservation', 'owned-cleanup'},
    'integration/podman/remote-target': {
        'production-enrollment', 'ca-overlap-renewal-revocation', 'outbound-mtls',
        'application-plan-apply-status-doctor-repair-destroy', 'secret-tls',
        'foreign-preservation', 'owned-cleanup'},
    'integration/static/live-console': {
        'oidc-origin-role', 'plan-apply-result-events', 'logs-follow-cancel',
        'terminal-resize-close', 'core-setup', 'application-lifecycle',
        'production-rotation', 'owned-cleanup'},
}


def commitment(value):
    raw = json.dumps(value, sort_keys=True, separators=(',', ':')).encode()
    return 'sha256:' + hashlib.sha256(raw).hexdigest()


def load_private_pins(path):
    # This file contains private source identities, never authentication tokens.
    # Reject symlinks and group/world access before opening it.
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        info = os.fstat(fd)
        if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or
                info.st_mode & 0o077 or info.st_size > 1024 * 1024):
            raise ValueError('private evidence configuration protection differs')
        with os.fdopen(fd, 'r', closefd=False) as source:
            value = json.load(source)
    finally:
        os.close(fd)
    if (not isinstance(value, dict) or
            value.get('schema') != 'baseharbor.private-evidence-pins/v1' or
            not isinstance(value.get('gates'), dict)):
        raise ValueError('private evidence configuration schema differs')
    return value['gates']


class PrivateGitHub:
    def __init__(self):
        token = os.environ.get('BASEHARBOR_PRIVATE_EVIDENCE_TOKEN')
        if not token:
            raise ValueError('authenticated private evidence access is required')
        self.environment = {**os.environ, 'GH_TOKEN': token}

    def command(self, repository, path, paginate=False):
        if repository not in REPOSITORIES.values():
            raise ValueError('private evidence repository is not trusted')
        args = ['gh', 'api'] + (['--paginate', '--slurp'] if paginate else [])
        args.append('repos/' + repository + '/' + path)
        try:
            return subprocess.check_output(args, env=self.environment, stderr=subprocess.DEVNULL)
        except (OSError, subprocess.CalledProcessError):
            # API errors must not copy private URLs, source identifiers or tokens
            # into public job logs.
            raise ValueError('private evidence origin could not be authenticated') from None

    def pages(self, repository, path, field):
        return [row for page in json.loads(self.command(repository, path, True))
                for row in page[field]]

    def archive(self, repository, artifact):
        return self.command(repository, 'actions/artifacts/' + str(artifact['id']) + '/zip')


class PrivateEvidenceVerifier:
    def __init__(self, pins, api, read_archive):
        self.pins, self.api, self.read_archive = pins, api, read_archive

    def verify(self, manifest, requirement, candidate, demo):
        key = requirement['id']
        roles = set(requirement['dependencies']) & set(REPOSITORIES)
        advertised = manifest.get('consumer_commitments')
        pins = self.pins.get(key)
        if (key not in QUALIFICATIONS or not isinstance(advertised, dict) or
                set(advertised) != roles or not isinstance(pins, dict) or set(pins) != roles):
            raise ValueError('private consumer commitments or required roles differ')
        for role in sorted(roles):
            self.verify_role(role, key, pins[role], advertised[role], candidate, demo)

    def verify_role(self, role, gate, pin, advertised, candidate, demo):
        if (not isinstance(pin, dict) or set(pin) != {'repository', 'commit', 'workflow'} or
                pin.get('repository') != REPOSITORIES[role] or pin.get('workflow') != WORKFLOW or
                not isinstance(pin.get('commit'), str) or not re.fullmatch('[0-9a-f]{40}', pin['commit']) or
                advertised != commitment(pin)):
            raise ValueError('private consumer source commitment differs')
        repository, source = pin['repository'], pin['commit']
        runs = self.api.pages(repository,
                              'actions/workflows/release-integration.yml/runs?per_page=100&head_sha=' + source,
                              'workflow_runs')
        # Resolve the newest exact-source push before looking at its outcome.
        # In-progress, failed and cancelled newer runs cannot borrow old success.
        matching = [run for run in runs if run.get('head_sha') == source and
                    run.get('event') == 'push' and run.get('path') == WORKFLOW and
                    run.get('repository', {}).get('full_name') == repository and
                    run.get('head_repository', {}).get('full_name') == repository]
        if not matching:
            raise ValueError('private consumer has no trusted exact-source push origin')
        run = max(matching, key=lambda row: row['id'])
        attempt = run.get('run_attempt')
        if (type(run.get('id')) is not int or type(attempt) is not int or attempt < 1 or
                run.get('status') != 'completed'):
            raise ValueError('latest private consumer run is not completed')
        jobs = self.api.pages(repository,
                              'actions/runs/' + str(run['id']) + '/jobs?per_page=100&filter=all', 'jobs')
        matching_jobs = [job for job in jobs if job.get('name') == 'Integration · ' + gate and
                         job.get('run_attempt') == attempt]
        if len(matching_jobs) != 1:
            raise ValueError('latest private consumer attempt needs one qualification job')
        job = matching_jobs[0]
        if (job.get('head_sha') != source or type(job.get('id')) is not int or
                job.get('status') != 'completed' or job.get('conclusion') != 'success'):
            raise ValueError('latest private consumer qualification job did not succeed')
        name = 'private-integration-' + gate.replace('/', '-') + '-' + str(run['id']) + '-' + str(attempt)
        artifacts = self.api.pages(repository,
                                   'actions/runs/' + str(run['id']) + '/artifacts?per_page=100', 'artifacts')
        matches = [artifact for artifact in artifacts if artifact.get('name') == name]
        if len(matches) != 1 or matches[0].get('expired') is not False:
            raise ValueError('private consumer artifact is missing, duplicated or expired')
        artifact = matches[0]
        origin = artifact.get('workflow_run', {})
        if origin.get('id') != run['id'] or origin.get('head_sha') != source:
            raise ValueError('private consumer artifact source binding differs')
        receipt, _ = self.read_archive(self.api.archive(repository, artifact), artifact.get('digest'))
        expected = {'schema': SCHEMA, 'repository': repository, 'consumer_commit': source,
                    'core_commit': candidate, 'demo_commit': demo, 'role': role, 'gate': gate,
                    'workflow_run_id': str(run['id']), 'workflow_run_attempt': attempt,
                    'job_id': job['id'], 'result': 'success', 'cleanup_result': 'success',
                    'release_eligible': True, 'production_authority': True}
        if (any(receipt.get(field) != value or type(receipt.get(field)) is not type(value)
                for field, value in expected.items()) or
                not isinstance(receipt.get('qualifications'), dict) or
                any(receipt['qualifications'].get(item) is not True for item in QUALIFICATIONS[gate]) or
                not isinstance(receipt.get('build_sha256'), str) or
                not re.fullmatch('[0-9a-f]{64}', receipt['build_sha256'])):
            raise ValueError('private consumer receipt binding, scope or cleanup differs')
        # Authenticated private archives and IDs stay private. The public
        # coverage stores only its candidate-owned immutable commitment.
