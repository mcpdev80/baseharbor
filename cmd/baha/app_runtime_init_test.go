package main

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestParseRepositoryInitOptions(t *testing.T) {
	opts, err := parseRepositoryInitOptions([]string{"--hostname", "mail.example.test", "--tls=existing", "--cert-dir", "/tmp/certs", "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Hostname != "mail.example.test" || opts.TLSMode != "existing" || opts.CertDir != "/tmp/certs" || !opts.Yes {
		t.Fatalf("unexpected options: %#v", opts)
	}
	if _, err := parseRepositoryInitOptions([]string{"--tls=openbao-pki"}); err == nil {
		t.Fatal("expected unsupported TLS mode to fail")
	}
}

func TestRepositoryInitStateRoundTrip(t *testing.T) {
	root := t.TempDir()
	state := repositoryInitState{
		Hostname:        "mail.example.test",
		TLSMode:         "existing",
		CertDir:         "/operator/certs",
		TLSDir:          filepath.Join(root, ".baseharbor", "tls"),
		RuntimeProvider: bhruntime.ProviderCompose,
	}
	if err := writeRepositoryInitState(root, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadRepositoryInitState(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != state {
		t.Fatalf("state mismatch: got %#v want %#v", loaded, state)
	}
	info, err := os.Stat(repositoryInitEnvPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("init env mode = %o, want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(repositoryInitEnvPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "BASEHARBOR_RUNTIME_PROVIDER=compose\n") {
		t.Fatalf("runtime provider missing from init state: %s", data)
	}
}

func TestRuntimeHostnameRoundTripPreservesLiteralDots(t *testing.T) {
	root := t.TempDir()
	const hostname = "mailflow.mcp-dev.de"
	if err := validateRuntimeHostname(hostname); err != nil {
		t.Fatal(err)
	}
	state := repositoryInitState{
		Hostname:        hostname,
		TLSMode:         "existing",
		CertDir:         "/operator/certs",
		TLSDir:          filepath.Join(root, ".baseharbor", "tls"),
		RuntimeProvider: bhruntime.ProviderCompose,
	}
	if err := writeRepositoryInitState(root, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadRepositoryInitState(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Hostname != hostname {
		t.Fatalf("hostname round-trip = %q, want %q", loaded.Hostname, hostname)
	}
	data, err := os.ReadFile(repositoryInitEnvPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\\.") {
		t.Fatalf("persisted hostname unexpectedly escaped dots: %q", data)
	}
	if !strings.Contains(string(data), "BASEHARBOR_HOSTNAME="+hostname+"\n") {
		t.Fatalf("persisted hostname missing exact literal value: %q", data)
	}
}

func TestRuntimeHostnameRejectsBackslashEscapedDots(t *testing.T) {
	if err := validateRuntimeHostname(`mailflow\.mcp-dev.de`); err == nil {
		t.Fatal("backslash-escaped hostname was accepted")
	}
}

func TestRepositoryInitStateDefaultsLegacyRuntimeProviderToCompose(t *testing.T) {
	root := t.TempDir()
	path := repositoryInitEnvPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("BASEHARBOR_HOSTNAME=mail.example.test\nBASEHARBOR_TLS_MODE=local\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := loadRepositoryInitState(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.RuntimeProvider != bhruntime.ProviderCompose {
		t.Fatalf("runtime provider = %q, want %q", state.RuntimeProvider, bhruntime.ProviderCompose)
	}
}

func TestDetectCertificatePairPrefersFullchain(t *testing.T) {
	dir := t.TempDir()
	certPEM, keyPEM := testCertificatePair(t, "mail.example.test")
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fullchain.pem"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "privkey.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	pair, err := detectCertificatePair(dir, "mail.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(pair.CertPath) != "fullchain.pem" {
		t.Fatalf("certificate = %s, want fullchain.pem", pair.CertPath)
	}
	if filepath.Base(pair.KeyPath) != "privkey.pem" {
		t.Fatalf("key = %s, want privkey.pem", pair.KeyPath)
	}
}

func TestDetectCertificatePairRejectsWrongHostname(t *testing.T) {
	dir := t.TempDir()
	certPEM, keyPEM := testCertificatePair(t, "other.example.test")
	if err := os.WriteFile(filepath.Join(dir, "fullchain.pem"), certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "privkey.pem"), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := detectCertificatePair(dir, "mail.example.test"); err == nil {
		t.Fatal("expected hostname mismatch to fail")
	}
}

func TestWriteNormalizedTLSFiles(t *testing.T) {
	dir := t.TempDir()
	certPEM, keyPEM := testCertificatePair(t, "mail.example.test")
	pair := detectedCertificatePair{CertPEM: certPEM, KeyData: keyPEM}
	if err := writeNormalizedTLSFiles(dir, pair); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cert.pem", "key.pem"} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o, want 600", name, info.Mode().Perm())
		}
	}
}

func TestPromptTLSMode(t *testing.T) {
	for input, want := range map[string]string{
		"1\n": "acme",
		"2\n": "existing",
		"3\n": "local",
	} {
		got, err := promptTLSMode(runtimeInitReader(input), &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("input %q => %q, want %q", input, got, want)
		}
	}
}

func runtimeInitReader(input string) *bufio.Reader {
	return bufio.NewReader(bytes.NewBufferString(input))
}

func testCertificatePair(t *testing.T, hostname string) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: hostname},
		DNSNames:     []string{hostname},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}
