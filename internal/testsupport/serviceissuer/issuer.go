package serviceissuer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type Issuer struct {
	ca    *x509.Certificate
	caKey *ecdsa.PrivateKey
}

func New(t testing.TB) *Issuer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "BaseHarbor Test CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &Issuer{ca: ca, caKey: key}
}

func (i *Issuer) TrustBundle(context.Context) (serviceaccess.TrustBundle, error) {
	return serviceaccess.TrustBundle{
		IssuerReference: "test://baseharbor",
		PEM:             pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: i.ca.Raw}),
	}, nil
}

func (i *Issuer) Issue(_ context.Context, request serviceaccess.CertificateRequest) (serviceaccess.IssuedCertificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return serviceaccess.IssuedCertificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return serviceaccess.IssuedCertificate{}, err
	}
	ttl := request.TTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: request.CommonName},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(ttl),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		DNSNames:     append([]string(nil), request.DNSNames...),
		IPAddresses:  append([]net.IP(nil), request.IPAddresses...),
		URIs:         request.URIs,
	}
	if len(request.DNSNames) > 0 || len(request.IPAddresses) > 0 {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, i.ca, &key.PublicKey, i.caKey)
	if err != nil {
		return serviceaccess.IssuedCertificate{}, err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return serviceaccess.IssuedCertificate{}, err
	}
	return serviceaccess.IssuedCertificate{
		IssuerReference: "test://baseharbor",
		Certificate:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		PrivateKey:      pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}),
		IssuingCA:       pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: i.ca.Raw}),
		Serial:          serial.Text(16),
		ExpiresAt:       template.NotAfter,
	}, nil
}

func (i *Issuer) Renew(ctx context.Context, _ serviceaccess.IssuedCertificate, request serviceaccess.CertificateRequest) (serviceaccess.IssuedCertificate, error) {
	return i.Issue(ctx, request)
}

func (i *Issuer) Revoke(context.Context, string) error { return nil }

func (i *Issuer) Status(context.Context) (serviceaccess.IssuerStatus, error) {
	return serviceaccess.IssuerStatus{IssuerReference: "test://baseharbor", Ready: true}, nil
}

var _ serviceaccess.Issuer = (*Issuer)(nil)
