#!/usr/bin/env python3
"""Conservative v0.4.26 PR area-to-existing-gate selection (#892)."""
import argparse
import hashlib
import json
import os
import re
import subprocess
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

WIZARD_BASE_CHECKS = [
    {'id': 'cli-gofmt', 'owner': 'cmd/baha formatting',
     'command': 'test -z "$(gofmt -l cmd/baha)"'},
    {'id': 'cli-vet', 'owner': 'cmd/baha package', 'command': 'go vet ./cmd/baha'},
    {'id': 'cli-build', 'owner': 'cmd/baha package', 'command': 'go build ./cmd/baha'},
]

def wizard_check(identifier):
    return {'id': identifier, 'owner': 'trusted wizard component executor',
            'command': 'python3 scripts/pr-check-runner.py execute --check ' +
                       identifier + ' --repository .'}

# Finite component inventory from core-911..core-921. This is an exact path
# map, never a cmd/baha prefix exemption. Pure read/presentation profiles use
# hosted Go checks only; mutation wiring selects the existing small lifecycle
# gates in addition to its contract/build checks.
WIZARD_COMPONENTS = {
    'cmd/baha/app_edit_load.go': ('wizard-app-edit-load-tests', ()),
    'cmd/baha/app_edit_load_test.go': ('wizard-app-edit-load-tests', ()),
    'cmd/baha/app_capability_selection.go': ('wizard-capability-selection-tests', ()),
    'cmd/baha/app_capability_selection_test.go': ('wizard-capability-selection-tests', ()),
    'cmd/baha/app_provider_choice.go': ('wizard-provider-choice-tests', ()),
    'cmd/baha/app_provider_choice_test.go': ('wizard-provider-choice-tests', ()),
    'cmd/baha/app_footprint_presenter.go': ('wizard-footprint-presenter-tests', ()),
    'cmd/baha/app_footprint_presenter_test.go': ('wizard-footprint-presenter-tests', ()),
    'cmd/baha/app_change_preview.go': ('wizard-change-presentation-tests', ()),
    'cmd/baha/app_change_preview_test.go': ('wizard-change-presentation-tests', ()),
    'cmd/baha/app_change_result_presenter.go': ('wizard-change-presentation-tests', ()),
    'cmd/baha/app_change_result_presenter_test.go': ('wizard-change-presentation-tests', ()),
    'cmd/baha/app_wizard_controller.go': ('wizard-controller-tests', ()),
    'cmd/baha/app_wizard_controller_test.go': ('wizard-controller-tests', ()),
    'cmd/baha/app_new.go': ('wizard-app-new-tests',
                            ('atomic/docker/lifecycle', 'atomic/podman/lifecycle')),
    'cmd/baha/app_new_test.go': ('wizard-app-new-tests',
                                 ('atomic/docker/lifecycle', 'atomic/podman/lifecycle')),
    'cmd/baha/app_new_wizard.go': ('wizard-app-new-tests',
                                   ('atomic/docker/lifecycle', 'atomic/podman/lifecycle')),
    'cmd/baha/app_new_wizard_test.go': ('wizard-app-new-tests',
                                        ('atomic/docker/lifecycle', 'atomic/podman/lifecycle')),
    'cmd/baha/app_edit.go': ('wizard-app-edit-tests',
                             ('atomic/docker/lifecycle', 'atomic/podman/lifecycle')),
    'cmd/baha/app_edit_test.go': ('wizard-app-edit-tests',
                                  ('atomic/docker/lifecycle', 'atomic/podman/lifecycle')),
    'cmd/baha/app_edit_apply.go': ('wizard-app-edit-tests',
                                   ('atomic/docker/lifecycle', 'atomic/podman/lifecycle')),
    'cmd/baha/app_edit_apply_test.go': ('wizard-app-edit-tests',
                                        ('atomic/docker/lifecycle', 'atomic/podman/lifecycle')),
    'cmd/baha/app_footprint_show.go': ('wizard-footprint-command-tests', ()),
    'cmd/baha/app_footprint_show_test.go': ('wizard-footprint-command-tests', ()),
    'cmd/baha/app_machine_adapters.go': ('wizard-machine-adapter-tests', ()),
    'cmd/baha/app_machine_adapters_test.go': ('wizard-machine-adapter-tests', ()),
}
# Individually reviewed core-921 shared registration sources; no prefix matching,
# no broad cmd/baha exemption, no unreviewed new files, renames or deletes.
# Production MCP and command files are mixed execution/registration surfaces.
# No file-wide qualification is safe: even a one-line auth/write change
# requires independent behavioral review and remains unresolved.
REGISTRATION_SHARED_FILES = frozenset()
REGISTRATION_REFERENCE_FILES = frozenset({
    'docs/reference/cli-machine-coverage.md',
    'docs/reference/mcp.md',
    'docs/de/reference/cli-machine-coverage.md',
    'docs/de/reference/mcp.md',
    'docs/cli/command-index.md',
    'docs/de/cli/command-index.md',
})
REGISTRATION_TEST_FILES = frozenset({
    'cmd/baha/registration_ci_readiness_test.go',
})
REGISTRATION_COVERAGE = REGISTRATION_REFERENCE_FILES | REGISTRATION_TEST_FILES

APPID_FIX_REVIEWED_HEAD = '0e213d42c74cfdb91964592c17e3914a848207b9'
APPID_FIX_REVIEWED_FILES = {'cmd/baha/app_workload_integration_test.go': 'modified',
                            'internal/application/manifest_test.go': 'modified'}

# Exact final-check repair: await mandatory policy and bounded authenticated
# API job readiness. The adapter comes only from the immutable trusted base;
# the verifier and original identity guards stay unchanged. No runtime exception.
WORKFLOW_POLICY_DEPENDENCY_REVIEWED_HEAD = '013d7e4d491afc58a31c7934b371ab0dc35f3346'
WORKFLOW_POLICY_DEPENDENCY_REVIEWED_FILES = {
    '.github/workflows/ci.yml': 'modified',
    'scripts/pr-authenticated-results.py': 'modified',
    'scripts/test_pr_authenticated_results.py': 'modified',
}

# Independently reviewed public MCP bootstrap: pin an omitted initial revision
# through the existing read owner, then revalidate; apply consent stays strict.
# Native job 119314164799 (run 39626844187) proved the fresh public path.
MCP_PUBLIC_BOOTSTRAP_REVIEWED_HEAD = 'f762f4480a2eb6b29ceebb35ae4032cb2ce7288c'
MCP_PUBLIC_BOOTSTRAP_REVIEWED_FILES = {
    'cmd/baha/app_machine_adapters.go': 'modified',
    'cmd/baha/app_mcp_provider_edit_native_test.go': 'modified',
    'cmd/baha/app_registration_921.go': 'modified',
    'cmd/baha/registration_921_test.go': 'modified',
}
# Terminal adapter identity only: preserve Bubble Tea's cancellable input
# contract across New preview/No. Existing real PTY tests execute in app-new.
NEW_TERMINAL_INPUT_REVIEWED_HEAD = '99901ff2e787157ffb5e8d7d5206cf21272bd6c3'
NEW_TERMINAL_INPUT_REVIEWED_FILES = {
    'cmd/baha/app_new_wizard_test.go': 'modified',
    'cmd/baha/application_provider_preferences.go': 'modified',
}

