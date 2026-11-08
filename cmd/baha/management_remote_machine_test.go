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

func TestPinnedRemoteMachineExecutionKeepsAuthorityAndIdempotency(t *testing.T) {
	discovery := machine.MachineDiscovery()
	discovery.HTTP = machine.MachineHTTPBindings()
	var postCount int
	forgedLocation := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer operator-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/machine/discovery":
			_ = json.NewEncoder(w).Encode(discovery)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/machine/executions":
			var request struct {
				OperationID string                   `json:"operation_id"`
				Context     machine.OperationContext `json:"context"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil ||
				r.Header.Get("Idempotency-Key") != "stable-req-one" ||
				request.OperationID != "status" || request.Context.Target != "remote" ||
				request.Context.Environment != "dev" || request.Context.Application != "demo" {
				http.Error(w, "bad machine request", http.StatusBadRequest)
				return
			}
			postCount++
			location := "/api/v1/machine/executions/execution-one"
			if forgedLocation {
				location = "https://foreign.example/steal"
			}
			w.Header().Set("Location", location)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(machine.Execution{
				ContractVersion: machine.ExecutionContractVersion,
				ExecutionID: "execution-one",
				OperationID: "status",
				Context: request.Context,
				State: machine.ExecutionPending,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	connection := &verifiedRemoteCore{
		client: server.Client(), origin: server.URL, token: "operator-token",
		target: "remote", tenant: "tenant-a",
	}
	selectors := machine.OperationContext{Application: "demo", Environment: "dev", Target: "remote"}
	if _, err := connection.requestMachineExecution(context.Background(), "status", selectors, nil, "stable-req-one"); err != nil {
		t.Fatalf("pinned machine operation rejected: %v", err)
	}
	if postCount != 1 {
		t.Fatalf("expected one admitted operation, got %d", postCount)
	}
	selectors.Target = "foreign"
	if _, err := connection.requestMachineExecution(context.Background(), "status", selectors, nil, "stable-req-one"); err == nil {
		t.Fatal("foreign Target submitted a remote machine operation")
	}
	if postCount != 1 {
		t.Fatal("foreign Target reached remote execution endpoint")
	}
	selectors.Target = "remote"
	forgedLocation = true
	if _, err := connection.requestMachineExecution(context.Background(), "status", selectors, nil, "stable-req-one"); err == nil {
		t.Fatal("foreign execution Location was trusted")
	}
}
