package main

import (
	"strings"
	"testing"
)

func TestCanonicalHumanCommandsAreDiscoverable(t *testing.T) {
	root := rootCommand()
	commands := map[string]bool{}
	for _, cmd := range root.Children {
		if cmd == nil {
			continue
		}
		if commands[cmd.Name] {
			t.Errorf("duplicate top-level command %q", cmd.Name)
		}
		commands[cmd.Name] = true
	}
	for _, name := range []string{"new", "init", "up", "down", "status", "open", "inspect", "plan", "doctor", "update", "backup", "restore", "destroy", "list"} {
		if !commands[name] {
			t.Errorf("canonical developer task %q missing", name)
		}
	}
	for _, name := range []string{"target", "provider", "workspace", "stack", "config", "mcp"} {
		if !commands[name] {
			t.Errorf("advanced namespace %q not discoverable", name)
		}
	}
}

func TestHumanCommandHelpAdvertisesSafeAlternatives(t *testing.T) {
	root := rootCommand()
	for _, cmd := range root.Children {
		if cmd.Name == "new" {
			if !strings.Contains(cmd.Long, "Interactively choose") {
				t.Fatal("creation wizard not explained")
			}
		}
		if cmd.Name == "doctor" {
			if !strings.Contains(cmd.Usage, "--fix --yes") {
				t.Fatal("doctor repair not explicitly approved")
			}
		}
	}
}
