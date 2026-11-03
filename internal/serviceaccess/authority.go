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
// presents (IssuerReference), and the freshness bound is the live
// IssuedCertificate.ExpiresAt observed through the boundary.
type CertificateStatus struct {
	// Identity is the connection identity observed through the boundary
	// (IssuerStatus.IssuerReference / TrustBundle.IssuerReference).
	Identity string
	// IssuedCertificate is the live leaf observed through the boundary. Its
	// ExpiresAt is the freshness bound; its Serial is the foreign/revoked
	// target of the read-only Revoked check.
	IssuedCertificate IssuedCertificate
}

// Fresh reports whether the observed connection is current. The boundary is
// exactly now <= Issuer.ExpiresAt: a certificate that is already past its
// expiration is stale, a missing (zero) or future expiration is not an
// authoritative observation and fails closed. No separate maxAge constant is
// used.
func (s CertificateStatus) Fresh(now time.Time) error {
	exp := s.IssuedCertificate.ExpiresAt
	if exp.IsZero() {
		return ErrInvalidObservation
	}
	if now.After(exp) {
		return ErrStaleObservation
	}
	return nil
}

// FreshlyObservedConnection proves that the observed management connection is
// current (Fresh) and not foreign or revoked. It consumes a real selection and
// the read-only Revoke(serial) boundary:
//
//   - the observed identity must equal the authoritative connection identity;
//   - the connection must be live (Status().Ready);
//   - the observed connection must be current (Fresh);
//   - a revoked connection (Revoked) is rejected as foreign.
//
// Every failure fails closed with the matching sentinel error. The read-only
// Revoked(serial) rejects without changing any issuer state; mutating Revoke is
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
	// The read-only foreign/revoked check: a revoked certificate serial
	// rejects. Revoked never writes state; a missing, foreign or unreadable
	// revocation status fails closed by rejecting the connection.
	if issuer.Revoked(ctx, observed.IssuedCertificate.Serial) {
		return ErrForeignAuthority
	}
	return nil
}

// ForeignOrRevokedConnection is the minimal read-only foreign/revoked check for
// a connection that is already known fresh: it fails closed when the observed
// identity does not match, or when the connection is not live, or when the
// read-only Revoked(serial) rejects the observed serial.
func ForeignOrRevokedConnection(ctx context.Context, issuer Issuer, observed CertificateStatus, authoritativeConnection string) error {
	if observed.Identity != authoritativeConnection {
		return ErrForeignAuthority
	}
	status, err := issuer.Status(ctx)
	if err != nil || !status.Ready {
		return ErrNotReady
	}
	if issuer.Revoked(ctx, observed.IssuedCertificate.Serial) {
		return ErrForeignAuthority
	}
	return nil
}
