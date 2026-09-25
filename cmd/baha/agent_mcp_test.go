package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestAgentDescribeJSON(t *testing.T) {
	var out bytes.Buffer
	if err := runWithIO(context.Background(), []string{"agent", "describe", "-o", "json"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var got agentDescription
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ContractVersion != machine.ContractVersion {
		t.Fatalf("contract version = %q", got.ContractVersion)
	}
	if got.MCP.ProtocolVersion != "2026-07-28" || got.MCP.Transport != "stdio" || got.MCP.Remote {
		t.Fatalf("unexpected MCP discovery: %#v", got.MCP)
	}
	if len(got.Operations) != len(machine.Operations()) {
		t.Fatalf("operations = %d, want %d", len(got.Operations), len(machine.Operations()))
	}
	if len(got.MCP.Tools) != len(machine.MCPTools()) {
		t.Fatalf("MCP tools = %d, want %d", len(got.MCP.Tools), len(machine.MCPTools()))
	}
	destroy, ok := machine.OperationByID("destroy")
	if !ok || destroy.Safety != machine.SafetyDestructive || !destroy.ConfirmationRequired {
		t.Fatalf("destroy safety metadata = %#v", destroy)
	}
	for _, operation := range got.Operations {
		if operation.MCPTool == "" {
			t.Fatalf("operation missing MCP tool: %#v", operation)
		}
	}
}

func TestMCPGenericClientDiscoversCompleteSemanticSurfaceAndExercisesReadOnlyTools(t *testing.T) {
	target := configureTestTarget(t)
	root := t.TempDir()
	manifest := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "mcp-demo",
		Environment: "dev",
		Workload: application.WorkloadConfig{
			Compose:  "compose.yaml",
			Services: []string{"api"},
		},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, application.RepositoryManifestName), []byte(manifest.YAML()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: alpine:3.22\n    command: [\"sleep\", \"3600\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const secretMarker = "BASEHARBOR_MCP_SECRET_DO_NOT_LEAK_7a01"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DATABASE_PASSWORD="+secretMarker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := testDeploymentStore(t, target, manifest)
	if _, err := store.Create(manifest); err != nil {
		t.Fatal(err)
	}
	registerTestDeployment(t, target, manifest, root, filepath.Join(root, application.RepositoryManifestName))
	server := newMCPServer(store)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "baseharbor-test-client", Version: "v1"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	list, err := clientSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]machine.Operation{}
	for _, operation := range machine.Operations() {
		if operation.MCPTool != "" {
			want[operation.MCPTool] = operation
		}
	}
	if len(list.Tools) != len(want) {
		t.Fatalf("MCP tool count = %d, want %d", len(list.Tools), len(want))
	}
	for _, tool := range list.Tools {
		operation, exists := want[tool.Name]
		if !exists {
			t.Fatalf("unexpected MCP tool %q", tool.Name)
		}
		if tool.Annotations == nil {
			t.Fatalf("tool %q has no safety annotations", tool.Name)
		}
		if tool.Annotations.ReadOnlyHint != (operation.Safety == machine.SafetyReadOnly) {
			t.Fatalf("tool %q read-only annotation does not match operation %#v", tool.Name, operation)
		}
		if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint != (operation.Safety == machine.SafetyDestructive) {
			t.Fatalf("tool %q destructive annotation does not match operation %#v", tool.Name, operation)
		}
	}
	for _, forbidden := range []string{"exec", "shell", "docker", "compose", "podman"} {
		for _, tool := range list.Tools {
			if strings.Contains(strings.ToLower(tool.Name), forbidden) {
				t.Fatalf("forbidden generic execution primitive exposed: %s", tool.Name)
			}
		}
	}

	calls := []struct {
		name string
		args map[string]any
	}{
		{name: "baseharbor.target", args: map[string]any{}},
		{name: "baseharbor.inspect", args: map[string]any{"path": root}},
		{name: "baseharbor.plan", args: map[string]any{"name": manifest.Name}},
		{name: "baseharbor.status", args: map[string]any{"name": manifest.Name}},
		{name: "baseharbor.doctor", args: map[string]any{"name": manifest.Name}},
		{name: "baseharbor.observe", args: map[string]any{"name": manifest.Name}},
		{name: "baseharbor.policy.check", args: map[string]any{"name": manifest.Name}},
		{name: "baseharbor.policy.explain", args: map[string]any{"name": manifest.Name}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("tool returned error: %#v", result.Content)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(encoded, []byte(`"contract_version":"v1"`)) {
				t.Fatalf("machine contract version missing from %s: %s", tc.name, encoded)
			}
			if bytes.Contains(encoded, []byte(secretMarker)) {
				t.Fatalf("secret marker leaked from %s", tc.name)
			}
		})
	}

	t.Run("destroy requires explicit approval", func(t *testing.T) {
		result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "baseharbor.destroy",
			Arguments: map[string]any{"name": manifest.Name, "approval": false},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatalf("destroy without approval unexpectedly succeeded: %#v", result)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(encoded, []byte(`"code":"approval_required"`)) {
			t.Fatalf("destroy approval error is not typed: %s", encoded)
		}
	})

	t.Run("backup rejects inline-or-missing secret input before mutation", func(t *testing.T) {
		result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "baseharbor.backup",
			Arguments: map[string]any{"name": manifest.Name},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatalf("backup without password file unexpectedly succeeded: %#v", result)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(encoded, []byte(`"code":"validation_failed"`)) {
			t.Fatalf("backup validation error is not typed: %s", encoded)
		}
		if bytes.Contains(encoded, []byte(secretMarker)) {
			t.Fatalf("secret marker leaked from backup validation: %s", encoded)
		}
	})
}

func TestMachineCLIErrorClassification(t *testing.T) {
	err := usageError("bad input", "use a valid input")
	got := machine.Classify(classifyMachineCLIError(err))
	if got.Code != machine.ErrorValidationFailed {
		t.Fatalf("code = %q", got.Code)
	}
	if got.Next != "use a valid input" {
		t.Fatalf("next = %q", got.Next)
	}
}
