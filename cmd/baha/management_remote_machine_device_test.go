package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDeviceOnlyRemoteMachineDiscoveryMakesRequest(t *testing.T) {
	var called atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/machine/discovery" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("device-only remote machine request must not send an OIDC bearer")
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	core := &verifiedRemoteCore{
		client: server.Client(), origin: server.URL,
		target: "core-test", tenant: "11111111-1111-4111-8111-111111111111",
	}
	_, err := core.discoverMachineOperations(context.Background())
	if !called.Load() || err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("device-only machine request was rejected before reaching Core: called=%v err=%v", called.Load(), err)
	}
}

func TestDeviceOnlySourceUploadMakesRequest(t *testing.T) {
	var called atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/machine/source-transfers" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("mTLS device request unexpectedly sent an OIDC bearer")
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	core := &verifiedRemoteCore{
		client: server.Client(), origin: server.URL,
		target: "core-test", tenant: "11111111-1111-4111-8111-111111111111",
	}
	_, err := core.uploadLocalSource(context.Background(), "core-test", "demo", "dev", t.TempDir())
	if !called.Load() || err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("device-only upload was rejected before reaching Core: called=%v err=%v", called.Load(), err)
	}
}
