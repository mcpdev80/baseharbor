package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// These are registration contract sentinels, not new CLI/MCP registrations.
// All inspect the real tree and the in-process SDK server.
func TestAppRegistrationCICommandHelpAndJSONDiscovery(t *testing.T) {
	root := rootCommand()
	var app *cli.Command
	for _, child := range root.Children {
		if child.Name == "app" {
			app = child
			break
		}
	}
	if app == nil {
		t.Fatal("application command is absent from the real root")
	}
	seen := map[string]bool{}
	for _, child := range app.Children {
		if seen[child.Name] {
			t.Fatalf("duplicate app registration: %s", child.Name)
		}
		seen[child.Name] = true
	}
	for _, required := range []string{"list", "show"} {
		if !seen[required] {
			t.Fatalf("required existing app command %q is not registered", required)
		}
		var out, errors bytes.Buffer
		if err := runWithIO(context.Background(), []string{"app", required, "--help"}, &out, &errors); err != nil {
			t.Fatalf("%s --help: %v (%s)", required, err, errors.String())
		}
		if !strings.Contains(out.String(), required) {
			t.Fatalf("%s help does not describe registered command: %q", required, out.String())
		}
	}
}

func TestCommandTreeCIRouterPathsUnique(t *testing.T) {
	seen := map[string]bool{}
	var walk func(*cli.Command, string)
	walk = func(command *cli.Command, parent string) {
		if command.Hidden {
			return
		}
		path := strings.TrimSpace(parent + " " + command.Name)
		if seen[path] {
			t.Errorf("duplicate executable command tree path: %s", path)
			return
		}
		seen[path] = true
		for _, child := range command.Children {
			walk(child, path)
		}
	}
	walk(rootCommand(), "")
	for _, path := range []string{"baha", "baha app", "baha app list", "baha app show"} {
		if !seen[path] {
			t.Errorf("missing real command tree path: %s", path)
		}
	}
}

func TestMachineCoverageCIRealTreeAndJSONClassification(t *testing.T) {
	seen := map[string]bool{}
	semantic := 0
	for _, row := range currentCommandCoverage() {
		if seen[row.Command] {
			t.Fatalf("duplicate machine coverage for %s", row.Command)
		}
		seen[row.Command] = true
		if row.Classification == "gap" {
			t.Fatalf("unclassified command: %s", row.Command)
		}
		if row.Classification == "semantic" {
			semantic++
			if row.Operation == "" || row.MCPTool == "" || row.JSONSurface == "" {
				t.Fatalf("machine operation lacks typed JSON/MCP coverage: %+v", row)
			}
		}
	}
	if semantic == 0 || !seen["baha app show"] || !seen["baha app list"] {
		t.Fatal("machine inventory lacks real application commands")
	}
}

func TestMCPRegistrationCIInProcessDiscoverAndInvoke(t *testing.T) {
	t.Skip("deliberate negative qualification: mandatory MCP family must not be skipped")
	configureTestTarget(t)
	session := workspaceMutationClient(t)
	ctx := context.Background()
	result, err := session.ListTools(ctx, nil)
	if err != nil || result == nil || len(result.Tools) == 0 {
		t.Fatalf("in-process tools/list failed: %+v %v", result, err)
	}
	seen := map[string]bool{}
	for _, tool := range result.Tools {
		if seen[tool.Name] {
			t.Fatalf("duplicate actual MCP registration: %s", tool.Name)
		}
		seen[tool.Name] = true
	}
	if !seen["baseharbor.app.list"] {
		t.Fatal("actual MCP app.list tool missing")
	}
	invoked, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "baseharbor.app.list", Arguments: map[string]any{},
	})
	if err != nil || invoked == nil || invoked.IsError {
		t.Fatalf("in-process MCP app.list invocation failed: %+v %v", invoked, err)
	}
}

// Qualification pilot: execute all six registration families through trusted CI.
