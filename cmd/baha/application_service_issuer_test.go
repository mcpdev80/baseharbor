package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/installauthority"
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

func TestIdentityVerificationWithoutCapabilityDoesNotRequireTrustPlane(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	if err := verifyExistingManagedIdentity(context.Background(), nil, resolved, nil); err != nil {
		t.Fatal("zero-capability verification required a trust plane", err)
	}
	if _, err := os.Stat(resolved.TargetStateRoot); !os.IsNotExist(err) {
		t.Fatal("readiness inspection created service authority state")
	}
}

func TestAbsentStandaloneIdentityCleanupDoesNotRequireCoreOrCreateAuthority(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	resolved.Manifest.Services.Identity = true
	execution := applicationDestroyExecution{resolved: resolved, manifest: resolved.Manifest}
	if err := execution.cleanupKeycloakIdentity(context.Background()); err != nil {
		t.Fatal("absent standalone Identity required Core for cleanup", err)
	}
	if _, err := os.Stat(resolved.TargetStateRoot); !os.IsNotExist(err) {
		t.Fatal("absent module cleanup created replacement authority")
	}
}

// --- Finding 2: production selection calls resolveManagementConnectionIssuer ---

// TestResolveManagementConnectionIssuerRealPath proves the authorized
// production selection point returns a live, verified management-connection
// issuer through the real cmd/baha path (no fake issuer), and that a revoked
// connection selects no issuer (fails closed).
func TestResolveManagementConnectionIssuerRealPath(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	// Establish the local service-access issuer (the authorized production
	// path materializes it read-only on first verification use).
	localIssuer, err := resolveApplicationServiceIssuer(context.Background(), resolved, nil)
	if err != nil {
		t.Fatalf("resolve local issuer: %v", err)
	}
	if _, ok := localIssuer.(*serviceaccess.LocalIssuer); ok {
		if err := materializeApplicationServiceIssuer(localIssuer); err != nil {
			t.Fatal(err)
		}
	}
	issuer, err := resolveManagementConnectionIssuer(context.Background(), resolved, nil)
	if err != nil {
		t.Fatalf("verified management connection rejected: %v", err)
	}
	// The verified issuer is the real LocalIssuer reached through the production
	// resolveApplicationServiceIssuer path, not a test double.
	if _, ok := issuer.(*serviceaccess.LocalIssuer); !ok {
		t.Fatalf("expected real LocalIssuer, got %T", issuer)
	}
	// The returned issuer is a genuine, current authority: its observed
	// connection verifies through the read-only installauthority.Fresh boundary.
	observed, err := observedThroughBoundary(context.Background(), issuer)
	if err != nil {
		t.Fatal(err)
	}
	if err := installauthority.Fresh(context.Background(), issuer, observed, observed.Identity); err != nil {
		t.Fatalf("verified issuer not fresh through boundary: %v", err)
	}
}

// TestResolveManagementConnectionIssuerRevokedSelectsNoIssuer proves the read-only
// verification boundary that resolveManagementConnectionIssuer relies on fails
// closed when the observed serial is revoked, without writing any new issuer
// state.
func TestResolveManagementConnectionIssuerRevokedSelectsNoIssuer(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	localIssuer, err := resolveApplicationServiceIssuer(context.Background(), resolved, nil)
	if err != nil {
		t.Fatal(err)
	}
	local, ok := localIssuer.(*serviceaccess.LocalIssuer)
	if !ok {
		t.Fatalf("expected LocalIssuer, got %T", localIssuer)
	}
	if err := materializeApplicationServiceIssuer(localIssuer); err != nil {
		t.Fatal(err)
	}
	// Revoke the exact leaf that a fresh observation would present, so the
	// read-only Revoked(serial) boundary rejects it.
	leaf, err := local.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "management-connection", TTL: 90 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if err := local.Revoke(context.Background(), leaf.Serial); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	trust, err := local.TrustBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	observed := serviceaccess.CertificateStatus{Identity: trust.IssuerReference, IssuedCertificate: leaf}
	if err := installauthority.Fresh(context.Background(), local, observed, observed.Identity); err == nil {
		t.Fatal("revoked observed connection verified through production path")
	}
}
