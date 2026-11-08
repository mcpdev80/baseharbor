package main

import (
	"bytes"
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

	"github.com/mcpdev80/baseharbor/internal/machine"
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
	var execution machine.Execution
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-id-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/management/installation":
			_ = json.NewEncoder(w).Encode(identity)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/machine/discovery":
			discovery := machine.MachineDiscovery()
			discovery.HTTP = machine.MachineHTTPBindings()
			_ = json.NewEncoder(w).Encode(discovery)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/machine/executions":
			var request struct {
				OperationID string                   `json:"operation_id"`
				Context     machine.OperationContext `json:"context"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil ||
				request.OperationID != "status" || request.Context.Target != node.Name ||
				request.Context.Environment != "dev" || request.Context.Application != "demo" ||
				r.Header.Get("Idempotency-Key") == "" {
				http.Error(w, "invalid status request", http.StatusBadRequest)
				return
			}
			execution = machine.Execution{
				ContractVersion: machine.ExecutionContractVersion,
				ExecutionID:     "remote-status-one",
				OperationID:     request.OperationID,
				Context:         request.Context,
				State:           machine.ExecutionSucceeded,
				Result:          json.RawMessage(`{"ready":true}`),
			}
			w.Header().Set("Location", "/api/v1/machine/executions/remote-status-one")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(machine.Execution{
				ContractVersion: machine.ExecutionContractVersion,
				ExecutionID:     execution.ExecutionID,
				OperationID:     execution.OperationID,
				Context:         execution.Context,
				State:           machine.ExecutionPending,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/machine/executions/remote-status-one":
			_ = json.NewEncoder(w).Encode(execution)
		default:
			http.NotFound(w, r)
		}
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
		return verifyRemoteManagementConnection(context.Background(), node.Name, node.RuntimeProvider, node.TenantID)
	}
	if err := verify(); err != nil {
		t.Fatalf("valid pinned remote Core rejected: %v", err)
	}

	var statusOutput bytes.Buffer
	if err := remoteApplicationStatus(context.Background(), node,
		[]string{"demo", "--environment", "dev"}, &statusOutput); err != nil {
		t.Fatalf("verified remote status CLI failed: %v", err)
	}
	var verifiedStatus struct {
		Ready bool `json:"ready"`
	}
	if err := json.Unmarshal(statusOutput.Bytes(), &verifiedStatus); err != nil || !verifiedStatus.Ready {
		t.Fatalf("remote status CLI returned no confirmed READY result: %q (%v)", statusOutput.String(), err)
	}

	for _, foreignTenant := range []string{"", "22222222-2222-4222-8222-222222222222"} {
		if err := verifyRemoteManagementConnection(context.Background(), node.Name, node.RuntimeProvider, foreignTenant); err == nil {
			t.Fatalf("remote Core accepted mismatched or empty configured Target tenant %q", foreignTenant)
		}
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
