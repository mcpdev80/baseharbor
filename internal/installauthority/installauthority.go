// Package installauthority is the stdlib-only leaf that holds the minimal
// shared contract primitives between coreinstallation (the installation authority
// and single local state/lock owner) and serviceaccess (the provider-neutral
// management-connection boundary). It imports nothing in the baseharbor module, so
// it can be depended on by both packages without forming an import cycle:
//
//   - coreinstallation declares a local Issuer-consumer interface and consumes the
//     leaf's decision helpers over any type that satisfies it.
//   - serviceaccess exposes the real production Issuer here as a thin adapter so
//     coreinstallation never imports serviceaccess for authority purposes.
//
// The lifecycle-lock path itself stays on develop (coreinstallation owns the
// single local state/lock owner); this leaf only carries the decision contract,
// not the lock.
package installauthority

import (
	"context"
	"time"
)

// Issuer is the provider-neutral management-connection boundary consumed by the
// production control plane. coreinstallation declares this interface locally and
// imports only this leaf, never serviceaccess; serviceaccess provides the real
// implementation as an adapter (see serviceaccess.IssuerAdapter).
//
// Identity is the IssuerReference presented through Status/TrustBundle, liveness
// is Status().Ready, freshness is IssuedCertificate.ExpiresAt, and the read-only
// Revoke(serial) is the foreign/revoked check.
type Issuer interface {
	Status(ctx context.Context) (IssuerStatus, error)
	Revoke(ctx context.Context, serial string) error
}

// IssuerStatus is the live status of a management connection through an Issuer.
type IssuerStatus struct {
	IssuerReference string
	Ready           bool
}

// IssuedCertificate is the live leaf observed through the boundary. Its
// ExpiresAt is the freshness bound; its Serial is the foreign/revoked target of
// the read-only Revoke check.
type IssuedCertificate struct {
	IssuerReference string
	Serial          string
	ExpiresAt       time.Time
}

// CertificateStatus is a live observation of a management connection through a
// real Issuer. Identity is the connection identity the issuer presents, and the
// freshness bound is the live IssuedCertificate.ExpiresAt observed through the
// boundary.
type CertificateStatus struct {
	Identity            string
	IssuedCertificate   IssuedCertificate
}

// Fresh reports whether the observed connection is current. The bound is
// exactly now <= IssuedCertificate.ExpiresAt: a certificate already past its
// expiration is stale, a missing (zero) or future expiration is not an
// authoritative observation and fails closed.
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
// current (Fresh) and not foreign or revoked. It consumes a real Issuer and the
// read-only Revoke(serial) boundary:
//
//   - the observed identity must equal the authoritative connection identity;
//   - the connection must be live (Status().Ready);
//   - the observed connection must be current (Fresh);
//   - a revoked connection is rejected as foreign.
//
// Every failure fails closed with the matching sentinel error.
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
	if err := issuer.Revoke(ctx, observed.IssuedCertificate.Serial); err != nil {
		return ErrForeignAuthority
	}
	return nil
}

// Sentinel errors of the read-only management-connection boundary: a missing,
// foreign, not-ready or expired connection never selects a local fallback.
var (
	// ErrMissingAuthority is returned when no management connection is present at
	// the decision path.
	ErrMissingAuthority = errMissingAuthority
	// ErrForeignAuthority is returned when the management connection identity is
	// foreign or was revoked.
	ErrForeignAuthority = errForeignAuthority
	// ErrNotReady is returned when the live management connection did not confirm
	// current readiness.
	ErrNotReady = errNotReady
	// ErrStaleObservation is returned when the observed connection is past its
	// freshness bound.
	ErrStaleObservation = errStaleObservation
	// ErrInvalidObservation is returned when the observed connection carries a
	// missing (zero) or future expiration.
	ErrInvalidObservation = errInvalidObservation
)
