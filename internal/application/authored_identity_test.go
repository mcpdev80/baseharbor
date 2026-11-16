package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthoredIdentityRejectedBeforeStateMutation(t *testing.T) {
	for _, id := range []string{"", contractAppID, "not-a-uuid"} {
		input := "version: 1\napp:\n  id: " + id + "\n  name: demo\n  environment: dev\n"
		root := t.TempDir()
		path := filepath.Join(root, RepositoryManifestName)
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadManifestFile(path); err == nil || !strings.Contains(err.Error(), "system-owned") {
			t.Fatalf("authored ID %q accepted: %v", id, err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 1 {
			t.Fatalf("intake materialized state: %v", err)
		}
		after, _ := os.ReadFile(path)
		if string(after) != input {
			t.Fatal("intake modified authoring")
		}
	}
}

func TestCanonicalIdentityAndAuthoredSerializationBoundary(t *testing.T) {
	m := New("demo", "dev", true, false, false)
	canonical, err := ParseCanonicalYAML(m.YAML())
	if err != nil || canonical.ApplicationID != m.ApplicationID {
		t.Fatalf("internal identity lost: %v", err)
	}
	if _, err := ParseYAML(m.YAML()); err == nil {
		t.Fatal("canonical record accepted as developer input")
	}
	for i := 0; i < 3; i++ {
		authored, err := ParseYAML(m.AuthoredYAML())
		if err != nil || authored.ApplicationID != "" || !authored.Services.SQL {
			t.Fatalf("authoring leaked ID or lost intent: %v", err)
		}
	}
	if m.ApplicationID != canonical.ApplicationID {
		t.Fatal("serialization changed system identity")
	}
}

func TestCanonicalRecordMissingIdentityFailsWithoutAllocation(t *testing.T) {
	input := "version: 1\napp:\n  name: demo\n  environment: dev\nservices:\n  sql: true\n"
	if _, err := ParseCanonicalYAML(input); err == nil {
		t.Fatal("corrupt internal identity accepted")
	}
	m, err := ParseYAML(input)
	if err != nil || m.ApplicationID != "" {
		t.Fatalf("authored identity allocated: %v", err)
	}
}
