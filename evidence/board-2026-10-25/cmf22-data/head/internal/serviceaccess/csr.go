package serviceaccess

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// CSRIssuer signs a client-owned key through the existing protected authority.
// It must never generate or return a client private key.
type CSRIssuer interface {
	Issuer
	SignCSR(context.Context, CSRSigningRequest) (IssuedCertificate, error)
}

// CoreCSRIssuer is the separate managed server-identity signing boundary.
// Operator-authorized Core keys cannot be signed through node enrollment.
type CoreCSRIssuer interface {
	Issuer
	SignCoreCSR(context.Context, CSRSigningRequest) (IssuedCertificate, error)
}

type CSRSigningRequest struct {
	CSRPEM   []byte
	Identity string
	TTL      time.Duration
	// DNSNames is authorized only by the internal Core server signing boundary.
	// Node enrollment must never grant server-name authority.
	DNSNames []string
}

func (r CSRSigningRequest) Validate() (*x509.CertificateRequest, error) {
	if len(r.DNSNames) != 0 {
		return nil, errors.New("node CSR cannot authorize TLS server names")
	}
	return r.validate(nil)
}

// ValidateCore binds DNS SANs to an explicit internal server authorization.
// The CSR itself is never the source of authority for additional names.
func (r CSRSigningRequest) ValidateCore() (*x509.CertificateRequest, error) {
	if !regexp.MustCompile(`^spiffe://baseharbor/platform/core/[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`).MatchString(r.Identity) || len(r.DNSNames) > 8 {
		return nil, errors.New("Core CSR identity or server-name count is invalid")
	}
	seen := map[string]bool{}
	for _, name := range r.DNSNames {
		if len(name) == 0 || len(name) > 253 || strings.ToLower(name) != name || net.ParseIP(name) != nil || seen[name] {
			return nil, errors.New("Core CSR server name must be canonical and unique")
		}
		for _, label := range strings.Split(name, ".") {
			if len(label) == 0 || len(label) > 63 || !regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`).MatchString(label) {
				return nil, errors.New("Core CSR server name is invalid")
			}
		}
		seen[name] = true
	}
	return r.validate(r.DNSNames)
}

func (r CSRSigningRequest) validate(authorizedDNS []string) (*x509.CertificateRequest, error) {
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
	if (csr.Subject.CommonName != "" && csr.Subject.CommonName != r.Identity) || len(csr.URIs) != 1 || csr.URIs[0].String() != r.Identity || !slices.Equal(csr.DNSNames, authorizedDNS) || len(csr.IPAddresses) != 0 || len(csr.EmailAddresses) != 0 {
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
