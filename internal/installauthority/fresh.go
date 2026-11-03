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
	Revoked(ctx context.Context, serial string) bool
	RevokedStatus(ctx context.Context, serial string) (revoked bool, err error)
}

// Fresh proves that the observed management connection is live, current and not
// foreign or revoked on the real production selection path:
//
//   - identity:   the observed reference must equal authoritativeConnection;
//   - liveness:   Status(ctx).Ready must be true; a failed live check is not
//     downgraded to current READY;
//   - freshness:  the observed IssuedCertificate.ExpiresAt is the
//     bound; now <= ExpiresAt is fresh, a missing (zero) or future expiration
//     fails closed;
//   - foreign:    a revoked observed serial fails closed as foreign.
//
// Every failure returns the matching sentinel error; no local fallback is
// selected.
func Fresh(
	ctx context.Context,
	issuer Issuer,
	observed serviceaccess.CertificateStatus,
	authoritativeConnection string,
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
	exp := observed.IssuedCertificate.ExpiresAt
	if exp.IsZero() {
		return serviceaccess.ErrInvalidObservation
	}
	if time.Now().After(exp) {
		return serviceaccess.ErrStaleObservation
	}
	if issuer.Revoked(ctx, observed.IssuedCertificate.Serial) {
		return serviceaccess.ErrForeignAuthority
	}
	// Fail-closed tri-state check: an unreadable, missing, foreign or malformed
	// revocation status (err != nil) rejects the connection as foreign, so an
	// unknown status can never be accepted as safe.
	if _, err := issuer.RevokedStatus(ctx, observed.IssuedCertificate.Serial); err != nil {
		return serviceaccess.ErrForeignAuthority
	}
	return nil
}
