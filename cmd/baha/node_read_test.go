package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/tenancy"
)

// The regressions below exercise the production read path that `baha node list`
// and `baha node status` actually take in the field: local node registration ->
// requestNodeLifecycle -> the real Core lifecycle status handler
// (internal/targetenrollment HTTPHandler over a NodeLifecycleStore) ->
// classifyNodeTrust -> the rendered CLI output. No test helper reimplements the
// state rule; every assertion reads the CLI's own output.
//
// Core TLS terminates at a local stunnel-style front (internal/serviceaccess/
// tlslistener.go); the enrollment handler is the same ServeHTTP that
// internal/controlplaneruntime/server_run.go mounts on the operator mux. The
// in-process server therefore runs the production handler behind a proxy that
// enforces the same requirement the front end does: a request arriving without
// TLS is refused.
const (
	nodeTestTenantID = "12345678-1234-1234-1234-123456789abc"
	nodeTestNode     = "node-a"
	nodeTestTarget   = "edge-a"
	nodeTestIdentity = "baseharbor-node-cli-test-identity"
)

// nodeCoreAuthority is the in-process stand-in for Core's authoritative
// enrollment row (connector_nodes): issued certificate serial, not-after and
// revocation. It stores lifecycle facts only, never a trust-state label: the
// PENDING/ENROLLED/EXPIRED/REVOKED decision stays entirely inside the CLI.
type nodeCoreAuthority struct {
	mu                  sync.Mutex
	status              targetenrollment.NodeStatus
	err                 error
	statusReads         int
	grantLifetime       time.Duration
	grants              map[string]targetenrollment.Grant
	createErr           error
	core                *targetenrollment.Authority
	certificateLifetime time.Duration
}

func (a *nodeCoreAuthority) Status(context.Context, targetenrollment.Scope) (targetenrollment.NodeStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statusReads++
	if a.err != nil {
		return targetenrollment.NodeStatus{}, a.err
	}
	return a.status, nil
}

// RevokeCurrent mirrors internal/database/connector_enrollment.go:216-251: the
// current certificate serial is marked revoked in the node row. An unissued node
// has nothing to revoke and fails closed.
func (a *nodeCoreAuthority) RevokeCurrent(context.Context, targetenrollment.Scope) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.status.Enrolled || a.status.Serial == "" {
		return "", targetenrollment.ErrDenied
	}
	a.status.Revoked = true
	a.err = nil
	return a.status.Serial, nil
}

func (a *nodeCoreAuthority) Create(_ context.Context, grant targetenrollment.Grant) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.createErr != nil {
		return a.createErr
	}
	if a.certificateLifetime != 0 {
		grant.CertificateTTL = a.certificateLifetime
	}
	if a.grants == nil {
		a.grants = map[string]targetenrollment.Grant{}
	}
	a.grants[grant.TokenDigest] = grant
	return nil
}

func (a *nodeCoreAuthority) Consume(_ context.Context, scope targetenrollment.Scope, tokenDigest, nonceDigest, _ string, now time.Time) (targetenrollment.Grant, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return targetenrollment.Grant{}, a.err
	}
	grant, ok := a.grants[tokenDigest]
	if !ok || grant.Scope != scope || grant.NonceDigest != nonceDigest || !grant.ExpiresAt.After(now) {
		return targetenrollment.Grant{}, targetenrollment.ErrDenied
	}
	delete(a.grants, tokenDigest)
	return grant, nil
}

func (a *nodeCoreAuthority) RecordIssued(_ context.Context, scope targetenrollment.Scope, _ string, serial string, expires time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return a.err
	}
	for _, grant := range a.grants {
		if grant.Scope == scope {
			return targetenrollment.ErrDenied
		}
	}
	a.status = targetenrollment.NodeStatus{Enrolled: true, Serial: serial, ExpiresAt: expires}
	return nil
}

func (a *nodeCoreAuthority) setGrantLifetime(d time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.grantLifetime = d
}

