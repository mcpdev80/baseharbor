package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPTargetCreateAndDeleteRequireCurrentConsentAndPreserveOwnedState(t *testing.T) {
	configureTestTarget(t)
	session := workspaceMutationClient(t)
	input := machineTargetCreateInput{Name: "podman-fixture", RuntimeProvider: "podman", Access: "local-podman", Reference: "local"}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.target.create", Arguments: input})
	if err != nil || result.IsError {
		t.Fatalf("create: %#v %v", result, err)
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Targets[input.Name].Runtime.Provider != "podman" {
		t.Fatal("target not saved")
	}
	root, err := deployment.TargetStateRoot(input.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(root, "owned-state")
	if err := os.WriteFile(owned, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.target.delete", Arguments: machineTargetDeleteInput{Name: input.Name}})
	if err != nil || !result.IsError {
		t.Fatal("target with owned state deleted")
	}
	plan, err := buildTargetDefinitionChangePlan(context.Background(), input.Name, "")
	if err != nil {
		t.Fatal(err)
	}
	consent := application.ProviderChangeConsent{PlanID: plan.Scope.PlanID, First: true, Second: true}
	var out bytes.Buffer
	if err := deleteTarget(withProviderChangeConsent(context.Background(), &consent), []string{input.Name, "-o", "json"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var deleted targetMutationResult
	if err := json.Unmarshal(out.Bytes(), &deleted); err != nil || !deleted.Deleted {
		t.Fatalf("delete JSON: %s %v", out.Bytes(), err)
	}
	if actual, err := os.ReadFile(owned); err != nil || string(actual) != "owned" {
		t.Fatal("Target detachment removed owned state", err)
	}
	restore, err := buildTargetDefinitionChangePlan(context.Background(), input.Name, deleted.RecoveryFile)
	if err != nil {
		t.Fatal(err)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.target.create", Arguments: machineTargetCreateInput{Name: input.Name, RecoveryFile: deleted.RecoveryFile, ProviderChangeConsent: &application.ProviderChangeConsent{PlanID: restore.Scope.PlanID, First: true, Second: true}}})
	if err != nil || result.IsError {
		t.Fatalf("MCP restoration: %#v %v", result, err)
	}
	cfg, err = deployment.LoadConfig()
	if err != nil || cfg.Targets[input.Name].Runtime.Provider != "podman" {
		t.Fatal("original Target binding not restored", err)
	}
}
