package serviceaccess

import (
	"context"
	"time"
)

// authority.go holds the minimal integration trammels that connect the
// v0.4.27 authority contract (see coreinstallation/authority.go and ADR 0021)
// to the real production authority boundary: the provider-neutral
// serviceaccess.Issuer consumed by the control plane.
//
// These helpers do NOT change any production ServiceIssuer / LocalIssuer
// behavior. They only add a small value (CertificateStatus) that carries the
// connection identity and the live IssuedCertificate observed through the
// boundary, and two decision helpers (FreshlyObservedConnection,
// ForeignOrRevokedConnection) that consume a real selection and the read-only
// Revoked(serial) foreign/revoked check. Every production issuer already
// satisfies Issuer, so no service code changes.

// CertificateStatus is a live observation of a management connection through a
// real serviceaccess.Issuer. Identity is the connection identity the issuer
// presents (IssuerReference), the freshness bound is the observation time, and
// the Serial is the foreign/revoked target of the read-only RevokedStatus check.
type CertificateStatus struct {
	// Identity is the connection identity observed through the boundary
	// (IssuerStatus.IssuerReference / TrustBundle.IssuerReference).
	Identity string
	// IssuedCertificate is the live leaf observed through the boundary. Its
	// Serial is the foreign/revoked target of the read-only RevokedStatus check.
	IssuedCertificate IssuedCertificate
	// ObservedAt is the observation time of this status, captured when the
	// status was produced through the boundary. It is the freshness bound the
	// verification boundary checks; it is not a certificate expiry.
	ObservedAt time.Time
}

// Fresh reports whether the observed connection is current. The boundary is the
// observation time: now-ObservedAt must be within the freshness window
// (ObservedAt must be non-zero, not in the future and recent enough). The
// certificate expiry is not the freshness bound; a positive certificate expiry
// is normally in the future, so it cannot serve as the observation freshness
// bound.
func (s CertificateStatus) Fresh(now time.Time) error {
	if s.ObservedAt.IsZero() {
		return ErrInvalidObservation
	}
	if s.ObservedAt.After(now) {
		return ErrInvalidObservation
	}
	if now.Sub(s.ObservedAt) > s.maxAge() {
		return ErrStaleObservation
	}
	return nil
}

// maxAge is the observation freshness window. The zero value fails closed: any
// past observation is stale.
func (s CertificateStatus) maxAge() time.Duration {
	const defaultWindow = 5 * time.Minute
	if s.ObservedAt.IsZero() {
		return 0
	}
	return defaultWindow
}

// FreshlyObservedConnection proves that the observed management connection is
// current (Fresh) and not foreign or revoked. It consumes a real selection and
// the read-only tri-state RevokedStatus boundary:
//
//   - the observed identity must equal the authoritative connection identity;
//   - the connection must be live (Status().Ready);
//   - the observed connection must be current (Fresh);
//   - a revoked connection (RevokedStatus) is rejected as foreign, and an
//     unreadable revocation status is rejected as foreign too.
//
// Every failure fails closed with the matching sentinel error. The read-only
// RevokedStatus rejects without changing any issuer state; mutating Revoke is
// never called on a verification path.
func FreshlyObservedConnection(ctx context.Context, issuer Issuer, observed CertificateStatus, authoritativeConnection string) error {
	if observed.Identity != authoritativeConnection {
		return ErrForeignAuthority
	}
	if issuer == nil {
		return ErrMissingAuthority
	}
	status, err := issuer.Status(ctx)
	if err != nil || !status.Ready {
		return ErrNotReady
	}
	if err := observed.Fresh(time.Now()); err != nil {
		return err
	}
	// The single read-only tri-state foreign/revoked check: an unreadable or
	// revoked status rejects. RevokedStatus never writes state; a missing,
	// foreign or unreadable revocation status fails closed by rejecting the
	// connection.
	revoked, revErr := issuer.RevokedStatus(ctx, observed.IssuedCertificate.Serial)
	if revErr != nil || revoked {
		return ErrForeignAuthority
	}
	return nil
}

// ForeignOrRevokedConnection is the minimal read-only foreign/revoked check for
// a connection that is already known fresh: it fails closed when the observed
// identity does not match, or when the connection is not live, or when the
// read-only tri-state RevokedStatus rejects the observed serial or an
// unreadable revocation status.
func ForeignOrRevokedConnection(ctx context.Context, issuer Issuer, observed CertificateStatus, authoritativeConnection string) error {
	if observed.Identity != authoritativeConnection {
		return ErrForeignAuthority
	}
	status, err := issuer.Status(ctx)
	if err != nil || !status.Ready {
		return ErrNotReady
	}
	revoked, revErr := issuer.RevokedStatus(ctx, observed.IssuedCertificate.Serial)
	if revErr != nil || revoked {
		return ErrForeignAuthority
	}
	return nil
}
