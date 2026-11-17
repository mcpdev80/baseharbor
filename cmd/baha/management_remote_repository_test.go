package main

import (
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestRemoveStaleRemoteRepositoryIdentity(t *testing.T) {
	owner := application.Store{Root: filepath.Join(t.TempDir(), ".baseharbor", "apps")}
	manifest := application.New("demo", "dev", false, false, false)
	manifest.Services.SQL = true
	manifest.ApplicationID = "11111111-1111-4111-8111-111111111111"
	if _, err := owner.Create(manifest); err != nil {
		t.Fatal(err)
	}
	if err := removeStaleRemoteRepositoryIdentity(owner, manifest.Name, manifest.ApplicationID); err != nil {
		t.Fatal(err)
	}
	items, err := owner.List()
	if err != nil || len(items) != 0 {
		t.Fatalf("stale repository identity remains: items=%#v err=%v", items, err)
	}
}

func TestRemoveStaleRemoteRepositoryIdentityPreservesConcurrentReplacement(t *testing.T) {
	owner := application.Store{Root: filepath.Join(t.TempDir(), ".baseharbor", "apps")}
	manifest := application.New("demo", "dev", false, false, false)
	manifest.Services.SQL = true
	manifest.ApplicationID = "22222222-2222-4222-8222-222222222222"
	if _, err := owner.Create(manifest); err != nil {
		t.Fatal(err)
	}
	if err := removeStaleRemoteRepositoryIdentity(owner, manifest.Name, "11111111-1111-4111-8111-111111111111"); err == nil {
		t.Fatal("changed identity was removed")
	}
	if _, _, err := owner.Load(manifest.Name); err != nil {
		t.Fatalf("repository identity was removed after its value changed: %v", err)
	}
}
