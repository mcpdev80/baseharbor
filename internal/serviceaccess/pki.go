package serviceaccess

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TLSMaterial struct {
	Source            PKISource `json:"source"`
	CA                string    `json:"ca"`
	ServerCertificate string    `json:"server_certificate"`
	ServerKey         string    `json:"server_key"`
	ClientCertificate string    `json:"client_certificate,omitempty"`
	ClientKey         string    `json:"client_key,omitempty"`
	ServerName        string    `json:"server_name"`
}

func EnsureTLSMaterial(policy Policy, dir string, dnsNames ...string) (TLSMaterial, error) {
	if !policy.TLSRequired {
		return TLSMaterial{}, errors.New("BaseHarbor managed service access cannot disable TLS")
	}
	if policy.PKISource == PKIExternal || policy.PKISource == PKIBYOC {
		return externalTLSMaterial(policy)
	}
	if policy.PKISource != PKIManagedLocal {
		return TLSMaterial{}, fmt.Errorf("unsupported PKI source %q", policy.PKISource)
	}
	return ensureManagedLocal(policy, dir, dnsNames)
}

func ExistingTLSMaterial(policy Policy, dir string) (TLSMaterial, error) {
	if policy.PKISource == PKIExternal || policy.PKISource == PKIBYOC {
		return externalTLSMaterial(policy)
	}
	if policy.PKISource != PKIManagedLocal {
		return TLSMaterial{}, fmt.Errorf("unsupported PKI source %q", policy.PKISource)
	}
	material := TLSMaterial{
		Source: PKIManagedLocal,
		CA: filepath.Join(dir, "ca.pem"),
		ServerCertificate: filepath.Join(dir, "server-cert.pem"),
		ServerKey: filepath.Join(dir, "server-key.pem"),
		ClientCertificate: filepath.Join(dir, "client-cert.pem"),
		ClientKey: filepath.Join(dir, "client-key.pem"),
		ServerName: policy.ServerName,
	}
	for _, path := range []string{material.CA, material.ServerCertificate, material.ServerKey, material.ClientCertificate, material.ClientKey} {
		if _, err := os.Stat(path); err != nil {
			return TLSMaterial{}, err
		}
	}
	if err := validateMaterial(material, true); err != nil {
		return TLSMaterial{}, err
	}
	return material, nil
}

func externalTLSMaterial(policy Policy) (TLSMaterial, error) {
	if policy.ServerCertificate == "" || policy.ServerKey == "" || policy.TrustBundle == "" {
		return TLSMaterial{}, errors.New("external TLS material is incomplete")
	}
	material := TLSMaterial{
		Source: policy.PKISource,
		CA: policy.TrustBundle,
		ServerCertificate: policy.ServerCertificate,
		ServerKey: policy.ServerKey,
		ClientCertificate: policy.ClientCertificate,
		ClientKey: policy.ClientKey,
		ServerName: policy.ServerName,
	}
	if err := validateMaterial(material, policy.AuthenticationRequired && policy.Authentication == AuthenticationMTLS); err != nil {
		return TLSMaterial{}, err
	}
	return material, nil
}

func ensureManagedLocal(policy Policy, dir string, dnsNames []string) (TLSMaterial, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return TLSMaterial{}, fmt.Errorf("create managed service PKI directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return TLSMaterial{}, err
	}
	material := TLSMaterial{
		Source: PKIManagedLocal,
		CA: filepath.Join(dir, "ca.pem"),
		ServerCertificate: filepath.Join(dir, "server-cert.pem"),
		ServerKey: filepath.Join(dir, "server-key.pem"),
		ClientCertificate: filepath.Join(dir, "client-cert.pem"),
		ClientKey: filepath.Join(dir, "client-key.pem"),
		ServerName: policy.ServerName,
	}
	if valid, err := managedMaterialValid(material, dnsNames); err != nil {
		return TLSMaterial{}, err
	} else if valid {
		return material, nil
	}

	caCert, caKey, err := generateCA()
	if err != nil {
		return TLSMaterial{}, err
	}
	names := uniqueNames(append([]string{policy.ServerName, "localhost"}, dnsNames...))
	serverCert, serverKey, err := issueCertificate(caCert, caKey, names, false)
	if err != nil {
		return TLSMaterial{}, err
	}
	clientCert, clientKey, err := issueCertificate(caCert, caKey, nil, true)
	if err != nil {
		return TLSMaterial{}, err
	}
	files := map[string][]byte{
		material.CA: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}),
		material.ServerCertificate: serverCert,
		material.ServerKey: serverKey,
		material.ClientCertificate: clientCert,
		material.ClientKey: clientKey,
	}
	for path, data := range files {
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, "-key.pem") {
			mode = 0o600
		}
		if err := writeAtomic(path, data, mode); err != nil {
			return TLSMaterial{}, err
		}
	}
	return material, nil
}

