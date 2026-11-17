# Normal CI check routing audit

This source-policy snapshot covers the complete current check registry and tracked
Go/CI/script paths at target `9a57107f`. The executable planner, runner and workflow
remain the authorities. This document is not a test-result manifest or a merge gate.

## Checked boundaries

- All normal check IDs agree between planner declarations, the runner registry and
  the actual outer `VALIDATION` shell cases. Unknown check IDs still fail; an
  otherwise valid new source path receives normal conservative checks.
- Every required named Wizard family and every literal Guided, Inspect/Reconcile,
  provider/doctor and Core-integration Go filter matches real test functions.
  A Linux `go test ./cmd/baha -list .` discovery also found every Wizard family.
- Every PR-selectable native/contract gate has matrix dispatch, consumer dependency
  classification and final dependency-result handling. Required skipped jobs fail.
- All 65 checks in the current release catalog have normal release executors and
  declared consumer dependencies. This does not run a full release matrix.
- Every tracked Go file in cmd/internal/contracts/spec, CI workflow and Python/shell/
  JSON/Markdown helper is projected individually through both the targeted policy
  and the production conservative fallback. The production entrypoint enables
  fallback automatically; actual PR source identity and complete diffs remain
  independently checked.

## Corrections in this change

- Go JSON profiles separate stdout test events from stderr tool diagnostics and
  preserve failure diagnostics instead of crashing with JSONDecodeError.
- Direct Authority package files and Deployment files receive actual package checks;
  production Deployment changes also select their real consumer/native lifecycles.
- Shared Runtime package tests include nested packages, and Broker changes receive
  their own package tests. A native Demo lifecycle alone does not execute a changed
  Runtime/Broker unit test. Mixed CLI test changes execute the CLI package.
- Changed restart fixtures select the existing control-plane-recovery checks and run
  both required opt-in tests. A changed native backup fixture selects the existing
  backup-restore checks and runs its CLI disaster-recovery test as a bounded follow-up.
- Native follow-ups accept an older event base only when Git proves the selected
  effective base, authenticated PR head, candidate parents and target ancestry.
  No Actions run/job API or artifact interpretation is involved.
- Valid source outside a narrow mapping receives normal full Go unit tests, vet
  and build. Product/runtime-area changes also run Docker/Podman lifecycle checks.
  This is not the full release matrix; unknown metadata does not boot runtimes.
  Invalid diffs, source identities, paths and missing catalog executors still fail.

## Normal check index

The registry contains **51 checks**. Exact command and test-family details live in `scripts/pr-gate-plan.py`, `scripts/pr-check-runner.py` and `.github/workflows/ci.yml`.

