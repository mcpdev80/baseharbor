# Evidence: w90-device-enroll — one-use recipient-bound workstation enrollment (t-20261018-r27)

## Lane / Mission
- Lane: w90, Coord f49a3303, Worker model laptop
- Card: t-20261018-r27-device-enrollment (active, owner coord-welle4 lane w90)
- Mission: coordinator/missions/w90-device-enroll.txt (Option A, Owner scope decision 2026-11-04)
- Worktree: /tmp/opencode/w90-enroll-wt (base develop 3d08ef72)
- Branch: work/r27-workstation-enrollment
- Candidate SHA: 299f81434ef996b5408363dfd7b3eb67e4b1a148
- PR: #1364 (draft), base develop 3d08ef72
- CI: run 41800237126 terminal SUCCESS on candidate (draft gates: Workflow policy + Source format)

## Scope decision (Owner, 2026-11-04)
- Deliverable = Option A: a NEW bounded workstation-enrollment slice, own namespace, first-class
  workstation identity, distinct one-use store. Owner rationale: card wording "owner/HTTP adapter"
  = own slice; Option B would fight over shared index/registration files owned by a DIFFERENT task
  (t-20261018-r27-public-surfaces) — no touch; Option C contradicts card "standalone unused adapter
  is not complete".
- Encapsulation rule: own namespace contracts/workstationenrollment/v1 + internal/workstationenrollment.
  NO edits to shared register files. If a shared entry seems necessary: do NOT edit, report in END as
  coordination-need.

