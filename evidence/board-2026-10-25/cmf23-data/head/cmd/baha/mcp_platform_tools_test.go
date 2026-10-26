package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNewPlatformToolsDenyManagedOperationsBeforeRuntimeOrInput(t *testing.T) {
	configureTestTarget(t)
	manifest := workspaceMutationFixture(t, "prod")
	t.Chdir(filepath.Dir(manifest))
	session := workspaceMutationClient(t)
	for _, id := range []string{"control-plane.status", "control-plane.doctor", "control-plane.up", "control-plane.stop", "control-plane.repair", "control-plane.destroy", "installation.destroy", "openbao.status", "openbao.bootstrap", "openbao.unseal", "openbao.rotate", "dev.domain", "dev.credentials", "trust.status", "trust.export", "trust.install", "release.check"} {
		arguments := map[string]any{}
		switch id {
		case "control-plane.destroy", "installation.destroy", "trust.install":
			arguments["approval"] = true
		case "openbao.bootstrap", "openbao.unseal", "openbao.rotate":
			arguments["recovery_file"] = "missing-sensitive-input"
		case "dev.credentials":
			arguments["password_file"] = "missing-sensitive-input"
		case "trust.export":
			arguments["path"] = filepath.Join(t.TempDir(), "public-ca")
		}
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor." + id, Arguments: arguments})
		if err != nil || !result.IsError {
			t.Fatalf("%s not denied: %#v %v", id, result, err)
		}
		encoded, _ := json.Marshal(result)
		if !bytes.Contains(encoded, []byte("authentication_failed")) || bytes.Contains(encoded, []byte("missing-sensitive-input")) {
			t.Fatalf("%s performed pre-auth work: %s", id, encoded)
		}
	}
}

func TestPlatformDestructionAndHostTrustNeedApprovalBeforeRuntime(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(t.TempDir())
	session := workspaceMutationClient(t)
	for _, id := range []string{"control-plane.destroy", "installation.destroy", "trust.install"} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor." + id, Arguments: map[string]any{"approval": false}})
		if err != nil || !result.IsError {
			t.Fatalf("%s accepted without approval", id)
		}
		encoded, _ := json.Marshal(result)
		if !bytes.Contains(encoded, []byte("approval_required")) {
			t.Fatalf("%s checked runtime before approval: %s", id, encoded)
		}
	}
}

func TestDevelopmentCredentialsMCPNeverReturnsProtectedValue(t *testing.T) {
	target := configureTestTarget(t)
	t.Chdir(t.TempDir())
	protected := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(protected, []byte("NEVER_EXPOSE_DEVELOPMENT_PASSWORD\n"), 0600); err != nil {
		t.Fatal(err)
	}
	session := workspaceMutationClient(t)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.dev.credentials", Arguments: map[string]any{"username": "developer", "password_file": protected}})
	if err != nil || result.IsError {
		t.Fatalf("MCP development configuration failed: %#v %v", result, err)
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte("NEVER_EXPOSE")) || !bytes.Contains(encoded, []byte("protected_file")) {
		t.Fatalf("credential result leaked: %s", encoded)
	}
	stored, err := devaccess.Load(target.Name, "dev")
	if err != nil || stored.Username != "developer" || stored.Password != "NEVER_EXPOSE_DEVELOPMENT_PASSWORD" {
		t.Fatalf("credential authority changed input: %v", err)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor.dev.credentials", Arguments: map[string]any{"reset": true}})
	if err != nil || result.IsError {
		t.Fatal("credential reset failed")
	}
	replacement, err := devaccess.Load(target.Name, "dev")
	if err != nil || replacement.Password == stored.Password {
		t.Fatal("credential reset did not rotate authority")
	}
	encoded, _ = json.Marshal(result)
	if bytes.Contains(encoded, []byte(replacement.Password)) {
		t.Fatal("rotated credential leaked")
	}
}
