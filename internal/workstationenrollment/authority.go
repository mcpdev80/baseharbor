package workstationenrollment

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Owner is the bounded lifecycle seam that anchors the currently issued
// workstation leaf for a scope. The device-lifecycle Renew/Revoke methods need
// the serial of the leaf that is currently authoritative for a scope so they can
// renew or revoke exactly that leaf; the one-use enrollment store never keeps
// the issued serial, so the production path supplies it from the durable
// registry (internal/database) and tests supply it from an in-memory tracker.
// The seam is deliberately narrow: it answers only "what serial is currently
// authoritative for this scope, if any?".
type Owner interface {
	// CurrentSerial returns the serial of the leaf currently authoritative for
	// the scope. ok is false when no leaf has been issued for the scope yet.
	// A missing, foreign or unreadable record fails closed with err != nil.
	CurrentSerial(ctx context.Context, scope Scope) (serial string, ok bool, err error)
}

// Authority binds a one-use, recipient-bound workstation enrollment to the
// existing authoritative issuer (internal/serviceaccess). It creates a short
// bootstrap authorization, consumes a single-use store exactly once when the
// workstation enrolls, signs the workstation-owned key through the protected
// signing boundary, and verifies the signed leaf against the authoritative
// trust bundle before returning it.
//
// The one-use store enforces single use: a consumed bootstrap fails closed. A
// foreign scope, an elapsed/future window, a missing/invalid revocation status
// or a malformed leaf all fail closed. No private key is generated or returned;
// only the signed leaf and public trust are produced.
//
// The bounded device-lifecycle methods Renew, Revoke and RenewAfterRevoked live
// here too. They operate on exactly one scope: Renew re-signs that scope's key
// for a fresh leaf alone, Revoke adds only that scope's serial to the issuer
// CRL, and RenewAfterRevoked refuses a renewal whose current leaf is already
// revoked. A foreign scope, an unreadable revocation status or a missing issued
// identity all fail closed. The current leaf serial is anchored through the
// Owner seam.
type Authority struct {
	store  Store
	issuer serviceaccess.CSRIssuer
	owner  Owner
}

// New returns an Authority or an error if the store or issuer is nil. The owner
// seam is optional: a nil owner means the lifecycle methods cannot anchor the
// current issued serial and report it unavailable, so a caller that never
// supplied a durable registry still builds an authority for Create/Enroll only.
func New(store Store, issuer serviceaccess.CSRIssuer, owner Owner) (*Authority, error) {
	if store == nil || issuer == nil {
		return nil, errors.New("workstation enrollment requires a one-use store and an existing issuer")
	}
	return &Authority{store: store, issuer: issuer, owner: owner}, nil
}

// Bootstrap contains the one-use bootstrap credentials and the validity
// window.
type Bootstrap struct {
	RequestScope   Scope         `json:"-"`
	Token          string        `json:"token"`
	Nonce          string        `json:"nonce"`
	ExpiresAt      time.Time     `json:"expires_at"`
	CertificateTTL time.Duration `json:"certificate_ttl"`
}

// Create persists a one-use bootstrap authorization for the given scope and
// returns the digested credentials plus the short-lived validity window.
// Lifetime is bounded to five minutes; the certificate TTL is bounded to at
// most 24 hours.
func (a *Authority) Create(ctx context.Context, scope Scope, lifetime, certificateTTL time.Duration) (Bootstrap, error) {
	if ctx.Err() != nil || scope.Validate() != nil || lifetime <= 0 || lifetime > 5*time.Minute || certificateTTL <= 0 || certificateTTL > 24*time.Hour {
		return Bootstrap{}, ErrDenied
	}
	token, err := randomCredential()
	if err != nil {
		return Bootstrap{}, err
	}
	nonce, err := randomCredential()
	if err != nil {
		return Bootstrap{}, err
	}
	expiry := time.Now().UTC().Add(lifetime)
	bootstrap := Bootstrap{
		RequestScope:   scope,
		Token:          token,
		Nonce:          nonce,
		ExpiresAt:      expiry,
		CertificateTTL: certificateTTL,
	}
	if err := a.store.Create(ctx, bootstrap); err != nil {
		return Bootstrap{}, errors.New("cannot persist workstation enrollment authorization")
	}
	return bootstrap, nil
}

