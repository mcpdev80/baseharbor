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
