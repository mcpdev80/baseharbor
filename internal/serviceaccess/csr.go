package serviceaccess

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/url"
	"strings"
	"time"
)

// CSRIssuer signs a client-owned key through the existing protected authority.
// It must never generate or return a client private key.
type CSRIssuer interface {
	Issuer
	SignCSR(context.Context, CSRSigningRequest) (IssuedCertificate, error)
}

type CSRSigningRequest struct {
	CSRPEM   []byte
	Identity string
	TTL      time.Duration
}

func (r CSRSigningRequest) Validate() (*x509.CertificateRequest, error) {
	identity, err := url.Parse(r.Identity)
	if err != nil || identity.Scheme != "spiffe" || identity.Host == "" || identity.User != nil || identity.RawQuery != "" || identity.Fragment != "" || identity.Path == "" || identity.Opaque != "" {
		return nil, errors.New("CSR signing requires one scoped URI identity")
	}
	if r.TTL <= 0 || r.TTL > 24*time.Hour || len(r.CSRPEM) > 65536 {
		return nil, errors.New("CSR signing TTL or size is outside the supported bounds")
	}
	block, rest := pem.Decode(r.CSRPEM)
	if !strings.HasPrefix(strings.TrimSpace(string(r.CSRPEM)), "-----BEGIN CERTIFICATE REQUEST-----") || block == nil || block.Type != "CERTIFICATE REQUEST" || len(block.Headers) != 0 || strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("CSR must contain exactly one PKCS#10 request")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil {
		return nil, errors.New("CSR signature is invalid")
	}
	if (csr.Subject.CommonName != "" && csr.Subject.CommonName != r.Identity) || len(csr.URIs) != 1 || csr.URIs[0].String() != r.Identity || len(csr.DNSNames) != 0 || len(csr.IPAddresses) != 0 || len(csr.EmailAddresses) != 0 {
		return nil, errors.New("CSR identity differs from the authorized node identity")
	}
	for _, extension := range csr.Extensions {
		if !extension.Id.Equal([]int{2, 5, 29, 17}) {
			return nil, errors.New("CSR contains an unapproved extension")
		}
	}
	switch key := csr.PublicKey.(type) {
	case ed25519.PublicKey:
		if len(key) != ed25519.PublicKeySize {
			return nil, errors.New("CSR key is invalid")
		}
	case *ecdsa.PublicKey:
		if key.Curve.Params().BitSize < 256 {
			return nil, errors.New("CSR key strength is insufficient")
		}
	case *rsa.PublicKey:
		if key.N.BitLen() < 2048 || key.N.BitLen() > 4096 {
			return nil, errors.New("CSR RSA key strength is outside the supported bounds")
		}
	default:
		return nil, errors.New("CSR public key type is unsupported")
	}
	return csr, nil
}
