package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestMCPGenericClientRealApplicationLifecycle(t *testing.T) {
	if os.Getenv("BASEHARBOR_MCP_LIFECYCLE_ACCEPTANCE") != "true" {
		t.Skip("MCP lifecycle acceptance is opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := bhruntime.DetectCompose(ctx); err != nil {
		t.Skipf("runtime unavailable: %v", err)
	}
	ensureRuntimeIntegrationTrustPlane(t, ctx)

	root := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	manifest := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "mcp-lifecycle",
		Environment: "dev",
		Services:    application.Services{SQL: true},
		Workload: application.WorkloadConfig{
			Compose:  "compose.yaml",
			Services: []string{"api"},
		},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(application.RepositoryManifestName, []byte(manifest.YAML()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("compose.yaml", []byte("services:\n  api:\n    image: alpine:3.22\n    command: [\"sleep\", \"3600\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".gitignore", []byte(".baseharbor/\n*.bhbackup\nbackup-password\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	remote := filepath.Join(t.TempDir(), "remote.git")
	mustGitUpdateTest(t, "", "init", "--bare", remote)
	mustGitUpdateTest(t, root, "init", "-b", "main")
	configureGitUpdateTestIdentity(t, root)
	mustGitUpdateTest(t, root, "add", application.RepositoryManifestName, "compose.yaml", ".gitignore")
	mustGitUpdateTest(t, root, "commit", "-m", "initial")
	mustGitUpdateTest(t, root, "remote", "add", "origin", remote)
	mustGitUpdateTest(t, root, "push", "-u", "origin", "main")

	server := newMCPServer(application.DefaultStore())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "generic-coding-agent", Version: "v1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.inspect", map[string]any{"path": root})
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.plan", map[string]any{})
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.apply", map[string]any{})
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.status", map[string]any{})
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.doctor", map[string]any{})
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.observe", map[string]any{})

	upstream := filepath.Join(t.TempDir(), "upstream")
	mustGitUpdateTest(t, "", "clone", "--branch", "main", remote, upstream)
	configureGitUpdateTestIdentity(t, upstream)
	if err := os.WriteFile(filepath.Join(upstream, "README.md"), []byte("updated through MCP acceptance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGitUpdateTest(t, upstream, "add", "README.md")
	mustGitUpdateTest(t, upstream, "commit", "-m", "upstream update")
	mustGitUpdateTest(t, upstream, "push", "origin", "main")
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.update", map[string]any{"no_backup": true})

	resolved, err := resolveApplication(application.DefaultStore(), nil, "MCP lifecycle acceptance drift")
	if err != nil {
		t.Fatal(err)
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := detectComposeForApplication(ctx, resolved, bhruntime.CapabilityWorkloadLifecycle)
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := stopRepositoryWorkload(ctx, runtime, resolved, files)
	if err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("expected repository workload to be stopped to create repairable drift")
	}
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.repair", map[string]any{})
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.doctor", map[string]any{})

	passwordPath := filepath.Join(root, "backup-password")
	if err := os.WriteFile(passwordPath, []byte("correct horse battery staple\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(root, "mcp-lifecycle.bhbackup")
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.backup", map[string]any{
		"password_file": passwordPath,
		"output_path":   backupPath,
	})
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("MCP backup archive missing: %v", err)
	}

	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.destroy", map[string]any{"approval": true})
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.restore", map[string]any{
		"backup_path":   backupPath,
		"password_file": passwordPath,
	})
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.status", map[string]any{})
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.doctor", map[string]any{})
	callMCPAcceptanceTool(t, ctx, clientSession, "baseharbor.destroy", map[string]any{"approval": true})
}

func callMCPAcceptanceTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s transport call failed: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s returned machine error: %s", name, formatMCPAcceptanceContent(result))
	}
	return result
}

func formatMCPAcceptanceContent(result *mcp.CallToolResult) string {
	if result == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", result.Content)
}
