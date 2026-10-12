package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestImpactContentRevisionDetectsSecretChangeWithRestoredMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "runtime.env")
	before := "password=first-secret\n"
	after := "password=other-secret\n"
	if err := os.WriteFile(path, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	observe := func() application.ProviderImpactObservation {
		materials, complete, err := observeOwnedProviderFiles("target/local", "local", "one", root)
		if err != nil || !complete {
			t.Fatal(err)
		}
		return application.ProviderImpactObservation{Providers: map[string]application.ProviderImpactFact{"one": {Materials: materials}}}
	}
	first := observe()
	firstRevision, err := applicationImpactFileRevision(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(after), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	second := observe()
	firstMaterial, _ := json.Marshal(first)
	secondMaterial, _ := json.Marshal(second)
	if string(firstMaterial) != string(secondMaterial) {
		t.Fatal("fixture must preserve previously exposed metadata")
	}
	secondRevision, err := applicationImpactFileRevision(second)
	if err != nil || firstRevision == secondRevision {
		t.Fatal("changed secret content kept stale consent", err)
	}
	for _, output := range []string{string(firstMaterial), string(secondMaterial), firstRevision, secondRevision} {
		if strings.Contains(output, "first-secret") || strings.Contains(output, "other-secret") {
			t.Fatal("secret leaked into impact evidence")
		}
	}
}

func TestImpactContentRevisionCannotFollowChangedRecoveryOwner(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "runtime.env")
	if err := os.WriteFile(path, []byte("retained secret"), 0600); err != nil {
		t.Fatal(err)
	}
	location := &application.ResourceLocation{Kind: "path", Reference: path}
	o := application.ProviderImpactObservation{Providers: map[string]application.ProviderImpactFact{"one": {Materials: []application.ProviderMaterial{{Ownership: capability.OwnershipBaseHarbor, Verified: true, Location: location}}}}}
	if _, err := applicationImpactFileRevision(o); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(root, "foreign")); err != nil {
		t.Fatal(err)
	}
	if _, err := applicationImpactFileRevision(o); err == nil {
		t.Fatal("changed linked owner retained acknowledgement authority")
	}
}