// bindCertificateLifetime pins the certificate TTL of the next grant the caller
// requests through the production add path. Core commits this value on issuance;
// it is the store-side mirror of a certificate lifetime, not a CLI input.
func (a *nodeCoreAuthority) bindCertificateLifetime(d time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Core clamps a sub-second TTL to its one-second signing floor; below that
	// the grant would be refused rather than issued and already expired.
	if d < time.Second {
		d = time.Second
	}
	a.certificateLifetime = d
}

// advanceIssuedCertificateToExpiry stands in for the remote host coming back
// after the certificate lifetime elapsed: the certificate is still issued and
// unrevoked, only its not-after is in the past. The CLI must classify EXPIRED
// from the certificate lifecycle; the one-use grant long expired before that.
func (a *nodeCoreAuthority) advanceIssuedCertificateToExpiry() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status.ExpiresAt = time.Now().Add(-time.Minute).UTC()
	a.err = nil
}

// redemption reports the certificate lifecycle Core holds right now: whether an
// identity was issued at all and with which expiry.
func (a *nodeCoreAuthority) redemption() (targetenrollment.NodeStatus, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status, a.status.Enrolled
}

func (a *nodeCoreAuthority) reads() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.statusReads
}

func (a *nodeCoreAuthority) setStatus(status targetenrollment.NodeStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = status
	a.err = nil
}

func (a *nodeCoreAuthority) setError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.err = err
}

