package installauthority

import "errors"

var (
	errMissingAuthority = errors.New("missing management connection authority, fails closed")
	errForeignAuthority = errors.New("foreign management connection authority, fails closed")
	errNotReady         = errors.New("management connection not verified as ready, not current READY")
	errStaleObservation = errors.New("stale observed-at connection, not current READY")
	errInvalidObservation = errors.New("invalid observed-at connection, not current READY")
)
