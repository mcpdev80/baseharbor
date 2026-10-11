package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func TestApplicationServiceIssuerPlansWithoutRuntimeOrStateWrites(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	issuer, err := resolveApplicationServiceIssuer(context.Background(), resolved, nil)
	if err != nil {
		t.Fatal(err)
	}
	local, ok := issuer.(*serviceaccess.LocalIssuer)
	if !ok {
		t.Fatalf("unexpected runtime-dependent issuer: %T", issuer)
	}
	if _, err := os.Stat(resolved.TargetStateRoot); !os.IsNotExist(err) {
		t.Fatal("PKI preflight wrote Target state")
	}
	if err := materializeApplicationServiceIssuer(issuer); err != nil {
		t.Fatal(err)
	}
	trust, err := local.TrustBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// An unrelated later runtime must not implicitly migrate existing trust.
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	again, err := resolveApplicationServiceIssuer(context.Background(), resolved, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := again.TrustBundle(context.Background())
	if err != nil || second.IssuerReference != trust.IssuerReference {
		t.Fatal("existing authority was changed", err)
	}
}

func TestApplicationServiceIssuerRejectsDamagedRuntimeAndChangedTarget(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	root, err := targetRuntimeStateRoot(resolved.Target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveApplicationServiceIssuer(context.Background(), resolved, nil); err == nil {
		t.Fatal("partial selected runtime bypassed through new local authority")
	}
	if _, err := os.Stat(filepath.Join(resolved.TargetStateRoot, "service-access")); !os.IsNotExist(err) {
		t.Fatal("damaged runtime created replacement authority")
	}
	resolved.Target.Scope = "foreign"
	if _, err := resolveApplicationServiceIssuer(context.Background(), resolved, nil); err == nil {
		t.Fatal("changed Target accepted")
	}
}
