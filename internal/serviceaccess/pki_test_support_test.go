package serviceaccess

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
	"time"
)

type testIssuer struct {
	ca    *x509.Certificate
	caKey *ecdsa.PrivateKey
}

func newTestIssuer(t interface{ Fatal(...any) }) *testIssuer {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
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
	return &testIssuer{ca: ca, caKey: key}
}

func (i *testIssuer) TrustBundle(context.Context) (TrustBundle, error) {
	return TrustBundle{
		IssuerReference: "test://issuer",
		PEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: i.ca.Raw}),
	}, nil
}

func (i *testIssuer) Issue(_ context.Context, request CertificateRequest) (IssuedCertificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return IssuedCertificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return IssuedCertificate{}, err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: request.CommonName},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(request.TTL),
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
		return IssuedCertificate{}, err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return IssuedCertificate{}, err
	}
	return IssuedCertificate{
		IssuerReference: "test://issuer",
		Certificate:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		PrivateKey:      pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}),
		IssuingCA:       pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: i.ca.Raw}),
		Serial:          serial.Text(16),
		ExpiresAt:       template.NotAfter,
	}, nil
}

func (i *testIssuer) Renew(ctx context.Context, _ IssuedCertificate, request CertificateRequest) (IssuedCertificate, error) {
	return i.Issue(ctx, request)
}

func (i *testIssuer) Revoke(context.Context, string) error { return nil }

func (i *testIssuer) Status(context.Context) (IssuerStatus, error) {
	return IssuerStatus{IssuerReference: "test://issuer", Ready: true}, nil
}