func managedMaterialValid(material TLSMaterial, dnsNames []string) (bool, error) {
	for _, path := range []string{material.CA, material.ServerCertificate, material.ServerKey, material.ClientCertificate, material.ClientKey} {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return false, nil
		} else if err != nil {
			return false, err
		}
	}
	if err := validateMaterial(material, true); err != nil {
		return false, nil
	}
	ca, err := readCertificate(material.CA)
	if err != nil {
		return false, nil
	}
	server, err := readCertificate(material.ServerCertificate)
	if err != nil {
		return false, nil
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	for _, name := range uniqueNames(append([]string{material.ServerName, "localhost"}, dnsNames...)) {
		if name == "" {
			continue
		}
		if _, err := server.Verify(x509.VerifyOptions{Roots: roots, DNSName: name, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			return false, nil
		}
	}
	return true, nil
}

func validateMaterial(material TLSMaterial, requireClient bool) error {
	ca, err := readCertificate(material.CA)
	if err != nil {
		return fmt.Errorf("read service TLS trust bundle: %w", err)
	}
	if !ca.IsCA && material.Source == PKIManagedLocal {
		return errors.New("managed-local service trust certificate is not a CA")
	}
	server, err := readCertificate(material.ServerCertificate)
	if err != nil {
		return fmt.Errorf("read service TLS server certificate: %w", err)
	}
	if time.Now().After(server.NotAfter) {
		return errors.New("service TLS server certificate is expired")
	}
	if err := keyMatches(material.ServerKey, server); err != nil {
		return fmt.Errorf("service TLS server key: %w", err)
	}
	if requireClient {
		client, err := readCertificate(material.ClientCertificate)
		if err != nil {
			return fmt.Errorf("read service TLS client certificate: %w", err)
		}
		if time.Now().After(client.NotAfter) {
			return errors.New("service TLS client certificate is expired")
		}
		if err := keyMatches(material.ClientKey, client); err != nil {
			return fmt.Errorf("service TLS client key: %w", err)
		}
	}
	return nil
}

func generateCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := serialNumber()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{CommonName: "BaseHarbor Managed Service CA", Organization: []string{"BaseHarbor"}},
		NotBefore: now.Add(-5 * time.Minute),
		NotAfter: now.AddDate(2, 0, 0),
		IsCA: true,
		BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	template.Raw = der
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func issueCertificate(ca *x509.Certificate, caKey *ecdsa.PrivateKey, dnsNames []string, client bool) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := serialNumber()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{CommonName: "BaseHarbor Managed Service", Organization: []string{"BaseHarbor"}},
		NotBefore: now.Add(-5 * time.Minute),
		NotAfter: now.Add(30 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	if client {
		template.Subject.CommonName = "baseharbor-service-client"
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.DNSNames = dnsNames
		for _, name := range dnsNames {
			if ip := net.ParseIP(name); ip != nil {
				template.IPAddresses = append(template.IPAddresses, ip)
			}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), nil
}

func readCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("certificate PEM is invalid")
	}
	return x509.ParseCertificate(block.Bytes)
}

func keyMatches(path string, cert *x509.Certificate) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		return errors.New("private key PEM is invalid")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || !key.PublicKey.Equal(cert.PublicKey) {
		return errors.New("certificate and private key do not match")
	}
	return nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func serialNumber() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func uniqueNames(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func sameCertificate(a, b *x509.Certificate) bool {
	return a != nil && b != nil && bytes.Equal(a.Raw, b.Raw)
}
