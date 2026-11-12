package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPAuthoringAssignsCanonicalIdentityWithoutClientID(t *testing.T) {
	configureTestTarget(t)
	session := workspaceMutationClient(t)
	intent := map[string]any{"Version": 1, "Name": "machine-owned", "Environment": "dev", "Workload": map[string]any{"Components": []string{"api"}}}
	for _, operation := range []string{"create", "adopt"} {
		t.Run(operation, func(t *testing.T) {
			intent["Name"] = "machine-" + operation
			args := map[string]any{"intent": intent}
			root := t.TempDir()
			if operation == "adopt" {
				args["repository"] = root
				if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example.invalid/workload:fixed\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.app." + operation, Arguments: args})
			if err != nil || result.IsError {
				failure, _ := json.Marshal(result)
				t.Fatalf("author without identity: %v %s", err, failure)
			}
			encoded, _ := json.Marshal(result.StructuredContent)
			var created applicationAdoptionResult
			if err := json.Unmarshal(encoded, &created); err != nil {
				t.Fatal(err)
			}
			if err := application.ValidateApplicationID(created.ApplicationID); err != nil {
				t.Fatal(err)
			}
			authored, err := application.LoadManifestFile(created.Manifest)
			if err != nil || authored.ApplicationID != "" {
				t.Fatalf("authored identity leaked: %+v %v", authored, err)
			}
			if operation == "adopt" {
				owner := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
				canonical, _, err := owner.Load(created.Application)
				if err != nil || canonical.ApplicationID != created.ApplicationID {
					t.Fatal("machine output differs from existing canonical owner")
				}
				before, _ := os.ReadFile(created.Manifest)
				repeated, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.app.adopt", Arguments: args})
				if err != nil || !repeated.IsError {
					t.Fatal("existing authored source overwritten")
				}
				after, _ := os.ReadFile(created.Manifest)
				canonical, _, _ = owner.Load(created.Application)
				if string(before) != string(after) || canonical.ApplicationID != created.ApplicationID {
					t.Fatal("rejected repeated adoption changed source or identity")
				}
			} else {
				target, _ := effectiveTarget(context.Background())
				record, found, err := deployment.FindDeployment(target.Name, created.ApplicationID, "dev")
				if err != nil || !found || record.Source.Manifest != created.Manifest {
					t.Fatal("machine-generated identity was not durably registered")
				}
				repeated, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.app.create", Arguments: args})
				if err != nil || !repeated.IsError {
					t.Fatal("repeated creation allocated a second owner for the same app")
				}
				records, err := deployment.ListDeployments(target.Name)
				if err != nil || len(records) != 1 || records[0].Identity.ApplicationID != created.ApplicationID {
					t.Fatal("rejected repeated creation changed registered identity")
				}
			}
		})
	}
}

func TestMCPAuthoringRejectsSuppliedIdentityBeforeMutation(t *testing.T) {
	target := configureTestTarget(t)
	session := workspaceMutationClient(t)
	for _, operation := range []string{"create", "adopt"} {
		root := t.TempDir()
		intent := map[string]any{"Version": 1, "ApplicationID": application.MustNewApplicationID(), "Name": "forbidden-id", "Environment": "dev", "Workload": map[string]any{"Components": []string{"api"}}}
		args := map[string]any{"intent": intent}
		if operation == "adopt" {
			args["repository"] = root
		}
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.app." + operation, Arguments: args})
		if err != nil || !result.IsError {
			t.Fatalf("client identity accepted: %v %+v", err, result)
		}
		encoded, _ := json.Marshal(result)
		if !strings.Contains(string(encoded), "ApplicationID") {
			t.Fatalf("rejected for wrong reason: %s", encoded)
		}
		files, _ := os.ReadDir(root)
		records, err := deployment.ListDeployments(target.Name)
		if len(files) != 0 || err != nil || len(records) != 0 {
			t.Fatal("rejected client identity mutated repository/deployments")
		}
	}
}

func TestRepositoryAdoptionReusesRetainedCanonicalIdentity(t *testing.T) {
	configureTestTarget(t)
	root := t.TempDir()
	owner := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	prior := application.New("retry-adopt", "dev", true, false, false)
	if _, err := owner.Create(prior); err != nil {
		t.Fatal(err)
	}
	intent := prior
	intent.ApplicationID = ""
	result, err := persistRepositoryApplication(context.Background(), root, intent, nil)
	if err != nil || result.ApplicationID != prior.ApplicationID {
		t.Fatalf("retained retry owner changed: %+v %v", result, err)
	}
}

func TestRepositoryAdoptionDefersFreshIdentityToRemoteCore(t *testing.T) {
	ctx, _, node := remoteApplicationCoreFixtureEmpty(t)
	t.Setenv("BASEHARBOR_TARGET", node.Name)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example.invalid/workload:fixed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	intent := application.New("remote-demo", "dev", true, false, false)
	intent.ApplicationID = ""
	result, err := persistRepositoryApplication(ctx, root, intent, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ApplicationID != "" {
		t.Fatalf("workstation assigned remote app identity %q", result.ApplicationID)
	}
	authored, err := application.LoadManifestFile(result.Manifest)
	if err != nil || authored.ApplicationID != "" {
		t.Fatalf("authored remote manifest contains a client identity: %+v %v", authored, err)
	}
	owner := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	items, err := owner.List()
	if err != nil || len(items) != 0 {
		t.Fatalf("remote guided init persisted a competing local owner: %+v %v", items, err)
	}
}

func TestMCPAuthoringAuthorizationDenialPrecedesIdentityPersistence(t *testing.T) {
	target := configureTestTarget(t)
	t.Setenv(operatorauth.EnvIssuer, "")
	t.Setenv(operatorauth.EnvClientID, "")
	session := workspaceMutationClient(t)
	for _, operation := range []string{"create", "adopt"} {
		root := t.TempDir()
		args := map[string]any{"intent": map[string]any{"Version": 1, "Name": "denied-authoring", "Environment": "prod", "Workload": map[string]any{"Components": []string{"api"}}}}
		if operation == "adopt" {
			args["repository"] = root
		}
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.app." + operation, Arguments: args})
		if err != nil || !result.IsError {
			t.Fatalf("authorization denial bypassed: %v %+v", err, result)
		}
		encoded, _ := json.Marshal(result)
		if !strings.Contains(string(encoded), "authentication_failed") {
			t.Fatalf("denied for wrong reason: %s", encoded)
		}
		files, _ := os.ReadDir(root)
		records, err := deployment.ListDeployments(target.Name)
		if len(files) != 0 || err != nil || len(records) != 0 {
			t.Fatal("denied authoring persisted identity/source/deployment")
		}
	}
}

func TestMCPAuthoringWorkloadComposeIsOptional(t *testing.T) {
	// Regression: #1265 made Workload.Compose required in the generated MCP intent
	// schema, rejecting valid authoring intent that omits it. Source provenance is
	// optional portable intent (spec/application-contract-v1.md). A valid adopt
	// intent with only workload.components must pass schema validation without it.
	target := configureTestTarget(t)
	root := t.TempDir()
	session := workspaceMutationClient(t)
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example.invalid/workload:fixed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"repository": root, "intent": map[string]any{"Version": 1, "Name": "coreless", "Environment": "dev", "Workload": map[string]any{"Components": []string{"api"}}}}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.app.adopt", Arguments: args})
	if err != nil || result.IsError {
		failure, _ := json.Marshal(result)
		t.Fatalf("valid intent without Compose rejected: %v %s", err, failure)
	}
	if _, err := deployment.ListDeployments(target.Name); err != nil {
		t.Fatalf("deployment lookup after valid adopt failed: %v", err)
	}
}

func TestMCPAuthoringSchemaStillRejectsMissingRequiredField(t *testing.T) {
	// Companion negative case to TestMCPAuthoringWorkloadComposeIsOptional:
	// removing Compose must be optional, but an actually required intent field
	// (Name) must still be rejected by the generated schema itself, not by later
	// business logic. If the omitempty fix ever over-relaxed the schema, this
	// assertion fails, which proves the required gate is intact.
	root := t.TempDir()
	session := workspaceMutationClient(t)
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  api:\n    image: example.invalid/workload:fixed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(root)
	args := map[string]any{"repository": root, "intent": map[string]any{"Version": 1, "Environment": "dev", "Workload": map[string]any{"Components": []string{"api"}}}}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.app.adopt", Arguments: args})
	if err != nil || !result.IsError {
		t.Fatalf("missing required field Name accepted: %+v", result)
	}
	encoded, _ := json.Marshal(result)
	if !strings.Contains(string(encoded), "missing properties") || !strings.Contains(string(encoded), "Name") {
		t.Fatalf("rejected for wrong reason: %s", encoded)
	}
	after, _ := os.ReadDir(root)
	if len(before) != len(after) {
		t.Fatalf("schema rejection mutated repository: before=%v after=%v", names(before), names(after))
	}
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
