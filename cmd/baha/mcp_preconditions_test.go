package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

func TestExpectedPreconditionsShareTypedCLIMCPErrors(t *testing.T) {
	cases := []struct {
		name string
		err error
		want machine.ErrorCode
	}{
		{"workspace model", development.ErrWorkspaceModelMissing, machine.ErrorSourceMissing},
		{"workspace source", development.ErrWorkspaceSourceMissing, machine.ErrorSourceMissing},
		{"repository manifest", application.ErrRepositoryManifestNotFound, machine.ErrorSourceMissing},
		{"organization not configured", orgconfig.ErrNotConfigured, machine.ErrorValidationFailed},
		{"unsupported organization source", orgconfig.ErrUnsupportedSource, machine.ErrorUnsupported},
		{"registered deployment absent", &machine.Error{Code: machine.ErrorNotFound, Message: "deployment absent"}, machine.ErrorNotFound},
		{"deadline", context.DeadlineExceeded, machine.ErrorTimeout},
		{"cancellation", context.Canceled, machine.ErrorInternal},
		{"unknown failure", errors.New("unexpected storage failure"), machine.ErrorInternal},
		{"unknown matching words", errors.New("organization not configured; deployment not found"), machine.ErrorInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cliError := machine.Classify(classifyMachineCLIError(tc.err))
			if cliError.Code != tc.want {
				t.Fatalf("CLI code = %s, want %s", cliError.Code, tc.want)
			}
			result, payload, err := machineMCPFailure(tc.err)
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || !result.IsError {
				t.Fatalf("missing MCP error result: %#v", result)
			}
			typed, ok := payload.(machineToolError)
			if !ok || typed.Error == nil || typed.Error.Code != tc.want {
				t.Fatalf("MCP error payload = %#v, want %s", payload, tc.want)
			}
		})
	}
}

func TestMCPReadEntryExpectedPreconditions(t *testing.T) {
	configureTestTarget(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	server := newMCPServer(application.DefaultStore())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "preconditions-test", Version: "v1"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, &mcp.ClientSessionOptions{ProtocolVersion: "2026-07-28"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	cases := []struct {
		tool string
		args map[string]any
		want machine.ErrorCode
	}{
		{"organization.inspect", map[string]any{"environment": "dev"}, machine.ErrorValidationFailed},
		{"status", map[string]any{"name": "unregistered-core754", "environment": "dev"}, machine.ErrorNotFound},
		{"policy.explain", map[string]any{"name": "unregistered-core754", "environment": "dev"}, machine.ErrorNotFound},
		{"workspace.status", map[string]any{}, machine.ErrorSourceMissing},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
				Name: "baseharbor." + tc.tool, Arguments: tc.args,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || !result.IsError || len(result.Content) == 0 {
				t.Fatalf("expected structured MCP precondition failure: %#v", result)
			}
			content, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("unexpected MCP error content: %T", result.Content[0])
			}
			var payload machineToolError
			if err := json.Unmarshal([]byte(content.Text), &payload); err != nil {
				t.Fatalf("decode MCP error: %v: %s", err, content.Text)
			}
			if payload.Error == nil || payload.Error.Code != tc.want {
				t.Fatalf("%s error = %s, want %s (%s)", tc.tool, fmt.Sprint(payload.Error), tc.want, content.Text)
			}
		})
	}
}
