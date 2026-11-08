package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func TestRemoteCoreHandshakePinsInstallationIssuerTenantAndCA(t *testing.T) {
	_, _, node := remoteApplicationCoreFixtureEmpty(t)
	const installation = "11111111-1111-4111-8111-111111111111"
	const operatorIssuer = "https://oidc.example/realms/operators"
	const coreIssuer = "https://identity.example/realms/baseharbor"

	identity := struct {
		Version        string `json:"version"`
		InstallationID string `json:"installation_id"`
		IdentityIssuer string `json:"identity_issuer"`
		OperatorIssuer string `json:"operator_issuer"`
		TenantID       string `json:"tenant_id"`
	}{managementInstallationVersion, installation, coreIssuer, operatorIssuer, node.TenantID}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/management/installation" || r.Header.Get("Authorization") != "Bearer test-id-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(identity)
	}))
	defer server.Close()
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	digest := sha256.Sum256(cert)
	profile := remoteConnectionProfile{Version: remoteConnectionProfileVersion, Name: "external-core"}
	profile.Core.InstallationID = installation
	profile.Core.URL = server.URL
	profile.Trust.CAPEM = string(cert)
	profile.Trust.SHA256 = hex.EncodeToString(digest[:])
	profile.Targets = append(profile.Targets, struct {
		Name            string `json:"name"`
		TenantID        string `json:"tenant_id"`
		RuntimeProvider string `json:"runtime_provider"`
		Scope           string `json:"scope,omitempty"`
	}{Name: node.Name, TenantID: node.TenantID, RuntimeProvider: node.RuntimeProvider})
	profile.Access = &struct {
		Issuer   string   `json:"issuer"`
		ClientID string   `json:"client_id"`
		Scopes   []string `json:"scopes"`
	}{Issuer: operatorIssuer, ClientID: "baha-cli"}
	if err := operatorauth.SaveSession(operatorauth.Session{
		Target: node.Name, Environment: "dev", Issuer: operatorIssuer,
		Subject: "operator", ClientID: "baha-cli", IDToken: "test-id-token",
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "connection.json")
	saveProfile := func() {
		t.Helper()
		data, err := json.Marshal(profile)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	saveProfile()
	t.Setenv("BASEHARBOR_CONNECTION_PROFILE", path)
	verify := func() error {
		return verifyRemoteManagementConnection(context.Background(), node.Name, node.RuntimeProvider)
	}
	if err := verify(); err != nil {
		t.Fatalf("valid pinned remote Core rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func()
	}{
		{"foreign-installation", func() { profile.Core.InstallationID = "22222222-2222-4222-8222-222222222222" }},
		{"foreign-tenant", func() { profile.Targets[0].TenantID = "22222222-2222-4222-8222-222222222222" }},
		{"wrong-pin", func() { profile.Trust.SHA256 = strings.Repeat("0", 64) }},
		{"wrong-runtime", func() { profile.Targets[0].RuntimeProvider = "docker" }},
		{"wrong-operator-issuer", func() { profile.Access.Issuer = "https://foreign.example" }},
		{"expired-profile", func() { expired := time.Now().Add(-time.Minute); profile.ExpiresAt = &expired }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			saved := profile
			targets := append([]struct {
				Name            string `json:"name"`
				TenantID        string `json:"tenant_id"`
				RuntimeProvider string `json:"runtime_provider"`
				Scope           string `json:"scope,omitempty"`
			}(nil), profile.Targets...)
			saved.Targets = targets
			test.mutate()
			saveProfile()
			if err := verify(); err == nil {
				t.Fatalf("%s escaped remote connection verification", test.name)
			}
			profile = saved
			saveProfile()
		})
	}
}