// nodeCoreFixture wires the real Core enrollment HTTP surface in-process and
// leaves behind the CLI state a real `baha node add` produces: node registration
// file, operator session and deployment config with a connector access entry.
func nodeCoreFixture(t *testing.T, initial targetenrollment.NodeStatus) *nodeCoreAuthority {
	t.Helper()
	stateRoot := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateRoot)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(stateRoot, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(stateRoot, "data"))
	t.Setenv("BASEHARBOR_TARGET", "")

	authority := &nodeCoreAuthority{status: initial, grantLifetime: 2 * time.Minute}
	issuer, err := serviceaccess.EnsureLocalIssuer(filepath.Join(stateRoot, "core-pki"))
	if err != nil {
		t.Fatal(err)
	}
	core, err := targetenrollment.New(authority, issuer)
	if err != nil {
		t.Fatal(err)
	}
	authority.core = core
	handler, err := targetenrollment.NewHTTP(core, func(_ context.Context, target, node, environment string) (targetenrollment.Scope, error) {
		if target != nodeTestTarget || node != nodeTestNode || environment != "dev" {
			return targetenrollment.Scope{}, targetenrollment.ErrDenied
		}
		return targetenrollment.Scope{TenantID: nodeTestTenantID, TargetID: target, NodeID: node, Runtime: "podman"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method != http.MethodPost:
			w.WriteHeader(http.StatusMethodNotAllowed)
		case r.URL.Path == targetenrollment.StatusPath, r.URL.Path == targetenrollment.AuthorizationPath:
			nodeOperatorRequest(w, r, handler)
		case r.URL.Path == targetenrollment.EnrollmentPath:
			// The scoped bootstrap exchange bypasses operator OIDC in
			// production (internal/controlplaneruntime/server_run.go:311-313).
			handler.ServeHTTP(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	server.Listener = listener
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	// Core's public HTTPS front and the enrollment signing authority are the
	// same managed local PKI: the server leaf is one of this issuer's own
	// certificates, so the CLI trusts the endpoint with the very CA bundle its
	// enrollment produced.
	serverCertificate, err := nodeServerCertificate(t, issuer, listener.Addr().(*net.TCPAddr).IP.String())
	if err != nil {
		t.Fatal(err)
	}
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCertificate}}
	server.StartTLS()
	t.Cleanup(server.Close)

	caFile := filepath.Join(t.TempDir(), "core-ca.pem")
	trust, err := issuer.TrustBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caFile, trust.PEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := operatorauth.SaveSession(operatorauth.Session{
		Target: "local", Environment: "dev", Issuer: "https://issuer.test", Subject: "operator-a",
		ClientID: "baha-cli", IDToken: nodeTestIdentity, ExpiresAt: time.Now().Add(6 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveNodeRecord(nodeRecord{
		ContractVersion: machine.ContractVersion, TenantID: nodeTestTenantID, TargetID: nodeTestTarget,
		NodeID: nodeTestNode, Runtime: "podman", Environment: "dev", CoreTarget: "local",
		CoreURL: server.URL, CoreAddress: server.Listener.Addr().String(), ServerName: "127.0.0.1", CAFile: caFile,
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DefaultTarget = "local"
	cfg.Access["local"] = deployment.AccessDefinition{Provider: "local", Reference: "local"}
	cfg.Access["node-"+nodeTestNode] = deployment.AccessDefinition{Provider: nodeConnectorProvider, Reference: nodeTestNode}
	cfg.Targets["local"] = deployment.TargetDefinition{Runtime: deployment.RuntimeDefinition{Provider: "docker"}, Access: deployment.TargetAccess{Reference: "local"}}
	cfg.Targets[nodeTestTarget] = deployment.TargetDefinition{TenantID: nodeTestTenantID, Runtime: deployment.RuntimeDefinition{Provider: "podman"}, Access: deployment.TargetAccess{Reference: "node-" + nodeTestNode}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return authority
}

// nodeOperatorRequest applies the operator authentication Core enforces on the
// authorization and lifecycle routes
// (internal/targetenrollment/http.go:110-124,167-181) before handing the request
// to the production handler.
func nodeOperatorRequest(w http.ResponseWriter, r *http.Request, handler http.Handler) {
	if r.Header.Get("Authorization") != "Bearer "+nodeTestIdentity {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	expires := time.Now().Add(time.Hour)
	ctx := identity.WithPrincipal(r.Context(), &identity.Principal{Issuer: "https://issuer.test",
		Subject: "operator-a", ExpiresAt: &expires})
	ctx = tenancy.WithContext(ctx, &tenancy.Context{TenantID: nodeTestTenantID, ExternalIdentityID: "identity-a", Roles: []string{"editor"}})
	handler.ServeHTTP(w, r.WithContext(ctx))
}

// nodeCoreTarget reads back the registration the fixture wrote, so every helper
// goes through the same persisted state the CLI commands use.
func nodeCoreTarget(t *testing.T) nodeRecord {
	t.Helper()
	record, err := loadNodeRecord(nodeTestTarget)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// nodeServerCertificate issues the Core front-end leaf through the same local
// authority that signs node identities, so the recorded CA file really is the
// trust anchor of the endpoint the CLI talks to.
func nodeServerCertificate(t *testing.T, issuer *serviceaccess.LocalIssuer, host string) (tls.Certificate, error) {
	t.Helper()
	request := serviceaccess.CertificateRequest{CommonName: host, TTL: time.Hour}
	if ip := net.ParseIP(host); ip != nil {
		request.IPAddresses = []net.IP{ip}
	} else {
		request.DNSNames = []string{host}
	}
	issued, err := issuer.Issue(context.Background(), request)
	if err != nil {
		return tls.Certificate{}, err
	}
	leafBlock, _ := pem.Decode(issued.Certificate)
	keyBlock, _ := pem.Decode(issued.PrivateKey)
	if leafBlock == nil || keyBlock == nil {
		return tls.Certificate{}, errors.New("local issuer published unusable server material")
	}
	return tls.X509KeyPair(issued.Certificate, issued.PrivateKey)
}

// requestNodeGrantThroughProductionAddPath obtains a bootstrap authorization
// through requestNodeAuthorization, the exact HTTP call `baha node add` makes in
// production. A nil authority uses the CLI's default grant lifetime.
func requestNodeGrantThroughProductionAddPath(t *testing.T, authority *nodeCoreAuthority) targetenrollment.Bootstrap {
	t.Helper()
	grantLifetime, certificateLifetime := nodeGrantLifetime, nodeCertificateLifetime
	if authority != nil {
		authority.mu.Lock()
		grantLifetime, certificateLifetime = authority.grantLifetime, authority.certificateLifetime
		authority.mu.Unlock()
		if certificateLifetime == 0 {
			certificateLifetime = nodeCertificateLifetime
		}
	}
	record := nodeCoreTarget(t)
	grant, err := requestNodeAuthorization(context.Background(), record.CoreTarget, record.Environment, record.CoreURL, record.CAFile,
		nodeTestTarget, nodeTestNode, grantLifetime, certificateLifetime)
	if err != nil {
		t.Fatalf("production node grant request failed: %v", err)
	}
	return grant
}

// nodeRedeemGrant runs the same enrollment exchange the connector performs on
// the remote host: it posts a client-generated CSR to Core's scoped bootstrap
// route. Success means Core issued and committed a node certificate, the only
// fact that can make a node ENROLLED. certificateLifetime is committed through
// the grant so a test can produce an already-expired certificate deterministically.
func nodeRedeemGrant(t *testing.T, authority *nodeCoreAuthority, bootstrap targetenrollment.Bootstrap, certificateLifetime time.Duration) {
	t.Helper()
	scope := nodeTestScope()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	identityURL, err := url.Parse(scope.Identity())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{URIs: []*url.URL{identityURL}}, key)
	if err != nil {
		t.Fatal(err)
	}
	// The certificate lifetime is bound to the grant Core issued, exactly as the
	// production add path pins it, so Enroll can only mint what that grant allows.
	authority.bindCertificateLifetime(certificateLifetime)
	issued, err := authority.core.Enroll(context.Background(), targetenrollment.Request{
		Scope: scope, Token: bootstrap.Token, Nonce: bootstrap.Nonce,
		CSRPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: raw}),
	})
	if err != nil {
		t.Fatalf("production Core enrollment (redemption) failed: %v", err)
	}
	if !issued.Certificate.ExpiresAt.After(time.Time{}) {
		t.Fatal("Core must report the issued certificate expiry")
	}
}

// nodeCoreFixtureWithGrant leaves the CLI state `baha node add` produces after a
// successful grant issuance: the local record carries the Core grant expiry and
// Core's enrollment row is still unredeemed (no certificate at all).
func nodeCoreFixtureWithGrant(t *testing.T, grantExpiry time.Time) *nodeCoreAuthority {
	t.Helper()
	authority := nodeCoreFixture(t, targetenrollment.NodeStatus{})
	setNodeGrantExpiry(t, grantExpiry)
	return authority
}

// setNodeGrantExpiry overwrites only the grant metadata of the stored record, so
// a test can prove no trust label or displayed expiry follows that field.
func setNodeGrantExpiry(t *testing.T, expiry time.Time) {
	t.Helper()
	record, err := loadNodeRecord(nodeTestTarget)
	if err != nil {
		t.Fatal(err)
	}
	record.GrantExpiresAt = expiry.UTC().Round(time.Second)
	if err := saveNodeRecord(record); err != nil {
		t.Fatal(err)
	}
}

// readNodeRecordRaw returns the stored registration file as a generic map, so a
// test can assert exactly which fields the local registration surface carries.
func readNodeRecordRaw(t *testing.T, target string) map[string]any {
	t.Helper()
	path, err := nodeRecordPath(target)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	return stored
}

// TestNodeListStateTableDrivesProductionRedemptionPath is the load-bearing
// regression for the trust state: every case drives the production path in the
// same shape `baha node connect` and `baha node list` take in the field — a
// grant Core really issued, a CSR Core really redeems through
// Authority.Enroll -> RecordIssued, then the CLI's own lifecycle read,
// classification and rendered output. No case presets a state label.
func TestNodeListStateTableDrivesProductionRedemptionPath(t *testing.T) {
	for _, test := range []struct {
		name        string
		lifetime    time.Duration
		certificate time.Duration
		expire      bool
		wantState   string
	}{
		{name: "enrolled", lifetime: 2 * time.Minute, certificate: time.Hour, wantState: "ENROLLED"},
		{name: "certificate expired", lifetime: 2 * time.Minute, certificate: time.Hour, expire: true, wantState: "EXPIRED"},
		{name: "grant unredeemed", lifetime: 2 * time.Minute, certificate: time.Hour, wantState: "PENDING"},
	} {
		t.Run(test.name, func(t *testing.T) {
			grantLifetime, certificateLifetime := test.lifetime, test.certificate
			authority := nodeCoreFixture(t, targetenrollment.NodeStatus{})
			authority.setGrantLifetime(grantLifetime)
			bootstrap := requestNodeGrantThroughProductionAddPath(t, authority)

			out, errOut := &strings.Builder{}, &strings.Builder{}
			if err := nodeList(context.Background(), nil, out, errOut); err != nil {
				t.Fatalf("production nodeList failed: %v (%s)", err, errOut)
			}
			before := out.String()
			if !strings.Contains(before, "PENDING") || strings.Contains(before, "ENROLLED") || strings.Contains(before, "EXPIRED") {
				t.Fatalf("an unredeemed grant must read PENDING before redemption, got:\n%s", before)
			}
			if strings.Contains(before, bootstrap.ExpiresAt.Format(time.RFC3339)) {
				t.Fatalf("PENDING must not surface the grant expiry as a trust expiry, got:\n%s", before)
			}
			if test.wantState == "PENDING" {
				if authority.reads() == 0 {
					t.Fatal("node list must consult the lifecycle authority before labelling a node")
				}
				return
			}

			nodeRedeemGrant(t, authority, bootstrap, certificateLifetime)
			if test.expire {
				authority.advanceIssuedCertificateToExpiry()
			}

			out, errOut = &strings.Builder{}, &strings.Builder{}
			if err := nodeList(context.Background(), nil, out, errOut); err != nil {
				t.Fatalf("production nodeList after redemption failed: %v (%s)", err, errOut)
			}
			table := out.String()
			if !strings.Contains(table, test.wantState) {
				t.Fatalf("redemption must render %s from the certificate lifecycle, got:\n%s", test.wantState, table)
			}
			for _, other := range []string{"ENROLLED", "EXPIRED", "REVOKED", "PENDING"} {
				if other != test.wantState && strings.Contains(table, other) {
					t.Fatalf("list must render exactly one trust label (%s), got %s:\n%s", test.wantState, other, table)
				}
			}
			if !strings.Contains(table, "node-a") || !strings.Contains(table, "edge-a") {
				t.Fatalf("list must still identify the node, got:\n%s", table)
			}
			if authority.reads() == 0 {
				t.Fatal("node list never consulted the authoritative Core lifecycle status")
			}
		})
	}
}

func TestNodeListRevokedComesFromLifecycleAuthorityNotGrant(t *testing.T) {
	authority := nodeCoreFixture(t, targetenrollment.NodeStatus{})
	bootstrap := requestNodeGrantThroughProductionAddPath(t, authority)
	nodeRedeemGrant(t, authority, bootstrap, time.Hour)
	setNodeGrantExpiry(t, time.Now().Add(9*time.Minute))

	out, errOut := &strings.Builder{}, &strings.Builder{}
	if err := nodeList(context.Background(), nil, out, errOut); err != nil {
		t.Fatalf("production nodeList failed: %v", err)
	}
	if !strings.Contains(out.String(), "ENROLLED") {
		t.Fatalf("a redeemed certificate must read ENROLLED before disconnect, got:\n%s", out)
	}

	// Revocation is a Core-side act: the same production lifecycle store flips to
	// revoked and the CLI must report it. Locally nothing but a grant expiry is
	// recorded, so REVOKED cannot be invented from local state.
	if err := authority.core.Disconnect(context.Background(), nodeTestScope()); err != nil {
		t.Fatalf("production Core disconnect failed: %v", err)
	}
	out, errOut = &strings.Builder{}, &strings.Builder{}
	if err := nodeList(context.Background(), nil, out, errOut); err != nil {
		t.Fatalf("production nodeList after revocation failed: %v", err)
	}
	table := out.String()
	if !strings.Contains(table, "REVOKED") {
		t.Fatalf("a revoked lifecycle identity must render REVOKED, got:\n%s", table)
	}
	if strings.Contains(table, "ENROLLED") || strings.Contains(table, "PENDING") {
		t.Fatalf("REVOKED must outrank the other labels, got:\n%s", table)
	}
}

func TestNodeListAuthorityUnavailableFailsClosedToPending(t *testing.T) {
	authority := nodeCoreFixture(t, targetenrollment.NodeStatus{})
	bootstrap := requestNodeGrantThroughProductionAddPath(t, authority)
	nodeRedeemGrant(t, authority, bootstrap, time.Hour)
	// A live enrolled identity, then the lifecycle store becomes unreadable.
	authority.setError(targetenrollment.ErrDenied)

	out, errOut := &strings.Builder{}, &strings.Builder{}
	if err := nodeList(context.Background(), nil, out, errOut); err != nil {
		t.Fatalf("node list must stay usable when one authority read fails: %v", err)
	}
	table := out.String()
	if !strings.Contains(table, "PENDING") || strings.Contains(table, "ENROLLED") {
		t.Fatalf("an unreadable lifecycle authority must fail closed to PENDING, got:\n%s", table)
	}
}

func TestNodeListMissingOperatorSessionFailsClosedToPending(t *testing.T) {
	authority := nodeCoreFixture(t, targetenrollment.NodeStatus{})
	bootstrap := requestNodeGrantThroughProductionAddPath(t, authority)
	nodeRedeemGrant(t, authority, bootstrap, time.Hour)
	if err := operatorauth.ClearSession("local", "dev"); err != nil {
		t.Fatal(err)
	}

	out, errOut := &strings.Builder{}, &strings.Builder{}
	if err := nodeList(context.Background(), nil, out, errOut); err != nil {
		t.Fatalf("node list must stay usable without an operator session: %v", err)
	}
	table := out.String()
	if !strings.Contains(table, "PENDING") || strings.Contains(table, "ENROLLED") {
		t.Fatalf("a missing session must never be reported as enrollment, got:\n%s", table)
	}
}

func TestNodeListLegacyRecordWithEnrolledFlagStaysPending(t *testing.T) {
	// A registration file from an earlier build may still carry a grant-derived
	// enrollment claim. The lifecycle authority is the only source: an
	// unredeemed node stays PENDING even when the local file claims enrollment.
	authority := nodeCoreFixture(t, targetenrollment.NodeStatus{})
	path, err := nodeRecordPath(nodeTestTarget)
	if err != nil {
		t.Fatal(err)
	}
	stored := readNodeRecordRaw(t, nodeTestTarget)
	stored["enrolled"] = true
	stored["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	legacy, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	out, errOut := &strings.Builder{}, &strings.Builder{}
	if err := nodeList(context.Background(), nil, out, errOut); err != nil {
		t.Fatalf("production nodeList failed: %v", err)
	}
	table := out.String()
	if !strings.Contains(table, "PENDING") || strings.Contains(table, "ENROLLED") {
		t.Fatalf("a local enrollment claim is no authority; must stay PENDING, got:\n%s", table)
	}
	if authority.reads() == 0 {
		t.Fatal("node list must consult the lifecycle authority")
	}
}

func TestNodeStatusStateMatchesProductionListRendering(t *testing.T) {
	for _, test := range []struct {
		name        string
		certificate time.Duration
		expire      bool
		disconnect  bool
		wantState   string
	}{
		{name: "enrolled", certificate: time.Hour, wantState: "ENROLLED"},
		{name: "certificate expired", certificate: time.Hour, expire: true, wantState: "EXPIRED"},
		{name: "revoked", certificate: time.Hour, disconnect: true, wantState: "REVOKED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			authority := nodeCoreFixture(t, targetenrollment.NodeStatus{})
			bootstrap := requestNodeGrantThroughProductionAddPath(t, authority)
			nodeRedeemGrant(t, authority, bootstrap, test.certificate)
			if test.expire {
				authority.advanceIssuedCertificateToExpiry()
			}
			if test.disconnect {
				if err := authority.core.Disconnect(context.Background(), nodeTestScope()); err != nil {
					t.Fatalf("production Core disconnect failed: %v", err)
				}
			}

			statusOut, errOut := &strings.Builder{}, &strings.Builder{}
			if err := nodeStatus(context.Background(), []string{nodeTestTarget}, statusOut, errOut); err != nil {
				t.Fatalf("production nodeStatus failed: %v (%s)", err, errOut)
			}
			if !strings.Contains(statusOut.String(), "State   "+test.wantState) {
				t.Fatalf("node status must render %s from the certificate lifecycle, got:\n%s", test.wantState, statusOut)
			}

			// The single shared classifier must agree between the two commands.
			listOut, listErr := &strings.Builder{}, &strings.Builder{}
			if err := nodeList(context.Background(), nil, listOut, listErr); err != nil {
				t.Fatalf("production nodeList failed: %v", err)
			}
			if !strings.Contains(listOut.String(), test.wantState) {
				t.Fatalf("node list and node status must agree on %s, got:\n%s", test.wantState, listOut)
			}
		})
	}
}

func TestNodeStatusJSONCarriesRevokedAndCertificateExpiry(t *testing.T) {
	authority := nodeCoreFixture(t, targetenrollment.NodeStatus{})
	bootstrap := requestNodeGrantThroughProductionAddPath(t, authority)
	nodeRedeemGrant(t, authority, bootstrap, time.Hour)
	if err := authority.core.Disconnect(context.Background(), nodeTestScope()); err != nil {
		t.Fatalf("production Core disconnect failed: %v", err)
	}

	out, errOut := &strings.Builder{}, &strings.Builder{}
	if err := nodeStatus(context.Background(), []string{nodeTestTarget, "--json"}, out, errOut); err != nil {
		t.Fatalf("production nodeStatus --json failed: %v", err)
	}
	var result nodeStatusResult
	if err := json.Unmarshal([]byte(out.String()), &result); err != nil {
		t.Fatalf("status --json decode: %v", err)
	}
	if !result.Revoked {
		t.Fatalf("revoked must be sourced from the lifecycle authority: %s", out)
	}
	if _, redeemed := authority.redemption(); !redeemed {
		t.Fatal("the fixture must hold a redeemed certificate before disconnect")
	}
	issued, _ := authority.redemption()
	if !result.ExpiresAt.Equal(issued.ExpiresAt) {
		t.Fatalf("status expiry %v must be the Core-issued certificate expiry %v", result.ExpiresAt, issued.ExpiresAt)
	}

	out.Reset()
	if err := nodeStatus(context.Background(), []string{nodeTestTarget}, out, errOut); err != nil {
		t.Fatalf("production nodeStatus table failed: %v", err)
	}
	if !strings.Contains(out.String(), "State   REVOKED") {
		t.Fatalf("a revoked node must render REVOKED, got:\n%s", out)
	}
}

func TestNodeStatusSerialAndCapabilitiesFailClosedThroughProductionPath(t *testing.T) {
	authority := nodeCoreFixture(t, targetenrollment.NodeStatus{})
	bootstrap := requestNodeGrantThroughProductionAddPath(t, authority)
	nodeRedeemGrant(t, authority, bootstrap, time.Hour)

	out, errOut := &strings.Builder{}, &strings.Builder{}
	if err := nodeStatus(context.Background(), []string{nodeTestTarget, "--json"}, out, errOut); err != nil {
		t.Fatalf("production nodeStatus --json failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out.String()), &decoded); err != nil {
		t.Fatalf("status --json not parseable: %v", err)
	}
	if _, ok := decoded["serial"]; ok {
		t.Fatalf("status must not emit a serial the Core status projection omits: %s", out)
	}
	if _, ok := decoded["capabilities"]; ok {
		t.Fatalf("status must not emit capabilities the Core status projection omits: %s", out)
	}
	if decoded["enrolled"] != true || decoded["revoked"] != false {
		t.Fatalf("the lifecycle verdicts must survive the production round trip: %s", out)
	}

	out.Reset()
	if err := nodeStatus(context.Background(), []string{nodeTestTarget}, out, errOut); err != nil {
		t.Fatalf("production nodeStatus table failed: %v", err)
	}
	if strings.Contains(out.String(), "Serial") {
		t.Fatalf("the table must omit a serial Core does not publish, got:\n%s", out)
	}
}

func TestNodeStatusUnconfirmedCoreIdentityFailsClosed(t *testing.T) {
	nodeCoreFixture(t, targetenrollment.NodeStatus{})
	if err := operatorauth.ClearSession("local", "dev"); err != nil {
		t.Fatal(err)
	}
	// A configured connector node whose lifecycle authority cannot be reached
	// must fail closed: the CLI never renders a positive state it could not read.
	out, errOut := &strings.Builder{}, &strings.Builder{}
	if err := nodeStatus(context.Background(), []string{nodeTestTarget}, out, errOut); err == nil {
		t.Fatalf("an unreachable authority must not render a state: %s", out)
	}
	if out.Len() != 0 {
		t.Fatalf("a failed status must not emit partial output: %s", out)
	}
}

func TestNodeCapabilitiesDecodingHonoursSource(t *testing.T) {
	// No capabilities key -> ok=false, no fabrication.
	if _, ok := decodeCapabilitiesFromCoreResponse([]byte(`{"contract_version":"x"}`)); ok {
		t.Fatal("absent capabilities must decode as ok=false")
	}
	// Empty array -> no capabilities surfaced (ok=true, len 0): the honest
	// "Core returned no capability descriptors" verdict.
	if got, ok := decodeCapabilitiesFromCoreResponse([]byte(`{"capabilities":[]}`)); ok != true || len(got) != 0 {
		t.Fatalf("empty capabilities must decode as ok=true, nil, got %+v ok=%v", got, ok)
	}
	// Malformed -> ok=false.
	if _, ok := decodeCapabilitiesFromCoreResponse([]byte(`{"capabilities":"nope"}`)); ok {
		t.Fatal("malformed capabilities must decode as ok=false")
	}
	// Well-formed -> the advertised capability is surfaced verbatim.
	got, ok := decodeCapabilitiesFromCoreResponse([]byte(`{"capabilities":[{"name":"runtime.detect","available":true,"detail":"detects"}]}`))
	if !ok || len(got) != 1 || got[0].Name != "runtime.detect" || !got[0].Available {
		t.Fatalf("well-formed capabilities must surface exactly, got %+v ok=%v", got, ok)
	}
	// Blank name is structurally malformed -> ok=true with nil (no fabricated
	// capability entry; the response shape was valid but yielded nothing).
	if got, ok := decodeCapabilitiesFromCoreResponse([]byte(`{"capabilities":[{"name":"  "}]}`)); ok != true || len(got) != 0 {
		t.Fatalf("blank capability name must decode as ok=true, nil, got %+v ok=%v", got, ok)
	}
}

func TestNodeAddPersistsGrantExpiryAsRegistrationMetadataOnly(t *testing.T) {
	// The local registration file documents the Core-issued one-use grant it is
	// waiting on. That field must be named so it cannot be mistaken for
	// enrollment truth, and the registration must never carry a positive
	// enrollment or certificate-expiry claim.
	nodeCoreFixture(t, targetenrollment.NodeStatus{})
	bootstrap := requestNodeGrantThroughProductionAddPath(t, nil)
	record, err := loadNodeRecord(nodeTestTarget)
	if err != nil {
		t.Fatal(err)
	}
	record.GrantExpiresAt = bootstrap.ExpiresAt.UTC().Round(time.Second)
	if err := saveNodeRecord(record); err != nil {
		t.Fatal(err)
	}
	stored := readNodeRecordRaw(t, nodeTestTarget)
	value, ok := stored["grant_expires_at"]
	if !ok {
		t.Fatalf("the registration must document the pending grant: %v", stored)
	}
	parsed, err := time.Parse(time.RFC3339, value.(string))
	if err != nil || !parsed.Equal(bootstrap.ExpiresAt.UTC().Round(time.Second)) {
		t.Fatalf("grant_expires_at %v must round-trip the Core grant expiry %v", value, bootstrap.ExpiresAt)
	}
	if _, ok := stored["enrolled"]; ok {
		t.Fatalf("the local registration must not claim enrollment: %v", stored)
	}
	if _, ok := stored["expires_at"]; ok {
		t.Fatalf("the local registration must not claim a certificate expiry: %v", stored)
	}
}

func nodeTestScope() targetenrollment.Scope {
	return targetenrollment.Scope{TenantID: nodeTestTenantID, TargetID: nodeTestTarget, NodeID: nodeTestNode, Runtime: "podman"}
}
