package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"time"
)

// This test drives the production HTTPS enrollment HTTP endpoint, not merely
// Authority.Enroll, and rejects foreign scope and credential replay.
func TestNodeEnrollmentHTTPSOneUseAndForeignScope(t *testing.T) {
	authority := nodeCoreFixture(t, targetenrollment.NodeStatus{})
	grant := requestNodeGrantThroughProductionAddPath(t, authority)
	record := nodeCoreTarget(t)
	client, err := nodeHTTPSClient(record.CAFile)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	identityURL, err := url.Parse(nodeTestScope().Identity())
	if err != nil {
		t.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader,
		&x509.CertificateRequest{URIs: []*url.URL{identityURL}}, key)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]string{
		"contract_version": "baseharbor.target-access-enrollment/v1",
		"tenant_id": nodeTestTenantID,
		"node_id": nodeTestNode,
		"target_id": nodeTestTarget,
		"runtime": "podman",
		"csr_pem": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})),
		"nonce": grant.Nonce,
	}
	post := func(body map[string]string, token string) (int, []byte) {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
			record.CoreURL+targetenrollment.EnrollmentPath, bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, data
	}
	if status, _ := post(payload, ""); status != http.StatusUnauthorized {
		t.Fatalf("missing bootstrap token: HTTP %d, want 401", status)
	}
	foreign := make(map[string]string, len(payload))
	for k, v := range payload {
		foreign[k] = v
	}
	foreign["tenant_id"] = "22222222-2222-4222-8222-222222222222"
	if status, _ := post(foreign, grant.Token); status != http.StatusForbidden {
		t.Fatalf("foreign tenant: HTTP %d, want 403", status)
	}
	status, data := post(payload, grant.Token)
	if status != http.StatusOK {
		t.Fatalf("valid one-use HTTPS enrollment: HTTP %d: %s", status, data)
	}
	var issued struct {
		CertificatePEM string    `json:"certificate_pem"`
		NotAfter       time.Time `json:"not_after"`
	}
	if err := json.Unmarshal(data, &issued); err != nil {
		t.Fatal(err)
	}
	if len(issued.CertificatePEM) == 0 || !issued.NotAfter.After(time.Now()) {
		t.Fatal("successful enrollment did not issue a valid certificate")
	}
	state, ok := authority.redemption()
	if !ok || state.Revoked || !state.Enrolled {
		t.Fatalf("Core lifecycle did not persist enrollment: %+v", state)
	}
	if status, _ := post(payload, grant.Token); status != http.StatusForbidden {
		t.Fatalf("spent enrollment token replay: HTTP %d, want 403", status)
	}
}
