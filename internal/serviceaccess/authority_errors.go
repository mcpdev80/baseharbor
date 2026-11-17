package serviceaccess

import "errors"

// Sentinel errors used by the v0.4.27 authority-decision trammels
// (authority.go). These are the read-only boundary failures of the
// provider-neutral serviceaccess.Issuer: a missing, foreign, not-ready or
// expired connection never selects a local fallback.
var (
	// ErrMissingAuthority is returned when no management connection is present
	// at the decision path.
	ErrMissingAuthority = errors.New("missing management connection authority, fails closed")
	// ErrForeignAuthority is returned when the management connection identity is
	// foreign or was revoked.
	ErrForeignAuthority = errors.New("foreign management connection authority, fails closed")
	// ErrNotReady is returned when the live management connection did not confirm
	// current readiness.
	ErrNotReady = errors.New("management connection not verified as ready, not current READY")
	// ErrStaleObservation is returned when the observed connection is past its
	// observed-at freshness bound.
	ErrStaleObservation = errors.New("stale observed-at connection, not current READY")
	// ErrInvalidObservation is returned when the observed connection carries a
	// missing (zero) or future observation time.
	ErrInvalidObservation = errors.New("invalid observed-at connection, not current READY")
	// ErrExpiredAuthority is returned when the observed connection's certificate
	// carries a zero or past expiry on the verification path: the observed CA
	// certificate is not a live, current authority, so the path fails closed. This
	// is the real certificate-lifetime bound, distinct from the observed-at
	// freshness window (ErrStaleObservation).
	ErrExpiredAuthority = errors.New("management connection certificate expired, not current, fails closed")
)

// Internal read-only revocation-status failures. The boundary treats any of
// these (err != nil) as a rejection: an unknown revocation status must never be
// accepted as safe.
var (
	// errMissingRevocationStatus is returned when no CRL is present: the
	// revocation status cannot be confirmed, so the path fails closed.
	errMissingRevocationStatus = errors.New("revocation status unreadable: no CRL, fails closed")
	// errMalformedSerial is returned when the serial cannot be parsed or is
	// outside the supported range: the target cannot be checked, fails closed.
	errMalformedSerial = errors.New("revocation target serial malformed, fails closed")
)