## Contract reading (accepted predecessors)
- t-20261018-r27-authority-contract (#1348 accepted): one installation owner for local/remote;
  serviceaccess.Issuer identity/revocation/freshness boundary; resolveManagementConnectionIssuer at
  cmd/baha/application_service_issuer.go.
- t-20261018-r27-profile-contract (#1324 accepted): management-access/v1 EnrollDevice binds the NODE
  one-use store (internal/targetenrollment). This is the node path I must NOT touch.
- t-20261022-r27-ci-access (#1357 accepted): one integration candidate per chain; CI discipline.
- Native delivery decision 2026-10-18-r27: reuse existing authorities, no private framework.
- credential-access-v1.md: Class C machine identity isolated; device-enrollment is sensitive one-use.
- The current develop device-enrollment = NODE/connector enrollment (internal/targetenrollment,
  Core tenant/Target/node SPIFFE). Workstation identity = installation-scoped SPIFFE
  spiffe://baseharbor/workstations/<tenant>/<station>, distinct.

## PLAN-Checkliste
- Read task + all accepted predecessors + contract specs; fix write-set: own namespace only, no shared register edits
- Create numbered plan in all three places: board json, ## PLAN of evidence, native todo (board sufficient; no todowrite tool in this opencode session)
- Author internal/workstationenrollment: distinct Scope, one-use store, Authority.Enroll validate-then-consume fail-closed, RevokedStatus boundary
- Author contracts/workstationenrollment/v1: go types + fail-closed ValidateKindEnforcement + JSON schema
- Author HTTP adapter at management boundary: one-use request/response, recipient binding, short-lived validity, registry-fakt with scope+time
- Write tests: four rote-first cases + positive with scope+time; reuse serviceaccess issuer + ValidateSignedCSR
- Local checks: go build, go test, go vet; fix real failures
- Draft PR #1364; gofmt-clean; push; CI draft-gates SUCCESS; ls-remote==head
- Handoff: plan task note + END_DONE

## Deliverable
Own namespace, production-shape, at the management boundary:

### internal/workstationenrollment/scope.go
- Scope{TenantID (installation UUID), Station (single DNS label)}. Identity =
  spiffe://baseharbor/workstations/<tenant>/<station> — installation-scoped, NOT a Core connector
  SPIFFE. Validate() rejects malformed/empty. digest() shared SHA-256 hex.

### internal/workstationenrollment/authority.go
- Authority{store Store, issuer serviceaccess.CSRIssuer}. New() nil-guards. Create() binds lifetime
  (<=5 min) and cert TTL (<=24h), persists one-use, returns digested bootstrap. Enroll() is
  validate-then-consume: validCredential(token/nonce) -> validate CSR -> store.Consume (single use)
  -> SignCSR -> TrustBundle -> ValidateSignedCSR against authoritative trust -> RevokedStatus
  (err!=nil OR revoked fails closed). No private key generated/returned.

### internal/workstationenrollment/store.go
- Store interface {Create, Consume}. memoryStore: concurrency-safe (sync.Mutex), persists consumption
  before returning success (concurrent single-use cannot both succeed), rejects expired grants.

### internal/workstationenrollment/request.go
- Request{scope, token, nonce, csrPEM, ttl}, NewRequest() scope-validates. Result{Certificate, Trust,
  Serial, ExpiresAt}.

### internal/workstationenrollment/http/ (package workstationenrollmenthttp)
- http.go: BootstrapRequest/Response (token/nonce/validity-window), EnrollRequest/Response (leaf +
  trust + registry-fakt: installation/station/serial/expires_at/at).
- http_handler.go: Handler{Enrollment}. ServeBootstrap (POST /bootstrap) + ServeEnroll (POST /enroll).
  Binds recipient/scope from request, single-use consume, returns registry-fakt. No private key.
  Router(mux) registration; shared register files NOT touched.

### contracts/workstationenrollment/v1/
- workstationenrollment.go: Record, ImportCapability, Storage, Bootstrap window,
  ValidateKindEnforcement (fail-closed), EnrollDevice(enforcer, issuer, record, request) — validates
  kind/import/window/recipient-scope before consuming one-use. Reuses ws.Scope/Request +
  serviceaccess.CSRIssuer.
- workstationenrollment.schema.json: draft 2020-12, $id canonical path, enum kind, recipient
  SPIFFE workstation pattern, bootstrap TTL bounds, storage if/then.

## Rote-first results (RED cases proven)
1. Second use of a one-use bootstrap => rejected: TestSecondUseRejected (Authority.Enroll replay +
   store.Consume duplicate both fail closed).
2. Elapsed/expire window => rejected: TestExpiryRejected (memoryStore consumes an expired grant =>
   ErrDenied).
3. Foreign recipient/scope => rejected: TestForeignRecipientRejected (bootstrap presented under a
   foreign station scope fails closed; store rejects mismatched consume).
4. Malformed/expired leaf => rejected: TestMalformedLeafRejected (empty CSR fails closed).
5. Positive: TestEnrollPositiveIssuesLeafWithScopeAndTime — single use issues a leaf bound to the
   workstation scope, trust bundle carries issuer reference, expiry live; registry-fakt records the
   installation-scoped SPIFFE identity; store consumed exactly one.
6. HTTP: TestHTTPBootstrapThenEnroll — bootstrap returns validity window; enroll consumes once and
   returns leaf + trust + registry-fakt (installation/station/serial/expires_at/live/at); replay
   fails closed. TestHTTPBootstrapRejectsInvalidStation — invalid scope rejected (4xx).
7. Contract: TestRecordPositiveValidates + TestRecordForeignRecipientRejected +
   TestRecordScopeMismatchRejected + TestRecordElapsedWindowRejected + TestRecordImportOpenRejected.

## Local checks (real runs)
- go build ./... : EXIT 0 (only my new packages; no other package touched)
- go test ./internal/workstationenrollment/... ./contracts/workstationenrollment/... : all ok
- go vet ./internal/workstationenrollment/... ./contracts/workstationenrollment/... : EXIT 0
- gofmt -l internal/workstationenrollment/ contracts/workstationenrollment/ : clean (formatted, then re-tested)
- scripts/source-readability-audit.sh : Go file size PASS, Function span PASS, Regression ratchet PASS
- CI 41800237126 on 299f8143 : terminal SUCCESS. Draft-gate jobs: Workflow policy -> success,
  Source format -> success. (Full product CI gates are draft==false; draft PR is correct per card
  "PR DRAFT".)

## Real failures fixed (diagnosed, not swept)
- TTL-before-Validate: Authority.Enroll called signing.Validate() before signing.TTL was set;
  serviceaccess.CSRSigningRequest.validate() requires r.TTL > 0, so the positive enroll failed with
  "invalid, expired, or consumed". Fixed by setting signing.TTL = request.ttl() before validate().
- CSR identity: ValidateSignedCSR requires len(leaf.URIs)==1 and leaf.Subject.CommonName==Identity.
  newCSRPEM() now carries the SPIFFE identity as both CommonName AND URI subject (Go encodes URI
  inside the 2.5.29.17 SAN extension, not a separate extension, so validation's extension allow-list
  is satisfied).
- LocalIssuer is NOT a serviceaccess.CSRIssuer (no SignCSR method); serviceissuer.New(t) from
  testsupport implements SignCSR + RevokedStatus and is the trusted CA used in tests.
- HTTP round-trip: BootstrapResponse previously concatenated token|nonce; the workstation must return
  the bare token AND nonce at enroll, so BootstrapResponse now returns Token + Nonce separately.

## repo-rule compliance
- Write-set: ONLY new files under contracts/workstationenrollment/v1/ and
  internal/workstationenrollment/ (authority.go, request.go, scope.go, store.go,
  http/http.go, http/http_handler.go, + _test.go files). NO edits to any existing tracked file.
- No node-enrollment files touched (internal/targetenrollment untouched).
- No shared register/registration files edited; Owner scope decision says report any such need in END.
- conformance/provider/v1/testdata/external/go.mod was a go build artifact; reverted to keep write-set
  clean (git checkout + removed generated go.sum).
- Go file-size: all <= 200 lines (max 185: contracts/workstationenrollment/v1/workstationenrollment.go).
- .github/scripts: untouched.
- No secrets in evidence; leaf PEM is a test-only ephemeral CA, not a production credential.

## Handoff
- PR #1364 (draft) head 299f8143, base develop 3d08ef72. Draft PR is correct per card ("PR DRAFT");
  full product CI + Maintainer acceptance are NOT authorized by this note.
- One-use proven (rote case 1 + HTTP replay). Separate device identity at installation scope.
- No shared-register coordination-need surfaced (none appeared during implementation).
- Next: Owner/Maintainer review + CODEX-REVIEW against this evidence; CODEX-MERGE only if it passes.

## END-LANE-STATUS
END_DONE w90-device-enroll: head=299f8143 pr=1364 one-use=proven ls-remote=299f8143 |
bounded workstation-enrollment owner/HTTP adapter delivered in own namespace, four rote-first cases
pass, draft CI 41800237126 SUCCESS; PR #1364 draft awaiting Maintainer/Codex acceptance.
