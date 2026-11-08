package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestPinnedRemoteMachineDiscoveryRejectsForeignBindings(t *testing.T) {
	discovery := machine.MachineDiscovery()
	discovery.HTTP = machine.MachineHTTPBindings()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/machine/discovery" || r.Header.Get("Authorization") != "Bearer operator-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(discovery)
	}))
	defer server.Close()
	connection := &verifiedRemoteCore{
		client: server.Client(), origin: server.URL, token: "operator-token",
		target: "remote", tenant: "tenant-a",
	}
	if _, err := connection.discoverMachineOperations(context.Background()); err != nil {
		t.Fatalf("pinned machine discovery refused: %v", err)
	}
	original := discovery.HTTP["execute"]
	discovery.HTTP["execute"] = machine.HTTPBinding{Href: "https://foreign.example/executions", Method: "POST"}
	if _, err := connection.discoverMachineOperations(context.Background()); err == nil {
		t.Fatal("foreign machine execution endpoint accepted")
	}
	discovery.HTTP["execute"] = original
	discovery.ExecutionVersion = "unsupported"
	if _, err := connection.discoverMachineOperations(context.Background()); err == nil {
		t.Fatal("foreign machine execution contract accepted")
	}
}

func TestRemoteMachineDiscoveryRequiresVerifiedConnection(t *testing.T) {
	var connection verifiedRemoteCore
	if _, err := connection.discoverMachineOperations(context.Background()); err == nil {
		t.Fatal("machine discovery accepted an unverified connection")
	}
}
