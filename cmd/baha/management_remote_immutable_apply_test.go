package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestRemoteApplyNeverPackagesDirtyWorkstationSource(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	manifest := application.New("demo", "dev", false, false, false)
	manifestPath := filepath.Join(root, application.RepositoryManifestName)
	if err := os.WriteFile(manifestPath, []byte(manifest.AuthoredYAML()), 0600); err != nil {
		t.Fatal(err)
	}
	// A file that is not part of the registered Git candidate must never
	// appear in the machine HTTP request, even if it exists in the checkout.
	secret := "untracked-workstation-marker-not-authorized-for-core"
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	model := development.SourceModel{
		SchemaVersion: development.SourceModelVersion, Application: "demo",
		Sources: []development.SourceDefinition{{
			ID: "app", Type: development.SourceRepository,
			Repository: "https://git.example/demo.git", Ref: strings.Repeat("a", 40),
		}},
		Components: []development.ComponentSource{{Component: "app", Source: "app"}},
	}
	if _, err := development.WriteSourceModel(manifestPath, model); err != nil {
		t.Fatal(err)
	}
	_, _, base, err := remoteMutationArgs("apply", []string{"demo"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := remoteImmutableApplyPayload("demo", "dev", base)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte(secret)) || bytes.Contains(payload, []byte("source_snapshot_digest")) {
		t.Fatal("remote apply transferred local worktree bytes or snapshot authority")
	}
	var received struct {
		SourceModel *development.SourceModel `json:"source_model"`
	}
	if err := json.Unmarshal(payload, &received); err != nil || received.SourceModel == nil ||
		received.SourceModel.Sources[0].Ref != strings.Repeat("a", 40) {
		t.Fatalf("immutable Git source selection missing from remote request: %v", err)
	}
	model.Sources[0].Ref = "main"
	if _, err := development.WriteSourceModel(manifestPath, model); err != nil {
		t.Fatal(err)
	}
	if _, err := remoteImmutableApplyPayload("demo", "dev", base); err == nil {
		t.Fatal("mutable Git branch was accepted for remote apply")
	}
}