# Pre-execution Edit refusal accuracy only: the unchanged Execute callback
# marks entry before lifecycle work, so real partial receipts stay unchanged.
# This frozen source adds a Guided wrapper for every affected unit regression;
# the earlier source without that mandatory coverage receives no exemption.
EDIT_REFUSAL_RESULT_REVIEWED_HEAD = 'c6dfb988bb4f1fd295b6beb950980bb257e3699a'
EDIT_REFUSAL_RESULT_REVIEWED_FILES = {
    'cmd/baha/app_edit_apply.go': 'modified',
    'cmd/baha/app_edit_apply_test.go': 'modified',
}

# Exact Docker fixture/workflow source: all eight native runtime families passed
# run 39619782116, including cleanup and authenticated readiness. Existing Guided
# regressions exercise the fixture's strict consent, owner and material checks;
# accepted target merges also retain the independently qualified MCP workflow.
DOCKER_RUNTIME_FIXTURE_REVIEWED_HEAD = 'c1337192e06e63f8210ace9e5687d8dc8f70f6d2'
DOCKER_RUNTIME_FIXTURE_REVIEWED_FILES = {
    '.github/workflows/docker-runtime-acceptance.yml': 'modified',
    'cmd/baha/app_repository_integration_test.go': 'modified',
    'cmd/baha/app_workload_integration_test.go': 'modified',
    'cmd/baha/repository_runtime_fixture_test.go': 'added',
    'scripts/test_ci_docker_runtime_doctor.py': 'added',
}

# Independently reviewed withdrawal projection; canonical persisted provider
# preferences remain intact. Native MCP run 39605072620 also proves this source.
RETIREMENT_PREFERENCES_REVIEWED_HEAD = '3f6772fc75243eaceb00c669f8ed1cd4c4b694e4'
RETIREMENT_PREFERENCES_REVIEWED_FILES = {
    'cmd/baha/application_runtime_retirement_intent.go': 'modified',
    'cmd/baha/application_runtime_retirement_preferences_test.go': 'added',
}

# Native PG17 rootless recovery passed run 39605258769, including cleanup and
# foreign-resource preservation. Retirement must already be integrated: its
# two files are prerequisites, not part of this exact 17-file qualification.
PODMAN_PG17_RECOVERY_REVIEWED_HEAD = '77eb9a120f46b42281198fcee1de788c9779be18'
PODMAN_PG17_RECOVERY_REVIEWED_FILES = {
    '.github/workflows/targeted-integration.yml': 'modified',
    'cmd/baha/app_backup_restore.go': 'modified',
    'cmd/baha/app_backup_restore_integration_test.go': 'modified',
    'cmd/baha/app_backup_runtime_fixture_test.go': 'added',
    'cmd/baha/app_backup_writer_recovery_test.go': 'added',
    'cmd/baha/app_broker.go': 'modified',
    'cmd/baha/app_workload.go': 'modified',
    'cmd/baha/app_workload_stop_test.go': 'modified',
    'cmd/baha/native_fixture_cleanup_diagnostics.go': 'added',
    'cmd/baha/native_fixture_cleanup_diagnostics_test.go': 'modified',
    'cmd/baha/native_fixture_cleanup_test.go': 'modified',
    'cmd/baha/runtime_destroy_inventory_test.go': 'modified',
    'internal/application/postgres_backup.go': 'modified',
    'internal/application/postgres_backup_integration_test.go': 'modified',
    'internal/application/postgres_backup_test.go': 'modified',
    'internal/runtime/service_stop.go': 'modified',
    'scripts/test_pr_selected_runtime_matrix.py': 'modified',
}

# Exact parsing repair: Automatic maps to the existing empty provider selection;
# no lifecycle, authorization or runtime authority changes are covered here.
EDIT_AUTOMATIC_INPUT_REVIEWED_HEAD = '673bdcdaed9b81fa7f810b018961c02943c89ca7'
EDIT_AUTOMATIC_INPUT_REVIEWED_FILES = {
    'cmd/baha/app_edit.go': 'modified',
    'cmd/baha/app_edit_apply_test.go': 'modified',
}

# Zero-needs Edit preview envelope repair only: the planner routes an empty
# change-request set through the existing impact owner; no execution, consent
# or registration surface changes. The bound source carries the guided-lane
# regression inside the existing wizard-app-edit-tests check filter.
EDIT_ZERO_PREVIEW_REVIEWED_HEAD = 'c37933d9688615ba574cfc3b9ffc817bd03c4f3e'
EDIT_ZERO_PREVIEW_REVIEWED_FILES = {
    'cmd/baha/app_edit_apply_result.go': 'modified',
    'cmd/baha/app_edit_zero_preview_test.go': 'added',
}
RUNTIME_STEP_CAP_REVIEWED_HEAD = '75efeef89b61d848d5ea351877ba6423a31e58a6'
RUNTIME_STEP_CAP_REVIEWED_FILES = {
    '.github/workflows/pre-release-runtime-suite.yml': 'modified',
}
DISPATCH_GATE_REF_REVIEWED_HEAD = '0267dbdb71f63a2c77e193618e6fba4106e5cd5b'
DISPATCH_GATE_REF_REVIEWED_FILES = {
    '.github/workflows/ci.yml': 'modified',
    '.github/workflows/pre-release-runtime-suite.yml': 'modified',
    '.github/workflows/targeted-demo-acceptance.yml': 'modified',
    'scripts/targeted-gate-ref.py': 'added',
    'scripts/test_targeted_gate_ref.py': 'added',
}
# Bind complete v0.4.26 resolver routing, final consumer origin and required
# producer dependencies. Existing CI runs the real shell/approval regressions.
ECOSYSTEM_RESOLVER_REVIEWED_HEAD = '46e80866db0dfeecc10c4472a0f1f585d11ff164'
ECOSYSTEM_RESOLVER_REVIEWED_FILES = {
    '.github/workflows/pre-release.yml': 'modified',
    '.github/workflows/release.yml': 'modified',
    '.github/workflows/targeted-demo-acceptance.yml': 'modified',
    'scripts/pre-release-resume.py': 'modified',
    'scripts/test_ecosystem_release_pins.py': 'modified',
    'scripts/test_pre_release_resume.py': 'modified',
}

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
# Focused, package-scoped checks for the read-only repository-inspection
# reconciliation surface. The reconcile command is pure in-memory matching with
# package unit tests and a thin cmd/baha consumer, so these static checks cover
# its actual risk without a native runtime gate. They never exempt the whole
# package: detector, model, finding or manifest files remain unresolved below.
INSPECT_CHECKS = [
    {'id': 'inspect-gofmt', 'owner': 'internal/repositoryinspect formatting',
     'command': 'test -z "$(gofmt -l internal/repositoryinspect)"'},
    {'id': 'inspect-package-tests', 'owner': 'internal/repositoryinspect',
     'command': 'go test ./internal/repositoryinspect/ -count=1'},
    {'id': 'inspect-vet', 'owner': 'internal/repositoryinspect',
     'command': 'go vet ./internal/repositoryinspect/'},
    {'id': 'inspect-consumer-tests', 'owner': 'cmd/baha inspect/reconcile consumers',
     'command': "go test ./cmd/baha -run 'Inspect|Reconcile' -count=1"},
    {'id': 'inspect-consumer-build', 'owner': 'cmd/baha package',
     'command': 'go build -o "$RUNNER_TEMP/baha-inspect" ./cmd/baha'},
]
# Explicitly reviewed reconciliation files that the narrow lane covers. The
# package tests run at the top-level package only, so this is an exact reviewed
# allowlist, not a prefix or name heuristic: any other repositoryinspect file,
# including a future reconcile*.go or a nested subpackage, stays unresolved.
INSPECT_RECONCILE_REVIEWED = frozenset([
    'internal/repositoryinspect/reconcile.go',
    'internal/repositoryinspect/reconcile_default_instance_test.go',
])
# core-751 prospective scope is explicit but never a path-only qualification.
# Source review and immutable commit binding must precede resolved coverage.
INSPECT_IMPORT_CANDIDATE_FILES = {
    'internal/repositoryinspect/capability_detection_runtime.go': 'modified',
    'internal/repositoryinspect/capability_detection_js_context.go': 'added',
    'internal/repositoryinspect/capability_import_detection_test.go': 'added',
}
INSPECT_IMPORT_REQUIRED_IDS = frozenset(check['id'] for check in INSPECT_CHECKS)

