package main

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLocalStdioRuntimeAuthorityProcess(t *testing.T) {
	if os.Getenv("BASEHARBOR_TEST_LOCAL_STDIO_HELPER") != "1" { t.Skip("subprocess entry for the isolated actual local stdio runtime acceptance") }
	if err := runWithIO(context.Background(), []string{"mcp", "serve"}, os.Stdout, os.Stderr); err != nil { os.Exit(1) }
	os.Exit(0)
}

func qualifyActualLocalStdioWorkloadAuthority(t *testing.T, ctx context.Context, target deployment.ResolvedTarget, manifest application.Manifest) {
	t.Helper()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLocalStdioRuntimeAuthorityProcess$")
	cmd.Env = append(os.Environ(), "BASEHARBOR_TEST_LOCAL_STDIO_HELPER=1")
	client := mcp.NewClient(&mcp.Implementation{Name: "actual-local-workload-authority", Version: "v1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if err != nil { t.Fatal("actual local stdio transport did not initialize") }
	defer session.Close()
	for _, tool := range []string{"baseharbor.plan", "baseharbor.apply", "baseharbor.status", "baseharbor.doctor"} {
		callMCPAcceptanceTool(t, ctx, session, tool, map[string]any{"target": target.Name, "environment": manifest.Environment, "name": manifest.Name})
	}
	t.Log("actual local stdio prod plan/apply/status/doctor succeeded on the owned rootless Target without an OIDC login or management bootstrap")
}
