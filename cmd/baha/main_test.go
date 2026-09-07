package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"init"}); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	path := filepath.Join(dir, "baseharbor.yaml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("config not created: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected permissions: %o", info.Mode().Perm())
	}
}

func TestUnknownCommandFails(t *testing.T) {
	if err := run([]string{"does-not-exist"}); err == nil {
		t.Fatal("expected unknown command to fail")
	}
}
