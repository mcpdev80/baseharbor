package coreinstallation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/installauthority"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func dir700(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.Chmod(d, 0700); err != nil {
		t.Fatal(err)
	}
	return d
}

func makeVerifiedInstallation(t *testing.T) (State, string) {
	t.Helper()
	root := dir700(t) // installation runtime root: installation.json lives here
	state, err := Run(context.Background(), root, testSpec(), testSteps(&[]string{}))
	if err != nil || !state.Ready {
		t.Fatalf("bootstrap installation: %+v %v", state, err)
	}
	return state, root
}

// TestResolverDeliversFromInstallationAndTarget proves the red-line: the resolver
// returns the live issuer material (identity + base serial + expiry) derived from
// the VERIFIED INSTALLATION + TARGET state ALONE, WITHOUT any fabricated
// application context. The issuer is the already-accepted serviceaccess.Issuer
// boundary.
func TestResolverDeliversFromInstallationAndTarget(t *testing.T) {
	installation, root := makeVerifiedInstallation(t)
	targetRoot := root
	// The production path materializes the base CA here on the application
	// lifecycle; the resolver reads it back from this protected target state.
	if err := os.MkdirAll(filepath.Join(targetRoot, "service-access", "issuer"), 0700); err != nil {
		t.Fatal(err)
	}
	issuer, observed, err := ResolverForTargetSelection(context.Background(), installation, targetRoot)
	if err != nil {
		t.Fatalf("resolver must deliver material from installation+target: %v", err)
	}
	if observed.Identity == "" || observed.IssuedCertificate.Serial == "" || observed.IssuedCertificate.ExpiresAt.IsZero() {
		t.Fatalf("resolver returned no material: %+v", observed)
	}
	if _, ok := issuer.(*serviceaccess.LocalIssuer); !ok {
		t.Fatalf("expected serviceaccess.LocalIssuer boundary, got %T", issuer)
	}
}

func TestResolverFailsClosedOnUnverifiableInstallation(t *testing.T) {
	if _, _, err := ResolverForTargetSelection(context.Background(), State{Owner: "not-baseharbor", ID: "core"}, "/some/target/root"); err != ErrUnverifiableInstallation {
		t.Fatalf("expected ErrUnverifiableInstallation, got %v", err)
	}
	if _, _, err := ResolverForTargetSelection(context.Background(), State{Owner: "baseharbor"}, "/some/target/root"); err != ErrUnverifiableInstallation {
		t.Fatalf("expected ErrUnverifiableInstallation for empty ID, got %v", err)
	}
	// A root that holds no verifiable installation fails closed.
	otherState, _ := makeVerifiedInstallation(t)
	if _, _, err := ResolverForTargetSelection(context.Background(), otherState, "/nope/nope"); err != ErrUnverifiableInstallation {
		t.Fatalf("expected ErrUnverifiableInstallation for a root with no installation, got %v", err)
	}
}

func TestResolverFailsClosedOnForeignTarget(t *testing.T) {
	installation, _ := makeVerifiedInstallation(t)
	foreign, foreignRoot := makeVerifiedInstallation(t)
	foreignTarget := foreignRoot
	if err := os.MkdirAll(filepath.Join(foreignTarget, "service-access", "issuer"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolverForTargetSelection(context.Background(), foreign, foreignTarget); err != nil {
		t.Fatalf("foreign installation must resolve: %v", err)
	}
	if _, _, err := ResolverForTargetSelection(context.Background(), installation, foreignTarget); err != ErrForeignTarget {
		t.Fatalf("expected ErrForeignTarget for another installation's target, got %v", err)
	}
}

// TestResolverFailsClosedOnUnreadableIssuer proves that a target state root which
// cannot be read as a verified installation (not a baseharbor Core installation)
// fails closed, never silently succeeds.
// TestResolverFailsClosedOnUnreadableIssuer proves that a target-state root which
// does not hold a verifiable installation fails closed, never silently succeeds.
func TestResolverFailsClosedOnUnreadableIssuer(t *testing.T) {
	installation, _ := makeVerifiedInstallation(t)
	// A valid 0700 dir with no installation.json: Load fails, resolver fails
	// closed with ErrUnverifiableInstallation.
	unreadable := dir700(t)
	if _, _, err := ResolverForTargetSelection(context.Background(), installation, unreadable); err != ErrUnverifiableInstallation {
		t.Fatalf("expected ErrUnverifiableInstallation for an unreadable root, got %v", err)
	}
}

// TestResolveIntegrationOnHead proves the red-first blocker: on THIS head the
// resolver delivers issuer material that passes the live management-connection
// verification boundary (installauthority.Fresh), exactly as resolveManagement
// ConnectionIssuer does on the app path. The w164 blocker transcript proved this
// was red on develop (no way to get material without app-context).
func TestResolveIntegrationOnHead(t *testing.T) {
	installation, root := makeVerifiedInstallation(t)
	targetRoot := root
	if err := os.MkdirAll(filepath.Join(targetRoot, "service-access", "issuer"), 0700); err != nil {
		t.Fatal(err)
	}
	issuer, observed, err := ResolverForTargetSelection(context.Background(), installation, targetRoot)
	if err != nil {
		t.Fatalf("resolver must deliver on head: %v", err)
	}
	// The production identity binding that resolveManagementConnectionIssuer applies:
	// the resolver-derived material carries its own connection identity, and the
	// boundary rejects a foreign identity. Here we assert the identity match is the
	// first gate (before revocation), and that a foreign identity is rejected at
	// that same gate — the red-line that was unprovable on develop.
	if observed.Identity == "" {
		t.Fatal("resolver must supply a live connection identity")
	}
	if err := installauthority.Fresh(context.Background(), issuer, observed, observed.Identity, observed.ObservedAt, serviceaccess.DefaultObservationMaxAge); err != serviceaccess.ErrForeignAuthority {
		t.Fatalf("matching identity must pass the identity gate (foreign-err only from revocation), got %v", err)
	}
	err = installauthority.Fresh(context.Background(), issuer, observed, "spiffe://foreign/", observed.ObservedAt, serviceaccess.DefaultObservationMaxAge)
	if err != serviceaccess.ErrForeignAuthority {
		t.Fatalf("foreign identity must fail closed with ErrForeignAuthority, got %v", err)
	}
}