| Check ID | Declared normal execution |
| --- | --- |
| `authority-package-tests` | `python3 scripts/pr-check-runner.py execute --check authority-package-tests --repository .` |
| `broker-package-tests` | `python3 scripts/pr-check-runner.py execute --check broker-package-tests --repository .` |
| `capability-package-tests` | `go test ./internal/capability/... -count=1` |
| `ci-actionlint` | `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck=` |
| `ci-executor-tests` | `python3 scripts/pr-check-runner.py execute --check ci-executor-tests --repository .` |
| `ci-policy` | `bash scripts/ci-workflow-policy.sh` |
| `cli-build` | `go build ./cmd/baha` |
| `cli-gofmt` | `test -z "$(gofmt -l cmd/baha)"` |
| `cli-guided-tests` | `go test ./cmd/baha -run '^TestGuided' -count=1` |
| `cli-package-tests` | `go test ./cmd/baha -count=1` |
| `cli-vet` | `go vet ./cmd/baha` |
| `delivery-package-tests` | `python3 scripts/pr-check-runner.py execute --check delivery-package-tests --repository .` |
| `deployment-package-tests` | `python3 scripts/pr-check-runner.py execute --check deployment-package-tests --repository .` |
| `documentation-audit` | `bash scripts/documentation-audit.sh` |
| `go-build` | `go build ./...` |
| `go-unit-tests` | `go test ./... -count=1` |
| `go-vet` | `go vet ./...` |
| `inspect-consumer-build` | `go build -o "$RUNNER_TEMP/baha-inspect" ./cmd/baha` |
| `inspect-consumer-tests` | `go test ./cmd/baha -run 'Inspect\|Reconcile' -count=1` |
| `inspect-gofmt` | `test -z "$(gofmt -l internal/repositoryinspect)"` |
| `inspect-package-tests` | `go test ./internal/repositoryinspect/ -count=1` |
| `inspect-vet` | `go vet ./internal/repositoryinspect/` |
| `logs-package-tests` | `python3 scripts/pr-check-runner.py execute --check logs-package-tests --repository .` |
| `mkdocs-de-strict` | `mkdocs build --strict --config-file mkdocs.de.yml` |
| `mkdocs-en-strict` | `mkdocs build --strict --config-file mkdocs.yml` |
| `openbao-package-tests` | `python3 scripts/pr-check-runner.py execute --check openbao-package-tests --repository .` |
| `pages-candidate-build` | `build candidate documentation without deploying Pages` |
| `pins-data-validation` | `python3 scripts/pr-check-runner.py execute --check pins-data-validation --repository .` |
| `pr-planner-tests` | `python3 -m unittest scripts/test_pr_gate_plan.py` |
| `receipt-execution-tests` | `PYTHONPATH=scripts python3 -m unittest scripts/test_receipt_writer.py scripts/test_receipt_postrun.py` |
| `resolver-consumer-build` | `go build -o "$RUNNER_TEMP/baha-resolver" ./cmd/baha` |
| `resolver-gofmt` | `test -z "$(gofmt -l internal/runtime/resolver)"` |
| `resolver-package-tests` | `go test ./internal/runtime/resolver/ -count=1` |
| `resolver-vet` | `go vet ./internal/runtime/resolver/` |
| `runtime-provider-package-tests` | `go test ./internal/runtime/... ./internal/health ./internal/providers/runtime/podman -count=1` |
| `source-contracts` | `go test ./internal/application/... ./contracts/...` |
| `wizard-app-edit-load-tests` | `trusted runner: required named CLI test families` |
| `wizard-app-edit-tests` | `trusted runner: required named CLI test families` |
| `wizard-app-new-tests` | `trusted runner: required named CLI test families` |
| `wizard-capability-selection-tests` | `trusted runner: required named CLI test families` |
| `wizard-change-presentation-tests` | `trusted runner: required named CLI test families` |
| `wizard-controller-tests` | `trusted runner: required named CLI test families` |
| `wizard-core754-mcp-tests` | `trusted runner: required named CLI test families` |
| `wizard-core918-native-sql-safety-tests` | `trusted runner: required named CLI test families` |
| `wizard-core921-tests` | `trusted runner: required named CLI test families` |
| `wizard-footprint-command-tests` | `trusted runner: required named CLI test families` |
| `wizard-footprint-presenter-tests` | `trusted runner: required named CLI test families` |
| `wizard-machine-adapter-tests` | `trusted runner: required named CLI test families` |
| `wizard-provider-choice-tests` | `trusted runner: required named CLI test families` |
| `wizard-registration-tests` | `trusted runner: required named CLI test families` |
| `wizard-runtime-retirement-tests` | `trusted runner: required named CLI test families` |

## PR cadence

Draft updates run workflow policy plus source formatting/readability only. Marking
a PR ready for review starts the normal selected checks; updates to a ready PR run
the required selection again for the new source. Converting back to Draft stops
expensive PR selection. Ready-PR completion requires source format and every other
required job to succeed. The automatic develop-push duplicate has been removed;
explicit validation branches and manual workflow dispatch retain their own checks.
No completed-run lookup or evidence reuse is used to avoid duplicate work.

## Complete tracked-path projection

The production planner first selects known targeted checks. If a valid source change
has no narrow mapping, it selects `go-unit-tests`, `go-vet` and `go-build`; product
or runtime scopes additionally select both lifecycle checks. Invalid or ambiguous
source/diff input and absent catalog executors still block. A broad selection is
not a waiver: the actual ordinary checks must run and pass.

All **1,992 baseline tracked paths** are covered by this production selection:
**699 targeted routes** and **1,293 conservative fallback routes**. The default
internal diagnostic API can still expose the targeted-only gap; the production
CLI explicitly enables fallback. These are source-routing projections, not claims
that a candidate's tests passed.

The table lists every package/directory requiring fallback for at least one path.
These paths are not blocked merely because a narrow mapping is absent; their
normal broad Go checks and applicable lifecycle tests provide conservative coverage.

