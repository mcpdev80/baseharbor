package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPStackCreateListShowShareCLIJSON(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(t.TempDir())
	session := workspaceMutationClient(t)
	options, err := parseStackCreateOptions([]string{"team-fixture", "--component", "api:application:go"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.stack.create", Arguments: machineStackCreateInput{Profile: options.Profile, Scope: options.Scope}})
	if err != nil || result.IsError {
		t.Fatalf("create: %#v %v", result, err)
	}
	var output bytes.Buffer
	if err := stackShowCommand().Run(context.Background(), []string{"team-fixture", "-o", "json"}, &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var cliResult stackShowResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatal(err)
	}
	if cliResult.Profile.Metadata.Name != "team-fixture" || len(cliResult.Sources) == 0 {
		t.Fatalf("show: %#v", cliResult)
	}
	for _, tool := range []string{"baseharbor.stack.list", "baseharbor.stack.show"} {
		arguments := map[string]any{}
		if tool == "baseharbor.stack.show" {
			arguments["name"] = "team-fixture"
		}
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: arguments})
		if err != nil || result.IsError {
			payload, _ := json.Marshal(result)
			t.Fatalf("%s: %s %v", tool, payload, err)
		}
		encoded, _ := json.Marshal(result)
		if !bytes.Contains(encoded, []byte("team-fixture")) {
			t.Fatalf("%s missing profile", tool)
		}
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.stack.create", Arguments: machineStackCreateInput{Profile: options.Profile}})
	if err != nil || !result.IsError {
		t.Fatal("duplicate accepted")
	}
}
func TestStackManagedDenialPrecedesCLIAndMCPWrites(t *testing.T) {
	configureTestTarget(t)
	manifest := workspaceMutationFixture(t, "prod")
	t.Chdir(filepath.Dir(manifest))
	options, err := parseStackCreateOptions([]string{"denied-fixture", "--component", "api:application:go"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createStackProfile(context.Background(), options.Profile, options.Scope); err == nil {
		t.Fatal("managed mutation accepted")
	}
	session := workspaceMutationClient(t)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.stack.create", Arguments: machineStackCreateInput{Profile: options.Profile}})
	if err != nil || !result.IsError {
		t.Fatalf("managed mutation accepted: %#v %v", result, err)
	}
	root, err := development.UserProfileRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "denied-fixture.yaml")); !os.IsNotExist(err) {
		t.Fatalf("denied profile written: %v", err)
	}
}
