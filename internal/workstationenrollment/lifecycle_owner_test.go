package workstationenrollment_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// memoryOwner is the in-memory LifecycleOwner for tests. It anchors the current
// issued leaf serial for each scope the production path has recorded. Renew and
// Revoke operate on exactly the serial it holds for a scope; a scope with no
// recorded serial fails closed, so a lifecycle operation on a workstation that
// never enrolled cannot touch anything.
type memoryOwner struct {
	mu     sync.Mutex
	issued map[ws.Scope]string
}

// newMemoryOwner returns an empty in-memory lifecycle owner.
func newMemoryOwner() *memoryOwner {
	return &memoryOwner{issued: map[ws.Scope]string{}}
}

// RecordEnrolled installs the currently issued serial for a scope: the effect of
// a completed enrollment in the durable store, which the production seam reads
// back through CurrentSerial.
func (m *memoryOwner) RecordEnrolled(scope ws.Scope, serial string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.issued[scope] = serial
}

// CurrentSerial returns the recorded issued serial for a scope. ok is false and
// err != nil when no serial was recorded for the scope (a scope that never
// enrolled fails closed).
func (m *memoryOwner) CurrentSerial(_ context.Context, scope ws.Scope) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	serial, ok := m.issued[scope]
	if !ok {
		return "", false, ws.ErrDenied
	}
	return serial, true, nil
}

// crlIssuer is a serviceaccess.CSRIssuer for the security cell: a single real
// serviceaccess.LocalIssuer that both signs CSRs and maintains the CRL, so
// Revoke truly invalidates a serial.
type crlIssuer struct{ local *serviceaccess.LocalIssuer }

// newCRLIssuer builds a crlIssuer over a freshly initialized LocalIssuer.
func newCRLIssuer(t *testing.T) *crlIssuer {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "pki")
	local, err := serviceaccess.EnsureLocalIssuer(dir)
	if err != nil {
		t.Fatal(err)
	}
	return &crlIssuer{local: local}
}

// newCRLAuthority builds a one-use Authority over a real CRL-backed CSRIssuer.
func newCRLAuthority(t *testing.T, crl *crlIssuer, scope ws.Scope) (*ws.Authority, *memoryOwner) {
	t.Helper()
	store := ws.NewMemoryStore()
	owner := newMemoryOwner()
	authority, err := ws.New(store, crl, owner)
	if err != nil {
		t.Fatal(err)
	}
	return authority, owner
}

// SignCSR signs a workstation-owned CSR through the real local signing boundary.
func (c *crlIssuer) SignCSR(ctx context.Context, r serviceaccess.CSRSigningRequest) (serviceaccess.IssuedCertificate, error) {
	return c.local.SignCSR(ctx, r)
}

func (c *crlIssuer) TrustBundle(ctx context.Context) (serviceaccess.TrustBundle, error) {
	return c.local.TrustBundle(ctx)
}

func (c *crlIssuer) Issue(ctx context.Context, r serviceaccess.CertificateRequest) (serviceaccess.IssuedCertificate, error) {
	return c.local.Issue(ctx, r)
}

func (c *crlIssuer) Renew(ctx context.Context, current serviceaccess.IssuedCertificate, r serviceaccess.CertificateRequest) (serviceaccess.IssuedCertificate, error) {
	return c.local.Renew(ctx, current, r)
}

func (c *crlIssuer) Revoke(ctx context.Context, serial string) error {
	return c.local.Revoke(ctx, serial)
}

func (c *crlIssuer) Revoked(ctx context.Context, serial string) bool {
	return c.local.Revoked(ctx, serial)
}

func (c *crlIssuer) RevokedStatus(ctx context.Context, serial string) (bool, error) {
	return c.local.RevokedStatus(ctx, serial)
}

func (c *crlIssuer) Status(ctx context.Context) (serviceaccess.IssuerStatus, error) {
	return c.local.Status(ctx)
}

func (c *crlIssuer) BaseSerial(ctx context.Context) (string, error) {
	return c.local.BaseSerial(ctx)
}

func (c *crlIssuer) BaseExpiry(ctx context.Context) (time.Time, error) {
	return c.local.BaseExpiry(ctx)
}