| Scope | Targeted paths | Normal fallback paths |
| --- | ---: | ---: |
| `cmd/baha` | 344 | 327 |
| `internal/apierror` | 0 | 2 |
| `internal/application` | 26 | 247 |
| `internal/applicationbackup` | 0 | 20 |
| `internal/applicationinput` | 0 | 2 |
| `internal/applicationlifecycle` | 0 | 1 |
| `internal/applicationruntimeapi` | 0 | 2 |
| `internal/applicationruntimeauth` | 0 | 3 |
| `internal/applicationsecret` | 0 | 4 |
| `internal/applicationsecretapi` | 0 | 4 |
| `internal/auth` | 0 | 6 |
| `internal/authorization` | 0 | 2 |
| `internal/availability` | 0 | 4 |
| `internal/capability` | 5 | 23 |
| `internal/cli` | 0 | 8 |
| `internal/connectivityrelay` | 0 | 4 |
| `internal/controlplaneapi` | 0 | 1 |
| `internal/controlplaneruntime` | 0 | 7 |
| `internal/corebackup/etcd` | 0 | 8 |
| `internal/coreupdate` | 0 | 55 |
| `internal/credential` | 0 | 8 |
| `internal/credential/openbao` | 0 | 3 |
| `internal/database` | 0 | 15 |
| `internal/dcspki` | 0 | 2 |
| `internal/devaccess` | 0 | 6 |
| `internal/development` | 0 | 25 |
| `internal/development/goadapter` | 0 | 3 |
| `internal/development/nextjsadapter` | 0 | 2 |
| `internal/development/pythonadapter` | 0 | 2 |
| `internal/development/quarkusadapter` | 0 | 2 |
| `internal/devgateway` | 0 | 7 |
| `internal/endpoint` | 0 | 2 |
| `internal/evidence` | 0 | 4 |
| `internal/exposure` | 0 | 5 |
| `internal/extension` | 0 | 8 |
| `internal/externalprovider` | 0 | 24 |
| `internal/health` | 1 | 1 |
| `internal/hostresource` | 0 | 5 |
| `internal/hosttrust` | 0 | 4 |
| `internal/httpsecurity` | 0 | 2 |
| `internal/identity` | 0 | 2 |
| `internal/identityprovider` | 0 | 66 |
| `internal/logs` | 2 | 22 |
| `internal/machine` | 0 | 13 |
| `internal/machinehttp` | 0 | 14 |
| `internal/machinereadmodels` | 0 | 1 |
| `internal/metrics` | 0 | 21 |
| `internal/objectstorage` | 0 | 32 |
| `internal/observability` | 0 | 2 |
| `internal/openbao` | 4 | 52 |
| `internal/operatorauth` | 0 | 9 |
| `internal/orgconfig` | 0 | 7 |
| `internal/policy` | 0 | 2 |
| `internal/preflight` | 0 | 2 |
| `internal/provider/authoring` | 0 | 3 |
| `internal/provider/builtin` | 0 | 4 |
| `internal/providerbinding` | 0 | 7 |
| `internal/providerconformance` | 0 | 5 |
| `internal/provideroperation` | 0 | 1 |
| `internal/providers/runtime/docker` | 0 | 1 |
| `internal/providertopology` | 0 | 6 |
| `internal/providerui` | 0 | 1 |
| `internal/providerupgrade` | 0 | 3 |
| `internal/providerupgrade/keycloak` | 0 | 4 |
| `internal/providerupgrade/openbao` | 0 | 5 |
| `internal/reconciliation` | 0 | 2 |
| `internal/repositoryinspect` | 4 | 29 |
| `internal/runtimeapidocs` | 0 | 2 |
| `internal/runtimeexecutor` | 0 | 5 |
| `internal/runtimeexplorer` | 0 | 20 |
| `internal/runtimeobservability` | 0 | 2 |
| `internal/runtimeoperation` | 0 | 2 |
| `internal/runtimeresourceapi` | 0 | 3 |
| `internal/targetaccess` | 0 | 2 |
| `internal/targetenrollment` | 0 | 11 |
| `internal/targetsession` | 0 | 35 |
| `internal/telemetry` | 0 | 17 |
| `internal/tenancy` | 0 | 2 |
| `internal/testsupport/containersecurity` | 0 | 2 |
| `internal/testsupport/runtimeprovider` | 0 | 1 |
| `internal/traces` | 0 | 12 |
| `spec/runtime-api/v1` | 0 | 1 |

## Current open PR source projections

The complete clean merge diff is resolved against target `9a57107f`, rather than
assuming a PR title describes its entire source set. These are routing results,
not successful workflow claims. Product owners may push newer source afterwards.

| PR source | Changed files | Routing result | Selected native checks |
| --- | ---: | --- | --- |
| #1348 `8f7ad334` | 29 | resolved | `atomic/docker/lifecycle`, `atomic/podman/lifecycle` |
| #1347 `59bcb5b5` | 26 | resolved | `atomic/docker/backup-restore`, `atomic/docker/lifecycle`, `atomic/podman/backup-restore`, `atomic/podman/lifecycle` |
| #1334 `df9aacbe` | 7 | resolved | none |
| #1333 `6a335b4b` | 10 | resolved | `atomic/docker/lifecycle`, `atomic/podman/lifecycle` |

All selected package/consumer checks must still run and pass. Actual product
assertion failures remain product work; a routing correction cannot waive them.

## Remaining product and policy boundaries

State Owner application/native failures are product integration failures; assigning
package tests must not make them disappear. Package discovery, missing tests,
malformed output, failed tests and required skipped tests remain blocking.
Historical candidate-specific rules remain narrow optimizations. Valid rename,
delete, nested-package and source/security changes outside them receive normal
conservative checks; malformed source inputs never receive a success exemption.

`test_ci_check_routes.py` checks the cross-layer registry, named source filters,
selected-gate dispatch/final handling and complete release-catalog classification.
`test_pr_ci_fallback.py` verifies the production fallback entrypoint, invalid-source
rejection and the Draft/ready workflow cadence without a duplicate develop push.
`test_pr_runtime_source.py` exercises the native source guard with real temporary
Git histories, including advanced bases and rejected foreign/stale identities.
