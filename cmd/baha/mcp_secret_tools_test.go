package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"testing"
)

func TestSecretToolsDenyManagedOperationsBeforeProtectedInput(t *testing.T) {
	configureTestTarget(t)
	manifest := workspaceMutationFixture(t, "prod")
	t.Chdir(filepath.Dir(manifest))
	session := workspaceMutationClient(t)
	for _, id := range []string{"secret.list", "secret.set", "secret.delete", "secret.tls-set"} {
		input := map[string]any{}
		switch id {
		case "secret.set":
			input["key"] = "API_TOKEN"
			input["file"] = "missing-private-input"
		case "secret.delete":
			input["key"] = "API_TOKEN"
			input["approval"] = true
		case "secret.tls-set":
			input["certificate_file"] = "missing-cert"
			input["private_key_file"] = "missing-key"
		}
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "baseharbor." + id, Arguments: input})
		if err != nil || !result.IsError {
			t.Fatalf("%s accepted: %#v %v", id, result, err)
		}
		payload, _ := json.Marshal(result)
		if !bytes.Contains(payload, []byte("authentication_failed")) || bytes.Contains(payload, []byte("missing-private-input")) {
			t.Fatalf("%s denial leaked or read protected input: %s", id, payload)
		}
	}
}
func TestProtectedSecretInputPreservesBytesAndRejectsUnsafeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	secret := []byte("DO_NOT_EXPOSE\nexact bytes\n")
	if err := os.WriteFile(path, secret, 0600); err != nil {
		t.Fatal(err)
	}
	value, err := readProtectedSecretInput(path)
	if err != nil || !bytes.Equal(value, secret) {
		t.Fatalf("protected input changed: %v", err)
	}
	zeroBytes(value)
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readProtectedSecretInput(path); err == nil {
		t.Fatal("publicly readable secret accepted")
	}
	link := path + "-link"
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readProtectedSecretInput(link); err == nil {
		t.Fatal("symlink accepted")
	}
}
