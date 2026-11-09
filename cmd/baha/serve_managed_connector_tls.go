package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type managedCoreConnectorMaterial struct {
	Certificate string
	Key         string
	CA          string
}

// issueManagedCoreConnectorTLS signs a Core-owned CSR using the dedicated
// serverAuth-only OpenBao role. The locally generated key never enters OpenBao.
// The installed Core UUID is the stable authority identity, not a requested
// or user-supplied SPIFFE name.
func issueManagedCoreConnectorTLS(ctx context.Context, issuer serviceaccess.CoreCSRIssuer, root, installationID, host string) (managedCoreConnectorMaterial, error) {
	var result managedCoreConnectorMaterial
	if net.ParseIP(host) != nil {
		return result, errors.New("managed Core Connector CSR requires a DNS hostname; the server-name signing contract does not authorize IP SANs")
	}
	identity := "spiffe://baseharbor/platform/core/" + installationID
	uri, err := url.Parse(identity)
	if err != nil {
		return result, err
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return result, err
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: identity},
		URIs:     []*url.URL{uri},
		DNSNames: []string{host},
	}, key)
	if err != nil {
		return result, fmt.Errorf("create Core Connector certificate request: %w", err)
	}
	request := serviceaccess.CSRSigningRequest{
		Identity: identity,
		CSRPEM:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}),
		DNSNames: []string{host},
		TTL:      24 * time.Hour,
	}
	if _, err := request.ValidateCore(); err != nil {
		return result, fmt.Errorf("verify Core Connector CSR identity: %w", err)
	}
	signed, err := issuer.SignCoreCSR(ctx, request)
	if err != nil {
		return result, err
	}
	if len(signed.PrivateKey) != 0 {
		return result, errors.New("Core Connector signer unexpectedly returned a private key")
	}
	trust, err := issuer.TrustBundle(ctx)
	if err != nil {
		return result, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(trust.PEM) {
		return result, errors.New("Core Connector signer trust bundle is invalid")
	}
	block, remainder := pem.Decode(signed.Certificate)
	if block == nil || block.Type != "CERTIFICATE" || len(remainder) != 0 {
		return result, errors.New("Core Connector signer returned invalid certificate")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil || leaf.IsCA || leaf.Subject.CommonName != identity || len(leaf.URIs) != 1 ||
		leaf.URIs[0].String() != identity || len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != host ||
		len(leaf.IPAddresses) != 0 || len(leaf.ExtKeyUsage) != 1 ||
		leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth || len(leaf.UnknownExtKeyUsage) != 0 ||
		leaf.NotBefore.After(time.Now()) || !leaf.NotAfter.After(time.Now()) {
		return result, errors.New("Core Connector signed certificate violates SPIFFE/SAN/serverAuth identity")
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots: roots, DNSName: host,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		return result, fmt.Errorf("verify Core Connector certificate against owned issuer: %w", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return result, err
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	if _, err := tls.X509KeyPair(signed.Certificate, privatePEM); err != nil {
		return result, fmt.Errorf("Core Connector certificate is not bound to its local key: %w", err)
	}
	directory := filepath.Join(root, "managed-serving", "core-connector")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return result, err
	}
	result.Certificate = filepath.Join(directory, "server.pem")
	result.Key = filepath.Join(directory, "server-key.pem")
	result.CA = filepath.Join(directory, "ca.pem")
	// Never publish a certificate/keypair until the full signed certificate,
	// private-key match and authoritative trust chain have been verified.
	for _, file := range []struct {
		path string
		data []byte
		mode os.FileMode
	}{
		{result.Key, privatePEM, 0o600},
		{result.Certificate, signed.Certificate, 0o600},
		{result.CA, trust.PEM, 0o600},
	} {
		if err := replaceManagedCoreTLSFile(file.path, file.data, file.mode); err != nil {
			return managedCoreConnectorMaterial{}, err
		}
	}
	return result, nil
}

func replaceManagedCoreTLSFile(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".core-serving-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
