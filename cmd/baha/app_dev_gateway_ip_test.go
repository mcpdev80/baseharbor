package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagementIPAccessUsesOwningCoreEndpoint(t *testing.T) {
	target := configureTestTarget(t)
	// A Core must not accidentally read the workstation's connection profile.
	t.Setenv("BASEHARBOR_CONNECTION_PROFILE", "/missing/workstation/profile")
	address, err := remoteCoreIPAccessAddress(target)
	if err != nil || address != "" {
		t.Fatalf("missing endpoint: %q %v", address, err)
	}
	path, err := ownedCoreLANEndpointPath(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	payload := `{"version":"baseharbor.core-lan-endpoint/v1","target":"docker-dev","url":"https://192.168.50.5:34491"}`
	if err := os.WriteFile(path, []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	address, err = remoteCoreIPAccessAddress(target)
	if err != nil || address != "192.168.50.5" {
		t.Fatalf("owned endpoint not used: %q %v", address, err)
	}
	if err := os.WriteFile(path, []byte(`{"version":"baseharbor.core-lan-endpoint/v1","target":"foreign","url":"https://192.168.50.5:34491"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := remoteCoreIPAccessAddress(target); err == nil {
		t.Fatal("foreign Core endpoint accepted")
	}
}
