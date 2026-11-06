package workstationlifecycle_test

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

	wsl "github.com/mcpdev80/baseharbor/contracts/workstationlifecycle/v1"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

const tenantID = "11111111-1111-4111-8111-111111111111"

// mockLifecycle records the calls the contract entrypoints make so the contract
// test can assert the fail-closed guards WITHOUT a real issuer. It captures the
// scope each call was bound to.
type mockLifecycle struct {
	renewCalls  []ws.Scope
	revokeCalls []ws.Scope
	scopeErr    error
}

func (m *mockLifecycle) Renew(ctx context.Context, request ws.Request) (ws.Result, error) {
	m.renewCalls = append(m.renewCalls, request.Scope())
	return ws.Result{}, nil
}

func (m *mockLifecycle) Revoke(ctx context.Context, scope ws.Scope) error {
	m.revokeCalls = append(m.revokeCalls, scope)
	return m.scopeErr
}

func (m *mockLifecycle) RevokedState(ctx context.Context, scope ws.Scope) (bool, error) {
	return false, m.scopeErr
}

func baselineRecord() wsl.Record {
	return wsl.Record{
		Kind:           wsl.RecordKindWorkstationLifecycle,
		Version:        wsl.Version,
		InstallationID: tenantID,
		Recipient:      "spiffe://baseharbor/workstations/" + tenantID + "/dev-laptop-01",
		Scope:          "dev-laptop-01",
	}
}

// POSITIVE: a valid record renews exactly the scope it names.
func TestRenewDeviceBoundToRecordScope(t *testing.T) {
	mock := &mockLifecycle{}
	rec := baselineRecord()
	scope := ws.Scope{TenantID: rec.InstallationID, Station: rec.Scope}
	_, err := wsl.RenewDevice(context.Background(), mock, rec, csrBytes(t, scope), 2*time.Hour)
	if err != nil {
		t.Fatalf("RenewDevice: %v", err)
	}
	if len(mock.renewCalls) != 1 || mock.renewCalls[0].Station != "dev-laptop-01" {
		t.Fatalf("RenewDevice must renew the record's scope; got %+v", mock.renewCalls)
	}
}

// ROTE 1: a foreign scope fails closed without calling the lifecycle.
func TestRenewDeviceForeignScopeRejected(t *testing.T) {
	mock := &mockLifecycle{}
	rec := baselineRecord()
	rec.Scope = "attacker-02"
	scope := ws.Scope{TenantID: rec.InstallationID, Station: rec.Scope}
	if _, err := wsl.RenewDevice(context.Background(), mock, rec, csrBytes(t, scope), 2*time.Hour); err == nil {
		t.Fatal("a foreign scope must fail closed")
	}
	if len(mock.renewCalls) != 0 {
		t.Fatal("a rejected record must not reach the lifecycle")
	}
}

// ROTE 2: revocation is bound to the record's scope and fails closed on a
// malformed record.
func TestRevokeDeviceBoundToRecordScope(t *testing.T) {
	mock := &mockLifecycle{}
	rec := baselineRecord()
	if err := wsl.RevokeDevice(context.Background(), mock, rec); err != nil {
		t.Fatalf("RevokeDevice: %v", err)
	}
	if len(mock.revokeCalls) != 1 || mock.revokeCalls[0].Station != "dev-laptop-01" {
		t.Fatalf("RevokeDevice must revoke the record's scope; got %+v", mock.revokeCalls)
	}
}

// A record with an unknown kind fails closed.
func TestRecordUnknownKindRejected(t *testing.T) {
	mock := &mockLifecycle{}
	rec := baselineRecord()
	rec.Kind = "workstation-enrollment" // not a lifecycle kind
	if err := wsl.RevokeDevice(context.Background(), mock, rec); err == nil {
		t.Fatal("an unknown kind must fail closed")
	}
}

// A nil lifecycle fails closed.
func TestNilLifecycleFailsClosed(t *testing.T) {
	if _, err := wsl.RenewDevice(context.Background(), nil, baselineRecord(), csrBytes(nil, ws.Scope{}), time.Hour); err == nil {
		t.Fatal("a nil lifecycle must fail closed")
	}
	if err := wsl.RevokeDevice(context.Background(), nil, baselineRecord()); err == nil {
		t.Fatal("a nil lifecycle must fail closed on revoke")
	}
}

func csrBytes(_ *testing.T, scope ws.Scope) []byte {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	uri, _ := url.Parse(scope.Identity())
	csr, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: scope.Identity()}, URIs: []*url.URL{uri}}, key)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})
}
