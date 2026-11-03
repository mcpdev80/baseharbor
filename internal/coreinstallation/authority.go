package coreinstallation

import (
	"context"
	"errors"
	"time"

	"github.com/mcpdev80/baseharbor/internal/installauthority"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// The three distinct, never-interchanged authorities of the v0.4.27 contract
// (see ADR 0021). They are represented as separate meaningful types and the
// minimal authority decision is connected to the actual authorized production
// selection path:
//
//   - Installation  -> coreinstallation (this package) owns deployment state and
//     the single local state/lock owner. State.Load validates that the state is
//     a genuine baseharbor installation (Owner == "baseharbor"): this is the
//     authoritative consent/identity gate.
//   - Management connection -> serviceaccess.Issuer, the provider-neutral
//     certificate-authority boundary consumed by the production control plane.
//     Its Status/Ready is liveness, its IssuerReference is the connection
//     identity, its read-only Revoke(serial) rejects a foreign (revoked)
//     connection, and the live IssuedCertificate.ExpiresAt is the freshness
//     bound.
//   - Target        -> the selected deployment target (coreinstallation.Spec.Target),
//     distinct from both the installation and its management connection.
//
// No local fallback is selected for a missing, foreign or expired remote
// authority: a missing or foreign selection never selects a local runtime.

// ErrConflatedSelection is returned when the installation, management connection
// and Target are interchanged as if they were the same authority.
var ErrConflatedSelection = errors.New("management connection, Target and installation are distinct selections")

// ErrRemoteStateRejected is returned when a remote client tries to own
// authoritative state, a stale observation is accepted as current, or local
// execution is used as a fallback for a missing/foreign/expired authority.
var ErrRemoteStateRejected = errors.New("remote client does not own authoritative state and has no local fallback")

// ErrStaleObservation is returned when a stale observed-at observation is
// accepted as current state instead of being reported as stale.
var ErrStaleObservation = errors.New("stale observed-at observation rejected, not current READY")

// ErrMissingAuthority is returned when no management connection identity is
// present at the decision path.
var ErrMissingAuthority = errors.New("missing management connection authority, fails closed")

// ErrForeignAuthority is returned when the management connection identity is
// foreign (does not match the authoritative context) or was revoked.
var ErrForeignAuthority = errors.New("foreign management connection authority, fails closed")

// ErrNotReady is returned when the live management-connection verification did
// not confirm current readiness; a failed live check never becomes current READY.
var ErrNotReady = errors.New("management connection not verified as ready, not current READY")

// ErrFutureObservation is returned when an observation timestamp lies in the
// future; a future observation is not an authoritative observation of the past.
var ErrFutureObservation = errors.New("observation timestamp is in the future, not an authoritative observation")

// ErrInvalidObservation is returned when the observation timestamp is zero; a
// missing observation time is not authoritative.
var ErrInvalidObservation = errors.New("observation timestamp is missing, not authoritative")

// Selection names the installation and the Target as two independent selections.
// The management connection is the third, distinct authority and is validated
// through the serviceaccess.Issuer boundary, not as a string on this value.
type Selection struct {
	Installation string `json:"installation"`
	Target       string `json:"target"`
}

// Validate rejects an incomplete selection. A selection must address both the
// installation and a distinct Target.
func (s Selection) Validate() error {
	if s.Installation == "" {
		return errors.New("selection requires an installation")
	}
	if s.Target == "" {
		return errors.New("selection requires a Target")
	}
	return nil
}

// AssertDistinct rejects a selection that conflates the installation with its
// Target. The three authorities are independent and never interchanged.
func (s Selection) AssertDistinct() error {
	if s.Installation == s.Target {
		return ErrConflatedSelection
	}
	return nil
}

// StateOwner is the single authoritative owner of deployment state/lock for one
// installation's admitted Targets. Local direct execution owns this state; no
// second local owner exists and no reconciliation runs between a "local cache"
// and a "remote truth".
type StateOwner string

const (
	// LocalOwner is the sole authoritative owner for local direct execution.
	LocalOwner StateOwner = "local-core"
)

// ObservedState is an authoritative remote observation of a single installation
// through its management connection. It never owns state and never falls back to
// local execution.
//
// ObservedAt is an observation time, not an expiry deadline. The explicit
// freshness policy (see the contract) rejects zero, future and expired
// observations, and a failed live verification can never become current READY.
type ObservedState struct {
	// Installation is the baseharbor installation identity (== coreinstallation
	// identity that State.Load validates).
	Installation string `json:"installation"`
	// Target is the distinct selection addressed through the management
	// connection; it must never equal Installation.
	Target string `json:"target"`
	// ObservedAt is the observation time of the last confirmed management
	// connection. It is not a deadline.
	ObservedAt time.Time `json:"observed_at"`
	// MaxAge is the observation freshness window. The observation is fresh only
	// when now-ObservedAt <= MaxAge. The zero value fails closed: any past
	// observation is stale.
	MaxAge time.Duration `json:"max_age"`
}

// RejectOwnership proves that a remote client never owns authoritative state and
// that a missing/foreign/expired management connection selects no local
// execution. A remote client is never the local owner.
func (o ObservedState) RejectOwnership() error {
	if o.Target == o.Installation {
		return ErrConflatedSelection
	}
	if o.Installation == "" || o.Target == "" {
		return ErrMissingAuthority
	}
	// A remote client is never the local owner; selecting local execution for a
	// missing/foreign/expired authority fails closed.
	return ErrRemoteStateRejected
}

// Fresh applies the freshness policy to an observation time. It is the single
// authoritative freshness policy consumed by every verification boundary,
// including installauthority.Fresh, so there is one policy, not a second copy.
//
//   - a zero observation time is not authoritative (ErrInvalidObservation);
//   - a future observation time is not an authoritative observation of the past
//     (ErrFutureObservation);
//   - an observation older than MaxAge relative to now is stale (ErrStaleObservation);
//   - an observation within the window is fresh.
//
// The observation is authoritative exactly when it is non-zero, not in the
// future and recent enough (now-ObservedAt <= MaxAge).
func (o ObservedState) Fresh(now time.Time) error {
	return serviceaccess.FreshnessWindow(o.ObservedAt, o.MaxAge, now, ErrInvalidObservation, ErrFutureObservation, ErrStaleObservation)
}

// AuthoritativeConnection proves that a remote decision is made through a
// genuine, live, non-expired management connection on the actual production
// selection path. It consumes the real serviceaccess.Issuer boundary:
//
//   - Identity:   the issuer reference observed at the decision path must equal
//     the authoritative connection identity (foreign references are rejected).
//   - Liveness:   Status().Ready must be true; a failed live check is not
//     downgraded to current READY.
//   - Freshness:  the observed connection's IssuedCertificate.ExpiresAt is the
//     bound; now <= ExpiresAt is fresh, a past/future/zero expiration fails
//     closed.
//   - Foreign:    a revoked connection (Revoked) is rejected as foreign.
//
// Any failure fails closed with the matching sentinel error; no local fallback
// is selected. The observed connection is captured as a CertificateStatus so the
// identity, liveness and the read-only Revoked(serial) foreign/revoked check all
// run over the same real selection.
func (o ObservedState) AuthoritativeConnection(
	ctx context.Context,
	issuer serviceaccess.Issuer,
	observed serviceaccess.CertificateStatus,
	authoritativeConnection string,
) error {
	// 1. Distinctness of the three selections: installation, Target and the
	// management connection are never interchanged.
	if o.Installation == "" || o.Target == "" || o.Target == o.Installation {
		return ErrConflatedSelection
	}
	// 2. A management connection must be present at the decision path; a missing
	// one fails closed before any liveness or freshness work.
	if issuer == nil {
		return ErrMissingAuthority
	}
	// 3. Observed-at freshness: the observation time is authoritative only when
	// it is non-zero, not in the future, and not stale. This check runs before
	// the certificate-exiry/liveness work so a missing/future/stale observation
	// is reported as the freshness sentinel, not as a liveness or foreign
	// failure.
	if err := o.Fresh(time.Now()); err != nil {
		return err
	}
	// 4. Liveness + identity + foreign/revoked rejection over the real
	// production selection path. The read-only tri-state RevokedStatus boundary
	// confirms the observed serial without changing issuer state and fails
	// closed on an unreadable revocation status. The observed-at freshness bound
	// is supplied from the same observation this method already validated.
	if err := installauthority.Fresh(ctx, issuer, observed, authoritativeConnection, o.ObservedAt, o.MaxAge); err != nil {
		return mapBoundaryErr(err)
	}
	// A foreign connection never selects a local runtime.
	return nil
}

// VerifyConnectionIdentity proves that a connection identity is genuine, live,
// current and not foreign, and that a revoked identity is rejected. It is the
// minimal executable realization of the management-connection role against the
// production serviceaccess.Issuer boundary: liveness and identity through
// Status, freshness through a captured observed-at time, and the foreign/revoked
// rejection through the read-only tri-state RevokedStatus.
func VerifyConnectionIdentity(
	ctx context.Context,
	issuer serviceaccess.Issuer,
	observed serviceaccess.CertificateStatus,
	authoritativeConnection string,
) error {
	if issuer == nil {
		return ErrMissingAuthority
	}
	observedAt := observed.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	return installauthority.Fresh(ctx, issuer, observed, authoritativeConnection, observedAt, serviceaccess.DefaultObservationMaxAge)
}

// mapBoundaryErr maps a serviceaccess boundary sentinel to the corresponding
// coreinstallation sentinel so callers see the contract's own errors.
func mapBoundaryErr(err error) error {
	switch err {
	case serviceaccess.ErrMissingAuthority:
		return ErrMissingAuthority
	case serviceaccess.ErrForeignAuthority:
		return ErrForeignAuthority
	case serviceaccess.ErrNotReady:
		return ErrNotReady
	case serviceaccess.ErrStaleObservation:
		return ErrStaleObservation
	case serviceaccess.ErrInvalidObservation:
		return ErrInvalidObservation
	default:
		return err
	}
}

// AssertStateIsGenuineInstallation verifies that a loaded installation state is
// a genuine baseharbor installation (Owner == "baseharbor") on the production
// decision path. This is the authoritative consent/identity gate; a foreign or
// incompatible installation is rejected and selects no local execution.
func AssertStateIsGenuineInstallation(state State) error {
	if state.Owner != "baseharbor" {
		return ErrForeignAuthority
	}
	if state.ID == "" {
		return ErrMissingAuthority
	}
	return nil
}
