package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteExclusiveOnboardingProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	original := []byte("{\"version\":\"first\"}\n")
	if err := writeExclusiveOnboardingProfile(path, original); err != nil {
		t.Fatalf("first profile creation failed: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("profile must remain 0600, got %04o", info.Mode().Perm())
	}
	if err := writeExclusiveOnboardingProfile(path, []byte("overwrite")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing profile was not rejected: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatal("existing remote profile was changed")
	}
}

func TestWriteExclusiveOnboardingProfileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("protected"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "profile.json")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusiveOnboardingProfile(link, []byte("evil")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("symlink profile was not rejected: %v", err)
	}
	data, err := os.ReadFile(victim)
	if err != nil || string(data) != "protected" {
		t.Fatalf("symlink target changed: %v", err)
	}
}

func TestPrivateOnboardingDirectoryPreflight(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "not-created")
	if err := checkPrivateOnboardingDirectory(missing); err != nil {
		t.Fatalf("a missing directory is valid before creation: %v", err)
	}
	private := filepath.Join(root, "private")
	if err := os.Mkdir(private, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := checkPrivateOnboardingDirectory(private); err != nil {
		t.Fatalf("private directory rejected: %v", err)
	}
	if err := os.Chmod(private, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkPrivateOnboardingDirectory(private); err == nil {
		t.Fatal("public identity directory accepted")
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if err := checkPrivateOnboardingDirectory(link); err == nil {
		t.Fatal("symlinked remote profile directory accepted")
	}
}

func TestEnsurePrivateOnboardingDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "private")
	if err := ensurePrivateOnboardingDirectory(dir); err != nil {
		t.Fatalf("create private directory: %v", err)
	}
	info, err := os.Lstat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("created directory is not 0700: %v %v", info, err)
	}
	if err := ensurePrivateOnboardingDirectory(dir); err != nil {
		t.Fatalf("reuse owned directory: %v", err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateOnboardingDirectory(dir); err == nil {
		t.Fatal("permissive directory accepted")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateOnboardingDirectory(link); err == nil {
		t.Fatal("symlinked directory accepted")
	}
}

func TestEnsurePrivateOnboardingDirectoryTreeCreatesFreshXDGPath(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, ".local", "share", "baseharbor")
	if err := ensurePrivateOnboardingDirectoryTree(dataRoot); err != nil {
		t.Fatalf("create fresh data-root hierarchy: %v", err)
	}
	info, err := os.Lstat(dataRoot)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("fresh data root is not a private real directory: info=%v err=%v", info, err)
	}
	for _, child := range []string{filepath.Join(dataRoot, "remote-connections"), filepath.Join(dataRoot, "remote-workstations")} {
		if err := ensurePrivateOnboardingDirectory(child); err != nil {
			t.Fatalf("create child below new data root: %v", err)
		}
	}
}

func TestEnsurePrivateOnboardingDirectoryTreeRejectsSymlinkAncestor(t *testing.T) {
	root := t.TempDir()
	victim := filepath.Join(root, "victim")
	if err := os.Mkdir(victim, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateOnboardingDirectoryTree(filepath.Join(link, "remote-connections")); err == nil {
		t.Fatal("symlinked data-root ancestor accepted")
	}
}
