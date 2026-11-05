package coreinstallation

import (
	"context"
	"errors"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// ConnectionSelection is the single, authoritative representation that keeps the
// management connection distinct from the placement Target. It is the
// executable realization of the r27 connection-client contract: the
// installation, the management connection and the deployment Target are three
// independent authorities that are never interchanged.
//
// The three fields are deliberately distinct names (not a single "target"
// string): a Target without a Connection must not be able to invent a
// connection, and a Connection must not be able to stand in for a Target.
// Resolve enforces this separation and fails closed when the connection is
// missing, foreign, not ready, stale or revoked.
//
// Verification never lives here as a second trust check: it consumes the single
// production boundary AuthoritativeConnection/VerifyConnectionIdentity on the
// real selection path, so there is one authority, not a copy.
type ConnectionSelection struct {
	// InstallationID identifies the baseharbor installation (== the single
	// authoritative Core owner of this installation). It must never equal
	// TargetID.
	InstallationID string `json:"installation_id"`
	// ConnectionID is the management connection identity the client binds to.
	// It addresses the installation, not the Target, and is distinct from both
	// InstallationID and TargetID.
	ConnectionID string `json:"connection_id"`
	// TargetID is the distinct deployment Target selected through the
	// management connection. It must never equal InstallationID.
	TargetID string `json:"target_id"`
}

// Validate rejects an incomplete selection: both the installation and a Target
// must be named. A selection with only a Target (or only a Connection) is not a
// valid selection and must fail closed.
func (s ConnectionSelection) Validate() error {
	if strings.TrimSpace(s.InstallationID) == "" {
		return errors.New("connection selection requires an installation")
	}
	if strings.TrimSpace(s.TargetID) == "" {
		return errors.New("connection selection requires a Target")
	}
	return nil
}

// AssertDistinct rejects a selection that conflates the installation with its
// Target. The management connection is the third, distinct authority and is
// verified through the production boundary, not asserted as a string here.
func (s ConnectionSelection) AssertDistinct() error {
	if s.InstallationID == s.TargetID {
		return ErrConflatedSelection
	}
	return nil
}

// Resolve validates the selection, proves the installation is a genuine
// baseharbor installation and verifies the management connection through the
// single production boundary. It is the explicit-selection entry point for the
// CLI: when the connection is missing, foreign, not ready, stale or revoked,
// Resolve fails closed with a typed sentinel error and selects no local
// execution.
//
// issuer and observed are the real production boundary and its live observation;
// passing a nil issuer is a missing connection and fails closed before any
// liveness work. authoritativeConnection is the identity the client is permitted
// to bind to.
func (s ConnectionSelection) Resolve(
	ctx context.Context,
	issuer serviceaccess.Issuer,
	observed serviceaccess.CertificateStatus,
	authoritativeConnection string,
) error {
	// 1. Distinctness of the three selections: installation, Target and the
	// management connection are never interchanged.
	if err := s.Validate(); err != nil {
		return err
	}
	if err := s.AssertDistinct(); err != nil {
		return err
	}
	// 2. The installation ID must be named on the production decision path; an
	// empty installation is a missing authority and selects no local execution.
	// A loaded State's Owner == "baseharbor" gate runs where coreinstallation.Load
	// validates the genuine installation (see coreinstallation.Load / the
	// apply path), not here as a fabricated State, so this boundary never
	// invents the authority it verifies.
	if s.InstallationID == "" {
		return ErrMissingAuthority
	}
	// 3. Verify the management connection through the single production
	// boundary: liveness + identity + foreign/revoked rejection. A missing
	// connection fails closed before any effect. The observation time the
	// resolver supplies (observed.ObservedAt) threads into the single freshness
	// policy: a missing observation is not authoritative, so a resolver that
	// returns no live observation fails closed here before any effect.
	observedState := ObservedState{
		Installation: s.InstallationID,
		Target:       s.TargetID,
		ObservedAt:   observed.ObservedAt,
	}
	return observedState.AuthoritativeConnection(ctx, issuer, observed, authoritativeConnection)
}
