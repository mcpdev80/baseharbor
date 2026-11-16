package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func coreStopFixture(t *testing.T) bhruntime.Files {
	t.Helper()
	target := configureTestTarget(t)
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	files, err := bhruntime.EnsureFilesForProjectAndResources(root, targetRuntimeProjectName(target), bhruntime.SharedResourceProjectName(target.Name), bhruntime.Ports{Postgres: 5432, OpenBao: 8200}, false)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestCoreStopAuthorizesEveryManagedConsumerBeforeAcknowledgement(t *testing.T) {
	for _, source := range []string{"deployment", "binding", "unbound-owner", "management"} {
		t.Run(source, func(t *testing.T) {
			files := coreStopFixture(t)
			t.Setenv(operatorauth.EnvIssuer, "")
			t.Setenv(operatorauth.EnvClientID, "")
			target, err := effectiveTarget(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			root, err := targetDataRoot(target)
			if err != nil {
				t.Fatal(err)
			}
			m := application.New("protected-consumer", "prod", true, false, false)
			m.ApplicationID = application.MustNewApplicationID()
			switch source {
			case "deployment":
				registerTestDeployment(t, target, m, t.TempDir(), "baseharbor.yaml")
			case "binding", "unbound-owner":
				if err := application.ReconcileReferenceProviderRegistryAt(root, m); err != nil {
					t.Fatal(err)
				}
				if source == "unbound-owner" {
					store := capability.RegistryStore{Path: filepath.Join(root, "provider-registry.json")}
					registry, err := store.Load()
					if err != nil {
						t.Fatal(err)
					}
					registry.Bindings = nil
					for index := range registry.Instances {
						registry.Instances[index].Scope = capability.ScopeApplication
						registry.Instances[index].SharingBoundary = ""
						registry.Instances[index].OwnerApplicationID = m.ApplicationID
						registry.Instances[index].OwnerApplication = m.Name
						registry.Instances[index].OwnerEnvironment = m.Environment
					}
					if err := store.Save(registry); err != nil {
						t.Fatal(err)
					}
				}
			case "management":
				spec := coreinstallation.Spec{Target: target.Name, Runtime: target.RuntimeProvider, MachineRole: coreinstallation.Deployment}
				state := coreinstallation.State{Version: "baseharbor.core-installation/v1", ID: m.ApplicationID, Owner: "baseharbor", Spec: spec, Defaults: spec.MachineRole.Defaults(), Placement: "shared", Phase: "preflight", Capabilities: map[string]bool{"sql": false, "secrets": false, "identity": false}}
				data, err := json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(filepath.Dir(files.Compose), "installation.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(files.Env)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			_, err = authorizeControlPlaneStop(withProviderChangeConsent(t.Context(), &application.ProviderChangeConsent{PlanID: "old", First: true, Second: true}), &out)
			if machine.Classify(err).Code != machine.ErrorAuthenticationFailed || out.Len() != 0 {
				t.Fatalf("managed %s owner bypassed before acknowledgement: %v %s", source, err, out.String())
			}
			after, err := os.ReadFile(files.Env)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("authorization denial changed retained credentials", err)
			}
		})
	}
}

func TestCoreStopUsesStableScopeRequiresTwoAcknowledgementsAndBindsCredentialBytes(t *testing.T) {
	files := coreStopFixture(t)
	p, err := buildControlPlaneStopPlan(t.Context())
	if err != nil || p.Scope.Attention != "Achtung" || p.Scope.ConfirmationsRequired != 2 || p.Scope.Complete || p.Scope.Destructive || len(p.Services) == 0 || len(p.Materials) == 0 {
		t.Fatalf("Core stop plan: %+v %v", p, err)
	}
	for i := 0; i < 10; i++ {
		current, err := buildControlPlaneStopPlan(t.Context())
		if err != nil || current.Scope.PlanID != p.Scope.PlanID {
			t.Fatal("unstable unchanged owner plan", err)
		}
	}
	for _, ack := range []*application.ProviderChangeConsent{nil, {PlanID: p.Scope.PlanID, First: true}, {PlanID: "stale", First: true, Second: true}} {
		_, err := authorizeControlPlaneStop(withProviderChangeConsent(t.Context(), ack), io.Discard)
		var failure *machine.Error
		if !errors.As(err, &failure) || failure.Details == nil {
			t.Fatal("missing/stale acknowledgement accepted", err)
		}
	}
	ack := &application.ProviderChangeConsent{PlanID: p.Scope.PlanID, First: true, Second: true}
	if _, err := authorizeControlPlaneStop(withProviderChangeConsent(t.Context(), ack), io.Discard); err != nil {
		t.Fatal("deliberately selected outage prohibited", err)
	}
	data, err := os.ReadFile(files.Env)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(files.Env)
	data[len(data)/2] ^= 1
	if err := os.WriteFile(files.Env, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(files.Env, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, err := authorizeControlPlaneStop(withProviderChangeConsent(t.Context(), ack), io.Discard); err == nil {
		t.Fatal("same-size same-mtime credential mutation retained acknowledgement")
	}
}

func TestCoreStopCLIJSONAndMCPExposeSameCurrentScopeWithoutEnvironmentConsentReuse(t *testing.T) {
	coreStopFixture(t)
	var out bytes.Buffer
	err := runWithIO(withProviderChangeConsent(t.Context(), nil), []string{"down", "--json"}, &out, io.Discard)
	var cliFailure *machine.Error
	if !errors.As(err, &cliFailure) || cliFailure.Code != machine.ErrorApprovalRequired || out.Len() != 0 {
		t.Fatalf("CLI stop bypass or JSON progress leak: %v %s", err, out.String())
	}
	t.Setenv(providerChangeConsentEnv, `{"plan_id":"ambient","first":true,"second":true}`)
	session := workspaceMutationClient(t)
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "baseharbor.control-plane.stop", Arguments: machineControlPlaneStopInput{}})
	if err != nil || !result.IsError {
		t.Fatalf("MCP stop bypass: %+v %v", result, err)
	}
	raw, _ := json.Marshal(result.StructuredContent)
	var failure machineToolError
	if json.Unmarshal(raw, &failure) != nil || failure.Error == nil || failure.Error.Code != cliFailure.Code {
		t.Fatalf("MCP error drift: %s", raw)
	}
	cliPlan, _ := json.Marshal(cliFailure.Details)
	mcpPlan, _ := json.Marshal(failure.Error.Details)
	var cli, machinePlan controlPlaneStopPlan
	if json.Unmarshal(cliPlan, &cli) != nil || json.Unmarshal(mcpPlan, &machinePlan) != nil || cli.Scope.PlanID != machinePlan.Scope.PlanID {
		t.Fatal("CLI/MCP consequence authority drift")
	}
}
