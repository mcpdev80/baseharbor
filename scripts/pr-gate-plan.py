#!/usr/bin/env python3
"""Conservative v0.4.26 PR area-to-existing-gate selection (#892)."""
import argparse
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
AUTHORITY_PACKAGE_CHECKS = [
    {'id': 'authority-package-tests', 'owner': 'installation authority and stable identity',
     'command': 'python3 scripts/pr-check-runner.py execute --check authority-package-tests --repository .'},
]
AUTHORITY_PACKAGES = ('internal/coreinstallation', 'internal/stableid', 'internal/serviceaccess',
                      'internal/installauthority')


DELIVERY_PACKAGE_CHECKS = [{'id': 'delivery-package-tests', 'owner': 'delivery owner and selection contract',
    'command': 'python3 scripts/pr-check-runner.py execute --check delivery-package-tests --repository .'}]

DEPLOYMENT_PACKAGE_CHECKS = [{'id': 'deployment-package-tests', 'owner': 'deployment state and Target ownership',
    'command': 'python3 scripts/pr-check-runner.py execute --check deployment-package-tests --repository .'}]

BROKER_PACKAGE_CHECKS = [{'id': 'broker-package-tests', 'owner': 'runtime broker package',
    'command': 'python3 scripts/pr-check-runner.py execute --check broker-package-tests --repository .'}]

CLI_PACKAGE_CHECKS = [{'id': 'cli-package-tests', 'owner': 'cmd/baha package tests',
    'command': 'go test ./cmd/baha -count=1'}]

RUNTIME_PROVIDER_CHECKS = [
    {'id': 'runtime-provider-package-tests', 'owner': 'runtime and provider packages',
     'command': 'go test ./internal/runtime/... ./internal/health ./internal/providers/runtime/podman -count=1'},
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
                             ('atomic/docker/data-capabilities', 'atomic/podman/data-capabilities')),
    'cmd/baha/app_edit_test.go': ('wizard-app-edit-tests',
                                  ('atomic/docker/data-capabilities', 'atomic/podman/data-capabilities')),
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

# Exact reviewed appid test-regression follow-up (#1046): six test-only sources
# normalizing t.TempDir() roots to 0700 so machine umask cannot mask the
# protected-owner guards differently between local runs and CI. No production,
# parser or guard semantics change; no generic internal/application exemption.
APPID_ENV_ROBUST_TEST_REVIEWED_HEAD = '1ec67f8d4b259d62bd108ee678f25c288b8b0564'
APPID_ENV_ROBUST_TEST_REVIEWED_FILES = {
    'internal/application/external_provider_registry_test.go': 'modified',
    'internal/application/external_provider_removal_retry_test.go': 'modified',
    'internal/application/external_provider_scope_test.go': 'modified',
    'internal/application/protected_tempdir_test.go': 'added',
    'internal/application/runtime_binding_projection_reset_test.go': 'modified',
    'internal/application/telemetry_authorization_test.go': 'modified',
}

# Exact final-check repair: await mandatory policy and bounded authenticated
# API job readiness. The adapter comes only from the immutable trusted base;
# the verifier and original identity guards stay unchanged. No runtime exception.

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

