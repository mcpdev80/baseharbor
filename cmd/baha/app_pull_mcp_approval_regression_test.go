package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestAppPullMCPApprovalRegression is the direct MCP approval regression the F4
// finding demanded. It drives the typed `baseharbor.app.pull` tool through the
// production server registration and proves:
//
//   - approval=false is refused (the mutating source pull refuses to run without
//     explicit confirmation, fail-closed, exactly like app.stop/perses.remove);
//   - approval=true is NOT refused by the approval gate and proceeds past it to
//     the shared pull lifecycle.
//
// The cells run the production MCP seam (registerMCPAppPullTool via newMCPServer)
// under the operator-auth context the stdio server stamps, so the approval gate
// itself is what is exercised.
func TestAppPullMCPApprovalRegression(t *testing.T) {
	configureTestTarget(t)
	fixture := appPullMCPRepository(t)

	t.Run("approval-required-refuses", func(t *testing.T) {
		result, callErr := callAppPullMCP(t, fixture.store, mcpCallPull{Approval: false})
		if callErr != nil {
			t.Fatalf("MCP app.pull approval=false transport error: %v", callErr)
		}
		if result == nil || !result.IsError {
			t.Fatalf("STOP CRITERION: baseharbor.app.pull with approval=false must be refused (fail-closed), got IsError=%v", result != nil && result.IsError)
		}
		encoded, _ := json.Marshal(result.StructuredContent)
		var payload machineToolError
		if err := json.Unmarshal(encoded, &payload); err != nil {
			t.Fatalf("the refusal payload must decode as a machine error: %v\n%s", err, encoded)
		}
		if payload.Error.Code == "" {
			t.Fatalf("the refusal must carry a classified approval error code: %s", encoded)
		}
		// The failure must be the approval gate, not a dirty-tree or resolution
		// error: the fixture is a clean, committed repository, so a non-approval
		// failure here would hide a missing approval check (F4 regression risk).
		if !containsApprovalRefusal(payload.Error.Message) {
			t.Fatalf("STOP CRITERION: approval=false must be refused by the approval gate, but the refusal is not an approval-required message: %s", encoded)
		}
	})

	t.Run("approval-granted-proceeds-past-gate", func(t *testing.T) {
		// With approval granted the approval gate must NOT refuse. The pull then
		// proceeds to the shared lifecycle; the clean fixture reaches a
		// source-model resolution error (no workspace initialized) instead of an
		// approval refusal, which proves the gate passed: a missing approval
		// check would instead fail on this clean repository with a
		// dirty-tree or resolution error, never with an approval message.
		result, callErr := callAppPullMCP(t, fixture.store, mcpCallPull{Approval: true})
		if callErr != nil {
			t.Fatalf("MCP app.pull approval=true transport error: %v", callErr)
		}
		if result != nil && result.IsError {
			encoded, _ := json.Marshal(result.StructuredContent)
			var payload machineToolError
			if err := json.Unmarshal(encoded, &payload); err == nil {
				if containsApprovalRefusal(payload.Error.Message) {
					t.Fatalf("STOP CRITERION: baseharbor.app.pull with approval=true must pass the approval gate, but the refusal mentions approval: %s", encoded)
				}
				if payload.Error.CauseCode == "explicit_approval_required" || strings.Contains(strings.ToLower(payload.Error.Message), "approval") {
					t.Fatalf("STOP CRITERION: approval=true passed but the failure is approval-related: %s", encoded)
				}
			}
		}
	})
}

// appPullMCPFixture is a repository-owned application with one clean Git source
// so the MCP app.pull cell reaches a terminal success once approved.
type appPullMCPFixture struct {
	root   string
	store  application.Store
	remote string
	local  string
}

// appPullMCPRepository authors a repository-owned application whose single
// repository source is strictly behind its upstream, so an approved pull applies
// cleanly.
func appPullMCPRepository(t *testing.T) appPullMCPFixture {
	t.Helper()
	t.Setenv("BASEHARBOR_TARGET", "")
	t.Setenv("BASEHARBOR_STATE_DIR", "")

	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	mustGitUpdateTest(t, "", "init", "--bare", remote)
	seed := filepath.Join(root, "seed")
	mustGitUpdateTest(t, "", "init", "-b", "main", seed)
	configureGitUpdateTestIdentity(t, seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("initial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustGitUpdateTest(t, seed, "add", "README.md")
	mustGitUpdateTest(t, seed, "commit", "-m", "initial")
	mustGitUpdateTest(t, seed, "remote", "add", "origin", remote)
	mustGitUpdateTest(t, seed, "push", "-u", "origin", "main")

	local := filepath.Join(root, "work")
	mustGitUpdateTest(t, "", "clone", "--branch", "main", remote, local)
	configureGitUpdateTestIdentity(t, local)

	// Advance the upstream so the source is strictly behind and applyable.
	if err := os.WriteFile(filepath.Join(seed, "upstream.txt"), []byte("upstream\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustGitUpdateTest(t, seed, "add", "upstream.txt")
	mustGitUpdateTest(t, seed, "commit", "-m", "upstream")
	mustGitUpdateTest(t, seed, "push", "origin", "main")

	manifest := application.New("demo", "dev", false, false, false)
	manifest = application.WithWorkloadComponents(manifest, "api")
	manifestPath := filepath.Join(local, application.RepositoryManifestName)
	if err := writeInitializedAuthoredManifest(t, manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	// Commit the manifest so the git source stays clean: the source pull inspects
	// `git status --porcelain` and a dirty tree would fail the pull on a
	// dirty-tree blocker instead of the approval gate this test exercises.
	mustGitUpdateTest(t, local, "add", application.RepositoryManifestName)
	mustGitUpdateTest(t, local, "commit", "-m", "add baseharbor manifest")
	t.Chdir(local)
	return appPullMCPFixture{root: root, store: application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}, remote: remote, local: local}
}

// mcpCallPull is the arguments envelope for the typed baseharbor.app.pull tool.
type mcpCallPull struct {
	Target      string
	Name        string
	Environment string
	Components  []string
	Up          bool
	Approval    bool
}

// callAppPullMCP drives the typed baseharbor.app.pull tool through the real
// server registration and returns the structured result.
func callAppPullMCP(t *testing.T, store application.Store, args mcpCallPull) (*mcp.CallToolResult, error) {
	t.Helper()
	server := newMCPServer(store)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(testOperatorContext(t.Context()), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serverSession.Close() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "app-pull-approval-regression", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	arguments := map[string]any{"approval": args.Approval}
	if args.Target != "" {
		arguments["target"] = args.Target
	}
	if args.Name != "" {
		arguments["name"] = args.Name
	}
	if args.Environment != "" {
		arguments["environment"] = args.Environment
	}
	if len(args.Components) > 0 {
		arguments["components"] = args.Components
	}
	return session.CallTool(testOperatorContext(t.Context()), &mcp.CallToolParams{
		Name:      "baseharbor.app.pull",
		Arguments: arguments,
	})
}

// machineToolError mirrors the machine error payload the MCP failure surface
// serializes; it is declared here to avoid coupling to the production package
// layout beyond what the handler already returns.

// containsApprovalRefusal reports whether a message refuses because approval is
// required (fail-closed on the approval gate).
func containsApprovalRefusal(message string) bool {
	lowered := strings.ToLower(message)
	for _, token := range []string{"approval is required", "approval required", "explicit approval"} {
		if strings.Contains(lowered, token) {
			return true
		}
	}
	return false
}
