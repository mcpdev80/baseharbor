// Package installauthority is the stdlib-free leaf that connects the v0.4.27
// installation-authority contract (internal/coreinstallation/authority.go) to
// the real production authority boundary (serviceaccess.Issuer).
//
// coreinstallation imports serviceaccess for the management-connection boundary,
// so it cannot import serviceaccess directly and also be imported by
// serviceaccess through the lifecycle-lock break at internal/stableid. The
// acyclic leaf here re-exports the exact decision used on the production path
// without adding any behaviour: a caller passes a real serviceaccess.Issuer and
// a live observed CertificateStatus, and this leaf returns the contract's own
// sentinel errors.
package installauthority

import (
	"context"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Issuer is the minimal production authority boundary the installation-authority
// decision consumes. Only the read-only methods used on a verification path are
// named; the mutating Revoke(serial) production action is intentionally absent
// so no verification path can change issuer state.
type Issuer interface {
	TrustBundle(ctx context.Context) (serviceaccess.TrustBundle, error)
	Status(ctx context.Context) (serviceaccess.IssuerStatus, error)
	RevokedStatus(ctx context.Context, serial string) (revoked bool, err error)
}

// Fresh proves that the observed management connection is live, current and not
// foreign or revoked on the real production selection path:
//
//   - identity:   the observed reference must equal authoritativeConnection;
//   - liveness:   Status(ctx).Ready must be true; a failed live check is not
//     downgraded to current READY;
//   - freshness:  the observation is authoritative only when observedAt is
//     non-zero, not in the future and not older than maxAge relative to now —
//     this is the single observed-at freshness policy;
//   - foreign:    a single read-only tri-state revocation call rejects a
//     revoked serial or any unreadable revocation status as foreign.
//
// Every failure returns the matching sentinel error; no local fallback is
// selected.
func Fresh(
	ctx context.Context,
	issuer Issuer,
	observed serviceaccess.CertificateStatus,
	authoritativeConnection string,
	observedAt time.Time,
	maxAge time.Duration,
) error {
	if observed.Identity != authoritativeConnection {
		return serviceaccess.ErrForeignAuthority
	}
	if issuer == nil {
		return serviceaccess.ErrMissingAuthority
	}
	status, err := issuer.Status(ctx)
	if err != nil || !status.Ready {
		return serviceaccess.ErrNotReady
	}
	// Observed-at freshness delegates to the single verification-path kernel;
	// a zero or future observation and a stale observation (now-observedAt past
	// the bound) fail closed before any revocation work. A missing bound falls
	// back to the default recent window so the boundary never opens on an
	// unbounded observation.
	if err := serviceaccess.FreshnessWindow(observedAt, maxAge, time.Now(), serviceaccess.ErrInvalidObservation, serviceaccess.ErrInvalidObservation, serviceaccess.ErrStaleObservation); err != nil {
		return err
	}
	// Exactly one authoritative tri-state revocation call: an unreadable,
	// missing or malformed status (err != nil) rejects, and a revoked serial
	// rejects. There is no window between two calls in which the status could
	// change, so "unknown" can never be accepted as safe.
	revoked, revErr := issuer.RevokedStatus(ctx, observed.IssuedCertificate.Serial)
	if revErr != nil || revoked {
		return serviceaccess.ErrForeignAuthority
	}
	return nil
}
