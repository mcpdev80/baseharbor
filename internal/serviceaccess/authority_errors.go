package serviceaccess

import (
	"github.com/mcpdev80/baseharbor/internal/installauthority"
)

// Sentinel errors of the v0.4.27 authority-decision trammels. These are the
// read-only boundary failures of the provider-neutral serviceaccess.Issuer,
// re-exported from the installauthority leaf so existing callers that reference
// serviceaccess.Err* still resolve. They are the same values as the leaf
// sentinels; a missing, foreign, not-ready or expired connection never selects a
// local fallback.
var (
	// ErrMissingAuthority is returned when no management connection is present
	// at the decision path.
	ErrMissingAuthority = installauthority.ErrMissingAuthority
	// ErrForeignAuthority is returned when the management connection identity is
	// foreign or was revoked.
	ErrForeignAuthority = installauthority.ErrForeignAuthority
	// ErrNotReady is returned when the live management connection did not confirm
	// current readiness.
	ErrNotReady = installauthority.ErrNotReady
	// ErrStaleObservation is returned when the observed connection is past its
	// Issuer freshness bound.
	ErrStaleObservation = installauthority.ErrStaleObservation
	// ErrInvalidObservation is returned when the observed connection carries a
	// missing (zero) or future expiration.
	ErrInvalidObservation = installauthority.ErrInvalidObservation
)
