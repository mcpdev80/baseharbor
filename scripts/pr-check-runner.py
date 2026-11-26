#!/usr/bin/env python3
"""Trusted selected-check execution and normal CI test execution."""
import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

WIZARD_TEST_FAMILIES = {
    'wizard-runtime-retirement-tests': (
        r'^TestRuntimeRetirementVerifiesPreviouslyUnboundCoreSQLForHTTPOnlyApplication$',
        r'^TestRuntimeRetirementVerifiesReleasedLogicalSQLWithoutStoppingRequiredCore$',
        r'^TestRuntimeRetirementRejectsUnverifiedLogicalSQLRelease$',
        r'^TestRuntimeRetirementStillRequiresMeasuredPhysicalAbsence$',
        r'^TestReusedCoreSQLObservationRequiresExactPhysicalOwner$',
        r'^TestNativeBackendRetirement',
    ),
    'wizard-app-edit-load-tests': (r'^TestAppEditLoad',),
    'wizard-capability-selection-tests': (r'^TestAppCapabilitySelection',),
    'wizard-provider-choice-tests': (r'^TestAppProviderChoice',),
    'wizard-footprint-presenter-tests': (r'^TestAppFootprintPresenter',),
    'wizard-change-presentation-tests': (r'^TestAppChangePreview', r'^TestAppChangeResultPresenter'),
    'wizard-controller-tests': (r'^TestAppWizardController',),
    'wizard-app-new-tests': (r'^TestAppNewWizard', r'^TestGuidedStackProfile', r'^TestOrgDefault'),
    # Command regressions cover preview failures, SQL preferences and replacement intent.
    # Stable-preview regressions execute in the existing Edit family; shared
    # controller, New and machine-adapter families also verify coherent reads
    # and rejection of expired partial observations across source/read scopes.
    'wizard-app-edit-tests': (r'^TestAppEditApply', r'^TestAppEditCommand'),
    'wizard-footprint-command-tests': (r'^TestAppFootprintShow',),
    'wizard-machine-adapter-tests': (r'^TestAppMachineAdapters',),
    'ci-integration-operator-seam-tests': (r'^TestCIIntegrationOperator',),
    # core-921 reviewed production registration regressions; each family distinct
    # and must actually RUN/PASS (never a combined OR that can hide a miss).
    'wizard-core921-tests': (
        r'^TestCore921ActualCommandTreeHelpAndReadOnlyEntry$',
        r'^TestCore921MCPRealDiscoveryAndRejectIncompleteEdit$',
        r'^TestCore921MCPEditSchemaIsSnakeCaseAndPhaseConditional$',
        r'^TestCore921MCPProposeReachesHandlerWithZeroWrites$',
        r'^TestCore921MCPEditHandlerLevelNegatives$',
    ),
    'wizard-core918-native-sql-safety-tests': (
        r'^TestNativeSQLInventoryRejectsUnverifiedMaterialRegardlessOfKnownPath',
        r'^TestNativeSQLInventoryKnownFilesRequireActualInspection',
        r'^TestNativeSQLGeneratedProjectionRequiresProtectedAncestorAndSource',
        r'^TestNativeSQLProductionMaterialGateRejectsOversizedAndBrokenPublicSources',
    ),
    # Each required registration family is distinct and must actually RUN/PASS.
    # The MCP family exercises real in-process SDK discovery and invocation,
    # rather than treating a subprocess/stdio smoke as registration coverage.
    'wizard-core754-mcp-tests': (
        r'^TestExpectedPreconditionsShareTypedCLIMCPErrors$',
        r'^TestMCPReadEntryExpectedPreconditions$',
    ),
    'wizard-registration-tests': (
        r'^TestAppRegistrationCI',
        r'^TestCommandTreeCI',
        r'^TestMachineCoverageCI',
        r'^TestMachineDocumentationTracksRegistryAndCommandTree',
        r'^TestHumanCommandIndexTracksExactCommandMetadata',
        r'^TestMCPRegistrationCI',
    ),
}

WIZARD_TEST_TAGS = {
    'ci-integration-operator-seam-tests': ('baseharbor_ci_operator_testonly',),
}

