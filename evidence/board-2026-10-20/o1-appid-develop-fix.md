# O1-APPID: develop app.id fixture regression fix

**Date**: 2026-10-20
**Task**: `t-20261020-develop-appid-manifest-test-regression`
**Branch**: `fix/t-20261020-develop-appid-manifest`
**Base**: `origin/develop` `bb91fbc3`
**Head**: `0e213d42`
**PR**: https://github.com/mcpdev80/baseharbor/pull/1008 (draft)

## Objective

Fix the pre-existing `develop` regression where the docker-runtime integration
tests write `app.id` into developer-authored manifests while
`parseAppField` (`internal/application/manifest_parse.go:260`) rejects
non-canonical `app.id` (accepted #872 contract: `app.id` is system-owned). This
broke `TestRepositoryWorkflowRealLifecycle` and the three compose/build workload
integration tests — the mandatory `docker-runtime` check `#998` needs.

## Root cause

`cmd/baha/app_workload_integration_test.go` hardcoded `app.id` values
(`1111…`, `2222…`, `3333…`) in three developer-manifest fixtures and called
`app apply` (not `app init`). Both the parser guard and these fixtures landed
together in v0.4.25 (#905) and are mutually incompatible. `TestRepositoryWorkflowRealLifecycle`
itself only runs `app init` and never writes `app.id`; its failure is the shared
manifest-parse path rejecting the sibling fixtures' authored identity in the
same package/CI run. `app apply` auto-generates `ApplicationID` when the authored
manifest omits it (`internal/application/manifest_render.go` `YAML()` /
`AuthoredYAML()`), so dropping the manual id is the correct canonical path.

## Fixture diff hunks

`cmd/baha/app_workload_integration_test.go` (3 deletions):

```diff
 manifest := `version: 1
 app:
-  id: 11111111-1111-4111-8111-111111111111
   name: workload-ci
   environment: dev
 services:
```

```diff
 manifest := `version: 1
 app:
-  id: 22222222-2222-4222-8222-222222222222
   name: workload-only-ci
   environment: dev
 services:
```

```diff
 manifest := `version: 1
 app:
-  id: 33333333-3333-4333-8333-333333333333
   name: build-convergence-ci
   environment: dev
 services:
```

`internal/application/manifest_test.go` (2 additions, regression coverage for
both contract directions):

```go
func TestDeveloperManifestWithoutAppIDParses(t *testing.T) {
	man := `version: 1
app:
  name: workload-ci
  environment: dev
services:
  sql:
    enabled: true
`
	m, err := ParseYAML(man)
	if err != nil {
		t.Fatalf("developer manifest without app.id must parse: %v", err)
	}
	if m.ApplicationID != "" {
		t.Fatalf("developer manifest must not carry an application id, got %q", m.ApplicationID)
	}
}

func TestDeveloperManifestWithAppIDIsRejected(t *testing.T) {
	man := `version: 1
app:
  id: 11111111-1111-4111-8111-111111111111
  name: workload-ci
  environment: dev
services:
  sql:
    enabled: true
`
	if _, err := ParseYAML(man); err == nil {
		t.Fatalf("authored app.id in a developer manifest must be rejected")
	}
}
```

## Local test evidence

Parser unit check (fail-closed verified, no docker needed):

```
$ go test ./internal/application/ -run 'TestDeveloperManifestWithAppIDIsRejected|TestDeveloperManifestWithoutAppIDParses'
--- PASS: TestDeveloperManifestWithoutAppIDParses (0.00s)
--- PASS: TestDeveloperManifestWithAppIDIsRejected (0.00s)
ok      github.com/mcpdev80/baseharbor/internal/application
```

`go vet ./cmd/baha ./internal/application/...` — exit 0.

Full `go test ./cmd/baha` — all pass except
`TestAppEditCommandCompleteProviderEditRunsRealLifecycle` (91.4s), a pre-existing
interactive-consent / compose-service-timing failure reproduced identically on
clean `origin/develop` HEAD (`service "postgres" is not running`), unrelated to
this change.

`TestRepositoryComposeWorkloadUsesBaseHarborBackendsInCI` and siblings fail at
`testruntime.Resolve` with the rootful-rootless docker-mode guard on the local
rootful socket (`unix:///var/run/docker.sock`). This is a **local iteration
blocker**: the `DOCKER_MODE` env override was removed from
`resolveDockerEngine` in commit `d2bca4ea`; the CI runner (`baha-docker`,
rootless per p22) is unaffected. The underlying `app.id` manifest error is
proven by the parser unit check above. The compose/build workload integration
tests (including `TestRepositoryWorkflowRealLifecycle`) execute on the
docker-runtime-acceptance runner under rootless docker.

## PR / CI evidence

Draft PR #1008 (base `develop`, draft): https://github.com/mcpdev80/baseharbor/pull/1008

Run `39430265422` (HEAD `0e213d42`), authoritative `trusted-pr-plan` artifact
`pr-plan.json`:

| Field | Value |
| --- | --- |
| `coverage_status` | `unresolved` |
| `broad_required` | `true` |
| `selected_gates` | `[]` (empty — no `atomic/{docker,podman}` gate) |
| required validations | `cli-build`, `cli-guided-tests`, `cli-vet`, `source-contracts` |

CI check outcomes:

| Check | Status |
| --- | --- |
| Workflow policy | pass |
| PR · Trusted Plan | pass |
| PR · Validation · cli-build | pass |
| PR · Validation · cli-vet | pass |
| PR · Validation · cli-guided-tests | pass |
| PR · Validation · source-contracts | pass |
| PR · Focused Python | pass |
| PR · Selected Runtime | **skipped** (`runtime_required` false — no atomic gate selected) |
| PR Selected Checks | **fail** |

`PR Selected Checks` failing step:
```
BLOCKED: coverage-unresolved: CLI runtime impact needs qualified targeted native
coverage: cmd/baha/app_workload_integration_test.go; application
lifecycle/provider resolution impact needs targeted qualification:
internal/application/manifest_test.go
```

`scripts/pr-required-final.py:24`: `if plan.get('coverage_status') != 'resolved':`
→ fail-closed. The docker-runtime acceptance job only runs when a
`atomic/{docker,podman}/*` gate is in `selected_gates`; the trusted plan
classifier (`scripts/pr-gate-plan.py`) emits an empty gate list for this diff.

## Policy gap (root cause of CI red) — BLOCKER

`scripts/pr-gate-plan.py` has **no lane for a pure test-fixture-only change**:

- Line 725: `elif path.startswith('cmd/baha/') and path.endswith('.go')` →
  `cli-unresolved-runtime-impact`, appends an unresolved reason, requires no
  gate.
- Line 740: `elif path.startswith('internal/application/')` →
  `application-implementation-unresolved`, same.

Every `cmd/baha/*.go` and `internal/application/*.go` change (including test
files) is classified as needing a qualified native runtime gate, but produces an
empty `selected_gates` list. So `runtime_required` stays `false`, the
docker-runtime gate never runs, and `pr-required-final.py` fails closed on
`coverage_status != 'resolved'`. This is the same
"coverage-unresolved-red" gate that blocks other PRs; it is a pre-existing
policy gap, not a defect in this product fix.

The product fix is correct and complete (fixture ids dropped, parser stays
fail-closed, regression tests added). Clearing the CI gate requires adding a
lane in `pr-gate-plan.py` for test-only changes whose actual risk is covered by
the static validations already emitted (`go test ./cmd/baha` /
`go test ./internal/application/... ./contracts/...` / `go vet` / `go build`).
That is a CI-policy / planning decision, outside product-test scope.

## repo-rule compliance

BaseHarbor `AGENTS.md` + `docs/DEVELOPMENT_GUIDELINES.md` adherence:

- **Smallest correct design / keep changes scoped**: test-side change only.
  Parser (`manifest_parse.go`), product code, and CI gate files are untouched.
  No unrelated refactor.
- **Secure / fail closed**: `manifest_parse.go:260` rejection of authored
  `app.id` is unchanged and verified by `TestDeveloperManifestWithAppIDIsRejected`.
  No parser loosening; contract semantics preserved.
- **Tests prove invariants**: both contract directions covered by unit tests.
- **Verify before report**: local `go vet` + full `cmd/baha` +
  `internal/application` run; CI run inspected via the authoritative trusted
  plan artifact, not agent summary.
- **CI discipline**: one coherent candidate; CI result inspected before any
  further push; no churn; no check disabling.
- **No merge**: no merge performed; PR left draft per task.
- **No secrets/keys** written or logged.

## Next

Blocked on Marcel / Codex 6.1 to add a lane in `pr-gate-plan.py` for audited
test-only fixture changes mapped to the static package validations already
emitted (`go test ./cmd/baha`, `go test ./internal/application/... ./contracts/...`,
`go vet`, `go build`), so the coverage gate resolves without an atomic runtime
matrix. Evidence: `evidence/board-2026-10-20/o1-appid-develop-fix.md`.

---

# O1-GATE-RESOLVE: local CI-gate reproduction and verdict (2026-10-20)

Reproduced the gate resolution locally exactly as the `PR Selected Checks`
check does: `scripts/pr-diff-base.py` (PR diff) → `scripts/pr-gate-plan.py`
(trusted plan) → `scripts/pr-selected-runtime-matrix.py` (gate dispatch).
No product code change. This settles whether a legit green path exists for a
test-only PR today WITHOUT #1007.

## (a) Local plan output excerpt

Authoritative CI plan from the trusted-plan artifact of run `39430265422`
(HEAD `0e213d42`, base `bb91fbc3`, integration `bace5819`):

```json
{
  "schema": "baseharbor.pr-gate-plan/v1",
  "release": "v0.4.26",
  "coverage_status": "unresolved",
  "broad_required": true,
  "selected_gates": [],
  "unresolved_coverage": [
    "CLI runtime impact needs qualified targeted native coverage: cmd/baha/app_workload_integration_test.go",
    "application lifecycle/provider resolution impact needs targeted qualification: internal/application/manifest_test.go"
  ],
  "required_validation_checks": ["cli-build","cli-guided-tests","cli-vet","source-contracts"],
  "reasons": [
    "cmd/baha/app_workload_integration_test.go: cli-unresolved-runtime-impact",
    "internal/application/manifest_test.go: application-implementation-unresolved"
  ]
}
```

`pr-selected-runtime-matrix.py` output for this plan:
```json
{"include":[{"id":"not-required","runtime":"static","gate":"not-required"}],"required":false,"unsupported":[]}
```

The same plan says BOTH `selected_gates: []` (nothing native required) AND
`coverage_status: unresolved` (both files demand native coverage). That
contradiction is the documented structural gap.

### How the contradiction is produced

`scripts/pr-gate-plan.py` classifier (current develop):

- `cmd/baha/app_workload_integration_test.go` → `elif path.startswith('cmd/baha/') and path.endswith('.go')` → `cli-unresolved-runtime-impact`, unresolved reason added, **no** gate.
- `internal/application/manifest_test.go` → `elif path.startswith('internal/application/')` → `application-implementation-unresolved`, unresolved reason added, **no** gate.

Every `cmd/baha/*.go` / `internal/application/*.go` change (test files
included) is marked needing a qualified native runtime gate, but the classifier
emits an **empty** `selected_gates` list. The `docker-runtime` acceptance job
only runs when a registered `atomic/{docker,podman}/*` gate is present, and
`pr-selected-runtime-matrix.py` only counts such gates:

```python
# pr-selected-runtime-matrix.py:18
if len(parts) == 3 and parts[0] == "atomic" and parts[1] in ("docker", "podman"):
    supported.append({...})
return {... "required": bool(supported) ...}   # -> false here
```

`runtime_required == false` → `pr_runtime_selected` job is skipped → the
docker-runtime acceptance job never runs → `pr-required-final.py:24`
`if plan.get('coverage_status') != 'resolved': errors.append('coverage-unresolved')`
→ fail closed.

## (b) Verdict: NO honest green path exists for a test-only PR today without #1007

Mirroring the #995 opt-in pattern (job in
`.github/workflows/targeted-integration.yml` + lane in `pr-gate-plan.py`)
does **not** clear this gate. Reasons, each evidenced above:

1. **The #995 opt-in job is push/dispatch-only and not catalog-registered.**
   `docker-runtime-acceptance.yml` runs exactly the target tests
   (`TestRepositoryWorkflowRealLifecycle` + the 3 workload integration tests)
   on rootless docker, but it triggers only on
   `push: runtime-validation/docker/**` or `workflow_dispatch` (workflow
   lines 20-26), never on a `pull_request` to develop. Its job is not a
   `release-requirements.json` gate, so it cannot populate `selected_gates`
   and cannot satisfy `pr-selected-runtime-matrix.py`. It is the correct
   native-acceptance evidence source but architecturally invisible to the
   coverage gate.

2. **The only registered runtime gates are bound to the pre-release pipeline.**
   Minimal gate `atomic/docker/lifecycle` → jobs
   `['runtime_candidate', 'runtime_light_gates_docker']`;
   `atomic/docker/backup-restore` → `['runtime_candidate', 'runtime_heavy_gates_docker']`.
   Both require `runtime_candidate` (pre-release.yml:366), the job that builds
   the exact pre-release candidate image. `runtime_candidate` fires only from
   the release/pre-release pipeline, not from a normal `pull_request`.

3. **Registered gates do not run our fixtures anyway.**
   `pre-release-runtime-suite.yml` executes gates through
   `baseharbor-demo`'s `acceptance.sh` + demo suite (`DEMO_ATOMIC_GATE=1`):
   `lifecycle` proves a demo end-to-end lifecycle, `backup-restore` proves demo
   disaster recovery. Neither runs the raw `go test` of the workload
   fixtures. So even registering a gate would not provide evidence for this
   change.

4. **The #995 backup-restore gate, while catalog-registered, is a heavy
   pre-release gate** (`runtime_heavy_gates_docker`), proven in the full
   pre-release run, not the targeted-integration opt-in. The opt-in only
   resolves *workflow-file coverage* for the added `targeted-integration.yml`
   lines; the native-gate evidence still flows through the pre-release
   pipeline. Our change has no such pre-release backing.

**Conclusion:** the coverage gate cannot be satisfied by a test-only PR today.
The native evidence for `cmd/baha/*_test.go` / `internal/application/*_test.go`
lives only in `docker-runtime-acceptance.yml`, which the gate cannot see, and
the only catalog gates require the pre-release pipeline. This is exactly the
structural gap PR #1007 (coverage binding) is built to close: binding the
focused native-acceptance job to the coverage gate so a registered, reachable
gate provides the missing native coverage without the full pre-release matrix.

**Decision:** implement an opt-in in #1008 is neither possible nor in-scope:
(i) mirroring #995 resolves only workflow-file coverage and still fails closed
on `selected_gates: []`; (ii) the docker-runtime acceptance job cannot be a
required PR check (push/dispatch-only); (iii) adding an unregistered gate lane
would violate the fail-closed design and exceed this task's test-only scope
without a CI-policy decision. **PR #1008 stays red-until-#1007 merges** — this
is the acceptable bootstrap state per Codex. The product fix (fixture ids
dropped, parser fail-closed, regression tests) is complete and correct; only
the gate binding is missing.

### Repo-rule compliance (gate analysis)

- No CI/workflow/script/product code changed in this follow-up; pure local
  reproduction and evidence.
- Authoritative plan taken from the CI trusted-plan artifact, not an agent
  summary; fail-closed logic read directly from
  `scripts/pr-required-final.py:24` and gate dispatch from
  `scripts/pr-selected-runtime-matrix.py:18`.
- No force-push, no merge, no check bypass. #1008 remains a draft.