# Independently reviewed exact candidates: commit ancestry and complete diff
# status are both mandatory, never a reusable path-only allowlist.
CORE_751_REVIEWED_HEAD = 'cf0a19bc4c8f398d7ee3fc07a635833168081ff5'
CORE_991_REVIEWED_HEAD = 'eeda633071714a0b43551ab947ad036ffaac1b1b'
CORE_982_REVIEWED_HEAD = '0ab44b7edb94618489cb505cf897ff7e6d73da6c'
# Independently qualified GC candidate is bound to its immutable full SHA.
# Any follow-up contents, changed file inventory or rename stays unresolved.
GC_999_REVIEWED_HEAD = '31089e281390c58189199b25d22925c590ab228d'
GC_999_REVIEWED_FILES = {
    '.github/workflows/artifact-gc.yml': 'modified',
    '.github/workflows/ci.yml': 'modified',
    'scripts/test_artifact_gc.py': 'modified',
}
BINDING_PR_1007_REVIEWED_FILES = {
    'scripts/pr-check-runner.py': 'modified',
    'scripts/pr-gate-plan.py': 'modified',
    'scripts/test_pr_gate_plan.py': 'modified',
}


RUNTIME_RESET_REVIEWED_HEAD = 'e6616b4504130d5143d32dd85acf65419e943f07'
RUNTIME_RESET_REVIEWED_FILES = {
    'scripts/ci-runtime-reset.sh': 'modified',
    'scripts/test_ci_runtime_reset_scope.py': 'added',
}
PODMAN_CONSENT_REVIEWED_HEAD = 'c3809b12c345f675359c85f4ac3aeffc8948eaab'
PODMAN_CONSENT_REVIEWED_FILES = {
    '.github/workflows/podman-acceptance.yml': 'modified',
    'scripts/ci-podman-typed-consent.py': 'added',
    'scripts/test_ci_podman_typed_consent.py': 'added',
}
MCP_PRECONDITIONS_REVIEWED_HEAD = '3dfac93c0775f0d6739e9d696033307250a81093'
MCP_PRECONDITIONS_REVIEWED_FILES = {
    'cmd/baha/mcp_preconditions_test.go': 'added',
}

EVIDENCE_RETENTION_REVIEWED_HEAD = '2e3325e142995aedd3d69a2e5569452127cbc58f'
EVIDENCE_RETENTION_REVIEWED_FILES = {
    '.github/workflows/ci.yml': 'modified',
    '.github/workflows/artifact-gc.yml': 'modified',
    '.github/workflows/docker-runtime-acceptance.yml': 'modified',
    '.github/workflows/podman-acceptance.yml': 'modified',
    'scripts/test_artifact_gc.py': 'modified',
}

SQL_RUNTIME_LAYOUT_REVIEWED_HEAD = '887eece62ffdfe2ba694ceb1f332dc5a74ce7872'
SQL_RUNTIME_LAYOUT_REVIEWED_FILES = {
    'internal/application/backend_probe_test.go': 'modified',
    'internal/application/runtime_environment.go': 'added',
    'internal/application/runtime_postgres.go': 'modified',
}

CORE_982_REVIEWED_FILES = {
    '.github/workflows/ci.yml': 'modified',
    'scripts/test_pr_actions_summary.py': 'added',
}
CORE_991_REVIEWED_FILES = {
    'scripts/pr-check-runner.py': 'modified',
    'scripts/pr-gate-plan.py': 'modified',
    'scripts/test_pr_check_runner.py': 'modified',
    'scripts/test_pr_gate_plan.py': 'modified',
}

# Independently reviewed immutable source and trusted merge-base for core-893 PR #930.
# SHA commits cryptographically bind file contents and tree; path matching alone never qualifies.
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
CORE_893_REVIEWED_HEAD_SHA = 'b0acfc32ddf4d940294a43f98db4618d3aba83cd'
CORE_893_TRUSTED_MERGE_BASE_SHA = '14d864dfd38801a56e54901ea520f3b02db134b2'
CORE_893_GATE_QUALIFICATION = {
    'integration/static/public-contracts',
    'atomic/docker/backup-restore',
    'atomic/podman/backup-restore',
}

# Immutable independently reviewed #953 CI/executor change; source-tree binding
# is the commit SHA, not a reusable matching-path exemption.  The stable
# merge-base permits ordinary integration of later develop commits.
CORE_752_CI_HEAD = '211d19b13a206040f0f32506e5f5f85d4a467287'
# Original review base is embodied in the authenticated reviewed commit ancestry.
# The current merge-base changes after normal develop integration.
CORE_752_CI_MERGE_BASE = 'e8b021fe63c61abc0990a560e5f2915bb70c2c13'
CORE_752_CI_PATHS = frozenset({
    '.github/workflows/ci.yml',
    'scripts/pr-check-runner.py', 'scripts/pr-gate-plan.py',
    'scripts/test_pr_check_runner.py', 'scripts/test_pr_gate_plan.py',
})

# Independent manager qualification of the corrected #957 source. This record
# is read only from trusted develop; it cannot authorize an unreviewed follow-up.
WIZARD_CI_REVIEWED_HEAD = '1a94d171166d0402b95740a5f71b52482fb61d9a'
WIZARD_CI_REVIEWED_PATHS = frozenset({
    '.github/workflows/ci.yml', 'scripts/pr-check-runner.py',
    'scripts/pr-gate-plan.py', 'scripts/test_pr_check_runner.py',
    'scripts/test_pr_gate_plan.py', 'scripts/test_pr_required_final.py',
    'scripts/test_pr_wizard_workflow_route.py',
})

# Independent Maintainer source review for registration CI (#974), not task
# acceptance or authorization of future core-921 production registration edits.
# Evidence: run 38929047340; actual hosted route pilots remain required after
# integration. The immutable commit and exact statuses bind this bootstrap.
REGISTRATION_CI_REVIEWED_HEAD = 'bb9c713ab0449b0c8243e7535a51b8e43195bf0c'
REGISTRATION_CI_REVIEWED_FILES = {
    'cmd/baha/registration_ci_readiness_test.go': 'added',
    'scripts/pr-check-runner.py': 'modified',
    'scripts/pr-gate-plan.py': 'modified',
    'scripts/test_pr_check_runner.py': 'modified',
    'scripts/test_pr_gate_plan.py': 'modified',
    'scripts/test_pr_registration_ci.py': 'added',
}