// Enroll consumes the one-use store exactly once, signs the workstation-owned
// CSR through the protected signing boundary, and verifies the issued leaf
// against the authoritative trust bundle before returning it.
//
// A consumed bootstrap, an invalid scope, an unreadable revocation status, or a
// malformed/expired leaf all fail closed without spending the one-use credential.
func (a *Authority) Enroll(ctx context.Context, request Request) (Result, error) {
	if ctx.Err() != nil || !validCredential(request.token) || !validCredential(request.nonce) {
		return Result{}, ErrDenied
	}
	signing := serviceaccess.CSRSigningRequest{CSRPEM: request.csrPEMValue(), Identity: request.scopeID().Identity(), TTL: request.ttl()}
	if _, err := signing.Validate(); err != nil {
		return Result{}, ErrDenied
	}
	bootstrap, err := a.store.Consume(ctx, request.scopeID(), request.token, request.nonce, request.ttl())
	if err != nil || bootstrap.CertificateTTL <= 0 || bootstrap.CertificateTTL > 24*time.Hour {
		return Result{}, ErrDenied
	}
	if ctx.Err() != nil {
		return Result{}, ErrDenied
	}
	signing.TTL = bootstrap.CertificateTTL
	issued, err := a.issuer.SignCSR(ctx, signing)
	if err != nil {
		return Result{}, errors.New("workstation enrollment signing failed; request a new authorization")
	}
	trust, err := a.issuer.TrustBundle(ctx)
	if err != nil {
		return Result{}, errors.New("workstation enrollment authority trust is unavailable")
	}
	if err := serviceaccess.ValidateSignedCSR(signing, issued, trust, time.Now().UTC()); err != nil {
		return Result{}, errors.New("workstation enrollment authority returned invalid certificate material")
	}
	if revoked, revErr := a.issuer.RevokedStatus(ctx, issued.Serial); revErr != nil || revoked {
		return Result{}, errors.New("workstation enrollment issued identity is revoked or its revocation status is unreadable")
	}
	return Result{
		Certificate: issued.Certificate,
		Trust:       trust,
		Serial:      issued.Serial,
		ExpiresAt:   issued.ExpiresAt,
	}, nil
}

// Renew re-signs exactly one scope's workstation key for a fresh, independently
// valid leaf. It is the renewal half of the device lifecycle: the workstation
// presents a fresh CSR for its owned key with the desired lifetime, and Renew
// re-signs only that key and returns a fresh leaf, leaving every other scope's
// leaf, every application binding and the platform untouched.
//
// Renew re-anchors the currently authoritative leaf for the scope through the
// Owner seam, refuses to renew a scope whose current identity is revoked or
// whose revocation status is unreadable, and verifies the fresh leaf against the
// trust bundle and the tri-state RevokedStatus before returning it. A canceled
// context, an invalid scope, an unreadable revocation status, an out-of-window
// TTL, a missing issued identity, or a malformed CSR all fail closed without
// issuing anything. No private key is generated or returned; only the fresh leaf
// and public trust are produced.
func (a *Authority) Renew(ctx context.Context, request Request) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, ErrDenied
	}
	scope := request.scopeID()
	if scope.Validate() != nil {
		return Result{}, ErrDenied
	}
	currentSerial, err := a.currentSerial(ctx, scope)
	if err != nil || currentSerial == "" {
		return Result{}, errors.New("workstation renewal cannot anchor the current issued identity for scope")
	}
	if revoked, revErr := a.issuer.RevokedStatus(ctx, currentSerial); revErr != nil || revoked {
		return Result{}, errors.New("workstation renewal is refused because the current identity is revoked or its revocation status is unreadable")
	}
	signing := serviceaccess.CSRSigningRequest{CSRPEM: request.csrPEMValue(), Identity: scope.Identity(), TTL: request.ttl()}
	if _, err := signing.Validate(); err != nil {
		return Result{}, ErrDenied
	}
	issued, err := a.issuer.SignCSR(ctx, signing)
	if err != nil {
		return Result{}, errors.New("workstation renewal signing failed")
	}
	trust, err := a.issuer.TrustBundle(ctx)
	if err != nil {
		return Result{}, errors.New("workstation renewal authority trust is unavailable")
	}
	if err := serviceaccess.ValidateSignedCSR(signing, issued, trust, time.Now().UTC()); err != nil {
		return Result{}, errors.New("workstation renewal authority returned invalid certificate material")
	}
	if revoked, revErr := a.issuer.RevokedStatus(ctx, issued.Serial); revErr != nil || revoked {
		return Result{}, errors.New("workstation renewal issued identity is revoked or its revocation status is unreadable")
	}
	return Result{
		Certificate: issued.Certificate,
		Trust:       trust,
		Serial:      issued.Serial,
		ExpiresAt:   issued.ExpiresAt,
	}, nil
}

