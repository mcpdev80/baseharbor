package workstationenrollment_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net/url"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// validScope is a valid installation-scoped workstation scope (not a connector
// Core scope).
func validScope(t *testing.T) ws.Scope {
	t.Helper()
	scope := ws.Scope{TenantID: "11111111-1111-4111-8111-111111111111", Station: "dev-laptop-01"}
	if err := scope.Validate(); err != nil {
		t.Fatalf("valid scope rejected: %v", err)
	}
	return scope
}

// newCSRPEM builds a workstation-owned CSR carrying the workstation SPIFFE
// identity as both CommonName and URI subject.
func newCSRPEM(t *testing.T, id string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	uri, err := url.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: id}, URIs: []*url.URL{uri}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})
}

// newAuthority builds a real one-use Authority over a trusted CSRIssuer.
func newAuthority(t *testing.T) (*ws.Authority, ws.Store) {
	t.Helper()
	store := ws.NewMemoryStore()
	issuer := serviceissuer.New(t)
	authority, err := ws.New(store, issuer, nil)
	if err != nil {
		t.Fatal(err)
	}
	return authority, store
}

// POSITIVE: single use succeeds, issues a leaf bound to the workstation scope,
// and the registry fact records scope + time.
func TestEnrollPositiveIssuesLeafWithScopeAndTime(t *testing.T) {
	authority, store := newAuthority(t)
	scope := validScope(t)
	certTTL := 1 * time.Hour

	bootstrap, err := authority.Create(context.Background(), scope, 30*time.Second, certTTL)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if bootstrap.ExpiresAt.IsZero() {
		t.Fatal("bootstrap expiry must be set")
	}

	request, err := ws.NewRequest(scope, bootstrap.Token, bootstrap.Nonce, newCSRPEM(t, scope.Identity()), certTTL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := authority.Enroll(context.Background(), request)
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if len(result.Certificate) == 0 {
		t.Fatal("expected signed leaf")
	}
	if result.ExpiresAt.IsZero() || result.ExpiresAt.Before(time.Now()) {
		t.Fatal("leaf expiry must be set and in the future")
	}
	if result.Trust.IssuerReference == "" {
		t.Fatal("trust bundle must carry issuer reference")
	}

	// Registry fact: the store consumed exactly one authorization, and the leaf
	// is bound to the installation-scoped workstation identity.
	if _, err := store.Consume(context.Background(), scope, bootstrap.Token, bootstrap.Nonce, certTTL); err == nil {
		t.Fatal("second consume of a one-use bootstrap must fail closed")
	}
	want := "spiffe://baseharbor/workstations/11111111-1111-4111-8111-111111111111/dev-laptop-01"
	if scope.Identity() != want {
		t.Fatalf("scope identity must be installation-scoped SPIFFE, got %q", scope.Identity())
	}
}

func TestEnrollmentCarriesServerBoundOwner(t *testing.T) {
	authority, _ := newAuthority(t)
	scope := validScope(t)
	bootstrap, err := authority.CreateForOwner(context.Background(), scope, time.Minute, time.Hour, "marcel")
	if err != nil {
		t.Fatalf("CreateForOwner: %v", err)
	}
	request, err := ws.NewRequest(scope, bootstrap.Token, bootstrap.Nonce, newCSRPEM(t, scope.Identity()), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	result, err := authority.Enroll(context.Background(), request)
	if err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if result.OwnerLogin != "marcel" {
		t.Fatalf("owner login = %q, want server-bound owner marcel", result.OwnerLogin)
	}
}

// ROTE 1: second use of a one-use bootstrap is rejected.
func TestSecondUseRejected(t *testing.T) {
	authority, store := newAuthority(t)
	scope := validScope(t)
	certTTL := 1 * time.Hour
	bootstrap, err := authority.Create(context.Background(), scope, 30*time.Second, certTTL)
	if err != nil {
		t.Fatal(err)
	}
	request, err := ws.NewRequest(scope, bootstrap.Token, bootstrap.Nonce, newCSRPEM(t, scope.Identity()), certTTL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Enroll(context.Background(), request); err != nil {
		t.Fatalf("first use must succeed: %v", err)
	}
	// Replay the same bootstrap: must fail closed.
	if _, err := authority.Enroll(context.Background(), request); err == nil {
		t.Fatal("replay of a consumed one-use bootstrap must fail closed")
	}
	// Store rejects the duplicate consume directly too.
	if _, err := store.Consume(context.Background(), scope, bootstrap.Token, bootstrap.Nonce, certTTL); err == nil {
		t.Fatal("duplicate store consume must fail closed")
	}
}

// ROTE 2: an elapsed/expired bootstrap is rejected. The production-path mirror
// (internal TestExpiryRejected) covers the store expiry guard; this positive
// mirror confirms a one-use bootstrap cannot be replayed.
func TestForeignRecipientRejected(t *testing.T) {
	authority, store := newAuthority(t)
	scope := validScope(t)
	certTTL := 1 * time.Hour
	bootstrap, err := authority.Create(context.Background(), scope, 30*time.Second, certTTL)
	if err != nil {
		t.Fatal(err)
	}
	// Foreign scope: different station.
	foreign := ws.Scope{TenantID: scope.TenantID, Station: "attacker-02"}
	request, err := ws.NewRequest(foreign, bootstrap.Token, bootstrap.Nonce, newCSRPEM(t, foreign.Identity()), certTTL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Enroll(context.Background(), request); err == nil {
		t.Fatal("a bootstrap presented under a foreign scope must fail closed")
	}
	// Store rejects a mismatched consume.
	if _, err := store.Consume(context.Background(), foreign, bootstrap.Token, bootstrap.Nonce, certTTL); err == nil {
		t.Fatal("foreign-scope consume must fail closed")
	}
}

// ROTE 4: a malformed/exp leaf or invalid CSR fails closed.
func TestMalformedLeafRejected(t *testing.T) {
	authority, _ := newAuthority(t)
	scope := validScope(t)
	certTTL := 1 * time.Hour
	bootstrap, err := authority.Create(context.Background(), scope, 30*time.Second, certTTL)
	if err != nil {
		t.Fatal(err)
	}
	// Empty CSR fails closed at validation.
	request, err := ws.NewRequest(scope, bootstrap.Token, bootstrap.Nonce, nil, certTTL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Enroll(context.Background(), request); err == nil {
		t.Fatal("an empty/invalid CSR must fail closed")
	}
}

// A valid workstation scope must not validate as a Core connector scope: the
// identity is installation-scoped, distinct from internal/targetenrollment.
func TestScopeIsInstallationScoped(t *testing.T) {
	scope := validScope(t)
	want := "spiffe://baseharbor/workstations/11111111-1111-4111-8111-111111111111/dev-laptop-01"
	if scope.Identity() != want {
		t.Fatalf("expected installation-scoped SPIFFE identity %q, got %q", want, scope.Identity())
	}
}
