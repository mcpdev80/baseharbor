package localhosts

import (
	"strings"
	"testing"
)

func TestHostsOwnershipLifecycle(t *testing.T) {
	id := ID("workstation", "dev-docker-1")
	block, err := Block(id, "192.0.2.17", []string{"auth.example.test", "demo.example.test", "auth.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	original := "127.0.0.1 localhost\n# operator content\n"
	added, err := Rewrite(original, Change{ID: id, Next: block})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(added, "auth.example.test") != 1 {
		t.Fatal("names not deduplicated")
	}
	next, err := Block(id, "192.0.2.18", []string{"demo.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := Rewrite(added, Change{ID: id, Previous: block, Next: next})
	if err != nil {
		t.Fatal(err)
	}
	cleaned, err := Rewrite(updated, Change{ID: id, Previous: next})
	if err != nil || cleaned != original {
		t.Fatalf("cleanup changed operator content: %v", err)
	}
}

func TestHostsHumanReadableMarkerAndLegacyMigration(t *testing.T) {
	id := ID("workstation", "dev-docker-1")
	legacy, err := Block(id, "192.0.2.17", []string{"demo.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	label := "target=dev-docker-1 installation=fc204331-0c16-44ad-9a10-a7c71df4919b"
	named, err := BlockWithLabel(id, label, "192.0.2.18", []string{"demo.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(named, "# BEGIN BaseHarbor "+label+"\n") || strings.Contains(named, id) {
		t.Fatalf("marker is not human-readable: %q", named)
	}
	current, err := Rewrite(legacy, Change{ID: id, Previous: legacy, NextLabel: label, Next: named})
	if err != nil || current != named {
		t.Fatalf("legacy block migration failed: %v", err)
	}
	cleaned, err := Rewrite(current, Change{ID: id, PreviousLabel: label, Previous: named})
	if err != nil || cleaned != "" {
		t.Fatalf("named block cleanup failed: %v", err)
	}
}

func TestHostsPreservesConflictsAndManualChanges(t *testing.T) {
	id := ID("workstation", "one")
	block, _ := Block(id, "192.0.2.17", []string{"demo.example.test"})
	cases := []struct{ name, current, previous string }{
		{"external-name", "192.0.2.9 demo.example.test\n", ""},
		{"different-case", "192.0.2.9 DEMO.EXAMPLE.TEST\n", ""},
		{"unrecorded-block", block, ""},
		{"modified-block", strings.ReplaceAll(block, "192.0.2.17", "192.0.2.19"), block},
		{"duplicate-block", block + block, block},
		{"unterminated-file", "127.0.0.1 localhost", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Rewrite(tc.current, Change{ID: id, Previous: tc.previous, Next: block}); err == nil {
				t.Fatal("unsafe edit accepted")
			}
		})
	}
}

func TestHostsRejectsUnusableNamesAndInjection(t *testing.T) {
	for _, name := range []string{"*.example.test", "localhost", "auth.localhost", "-bad.test", "bad..test", "bad.test\n127.0.0.1 evil", "127.0.0.1"} {
		if _, err := Block(ID("a", "b"), "192.0.2.17", []string{name}); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}

func TestHostsSharedAddressAliasesCleanIndependently(t *testing.T) {
	first, second := ID("workstation", "first"), ID("workstation", "second")
	one, _ := Block(first, "192.0.2.7", []string{"shared.example.test"})
	two, _ := Block(second, "192.0.2.7", []string{"shared.example.test"})
	current, err := Rewrite(one, Change{ID: second, Next: two})
	if err != nil {
		t.Fatal(err)
	}
	remaining, err := Rewrite(current, Change{ID: first, Previous: one})
	if err != nil || remaining != two {
		t.Fatal("cleanup removed the other connection's mapping")
	}
}
