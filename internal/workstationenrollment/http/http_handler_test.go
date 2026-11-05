package workstationenrollmenthttp_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
	wshttp "github.com/mcpdev80/baseharbor/internal/workstationenrollment/http"
)

const tenantID = "11111111-1111-4111-8111-111111111111"

// newHandler builds a Handler over a real one-use authority + trusted issuer.
func newHandler(t *testing.T) *wshttp.Handler {
	t.Helper()
	store := ws.NewMemoryStore()
	issuer := serviceissuer.New(t)
	authority, err := ws.New(store, issuer, nil)
	if err != nil {
		t.Fatal(err)
	}
	return wshttp.NewHandler(wshttpEnrollment{authority})
}

// wshttpEnrollment adapts the authority to the HTTP contract's Enrollment seam.
type wshttpEnrollment struct{ *ws.Authority }

func (e wshttpEnrollment) Create(ctx context.Context, scope ws.Scope, lifetime, certTTL time.Duration) (ws.Bootstrap, error) {
	return e.Authority.Create(ctx, scope, lifetime, certTTL)
}

func (e wshttpEnrollment) Enroll(ctx context.Context, request ws.Request) (ws.Result, error) {
	return e.Authority.Enroll(ctx, request)
}

func readBody(resp *http.Response) string {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return buf.String()
}

func newCSR(t *testing.T, id string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	uri, err := url.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: id}, URIs: []*url.URL{uri}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))
}

func TestHTTPBootstrapThenEnroll(t *testing.T) {
	handler := newHandler(t)
	mux := http.NewServeMux()
	handler.Router(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	scope := "dev-laptop-01"
	identity := "spiffe://baseharbor/workstations/" + tenantID + "/" + scope

	// Bootstrap: creates a one-use authorization.
	body, _ := json.Marshal(wshttp.BootstrapRequest{InstallationID: tenantID, Station: scope, CertificateTTLSeconds: 3600})
	resp, err := http.Post(server.URL+"/bootstrap", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap status = %d", resp.StatusCode)
	}
	var bs wshttp.BootstrapResponse
	if err := json.NewDecoder(resp.Body).Decode(&bs); err != nil {
		t.Fatal(err)
	}
	if bs.ExpiresAt.IsZero() {
		t.Fatal("bootstrap must carry a validity window")
	}

	// Enroll: consumes the one-use bootstrap and returns the registry fact.
	csr := newCSR(t, identity)
	reqBody, _ := json.Marshal(wshttp.EnrollRequest{InstallationID: tenantID, Station: scope, Token: bs.Token, Nonce: bs.Nonce, CSRPEM: csr})
	resp, err = http.Post(server.URL+"/enroll", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enroll status = %d, body=%s", resp.StatusCode, readBody(resp))
	}
	var fact wshttp.EnrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&fact); err != nil {
		t.Fatal(err)
	}
	if fact.Serial == "" || fact.LeafPEM == "" || fact.TrustPEM == "" {
		t.Fatalf("registry fact must carry leaf + trust: %+v", fact)
	}
	if fact.Station != scope || fact.InstallationID != tenantID {
		t.Fatalf("registry fact must record scope+installation: %+v", fact)
	}
	if fact.At.IsZero() {
		t.Fatal("registry fact must record the issuance time")
	}
	if !fact.ExpiresAt.After(time.Now()) {
		t.Fatal("issued leaf must be live (expiry in the future)")
	}

	// Second enroll with the same bootstrap must fail closed.
	resp, err = http.Post(server.URL+"/enroll", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("a consumed one-use bootstrap must fail closed on replay")
	}
}

func TestHTTPBootstrapRejectsInvalidStation(t *testing.T) {
	handler := newHandler(t)
	mux := http.NewServeMux()
	handler.Router(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	// An invalid station (a space breaks the single-label segment rule) fails.
	body, _ := json.Marshal(wshttp.BootstrapRequest{InstallationID: tenantID, Station: "bad station", CertificateTTLSeconds: 3600})
	resp, err := http.Post(server.URL+"/bootstrap", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("an invalid workstation scope must be rejected")
	}
}
