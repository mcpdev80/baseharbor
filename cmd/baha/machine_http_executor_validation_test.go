package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/identity"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
)

func TestHTTPAdvertisedReadOperationsReachInputValidation(t *testing.T) {
	executor := &bahaMachineExecutor{}
	for _, id := range executor.SupportedOperationIDs() {
		t.Run(id, func(t *testing.T) {
			_, err := executor.Execute(context.Background(), machine.Operation{ID: id}, machine.OperationContext{}, json.RawMessage(`{"SECRET-CREDENTIAL":"must-not-echo"}`), nil)
			if err == nil {
				t.Fatal("unexpected input accepted")
			}
			typed, ok := err.(*machine.Error)
			if !ok || typed.Code != machine.ErrorValidationFailed {
				t.Fatalf("advertised HTTP operation did not reach input validation: %v", err)
			}
			if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "must-not-echo") {
				t.Fatal("input credential echoed")
			}
		})
	}
}

func TestHTTPInputRejectsUnknownNullTrailingAndOversizedData(t *testing.T) {
	for _, raw := range []string{`{"target":"safe","SECRET-CREDENTIAL":"hidden"}`, "null", `{"target":"safe"}{"target":"foreign"}`, strings.Repeat(" ", (1<<20)+1)} {
		var input machineRuntimeTargetInput
		if err := decodeHTTPInput(json.RawMessage(raw), &input); err == nil {
			t.Fatal("invalid input accepted")
		} else if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "hidden") {
			t.Fatal("secret echoed")
		}
	}
	var input machineRuntimeTargetInput
	if err := decodeHTTPInput(json.RawMessage(`{"target":"safe","environment":"dev"}`), &input); err != nil || input.Target != "safe" || input.Environment != "dev" {
		t.Fatalf("valid input rejected: %+v %v", input, err)
	}
}

func TestHTTPActualCoreExecutorStartsAndAdvertisesCanonicalOperations(t *testing.T) {
	executor := newBahaMachineExecutor(application.Store{Root: t.TempDir()})
	handler, err := machinehttp.New(executor)
	if err != nil {
		t.Fatal("actual Core HTTP executor cannot start", err)
	}
	request := httptest.NewRequest(http.MethodGet, "https://core.example/api/v1/machine/discovery", nil)
	request.TLS = &tls.ConnectionState{}
	request = request.WithContext(identity.WithPrincipal(request.Context(), &identity.Principal{Issuer: "https://issuer.example", Subject: "startup-operator"}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var discovery machine.Discovery
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &discovery) != nil {
		t.Fatal("actual Core discovery failed", response.Code)
	}
	supported := executor.SupportedOperationIDs()
	if len(discovery.Operations) == 0 || len(discovery.Operations) != len(supported) {
		t.Fatal("actual Core HTTP support and discovery differ")
	}
	seen := map[string]bool{}
	for _, operation := range discovery.Operations {
		canonical, exists := machine.OperationByID(operation.ID)
		if !exists || canonical != operation || seen[operation.ID] {
			t.Fatal("actual Core HTTP operation differs from canonical metadata", operation.ID)
		}
		seen[operation.ID] = true
	}
	for _, id := range supported {
		if !seen[id] {
			t.Fatal("supported operation missing from real discovery", id)
		}
	}
}
