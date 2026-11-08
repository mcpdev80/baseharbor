package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
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

func TestCanonicalLogsCommandIsDiscoverable(t *testing.T) {
	root := rootCommand()
	commands := map[string]*cli.Command{}
	for _, cmd := range root.Children {
		if cmd == nil {
			continue
		}
		commands[cmd.Name] = cmd
	}
	logs, ok := commands["logs"]
	if !ok {
		t.Fatal("top-level 'baha logs' shorthand is missing; users fall back to the unrelated 'login' suggestion")
	}
	if logs.Run == nil {
		t.Fatal("top-level 'baha logs' shorthand has no handler")
	}
	if logs.Summary == "" {
		t.Fatal("top-level 'baha logs' shorthand lacks a public purpose")
	}
	// The shorthand must behave like the explicit app command: outside an
	// application repository it resolves an application deployment and reports a
	// concrete remediation instead of an unknown-command/login suggestion.
	err := logs.Run(context.Background(), nil, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("expected a resolution error outside an application repository")
	}
	if strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("top-level 'baha logs' still falls through to unknown-command: %v", err)
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
