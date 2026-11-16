package workstationenrollment

import (
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

// Request is the workstation-owned enrollment request. It carries the
// installation-scoped scope, the one-use bootstrap credentials, the workstation
// key signing request (CSR), and the requested certificate TTL. No private key
// is carried in the request; the signer is the workstation-owned key.
type Request struct {
	requestScope   Scope
	token          string
	nonce          string
	csrPEM         []byte
	certificateTTL time.Duration
}

// NewRequest builds an enrollment request. scope must be valid; token and nonce
// are the digested bootstrap credentials returned by Authority.Create.
func NewRequest(scope Scope, token, nonce string, csrPEM []byte, certificateTTL time.Duration) (Request, error) {
	if scope.Validate() != nil {
		return Request{}, ErrDenied
	}
	return Request{
		requestScope:   scope,
		token:          token,
		nonce:          nonce,
		csrPEM:         csrPEM,
		certificateTTL: certificateTTL,
	}, nil
}

// NewRenewRequest builds a device-lifecycle renewal request. scope must be
// valid; csrPEM is the workstation-owned key signing request (the same key the
// workstation enrolled with, or a freshly generated one for the same scope) and
// certificateTTL is the desired fresh leaf lifetime. No bootstrap credential is
// required: renewal is gated by the current identity's revocation status, not by
// a one-use bootstrap.
func NewRenewRequest(scope Scope, csrPEM []byte, certificateTTL time.Duration) (Request, error) {
	if scope.Validate() != nil {
		return Request{}, ErrDenied
	}
	if len(csrPEM) == 0 {
		return Request{}, ErrDenied
	}
	if certificateTTL <= 0 || certificateTTL > MaxCertificateTTL {
		return Request{}, ErrDenied
	}
	return Request{
		requestScope:   scope,
		token:          "",
		nonce:          "",
		csrPEM:         csrPEM,
		certificateTTL: certificateTTL,
	}, nil
}

// scopeID returns the request scope.
func (r Request) scopeID() Scope { return r.requestScope }

// Scope returns the request scope. It is the public accessor the device-lifecycle
// contract and HTTP adapters use to read the scope a validated request carries.
func (r Request) Scope() Scope { return r.requestScope }

// ttl returns the requested certificate TTL.
func (r Request) ttl() time.Duration { return r.certificateTTL }

// csrPEMValue returns the workstation CSR bytes.
func (r Request) csrPEMValue() []byte { return r.csrPEM }

// Result is the outcome of a validated one-use workstation enrollment: the
// signed leaf, the authoritative trust bundle, the issued serial, and the leaf
// expiry as reported by the issuer.
type Result struct {
	Certificate []byte
	Trust       serviceaccess.TrustBundle
	Serial      string
	ExpiresAt   time.Time
	OwnerLogin  string
}