OPENBAO_REQUIRED_TESTS = (
    'TestBootstrapKeepsRootAndUnsealSecretsOutOfCommandArguments',
    'TestBootstrapRejectsRecoveryFileInsideBaseHarborState',
    'TestBootstrapRejectsRecoveryDirectorySymlinkedIntoBaseHarborState',
    'TestLoadAdminCredentialsRejectsBroadPermissions',
    'TestInspectPrefersUnsealedHAMemberOverReachableSealedMember',
    'TestMemberStateAllowsBaoSealedExitHandlingWithoutShellErrexit',
    'TestRetryManagerProvisioningConvergesAfterTransientFailures',
    'TestUnsealConvergesAfterMemberRestartRace',
    'TestUnsealFailsFastWhenMemberNeverAnswers',
    'TestUnsealFailsImmediatelyOnPermanentNonTransientError',
    'TestManagerPolicyAllowsBothOpenBaoPolicyEndpoints',
    'TestManagerPolicyPermitsOnlyOwnCapabilityInspection',
    'TestProbeMemberStateCancellationDuringRetryBackoff',
    'TestProbeMemberStateInvalidJSONNoRetry',
    'TestIsTransientErrorRejectsIncidentalTemporary',
    'TestIsTransientErrorRejectsMixedPermanentTransport',
    'TestUnsealFailsOnPermanentPostUnsealVerification',
    'TestUnsealRecoversOnTransientPostUnsealVerification',
    'TestMemberPhaseSequenceIsEnforcedPerMember',
    'TestUnsealRejectsUnknownBAOAddrDestination',
    'TestPureTransportErrorStaysTransient',
)

VALIDATIONS = {'go-unit-tests', 'go-vet', 'go-build',
               'documentation-audit', 'mkdocs-en-strict', 'mkdocs-de-strict',
               'pages-candidate-build', 'source-contracts', 'cli-guided-tests',
               'cli-gofmt', 'cli-vet', 'cli-build', 'capability-package-tests',
                'openbao-package-tests', 'logs-package-tests', 'runtime-provider-package-tests',
                'pr-planner-tests', 'ci-policy', 'ci-actionlint',
                'ci-executor-tests', 'receipt-execution-tests',
                'inspect-gofmt', 'inspect-package-tests', 'inspect-vet',
               'inspect-consumer-tests', 'inspect-consumer-build',
               'authority-package-tests', 'delivery-package-tests', 'deployment-package-tests', 'broker-package-tests', 'cli-package-tests', 'identity-provider-package-tests', 'resolver-gofmt', 'resolver-package-tests', 'resolver-vet',
               'resolver-consumer-build',
                'pins-data-validation', 'demo-source-pin-validation'} | set(WIZARD_TEST_FAMILIES)

RECEIPT_EXECUTION_SUITES = ('test_receipt_writer', 'test_receipt_postrun')
CI_EXECUTION_SUITES = (
    'test_pr_diff_base', 'test_pr_required_final', 'test_pr_check_runner',
    'test_pr_runtime_request', 'test_pr_gate_dependencies',
    'test_pr_selected_runtime_matrix', 'test_ecosystem_release_pins',
    'test_targeted_gate_ref', 'test_artifact_gc', 'test_pre_release_check_plan',
    'test_ci_v0427_gate_catalog',
    'test_pre_release_integration', 'test_consumer_source_pins',
    'test_frozen_candidate_promotion', 'test_ci_check_routes',
    'test_pr_runtime_source', 'test_pr_runner_isolation', 'test_pr_ci_fallback',
    'test_pr_gate_coverage',
)


