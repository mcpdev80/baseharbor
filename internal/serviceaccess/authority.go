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
	return FreshnessWindow(s.ObservedAt, s.maxAge(), now, ErrInvalidObservation, ErrInvalidObservation, ErrStaleObservation)
}

// maxAge is the observation freshness window. The zero value fails closed: any
// past observation is stale. It delegates to the single DefaultObservationMaxAge
// bound, so no caller hardcodes a second minute- or hour-scale window.
func (s CertificateStatus) maxAge() time.Duration {
	if s.ObservedAt.IsZero() {
		return 0
	}
	return DefaultObservationMaxAge
}

// FreshnessWindow is the single verification-path observation-freshness kernel.
// It enforces the three shared rules for an observed-at freshness bound: a zero
// observation time is not authoritative (ErrInvalidObservation), a future
// observation is not an authoritative observation of the past (futureError),
// and an observation older than the window (now-ObservedAt > maxAge) is stale
// (ErrStaleObservation); a default window applies when maxAge is zero so a
// boundary never opens on an unbounded observation. The default window is the
// single DefaultObservationMaxAge bound, so there is one direction, not a
// second hard-coded number. It is the single source consumed by
// CertificateStatus.Fresh, coreinstallation.ObservedState.Fresh and
// installauthority.Fresh — no third copy of the zero/future/window rules.
// futureError is caller-owned so distinct sentinel errors stay distinct.
func FreshnessWindow(observedAt time.Time, maxAge time.Duration, now time.Time, zeroError, futureError, staleError error) error {
	if observedAt.IsZero() {
		return zeroError
	}
	if observedAt.After(now) {
		return futureError
	}
	window := maxAge
	if window == 0 {
		window = DefaultObservationMaxAge
	}
	if now.Sub(observedAt) > window {
		return staleError
	}
	return nil
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
