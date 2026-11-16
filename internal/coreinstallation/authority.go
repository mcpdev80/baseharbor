package coreinstallation

import (
	"context"
	"errors"
	"time"

	"path/filepath"

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

// ErrUnverifiableInstallation is returned when the installation that is the
// input to ResolverForTargetSelection is not a genuine, verified baseharbor
// Core installation. The live management-connection identity must derive from a
// verified installation; an unverifiable one fails closed before any issuer
// material is produced.
var ErrUnverifiableInstallation = errors.New("installation not verifiable as a genuine baseharbor Core installation, fails closed")

// ErrForeignTarget is returned when the target whose protected state is scanned
// for the live management-connection identity is not the target bound to the
// verified installation. A foreign target selects no issuer material: the
// installation and the target are two distinct authorities.
var ErrForeignTarget = errors.New("target not bound to the verified installation, fails closed")

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

// issuerStateDir is the protected sub-directory of a verified installation's
// target state that owns the live management-connection identity (the base CA
// authority). It is the single, fixed location the resolver reads: it never
// fabricates issuer material and never opens a second trust source.
func issuerStateDir(targetStateRoot string) string {
	return filepath.Join(targetStateRoot, "service-access", "issuer")
}

// observeLocalIssuer reads the base CA authority that the verified installation
// already owns at a protected target-state root through the
// serviceaccess.Issuer boundary WITHOUT fabricating it. It is the strict
// read-only counterpart to EnsureLocalIssuer: EnsureLocalIssuer generates and
// persists a new base CA when issuer.pem is absent, which is exactly the
// authority-creation-during-resolution failure round-2 removes. observeLocalIssuer
// therefore fails closed with ErrUnverifiableInstallation whenever the issuer is
// absent, unreadable, incomplete, partial or not canonically private — it never
// creates the directory, issuer.pem, or a new CA, and it never opens a second
// trust source. A missing authority must not resolve to a permissive empty.
//
// The observe-only shape mirrors the accepted production path in
// cmd/baha/application_service_issuer.go (resolveApplicationServiceIssuer): the
// existing issuer is validated with the read-only CheckLocalIssuer preflight and
// returned as a serviceaccess.LocalIssuer whose load() is deferred to the
// caller's demand — no issuer.pem is written here.
func observeLocalIssuer(dir string) (serviceaccess.Issuer, error) {
	// Read-only preflight: absent authority is a fresh realization and must be
	// recovered, not replaced; partial/unprotected state must be recovered, not
	// silently accepted. CheckLocalIssuer never touches disk for creation.
	if err := serviceaccess.CheckLocalIssuer(dir); err != nil {
		return nil, err
	}
	// The issuer is constructed read-only and validated on demand through its own
	// methods (load() runs only when issuer.pem is read). A missing issuer.pem
	// surfaces here as a missing-issuer error that the caller maps to
	// ErrUnverifiableInstallation before any issuer material is produced.
	return &serviceaccess.LocalIssuer{Dir: dir}, nil
}

// ResolverForTargetSelection is the single new exported resolver exposed by the
// accepted authority boundary internal/coreinstallation. It answers the exact
// question ConnectionSelection.Resolve asks for — the live management-connection
// issuer material — from the VERIFIED INSTALLATION plus the TARGET's own
// identity/state facts alone, without fabricating any application context.
//
// The v0.4.27 contract keeps the management connection separate from the
// placement Target and from the installation. The live identity of that
// management connection (the base CA serial, expiry and revocation) is owned by
// the verified installation in the protected target state at
// <targetStateRoot>/service-access/issuer and is exposed — already accepted — by
// the provider-neutral serviceaccess.Issuer boundary. resolveApplication, the
// app-lifecycle command graph and their resolvedApplication/workload inputs are
// NOT required to read that identity: only the installation root (via Load) and
// the target state root the installation owns are.
//
// ResolverForTargetSelection therefore:
//   - accepts the verified installation State (State.Load already validated it as
//     a genuine baseharbor installation, Owner == "baseharbor", and its identity
//     capability) and the target state root the installation owns;
//   - proves the target state root belongs to this verified installation by
//     loading it back through coreinstallation.Load and requiring the loaded
//     installation ID and bound Target to match — a foreign target yields
//     ErrForeignTarget, never a permissive empty;
//   - materializes and reads back the base CA through the serviceaccess.Issuer
//     boundary (the same production path does on the application lifecycle); a
//     root that holds no verifiable installation yields ErrUnverifiableInstallation;
//   - returns the issuer plus the live observation the selection boundary needs.
//
// The resolver only DERIVES (it does not decide): the authoritative
// liveness/identity/foreign-freshness verification still runs on
// ConnectionSelection.Resolve through the single production boundary. A missing
// installation fails closed with ErrUnverifiableInstallation before any issuer
// material is produced, so a bare `use`/`activate` path that cannot verify an
// installation never silently succeeds.
func ResolverForTargetSelection(
	ctx context.Context,
	verifiedInstallation State,
	targetStateRoot string,
) (serviceaccess.Issuer, serviceaccess.CertificateStatus, error) {
	// Fail closed before any issuer material: an installation that is not a
	// genuine, verified baseharbor Core installation supplies no authority.
	if err := AssertStateIsGenuineInstallation(verifiedInstallation); err != nil {
		return nil, serviceaccess.CertificateStatus{}, ErrUnverifiableInstallation
	}
	if targetStateRoot == "" {
		return nil, serviceaccess.CertificateStatus{}, ErrUnverifiableInstallation
	}
	// The target-state root must be THIS verified installation's own root. The
	// accepted production binding (cmd/baha/application_management_dependency.go)
	// loads the installation from the target-state root and requires
	// state.Spec.Target == the selected Target name: a root that does not load
	// back as the verified installation is a foreign target and fails closed.
	if err := verifyTargetRootBoundToInstallation(targetStateRoot, verifiedInstallation); err != nil {
		return nil, serviceaccess.CertificateStatus{}, err
	}
	// Observe and read back the base CA from the verified installation's own
	// protected target state through the already-accepted serviceaccess.Issuer
	// boundary. This is strictly READ-ONLY / derive-only: it must NOT materialize
	// issuer material. EnsureLocalIssuer fabricates and persists a new base CA
	// when issuer.pem is absent, so it is forbidden here — the resolver contract
	// is to DERIVE the existing authority, never to decide or create it. A root
	// that holds no verifiable installation fails closed with a named error
	// before any issuer material is produced, so a missing/incomplete authority
	// never silently fabricates a new trust root during selection.
	issuerDir := issuerStateDir(targetStateRoot)
	issuer, err := observeLocalIssuer(issuerDir)
	if err != nil {
		return nil, serviceaccess.CertificateStatus{}, ErrUnverifiableInstallation
	}
	observed, err := observedThroughTargetBoundary(ctx, issuer)
	if err != nil {
		return nil, serviceaccess.CertificateStatus{}, ErrUnverifiableInstallation
	}
	return issuer, observed, nil
}

// verifyTargetRootBoundToInstallation proves that the given target-state root is
// the verified installation's own root. It loads the installation from that root
// (the same accepted production binding cmd/baha/application_management_
// dependency.go uses) and requires the loaded state to be the verified
// installation bound to the same Target. A target root that does not load back
// as the verified installation is a foreign target and fails closed with
// ErrForeignTarget; a root that does not hold a verifiable installation is
// ErrUnverifiableInstallation.
func verifyTargetRootBoundToInstallation(targetStateRoot string, verifiedInstallation State) error {
	state, err := Load(targetStateRoot)
	if err != nil {
		return ErrUnverifiableInstallation
	}
	if state.ID != verifiedInstallation.ID || state.Spec.Target != verifiedInstallation.Spec.Target {
		return ErrForeignTarget
	}
	return nil
}

// observedThroughTargetBoundary is the read-only observation boundary that
// supplies the live connection identity (issuer reference, base serial, base
// expiry) observed through the existing issuer — the same read-only shape
// production uses — without issuing a new leaf. It is a package var so the
// observation can be substituted in tests while production always runs this
// function.
var observedThroughTargetBoundary = func(ctx context.Context, issuer serviceaccess.Issuer) (serviceaccess.CertificateStatus, error) {
	trust, err := issuer.TrustBundle(ctx)
	if err != nil {
		return serviceaccess.CertificateStatus{}, err
	}
	baseSerial, err := issuer.BaseSerial(ctx)
	if err != nil {
		return serviceaccess.CertificateStatus{}, err
	}
	baseExpiry, err := issuer.BaseExpiry(ctx)
	if err != nil {
		return serviceaccess.CertificateStatus{}, err
	}
	return serviceaccess.CertificateStatus{
		Identity: trust.IssuerReference,
		IssuedCertificate: serviceaccess.IssuedCertificate{
			IssuerReference: trust.IssuerReference,
			Serial:          baseSerial,
			ExpiresAt:       baseExpiry,
		},
		ObservedAt: time.Now().UTC(),
	}, nil
}
