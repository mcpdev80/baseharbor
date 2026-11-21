package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

// TestPersesLifecycleCommandIsWiredIntoAppRoot asserts the perses command is
// registered under `baha app` with exactly the four accepted subcommands, each
// with an executable Run. This guards against the command silently dropping out
// of the app subtree.
func TestPersesLifecycleCommandIsWiredIntoAppRoot(t *testing.T) {
	root := rootCommand()
	app := appTestChild(t, root, "app")
	var perses *cli.Command
	for _, child := range app.Children {
		if child.Name == "perses" {
			perses = child
			break
		}
	}
	if perses == nil {
		t.Fatal("perses command must be registered under `baha app`")
	}
	want := map[string]bool{"enable": false, "disable": false, "status": false, "remove": false}
	for _, child := range perses.Children {
		if _, ok := want[child.Name]; ok {
			want[child.Name] = true
			if child.Run == nil {
				t.Fatalf("perses %s must have an executable Run", child.Name)
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("perses %s subcommand missing", name)
		}
	}
}

// TestPersesStatusFailsClosedWithoutRepositoryManifest asserts that a perses
// status call with no repository application resolves fail-closed rather than
// inventing a false state. This proves the CLI does not declare a running or
// configured surface without manifest truth.
func TestPersesStatusFailsClosedWithoutRepositoryManifest(t *testing.T) {
	root := rootCommand()
	app := appTestChild(t, root, "app")
	var perses *cli.Command
	for _, child := range app.Children {
		if child.Name == "perses" {
			perses = child
		}
	}
	status := findPersesChild(t, perses, "status")
	var out, errOut bytes.Buffer
	err := status.Run(context.Background(), nil, &out, &errOut)
	if err == nil {
		t.Fatal("perses status without a repository application must fail closed")
	}
	if strings.Contains(strings.ToLower(out.String()), "verified") {
		t.Fatal("perses status must never declare 'verified' without manifest truth")
	}
}

func appTestChild(t *testing.T, root *cli.Command, name string) *cli.Command {
	t.Helper()
	for _, child := range root.Children {
		if child.Name == name {
			return child
		}
	}
	t.Fatalf("missing command %q", name)
	return nil
}

func findPersesChild(t *testing.T, perses *cli.Command, name string) *cli.Command {
	t.Helper()
	for _, child := range perses.Children {
		if child.Name == name {
			return child
		}
	}
	t.Fatalf("missing perses %s subcommand", name)
	return nil
}
