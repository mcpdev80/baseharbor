package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveNewApplicationRootUsesParentAndAppendsName(t *testing.T) {
	parent := t.TempDir()
	got, err := resolveNewApplicationRoot("catalog-api", parent)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(parent, "catalog-api")
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}

func TestResolveNewApplicationRootRejectsAmbiguousCurrentDirectory(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(original) }()

	if _, err := resolveNewApplicationRoot("different-name", ""); err == nil {
		t.Fatal("ambiguous current directory unexpectedly accepted")
	}
}

func TestResolveNewApplicationRootAllowsMatchingEmptyCurrentDirectory(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "catalog-api")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(original) }()

	got, err := resolveNewApplicationRoot("catalog-api", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("root = %q, want %q", got, dir)
	}
}

func TestResolveNewApplicationRootRejectsMatchingNonEmptyCurrentDirectory(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "catalog-api")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(original) }()

	if _, err := resolveNewApplicationRoot("catalog-api", ""); err == nil {
		t.Fatal("non-empty current directory unexpectedly accepted")
	}
}
