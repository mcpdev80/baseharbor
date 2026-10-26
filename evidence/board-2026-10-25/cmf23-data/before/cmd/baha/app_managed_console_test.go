package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestManagedBackendConsoleCommand(t *testing.T) {
	for _, tt := range []struct{ kind, instance, service string }{
		{"postgres", "default", "postgres"},
		{"postgres", "analytics", "postgres-analytics"},
		{"valkey", "default", "valkey"},
		{"valkey", "cache", "valkey-cache"},
	} {
		binding := application.ServiceBinding{Instance: tt.instance, Username: "appuser", Database: "appdb", Password: "secret-never-in-argv"}
		service, args, err := managedBackendCommand(tt.kind, binding)
		if err != nil {
			t.Fatal(err)
		}
		if service != tt.service {
			t.Errorf("%s/%s: service %q wanted %q", tt.kind, tt.instance, service, tt.service)
		}
		if strings.Contains(strings.Join(args, " "), "secret-never-in-argv") {
			t.Fatal("secret leaked into runtime command argv")
		}
		if args[0] != "sh" {
			t.Fatalf("expected in-container shell, got %v", args)
		}
	}
}

func TestManagedBackendConsoleRejectsIncompleteBindings(t *testing.T) {
	if _, _, err := managedBackendCommand("postgres", application.ServiceBinding{Instance: "default"}); err == nil {
		t.Fatal("missing database must fail closed")
	}
	if _, _, err := managedBackendCommand("mongo", application.ServiceBinding{Instance: "default"}); err == nil {
		t.Fatal("unknown backend kind must fail closed")
	}
	if _, _, err := managedBackendCommand("valkey", application.ServiceBinding{}); err == nil {
		t.Fatal("missing instance must fail closed")
	}
}