# Independently reviewed production registration, SDK error/consent semantics,
# and coreless SQL fixture. Full native MCP run 39605072620 passed every step,
# including cleanup. Retirement is a prerequisite integrated independently;
# the exact remaining 20 paths and clean Git merge bind this source only.
CORE_921_PRODUCTION_REVIEWED_HEAD = 'b31fe8918e319e3ad1505ffaaa1209c1343a5d13'
CORE_921_PRODUCTION_REVIEWED_FILES = {
    '.github/workflows/docker-runtime-acceptance.yml': 'modified',
    'cmd/baha/app_machine_adapters.go': 'modified',
    'cmd/baha/app_machine_adapters_test.go': 'modified',
    'cmd/baha/app_mcp_provider_edit_native_test.go': 'added',
    'cmd/baha/app_provider_choice.go': 'modified',
    'cmd/baha/app_registration_921.go': 'added',
    'cmd/baha/commands.go': 'modified',
    'cmd/baha/machine_coverage.go': 'modified',
    'cmd/baha/main.go': 'modified',
    'cmd/baha/mcp_tools.go': 'modified',
    'cmd/baha/registration_921_test.go': 'added',
    'contracts/machine/v1/control.golden.json': 'modified',
    'docs/cli/command-index.md': 'modified',
    'docs/de/cli/command-index.md': 'modified',
    'docs/de/reference/cli-machine-coverage.md': 'modified',
    'docs/de/reference/mcp.md': 'modified',
    'docs/reference/cli-machine-coverage.md': 'modified',
    'docs/reference/mcp.md': 'modified',
    'internal/machine/contract.go': 'modified',
    'scripts/test_pr_registration_ci.py': 'modified',
}

# Proposed exact qualification for core-917 (#975). This record becomes an
# authority only after independent policy review and integration into develop.
# Source inspection: PR review 5555616115. Focused + Docker/Podman evidence:
# run 38995396298, source d8caa81f, integration e1a7f0cb. These references explain
# the decision; current integration results must still be authenticated afresh.
# Never accept the same paths with different source, statuses or extra changes.
CORE_917_REVIEWED_HEAD = 'd8caa81f0c85bf4f9f48797408a9bfdb99c15b2e'
CORE_917_REVIEWED_FILES = {
    'cmd/baha/app_core_requirement.go': 'modified',
    'cmd/baha/app_new_wizard.go': 'modified',
    'cmd/baha/app_new_wizard_test.go': 'modified',
    'cmd/baha/app_wizard_controller.go': 'modified',
    'cmd/baha/application_backend_placement_owner.go': 'modified',
    'cmd/baha/application_provider_preferences.go': 'added',
    'internal/application/manifest.go': 'modified',
    'internal/application/provider_preferences.go': 'added',
    'internal/application/provider_preferences_test.go': 'added',
    'internal/application/provider_registry.go': 'modified',
    'internal/deployment/deployment_store.go': 'modified',
    'internal/development/project.go': 'modified',
}
CORE_917_REQUIRED_GATES = {'atomic/docker/lifecycle', 'atomic/podman/lifecycle'}

# Proposed independent core-918 qualification: immutable reviewed source, exact
# 21-file diff (including 15 previously unresolved paths), and only verified
# normal trusted-target merges. This record is NOT a reusable file allowlist.
CORE_918_REVIEWED_HEAD = '95eed33e6a3852c21b8dc1a25ccc17e601f38da8'
CORE_918_REVIEWED_FILES = {
    'cmd/baha/app_apply.go': 'modified',
    'cmd/baha/app_apply_lifecycle.go': 'modified',
    'cmd/baha/app_apply_result.go': 'added',
    'cmd/baha/app_apply_result_test.go': 'added',
    'cmd/baha/app_edit.go': 'added',
    'cmd/baha/app_edit_apply.go': 'added',
    'cmd/baha/app_edit_apply_result.go': 'added',
    'cmd/baha/app_edit_apply_test.go': 'added',
    'cmd/baha/app_edit_test.go': 'added',
    'cmd/baha/app_machine_adapters.go': 'modified',
    'cmd/baha/app_machine_adapters_test.go': 'modified',
    'cmd/baha/application_native_backend_impact.go': 'modified',
    'cmd/baha/application_native_backend_impact_test.go': 'added',
    'cmd/baha/application_native_backend_observation.go': 'modified',
    'cmd/baha/application_native_sql_projection.go': 'added',
    'cmd/baha/application_native_sql_projection_unix.go': 'added',
    'cmd/baha/application_native_sql_projection_windows.go': 'added',
    'cmd/baha/application_provider_impact.go': 'modified',
    'internal/application/backend_probe_test.go': 'modified',
    'internal/application/runtime_backend_probe.go': 'modified',
    'internal/application/runtime_postgres.go': 'modified',
}
CORE_918_REQUIRED_GATES = {'atomic/docker/lifecycle', 'atomic/podman/lifecycle'}

def core752_reviewed_merge_chain(target, head, reviewed=None, repository='.'):
    """Trust only clean, first-parent-preserving merges of trusted target commits.

    Every second parent must belong to the *current* trusted target history;
    each merge tree must be exactly Git's computed clean result. Never accept
    arbitrary amended content, rebases, cherry-picks or side-branch merges.
    """
    reviewed = reviewed or CORE_752_CI_HEAD
    def git(*args):
        try:
            return subprocess.run(['git', '-C', str(repository), *args],
                                  text=True, stdout=subprocess.PIPE,
                                  stderr=subprocess.PIPE, check=False)
        except OSError:
            return None
    if not all(isinstance(x, str) and len(x) == 40 for x in (target, head, reviewed)):
        return False
    for _ in range(64):
        if head == reviewed:
            return True
        parents = git('rev-list', '--parents', '-n', '1', head)
        if not parents or parents.returncode or len(parents.stdout.strip().split()) != 3:
            return False
        commit, previous, trusted = parents.stdout.strip().split()
        if commit != head:
            return False
        ancestry = git('merge-base', '--is-ancestor', trusted, target)
        if not ancestry or ancestry.returncode:
            return False
        expected = git('merge-tree', '--write-tree', previous, trusted)
        actual = git('rev-parse', '--verify', head + '^{tree}')
        if not expected or expected.returncode or not actual or actual.returncode:
            return False
        if expected.stdout.strip().splitlines()[0] != actual.stdout.strip():
            return False
        head = previous
    return False


# Execution of this reviewed test candidate is not content qualification.
CORE_754_TEST_HEAD = '5a3813385af5392a9be3095bd038424a4f9950b7'
CORE_754_TEST_PATH = 'cmd/baha/mcp_preconditions_test.go'
CORE_754_TEST_CHECK = 'wizard-core754-mcp-tests'

PLANNING_SCRIPTS = {'scripts/pr-gate-plan.py', 'scripts/test_pr_gate_plan.py',
                    'scripts/pr-diff-base.py', 'scripts/test_pr_diff_base.py'}
PLANNING_WORKFLOWS = {'.github/workflows/pages.yml'}
R27_PROTOTYPE_PATHS = frozenset({
    'scripts/r27_archive_publisher.py',
    'scripts/r27_archive_resolver.py',
    'scripts/r27_evidence_receipt.py',
    'scripts/r27_evidence_coverage.py',
    'scripts/r27_verified_coverage.py',
    'scripts/tests/test_r27_archive_publisher.py',
    'scripts/tests/test_r27_archive_resolver.py',
    'scripts/tests/test_r27_evidence_receipt.py',
    'scripts/tests/test_r27_evidence_coverage.py',
    'scripts/tests/test_r27_verified_coverage.py',
})
R27_PROTOTYPE_CHECKS = [{
    'id': 'r27-prototype-regressions',
    'owner': 'scripts/tests/test_r27_*.py (untrusted prototype, no issuer authority)',
    'command': 'PYTHONPATH=scripts python3 -m unittest '
               'scripts/tests/test_r27_archive_publisher.py '
               'scripts/tests/test_r27_archive_resolver.py '
               'scripts/tests/test_r27_evidence_receipt.py '
               'scripts/tests/test_r27_evidence_coverage.py '
               'scripts/tests/test_r27_verified_coverage.py',
}]

