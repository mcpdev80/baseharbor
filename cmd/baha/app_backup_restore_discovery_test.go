package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProtectedRestoreDiscoveryAndSelection(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	base, err := defaultGuidedBackupPath("webshop", "dev", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(base)
	newer := filepath.Join(dir, "webshop-dev-20261008T120000Z.bhbackup")
	older := filepath.Join(dir, "webshop-dev-20261007T120000Z.bhbackup")
	other := filepath.Join(dir, "world-readable.bhbackup")
	for _, path := range []string{newer, older} {
		if err := os.WriteFile(path, []byte("encrypted test fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(newer, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(older, yesterday, yesterday); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("not private"), 0644); err != nil {
		t.Fatal(err)
	}
	list, err := discoverGuidedBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Path != newer || list[1].Path != older {
		t.Fatalf("unexpected candidate order/scope: %v", list)
	}
	var output bytes.Buffer
	selected, err := promptGuidedBackupToRestore(strings.NewReader("2\n"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if selected != older {
		t.Fatalf("expected older selected %s, got %s", older, selected)
	}
	if !strings.Contains(output.String(), newer) {
		t.Fatalf("missing visible restore candidate: %s", output.String())
	}
}

func TestRestoreDiscoveryWithoutBackupsHasRemediation(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	_, err := promptGuidedBackupToRestore(strings.NewReader("1\n"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no saved BaseHarbor archives") {
		t.Fatalf("expected guidance for missing backup: %v", err)
	}
}
