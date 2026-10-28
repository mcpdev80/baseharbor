package dcspki

import (
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
	"time"
)

const (
	caLifetime   = 5 * 365 * 24 * time.Hour
	leafLifetime = 90 * 24 * time.Hour
	renewBefore  = 14 * 24 * time.Hour
	caGuard      = 90 * 24 * time.Hour
)

type Files struct {
	CA         string
	CAKey      string
	ServerCert string
	ServerKey  string
	ClientCert string
	ClientKey  string
}

func paths(dir string) Files {
	return Files{
		CA: filepath.Join(dir, "ca.pem"), CAKey: filepath.Join(dir, "ca-key.pem"),
		ServerCert: filepath.Join(dir, "server.pem"), ServerKey: filepath.Join(dir, "server-key.pem"),
		ClientCert: filepath.Join(dir, "client.pem"), ClientKey: filepath.Join(dir, "client-key.pem"),
	}
}

// Ensure creates and maintains the BaseHarbor-owned etcd PKI. Leaf certificates
// are rotated under the existing CA before expiry. CA rotation is deliberately
// fail-closed because changing etcd trust roots requires a fenced cluster
// migration and must never happen as a side effect of ordinary reconciliation.
func Ensure(dir string, serverNames []string) (Files, error) {
	if len(serverNames) == 0 {
		return Files{}, errors.New("etcd PKI requires at least one server identity")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Files{}, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return Files{}, err
	}
	f := paths(dir)
	all := []string{f.CA, f.CAKey, f.ServerCert, f.ServerKey, f.ClientCert, f.ClientKey}
	existing := 0
	for _, path := range all {
		st, err := os.Lstat(path)
		if err == nil {
			if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 {
				return Files{}, fmt.Errorf("unsafe etcd PKI path %s", filepath.Base(path))
			}
			existing++
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Files{}, err
		}
	}
	if existing != 0 && existing != len(all) {
		return Files{}, errors.New("partial etcd PKI state requires operator recovery")
	}
	if existing == 0 {
		if err := createAll(f, serverNames); err != nil {
			return Files{}, err
		}
		return f, nil
	}
	ca, caKey, err := loadCA(f)
	if err != nil {
		return Files{}, err
	}
	now := time.Now().UTC()
	if ca.NotAfter.Before(now.Add(caGuard)) {
		return Files{}, errors.New("etcd CA rotation requires explicit fenced DCS migration")
	}
	server, err := loadCertificate(f.ServerCert)
	if err != nil {
		return Files{}, err
	}
	client, err := loadCertificate(f.ClientCert)
	if err != nil {
		return Files{}, err
	}
	if server.NotAfter.Before(now.Add(renewBefore)) || client.NotAfter.Before(now.Add(renewBefore)) {
		if err := issueLeaves(f, serverNames, ca, caKey); err != nil {
			return Files{}, err
		}
	}
	return f, nil
}

func createAll(f Files, serverNames []string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	serial, err := randomSerial()
	if err != nil {
		return err
	}
	ca := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "BaseHarbor etcd DCS CA"},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(caLifetime),
		IsCA:         true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		return err
	}
	if err := writeCertificate(f.CA, der, 0o644); err != nil {
		return err
	}
	if err := writeKey(f.CAKey, key); err != nil {
		return err
	}
	return issueLeaves(f, serverNames, ca, key)
}

func issueLeaves(f Files, serverNames []string, ca *x509.Certificate, caKey *ecdsa.PrivateKey) error {
	now := time.Now().UTC()
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serverSerial, err := randomSerial()
	if err != nil {
		return err
	}
	server := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject:      pkix.Name{CommonName: "baseharbor-etcd-member"},
		NotBefore:    now.Add(-5 * time.Minute), NotAfter: now.Add(leafLifetime),
		DNSNames:    append([]string(nil), serverNames...),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, server, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	clientSerial, err := randomSerial()
	if err != nil {
		return err
	}
	client := &x509.Certificate{
		SerialNumber: clientSerial,
		Subject:      pkix.Name{CommonName: "baseharbor-etcd-recovery-client"},
		NotBefore:    now.Add(-5 * time.Minute), NotAfter: now.Add(leafLifetime),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, client, ca, &clientKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	if err := writeCertificate(f.ServerCert, serverDER, 0o644); err != nil {
		return err
	}
	if err := writeKey(f.ServerKey, serverKey); err != nil {
		return err
	}
	if err := writeCertificate(f.ClientCert, clientDER, 0o644); err != nil {
		return err
	}
	return writeKey(f.ClientKey, clientKey)
}

func loadCA(f Files) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	cert, err := loadCertificate(f.CA)
	if err != nil {
		return nil, nil, err
	}
	if !cert.IsCA {
		return nil, nil, errors.New("etcd trust anchor is not a CA")
	}
	data, err := os.ReadFile(f.CAKey)
	if err != nil {
		return nil, nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, nil, errors.New("invalid etcd CA key PEM")
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, ok := keyAny.(*ecdsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("etcd CA key is not ECDSA")
	}
	return cert, key, nil
}

func loadCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid etcd certificate PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

func randomSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func writeCertificate(path string, der []byte, mode os.FileMode) error {
	return writeAtomic(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), mode)
}

func writeKey(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	return writeAtomic(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if len(data) == 0 {
		return errors.New("empty etcd PKI material")
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
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
