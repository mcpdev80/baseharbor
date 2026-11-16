package targetenrollment

import (
	"context"
	"crypto/x509"
	"time"
)

// CurrentNodeTrust is consulted for active sessions as well as new admission.
// Handshake-time VerifiedChains alone cannot detect removal of a former CA.
type CurrentNodeTrust interface {
	NodeTrust(context.Context) (*x509.CertPool, error)
}

type liveTrustRegistry struct {
	NodeRegistry
	load func(context.Context) (*x509.CertPool, error)
}

// WithLiveTrust keeps persisted certificate admission and current CA trust as
// independent requirements. Neither a trusted CA nor an active row is enough.
func WithLiveTrust(registry NodeRegistry, load func(context.Context) (*x509.CertPool, error)) (NodeRegistry, error) {
	if registry == nil || load == nil {
		return nil, ErrDenied
	}
	return &liveTrustRegistry{NodeRegistry: registry, load: load}, nil
}

func (r *liveTrustRegistry) NodeTrust(ctx context.Context) (*x509.CertPool, error) {
	if ctx.Err() != nil {
		return nil, ErrDenied
	}
	return r.load(ctx)
}

func verifyCurrentNodeTrust(ctx context.Context, registry NodeRegistry, peer []*x509.Certificate, now time.Time) error {
	current, ok := registry.(CurrentNodeTrust)
	if !ok {
		return nil // Other callers still require their verified TLS chain and registry.
	}
	roots, err := current.NodeTrust(ctx)
	if err != nil || roots == nil || ctx.Err() != nil || len(peer) == 0 {
		return ErrDenied
	}
	intermediates := x509.NewCertPool()
	for _, certificate := range peer[1:] {
		if certificate == nil {
			return ErrDenied
		}
		intermediates.AddCert(certificate)
	}
	_, err = peer[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates,
		CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	if err != nil {
		return ErrDenied
	}
	return nil
}
