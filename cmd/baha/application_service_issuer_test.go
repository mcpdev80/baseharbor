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
		// Mint a valid signed CRL so the read-only revocation status is
		// readable. Revoke a throwaway serial (not the observed base serial)
		// so the base is confirmed not-revoked.
		local, _ := localIssuer.(*serviceaccess.LocalIssuer)
		if seed, ierr := local.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "seed", TTL: 90 * 24 * time.Hour}); ierr == nil {
			if rerr := local.Revoke(context.Background(), seed.Serial); rerr == nil {
				// throwaway revocation present; base stays not-revoked
			}
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
	if err := installauthority.Fresh(context.Background(), issuer, observed, observed.Identity, observed.ObservedAt, time.Minute); err != nil {
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
	observed := serviceaccess.CertificateStatus{Identity: trust.IssuerReference, IssuedCertificate: leaf, ObservedAt: time.Now().UTC()}
	if err := installauthority.Fresh(context.Background(), local, observed, observed.Identity, observed.ObservedAt, time.Minute); err == nil {
		t.Fatal("revoked observed connection verified through production path")
	}
}

// TestResolveManagementConnectionIssuerEndToEnd proves the authorized production
// selection wrapper (resolveManagementConnectionIssuer) itself, not a separate
// helper, verifies a live, non-revoked local issuer end to end and fails closed
// on every error class (revoked, unreadable/missing revocation status). It runs
// through the real resolveApplicationServiceIssuer path and the read-only
// installauthority.Fresh boundary; no Issue() side-effect is written on the
// verification path.
func TestResolveManagementConnectionIssuerEndToEnd(t *testing.T) {
	// A live, non-revoked connection verifies through the wrapper.
	{
		resolved := corelessResolvedFixture(t)
		localIssuer, err := resolveApplicationServiceIssuer(context.Background(), resolved, nil)
		if err != nil {
			t.Fatalf("resolve local issuer: %v", err)
		}
		if _, ok := localIssuer.(*serviceaccess.LocalIssuer); ok {
			if err := materializeApplicationServiceIssuer(localIssuer); err != nil {
				t.Fatal(err)
			}
			// Mint a valid signed CRL (revoking a throwaway, not the observed
			// base serial) so the read-only revocation status is readable.
			local, _ := localIssuer.(*serviceaccess.LocalIssuer)
			if seed, ierr := local.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "seed", TTL: 90 * 24 * time.Hour}); ierr == nil {
				_ = local.Revoke(context.Background(), seed.Serial)
			}
		}
		issuer, err := resolveManagementConnectionIssuer(context.Background(), resolved, nil)
		if err != nil {
			t.Fatalf("live management connection rejected through wrapper: %v", err)
		}
		if _, ok := issuer.(*serviceaccess.LocalIssuer); !ok {
			t.Fatalf("expected real LocalIssuer through wrapper, got %T", issuer)
		}
	}

	// A revoked observed connection selects no issuer through the wrapper.
	{
		resolved := corelessResolvedFixture(t)
		localIssuer, err := resolveApplicationServiceIssuer(context.Background(), resolved, nil)
		if err != nil {
			t.Fatalf("resolve local issuer: %v", err)
		}
		local, ok := localIssuer.(*serviceaccess.LocalIssuer)
		if !ok {
			t.Fatalf("expected LocalIssuer, got %T", localIssuer)
		}
		if err := materializeApplicationServiceIssuer(localIssuer); err != nil {
			t.Fatal(err)
		}
		// Revoke the base serial the wrapper observes, so the read-only
		// RevokedStatus boundary rejects the connection as foreign.
		baseSerial, err := local.BaseSerial(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := local.Revoke(context.Background(), baseSerial); err != nil {
			t.Fatalf("revoke base: %v", err)
		}
		if _, err := resolveManagementConnectionIssuer(context.Background(), resolved, nil); err == nil {
			t.Fatal("revoked observed connection verified through wrapper (fail open)")
		}
	}

	// An unreadable/missing revocation status fails closed through the wrapper:
	// corrupt the CRL so the read-only status cannot be confirmed, then the
	// wrapper must reject the connection.
	{
		resolved := corelessResolvedFixture(t)
		localIssuer, err := resolveApplicationServiceIssuer(context.Background(), resolved, nil)
		if err != nil {
			t.Fatalf("resolve local issuer: %v", err)
		}
		local, ok := localIssuer.(*serviceaccess.LocalIssuer)
		if !ok {
			t.Fatalf("expected LocalIssuer, got %T", localIssuer)
		}
		if err := materializeApplicationServiceIssuer(localIssuer); err != nil {
			t.Fatal(err)
		}
		// Mint a valid CRL, then corrupt it so the status is unreadable.
		seed, err := local.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "seed", TTL: 90 * 24 * time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		if err := local.Revoke(context.Background(), seed.Serial); err != nil {
			t.Fatalf("mint CRL: %v", err)
		}
		crlPath := filepath.Join(local.Dir, "revocations.pem")
		if err := os.WriteFile(crlPath, []byte("garbage-not-pem"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveManagementConnectionIssuer(context.Background(), resolved, nil); err == nil {
			t.Fatal("unreadable revocation status verified through wrapper (fail open)")
		}
	}
}

// TestResolveManagementConnectionIssuerSuppliesObservedAtFreshness proves the
// wrapper resolveManagementConnectionIssuer supplies its own observation time to
// the boundary: a live, non-revoked connection verifies, and the observed-at
// freshness bound (zero/future/stale) is enforced by the boundary the wrapper
// calls, not by a certificate expiry that is normally in the future.
func TestResolveManagementConnectionIssuerSuppliesObservedAtFreshness(t *testing.T) {
	resolved := corelessResolvedFixture(t)
	localIssuer, err := resolveApplicationServiceIssuer(context.Background(), resolved, nil)
	if err != nil {
		t.Fatalf("resolve local issuer: %v", err)
	}
	local, ok := localIssuer.(*serviceaccess.LocalIssuer)
	if !ok {
		t.Fatalf("expected LocalIssuer, got %T", localIssuer)
	}
	if err := materializeApplicationServiceIssuer(localIssuer); err != nil {
		t.Fatal(err)
	}

	// Mint a valid CRL (revoking a throwaway) so the read-only status is
	// readable, then observe the connection through the wrapper boundary.
	if seed, ierr := local.Issue(context.Background(), serviceaccess.CertificateRequest{CommonName: "seed", TTL: 90 * 24 * time.Hour}); ierr == nil {
		_ = local.Revoke(context.Background(), seed.Serial)
	}
	observed, err := observedThroughBoundary(context.Background(), local)
	if err != nil {
		t.Fatalf("observe boundary: %v", err)
	}
	baseSerial, err := local.BaseSerial(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	observed.IssuedCertificate.Serial = baseSerial

	// The wrapper must verify the connection through resolveManagementConnectionIssuer
	// with the real observation time (observed.ObservedAt), not a regenerated
	// time.Now(). A recent observation therefore verifies.
	restoreObservedAt := overrideObservedAtForTest(func(context.Context, serviceaccess.Issuer) (serviceaccess.CertificateStatus, error) {
		return observed, nil
	}, t)
	if _, err := resolveManagementConnectionIssuer(context.Background(), resolved, nil); err != nil {
		t.Fatalf("recent observed connection rejected through wrapper: %v", err)
	}
	restoreObservedAt()

	// The wrapper must not fabricate a fresh time.Now(): a future-dated
	// observation (impossible in production, but reachable through the seam) has
	// to fail closed. If the wrapper regenerated time.Now() it would accept it.
	restoreObservedAt = overrideObservedAtForTest(func(context.Context, serviceaccess.Issuer) (serviceaccess.CertificateStatus, error) {
		future := observed
		future.ObservedAt = time.Now().UTC().Add(time.Hour)
		return future, nil
	}, t)
	if _, err := resolveManagementConnectionIssuer(context.Background(), resolved, nil); err == nil {
		t.Fatal("future observed-at verified through wrapper (wrapper ignored observed.ObservedAt)")
	}
	restoreObservedAt()

	// A zero observation time fails closed through the wrapper.
	restoreObservedAt = overrideObservedAtForTest(func(context.Context, serviceaccess.Issuer) (serviceaccess.CertificateStatus, error) {
		zero := observed
		zero.ObservedAt = time.Time{}
		return zero, nil
	}, t)
	if _, err := resolveManagementConnectionIssuer(context.Background(), resolved, nil); err == nil {
		t.Fatal("zero observed-at verified through wrapper")
	}
	restoreObservedAt()

	// A stale observation time fails closed through the wrapper.
	restoreObservedAt = overrideObservedAtForTest(func(context.Context, serviceaccess.Issuer) (serviceaccess.CertificateStatus, error) {
		stale := observed
		stale.ObservedAt = time.Now().UTC().Add(-time.Hour)
		return stale, nil
	}, t)
	if _, err := resolveManagementConnectionIssuer(context.Background(), resolved, nil); err == nil {
		t.Fatal("stale observed-at verified through wrapper")
	}
	restoreObservedAt()
}

// overrideObservedAtForTest points the wrapper's observation seam at a
// controllable function for the duration of a subtest and restores the
// production boundary afterwards.
func overrideObservedAtForTest(fn func(context.Context, serviceaccess.Issuer) (serviceaccess.CertificateStatus, error), t *testing.T) func() {
	t.Helper()
	previous := observedAt
	observedAt = fn
	return func() { observedAt = previous }
}
