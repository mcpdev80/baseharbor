package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureBaseHarborAgentsSectionCreatesAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	changed, err := ensureBaseHarborAgentsSection(root)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected AGENTS.md creation")
	}
	path := filepath.Join(root, "AGENTS.md")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), baseHarborAgentsStart) || !strings.Contains(string(first), "Never place secret or credential values") {
		t.Fatalf("unexpected AGENTS.md:\n%s", first)
	}
	changed, err = ensureBaseHarborAgentsSection(root)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second update must be idempotent")
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("idempotent update changed AGENTS.md")
	}
}

func TestEnsureBaseHarborAgentsSectionPreservesUnrelatedInstructions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "AGENTS.md")
	original := "# Project rules\n\nKeep this instruction.\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureBaseHarborAgentsSection(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), original) {
		t.Fatalf("unrelated instructions were changed:\n%s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode changed to %o", info.Mode().Perm())
	}
}

func TestEnsureBaseHarborAgentsSectionFailsClosedOnBrokenMarkers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(path, []byte(baseHarborAgentsStart+"\nbroken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureBaseHarborAgentsSection(root); err == nil {
		t.Fatal("expected ambiguous managed section to fail")
	}
}

func TestExtractAgentsOption(t *testing.T) {
	args, agents, err := extractAgentsOption([]string{"demo", "--agents", "-e", "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if !agents || strings.Join(args, " ") != "demo -e dev" {
		t.Fatalf("args=%v agents=%v", args, agents)
	}
}
