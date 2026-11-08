package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestParseOperatorLoginArgs(t *testing.T) {
	args, init, err := parseOperatorLoginArgs([]string{"-e", "dev", "--init-tenant"})
	if err != nil || !init || !reflect.DeepEqual(args, []string{"-e", "dev"}) {
		t.Fatalf("parsed login flags = %v, %t, %v", args, init, err)
	}
	args, init, err = parseOperatorLoginArgs([]string{"-e", "test"})
	if err != nil || init || !reflect.DeepEqual(args, []string{"-e", "test"}) {
		t.Fatalf("ordinary login changed = %v, %t, %v", args, init, err)
	}
	if _, _, err = parseOperatorLoginArgs([]string{"--init-tenant", "--init-tenant"}); err == nil {
		t.Fatal("duplicate first-owner authorization must fail")
	}
}

func TestInitialOperatorBootstrapRejectsRemoteTarget(t *testing.T) {
	target := deployment.ResolvedTarget{AccessProvider: "baseharbor-node-connector"}
	if _, err := initialOperatorLocalCoreFiles(context.Background(), target); !errors.Is(err, errInitialOperatorLocalOnly) {
		t.Fatalf("remote target must not bootstrap initial tenant: %v", err)
	}
}

func TestInitialTenantDefaultAndCustomOptions(t *testing.T) {
	args, init, slug, name, err := parseInitialTenantLoginArgs([]string{"--init-tenant", "-e", "dev"})
	if err != nil || !init || slug != "baseharbor" || name != "BaseHarbor" || !reflect.DeepEqual(args, []string{"-e", "dev"}) {
		t.Fatalf("default login options: %v %t %s %s %v", args, init, slug, name, err)
	}
	args, init, slug, name, err = parseInitialTenantLoginArgs([]string{"-e", "prod", "--init-tenant", "--tenant-slug", "acme", "--tenant-name", "Acme GmbH"})
	if err != nil || !init || slug != "acme" || name != "Acme GmbH" || !reflect.DeepEqual(args, []string{"-e", "prod"}) {
		t.Fatalf("custom login options: %v %t %s %s %v", args, init, slug, name, err)
	}
	for _, invalid := range [][]string{{"--tenant-slug", "acme"}, {"--init-tenant", "--tenant-name"}, {"--init-tenant", "--tenant-slug", "a", "--tenant-slug", "b"}} {
		if _, _, _, _, err := parseInitialTenantLoginArgs(invalid); err == nil {
			t.Fatalf("invalid first-tenant options accepted: %v", invalid)
		}
	}
}

// TestParseLoginTrustArgsNoBrowserAcceptsHeadlessFlag proves the documented
// --no-browser companion flag is accepted by generic argument parsing (it no
// longer errors) AND is captured into a NoBrowser result so the CLI selects the
// headless path instead of launching a browser. This is the CLI-level
// regression the production command rejected before the fix.
func TestParseLoginTrustArgsNoBrowserAcceptsHeadlessFlag(t *testing.T) {
	remaining, ca, authCode, noBrowser, err := parseLoginTrustArgs([]string{"-e", "test", "--no-browser"})
	if err != nil {
		t.Fatalf("--no-browser must not error at the CLI: %v", err)
	}
	if noBrowser != true {
		t.Fatalf("--no-browser must be captured as NoBrowser=true, got %t", noBrowser)
	}
	// The remaining args keep the environment flag for extractApplicationEnvironment.
	// --no-browser is echoed back so generic parsing accepts it without error.
	if !reflect.DeepEqual(remaining, []string{"-e", "test", "--no-browser"}) {
		t.Fatalf("--no-browser must be echoed for generic parsing, got %v", remaining)
	}

	// A --ca file alongside --no-browser still parses and captures both values.
	remaining, ca, authCode, noBrowser, err = parseLoginTrustArgs([]string{"-e", "test", "--ca", "/tmp/ca.pem", "--auth-code-file", "/tmp/code.txt", "--no-browser"})
	if err != nil {
		t.Fatalf("--ca/--auth-code-file/--no-browser must parse: %v", err)
	}
	if !noBrowser || ca != "/tmp/ca.pem" || authCode != "/tmp/code.txt" {
		t.Fatalf("combined trust flags not captured: remaining=%v ca=%q authCode=%q noBrowser=%t", remaining, ca, authCode, noBrowser)
	}
}
