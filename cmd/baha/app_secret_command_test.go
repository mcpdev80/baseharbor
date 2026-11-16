package main

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

func TestApplicationSecretCommandsAreDiscoverable(t *testing.T) {
	root := rootCommand()
	app := findSecretTestChild(root, "app")
	if app == nil {
		t.Fatal("app command is missing")
	}
	secret := findSecretTestChild(app, "secret")
	if secret == nil {
		t.Fatal("app secret command is missing")
	}
	for _, name := range []string{"set", "list", "delete", "tls-set"} {
		child := findSecretTestChild(secret, name)
		if child == nil {
			t.Fatalf("app secret %s command is missing", name)
		}
		if child.Usage == "" || child.Summary == "" || child.Long == "" {
			t.Fatalf("app secret %s help metadata is incomplete", name)
		}
	}
}

func findSecretTestChild(parent *cli.Command, name string) *cli.Command {
	for _, child := range parent.Children {
		if child.Name == name {
			return child
		}
	}
	return nil
}
