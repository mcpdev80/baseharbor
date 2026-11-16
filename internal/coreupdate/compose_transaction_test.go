package coreupdate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestComposeCheckpointStagesReplaysAndRestores(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "core-compose.yaml")
	original := []byte("services:\n  openbao-member-1:\n    image: openbao:2.6.0\n  extra:\n    image: operator:1\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	delta := Delta{Installed: Realization{Kind: Secrets, Installation: "c", Scope: "shared", Instance: "openbao-member-1", Owner: "baseharbor", Image: "openbao:2.6.0", Version: "2.6.0", Digest: digestA}, Desired: Desired{Kind: Secrets, Image: "openbao:2.7.0", Version: "2.7.0", Digest: digestB}, Classification: BackupRequired}
	selected := map[string]Delta{"openbao-member-1": delta}
	tx := ComposeCheckpoint{Path: path, Directory: filepath.Join(dir, "checkpoints")}
	if err := tx.Capture(selected); err != nil {
		t.Fatal(err)
	}
	if before, err := os.ReadFile(path); err != nil || !bytes.Equal(before, original) {
		t.Fatal("backup capture mutated the installed provider")
	}
	if err := tx.Capture(selected); err != nil {
		t.Fatal(err)
	}
	if err := tx.Stage(selected); err != nil {
		t.Fatal(err)
	}
	pinned, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(pinned, original) || !bytes.Contains(pinned, []byte("openbao:2.7.0@"+digestB)) {
		t.Fatalf("image not pinned: %s", pinned)
	}
	if err := tx.Stage(selected); err != nil {
		t.Fatalf("idempotent stage: %v", err)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, pinned) {
		t.Fatal("replay altered pinned image")
	}
	if err := tx.Restore(selected); err != nil {
		t.Fatal(err)
	}
	recovered, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recovered, original) {
		t.Fatalf("Compose not restored exactly: %s", recovered)
	}
	if err := tx.Restore(selected); err != nil {
		t.Fatal(err)
	}
}
func TestComposeCheckpointRefusesForeignEditsAndSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	original := []byte("services:\n  postgres-member-1:\n    image: postgres:18\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	delta := Delta{Installed: Realization{Kind: SQL, Installation: "c", Scope: "shared", Instance: "postgres-member-1", Owner: "baseharbor", Image: "postgres:18", Version: "18", Digest: digestA}, Desired: Desired{Kind: SQL, Image: "postgres:18", Version: "18", Digest: digestB}, Classification: BackupRequired}
	selected := map[string]Delta{"postgres-member-1": delta}
	tx := ComposeCheckpoint{Path: path, Directory: filepath.Join(dir, "recovery")}
	if err := tx.Stage(selected); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("services:\n  postgres-member-1:\n    image: operator-owned:99\n")
	if err := os.WriteFile(path, foreign, 0600); err != nil {
		t.Fatal(err)
	}
	if err := tx.Restore(selected); err == nil {
		t.Fatal("overwrote foreign operator edit")
	}
	if err := tx.Stage(selected); err == nil {
		t.Fatal("staged over foreign operator edit")
	}
	if err := tx.Capture(selected); err == nil {
		t.Fatal("captured foreign operator edit as original")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "recovery"), path); err != nil {
		t.Fatal(err)
	}
	if err := tx.Stage(selected); err == nil {
		t.Fatal("followed foreign symlink")
	}
}
