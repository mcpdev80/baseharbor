package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/operatorauth"
)

func TestParseAppUpOptionsTreatsYesAsOption(t *testing.T) {
	opts, err := parseAppUpOptions([]string{"--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Yes {
		t.Fatal("--yes was not parsed")
	}
	if opts.Name != "" {
		t.Fatalf("--yes was interpreted as application name %q", opts.Name)
	}
}

func TestParseAppUpOptionsSupportsNameAndFlagsInEitherOrder(t *testing.T) {
	for _, args := range [][]string{
		{"demo", "--yes", "--skip-memory-preflight"},
		{"--yes", "--skip-memory-preflight", "demo"},
	} {
		opts, err := parseAppUpOptions(args)
		if err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		if opts.Name != "demo" || !opts.Yes || !opts.SkipMemoryPreflight {
			t.Fatalf("parse %v = %#v", args, opts)
		}
	}
}

func TestParseAppUpOptionsRejectsUnknownOption(t *testing.T) {
	if _, err := parseAppUpOptions([]string{"--unknown-option"}); err == nil {
		t.Fatal("unknown option was accepted")
	}
}

func TestAssumeYesContext(t *testing.T) {
	ctx := withAssumeYes(context.Background(), true)
	if !assumeYes(ctx) {
		t.Fatal("assume-yes context was not preserved")
	}
}

func TestAssumeYesDoesNotBootstrapMissingOperatorAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_OPERATOR_OIDC_ISSUER", "")
	t.Setenv("BASEHARBOR_OPERATOR_OIDC_CLIENT_ID", "")

	ctx := operatorauth.WithEnforcement(context.Background())
	ctx = withAssumeYes(ctx, true)
	_, err := resolveOperatorAuthBoundaryConfig(ctx, "missing-target", "test")
	if !errors.Is(err, operatorauth.ErrConfigurationRequired) {
		t.Fatalf("error = %v, want ErrConfigurationRequired", err)
	}
}
