package runtimeexecutor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/objectstorage"
)

// mockResourceManager implements the runtime executor ResourceManager interface
// so the handler can be exercised without any cloud or object-storage backend.
type mockResourceManager struct {
	created []string
	got     []string
	deleted []string
}

func (m *mockResourceManager) Create(ctx context.Context, application, environment, name string) (objectstorage.RuntimeResourceBinding, error) {
	m.created = append(m.created, name)
	return objectstorage.RuntimeResourceBinding{ResourceID: "res-" + name, Bucket: "rt", Endpoint: "https://s3.example"}, nil
}

func (m *mockResourceManager) Get(ctx context.Context, application, environment, name string) (objectstorage.RuntimeResourceBinding, error) {
	m.got = append(m.got, name)
	return objectstorage.RuntimeResourceBinding{ResourceID: "res-" + name, Bucket: "rt", Endpoint: "https://s3.example"}, nil
}

func (m *mockResourceManager) Delete(ctx context.Context, application, environment, name string) error {
	m.deleted = append(m.deleted, name)
	return nil
}

func workloadCertURI(t *testing.T, app, environment string) *url.URL {
	t.Helper()
	u, err := url.Parse(fmt.Sprintf("spiffe://baseharbor/apps/%s/%s", app, environment))
	if err != nil {
		t.Fatalf("spiffe url: %v", err)
	}
	return u
}

func workloadCert(t *testing.T, app, environment string) *x509.Certificate {
	t.Helper()
	return &x509.Certificate{
		Subject: pkix.Name{CommonName: app},
		URIs:    []*url.URL{workloadCertURI(t, app, environment)},
	}
}

// doExecute runs a single execute request through the handler with the given
// peer certificate (nil peer means no client cert) and returns the status code
// plus the parsed JSON body.
func doExecute(t *testing.T, h *Handler, cert *x509.Certificate, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/internal/v1/execute", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:1234"
	if cert != nil {
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
			Version:          tls.VersionTLS12,
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var parsed map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec.Code, parsed
}

// no-cert requests must fail closed with 401 — no workload identity, no access.
func TestHandlerRejectsMissingClientCertificate(t *testing.T) {
	h, err := NewHandler(&mockResourceManager{})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	code, _ := doExecute(t, h, nil, `{"capability":"object-storage.s3/v1","operation":"runtime.create","name":"db"}`)
	if code != 401 {
		t.Fatalf("missing client cert: want 401, got %d", code)
	}
}

// a malformed SPIFFE identity must fail closed — the identity is present but does
// not resolve to app/environment.
func TestHandlerRejectsBadSpiffeIdentity(t *testing.T) {
	h, _ := NewHandler(&mockResourceManager{})
	bad := &x509.Certificate{
		URIs: []*url.URL{mustURL(t, "spiffe://other/apps/a/b")},
	}
	code, _ := doExecute(t, h, bad, `{"capability":"object-storage.s3/v1","operation":"runtime.create","name":"db"}`)
	if code != 401 {
		t.Fatalf("bad spiffe identity: want 401, got %d", code)
	}
}

// the happy path: a valid client certificate selects the workload identity and
// the operation executes against the resource manager.
func TestHandlerExecutesRuntimeCreate(t *testing.T) {
	mock := &mockResourceManager{}
	h, _ := NewHandler(mock)
	cert := workloadCert(t, "acme", "dev")
	code, parsed := doExecute(t, h, cert, `{"capability":"object-storage.s3/v1","operation":"runtime.create","name":"db"}`)
	if code != 200 {
		t.Fatalf("runtime.create: want 200, got %d body=%v", code, parsed)
	}
	if parsed["resource_id"] != "res-db" {
		t.Fatalf("unexpected resource id: %v", parsed["resource_id"])
	}
	if len(mock.created) != 1 || mock.created[0] != "db" {
		t.Fatalf("resource manager create not called for db, got %v", mock.created)
	}
}

// wrong capability must fail closed — the handler only routes object-storage.
func TestHandlerRejectsWrongCapability(t *testing.T) {
	mock := &mockResourceManager{}
	h, _ := NewHandler(mock)
	code, _ := doExecute(t, h, workloadCert(t, "acme", "dev"), `{"capability":"apps.serve/v1","operation":"run","name":"x"}`)
	if code != 422 {
		t.Fatalf("wrong capability: want 422, got %d", code)
	}
	if len(mock.created)+len(mock.got)+len(mock.deleted) != 0 {
		t.Fatalf("no resource operation should run for wrong capability")
	}
}

// unknown JSON fields must fail closed — the handler uses DisallowUnknownFields.
func TestHandlerRejectsUnknownFields(t *testing.T) {
	h, _ := NewHandler(&mockResourceManager{})
	code, _ := doExecute(t, h, workloadCert(t, "acme", "dev"), `{"capability":"object-storage.s3/v1","operation":"runtime.create","name":"db","extra":true}`)
	if code != 400 {
		t.Fatalf("unknown fields: want 400, got %d", code)
	}
}

// non-POST or wrong path must fail closed with 404.
func TestHandlerRejectsWrongMethod(t *testing.T) {
	h, _ := NewHandler(&mockResourceManager{})
	req := httptest.NewRequest("GET", "/internal/v1/execute", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{workloadCert(t, "acme", "dev")}}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("wrong method: want 404, got %d", rec.Code)
	}
}

// an empty capability/name is a validation failure, not a silent no-op.
func TestHandlerRejectsEmptyName(t *testing.T) {
	h, _ := NewHandler(&mockResourceManager{})
	code, _ := doExecute(t, h, workloadCert(t, "acme", "dev"), `{"capability":"object-storage.s3/v1","operation":"runtime.create","name":""}`)
	if code != 422 {
		t.Fatalf("empty name: want 422, got %d", code)
	}
}

// runtime.get returns the binding; runtime.delete removes it. Both must route.
func TestHandlerRoutesGetAndDelete(t *testing.T) {
	mock := &mockResourceManager{}
	h, _ := NewHandler(mock)
	cert := workloadCert(t, "acme", "dev")

	code, _ := doExecute(t, h, cert, `{"capability":"object-storage.s3/v1","operation":"runtime.get","name":"db"}`)
	if code != 200 {
		t.Fatalf("runtime.get: want 200, got %d", code)
	}
	if len(mock.got) != 1 {
		t.Fatalf("runtime.get not routed, got %v", mock.got)
	}

	code, _ = doExecute(t, h, cert, `{"capability":"object-storage.s3/v1","operation":"runtime.delete","name":"db"}`)
	if code != 200 {
		t.Fatalf("runtime.delete: want 200, got %d", code)
	}
	if len(mock.deleted) != 1 {
		t.Fatalf("runtime.delete not routed, got %v", mock.deleted)
	}
}

// NewHandler must fail closed when the resource manager is nil.
func TestNewHandlerRejectsNilManager(t *testing.T) {
	if _, err := NewHandler(nil); err == nil {
		t.Fatal("NewHandler(nil): expected a closed error, got nil")
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url %q: %v", raw, err)
	}
	return u
}
