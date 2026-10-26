package providertopology

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/availability"
)

func TestExplicitIntentAndRetainedDatastoreTopology(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compose.yaml")
	for _, intent := range [][]availability.Requirement{nil, {{HA: false}}, {{HA: false, Instances: 1}}} {
		count, err := ResolveMembers(path, "data", 3, intent...)
		if err != nil || count != 1 {
			t.Fatalf("implicit HA: members=%d error=%v", count, err)
		}
	}
	count, err := ResolveMembers(path, "data", 3, availability.Requirement{HA: true})
	if err != nil || count != 3 {
		t.Fatalf("explicit HA: %d %v", count, err)
	}
	count, err = ResolveMembers(path, "data", 3, availability.Requirement{HA: true, Instances: 5})
	if err != nil || count != 5 {
		t.Fatalf("explicit member override: %d %v", count, err)
	}
	if _, err := ResolveMembers(path, "data", 3, availability.Requirement{Instances: 3}); err == nil {
		t.Fatal("member count silently enabled provider HA")
	}
	original := []byte("services:\n  data-1: {}\n  data-2: {}\n  data-3: {}\n  data-access: {}\n  data-admin: {}\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	count, err = ResolveMembers(path, "data", 3)
	if err != nil || count != 3 {
		t.Fatalf("maintenance changed retained topology: %d %v", count, err)
	}
	if _, err := ResolveMembers(path, "data", 3, availability.Requirement{}); err == nil {
		t.Fatal("silent datastore downgrade admitted")
	}
	if _, err := ResolveMembers(path, "data", 3, availability.Requirement{HA: true, Instances: 5}); err == nil {
		t.Fatal("silent datastore expansion admitted")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Fatal("rejected transition changed retained state")
	}
}

func TestUnsafeOrIncompleteRetainedTopologyFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compose.yaml")
	for _, data := range []string{"services:\n  data-1: {}\n  data-3: {}\n", "services:\n  data-admin: {}\n", "services: [invalid]"} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ExistingMembers(path, "data"); err == nil {
			t.Fatal("unverifiable topology accepted")
		}
	}
	if err := os.WriteFile(path, []byte("services:\n  data-1: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingMembers(path, "data"); err == nil {
		t.Fatal("unprotected topology accepted")
	}
	link := path + "-link"
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ExistingMembers(link, "data"); err == nil {
		t.Fatal("symlink topology accepted")
	}
}
