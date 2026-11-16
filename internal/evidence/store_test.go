package evidence

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditStoreOwnerOnlyAndScoped(t *testing.T) {
	root := t.TempDir()
	first := AuditEvent{
		SchemaVersion: SchemaVersion, ID: "one",
		Timestamp: time.Date(2026, 9, 26, 20, 0, 0, 0, time.UTC),
		Actor:     Actor{Interface: "cli", Identity: "local-operator"},
		Target:    "local", Application: "demo", Environment: "dev",
		Operation: "apply", Outcome: "success",
	}
	second := first
	second.ID = "two"
	second.Application = "other"
	if err := Append(root, first); err != nil {
		t.Fatal(err)
	}
	if err := Append(root, second); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, auditDirectory, auditFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("audit mode = %o", info.Mode().Perm())
	}
	events, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	filtered := Filter(events, "local", "demo", "dev")
	if len(filtered) != 1 || filtered[0].ID != "one" {
		t.Fatalf("unexpected filtered audit: %#v", filtered)
	}
}
