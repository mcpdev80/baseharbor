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

func TestCLIJSONAndMCPShareApplicationOverviewAndDevelopmentDomain(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(t.TempDir())
	var out, errOut bytes.Buffer
	if err := runWithIO(context.Background(), []string{"app", "create", "parity-demo", "--sql", "--json"}, &out, &errOut); err != nil {
		t.Fatalf("create: %v %s", err, errOut.String())
	}
	var created applicationAdoptionResult
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if err := application.ValidateApplicationID(created.ApplicationID); err != nil {
		t.Fatal(err)
	}
	session := workspaceMutationClient(t)
	for _, test := range []struct {
		args  []string
		tool  string
		input map[string]any
	}{
		{[]string{"app", "show", "parity-demo", "--json"}, "app.show", map[string]any{"name": "parity-demo"}},
		{[]string{"dev", "domain", "--json"}, "dev.domain", map[string]any{}},
		{[]string{"whoami", "-e", "dev", "--json"}, "operator.identity", map[string]any{"environment": "dev"}},
	} {
		out.Reset()
		errOut.Reset()
		if err := runWithIO(context.Background(), test.args, &out, &errOut); err != nil {
			t.Fatalf("%s CLI: %v %s", test.tool, err, errOut.String())
		}
		var cliResult any
		if err := json.Unmarshal(out.Bytes(), &cliResult); err != nil {
			t.Fatalf("%s CLI mixed human/JSON: %s", test.tool, out.String())
		}
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor." + test.tool, Arguments: test.input})
		if err != nil || result.IsError {
			t.Fatalf("%s MCP: %#v %v", test.tool, result, err)
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
			t.Fatalf("%s semantic drift: CLI %s MCP %s", test.tool, out.String(), encoded)
		}
	}
}
