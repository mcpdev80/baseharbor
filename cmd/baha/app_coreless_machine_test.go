package main

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCorelessPlanCLIJSONMCPParityAndStableIdentity(t *testing.T) {
	target := configureTestTarget(t)
	selection := minimalRepositorySelection(t)
	t.Chdir(selection.RepositoryRoot)
	initialized, err := resolveRepositoryLifecycleIntent(context.Background(), target, selection, "init")
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "plan", "--json"}, &out, &errOut); err != nil {
		t.Fatalf("CLI: %v %s", err, errOut.String())
	}
	var cliResult any
	if err := json.Unmarshal(out.Bytes(), &cliResult); err != nil {
		t.Fatalf("JSON contaminated with human output: %s", out.String())
	}
	session := workspaceMutationClient(t)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.plan", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("MCP: %+v %v", result, err)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var mcpResult any
	if err := json.Unmarshal(encoded, &mcpResult); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cliResult, mcpResult) {
		t.Fatalf("plan drift CLI=%s MCP=%s", out.String(), encoded)
	}
	var plan applicationLifecyclePlan
	if err := json.Unmarshal(encoded, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Footprint.Core != application.CoreNotRequired || plan.Footprint.ApplicationID != initialized.Manifest.ApplicationID || len(plan.Footprint.Additional) != 0 {
		t.Fatalf("incorrect coreless machine plan: %+v", plan)
	}
}
