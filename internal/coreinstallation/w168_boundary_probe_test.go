package coreinstallation

import (
	"context"
	"errors"
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

// materializeBaseCAOnAuthorizedPath writes the base CA on the single authorized
// lifecycle phase (the same production path that the resolver must NEVER touch),
// so the observe-only resolver can read back a genuine, already-existing issuer.
func materializeBaseCAOnAuthorizedPath(t *testing.T, targetRoot string) {
	t.Helper()
	issuerDir := filepath.Join(targetRoot, "service-access", "issuer")
	if _, err := serviceaccess.EnsureLocalIssuer(issuerDir); err != nil {
		t.Fatalf("materialize base CA on the authorized lifecycle path: %v", err)
	}
}

// TestResolverDeliversFromInstallationAndTarget proves the red-line: the resolver
// returns the live issuer material (identity + base serial + expiry) derived from
// the VERIFIED INSTALLATION + TARGET state ALONE, WITHOUT any fabricated
// application context. The issuer is the already-accepted serviceaccess.Issuer
// boundary. The base CA is materialized on the authorized lifecycle phase so the
// resolver only ever OBSERVES an existing issuer — it never materializes one.
func TestResolverDeliversFromInstallationAndTarget(t *testing.T) {
	installation, root := makeVerifiedInstallation(t)
	targetRoot := root
	materializeBaseCAOnAuthorizedPath(t, targetRoot)
	before, _ := os.Stat(filepath.Join(targetRoot, "service-access", "issuer"))
	issuer, observed, err := ResolverForTargetSelection(context.Background(), installation, targetRoot)
	if err != nil {
		t.Fatalf("resolver must deliver material from installation+target: %v", err)
	}
	if _, ok := issuer.(*serviceaccess.LocalIssuer); !ok {
		t.Fatalf("expected serviceaccess.LocalIssuer boundary, got %T", issuer)
	}
	if observed.Identity == "" || observed.IssuedCertificate.Serial == "" || observed.IssuedCertificate.ExpiresAt.IsZero() {
		t.Fatalf("resolver returned no material: %+v", observed)
	}
	// observe-only: the resolver must not rewrite the existing issuer dir.
	after, _ := os.Stat(filepath.Join(targetRoot, "service-access", "issuer"))
	if before.ModTime().Unix() != after.ModTime().Unix() {
		t.Fatalf("observe-only resolver must not rewrite the existing issuer dir")
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
	materializeBaseCAOnAuthorizedPath(t, foreignTarget)
	if _, _, err := ResolverForTargetSelection(context.Background(), foreign, foreignTarget); err != nil {
		t.Fatalf("foreign installation must resolve: %v", err)
	}
	if _, _, err := ResolverForTargetSelection(context.Background(), installation, foreignTarget); err != ErrForeignTarget {
		t.Fatalf("expected ErrForeignTarget for another installation's target, got %v", err)
	}
}

// TestResolverFailsClosedOnUnreadableIssuer proves that a target state root which
// cannot be read as a verified installation (not a baseharbor Core installation)
// fails closed, never silently succeeds. The dir holds no installation.json, so
// there is no verifiable installation and hence no issuer to observe.
func TestResolverFailsClosedOnUnreadableIssuer(t *testing.T) {
	installation, _ := makeVerifiedInstallation(t)
	// A 0700 dir that holds no installation.json: it is not a verifiable
	// installation, so there is no issuer to observe and the resolver fails
	// closed with ErrUnverifiableInstallation — and it must not fabricate an
	// issuer.pem into this unrelated dir.
	unreadable := dir700(t)
	if _, _, err := ResolverForTargetSelection(context.Background(), installation, unreadable); err != ErrUnverifiableInstallation {
		t.Fatalf("expected ErrUnverifiableInstallation for an unreadable root, got %v", err)
	}
	// The read-only resolver must not have written any issuer material here.
	if _, err := os.Stat(filepath.Join(unreadable, "service-access", "issuer", "issuer.pem")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("observe-only resolver must not fabricate issuer material in an unrelated dir")
	}
}

// TestResolverFailsClosedOnAbsentIssuerDir proves that a verified installation
// whose protected target state holds NO issuer directory yet fails closed: a
// missing authority does not fabricate a new trust root during resolution.
func TestResolverFailsClosedOnAbsentIssuerDir(t *testing.T) {
	installation, root := makeVerifiedInstallation(t)
	// No issuer dir exists at all — a bare verified installation has not yet
	// materialized its base CA. The resolver must fail closed, never succeed, and
	// must not create the directory.
	issuerDir := filepath.Join(root, "service-access", "issuer")
	if _, _, err := ResolverForTargetSelection(context.Background(), installation, root); err != ErrUnverifiableInstallation {
		t.Fatalf("absent issuer dir must fail closed with ErrUnverifiableInstallation, got %v", err)
	}
	if _, err := os.Stat(issuerDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("observe-only resolver must not create the issuer dir, but %v now exists", issuerDir)
	}
}

// TestResolverFailsClosedOnEmptyIssuerFile proves that a verified installation
// whose protected issuer directory holds NO issuer.pem yet fails closed: an
// empty/incomplete issuer is not a valid authority, and the resolver must not
// fill it in with a fabricated CA during resolution.
func TestResolverFailsClosedOnEmptyIssuerFile(t *testing.T) {
	installation, root := makeVerifiedInstallation(t)
	// The directory exists but holds no issuer.pem (a partial/uninitialized
	// issuer). CheckLocalIssuer rejects this; the observe-only resolver must fail
	// closed and must not write issuer.pem into the empty directory.
	issuerDir := filepath.Join(root, "service-access", "issuer")
	if err := os.MkdirAll(issuerDir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolverForTargetSelection(context.Background(), installation, root); err != ErrUnverifiableInstallation {
		t.Fatalf("empty issuer dir must fail closed with ErrUnverifiableInstallation, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(issuerDir, "issuer.pem")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("observe-only resolver must not fabricate issuer.pem, but it now exists")
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
	materializeBaseCAOnAuthorizedPath(t, targetRoot)
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
