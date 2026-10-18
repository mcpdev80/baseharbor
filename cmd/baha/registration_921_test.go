package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCore921ActualCommandTreeHelpAndReadOnlyEntry(t *testing.T) {
	var output, errors bytes.Buffer
	for _, path := range [][]string{
		{"app", "new", "--help"},
		{"app", "edit", "--help"},
		{"app", "footprint", "show", "--help"},
	} {
		output.Reset()
		errors.Reset()
		if err := runWithIO(context.Background(), path, &output, &errors); err != nil {
			t.Fatalf("%v --help: %v, %s", path, err, errors.String())
		}
		if !strings.Contains(strings.ToLower(output.String()), path[len(path)-2]) &&
			!strings.Contains(strings.ToLower(output.String()), "footprint") {
			t.Fatalf("%v unavailable from actual help: %q", path, output.String())
		}
	}
	var foundEdit, foundFootprint bool
	for _, row := range currentCommandCoverage() {
		if row.Command == "baha app edit" {
			foundEdit = row.Operation == "app.edit" && row.MCPTool == "baseharbor.app.edit"
		}
		if row.Command == "baha app footprint show" {
			foundFootprint = row.Operation == "app.footprint.show" && row.MCPTool == "baseharbor.app.footprint.show"
		}
	}
	if !foundEdit || !foundFootprint {
		t.Fatalf("actual tree machine coverage is missing: edit=%t footprint=%t", foundEdit, foundFootprint)
	}
}

func TestCore921MCPRealDiscoveryAndRejectIncompleteEdit(t *testing.T) {
	configureTestTarget(t)
	session := workspaceMutationClient(t)
	ctx := context.Background()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, tool := range tools.Tools {
		if found[tool.Name] {
			t.Fatalf("duplicate registered MCP tool %q", tool.Name)
		}
		found[tool.Name] = true
	}
	for _, name := range []string{"baseharbor.app.new", "baseharbor.app.edit", "baseharbor.app.footprint.show"} {
		if !found[name] {
			t.Fatalf("real MCP tools/list missing %s", name)
		}
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "baseharbor.app.edit",
		Arguments: map[string]any{"Target": "dev", "Phase": "apply"},
	})
	if err == nil && result != nil && !result.IsError {
		t.Fatal("MCP edit without verified proposal and consent unexpectedly succeeded")
	}
}
