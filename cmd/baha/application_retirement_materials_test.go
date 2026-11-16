package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestProviderOnlyRetirementDoesNotInventRuntimeMaterial(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "runtime")
	e := applicationDestroyExecution{files: application.RuntimeFiles{Dir: missing}, runtimeErr: application.ErrRuntimeNotApplied}
	retained, err := e.observeRetirementRuntimeMaterials()
	if err != nil || len(retained) != 0 {
		t.Fatalf("absent projection cannot retire: %v", err)
	}
	if _, err := os.Lstat(missing); !os.IsNotExist(err) {
		t.Fatal("inventory created runtime state")
	}
	e.runtimeErr = nil
	if _, err := e.observeRetirementRuntimeMaterials(); err == nil {
		t.Fatal("lost applied projection was accepted")
	}
	e.runtimeErr = application.ErrRuntimeNotApplied
	linkedParent := filepath.Join(root, "redirected")
	if err := os.Symlink(filepath.Join(root, "missing-parent"), linkedParent); err != nil {
		t.Fatal(err)
	}
	e.files.Dir = filepath.Join(linkedParent, "runtime")
	if _, err := e.observeRetirementRuntimeMaterials(); err == nil {
		t.Fatal("dangling ancestor treated as absent runtime owner")
	}
	e.files.Dir = missing
	foreign := filepath.Join(root, "foreign")
	if err := os.WriteFile(foreign, []byte("preserved"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, missing); err != nil {
		t.Fatal(err)
	}
	retained, err = e.observeRetirementRuntimeMaterials()
	if err != nil || len(retained) != 1 || retained[0].Verified {
		t.Fatal("linked existing projection treated as absent or verified")
	}
	b, err := os.ReadFile(foreign)
	if err != nil || string(b) != "preserved" {
		t.Fatal("foreign material changed")
	}
}
