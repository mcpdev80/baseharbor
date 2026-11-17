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
	// Revoked is the coarse read-only revocation boundary used by verification:
	// it reports whether the observed certificate serial is revoked without
	// changing any state. A missing, foreign, unparseable or unreadable
	// revocation status fails closed by returning false.
	Revoked(ctx context.Context, serial string) bool
	// RevokedStatus is the tri-state read-only revocation boundary used by the
	// fail-closed production path: it returns (revoked bool, err error) where
	// a missing, foreign, unparseable or unreadable revocation status reports
	// err != nil. The boundary rejects on err != nil OR revoked == true, so
	// "unknown" can never be accepted as safe.
	RevokedStatus(ctx context.Context, serial string) (revoked bool, err error)
	// BaseSerial is the read-only identity of the management connection itself:
	// the base (CA) certificate serial that issued the observed leaf. The
	// production selection boundary observes the existing connection through
	// this serial, never by issuing a new one.
	BaseSerial(ctx context.Context) (string, error)
	// BaseExpiry is the read-only freshness bound of the management connection:
	// the base (CA) certificate not-after. The production selection boundary
	// uses it as the freshness bound instead of issuing a fresh leaf.
	BaseExpiry(ctx context.Context) (time.Time, error)
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
	// Reference is the issuer identity (CA serial) that issued this leaf; it
	// carries the real CA serial so RevokedStatus can compare against the CRL at
	// the serial level. IssuerNotAfter is the issuing CA certificate NotAfter.
	Reference      string
	IssuerNotAfter time.Time
}

type IssuerStatus struct {
	IssuerReference string
	Ready           bool
}
