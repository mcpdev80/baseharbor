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
	// Issuer.ExpiresAt freshness bound.
	ErrStaleObservation = errors.New("stale observed-at connection, not current READY")
	// ErrInvalidObservation is returned when the observed connection carries a
	// missing (zero) or future expiration.
	ErrInvalidObservation = errors.New("invalid observed-at connection, not current READY")
)
