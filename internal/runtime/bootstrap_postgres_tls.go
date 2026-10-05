package runtime

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func ensureBootstrapPostgresTLS(stateDir string) error {
	dir := filepath.Join(stateDir, "providers", "postgresql", "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	caPath := filepath.Join(dir, "ca.pem")
	certPath := filepath.Join(dir, "server-cert.pem")
	keyPath := filepath.Join(dir, "server-key.pem")
	if filesExist(caPath, certPath, keyPath) {
		return writeControlPlanePostgresHAProxyConfig(dir)
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "BaseHarbor bootstrap PostgreSQL CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "postgres"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(24 * time.Hour),
		DNSNames:     []string{"postgres", "postgres-member-1", "postgres-member-2", "postgres-member-3"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		return err
	}

	if err := writePEM(caPath, "CERTIFICATE", caDER, 0o644); err != nil {
		return err
	}
	if err := writePEM(certPath, "CERTIFICATE", serverDER, 0o644); err != nil {
		return err
	}
	if err := writePEM(keyPath, "PRIVATE KEY", keyDER, 0o600); err != nil {
		return err
	}
	const hba = `local all all trust
hostssl all all 0.0.0.0/0 scram-sha-256
hostssl all all ::/0 scram-sha-256
hostnossl all all 0.0.0.0/0 reject
hostnossl all all ::/0 reject
`
	if err := os.WriteFile(filepath.Join(dir, "pg_hba.conf"), []byte(hba), 0o644); err != nil {
		return err
	}
	return writeControlPlanePostgresHAProxyConfig(dir)
}

func writeControlPlanePostgresHAProxyConfig(dir string) error {
	const config = `global
  log stdout format raw local0

defaults
  mode tcp
  log global
  timeout connect 5s
  timeout client 30s
  timeout server 30s

frontend postgres
  bind :5432
  default_backend primary

backend primary
  option httpchk GET /primary
  http-check expect status 200
  default-server check port 8008 inter 2s fall 2 rise 2 on-marked-down shutdown-sessions resolvers runtime-dns resolve-prefer ipv4 init-addr last,libc,none
  server postgres-1 postgres-member-1:5432 check
  server postgres-2 postgres-member-2:5432 check
  server postgres-3 postgres-member-3:5432 check

resolvers runtime-dns
  parse-resolv-conf
  hold valid 2s
  hold obsolete 1s
  hold nx 1s
  timeout resolve 1s
  timeout retry 1s
`
	return os.WriteFile(filepath.Join(dir, "haproxy.cfg"), []byte(config), 0o644)
}

func filesExist(paths ...string) bool {
	for _, path := range paths {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) error {
	data := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	if len(data) == 0 {
		return fmt.Errorf("encode %s", blockType)
	}
	return os.WriteFile(path, data, mode)
}

func ensureBootstrapOpenBaoTLS(stateDir string) error {
	dir := filepath.Join(stateDir, "providers", "openbao", "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	caPath := filepath.Join(dir, "ca.pem")
	certPath := filepath.Join(dir, "server-cert.pem")
	keyPath := filepath.Join(dir, "server-key.pem")
	if filesExist(caPath, certPath, keyPath) {
		return nil
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "BaseHarbor bootstrap OpenBao CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "openbao"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(24 * time.Hour),
		DNSNames:     []string{"openbao", "openbao-member-1", "openbao-member-2", "openbao-member-3"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		return err
	}

	if err := writePEM(caPath, "CERTIFICATE", caDER, 0o644); err != nil {
		return err
	}
	if err := writePEM(certPath, "CERTIFICATE", serverDER, 0o644); err != nil {
		return err
	}
	return writePEM(keyPath, "PRIVATE KEY", keyDER, 0o644)
}