# Owner-authorized exact read-only verifier correction, with a real resolver
# regression. This is source qualification; final native acceptance is separate.
UNBOUND_SQL_RETIREMENT_REVIEWED_HEAD = '938adee78ec2d10536f1834829facd8d9b1bec09'
UNBOUND_SQL_RETIREMENT_REVIEWED_FILES = {
    'cmd/baha/application_runtime_retirement_verification.go': 'modified',
    'cmd/baha/application_runtime_retirement_verification_test.go': 'modified',
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
EDIT_SQL_PREVIEW_REVIEWED_HEAD = 'd26a69c88e6a42ee3de5c0b9fb1aef8ff73d851d'
EDIT_SQL_PREVIEW_REVIEWED_FILES = {
    'cmd/baha/app_apply.go': 'modified',
    'cmd/baha/app_apply_result.go': 'modified',
    'cmd/baha/app_core_requirement.go': 'modified',
    'cmd/baha/app_edit.go': 'modified',
    'cmd/baha/app_edit_apply_result.go': 'modified',
    'cmd/baha/app_edit_test.go': 'modified',
    'cmd/baha/app_machine_adapters.go': 'modified',
    'cmd/baha/app_wizard_controller.go': 'modified',
    'cmd/baha/app_wizard_controller_test.go': 'modified',
    'cmd/baha/guided_application_read_context.go': 'modified',
}
EDIT_SQL_PREVIEW_CHECKS = ('wizard-app-edit-tests', 'wizard-provider-choice-tests',
                           'wizard-controller-tests', 'wizard-app-new-tests',
                           'wizard-machine-adapter-tests')
EDIT_SQL_PREVIEW_GATES = {'atomic/docker/data-capabilities', 'atomic/podman/data-capabilities'}
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
# Exact reviewed runtime native-binding projection of PR #1096 (b7b21dad): the
# credential-material + host-port projection source of the switch-back retention
# change. Its paired package regression runs under the existing source-contracts
# check (go test ./internal/application/... ./contracts/...), which qualifies
# this production file exactly like the receipt-pilot sources in the #1091
# bootstrap (83d53e86). No new gate, no runner change: the exact reviewed
# candidate resolves while every other internal/application/ source stays on
# the conservative unresolved branch below.
PROJECTION_REVIEWED_HEAD = 'b7b21dadfc2b4826d51be8a059328bcbdcad1a87'
PROJECTION_REVIEWED_FILES = frozenset({
    'internal/application/runtime_native_binding_projection.go',
    'internal/application/runtime_environment.go',
    'internal/application/runtime_placement_retention_test.go',
})
# #1265 application-implementation-unresolved (gatemap-r4): the manifest parser
# drives the real App-Apply/Promotion path.  manifest.go renders the concrete
# Manifest with its Provider/SQL/Valkey/Identity/TLS/readiness resolution;
# manifest_parse.go parses and validates it.  Bind these EXACT three files to
# the two EXISTING native lifecycle gates (atomic/docker/lifecycle,
# atomic/podman/lifecycle) — the #1259/#1262 pattern: reference existing gates,
# NO new gate, no wildcard over internal/application/*. Every other
# internal/application/* source stays on the conservative unresolved branch.
GATEMAP_R4_MANIFEST_FILES = frozenset({
    'internal/application/manifest.go',
    'internal/application/manifest_parse.go',
    'internal/application/v0425_contract_test.go',
})
PROJECTION_CHECKS = CODE_CHECKS
OPENBAO_CHECKS = [
    {'id': 'openbao-package-tests', 'owner': 'internal/openbao',
     'command': 'python3 scripts/pr-check-runner.py execute --check openbao-package-tests --repository .'},
]
LOGS_CHECKS = [
    {'id': 'logs-package-tests', 'owner': 'internal/logs',
     'command': 'python3 scripts/pr-check-runner.py execute --check logs-package-tests --repository .'},
]
OPENBAO_RETRY_HEAD = '9f715184af7b89ca3011aa4a0636c45138fe2c6b'
OPENBAO_RETRY_FILES = {'internal/openbao/bootstrap.go': 'modified',
                       'internal/openbao/bootstrap_test.go': 'modified'}
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
# core-750 permanent lane: exact reviewed source of the source-neutral shared
# summary projection. Like INSPECT_RECONCILE_REVIEWED this is an exact reviewed
# path allowlist, never a prefix or name heuristic: the package tests at
# internal/repositoryinspect/ cover this production file and its parity test,
# with zero native release gates. Every other internal/repositoryinspect/*.go
# file stays fail-closed unresolved on the branch below.
INSPECT_PROJECTION_REVIEWED = frozenset([
    'internal/repositoryinspect/inspect.go',
    'internal/repositoryinspect/workload_source_parity_test.go',
])
# Focused, package-scoped checks for the runtime resolver routing surface.
# The resolver is pure in-memory target routing with a thin cmd/baha consumer,
# so package unit tests, vet and the consumer build cover its actual risk
# without a native runtime gate. They never exempt the whole package: any
# other internal/runtime/resolver/*.go stays unresolved below.
RESOLVER_CHECKS = [
    {'id': 'resolver-gofmt', 'owner': 'internal/runtime/resolver formatting',
     'command': 'test -z "$(gofmt -l internal/runtime/resolver)"'},
    {'id': 'resolver-package-tests', 'owner': 'internal/runtime/resolver',
     'command': 'go test ./internal/runtime/resolver/ -count=1'},
    {'id': 'resolver-vet', 'owner': 'internal/runtime/resolver',
     'command': 'go vet ./internal/runtime/resolver/'},
    {'id': 'resolver-consumer-build', 'owner': 'cmd/baha package',
     'command': 'go build -o "$RUNNER_TEMP/baha-resolver" ./cmd/baha'},
]
# core-1220 permanent lane: exact reviewed source of the resolver routing
# qualification. Like INSPECT_PROJECTION_REVIEWED and INSPECT_RECONCILE_REVIEWED
# this is an exact reviewed path allowlist, never a prefix or name heuristic:
# the package tests at internal/runtime/resolver/ cover this production file,
# with zero native release gates. Every other internal/runtime/resolver/*.go
# file stays fail-closed unresolved on the branch below.
RESOLVER_ROUTING_REVIEWED = frozenset([
    'internal/runtime/resolver/resolver.go',
    'internal/runtime/resolver/resolver_test.go',
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

# Immutable independently reviewed #953 CI/executor change; source-tree binding
# is the commit SHA, not a reusable matching-path exemption.  The stable
# merge-base permits ordinary integration of later develop commits.
CORE_752_CI_HEAD = '211d19b13a206040f0f32506e5f5f85d4a467287'
# Original review base is embodied in the authenticated reviewed commit ancestry.
# The current merge-base changes after normal develop integration.
CORE_752_CI_MERGE_BASE = 'e8b021fe63c61abc0990a560e5f2915bb70c2c13'

# Independent manager qualification of the corrected #957 source. This record
# is read only from trusted develop; it cannot authorize an unreviewed follow-up.

# Independent Maintainer source review for registration CI (#974), not task
# acceptance or authorization of future core-921 production registration edits.
# Evidence: run 38929047340; actual hosted route pilots remain required after
# integration. The immutable commit and exact statuses bind this bootstrap.

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

# PR #1272 exact reviewed candidate (registration-mixed fix, head 72c14450):
# three modified sources — a negative-schema regression test, the Workload.Compose
# omitempty authoring fix, and the restored CHANGELOG note. No production authoring
# or mutation semantics beyond the omitempty tag; no generic cmd/baha/mcp_ or
# internal/application exemption. Coverage resolves only for this exact three-file
# candidate with a clean trusted-target integration, and only if the two native
# lifecycle gates actually execute the new cmd/baha tests (core-754/mcp pattern),
# not merely path-prefix or generic test coverage.
PR1272_REGISTRATION_MIXED_HEAD = '72c14450e13f63b05f3d29bd2c4e87c65b11488c'
PR1272_REGISTRATION_MIXED_FILES = {
    'cmd/baha/mcp_application_authoring_test.go': 'modified',
    'internal/application/manifest.go': 'modified',
    'CHANGELOG.md': 'modified',
}
PR1272_REGISTRATION_MIXED_GATES = {'atomic/docker/lifecycle', 'atomic/podman/lifecycle'}

# cmd/baha test files that are bound by an EXACT reviewed-test-candidate clause
# (APPID_FIX and its follow-ups). A PR whose head does NOT match that exact
# reviewed SHA keeps the fail-closed ancestry contract and must stay unresolved
# rather than inherit the generic cmd/baha/*_test.go cli-gofmt binding. Any
# OTHER cmd/baha/*_test.go (not in this set) is eligible for the generic
# test-only binding below.
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
# Commit receipts record only the merged SHA and PR. Their normal regression
# tests run with CI helper changes; no separate evidence or approval path exists.
RECEIPT_CONTROL_PLANE_SCRIPTS = frozenset({
    'scripts/receipt_writer.py', 'scripts/ci-workflow-policy.sh',
})
RECEIPT_CONTROL_PLANE_TEST_SCRIPTS = frozenset({
    'scripts/test_receipt_writer.py', 'scripts/test_receipt_postrun.py',
})
RECEIPT_CONTROL_PLANE_WORKFLOWS = frozenset({'.github/workflows/receipt-post-run.yml'})

CI_CHECKS = [
    {'id': 'pr-planner-tests', 'owner': 'scripts/test_pr_gate_plan.py',
     'command': 'python3 -m unittest scripts/test_pr_gate_plan.py'},
    {'id': 'ci-policy', 'owner': 'scripts/ci-workflow-policy.sh',
     'command': 'bash scripts/ci-workflow-policy.sh'},
    {'id': 'ci-executor-tests', 'owner': 'normal selected CI regressions',
     'command': 'python3 scripts/pr-check-runner.py execute --check ci-executor-tests --repository .'},
    {'id': 'receipt-execution-tests', 'owner': 'v0.4.27 receipt control plane',
     'command': 'PYTHONPATH=scripts python3 -m unittest scripts/test_receipt_writer.py scripts/test_receipt_postrun.py'},
]

WORKFLOW_SYNTAX_CHECKS = [
    {'id': 'ci-actionlint', 'owner': '.github/workflows workflow syntax',
     'command': 'go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck='},
]

# Lightweight Pins-Data validation for release consumer-pins metadata. A
# docs/releases/<tag>.consumer-pins.json file is immutable source data, not a
# code path: it records the exact immutable Core/Connector/Console/Demo commits
# that a release must reproduce. The pinned consumer-repository identities are
# private, so the validation reads only the file's embedded origin repository
# and commit and re-derives the acyclic, source-owned consumer graph without
# publishing or fetching private sources. No runtime gate is selected: a pins
# metadata add changes no executable code and requires no native release gate.
PINNED_CONSUMER_REPOSITORIES = {
    'core': 'mcpdev80/baseharbor', 'demo': 'mcpdev80/baseharbor-demo',
    'console': 'mcpdev80/baseharbor-console',
    'connector': 'mcpdev80/baseharbor-node-connector',
}
# Each gate must pin exactly one consumer role from the correct repository with
# a 40-hex commit; the joint live-console gate additionally requires the
# Connector origin that seeds the Console origin. This is the public surface of
# the immutable chain the release resolver reproduces.
PINNED_PINS_VALIDATION = [
    {'id': 'pins-data-validation', 'owner': 'scripts/ecosystem_release_pins.py',
     'command': ('python3 scripts/pr-check-runner.py execute --check pins-data-validation '
                 '--repository .')},
]
def openbao_retry_checks(diff, repository='.', context_repository=None,
                         context_target_branch=None):
    """Bind the frozen retry candidate and clean target merges to strict tests."""
    valid, _ = validated_diff(diff)
    context_repository = os.environ.get('GITHUB_REPOSITORY', '') if context_repository is None else context_repository
    context_target_branch = os.environ.get('GITHUB_BASE_REF', '') if context_target_branch is None else context_target_branch
    if (not valid or diff['broad_required'] or not diff.get('integration_sha') or
            context_repository != 'mcpdev80/baseharbor' or context_target_branch != 'develop' or
            len(diff['changes']) != len(OPENBAO_RETRY_FILES) or
            {row.get('path'): row.get('status') for row in diff['changes']} != OPENBAO_RETRY_FILES):
        return None
    if not core752_reviewed_merge_chain(diff['target_base_sha'], diff['head_sha'],
                                        OPENBAO_RETRY_HEAD, repository):
        return None
    try:
        import importlib.util
        spec = importlib.util.spec_from_file_location('openbao_diff', ROOT / 'pr-diff-base.py')
        resolver = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(resolver)
        actual = resolver.resolve_pr_diff(repository, diff['target_base_sha'],
                                          diff['head_sha'], diff['integration_sha'])
        if actual != diff or actual['broad_required']:
            return None
    except (OSError, subprocess.CalledProcessError, ValueError, TypeError, KeyError):
        return None
    return CODE_CHECKS + OPENBAO_CHECKS



def targeted_plan(diff, requirements, mapping, repository='.',
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
    if (valid and not diff['broad_required'] and not unresolved and
            identifiers['integration_sha'] is not None and
            len(paths) == len(EDIT_SQL_PREVIEW_REVIEWED_FILES) and
            path_status == EDIT_SQL_PREVIEW_REVIEWED_FILES and
            core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                         identifiers['head_sha'],
                                         EDIT_SQL_PREVIEW_REVIEWED_HEAD, repository)):
        require(CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS +
                [wizard_check(name) for name in EDIT_SQL_PREVIEW_CHECKS])
        absent = sorted(EDIT_SQL_PREVIEW_GATES - available)
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'unresolved' if absent else 'resolved',
                'unresolved_coverage': ['required SQL Edit gates absent: ' + ', '.join(absent)] if absent else [],
                'broad_required': bool(absent),
                'selected_gates': sorted(EDIT_SQL_PREVIEW_GATES & available),
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': 'Every required validation and both native SQL gates must execute and pass.',
                'reasons': ['Owner-qualified SQL Edit source: exact ten-file source and clean trusted-target integration; no native waiver.']}
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
        (RETIREMENT_PREFERENCES_REVIEWED_HEAD, RETIREMENT_PREFERENCES_REVIEWED_FILES,
         CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS,
         'reviewed withdrawal preferences projection preserving canonical owners'),
        (UNBOUND_SQL_RETIREMENT_REVIEWED_HEAD, UNBOUND_SQL_RETIREMENT_REVIEWED_FILES,
         CODE_CHECKS + CLI_CHECKS + WIZARD_BASE_CHECKS +
         [wizard_check('wizard-runtime-retirement-tests')],
         'Owner-qualified already-unbound logical SQL verification; native acceptance remains separate'),
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
    # PR #1272 registration-mixed authoring fix: three modified files only. No
    # generic cmd/baha/mcp_ or internal/application exemption. Coverage resolves
    # only for this exact three-file candidate with a clean trusted-target merge
    # and only if both native lifecycle gates are present and actually run the new
    # cmd/baha tests; any other head, path set or status, or a missing lifecycle
    # gate keeps the registration-mixed fallthrough unresolved.
    if (valid and not diff['broad_required'] and not unresolved and
            identifiers['integration_sha'] is not None and
            len(paths) == len(PR1272_REGISTRATION_MIXED_FILES) and
            set(paths) == set(PR1272_REGISTRATION_MIXED_FILES) and
            all(ch.get('status') == PR1272_REGISTRATION_MIXED_FILES.get(ch.get('path'))
                for ch in diff['changes']) and
            core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                         identifiers['head_sha'],
                                         PR1272_REGISTRATION_MIXED_HEAD)):
        require(WIZARD_BASE_CHECKS)
        absent = sorted(PR1272_REGISTRATION_MIXED_GATES - available)
        if absent:
            unresolved.append('reviewed registration-mixed lifecycle gates absent: ' + ', '.join(absent))
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'unresolved' if unresolved else 'resolved',
                'unresolved_coverage': unresolved, 'broad_required': bool(unresolved),
                'selected_gates': sorted(PR1272_REGISTRATION_MIXED_GATES & available),
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': ('Every listed validation and both native lifecycle '
                    'gates must execute and pass for the exact reviewed candidate; missing, '
                    'skipped and failed results fail closed. Not a path-wide registration-mixed '
                    'exemption.'),
                'reasons': ['PR #1272 exact three-file registration-mixed candidate at '
                            '72c14450 with clean trusted-target integration; both native '
                            'lifecycle gates must run the new cmd/baha authoring tests']}
    # The exact reviewed orchestration bundle receives named targeted coverage,
    # not a directory-level security/evidence exception.
    # This authority lives on trusted develop. Only the reviewed source or
    # proven clean first-parent merges from trusted target history qualify.
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
    # Exact reviewed appid test-regression follow-up only; coverage is the
    # existing static package validation that runs the changed test sources.
    if (valid and not diff['broad_required'] and not unresolved and
            identifiers['integration_sha'] is not None and
            len(paths) == len(APPID_ENV_ROBUST_TEST_REVIEWED_FILES) and
            set(paths) == set(APPID_ENV_ROBUST_TEST_REVIEWED_FILES) and
            all(ch.get('status') == APPID_ENV_ROBUST_TEST_REVIEWED_FILES.get(ch.get('path'))
                for ch in diff['changes']) and
            core752_reviewed_merge_chain(identifiers['target_base_sha'],
                                         identifiers['head_sha'],
                                         APPID_ENV_ROBUST_TEST_REVIEWED_HEAD)):
        require(CODE_CHECKS)
        return {'schema': 'baseharbor.pr-gate-plan/v1', **identifiers,
                'mapping_schema': mapping['schema'], 'release': release,
                'coverage_status': 'resolved', 'unresolved_coverage': [],
                'broad_required': False, 'selected_gates': [],
                'required_validation_checks': [validations[k] for k in sorted(validations)],
                'validation_contract': ('Require every listed validation; the exact reviewed '
                    'test-only source adds no product or guard behavior beyond the '
                    'umask-deterministic tmp roots.'),
                'reasons': ['exact reviewed appid test-regression follow-up: umask-deterministic '
                            'protected tmp roots in internal/application test sources only']}
    # Reuse the existing clean trusted-target merge proof, bound to the exact
    # independently reviewed wizard CI commit and its complete seven-file diff.
    # Review-owned trusted policy for exact #953 commit. Never read a policy
    # record from PR head. The verified diff resolver validates integration
    # parents/tree separately; a moved target base is allowed, but PR content
    # and original merge-base remain immutable.
    binding_checks = (openbao_retry_checks(
        diff, repository, context_repository, context_target_branch)
        if not unresolved else None)
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
        if re.fullmatch(r'docs/releases/v0\.4\.(?:23|24|25|26)\.consumer-pins\.json', path):
            area = 'pins-data'
            require(PINNED_PINS_VALIDATION)
            if path_status.get(path) != 'added':
                unresolved.append('pins-data file add requires exact reviewed candidate: ' + path)
        elif path == 'docs/releases/v0.4.26.selected-gates.json':
            area = 'release-preflight-selection'
            require(CI_CHECKS)
            expected = sorted(gate['id'] for gate in requirements['gates']
                              if release in gate['required_for']
                              and not gate['id'].startswith('journey/'))
            try:
                selection = json.loads(subprocess.check_output(
                    ['git', '-C', str(repository), 'show',
                     identifiers['head_sha'] + ':' + path], stderr=subprocess.DEVNULL))
                if (release != 'v0.4.26' or path_status.get(path) != 'added'
                        or selection != expected):
                    raise ValueError('selection differs from the complete preflight catalog')
            except (OSError, subprocess.CalledProcessError, ValueError, TypeError):
                unresolved.append('release preflight selection must match trusted non-journey catalog')
        elif path in REGISTRATION_COVERAGE:
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
        elif path in ('cmd/baha/application_service_issuer.go',
                      'cmd/baha/application_service_issuer_test.go',
                      'internal/openbao/service_issuer.go', 'internal/openbao/service_issuer_test.go',
                      'internal/openbao/service_pki.go', 'internal/openbao/service_pki_test.go') or \
                re.fullmatch(r'internal/testsupport/serviceissuer/[^/]+\.go', path):
            area = 'application-authority-issuer'
            require(AUTHORITY_PACKAGE_CHECKS + OPENBAO_CHECKS + CODE_CHECKS + CLI_PACKAGE_CHECKS)
            if not path.endswith('_test.go') and not path.startswith('internal/testsupport/'):
                selected.update(('atomic/docker/lifecycle', 'atomic/podman/lifecycle'))
        elif path == 'cmd/baha/deployment_record.go':
            area = 'cli-deployment-owner'
            require(DEPLOYMENT_PACKAGE_CHECKS + CODE_CHECKS + CLI_PACKAGE_CHECKS + CLI_CHECKS)
            selected.update(('atomic/docker/lifecycle', 'atomic/podman/lifecycle'))
        elif path == 'cmd/baha/app_backup_restore_integration_test.go':
            area = 'native-backup-tests'
            require(CODE_CHECKS + CLI_PACKAGE_CHECKS)
            selected.update(('atomic/docker/backup-restore', 'atomic/podman/backup-restore'))
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
        elif path.startswith('cmd/baha/') and path.endswith('_engine.go'):
            # CLI-side docker_engine.go and similar *_engine.go files that call the
            # runtime and bind Docker/Podman engine observations to the Compose backend.
            # These are true runtime-mutation surfaces that affect BOTH runner backends
            # (Docker and Podman/Quadlet) and must exercise the real atomic lifecycle
            # gates on both.  Only the specific *_engine.go files are matched — no
            # directory-level blanket resolution of all cmd/baha/*.go.
            area = 'cli-engine-runtime-impact'
            require(CODE_CHECKS)
            require(CLI_CHECKS)
            selected.add('atomic/docker/lifecycle')
            selected.add('atomic/podman/lifecycle')
        elif path == 'cmd/baha/docker_engine_podman_test.go':
            area = 'cli-engine-unit-tests'
            require(CLI_PACKAGE_CHECKS + WIZARD_BASE_CHECKS)

        elif path == 'cmd/baha/runtime_restart_integration_test.go':
            area = 'control-plane-restart-tests'
            require(CODE_CHECKS + CLI_CHECKS)
            selected.update(('atomic/docker/control-plane-recovery', 'atomic/podman/control-plane-recovery'))

        elif path in (
                'cmd/baha/podman_doctor_checks.go',
                'cmd/baha/podman_doctor_checks_test.go'):
            # Podman-target doctor readiness checks (PodmanRuntimeDoctorChecks and
            # its regression test).  collectControlPlaneDoctorChecks in
            # cmd/baha/control_plane_operations.go appends podman + quadlet
            # readiness checks here BEFORE the core-deployment early return, but
            # ONLY for target.RuntimeProvider == "podman"; the Docker-target
            # doctor output is intentionally untouched.  These three files are a
            # single runtime-mutation surface that affects BOTH runner backends
            # (Docker doctor output must stay clean, Podman doctor output gains
            # two named checks) and must exercise the real atomic lifecycle gates
            # on both, exactly like the other cmd/baha runtime-mutation files.
            # Bind to the SAME existing native lifecycle gates (atomic/docker/
            # lifecycle, atomic/podman/lifecycle) — no new gate, no weakening.
            # Without this branch the three files fell to the
            # cli-unresolved-runtime-impact catch-all and blocked PR Selected
            # checks (#1267).  This branch must sit BEFORE the
            # cli-unresolved-runtime-impact catch-all.
            area = 'cli-podman-doctor-runtime-impact'
            require(CODE_CHECKS)
            require(CLI_CHECKS)
            selected.add('atomic/docker/lifecycle')
            selected.add('atomic/podman/lifecycle')
        elif path == 'cmd/baha/control_plane_operations.go' and path_status.get(path) == 'modified':
            # The control-plane doctor path that appends the podman/quadlet
            # readiness checks for a Podman target.  It is the exact mutation the
            # new PodmanRuntimeDoctorChecks wires into, affects both runner
            # backends (Docker output stays clean, Podman output gains two checks)
            # and binds to the SAME existing native lifecycle gates as the runtime
            # acceptance family (atomic/docker/lifecycle, atomic/podman/lifecycle)
            # — no new gate, no weakening.  Without this branch this file fell to
            # the cli-unresolved-runtime-impact catch-all and blocked PR Selected
            # checks (#1267).  This branch must sit BEFORE the
            # cli-unresolved-runtime-impact catch-all.
            area = 'cli-podman-doctor-runtime-impact'
            require(CODE_CHECKS)
            require(CLI_CHECKS)
            selected.add('atomic/docker/lifecycle')
            selected.add('atomic/podman/lifecycle')
        elif path in ('cmd/baha/app_repository_integration_test.go',
                      'cmd/baha/app_workload_integration_test.go',
                      'internal/application/runtime_multi_integration_test.go'):
            area = 'native-integration-tests'
            require(CODE_CHECKS + CLI_CHECKS)
            selected.add('atomic/docker/lifecycle')
            selected.add('atomic/podman/lifecycle')
            if path_status.get(path) not in ('added', 'modified'):
                unresolved.append('native integration test removal or rename needs review: ' + path)
        elif path == 'cmd/baha/app_observability_acceptance_test.go':
            # Full-stack observability acceptance for the CLI (TestObservabilityFullStack
            # AcceptanceInCI).  This boots a real native container runtime (Docker and
            # Podman/Quadlet) and runs the observability full-stack acceptance end to
            # end.  It is the exact proof the observability acceptance gates drive and
            # runs on BOTH runners (the observability-acceptance.yml dispatches this
            # test to the docker and podman blocks).  Bind it to the SAME catalog
            # observability gates as the runtime acceptance family (atomic/docker/
            # observability, atomic/podman/observability) — no new gate, no weakening.
            # Without this branch the file fell to the cli-unresolved-runtime-impact
            # catch-all and blocked PR Selected checks (#1242).
            area = 'observability-acceptance'
            require(CODE_CHECKS)
            require(CLI_CHECKS)
            selected.add('atomic/docker/observability')
            selected.add('atomic/podman/observability')
        elif re.fullmatch(r'internal/runtimebroker/[^/]+\.go', path):
            area = 'runtimebroker-package'
            require(BROKER_PACKAGE_CHECKS)
            if not path.endswith('_test.go'):
                require(CODE_CHECKS + CLI_CHECKS)
                selected.update(('atomic/docker/lifecycle', 'atomic/podman/lifecycle'))

        elif path.endswith('_test.go') and path.startswith('cmd/baha/'):
            # Mixed implementation/test PRs also execute new CLI assertions.
            # Required opt-in native fixtures keep their explicit routes above.
            area = 'cli-package-tests'
            require(CLI_PACKAGE_CHECKS)
            require(CODE_CHECKS)
            require(CLI_CHECKS)
            require(WIZARD_BASE_CHECKS)
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
        elif path in ('internal/capability/lifecycle.go',
                      'internal/capability/service_binding.go',
                      'internal/capability/service_binding_test.go'):
            # core-390 service-binding enforcement (#1229): the reviewed
            # lifecycle/service_binding implementation + its test are covered by
            # the package's own capability-package-tests check (already in
            # CAPABILITY_CHECKS) plus the existing static public-contracts gate.
            # Bind to those EXISTING gates only (#1260 pattern: reference existing
            # gates, no new gate) — no new release gate, no widening. Any other
            # internal/capability/* production file stays fail-closed unresolved
            # below.
            area = 'capability-implementation-resolved'
            require(CODE_CHECKS)
            require(CAPABILITY_CHECKS)
            selected.add('integration/static/public-contracts')
        elif path.startswith('internal/capability/'):
            area = 'capability-implementation-unresolved'
            require(CODE_CHECKS)
            require(CAPABILITY_CHECKS)
            unresolved.append('provider/capability registry or lifecycle impact needs targeted qualification: ' + path)
        elif path.startswith('testdata/adoption-parity/'):
            # #1222 static adoption fixtures (README/compose/k8s/quadlet). Bind to
            # the same focused inspect checks that already run the
            # repositoryinspect parity test (#1259 observability-binding pattern):
            # no new gate, no widening. Only this prefix resolves; any other
            # unmatched file still falls to the fail-closed catch-all below.
            area = 'inspect-testdata-fixed'
            require(INSPECT_CHECKS)
        elif path in PROJECTION_REVIEWED_FILES:
            area = 'application-projection-reviewed-path'
            require(PROJECTION_CHECKS)
            if path_status.get(path) != 'modified' and path_status.get(path) != 'added':
                unresolved.append('runtime native binding projection source status requires exact reviewed candidate: ' + path)
        elif path.startswith('internal/application/shared_backends') or \
                path.startswith('internal/application/shared_backend_dependency') or \
                'shared_backend' in path and path.endswith('.go'):
            # Shared-backend provider resolution (shared_backends.go and its
            # sisters: shared_backend_dependency.go / selectSharedBackendDependency).
            # A consumer Manifest's SQL/Valkey providers resolve to a shared
            # Postgres/Valkey backend; service identity, credentials, TLS and
            # readiness all key off the resolved backend.  This is genuine
            # Provider-Resolution-Impact (see PR #1255: a clobbered
            # state.Environment routed readiness probes to a non-existent
            # shared-postgres-core).  The qualification gate is the hosted
            # runtime-validation/ha/shared-postgres targeted run, which runs the
            # ./internal/application TestSharedPostgres family on both runners.
            # The aggregator only recognises the eight catalog tokens; there is no
            # `integration/shared-postgres` token and the mission forbids inventing
            # one.  Map to the existing atomic/podman/lifecycle lifecycle gate — a
            # real catalog token that already maps to a real native lifecycle run
            # covering this shared Postgres service-identity/credential path.
            # Without this branch the files fell to application-implementation
            # -unresolved and blocked PR Selected checks (#1255).  This branch must
            # sit BEFORE the internal/application/ catch-all.
            area = 'application-shared-provider-resolution'
            require(CODE_CHECKS)
            selected.add('atomic/podman/lifecycle')
        elif path in GATEMAP_R4_MANIFEST_FILES:
            # #1265 application-implementation-unresolved: the manifest parser
            # drives the real App-Apply/Promotion path.  manifest.go renders the
            # concrete Manifest and its Provider/SQL/Valkey/Identity/TLS/readiness
            # resolution; manifest_parse.go parses/validates it.  Binding to the
            # two EXISTING native lifecycle gates (#1259/#1262 pattern: reference
            # existing gates, NO new gate, no wildcard over internal/application/*)
            # proves the real provider-resolution + readiness path on BOTH runners.
            # v0425_contract_test.go pins the v0.4.25 contract for both lifecycles.
            area = 'application-manifest-lifecycle'
            require(CODE_CHECKS)
            selected.add('atomic/docker/lifecycle')
            selected.add('atomic/podman/lifecycle')
        elif ((path.startswith('internal/health/podman_') and '/' not in path[len('internal/health/'):]) or
              (path.startswith('internal/providers/runtime/podman/') and
               '/' not in path[len('internal/providers/runtime/podman/'):])) and path.endswith('.go'):
            area = 'podman-health-package'
            require(CODE_CHECKS + CLI_CHECKS + RUNTIME_PROVIDER_CHECKS)
            if path_status.get(path) not in ('added', 'modified', 'deleted', 'renamed', 'copied'):
                unresolved.append('unsupported runtime package change: ' + path)
            selected.add('atomic/docker/lifecycle')
            selected.add('atomic/podman/lifecycle')
        elif path in ('internal/logs/render.go', 'internal/logs/journal_provider_render_test.go'):
            # Rendering changes affect runtime log collection as well as Go behavior.
            area = 'logs-rendering'
            require(LOGS_CHECKS)
            selected.add('atomic/docker/observability')
            selected.add('atomic/podman/observability')
            if (path == 'internal/logs/journal_provider_render_test.go' and
                    path_status.get(path) not in ('added', 'modified')):
                unresolved.append('journal provider regression removal requires review: ' + path)
        elif path.startswith('internal/application/'):
            area = 'application-implementation-unresolved'
            require(CODE_CHECKS)
            unresolved.append('application lifecycle/provider resolution impact needs targeted qualification: ' + path)
        elif path.startswith('internal/openbao/'):
            # Fail-closed OpenBao handling: no blanket path exemption.
            # Only the reviewed bootstrap source files (bootstrap.go /
            # bootstrap_test.go) receive resolved coverage.  Any other
            # added, deleted, renamed or unreviewed OpenBao production
            # file stays unresolved and requires independent diff review.
            area = 'openbao-unreviewed'
            require(CODE_CHECKS)
            unresolved.append('OpenBao production impact requires source/status binding or independent diff review: ' + path)
        elif path in INSPECT_IMPORT_CANDIDATE_FILES:
            area = 'inspect-import-unreviewed-source'
            require(INSPECT_CHECKS)
            if path_status.get(path) != INSPECT_IMPORT_CANDIDATE_FILES[path]:
                unresolved.append('import detector status requires source review: ' + path)
            unresolved.append('import detector requires independent immutable source binding: ' + path)
        elif path in INSPECT_RECONCILE_REVIEWED:
            area = 'inspect-reconciliation'
            require(INSPECT_CHECKS)
        elif path in INSPECT_PROJECTION_REVIEWED:
            area = 'inspect-implementation-resolved'
            require(INSPECT_CHECKS)
        elif path.startswith('internal/repositoryinspect/') and path.endswith('.go'):
            area = 'inspect-implementation-unresolved'
            require(INSPECT_CHECKS)
            unresolved.append('unreviewed repository inspection file needs targeted qualification: ' + path)
        elif path in RESOLVER_ROUTING_REVIEWED:
            area = 'resolver-routing-resolved'
            require(RESOLVER_CHECKS)
        elif path.startswith('internal/runtime/') and path.endswith('.go') and not path.startswith('internal/runtime/resolver/'):
            area = 'runtime-package'
            require(RUNTIME_PROVIDER_CHECKS)
            if not path.endswith('_test.go'):
                require(CODE_CHECKS + CLI_CHECKS)
                selected.update(('atomic/docker/lifecycle', 'atomic/podman/lifecycle'))
        elif path.startswith('cmd/baha/') and path.endswith('_engine.go'):
            # CLI-side docker_engine.go and similar *_engine.go files that call the
            # runtime and bind Docker/Podman engine observations to the Compose backend.
            # These are true runtime-mutation surfaces that affect BOTH runner backends
            # (Docker and Podman/Quadlet) and must exercise the real atomic lifecycle
            # gates on both.  Only the specific *_engine.go files are matched — no
            # directory-level blanket resolution of all cmd/baha/*.go.
            area = 'cli-engine-runtime-impact'
            require(CODE_CHECKS)
            require(CLI_CHECKS)
            selected.add('atomic/docker/lifecycle')
            selected.add('atomic/podman/lifecycle')

        elif path.startswith('internal/runtime/resolver/') and path.endswith('.go'):
            area = 'resolver-routing-unresolved'
            require(RESOLVER_CHECKS)
            unresolved.append('unreviewed runtime resolver routing file needs targeted qualification: ' + path)
        elif re.fullmatch(r'internal/deployment/[^/]+\.go', path):
            area = 'deployment-package'
            require(DEPLOYMENT_PACKAGE_CHECKS)
            if not path.endswith('_test.go'):
                # State/owner mutations affect application consumers and both runtimes.
                require(CODE_CHECKS + CLI_CHECKS)
                selected.update(('atomic/docker/lifecycle', 'atomic/podman/lifecycle'))
        elif re.fullmatch(r'internal/delivery/[^/]+\.go', path):
            area = 'delivery-package'
            require(DELIVERY_PACKAGE_CHECKS)
            require(CLI_CHECKS)
        elif any(re.fullmatch(re.escape(package) + r'/[^/]+\.go', path)
                 for package in AUTHORITY_PACKAGES):
            area = 'authority-package'
            require(AUTHORITY_PACKAGE_CHECKS + CODE_CHECKS + CLI_CHECKS)
        elif path.startswith('contracts/') and path.endswith(('.go', '.json')):
            area = 'contract'
            require(CODE_CHECKS)
            selected.add('integration/static/public-contracts')
        elif path.startswith('deploy/docker/'):
            area = 'docker-deployment-unresolved'
            require(CODE_CHECKS)
            unresolved.append('deployment/security/backup script impact needs specific native qualification: ' + path)
        elif path in RECEIPT_CONTROL_PLANE_WORKFLOWS:
            area = 'receipt-control-plane-workflow'
            require(CI_CHECKS)
            require(WORKFLOW_SYNTAX_CHECKS)
        elif path in RECEIPT_CONTROL_PLANE_TEST_SCRIPTS:
            area = 'receipt-control-plane-test'
            require(CI_CHECKS)
        elif path in RECEIPT_CONTROL_PLANE_SCRIPTS:
            area = 'receipt-control-plane-script'
            require(CI_CHECKS)
        elif path in PLANNING_WORKFLOWS:
            area = 'documentation-workflow'
            require(DOC_CHECKS)
            require(CI_CHECKS)
            require(WORKFLOW_SYNTAX_CHECKS)
        elif path in PLANNING_SCRIPTS:
            area = 'ci-planner'
            require(CI_CHECKS)
        elif path.startswith('.github/workflows/'):
            area = 'ci-workflow'
            require(CI_CHECKS)
            require(WORKFLOW_SYNTAX_CHECKS)
        elif path in ('go.mod', 'go.sum'):
            # Module and dependency files: a bump changes exactly the Go build and
            # test surface, never product or gate behaviour. Bind it to the same
            # Go build/test validation checks the CI matrix runs for a Go change
            # (source-contracts, cli-build, cli-vet, cli-guided-tests, cli-gofmt)
            # -- no new gate, no widening. Without this branch a Dependabot-only
            # go.mod/go.sum diff fell to the 'unknown impact' catch-all and left
            # PR Selected Checks coverage-unresolved (#1160).
            area = 'go-module-dependency'
            require(CODE_CHECKS)
            require(CLI_CHECKS)
        elif (path.startswith('scripts/ci-') or path.startswith('scripts/pr-') or
              path.startswith('scripts/test_pr_') or
              path.startswith('scripts/verify-') or
              (path.startswith('scripts/') and path.endswith(('.py', '.sh', '.json', '.md'))) or
              path in ('AGENTS.md', '.github/copilot-instructions.md') or
              path.startswith('evidence-r27/') or path.startswith('evidence/')):
            area = 'ci-executor'
            require(CI_CHECKS)
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