CI_CHECKS = [
    {'id': 'pr-planner-tests', 'owner': 'scripts/test_pr_gate_plan.py',
     'command': 'python3 -m unittest scripts/test_pr_gate_plan.py'},
    {'id': 'ci-policy', 'owner': 'scripts/ci-workflow-policy.sh',
     'command': 'bash scripts/ci-workflow-policy.sh'},
    {'id': 'ci-executor-verifier-tests', 'owner': 'core-893 CI executor/verifier',
     'command': 'PYTHONPATH=scripts python3 -m unittest scripts/test_pre_release_resume.py scripts/test_evidence_api_transport.py scripts/test_select_evidence_attempts.py'},
]

WORKFLOW_SYNTAX_CHECKS = [
    {'id': 'ci-actionlint', 'owner': '.github/workflows workflow syntax',
     'command': 'go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck='},
]

def trusted_binding_checks(diff, repository='.', context_repository=None,
                           context_target_branch=None):
    """Verify the frozen binding from the authenticated target, never PR content.

    Re-run the real diff resolver so caller-supplied inventories, integration
    parents and merge trees cannot substitute for Git evidence. This bounded
    record enables only the three CI files and the existing CI check catalog.
    Hash the frozen reviewed blobs; later source may only add exact Git-computed
    first-parent merges of authenticated target history. This permits strict
    up-to-date integration without accepting candidate-authored follow-ups.
    """
    valid, _ = validated_diff(diff)
    if not valid or diff['broad_required'] or not diff.get('integration_sha'):
        return None
    context_repository = (os.environ.get('GITHUB_REPOSITORY', '')
                          if context_repository is None else context_repository)
    context_target_branch = (os.environ.get('GITHUB_BASE_REF', '')
                            if context_target_branch is None else context_target_branch)
    if (context_repository != 'mcpdev80/baseharbor' or
            context_target_branch != 'develop'):
        return None

    def git(*args):
        return subprocess.run(['git', '-C', str(repository), *args],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                              check=True).stdout

    try:
        raw = git('show', diff['target_base_sha'] +
                  ':docs/internal/ci-binding-qualifications.md').decode('utf-8')
        blocks = re.findall(r'^```json\n(.*?)\n```$', raw,
                            flags=re.MULTILINE | re.DOTALL)
        if len(blocks) != 1 or raw.count('```') != 2:
            return None

        def unique_object(pairs):
            result = {}
            for key, value in pairs:
                if key in result:
                    raise ValueError('duplicate qualification key')
                result[key] = value
            return result

        record = json.loads(blocks[0], object_pairs_hook=unique_object)
        expected_files = {path: 'modified' for path in (
            'scripts/pr-check-runner.py', 'scripts/pr-gate-plan.py',
            'scripts/test_pr_gate_plan.py')}
        expected_ids = {check['id'] for check in CI_CHECKS}
        if (not isinstance(record, dict) or set(record) != {
                'repository', 'target_branch', 'reviewed_head', 'files',
                'file_hashes', 'required_checks'} or
                record['repository'] != context_repository or
                record['target_branch'] != context_target_branch or
                not isinstance(record['reviewed_head'], str) or
                re.fullmatch(r'[0-9a-f]{40}', record['reviewed_head']) is None or
                not core752_reviewed_merge_chain(
                    diff['target_base_sha'], diff['head_sha'],
                    record['reviewed_head'], repository) or
                record['files'] != expected_files or
                not isinstance(record['file_hashes'], dict) or
                set(record['file_hashes']) != set(expected_files) or
                not isinstance(record['required_checks'], list) or
                len(record['required_checks']) != len(expected_ids) or
                set(record['required_checks']) != expected_ids):
            return None
        for path, digest in record['file_hashes'].items():
            if (not isinstance(digest, str) or
                    re.fullmatch(r'[0-9a-f]{64}', digest) is None or
                    hashlib.sha256(git('cat-file', 'blob',
                                       record['reviewed_head'] + ':' + path)).hexdigest() != digest):
                return None
        # Import the resolver beside this trusted planner, not from the PR tree.
        import importlib.util
        spec = importlib.util.spec_from_file_location('binding_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(resolver)
        actual = resolver.resolve_pr_diff(repository, diff['target_base_sha'],
                                          diff['head_sha'], diff['integration_sha'])
        if actual != diff or actual['broad_required']:
            return None
        changes = actual['changes']
        if (len(changes) != len(expected_files) or
                {c.get('path'): c.get('status') for c in changes} != expected_files):
            return None
    except (OSError, subprocess.CalledProcessError, ValueError, TypeError, KeyError):
        return None
    return CI_CHECKS


def plan(diff, requirements, mapping, repository='.',
         context_repository=None, context_target_branch=None):
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
    path_status = {}
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
            for name in names:
                path_status[name] = status
    if not paths:
        unresolved.append('empty or unverified changes cannot exempt checks')
    selected = set()
    validations = {}
    def require(checks):
        for check in checks:
            validations[check['id']] = check
    # Exact source hashes bind all content, including individual file status.
    # Each clean develop merge is proved by the existing Git tree/ancestry owner.
    for reviewed, expected, checks, label in (
        (EDIT_REFUSAL_RESULT_REVIEWED_HEAD, EDIT_REFUSAL_RESULT_REVIEWED_FILES,
         CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS,
         'reviewed pre-execution Edit refusal accuracy with mandatory Guided regressions'),
        (MCP_PUBLIC_BOOTSTRAP_REVIEWED_HEAD, MCP_PUBLIC_BOOTSTRAP_REVIEWED_FILES,
         CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS +
         [wizard_check('wizard-machine-adapter-tests'), wizard_check('wizard-core921-tests')],
         'reviewed fresh public MCP proposal bootstrap with strict apply revision'),
        (NEW_TERMINAL_INPUT_REVIEWED_HEAD, NEW_TERMINAL_INPUT_REVIEWED_FILES,
         CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS + [wizard_check('wizard-app-new-tests')],
         'reviewed terminal Name delegation preserving New preview cancellation'),
        (DOCKER_RUNTIME_FIXTURE_REVIEWED_HEAD, DOCKER_RUNTIME_FIXTURE_REVIEWED_FILES,
         CI_CHECKS + CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS + WORKFLOW_SYNTAX_CHECKS,
         'reviewed native Docker fixture and authenticated eight-family runtime evidence'),
        (WORKFLOW_POLICY_DEPENDENCY_REVIEWED_HEAD, WORKFLOW_POLICY_DEPENDENCY_REVIEWED_FILES,
         CI_CHECKS + WORKFLOW_SYNTAX_CHECKS,
         'reviewed final-job dependency on mandatory authenticated workflow policy'),
        (RETIREMENT_PREFERENCES_REVIEWED_HEAD, RETIREMENT_PREFERENCES_REVIEWED_FILES,
         CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS,
         'reviewed withdrawal preferences projection preserving canonical owners'),
        (PODMAN_PG17_RECOVERY_REVIEWED_HEAD, PODMAN_PG17_RECOVERY_REVIEWED_FILES,
         CI_CHECKS + CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS + WORKFLOW_SYNTAX_CHECKS,
         'reviewed native PG17 rootless recovery and foreign-resource protection'),
        (EDIT_AUTOMATIC_INPUT_REVIEWED_HEAD, EDIT_AUTOMATIC_INPUT_REVIEWED_FILES,
         CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS +
         [wizard_check('wizard-app-edit-tests')],
         'reviewed Automatic input parsing repair; lifecycle unchanged'),
         (EDIT_ZERO_PREVIEW_REVIEWED_HEAD, EDIT_ZERO_PREVIEW_REVIEWED_FILES,
          CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS +
          [wizard_check('wizard-app-edit-tests')],
          'reviewed zero-needs Edit preview envelope with guided regression'),
         (RUNTIME_STEP_CAP_REVIEWED_HEAD, RUNTIME_STEP_CAP_REVIEWED_FILES,
          CI_CHECKS + WORKFLOW_SYNTAX_CHECKS,
          'reviewed data-capabilities runtime gate step budget split'),
         (DISPATCH_GATE_REF_REVIEWED_HEAD, DISPATCH_GATE_REF_REVIEWED_FILES,
          CI_CHECKS + WORKFLOW_SYNTAX_CHECKS,
          'reviewed bare-SHA dispatch input resolution before atomic gate checkout'),
        (ECOSYSTEM_RESOLVER_REVIEWED_HEAD, ECOSYSTEM_RESOLVER_REVIEWED_FILES,
         CI_CHECKS + WORKFLOW_SYNTAX_CHECKS,
         'reviewed v0.4.26 immutable ecosystem resolver activation'),
        (EVIDENCE_RETENTION_REVIEWED_HEAD, EVIDENCE_RETENTION_REVIEWED_FILES,
         CI_CHECKS + WORKFLOW_SYNTAX_CHECKS, 'reviewed evidence-retention source'),
        (RUNTIME_RESET_REVIEWED_HEAD, RUNTIME_RESET_REVIEWED_FILES,
         CI_CHECKS + WORKFLOW_SYNTAX_CHECKS, 'reviewed scoped-reset source'),
        (PODMAN_CONSENT_REVIEWED_HEAD, PODMAN_CONSENT_REVIEWED_FILES,
         CI_CHECKS + WORKFLOW_SYNTAX_CHECKS, 'reviewed operations-bound consent source'),
        (MCP_PRECONDITIONS_REVIEWED_HEAD, MCP_PRECONDITIONS_REVIEWED_FILES,
         WIZARD_BASE_CHECKS + [wizard_check(CORE_754_TEST_CHECK)],
         'reviewed MCP precondition tests'),
        (SQL_RUNTIME_LAYOUT_REVIEWED_HEAD, SQL_RUNTIME_LAYOUT_REVIEWED_FILES,
         CODE_CHECKS + CLI_CHECKS, 'reviewed SQL runtime layout source'),
        (CORE_751_REVIEWED_HEAD, INSPECT_IMPORT_CANDIDATE_FILES,
         INSPECT_CHECKS, 'core-751 reviewed import evidence'),
        (CORE_991_REVIEWED_HEAD, CORE_991_REVIEWED_FILES,
         CI_CHECKS + WORKFLOW_SYNTAX_CHECKS,
         'reviewed core-754/core-918 trusted CI integration'),
        (CORE_982_REVIEWED_HEAD, CORE_982_REVIEWED_FILES,
         CI_CHECKS + WORKFLOW_SYNTAX_CHECKS,
         'reviewed core-894 Actions summary presentation'),
         (GC_999_REVIEWED_HEAD, GC_999_REVIEWED_FILES,
           CI_CHECKS + WORKFLOW_SYNTAX_CHECKS,
           'reviewed exact #999 GC evidence retention candidate'),
     ):
        if (valid and not diff['broad_required'] and not unresolved and
                identifiers['integration_sha'] is not None and
                len(paths) == len(expected) and set(paths) == set(expected) and
                all(ch.get('status') == expected.get(ch.get('path'))
                    for ch in diff['changes']) and
                core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                             identifiers['head_sha'], reviewed)):
            require(checks)
            return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                    'mapping_schema': mapping['schema'], 'release': release,
                    'coverage_status': 'resolved', 'unresolved_coverage': [],
                    'broad_required': False, 'selected_gates': [],
                    'required_validation_checks': [validations[k] for k in sorted(validations)],
                    'validation_contract': ('Every listed test must execute and PASS on the '
                        'authenticated current integration. Missing, skipped and failed '
                        'validations block in the existing trusted final verifier.'),
                    'reasons': [label + ': exact source hash, complete changed status '
                                'set and proven clean trusted-target merge only']}
    # Reuse the existing content/ancestry proof for strict target updates.
    # Resolver validation proves the current PR integration parents and tree;
    # this rule only covers the immutable reviewed source and clean target merges.
    if (valid and not diff['broad_required'] and not unresolved and
            identifiers['integration_sha'] is not None and
            len(paths) == len(CORE_917_REVIEWED_FILES) and
            set(paths) == set(CORE_917_REVIEWED_FILES) and
            all(ch.get('status') == CORE_917_REVIEWED_FILES.get(ch.get('path'))
                for ch in diff['changes']) and
            core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                         identifiers['head_sha'], CORE_917_REVIEWED_HEAD)):
        require(CODE_CHECKS)
        require(CLI_CHECKS)
        require(WIZARD_BASE_CHECKS)
        require([wizard_check('wizard-app-new-tests'), wizard_check('wizard-controller-tests')])
        absent = sorted(CORE_917_REQUIRED_GATES - available)
        if absent:
            unresolved.append('reviewed core-917 gates absent from trusted catalog: ' + ', '.join(absent))
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'unresolved' if unresolved else 'resolved',
                'unresolved_coverage': unresolved, 'broad_required': bool(unresolved),
                'selected_gates': sorted(CORE_917_REQUIRED_GATES & available),
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': ('Require every listed validation and authenticated native gate '
                    'for the exact current integration; prior evidence is not automatically reusable. '
                    'If unresolved, final check MUST fail closed.'),
                'reasons': ['exact core-917 source reviewed in #975 review 5555616115; '
                            'New/controller/Guided/owner contracts and both native lifecycle gates; '
                            'qualification evidence run 38995396298, not a path-wide exemption']}
    # Independently reviewed full Edit/Apply source and exact statuses only.
    # No path-only approval: the normal trusted-target merge proof checks each
    # merge tree; unrelated changes after the review commit fail closed.
    if (valid and not diff['broad_required'] and not unresolved and
            identifiers['integration_sha'] is not None and
            len(paths) == len(CORE_918_REVIEWED_FILES) and
            set(paths) == set(CORE_918_REVIEWED_FILES) and
            all(change.get('status') == CORE_918_REVIEWED_FILES.get(change.get('path'))
                for change in diff['changes']) and
            core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                         identifiers['head_sha'], CORE_918_REVIEWED_HEAD)):
        require(CODE_CHECKS)
        require(CLI_CHECKS)
        require(WIZARD_BASE_CHECKS)
        require([wizard_check('wizard-app-edit-tests'),
                 wizard_check('wizard-machine-adapter-tests'),
                 wizard_check('wizard-core918-native-sql-safety-tests')])
        absent = sorted(CORE_918_REQUIRED_GATES - available)
        if absent:
            unresolved.append('reviewed core-918 required gates absent: ' + ', '.join(absent))
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'unresolved' if unresolved else 'resolved',
                'unresolved_coverage': unresolved, 'broad_required': bool(unresolved),
                'selected_gates': sorted(CORE_918_REQUIRED_GATES & available),
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': ('All checks and both native lifecycle gates must pass for '
                                        'the currently verified integration; missing/failed/skipped '
                                        'results fail closed.'),
                'reasons': ['Exact 21-file core-918 candidate, reviewed source SHA and '
                            'clean trusted-target integration only; no file-wide exemption']}
    # The exact reviewed orchestration bundle receives named targeted coverage,
    # not a directory-level security/evidence exception.
    if (valid and not diff['broad_required'] and
            identifiers['head_sha'] == CORE_893_REVIEWED_HEAD_SHA and
            identifiers['merge_base_sha'] == CORE_893_TRUSTED_MERGE_BASE_SHA and
            identifiers['integration_sha'] is not None and
            len(paths) == len(AUDITED_CORE_893_PATHS) and
            set(paths) == AUDITED_CORE_893_PATHS):
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
    # This authority lives on trusted develop. Only the reviewed source or
    # proven clean first-parent merges from trusted target history qualify.
    if (valid and not diff['broad_required'] and not unresolved and
            identifiers['integration_sha'] is not None and
            len(paths) == len(REGISTRATION_CI_REVIEWED_FILES) and
            set(paths) == set(REGISTRATION_CI_REVIEWED_FILES) and
            all(ch.get('status') == REGISTRATION_CI_REVIEWED_FILES.get(ch.get('path'))
                for ch in diff['changes']) and
            core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                         identifiers['head_sha'], REGISTRATION_CI_REVIEWED_HEAD)):
        require(CI_CHECKS)
        require(CODE_CHECKS)
        require(CLI_CHECKS)
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'resolved', 'unresolved_coverage': [],
                'broad_required': False, 'selected_gates': [],
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': ('Require every listed validation; exact reviewed source '
                    'integration does not accept registration task or future handler changes.'),
                'reasons': ['independently reviewed exact #974 registration CI source; '
                            'mandatory CI/source/CLI checks plus hosted focused registration regressions']}
    # core-921 reviewed production registration coverage. No path-only approval:
    # the exact shared registration handlers, the golden contract/registry, the
    # CLI/MCP parity sources and the six EN/DE reference docs must arrive as this
    # complete reviewed status set on a clean trusted-target merge. Unrelated
    # changes after the review commit fail closed; the normal merge proof checks
    # each merge tree. This is a shared-source coverage binding, not task
    # acceptance or authorization of future handler edits.
    # The native-proven candidate includes the complete SDK/fixture regression
    # inventory; an older registration-only subset cannot reuse its authority.
    if (valid and not diff['broad_required'] and not unresolved and
            identifiers['integration_sha'] is not None and
            len(paths) == len(CORE_921_PRODUCTION_REVIEWED_FILES) and
            set(paths) == set(CORE_921_PRODUCTION_REVIEWED_FILES) and
            all(ch.get('status') == CORE_921_PRODUCTION_REVIEWED_FILES.get(ch.get('path'))
                for ch in diff['changes']) and
            core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                         identifiers['head_sha'], CORE_921_PRODUCTION_REVIEWED_HEAD)):
        # Require the affected coverage explicitly so the new families actually
        # execute: the shared registration CI matrix, the machine-adapter and
        # provider-choice regression families, the core-921 TestCore921
        # regressions, plus the cmd/baha formatting gate. cli-guided-tests only
        # runs ^TestGuided and the existing registration runner does not select
        # ^TestCore921, so these are required as named lanes.
        require(CI_CHECKS)
        require(WORKFLOW_SYNTAX_CHECKS)
        require(CODE_CHECKS)
        require(CLI_CHECKS)
        require(DOC_CHECKS)
        require(WIZARD_BASE_CHECKS)
        require([wizard_check('wizard-registration-tests'),
                  wizard_check('wizard-machine-adapter-tests'),
                  wizard_check('wizard-provider-choice-tests'),
                  wizard_check('wizard-core921-tests')])
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'resolved', 'unresolved_coverage': [],
                'broad_required': False, 'selected_gates': [],
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': ('Require every listed validation; exact reviewed '
                    'source integration does not accept registration task or future handler changes.'),
                'reasons': ['independently reviewed exact core-921 production registration '
                            'bundle; mandatory CI/source/CLI/documentation checks on the complete '
                            'reviewed file set and clean trusted-target merge']}
    # Exact independently inspected #1008 fixture source only; no generic test exemption.
    if (valid and not diff['broad_required'] and not unresolved and
            identifiers['integration_sha'] is not None and
            len(paths) == len(APPID_FIX_REVIEWED_FILES) and
            set(paths) == set(APPID_FIX_REVIEWED_FILES) and
            all(ch.get('status') == APPID_FIX_REVIEWED_FILES.get(ch.get('path'))
                for ch in diff['changes']) and
            core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                         identifiers['head_sha'], APPID_FIX_REVIEWED_HEAD)):
        require(CODE_CHECKS)
        require(CLI_CHECKS)
        require(CI_CHECKS)
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'resolved', 'unresolved_coverage': [],
                'broad_required': False, 'selected_gates': [],
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': ('All hosted checks required; native Docker '
                                        'acceptance remains separately mandatory.'),
                'reasons': ['exact reviewed #1008 fixture-only correction']}
    # Reuse the existing clean trusted-target merge proof, bound to the exact
    # independently reviewed wizard CI commit and its complete seven-file diff.
    if (valid and not diff['broad_required'] and not unresolved and
            identifiers['integration_sha'] is not None and
            len(paths) == len(WIZARD_CI_REVIEWED_PATHS) and
            set(paths) == WIZARD_CI_REVIEWED_PATHS and
            all(ch.get('status') == ('added' if ch.get('path') ==
                'scripts/test_pr_wizard_workflow_route.py' else 'modified')
                for ch in diff['changes']) and
            core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                         identifiers['head_sha'], WIZARD_CI_REVIEWED_HEAD)):
        require(CI_CHECKS)
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'resolved', 'unresolved_coverage': [],
                'broad_required': False, 'selected_gates': [],
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': ('Enforce all required_validation_checks independently of selected_gates; '
                    'zero selected_gates is valid only when coverage_status is resolved. '
                    'If unresolved, final check MUST fail closed; never treat as no checks '
                    'or automatically run the complete release catalog.'),
                'reasons': ['independently reviewed exact #957 wizard planner/executor bundle; '
                            'focused hosted Go/workflow-route regressions and mandatory CI checks']}
    # Review-owned trusted policy for exact #953 commit. Never read a policy
    # record from PR head. The verified diff resolver validates integration
    # parents/tree separately; a moved target base is allowed, but PR content
    # and original merge-base remain immutable.
    if (valid and not diff['broad_required'] and not unresolved and
            core752_reviewed_merge_chain(identifiers['target_base_sha'], identifiers['head_sha']) and
            identifiers['integration_sha'] is not None and
            len(paths) == len(CORE_752_CI_PATHS) and
            set(paths) == CORE_752_CI_PATHS and
            all(ch.get('status') == 'modified' for ch in diff['changes'])):
        require(CI_CHECKS)
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'resolved', 'unresolved_coverage': [],
                'broad_required': False, 'selected_gates': [],
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': ('Enforce all required_validation_checks independently of selected_gates; '
                    'zero selected_gates is valid only when coverage_status is resolved. '
                    'If unresolved, final check MUST fail closed; never treat as no checks '
                    'or automatically run the complete release catalog.'),
                'reasons': ['independently reviewed exact #953 CI executor and planner change; '
                            'trusted CI validations and mandatory focused Python regressions']}
    binding_checks = trusted_binding_checks(
        diff, repository, context_repository, context_target_branch) if not unresolved else None
    if binding_checks:
        require(binding_checks)
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'resolved', 'unresolved_coverage': [],
                'broad_required': False, 'selected_gates': [],
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': 'All authenticated current-integration CI checks must execute and pass.',
                'reasons': ['Trusted target record: exact repository, branch, head, full '
                            'inventory, raw SHA-256 blobs and clean integration verified.']}
    for path in paths:
        if path in REGISTRATION_COVERAGE:
            area = 'registration-reviewed-path'
            require(CODE_CHECKS)
            require(WIZARD_BASE_CHECKS)
            require([wizard_check('wizard-registration-tests')])
            if path in REGISTRATION_REFERENCE_FILES:
                require(DOC_CHECKS)
            if path_status.get(path) not in ('added', 'modified'):
                unresolved.append('shared registration rename/delete/type change needs independent review: ' + path)
        elif path == CORE_754_TEST_PATH:
            area = 'core754-mcp-test-unresolved'
            require(WIZARD_BASE_CHECKS)
            require([wizard_check(CORE_754_TEST_CHECK)])
            if (len(paths) != 1 or path_status.get(path) != 'added' or
                    not (identifiers['head_sha'] == CORE_754_TEST_HEAD or
                         core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                                      identifiers['head_sha'],
                                                      CORE_754_TEST_HEAD))):
                unresolved.append('core-754 test source differs from exact reviewed candidate')
            unresolved.append('core-754 MCP test content requires independent source qualification: ' + path)
        elif path in {
            'cmd/baha/commands.go',
            'cmd/baha/app_commands.go',
            'cmd/baha/human_new.go',
            'cmd/baha/machine_coverage.go',
            'cmd/baha/mcp.go',
            'cmd/baha/mcp_tools.go',
            'cmd/baha/mcp_application_authoring.go',
            'cmd/baha/mcp_workspace_mutation.go',
        } or (path.startswith('cmd/baha/mcp_') and path.endswith('.go')):
            # Registration and execution logic coexist here; mere paths or
            # generic tests cannot qualify changed authorizers/mutators.
            area = 'registration-mixed-unresolved'
            unresolved.append('mixed registration/execution source needs independent diff review: ' + path)
        elif (path.startswith('docs/') and path.endswith(('.md', '.rst')) or
                path in ('README.md', 'CONTRIBUTING.md', 'CHANGELOG.md',
                         'mkdocs.yml', 'mkdocs.de.yml') or
                path.startswith('site/')):
            area = 'documentation'
            require(DOC_CHECKS)
        elif path in READ_ONLY_CLI:
            area = 'cli-read-only'
            require(CODE_CHECKS)
            require(CLI_CHECKS)
            selected.add('integration/static/public-contracts')
        elif path in WIZARD_COMPONENTS:
            area = 'wizard-component'
            check_id, native_gates = WIZARD_COMPONENTS[path]
            require(CODE_CHECKS)
            require(WIZARD_BASE_CHECKS)
            require([wizard_check(check_id)])
            selected.update(native_gates)
            if path_status.get(path) not in ('added', 'modified'):
                unresolved.append('wizard component rename/delete/type change needs explicit review: ' + path)
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
        elif path in INSPECT_IMPORT_CANDIDATE_FILES:
            area = 'inspect-import-unreviewed-source'
            require(INSPECT_CHECKS)
            if path_status.get(path) != INSPECT_IMPORT_CANDIDATE_FILES[path]:
                unresolved.append('import detector status requires source review: ' + path)
            unresolved.append('import detector requires independent immutable source binding: ' + path)
        elif path in INSPECT_RECONCILE_REVIEWED:
            area = 'inspect-reconciliation'
            require(INSPECT_CHECKS)
        elif path.startswith('internal/repositoryinspect/') and path.endswith('.go'):
            area = 'inspect-implementation-unresolved'
            require(INSPECT_CHECKS)
            unresolved.append('unreviewed repository inspection file needs targeted qualification: ' + path)
        elif path.startswith('contracts/') and path.endswith(('.go', '.json')):
            area = 'contract'
            require(CODE_CHECKS)
            selected.add('integration/static/public-contracts')
        elif path.startswith('deploy/docker/'):
            area = 'docker-deployment-unresolved'
            require(CODE_CHECKS)
            unresolved.append('deployment/security/backup script impact needs specific native qualification: ' + path)
        elif path in R27_PROTOTYPE_PATHS:
            if path_status.get(path) not in ('added', 'modified'):
                unresolved.append('r27 prototype deletion/rename/type change requires independent review: ' + path)
            area = 'r27-prototype-exact-path'
            require(R27_PROTOTYPE_CHECKS)
            require(CI_CHECKS)
        elif path == '.github/workflows/r27-prototype-tests.yml':
            if path_status.get(path) not in ('added', 'modified'):
                unresolved.append('r27 prototype workflow deletion/rename/type change requires independent review: ' + path)
            area = 'r27-prototype-workflow-exact-path'
            require(R27_PROTOTYPE_CHECKS)
            require(CI_CHECKS)
            require(WORKFLOW_SYNTAX_CHECKS)
        elif path in PLANNING_WORKFLOWS:
            area = 'documentation-workflow'
            require(DOC_CHECKS)
            require(CI_CHECKS)
            require(WORKFLOW_SYNTAX_CHECKS)
        elif path in PLANNING_SCRIPTS:
            area = 'ci-planner'
            require(CI_CHECKS)
        elif path.startswith('.github/workflows/'):
            area = 'runtime-workflow-unresolved'
            require(CI_CHECKS)
            require(WORKFLOW_SYNTAX_CHECKS)
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
    diff = json.loads(args.diff.read_text())
    requirements = json.loads(args.requirements.read_text())
    mapping = json.loads(args.mapping.read_text())
    # Authenticated evaluation context comes from the runner, never from the
    # candidate PR. On GitHub Actions the runner sets GITHUB_REPOSITORY
    # (owner/name) and GITHUB_BASE_REF (target branch) in the pull_request
    # context; these cannot be altered by the PR author. A blank value means
    # the authenticated context is unknown, which must fail closed: an empty
    # context yields an unresolved plan, never a trusted one.
    context_repository = os.environ.get('GITHUB_REPOSITORY', '').strip()
    context_target_branch = os.environ.get('GITHUB_BASE_REF', '').strip()
    if not context_repository or not context_target_branch:
        raise SystemExit(
            'refusing to qualify without authenticated runner context: '
            'GITHUB_REPOSITORY=%r GITHUB_BASE_REF=%r'
            % (context_repository, context_target_branch))
    result = plan(diff, requirements, mapping,
                  context_repository=context_repository,
                  context_target_branch=context_target_branch)
    print(json.dumps(result, indent=2, sort_keys=True))


if __name__ == '__main__':
    main()
