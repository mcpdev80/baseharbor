// Package workstationenrollmenthttp exposes the one-use workstation-enrollment
// boundary over HTTP at the management boundary. It is the production-shape
// caller: it receives a workstation bootstrap request, binds the recipient and
// scope, performs the single-use enrollment through the existing trusted issuer,
// and returns the signed leaf plus the registry fact (scope + time).
//
// It carries no secrets and never touches node/connector enrollment. Shared
// register or node files are not edited here.
package workstationenrollmenthttp

import (
	"context"
	"time"

	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// Enrollment is the one-use workstation-enrollment boundary.
type Enrollment interface {
	Create(ctx context.Context, scope ws.Scope, lifetime, certificateTTL time.Duration) (ws.Bootstrap, error)
	Enroll(ctx context.Context, request ws.Request) (ws.Result, error)
}

// BootstrapRequest is the workstation-owned HTTP request body.
type BootstrapRequest struct {
	InstallationID        string `json:"installation_id"`
	Station               string `json:"station"`
	CertificateTTLSeconds int    `json:"certificate_ttl_seconds"`
}

// BootstrapResponse is the HTTP response for a bootstrap creation. Token and
// Nonce are the one-use bootstrap credentials the workstation returns at enroll
// time. The store keeps only their digests; this response carries the bare
// values so the workstation can present exactly them.
type BootstrapResponse struct {
	BootstrapID           string    `json:"bootstrap_id"`
	Token                 string    `json:"token"`
	Nonce                 string    `json:"nonce"`
	ExpiresAt             time.Time `json:"expires_at"`
	CertificateTTLSeconds int       `json:"certificate_ttl_seconds"`
}

// EnrollRequest is the HTTP body for a one-use enrollment. Token and Nonce are
// the bootstrap credentials; CSRPEM is the workstation-owned key signing
// request.
type EnrollRequest struct {
	InstallationID string `json:"installation_id"`
	Station        string `json:"station"`
	Token          string `json:"token"`
	Nonce          string `json:"nonce"`
	CSRPEM         string `json:"csr_pem"`
}

// EnrollResponse is the HTTP response for a completed one-use enrollment. It
// carries the signed leaf and the registry fact (scope + time). No private key
// is returned.
type EnrollResponse struct {
	InstallationID string    `json:"installation_id"`
	Station        string    `json:"station"`
	Serial         string    `json:"serial"`
	ExpiresAt      time.Time `json:"expires_at"`
	LeafPEM        string    `json:"leaf_pem"`
	TrustPEM       string    `json:"trust_pem"`
	At             time.Time `json:"at"`
}
