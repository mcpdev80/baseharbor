package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func workspaceMutationFixture(t *testing.T, environment string) string {
	t.Helper()
	root := t.TempDir()
	manifest := application.Manifest{Version: application.CurrentVersion, ApplicationID: application.MustNewApplicationID(), Name: "workspace-demo", Environment: environment, Workload: application.WorkloadConfig{Components: []string{"api"}}}
	path := filepath.Join(root, application.RepositoryManifestName)
	if err := os.WriteFile(path, []byte(manifest.YAML()), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func workspaceMutationClient(t *testing.T) *mcp.ClientSession {
	t.Helper()
	server := newMCPServer(application.DefaultStore())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "workspace-parity-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestMCPWorkspaceInitAndMapShareCLIJSONOperations(t *testing.T) {
	configureTestTarget(t)
	manifest := workspaceMutationFixture(t, "dev")
	input := machineWorkspaceInitInput{Manifest: manifest, Sources: []development.SourceDefinition{{ID: "backend", Type: development.SourceRepository, Repository: "https://github.com/acme/api.git"}}, Components: []development.ComponentSource{{Component: "api", Source: "backend"}}}
	session := workspaceMutationClient(t)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.workspace.init", Arguments: input})
	if err != nil || result.IsError {
		t.Fatalf("MCP init: %#v %v", result, err)
	}
	model, _, err := development.LoadSourceModel(manifest)
	if err != nil || model.Components[0].Source != "backend" {
		t.Fatalf("model = %#v %v", model, err)
	}
	var cliOutput bytes.Buffer
	err = appWorkspaceInitCommand().Run(context.Background(), []string{"--manifest", manifest, "--source", "backend=https://github.com/acme/api.git", "--component", "api=backend", "-o", "json"}, &cliOutput, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	var cliResult workspaceMutationResult
	if err := json.Unmarshal(cliOutput.Bytes(), &cliResult); err != nil {
		t.Fatal(err)
	}
	if cliResult.SourceModelPath == "" {
		t.Fatal("CLI JSON lacks source-model identity")
	}
	checkout := t.TempDir()
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.workspace.map", Arguments: machineWorkspaceMapInput{Manifest: manifest, Source: "backend", Path: checkout}})
	if err != nil || result.IsError {
		t.Fatalf("MCP map: %#v %v", result, err)
	}
	mapping, _, err := development.LoadWorkspaceMapping(manifest, "workspace-demo")
	if err != nil || mapping.Sources["backend"] != checkout {
		t.Fatalf("mapping = %#v %v", mapping, err)
	}
}

func TestMCPWorkspaceManagedDenialPrecedesMutation(t *testing.T) {
	configureTestTarget(t)
	t.Setenv(operatorauth.EnvIssuer, "")
	t.Setenv(operatorauth.EnvClientID, "")
	manifest := workspaceMutationFixture(t, "prod")
	session := workspaceMutationClient(t)
	for _, tool := range []string{"baseharbor.workspace.init", "baseharbor.workspace.map"} {
		arguments := map[string]any{"manifest": manifest}
		if tool == "baseharbor.workspace.init" {
			arguments["sources"] = []any{}
			arguments["components"] = []any{}
		} else {
			arguments["source"] = "backend"
			arguments["path"] = t.TempDir()
		}
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: arguments})
		if err != nil || !result.IsError {
			t.Fatalf("managed operation was not denied: %#v %v", result, err)
		}
		payload, _ := json.Marshal(result)
		if !bytes.Contains(payload, []byte("authentication_failed")) {
			t.Fatalf("denial = %s", payload)
		}
	}
	if _, _, err := development.LoadSourceModel(manifest); err == nil {
		t.Fatal("denied operation wrote a source model")
	}
	if _, _, err := development.LoadWorkspaceMapping(manifest, "workspace-demo"); err == nil {
		t.Fatal("denied operation wrote a checkout mapping")
	}
}
