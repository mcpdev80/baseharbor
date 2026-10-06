// Package targetenrollment binds one-use enrollment to an existing Core issuer.
package targetenrollment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

var ErrDenied = errors.New("enrollment authorization is invalid, expired, or consumed")
var tenantID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var segment = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// Scope is selected and authorized by Core before creating a bootstrap grant.
// The Connector cannot choose another tenant, Target, runtime, or identity.
type Scope struct {
	TenantID string
	TargetID string
	NodeID   string
	Runtime  string
}

func (s Scope) Identity() string {
	return "spiffe://baseharbor/platform/connectors/" + s.TenantID + "/" + s.TargetID + "/" + s.NodeID
}

func (s Scope) Validate() error {
	if !tenantID.MatchString(s.TenantID) || !segment.MatchString(s.TargetID) || !segment.MatchString(s.NodeID) || (s.Runtime != "docker" && s.Runtime != "podman") {
		return ErrDenied
	}
	return nil
}

// Grant contains only digests, never the bootstrap bearer credential or nonce.
type Grant struct {
	Scope          Scope
	TokenDigest    string
	NonceDigest    string
	ExpiresAt      time.Time
	CertificateTTL time.Duration
}

// Store must persist consumption before returning success, including under
// concurrent requests and process restarts. Signing failures do not unconsume.
type Store interface {
	Create(context.Context, Grant) error
	Consume(context.Context, Scope, string, string, string, time.Time) (Grant, error)
	RecordIssued(context.Context, Scope, string, string, time.Time) error
}

type Authority struct {
	store  Store
	issuer serviceaccess.CSRIssuer
}

func New(store Store, issuer serviceaccess.CSRIssuer) (*Authority, error) {
	if store == nil || issuer == nil {
		return nil, errors.New("enrollment requires persistent grants and an existing Core issuer")
	}
	return &Authority{store: store, issuer: issuer}, nil
}

type Bootstrap struct {
	Token     string    `json:"token"`
	Nonce     string    `json:"nonce"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (a *Authority) Create(ctx context.Context, scope Scope, lifetime, certificateTTL time.Duration) (Bootstrap, error) {
	if ctx.Err() != nil || scope.Validate() != nil || lifetime <= 0 || lifetime > 10*time.Minute || certificateTTL < time.Second || certificateTTL > 24*time.Hour || certificateTTL%time.Second != 0 {
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
	expires := time.Now().UTC().Add(lifetime)
	grant := Grant{Scope: scope, TokenDigest: digest(token), NonceDigest: digest(nonce), ExpiresAt: expires, CertificateTTL: certificateTTL}
	if err := a.store.Create(ctx, grant); err != nil {
		return Bootstrap{}, errors.New("cannot persist enrollment authorization")
	}
	return Bootstrap{Token: token, Nonce: nonce, ExpiresAt: expires}, nil
}

type Request struct {
	Scope  Scope
	Token  string
	Nonce  string
	CSRPEM []byte
}

type Result struct {
	Certificate serviceaccess.IssuedCertificate
	Trust       serviceaccess.TrustBundle
}

func (a *Authority) Enroll(ctx context.Context, request Request) (Result, error) {
	if ctx.Err() != nil || request.Scope.Validate() != nil || !validCredential(request.Token) || !validCredential(request.Nonce) {
		return Result{}, ErrDenied
	}
	signing := serviceaccess.CSRSigningRequest{CSRPEM: request.CSRPEM, Identity: request.Scope.Identity(), TTL: time.Hour}
	csr, err := signing.Validate()
	if err != nil {
		return Result{}, ErrDenied
	}
	csrHash := sha256.Sum256(csr.Raw)
	grant, err := a.store.Consume(ctx, request.Scope, digest(request.Token), digest(request.Nonce), hex.EncodeToString(csrHash[:]), time.Now().UTC())
	if err != nil || grant.Scope != request.Scope || grant.CertificateTTL <= 0 || grant.CertificateTTL > 24*time.Hour {
		return Result{}, ErrDenied
	}
	if ctx.Err() != nil {
		return Result{}, ErrDenied
	}
	signing.TTL = grant.CertificateTTL
	issued, err := a.issuer.SignCSR(ctx, signing)
	if err != nil {
		return Result{}, errors.New("enrollment signing failed; request a new authorization")
	}
	trust, err := a.issuer.TrustBundle(ctx)
	if err != nil {
		return Result{}, errors.New("enrollment authority trust is unavailable")
	}
	if err := serviceaccess.ValidateSignedCSR(signing, issued, trust, time.Now()); err != nil {
		return Result{}, errors.New("enrollment authority returned invalid certificate material")
	}
	if err := a.store.RecordIssued(ctx, request.Scope, grant.TokenDigest, issued.Serial, issued.ExpiresAt); err != nil {
		revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = a.issuer.Revoke(revokeCtx, issued.Serial)
		return Result{}, errors.New("enrollment identity persistence failed; certificate admission is unavailable")
	}
	return Result{Certificate: issued, Trust: trust}, nil
}

func randomCredential() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", errors.New("enrollment entropy unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func validCredential(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == value
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
