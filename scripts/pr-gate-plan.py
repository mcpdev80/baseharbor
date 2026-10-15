#!/usr/bin/env python3
"""Conservative v0.4.26 PR area-to-existing-gate selection (#892)."""
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent

DOC_CHECKS = [
    {'id': 'documentation-audit', 'owner': 'scripts/documentation-audit.sh',
     'command': 'bash scripts/documentation-audit.sh'},
    {'id': 'mkdocs-en-strict', 'owner': '.github/workflows/pages.yml:build',
     'command': 'mkdocs build --strict --config-file mkdocs.yml'},
    {'id': 'mkdocs-de-strict', 'owner': '.github/workflows/pages.yml:build',
     'command': 'mkdocs build --strict --config-file mkdocs.de.yml'},
    {'id': 'pages-candidate-build', 'owner': '.github/workflows/pages.yml:build',
     'command': 'build candidate documentation without deploying Pages'},
]


def validated_diff(diff):
    if not isinstance(diff, dict):
        raise ValueError('diff-base result must be an object')
    problems = []
    if type(diff.get('broad_required')) is not bool:
        problems.append('broad_required must be an explicit boolean')
    changes = diff.get('changes')
    if not isinstance(changes, list) or any(not isinstance(c, dict) for c in changes):
        problems.append('changes must be an array of objects')
    reasons = diff.get('reasons')
    if not isinstance(reasons, list) or any(
            not isinstance(r, dict) or not isinstance(r.get('code'), str) or
            not isinstance(r.get('message'), str) for r in reasons):
        problems.append('reasons must be an array of code/message objects')
    for key in ('target_base_sha', 'head_sha', 'merge_base_sha'):
        value = diff.get(key)
        if not (isinstance(value, str) and len(value) == 40 and
                all(c in '0123456789abcdef' for c in value)):
            problems.append(key + ' must be a full lowercase commit SHA')
    integration = diff.get('integration_sha')
    if integration is not None and not (
            isinstance(integration, str) and len(integration) == 40 and
            all(c in '0123456789abcdef' for c in integration)):
        problems.append('integration_sha must be null or a full lowercase commit SHA')
    # Resolver fallbacks may legitimately omit an unavailable SHA. Never
    # reject those or turn them into an eligible narrow plan.
    if problems:
        return False, problems
    return True, []



# Validation checks are independent of release-gate requirements. A legitimate
# PR plan may require validations and select zero native release gates.
CODE_CHECKS = [
    {'id': 'source-contracts', 'owner': 'CI source / contracts',
     'command': 'go test ./internal/application/... ./contracts/...'},
]
CLI_CHECKS = [
    {'id': 'cli-guided-tests', 'owner': 'cmd/baha guided read-only adapter',
     'command': "go test ./cmd/baha -run '^TestGuided' -count=1"},
    {'id': 'cli-vet', 'owner': 'cmd/baha package', 'command': 'go vet ./cmd/baha'},
    {'id': 'cli-build', 'owner': 'cmd/baha package', 'command': 'go build ./cmd/baha'},
]
# Narrow, audited read-only file allowlist. Lifecycle, backup/restore,
# command routing and any newly added CLI source remain unresolved.
READ_ONLY_CLI = {
    'cmd/baha/guided_application_read_context.go',
    'cmd/baha/guided_application_read_context_test.go',
}
# Package-local conformance tests only. No production lifecycle, resolver or
# registry implementation is assumed safe from its package tests alone.
SAFE_CAPABILITY_TESTS = {
    'internal/capability/domain_test.go',
    'internal/capability/integration_contract_test.go',
}
CAPABILITY_CHECKS = [
    {'id': 'capability-package-tests', 'owner': 'internal/capability',
     'command': 'go test ./internal/capability/... -count=1'},
]
# Independently reviewed closed change bundle for core-893 PR #930.
# This is NOT a general CI allowlist: missing, additional, renamed or unrelated
# paths continue through the conservative unresolved classifier.
AUDITED_CORE_893_PATHS = frozenset([
    ".github/workflows/ci.yml",
    ".github/workflows/pre-release-runtime-suite.yml",
    "scripts/PR_RUNTIME_REQUEST_INTERFACE.md",
    "scripts/documentation-audit.sh",
    "scripts/pr-authenticated-results.py",
    "scripts/pr-check-runner.py",
    "scripts/pr-gate-dependencies.py",
    "scripts/pr-required-final.py",
    "scripts/pr-runtime-request.py",
    "scripts/pr-selected-runtime-matrix.py",
    "scripts/pr-verified-atomic-evidence.py",
    "scripts/test_pr_authenticated_results.py",
    "scripts/test_pr_check_runner.py",
    "scripts/test_pr_gate_dependencies.py",
    "scripts/test_pr_required_final.py",
    "scripts/test_pr_runtime_request.py",
    "scripts/test_pr_selected_runtime_matrix.py",
    "scripts/test_pr_verified_atomic_evidence.py"
])
CORE_893_GATE_QUALIFICATION = {
    'integration/static/public-contracts',
    'atomic/docker/backup-restore',
    'atomic/podman/backup-restore',
}

