package environment

import (
	"strings"
	"testing"
)

// TestNewIDIsValidUUID confirms every generated identity is a well-formed
// UUIDv4. This is the baseline the whole identity model depends on.
func TestNewIDIsValidUUID(t *testing.T) {
	for i := 0; i < 200; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatalf("NewID: %v", err)
		}
		if err := Validate(id); err != nil {
			t.Fatalf("generated id %q invalid: %v", id, err)
		}
	}
}

// TestNewIDUniqueness proves that two identities created in the same
// process are never equal — the uniqueness guarantee the deployment registry
// relies on.
func TestNewIDUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Fatalf("duplicate environment id %q generated", id)
		}
		seen[id] = true
	}
}

func TestValidateRejectsBadInput(t *testing.T) {
	for _, bad := range []string{"", "not-a-uuid", "00000000-0000-0000-0000-000000000000", "ABCDEF00-0000-4000-8000-000000000000"} {
		if err := Validate(bad); err == nil {
			t.Fatalf("Validate(%q) accepted invalid id", bad)
		}
	}
}

func TestValidateNonEmptyRejectsEmpty(t *testing.T) {
	if err := ValidateNonEmpty("   "); err == nil {
		t.Fatal("empty/whitespace id accepted")
	}
}

// TestNewBuildsValidEnvironment confirms New allocates an ID and a valid name.
func TestNewBuildsValidEnvironment(t *testing.T) {
	env, err := New("prod")
	if err != nil {
		t.Fatal(err)
	}
	if env.Name != "prod" {
		t.Fatalf("name=%q, want prod", env.Name)
	}
	if err := Validate(env.ID); err != nil {
		t.Fatalf("env id invalid: %v", err)
	}
}

func TestNewRejectsInvalidName(t *testing.T) {
	for _, bad := range []string{"", "/etc", "prod/", "PROD", "a/b", " - "} {
		if _, err := New(bad); err == nil {
			t.Fatalf("New(%q) accepted invalid name", bad)
		}
	}
}

// TestRenamePreservesID is the durable-identity guarantee: renaming an
// environment never changes its ID, matching ADR 0017.
func TestRenamePreservesID(t *testing.T) {
	env, err := New("dev")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := env.Rename("production")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ID != env.ID {
		t.Fatalf("rename changed id: %q -> %q", env.ID, renamed.ID)
	}
	if renamed.Name != "production" {
		t.Fatalf("name=%q, want production", renamed.Name)
	}
}

func TestRenameRejectsInvalidName(t *testing.T) {
	env, err := New("dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Rename("bad/name"); err == nil {
		t.Fatal("rename with invalid name accepted")
	}
}

// TestNewNameTrimming confirms leading/trailing whitespace is normalized, so a
// name cannot smuggle surrounding blanks into state paths.
func TestNewNameTrimming(t *testing.T) {
	env, err := New("  staging  ")
	if err != nil {
		t.Fatal(err)
	}
	if env.Name != "staging" {
		t.Fatalf("name not trimmed: %q", env.Name)
	}
	if strings.Contains(env.Name, " ") {
		t.Fatal("name still contains whitespace")
	}
}

func TestValidateNameAcceptsSlugsAndRejectsPathNames(t *testing.T) {
	for _, name := range []string{"prod", "qa-2", "prod.v1"} {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q): %v", name, err)
		}
	}
	for _, name := range []string{"", "..", "../outside", "../../outside", "/tmp/outside", `..\\outside`, "UPPER", "-prod", "prod/"} {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) unexpectedly succeeded", name)
		}
	}
}
