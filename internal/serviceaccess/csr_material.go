package serviceaccess

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"time"
)

// ValidateSignedCSR verifies public issuance against the authoritative trust,
// rather than trusting a CA supplied alongside an arbitrary signed leaf.
func ValidateSignedCSR(request CSRSigningRequest, issued IssuedCertificate, trust TrustBundle, now time.Time) error {
	csr, err := request.Validate()
	if err != nil {
		return err
	}
	invalid := errors.New("issued CSR material does not match the authorized client key, scope, or authority")
	block, rest := pem.Decode(issued.Certificate)
	if !strings.HasPrefix(strings.TrimSpace(string(issued.Certificate)), "-----BEGIN CERTIFICATE-----") || block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 || strings.TrimSpace(string(rest)) != "" || len(issued.PrivateKey) != 0 {
		return invalid
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil || leaf.IsCA || leaf.Subject.CommonName != request.Identity || len(leaf.URIs) != 1 || leaf.URIs[0].String() != request.Identity || len(leaf.DNSNames) != 0 || len(leaf.IPAddresses) != 0 || len(leaf.EmailAddresses) != 0 || len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth || len(leaf.UnknownExtKeyUsage) != 0 || leaf.KeyUsage != x509.KeyUsageDigitalSignature || !leaf.NotAfter.After(now) || leaf.NotAfter.After(now.Add(request.TTL+time.Minute)) || !issued.ExpiresAt.Equal(leaf.NotAfter) {
		return invalid
	}
	key, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err != nil || !bytes.Equal(key, csr.RawSubjectPublicKeyInfo) {
		return invalid
	}
	serial := strings.ReplaceAll(strings.ToLower(issued.Serial), ":", "")
	if serial == "" || strings.TrimLeft(serial, "0") != strings.TrimLeft(leaf.SerialNumber.Text(16), "0") {
		return invalid
	}
	roots, err := certificatePool(trust.PEM)
	if err != nil {
		return invalid
	}
	intermediates := x509.NewCertPool()
	for _, material := range append(append([][]byte(nil), issued.CAChain...), issued.IssuingCA) {
		for len(bytes.TrimSpace(material)) > 0 {
			part, trailing := pem.Decode(material)
			if !strings.HasPrefix(strings.TrimSpace(string(material)), "-----BEGIN CERTIFICATE-----") || part == nil || part.Type != "CERTIFICATE" || len(part.Headers) != 0 {
				return invalid
			}
			cert, err := x509.ParseCertificate(part.Bytes)
			if err != nil || !cert.IsCA {
				return invalid
			}
			intermediates.AddCert(cert)
			material = trailing
		}
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return invalid
	}
	return nil
}

func certificatePool(material []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	count := 0
	for len(bytes.TrimSpace(material)) > 0 {
		block, rest := pem.Decode(material)
		if !strings.HasPrefix(strings.TrimSpace(string(material)), "-----BEGIN CERTIFICATE-----") || block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("invalid authority trust bundle")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !cert.IsCA {
			return nil, errors.New("invalid authority trust certificate")
		}
		pool.AddCert(cert)
		count++
		material = rest
	}
	if count == 0 {
		return nil, errors.New("authority trust bundle is empty")
	}
	return pool, nil
}
