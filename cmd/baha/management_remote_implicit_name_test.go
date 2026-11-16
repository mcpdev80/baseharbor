package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRemoteApplyUsesOnlyLocalRepositoryNameWhenUnambiguous(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if _, _, _, err := remoteMutationArgs("apply", nil); err == nil {
		t.Fatal("nameless remote apply without repository identity was accepted")
	}
	local := application.New("demo", "dev", false, false, false)
	if err := os.WriteFile(filepath.Join(root, application.RepositoryManifestName),
		[]byte(local.AuthoredYAML()), 0600); err != nil {
		t.Fatal(err)
	}
	name, env, payload, err := remoteMutationArgs("apply", nil)
	if err != nil {
		t.Fatalf("valid local repository selector rejected: %v", err)
	}
	if name != "demo" || env != "dev" {
		t.Fatalf("remote selector = %q/%q; want demo/dev", name, env)
	}
	var received struct {
		Name        string `json:"name"`
		Environment string `json:"environment"`
	}
	if err := json.Unmarshal(payload, &received); err != nil {
		t.Fatal(err)
	}
	if received.Name != name || received.Environment != env {
		t.Fatalf("remote payload selectors do not match resolved intent: %+v", received)
	}
	if _, _, _, err := remoteMutationArgs("apply", []string{"--environment", "prod"}); err == nil {
		t.Fatal("remote apply accepted a foreign environment from the local repository")
	}
	// An explicit remote name stays authoritative: do not implicitly upload
	// or substitute local repository identity for a different selected app.
	explicit, _, _, err := remoteMutationArgs("apply", []string{"other-app"})
	if err != nil || explicit != "other-app" {
		t.Fatalf("explicit remote application selection was overwritten: %q, %v", explicit, err)
	}
}
