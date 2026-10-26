package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestDestructionPresentationDoesNotRewriteProtectedWorkloadBeforeConsent(t *testing.T) {
	r := retainedNativeBackendFixture(t)
	r.Manifest.Services = application.Services{}
	e, err := newApplicationDestroyExecution(t.Context(), r.Store, nil, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	e.manifest, e.resolved.Manifest = r.Manifest, r.Manifest
	path := filepath.Join(e.files.Dir, "workload.override.yaml")
	original := []byte("protected original applied workload metadata\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.renderDeletePlan(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(original) || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("read-only destruction presentation rewrote protected applied workload")
	}
}
