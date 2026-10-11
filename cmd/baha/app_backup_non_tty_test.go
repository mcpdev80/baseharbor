package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestNonTTYBackupDefaultUsesProtectedExternalArchive(t *testing.T) {
	configureTestTarget(t)
	repo := t.TempDir()
	t.Chdir(repo)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m := application.New("demo", "dev", true, false, false)
	if err := os.WriteFile(filepath.Join(repo, application.RepositoryManifestName), []byte(m.YAML()), 0644); err != nil {
		t.Fatal(err)
	}
	var report bytes.Buffer
	args, err := prepareNonInteractiveBackupOutput(context.Background(), application.DefaultStore(), []string{"--password-file", "/tmp/example-password-file"}, &report)
	if err != nil {
		t.Fatal(err)
	}
	path := backupOutputFlag(args)
	if path == "" || !filepath.IsAbs(path) {
		t.Fatalf("non-TTY did not get absolute destination: %v", args)
	}
	relative, err := filepath.Rel(repo, path)
	if err != nil {
		t.Fatal(err)
	}
	if relative == "." || !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		t.Fatalf("backup inside repo: %s", path)
	}
	if !strings.Contains(report.String(), path) {
		t.Fatalf("destination not visible: %s", report.String())
	}
}

func TestExplicitInRepoBackupDestinationIsNotRewritten(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)
	output := filepath.Join(repo, "manual.bhbackup")
	var report bytes.Buffer
	args := []string{"--password-file", "/tmp/password-file", "--output", output}
	got, err := prepareNonInteractiveBackupOutput(context.Background(), application.DefaultStore(), args, &report)
	if err != nil {
		t.Fatal(err)
	}
	if backupOutputFlag(got) != output {
		t.Fatalf("explicit output changed: %v", got)
	}
	if !strings.Contains(report.String(), "WARNING") {
		t.Fatalf("missing in-repository warning: %s", report.String())
	}
}
