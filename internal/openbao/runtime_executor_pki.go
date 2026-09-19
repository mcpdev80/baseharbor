package openbao

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const RuntimeExecutorDNSName = "baseharbor-runtime-executor"

type RuntimeExecutorMTLSFiles struct {
	CA   string
	Cert string
	Key  string
}

func EnsureRuntimeExecutorMTLSIdentity(ctx context.Context, executor Executor, platformFiles bhruntime.Files, dir string) (RuntimeExecutorMTLSFiles, bool, error) {
	dir = filepath.Clean(dir)
	if dir == "." || dir == "" {
		return RuntimeExecutorMTLSFiles{}, false, errors.New("runtime executor identity directory is required")
	}
	credentials, err := LoadAdminCredentials(platformFiles)
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, err
	}
	managerToken, err := loginManager(ctx, executor, platformFiles, credentials)
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, fmt.Errorf("authenticate OpenBao manager for runtime executor PKI: %w", err)
	}
	caCert, caKey, err := ensureRuntimeCA(ctx, executor, platformFiles, managerToken)
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return RuntimeExecutorMTLSFiles{}, false, fmt.Errorf("create runtime executor identity directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return RuntimeExecutorMTLSFiles{}, false, fmt.Errorf("protect runtime executor identity directory: %w", err)
	}
	files := RuntimeExecutorMTLSFiles{
		CA:   filepath.Join(dir, "ca.pem"),
		Cert: filepath.Join(dir, "executor-cert.pem"),
		Key:  filepath.Join(dir, "executor-key.pem"),
	}
	valid, err := runtimeExecutorIdentityValid(files, caCert)
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, err
	}
	if valid {
		return files, false, nil
	}
	cert, key, err := issueRuntimeExecutorCertificate(caCert, caKey)
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, err
	}
	for path, data := range map[string][]byte{
		files.CA:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}),
		files.Cert: cert,
		files.Key:  key,
	} {
		if err := writeRuntimeIdentityFile(path, data); err != nil {
			return RuntimeExecutorMTLSFiles{}, false, err
		}
	}
	return files, true, nil
}

func runtimeExecutorIdentityValid(files RuntimeExecutorMTLSFiles, ca *x509.Certificate) (bool, error) {
	caPEM, err := os.ReadFile(files.CA)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read runtime executor CA: %w", err)
	}
	block, _ := pem.Decode(caPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return false, nil
	}
	storedCA, err := x509.ParseCertificate(block.Bytes)
	if err != nil || !bytes.Equal(storedCA.Raw, ca.Raw) {
		return false, nil
	}
	return runtimeIdentityPairValid(files.Cert, files.Key, ca, x509.ExtKeyUsageServerAuth, RuntimeExecutorDNSName, "")
}

func issueRuntimeExecutorCertificate(ca *x509.Certificate, caKey *ecdsa.PrivateKey) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate runtime executor identity key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: RuntimeExecutorDNSName, Organization: []string{"BaseHarbor"}},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(30 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{RuntimeExecutorDNSName},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("issue runtime executor certificate: %w", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("encode runtime executor key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), nil
}
