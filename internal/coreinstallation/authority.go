package coreinstallation

import (
	"errors"
	"time"
)

// Selection names the three independent, never-interchanged authorities of the
// v0.4.27 contract (see ADR 0021). Management connection and Target are distinct
// from the installation and never become its authority.
type Selection struct {
	Installation string `json:"installation"`
	Target       string `json:"target"`
}

func (s Selection) Validate() error {
	if s.Installation == "" || s.Target == "" {
		return errors.New("selection requires both installation and target")
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

// RemoteAuthority records that a remote client only *addresses* the selected
// installation through its management connection and never owns authoritative
// state. Remote clients fall back to no local execution.
type RemoteAuthority struct {
	Installation string    `json:"installation"`
	ManagedBy    StateOwner `json:"managed_by"`
	// ObservedAt is the freshness bound. A remote observation is authoritative
	// only up to ObservedAt; beyond that it is stale and rejected, never claimed
	// READY.
	ObservedAt time.Time `json:"observed_at"`
}

// Fresh reports whether the remote observation is still authoritative. A missing
// or past deadline is stale and must be rejected, never downgraded to READY. The
// observation is fresh exactly at its observed-at instant and before.
func (a RemoteAuthority) Fresh(now time.Time) bool {
	return !a.ObservedAt.IsZero() && !a.ObservedAt.Before(now)
}

// ErrRemoteStateRejected is returned when a remote client tries to own state, a
// stale observation is accepted as current, or local execution is used as a
// fallback for a missing/foreign/expired remote authority.
var ErrRemoteStateRejected = errors.New("remote client does not own authoritative state and has no local fallback")

// ErrStaleObservation is returned when a cached last-known status is accepted as
// current state instead of being reported as stale.
var ErrStaleObservation = errors.New("stale observed-at observation rejected, not current READY")

// ErrLocalFallback is returned when a missing, foreign or expired remote
// management connection tries to select a local runtime or installation.
var ErrLocalFallback = errors.New("missing/foreign/expired remote authority fails closed, no local fallback")

// ErrConflatedSelection is returned when management connection, Target and
// installation are interchanged as if they were the same authority.
var ErrConflatedSelection = errors.New("management connection, Target and installation are distinct selections")

// AssertDistinct rejects a selection that conflates the installation with its
// management connection or Target. The three authorities are independent.
func (s Selection) AssertDistinct() error {
	if s.Installation == s.Target {
		return ErrConflatedSelection
	}
	return nil
}

// RejectOwnership proves that a remote client never owns authoritative state.
func (a RemoteAuthority) RejectOwnership() error {
	if a.ManagedBy != LocalOwner {
		return ErrRemoteStateRejected
	}
	return nil
}

// Accept rejects a stale remote observation. Callers must reject it instead of
// claiming current READY.
func (a RemoteAuthority) Accept(now time.Time) error {
	if !a.Fresh(now) {
		return ErrStaleObservation
	}
	return nil
}