def go_package_tests(repository, packages, label, *, allow_skips=False, required_tests=()):
    """Keep Go tool diagnostics separate from its JSON test-event stream."""
    result = subprocess.run(['go', 'test', *packages, '-count=1', '-json'],
        cwd=Path(repository), text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    diagnostics = (result.stdout or '') + (result.stderr or '')
    if result.returncode:
        raise RuntimeError(label + ' package tests failed (exit ' +
                           str(result.returncode) + '): ' + diagnostics)
    try:
        events = [json.loads(line) for line in result.stdout.splitlines() if line.strip()]
    except ValueError as error:
        raise RuntimeError(label + ' package tests emitted invalid Go JSON: ' + diagnostics) from error
    if any(not isinstance(event, dict) or not isinstance(event.get('Action'), str)
           or ('Package' in event and not isinstance(event['Package'], str))
           or ('Test' in event and not isinstance(event['Test'], str)) for event in events):
        raise RuntimeError(label + ' package tests emitted invalid Go events: ' + diagnostics)
    if any(event['Action'] == 'fail' for event in events):
        raise RuntimeError(label + ' package tests reported failure: ' + diagnostics)
    for package in packages:
        suffix = '/' + package.removeprefix('./')
        rows = [event for event in events if event.get('Package', '').endswith(suffix)]
        ran = {event['Test'] for event in rows if event['Action'] == 'run' and event.get('Test')}
        passed = {event['Test'] for event in rows if event['Action'] == 'pass' and event.get('Test')}
        skipped = {event['Test'] for event in rows if event['Action'] == 'skip' and event.get('Test')}
        # Whole CLI/Logs/OpenBao packages include separate opt-in native tests.
        # Named required tests must always run and pass, including OpenBao's bootstrap checks.
        finished = passed | skipped if allow_skips else passed
        if (not ran or not passed or not ran <= finished or (skipped and not allow_skips)
                or not any(event['Action'] == 'pass' and not event.get('Test') for event in rows)):
            raise RuntimeError(label.lower() + ' package lacks passing test execution: ' + package +
                               '\n' + diagnostics)
        missing = set(required_tests) - (ran & passed)
        if missing:
            raise RuntimeError('required ' + label + ' tests missing PASS: ' +
                               ', '.join(sorted(missing)) + '\n' + diagnostics)
    return packages


def execute_validation(check, repository):
    """Execute a trusted profile check against the candidate checkout.

    Go's -run exits zero when no tests match. Discover first and require at
    least one matching test before executing the family.
    """
    if check not in WIZARD_TEST_FAMILIES and check not in ('openbao-package-tests', 'logs-package-tests', 'receipt-execution-tests', 'ci-executor-tests', 'authority-package-tests', 'delivery-package-tests', 'deployment-package-tests', 'broker-package-tests', 'cli-package-tests', 'identity-provider-package-tests'):
        raise ValueError('unsupported executable validation')
    if check == 'authority-package-tests':
        # Some slices precede stableid; test every present authority package.
        packages = ['./' + name for name in ('internal/coreinstallation',
                    'internal/stableid', 'internal/serviceaccess', 'internal/installauthority',
                    'internal/operatorauth')
                    if (Path(repository) / name).is_dir()]
        if not packages:
            raise RuntimeError('no authority packages found')
        return go_package_tests(repository, packages, 'Authority')
    if check in ('delivery-package-tests', 'deployment-package-tests', 'broker-package-tests'):
        name = 'runtimebroker' if check == 'broker-package-tests' else check.removesuffix('-package-tests')
        package = 'internal/' + name
        if not (Path(repository) / package).is_dir():
            raise RuntimeError(name + ' package missing')
        return go_package_tests(repository, ['./' + package], name.capitalize())
    if check == 'identity-provider-package-tests':
        return go_package_tests(
            repository, ['./internal/identityprovider'], 'Identity', allow_skips=True,
            required_tests=(
                'TestKeycloakSingleComposeHasNoHAResources',
                'TestKeycloakSinglePostgresStartsWithoutPrivilegedEntrypoint',
            ),
        )
    if check == 'cli-package-tests':
        # The client-commands card requires these five actual PASS events, not
        # merely a successful package exit or skipped test cases. Only enforce
        # this suite for candidates that include the dedicated regression file.
        client_fixture = Path(repository) / 'cmd/baha/client_commands_behaviors_test.go'
        required = (
            'TestClientCommandsTwoHomesSameDeploymentPrecedence',
            'TestClientCommandsDenialOverTwoHomes',
            'TestClientCommandsServerLossOverTwoHomes',
            'TestClientCommandsStaleConsentOverTwoHomes',
            'TestClientCommandsErrorParityOverTwoHomes',
        ) if client_fixture.is_file() else ()
        result = go_package_tests(repository, ['./cmd/baha'], 'CLI', allow_skips=True,
                                  required_tests=required)
        if required:
            print(json.dumps({'check': check, 'required_pass': list(required)}))
        return result
    if check == 'logs-package-tests':
        # Native opt-in scenarios run in their separately selected runtime jobs.
        return go_package_tests(repository, ['./internal/logs'], 'Logs', allow_skips=True)
    if check == 'openbao-package-tests':
        # Require the frozen bootstrap unit families; native opt-in acceptance
        # tests keep their separate runtime gates. Discover once and run once.
        repo = Path(repository)
        listed = subprocess.run(
            ['go', 'test', './internal/openbao', '-list', '.'],
            cwd=repo, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if listed.returncode:
            raise RuntimeError('OpenBao test discovery failed: ' + listed.stderr.strip())
        matches = [
            line.strip() for line in listed.stdout.splitlines()
            if re.fullmatch(r'Test[A-Za-z0-9_]+', line.strip())
        ]
        missing = set(OPENBAO_REQUIRED_TESTS) - set(matches)
        if missing:
            raise RuntimeError('missing required OpenBao tests: ' + ', '.join(sorted(missing)))
        # Include all new unit tests, while retaining the mandatory bootstrap checks.
        go_package_tests(repository, ['./internal/openbao'], 'OpenBao', allow_skips=True,
                         required_tests=OPENBAO_REQUIRED_TESTS)
        return list(OPENBAO_REQUIRED_TESTS)
    if check in ('receipt-execution-tests', 'ci-executor-tests'):
        # Execute the registered suites and reject missing or empty modules.
        repo = Path(repository)
        modules = RECEIPT_EXECUTION_SUITES if check == 'receipt-execution-tests' else CI_EXECUTION_SUITES
        label = 'receipt' if check == 'receipt-execution-tests' else 'CI'
        for module in modules:
            if not (repo / 'scripts' / (module + '.py')).is_file():
                raise RuntimeError('registered ' + label + ' suite missing: ' + module + '.py')
        env = dict(os.environ, PYTHONPATH=str(repo / 'scripts'))
        executed = subprocess.run(
            [sys.executable, '-m', 'unittest', '-v', *modules],
            cwd=repo, text=True, stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT, env=env)
        if executed.returncode:
            raise RuntimeError(label + ' suite execution failed: ' + executed.stdout)
        # unittest -v prints "<method> (module.Class.test_method) ... ok".
        # The count and per-module presence come from the executed output, not
        # from a hardcoded filename list.
        if not re.search(r'(?m)^Ran [1-9][0-9]* tests?', executed.stdout):
            raise RuntimeError(check + ': no ' + label + ' test executed')
        for module in modules:
            if not re.search(r'\(' + re.escape(module) + r'\.', executed.stdout):
                raise RuntimeError(label + ' suite did not execute: ' + module)
        return [module + '.py' for module in modules]
    patterns = WIZARD_TEST_FAMILIES.get(check)
    if patterns is None:
        raise ValueError('unsupported executable validation')
    if check == 'wizard-core918-native-sql-safety-tests':
        patterns = tuple(pattern + chr(36) for pattern in patterns)
    repo = Path(repository)
    all_matches = []
    tags = WIZARD_TEST_TAGS.get(check, ())
    build_flags = ['-tags=' + ','.join(tags)] if tags else []
    for pattern in patterns:
        listed = subprocess.run(
            ['go', 'test', *build_flags, './cmd/baha', '-list', pattern],
            cwd=repo, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if listed.returncode:
            raise RuntimeError('test discovery failed for ' + pattern + ': ' + listed.stderr.strip())
        matches = [
            line.strip() for line in listed.stdout.splitlines()
            if re.fullmatch(r'Test[A-Za-z0-9_]+', line.strip())
            and re.search(pattern, line.strip())
        ]
        if not matches:
            raise RuntimeError('zero matching tests for ' + check + ' (' + pattern + ')')
        # Run each mandatory family independently so a combined OR expression
        # cannot replace missing tests or silently skip a required family.
        executed = subprocess.run(
            ['go', 'test', *build_flags, './cmd/baha', '-run', pattern, '-count=1', '-v'],
            cwd=repo, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        if executed.returncode:
            raise RuntimeError('test execution failed for ' + check +
                                ' (' + pattern + '): ' + executed.stdout)
        for name in matches:
            if not re.search(r'(?m)^=== RUN   ' + re.escape(name) + r'(?:$|/)', executed.stdout):
                raise RuntimeError('required test did not execute: ' + name)
            if re.search(r'(?m)^--- SKIP: ' + re.escape(name) + r'(?:$|\s)', executed.stdout):
                raise RuntimeError('required test skipped: ' + name)
            if not re.search(r'(?m)^--- PASS: ' + re.escape(name) + r'(?:$|\s)', executed.stdout):
                raise RuntimeError('required test missing PASS: ' + name)
        all_matches.extend(matches)
    return all_matches

def matrix(plan):
    checks = plan.get('required_validation_checks')
    if not isinstance(checks, list):
        raise ValueError('missing validation checks')
    ids = [row['id'] for row in checks]
    if len(ids) != len(set(ids)) or any(i not in VALIDATIONS for i in ids):
        raise ValueError('unsupported validation check')
    # Matrix cannot be empty; an explicit inert marker is NOT a PASS. Carry the
    # mapping release on the demo-source-pin-validation row so the trusted runner
    # validates that release explicitly instead of recomputing the catalog
    # maximum. Every other row stays release-agnostic.
    include = []
    for row in checks:
        entry = {'check': row['id']}
        if row['id'] == 'demo-source-pin-validation' and 'release' in row:
            entry['release'] = row['release']
        include.append(entry)
    return {'include': include or [{'check': 'not-required'}]}


def validate_pins_data(check, repository):
    """Validate source metadata with trusted sibling contract owners, offline.

    The workflow extracts this runner and its two dependencies from the pinned
    target. Candidate modules cannot replace the canonical graph/origin owners.
    This proves metadata consistency, not the existence of native evidence.
    """
    trusted = str(Path(__file__).resolve().parent)
    sys.path.insert(0, trusted)
    try:
        from ecosystem_release_pins import resolve_pins, validate_ecosystem_graph
        from consumer_source_pins import REPOSITORIES, source_origin
    finally:
        sys.path.pop(0)
    source = Path(repository).resolve()
    releases = source / 'docs/releases'
    if releases.resolve() != releases:
        raise RuntimeError('consumer-pins directory must not be a symlink')
    pins_files = sorted(releases.glob('*.consumer-pins.json'))
    if not pins_files:
        raise RuntimeError('no consumer-pins metadata found under docs/releases')

    def unique_object(pairs):
        value = {}
        for key, item in pairs:
            if key in value:
                raise ValueError('duplicate consumer-pins key')
            value[key] = item
        return value

    validated = []
    for pins_path in pins_files:
        try:
            match = re.fullmatch(r'v(0\.4\.(?:23|24|25|26))\.consumer-pins\.json', pins_path.name)
            if not match or pins_path.is_symlink() or not pins_path.is_file():
                raise ValueError('unsupported consumer-pins metadata path')
            pins = json.loads(pins_path.read_text(), object_pairs_hook=unique_object)
            release = match.group(1)
            expected_keys = {'schema', 'gates', 'ecosystem'} if release == '0.4.26' else {'schema', 'gates'}
            gates = {'integration/docker/remote-target': {'connector'},
                     'integration/podman/remote-target': {'connector'},
                     'integration/static/live-console': {'connector', 'console'}}
            if (not isinstance(pins, dict) or set(pins) != expected_keys or
                    pins['schema'] != 'baseharbor.consumer-evidence-pins/v1' or
                    not isinstance(pins['gates'], dict) or set(pins['gates']) != set(gates)):
                raise ValueError('consumer-pins schema or required gates differ')
            commits = set()
            for gate, roles in gates.items():
                row = pins['gates'][gate]
                if not isinstance(row, dict) or set(row) != roles:
                    raise ValueError('consumer-pins gate roles differ')
                for role, pin in row.items():
                    source_origin(role, gate, pin)
                commits.add(row['connector']['commit'])
            if len(commits) != 1:
                raise ValueError('consumer-pins Connector commits diverge')
            joint = {'repository': REPOSITORIES['connector'], 'commit': next(iter(commits)),
                     'gate': 'integration/docker/remote-target'}
            for pin in pins['gates']['integration/static/live-console'].values():
                if 'origin' in pin and pin['origin'] != joint:
                    raise ValueError('consumer-pins joint origin differs')
            if release == '0.4.26':
                graph = pins['ecosystem']
                validate_ecosystem_graph(graph)
                candidate = {'schema': 'baseharbor.ecosystem-candidate/v1',
                             'release': release, **{role: graph[role]['commit']
                                                   for role in ('core', 'demo', 'console')}}
                console = {key: candidate[key] for key in ('schema', 'release', 'core', 'demo')}
                expected = resolve_pins(graph['core']['commit'], graph['connector']['commit'],
                                        candidate, console, graph['core']['commit'], release=release)
                if pins != expected:
                    raise ValueError('consumer-pins differs from canonical ecosystem resolution')
        except (OSError, ValueError, TypeError, KeyError) as error:
            raise RuntimeError('consumer-pins file differs: ' + str(pins_path) + ': ' + str(error)) from error
        validated.append(pins_path.name)
    return validated


def _first_line(stderr):
    """Return the first non-empty line of a subprocess transcript, or ''.

    Surfaces a transport's real error (e.g. GitHub's "No commit found for
    SHA" / HTTP 422 for an unreadable cross-repo public repository) inside a
    RuntimeError so a fail-closed pin-resolution failure is diagnosable instead
    of collapsing into one generic message. Never includes tokens, headers or
    secrets - only the status line or first text line.
    """
    if not stderr:
        return ''
    for line in stderr.splitlines():
        if line.strip():
            return line.strip()
    return ''


def _semantic_version(tag):
    """Parse a dotted ``vX.Y.Z`` release tag into a comparable tuple.

    The parser tolerates an optional leading ``v`` and any number of trailing
    components, and treats a missing component as zero, so ``v0.4.28`` sorts
    above ``v0.4.27`` while ``v0.4`` sorts above ``v0.3.99``. Anything that is
    not a dotted numeric tag is rejected rather than silently compared as a
    string, so an unexpected release label fails the gate loudly.
    """
    if not isinstance(tag, str):
        raise ValueError('release tag must be a string: ' + repr(tag))
    body = tag[1:] if tag.startswith('v') else tag
    parts = body.split('.')
    if not parts or any(not chunk.isdigit() for chunk in parts):
        raise ValueError('release tag must be dotted numeric: ' + repr(tag))
    return tuple(int(chunk) for chunk in parts)


def _current_release(repository):
    """Return the active release tag from the trusted release-requirements.json.

    The demo source pin is release-scoped, so the validator must read the pin
    for whatever release the candidate actually targets, not a hard-coded
    v0.4.27. The supported release set lives in ``releases[]``; the active tag
    is the highest tag in that list by semantic version, derived
    deterministically so a v0.4.28 catalog always selects v0.4.28. A missing or
    malformed requirements file is a hard failure: the validator must never
    silently guess a path, and a catalog with no selectable tag is a hard
    failure.
    """
    try:
        requirements = json.loads(
            (Path(repository) / 'scripts' / 'release-requirements.json').read_text())
    except (OSError, ValueError, TypeError):
        raise RuntimeError('release-requirements.json unreadable or malformed')
    releases = requirements.get('releases')
    if not isinstance(releases, list) or not releases:
        raise RuntimeError('release-requirements.json has no release set')
    valid_tags = [tag for tag in releases if isinstance(tag, str)]
    if not valid_tags:
        raise RuntimeError('release-requirements.json has no release tag')
    try:
        release = max(valid_tags, key=_semantic_version)
    except ValueError as error:
        raise RuntimeError('release-requirements.json has an unreadable '
                           'release tag: ' + str(error))
    if release not in releases:
        raise RuntimeError('release-requirements.json active tag missing from '
                           'release set: ' + repr(release))
    return release


def validate_demo_source_pin(check, repository, release=None):
    """Require the active-release Demo pin to be one existing immutable commit.

    The Demo source pin is release-scoped: the pinned SHA lives at
    ``docs/releases/<release>.demo-ref`` where ``<release>`` defaults to the
    highest supported tag in the trusted ``release-requirements.json`` catalog.
    The trusted planner carries an explicit release on the matrix row for this
    one check; that mapping release wins and the catalog maximum is only a
    fallback for callers without an explicit release. A missing or malformed
    requirements file is a hard failure: the validator must never silently guess
    a path.
    """
    if check != 'demo-source-pin-validation':
        raise ValueError('unsupported Demo pin validation')
    if release is None:
        release = _current_release(repository)
    pin = Path(repository) / 'docs' / 'releases' / '{}.demo-ref'.format(release)
    if pin.is_symlink() or not pin.is_file():
        raise RuntimeError('{} Demo source pin is missing or unsafe'.format(release))
    raw = pin.read_text()
    if not re.fullmatch(r'[0-9a-f]{40}\n?', raw):
        raise RuntimeError('{} Demo source pin must contain exactly one full lowercase commit SHA'.format(release))
    sha = raw.strip()
    # Resolve the pin against the pinned Demo repository OUTSIDE this checkout.
    # The scratch demo repo must never be created, cleaned or deleted inside
    # `repository`: a checkout-local `_demo_repo` can be destroyed by
    # workflow-side cleanup (git clean -fdx) or, worse, can delete other
    # checkout content. A unique temp directory under the runner temp keeps the
    # checkout byte-for-byte unchanged. The CI job runs with
    # GH_TOKEN=${{ github.token }}, which cannot read a cross-repo public
    # repository, and GitHub answers 422/404 "No commit found for SHA" for a
    # repo a token cannot see - indistinguishable from a missing commit. Resolve
    # anonymously over https so the check works for a public repository without
    # cross-repo token rights. Fail-closed: any non-zero exit, non-commit object
    # or SHA mismatch raises, and the transport stderr / HTTP status is surfaced
    # in the message (no secrets: only the status code and first line) so the
    # failure is diagnosable instead of collapsing into one generic RuntimeError.
    scratch = tempfile.mkdtemp(prefix='baseharbor-demo-pin-')
    try:
        fetched = subprocess.run(
            ['git', 'clone', '--depth=1', '--filter=blob:none',
             '--no-checkout',
             'https://github.com/mcpdev80/baseharbor-demo', scratch],
            text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if fetched.returncode:
            raise RuntimeError(
                '{} Demo source pin verification failed: git clone error {}'
                .format(release, _first_line(fetched.stderr)))
        obj_type = subprocess.run(
            ['git', '-C', scratch, 'cat-file', '-t', 'origin/HEAD'],
            text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if obj_type.returncode or obj_type.stdout.strip() != 'commit':
            raise RuntimeError(
                '{} Demo source pin is not a commit in '
                'mcpdev80/baseharbor-demo: {}'
                .format(release, _first_line(obj_type.stderr)))
        # Require the fetched object SHA to equal the pinned SHA exactly.
        sha_only = subprocess.run(
            ['git', '-C', scratch, 'rev-parse', 'origin/HEAD'],
            text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if sha_only.returncode or sha_only.stdout.strip() != sha:
            raise RuntimeError(
                '{} Demo source pin does not resolve to the pinned '
                'SHA {}'.format(release, sha))
    finally:
        subprocess.run(['rm', '-rf', scratch],
                       text=True, stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL)
    return [sha]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['matrix', 'execute'])
    parser.add_argument('--plan', type=Path)
    parser.add_argument('--check')
    parser.add_argument('--release', default=None)
    parser.add_argument('--repository', default='.')
    args = parser.parse_args()
    if args.mode == 'execute':
        if not args.check:
            parser.error('--check is required for execute')
        if args.check == 'pins-data-validation':
            matched = validate_pins_data(args.check, args.repository)
            print(json.dumps({'check': args.check, 'matched_tests': matched}, sort_keys=True))
            return
        if args.check == 'demo-source-pin-validation':
            matched = validate_demo_source_pin(args.check, args.repository, args.release)
            print(json.dumps({'check': args.check, 'matched_tests': matched}, sort_keys=True))
            return
        matches = execute_validation(args.check, args.repository)
        print(json.dumps({'check': args.check, 'matched_tests': matches}, sort_keys=True))
        return
    if not args.plan:
        parser.error('--plan is required for matrix/results')
    plan = json.loads(args.plan.read_text())
    value = matrix(plan)
    print(json.dumps(value, sort_keys=True, separators=(',', ':')))


if __name__ == '__main__':
    main()
