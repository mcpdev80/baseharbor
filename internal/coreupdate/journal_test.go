package coreupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJournalDurableTransitionsAndResume(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "update.json")
	d := Delta{Installed: Realization{Kind: SQL, Installation: "core", Scope: "shared", Instance: "sql"}}
	j, err := LoadJournal(path, "0.4.24")
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Record(path, d, "applying"); err != nil {
		t.Fatal(err)
	}
	restored, err := LoadJournal(path, "0.4.24")
	if err != nil {
		t.Fatal(err)
	}
	if restored.Steps[JournalKey(d)] != "applying" {
		t.Fatal("interrupted update progress lost")
	}
	if err := restored.Record(path, d, "verify_failed"); err != nil {
		t.Fatal(err)
	}
	if err := restored.Record(path, d, "verified"); err != nil {
		t.Fatal(err)
	}
	if err := restored.Record(path, d, "applying"); err == nil {
		t.Fatal("verified provider regressed")
	}
	final, err := LoadJournal(path, "0.4.24")
	if err != nil {
		t.Fatal(err)
	}
	if final.Steps[JournalKey(d)] != "verified" {
		t.Fatal("journal changed after rejected transition")
	}
	if _, err := LoadJournal(path, "0.4.25"); err == nil {
		t.Fatal("cross-release journal reuse permitted")
	}
}
func TestJournalFailsClosedOnPermissionsAndInvalidState(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "journal")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadJournal(path, "0.4.24"); err == nil {
		t.Fatal("accepted world-readable journal")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadJournal(path, "0.4.24"); err == nil {
		t.Fatal("accepted malformed state")
	}
	d := Delta{Installed: Realization{Kind: Secrets, Installation: "a", Scope: "shared", Instance: "vault"}}
	j := Journal{Release: "0.4.24"}
	if err := j.Record(path, d, "unsupported"); err == nil {
		t.Fatal("accepted unknown journal state")
	}
}
