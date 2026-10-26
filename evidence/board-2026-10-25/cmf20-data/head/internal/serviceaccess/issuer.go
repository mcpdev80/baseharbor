package serviceaccess

import (
	"context"
	"net"
	"net/url"
	"time"
)

// Issuer is the provider-neutral certificate authority boundary used by
// service access. Implementations own issuer state and lifecycle; callers only
// receive the leaf material and public trust required for the selected runtime.
type Issuer interface {
	TrustBundle(ctx context.Context) (TrustBundle, error)
	Issue(ctx context.Context, request CertificateRequest) (IssuedCertificate, error)
	Renew(ctx context.Context, current IssuedCertificate, request CertificateRequest) (IssuedCertificate, error)
	Revoke(ctx context.Context, serial string) error
	Status(ctx context.Context) (IssuerStatus, error)
}

type CertificateRequest struct {
	CommonName  string
	DNSNames    []string
	IPAddresses []net.IP
	URIs        []*url.URL
	TTL         time.Duration
}

type TrustBundle struct {
	IssuerReference string
	PEM             []byte
}

type IssuedCertificate struct {
	IssuerReference string
	Certificate     []byte
	PrivateKey      []byte
	IssuingCA       []byte
	CAChain         [][]byte
	Serial          string
	ExpiresAt       time.Time
}

type IssuerStatus struct {
	IssuerReference string
	Ready           bool
}
