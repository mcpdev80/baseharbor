package workstationenrollment

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

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
type Authority struct {
	store  Store
	issuer serviceaccess.CSRIssuer
}

// New returns an Authority or an error if the store or issuer is nil.
func New(store Store, issuer serviceaccess.CSRIssuer) (*Authority, error) {
	if store == nil || issuer == nil {
		return nil, errors.New("workstation enrollment requires a one-use store and an existing issuer")
	}
	return &Authority{store: store, issuer: issuer}, nil
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