PLANNING_SCRIPTS = {'scripts/pr-gate-plan.py', 'scripts/test_pr_gate_plan.py',
                    'scripts/pr-diff-base.py', 'scripts/test_pr_diff_base.py'}
PLANNING_WORKFLOWS = {'.github/workflows/pages.yml'}
CI_CHECKS = [
    {'id': 'pr-planner-tests', 'owner': 'scripts/test_pr_gate_plan.py',
     'command': 'python3 -m unittest scripts/test_pr_gate_plan.py'},
    {'id': 'ci-policy', 'owner': 'scripts/ci-workflow-policy.sh',
     'command': 'bash scripts/ci-workflow-policy.sh'},
    {'id': 'ci-executor-verifier-tests', 'owner': 'core-893 CI executor/verifier',
     'command': 'PYTHONPATH=scripts python3 -m unittest scripts/test_pre_release_resume.py scripts/test_evidence_api_transport.py scripts/test_select_evidence_attempts.py'},
]

def plan(diff, requirements, mapping):
    if mapping.get('schema') != 'baseharbor.pr-gate-map/v1':
        raise ValueError('unsupported mapping schema')
    release = mapping['release']
    if release not in requirements.get('releases', []):
        raise ValueError('release absent from gate catalog')
    catalog = [g['id'] for g in requirements['gates'] if release in g['required_for']]
    if len(set(catalog)) != len(catalog) or not catalog:
        raise ValueError('duplicate or empty gate catalog')
    available = set(catalog)
    identifiers = {key: diff.get(key) for key in
                   ('target_base_sha', 'head_sha', 'merge_base_sha', 'integration_sha')}
    valid, problems = validated_diff(diff)
    unresolved = ['invalid diff-base input: ' + p for p in problems]
    reasons = []
    areas = set()
    paths = []
    if valid and diff['broad_required']:
        unresolved.append('diff-base requires unresolved coverage: ' +
                          json.dumps(diff.get('reasons', []), sort_keys=True))
    if valid:
        for change in diff['changes']:
            status = change.get('status')
            if status in ('renamed', 'copied'):
                names = [change.get('old_path'), change.get('new_path')]
            elif status in ('added', 'deleted', 'modified', 'type_changed'):
                names = [change.get('path')]
            else:
                unresolved.append('unrecognized change status: ' + repr(status))
                continue
            if any(not isinstance(n, str) or not n or n.startswith('/') or
                   '..' in n.split('/') for n in names):
                unresolved.append('invalid changed path')
                continue
            paths.extend(names)
    if not paths:
        unresolved.append('empty or unverified changes cannot exempt checks')
    selected = set()
    validations = {}
    def require(checks):
        for check in checks:
            validations[check['id']] = check
    # The exact reviewed orchestration bundle receives named targeted coverage,
    # not a directory-level security/evidence exception.
    if valid and not diff['broad_required'] and len(paths) == len(AUDITED_CORE_893_PATHS) and set(paths) == AUDITED_CORE_893_PATHS:
        require(CI_CHECKS)
        selected.update(CORE_893_GATE_QUALIFICATION)
        absent = sorted(selected - available)
        if absent:
            unresolved.append('reviewed core-893 gates absent from trusted catalog: ' + ', '.join(absent))
            selected.difference_update(absent)
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'unresolved' if unresolved else 'resolved',
                'unresolved_coverage': unresolved,
                'broad_required': bool(unresolved),
                'selected_gates': sorted(selected),
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': ('Enforce all required_validation_checks independently of selected_gates; '
                    'zero selected_gates is valid only when coverage_status is resolved. '
                    'If unresolved, final check MUST fail closed; never treat as no checks '
                    'or automatically run the complete release catalog.'),
                'reasons': ['exact independently reviewed core-893 executor/evidence bundle; targeted native/static qualification required']}
    for path in paths:
        if (path.startswith('docs/') and path.endswith(('.md', '.rst')) or
                path in ('README.md', 'CONTRIBUTING.md', 'mkdocs.yml', 'mkdocs.de.yml') or
                path.startswith('site/')):
            area = 'documentation'
            require(DOC_CHECKS)
        elif path in READ_ONLY_CLI:
            area = 'cli-read-only'
            require(CODE_CHECKS)
            require(CLI_CHECKS)
            selected.add('integration/static/public-contracts')
        elif path.startswith('cmd/baha/') and path.endswith('.go'):
            area = 'cli-unresolved-runtime-impact'
            require(CODE_CHECKS)
            require(CLI_CHECKS)
            unresolved.append('CLI runtime impact needs qualified targeted native coverage: ' + path)
        elif path in SAFE_CAPABILITY_TESTS:
            area = 'capability-conformance-test'
            require(CODE_CHECKS)
            require(CAPABILITY_CHECKS)
            selected.add('integration/static/public-contracts')
        elif path.startswith('internal/capability/'):
            area = 'capability-implementation-unresolved'
            require(CODE_CHECKS)
            require(CAPABILITY_CHECKS)
            unresolved.append('provider/capability registry or lifecycle impact needs targeted qualification: ' + path)
        elif path.startswith('internal/application/'):
            area = 'application-implementation-unresolved'
            require(CODE_CHECKS)
            unresolved.append('application lifecycle/provider resolution impact needs targeted qualification: ' + path)
        elif path.startswith('contracts/') and path.endswith(('.go', '.json')):
            area = 'contract'
            require(CODE_CHECKS)
            selected.add('integration/static/public-contracts')
        elif path.startswith('deploy/docker/'):
            area = 'docker-deployment-unresolved'
            require(CODE_CHECKS)
            unresolved.append('deployment/security/backup script impact needs specific native qualification: ' + path)
        elif path in PLANNING_WORKFLOWS:
            area = 'documentation-workflow'
            require(DOC_CHECKS)
            require(CI_CHECKS)
        elif path in PLANNING_SCRIPTS:
            area = 'ci-planner'
            require(CI_CHECKS)
        elif path.startswith('.github/workflows/'):
            area = 'runtime-workflow-unresolved'
            require(CI_CHECKS)
            unresolved.append('workflow execution/cleanup/evidence impact requires native qualification: ' + path)
        elif (path.startswith('scripts/ci-') or path.startswith('scripts/pr-') or
              path.startswith('scripts/test_pr_') or
              path.startswith('scripts/verify-') or 'evidence' in path):
            area = 'ci-executor-unresolved'
            require(CI_CHECKS)
            unresolved.append('CI execution/evidence impact requires specific qualification: ' + path)
        else:
            area = 'unresolved'
            unresolved.append('unknown impact for ' + path)
        areas.add(area)
        reasons.append(path + ': ' + area)
    absent = sorted(selected - available)
    if absent:
        unresolved.append('required mapped gates absent from trusted catalog: ' +
                          ', '.join(absent))
        selected.difference_update(absent)
    status = 'unresolved' if unresolved else 'resolved'
    return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
            'mapping_schema': mapping['schema'], 'release': release,
            'coverage_status': status, 'unresolved_coverage': unresolved,
            'broad_required': bool(unresolved),
            'selected_gates': sorted(selected),
            'required_validation_checks': [validations[k] for k in sorted(validations)],
            'validation_contract': (
                'Enforce all required_validation_checks independently of selected_gates; '
                'zero selected_gates is valid only when coverage_status is resolved. '
                'If unresolved, final check MUST fail closed; never treat as no checks '
                'or automatically run the complete release catalog.'),
            'reasons': reasons}

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--diff', type=Path, required=True, help='JSON output from scripts/pr-diff-base.py')
    parser.add_argument('--requirements', type=Path, default=ROOT / 'release-requirements.json')
    parser.add_argument('--mapping', type=Path, default=ROOT / 'pr-gate-map-v1.json')
    args = parser.parse_args()
    result = plan(json.loads(args.diff.read_text()), json.loads(args.requirements.read_text()),
                  json.loads(args.mapping.read_text()))
    print(json.dumps(result, indent=2, sort_keys=True))


if __name__ == '__main__':
    main()