CONSERVATIVE_CHECKS = [
    {'id': 'go-unit-tests', 'owner': 'all Go unit packages', 'command': 'go test ./... -count=1'},
    {'id': 'go-vet', 'owner': 'all Go packages', 'command': 'go vet ./...'},
    {'id': 'go-build', 'owner': 'all Go packages', 'command': 'go build ./...'},
]


def deduplicate_cli_package_validations(result):
    """One full CLI package run covers its Guided test subset at the same SHA.

    Keep the small Guided check when the full package is not selected. Only
    eliminate a test-subset execution, never independent format/vet/build,
    runtime-native, or security validation owners.
    """
    ids = {check['id'] for check in result['required_validation_checks']}
    if not {'cli-package-tests', 'cli-guided-tests'} <= ids:
        return result
    result = dict(result)
    result['required_validation_checks'] = [
        check for check in result['required_validation_checks']
        if check['id'] != 'cli-guided-tests'
    ]
    return result


def plan(diff, requirements, mapping, repository='.', context_repository=None,
         context_target_branch=None, *, conservative_fallback=False):
    """Try targeted selection; production CI tests valid unknown impact broadly."""
    result = targeted_plan(diff, requirements, mapping, repository,
                           context_repository, context_target_branch)
    if not conservative_fallback or result['coverage_status'] == 'resolved':
        return deduplicate_cli_package_validations(result)
    valid, _ = validated_diff(diff)
    reasons = result['unresolved_coverage']
    invalid = ('invalid diff-base', 'diff-base requires', 'invalid changed path',
               'unrecognized change status', 'empty or unverified changes',
               'required mapped gates absent', 'required SQL Edit gates absent',
               'release preflight selection')
    if (not valid or diff['broad_required'] or not diff.get('integration_sha')
            or context_repository not in (None, 'mcpdev80/baseharbor')
            or context_target_branch not in (None, 'develop')
            or any(reason.startswith(invalid) for reason in reasons)):
        return deduplicate_cli_package_validations(result)
    paths = [change[key] for change in diff['changes']
             for key in ('path', 'old_path', 'new_path') if key in change]
    if not paths:
        return deduplicate_cli_package_validations(result)
    checks = {row['id']: row for row in result['required_validation_checks']}
    for row in CONSERVATIVE_CHECKS:
        checks[row['id']] = row
    if any(path.startswith('internal/openbao/') for path in paths):
        for row in OPENBAO_CHECKS:
            checks[row['id']] = row
    gates = set(result['selected_gates'])
    if any(path.startswith(('cmd/', 'internal/', 'deploy/', 'contracts/', 'spec/', 'sdk/'))
           or path in ('go.mod', 'go.sum') for path in paths):
        gates.update(('atomic/docker/lifecycle', 'atomic/podman/lifecycle'))
    available = {gate['id'] for gate in requirements['gates']
                 if result['release'] in gate['required_for']}
    if gates - available:
        result['unresolved_coverage'].append('fallback requires unavailable runtime checks')
        return deduplicate_cli_package_validations(result)
    result.update(coverage_status='resolved', unresolved_coverage=[], broad_required=False,
                  fallback_used=True, fallback_reasons=reasons,
                  selected_gates=sorted(gates),
                  required_validation_checks=[checks[key] for key in sorted(checks)])
    result['reasons'].append('Unknown narrow coverage selects normal broad unit/vet/build checks and applicable runtime lifecycles.')
    return deduplicate_cli_package_validations(result)


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
    if (context_repository != 'mcpdev80/baseharbor' or
            context_target_branch != 'develop'):
        raise SystemExit(
            'refusing to qualify without authenticated runner context: '
            'GITHUB_REPOSITORY=%r GITHUB_BASE_REF=%r'
            % (context_repository, context_target_branch))
    result = plan(diff, requirements, mapping,
                  context_repository=context_repository,
                  context_target_branch=context_target_branch, conservative_fallback=True)
    print(json.dumps(result, indent=2, sort_keys=True))


if __name__ == '__main__':
    main()