// Revoke makes exactly one scope's currently issued identity invalid. It is the
// revocation half of the device lifecycle: Revoke adds scope A's current leaf
// serial to the issuer revocation list so that A's leaf now fails the
// tri-state RevokedStatus check on every subsequent verification, while every
// other scope's leaf, every application binding and the platform stay valid.
//
// A canceled context, an invalid scope, an unreadable issuer authority, or a
// missing issued identity for A all fail closed without touching any other
// identity. Revoke writes only the revocation list; it never reads or returns
// any private key.
func (a *Authority) Revoke(ctx context.Context, scope Scope) error {
	if ctx.Err() != nil || scope.Validate() != nil {
		return ErrDenied
	}
	currentSerial, err := a.currentSerial(ctx, scope)
	if err != nil {
		return errors.New("workstation revocation cannot anchor the current issued identity for scope")
	}
	if currentSerial == "" {
		return errors.New("workstation revocation found no issued identity for scope")
	}
	if err := a.issuer.Revoke(ctx, currentSerial); err != nil {
		return errors.New("workstation revocation failed to publish the revocation list")
	}
	return nil
}

// RenewAfterRevoked reports an explicit, non-silent lifecycle error: renewing a
// scope whose currently issued identity is already revoked. It reads the tri
// state RevokedStatus for the scope's current leaf and returns an error when it
// is revoked or unreadable, so a caller can never mistake a renewal of a
// revoked leaf for a clean success. A scope with no issued identity reports the
// same explicit error rather than a silent no-op.
func (a *Authority) RenewAfterRevoked(ctx context.Context, scope Scope) error {
	if ctx.Err() != nil || scope.Validate() != nil {
		return ErrDenied
	}
	currentSerial, err := a.currentSerial(ctx, scope)
	if err != nil {
		return errors.New("workstation renewal cannot anchor the current issued identity for scope")
	}
	if currentSerial == "" {
		return errors.New("workstation renewal found no issued identity for scope")
	}
	if revoked, revErr := a.issuer.RevokedStatus(ctx, currentSerial); revErr != nil || revoked {
		return errors.New("workstation renewal is refused because the current identity is revoked or its revocation status is unreadable")
	}
	return nil
}

// RevokedState reports whether the scope's currently issued leaf is revoked. It
// is the read half of the device lifecycle the portable contract needs: it reads
// the tri-state RevokedStatus for the scope's current leaf and returns the flag
// only when it is readable, so an unknown status is reported as an error rather
// than a safe "not revoked" and can never be read as accepted. A scope with no
// issued identity reports the same explicit error rather than a silent no-op.
func (a *Authority) RevokedState(ctx context.Context, scope Scope) (bool, error) {
	if ctx.Err() != nil || scope.Validate() != nil {
		return false, ErrDenied
	}
	currentSerial, err := a.currentSerial(ctx, scope)
	if err != nil {
		return false, errors.New("workstation revocation state cannot anchor the current issued identity for scope")
	}
	if currentSerial == "" {
		return false, errors.New("workstation revocation state found no issued identity for scope")
	}
	revoked, revErr := a.issuer.RevokedStatus(ctx, currentSerial)
	if revErr != nil {
		return false, errors.New("workstation revocation status is unreadable")
	}
	return revoked, nil
}

// currentSerial anchors the currently issued leaf serial for a scope through the
// Owner seam. A missing owner, owner or read failure fails closed with an empty
// serial and err != nil; ok=false means no identity was issued yet.
func (a *Authority) currentSerial(ctx context.Context, scope Scope) (string, error) {
	if a.owner == nil {
		return "", errors.New("workstation lifecycle owner is unavailable")
	}
	serial, ok, err := a.owner.CurrentSerial(ctx, scope)
	if err != nil || !ok {
		return "", err
	}
	return serial, nil
}

func randomCredential() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", errors.New("workstation enrollment entropy unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func validCredential(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == value
}
