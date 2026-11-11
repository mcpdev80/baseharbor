package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func TestRemoteCoreHandshakePinsInstallationIssuerTenantAndCA(t *testing.T) {
	ctx, _, node := remoteApplicationCoreFixtureEmpty(t)
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
	foreignResult := false
	doctorReady := true
	repairCalled := false
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
			needsApplication := func(operation string) bool {
				return operation == "status" || operation == "plan" || operation == "doctor" || operation == "app.preflight" || operation == "apply" || operation == "update" || operation == "destroy" || operation == "app.stop" || operation == "app.create"
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil ||
				(request.OperationID != "status" && request.OperationID != "plan" && request.OperationID != "doctor" && request.OperationID != "app.preflight" && request.OperationID != "app.list" && request.OperationID != "control-plane.status" && request.OperationID != "control-plane.doctor" && request.OperationID != "control-plane.repair" && request.OperationID != "apply" && request.OperationID != "update" && request.OperationID != "destroy" && request.OperationID != "app.stop" && request.OperationID != "app.create") || request.Context.Target != node.Name ||
				request.Context.Environment != "dev" || (needsApplication(request.OperationID) && request.Context.Application != "demo") ||
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
				Result:          json.RawMessage(`{"contract_version":"v1","target":"` + node.Name + `","application":"demo","environment":"dev","state":"running","ready":true,"checks":[]}`),
			}
			if request.OperationID != "status" {
				execution.Result = json.RawMessage(`{"contract_version":"v1","operation":"` + request.OperationID + `","target":"` + node.Name + `","application":"demo","environment":"dev","state":"running","verified":true,"status":{"contract_version":"v1","target":"` + node.Name + `","application":"demo","environment":"dev","state":"running","ready":true,"checks":[]}}`)
				if request.OperationID == "app.create" {
					execution.Result = json.RawMessage(`{"application":"demo","application_id":"33333333-3333-4333-8333-333333333333","environment":"dev","manifest":"/srv/baseharbor/demo/baseharbor.yaml"}`)
				}
				if request.OperationID == "destroy" {
					execution.Result = json.RawMessage(`{"contract_version":"v1","operation":"destroy","target":"` + node.Name + `","application":"demo","environment":"dev","state":"destroyed","verified":true}`)
				}
				if request.OperationID == "app.stop" {
					execution.Result = json.RawMessage(`{"application":"demo","environment":"dev","state":"stopped","data_preserved":true}`)
				}
				if request.OperationID == "plan" {
					execution.Result = json.RawMessage(`{"contract_version":"v1","application":"demo","environment":"dev","actions":[],"footprint":{},"provider_plan":{}}`)
				}
				if request.OperationID == "doctor" {
					execution.Result = json.RawMessage(`{"contract_version":"v1","target":"` + node.Name + `","application":"demo","environment":"dev","state":"running","healthy":true,"checks":[]}`)
				}
				if request.OperationID == "app.list" {
					execution.Result = json.RawMessage(`{"contract_version":"v1","deployments":[]}`)
				}
				if request.OperationID == "app.preflight" {
					execution.Result = json.RawMessage(`{"contract_version":"v1","application":"demo","environment":"dev","passed":true,"checks":[]}`)
				}
				if request.OperationID == "control-plane.status" {
					execution.Result = json.RawMessage(`{"target":"` + node.Name + `","state":"running","ready":true,"running":["core"],"checks":[],"availability_satisfied":true}`)
				}
				if request.OperationID == "control-plane.doctor" {
					ready := "false"
					if doctorReady {
						ready = "true"
					}
					execution.Result = json.RawMessage(`{"ready":` + ready + `,"checks":[{"name":"target-access","ready":` + ready + `}]}`)
				}
				if request.OperationID == "control-plane.repair" {
					repairCalled = true
					doctorReady = true
					execution.Result = json.RawMessage(`{"target":"` + node.Name + `","state":"running","ready":true,"running":["core"],"checks":[],"availability_satisfied":true}`)
				}
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
			result := execution
			if foreignResult {
				result.Result = json.RawMessage(`{"contract_version":"v1","target":"foreign","application":"demo","environment":"dev","state":"running","ready":true,"checks":[]}`)
			}
			_ = json.NewEncoder(w).Encode(result)
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
	for _, operation := range []string{"plan", "doctor", "app.preflight", "app.list", "control-plane.status", "control-plane.doctor"} {
		t.Run("verified-remote-read-"+operation, func(t *testing.T) {
			applicationName := "demo"
			var input any
			if operation == "app.list" || operation == "control-plane.status" || operation == "control-plane.doctor" {
				applicationName = ""
			}
			if operation == "app.list" {
				input = machineApplicationListInput{Target: node.Name}
			}
			result, err := remoteMachineRead(context.Background(), node, operation, applicationName, "dev", input)
			if err != nil || !json.Valid(result) {
				t.Fatalf("remote %s did not return a valid Core result: %s (%v)", operation, result, err)
			}
		})
	}
	if report, err := remoteControlPlaneStatus(context.Background(), node); err != nil || report.Target != node.Name || !report.Ready {
		t.Fatalf("remote control-plane status did not verify the selected Target: %#v (%v)", report, err)
	}
	if report, err := remoteControlPlaneDoctor(context.Background(), node); err != nil || !report.Ready {
		t.Fatalf("remote control-plane doctor did not verify readiness: %#v (%v)", report, err)
	}
	if err := configureCoreRemoteTargetForTest(node.Name); err != nil {
		t.Fatal(err)
	}
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })
	t.Setenv("DOCKER_HOST", "unix:///tmp/no-local-docker-for-remote-doctor.sock")
	root := rootCommand()
	for _, commandName := range []string{"status", "doctor"} {
		var command *cli.Command
		for _, child := range root.Children {
			if child.Name == commandName {
				command = child
				break
			}
		}
		if command == nil {
			t.Fatalf("root command %q is missing", commandName)
		}
		t.Run("root-remote-core-"+commandName, func(t *testing.T) {
			var output, stderr bytes.Buffer
			if err := command.Run(withTargetOverride(ctx, node.Name), nil, &output, &stderr); err != nil {
				t.Fatalf("remote root %s failed: %v; output=%s stderr=%s", commandName, err, output.String(), stderr.String())
			}
			if commandName == "status" && (!strings.Contains(output.String(), "Control Plane") || !strings.Contains(output.String(), "registered deployment(s)")) {
				t.Fatalf("remote root status did not render the Core observation: %s", output.String())
			}
			if commandName == "doctor" && !strings.Contains(output.String(), "READY") {
				t.Fatalf("remote root doctor did not render the Core observation: %s", output.String())
			}
			output.Reset()
			if err := command.Run(withTargetOverride(ctx, node.Name), []string{"-o", "json"}, &output, &stderr); err != nil || !json.Valid(output.Bytes()) {
				t.Fatalf("remote root %s JSON did not return the Core observation: %s (%v)", commandName, output.String(), err)
			}
		})
	}
	var remoteDoctor *cli.Command
	for _, child := range root.Children {
		if child.Name == "doctor" {
			remoteDoctor = child
			break
		}
	}
	doctorReady = false
	var repairedOutput, repairedStderr bytes.Buffer
	if remoteDoctor == nil || remoteDoctor.Run(withTargetOverride(ctx, node.Name), []string{"--fix", "--yes"}, &repairedOutput, &repairedStderr) != nil || !repairCalled || !strings.Contains(repairedOutput.String(), "READY") {
		t.Fatalf("remote doctor --fix did not repair and recheck the Core: output=%s stderr=%s", repairedOutput.String(), repairedStderr.String())
	}

	created, createErr := remoteCreateTargetManagedApplication(context.Background(), node,
		application.Manifest{Name: "demo", Environment: "dev"})
	if createErr != nil || created.Application != "demo" || created.ApplicationID == "" {
		t.Fatalf("remote app create did not verify authoritative Core receipt: %#v %v", created, createErr)
	}

	foreignResult = true
	statusOutput.Reset()
	if err := remoteApplicationStatus(context.Background(), node,
		[]string{"demo", "--environment", "dev"}, &statusOutput); err == nil {
		t.Fatal("foreign application status was accepted under the pinned execution identity")
	}
	if statusOutput.Len() != 0 {
		t.Fatalf("foreign application status leaked into CLI output: %q", statusOutput.String())
	}
	foreignResult = false

	for _, scenario := range []struct {
		operation string
		args      []string
	}{
		{"apply", []string{"demo"}},
		{"update", []string{"demo", "--no-backup"}},
		{"destroy", []string{"demo", "--yes"}},
		{"app.stop", []string{"demo"}},
	} {
		t.Run("verified-remote-"+scenario.operation, func(t *testing.T) {
			var output bytes.Buffer
			if err := remoteApplicationMutation(context.Background(), node, scenario.operation, scenario.args, &output); err != nil {
				t.Fatalf("remote %s rejected verified machine execution: %v", scenario.operation, err)
			}
			var result struct {
				Operation string `json:"operation"`
				Target    string `json:"target"`
				State     string `json:"state"`
			}
			if err := json.Unmarshal(output.Bytes(), &result); err != nil ||
				(scenario.operation != "app.stop" && (result.Operation != scenario.operation || result.Target != node.Name)) ||
				(scenario.operation == "app.stop" && result.State != "stopped") {
				t.Fatalf("remote %s returned incorrect completion: %q (%v)", scenario.operation, output.String(), err)
			}
		})
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

func configureCoreRemoteTargetForTest(name string) error {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	definition, ok := cfg.Targets[name]
	if !ok {
		return fmt.Errorf("test target %q is missing", name)
	}
	const access = "remote-core-profile"
	cfg.Access[access] = deployment.AccessDefinition{Provider: "baseharbor-core", Reference: "test-profile"}
	definition.Access = deployment.TargetAccess{Reference: access}
	cfg.Targets[name] = definition
	return cfg.Save()
}
