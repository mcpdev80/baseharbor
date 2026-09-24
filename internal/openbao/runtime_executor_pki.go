package openbao

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
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
	issuer := NewServiceIssuer(executor, platformFiles)
	trust, err := issuer.TrustBundle(ctx)
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, fmt.Errorf("resolve runtime executor trust bundle: %w", err)
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
	valid, err := runtimeExecutorIdentityValid(files, trust.PEM)
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, err
	}
	if valid {
		return files, false, nil
	}

	cert, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{
		CommonName:  RuntimeExecutorDNSName,
		DNSNames:    []string{RuntimeExecutorDNSName},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		TTL:         30 * 24 * time.Hour,
	})
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, fmt.Errorf("issue runtime executor identity: %w", err)
	}
	for path, data := range map[string][]byte{
		files.CA:   trust.PEM,
		files.Cert: cert.Certificate,
		files.Key:  cert.PrivateKey,
	} {
		if err := writeRuntimeIdentityFile(path, data); err != nil {
			return RuntimeExecutorMTLSFiles{}, false, err
		}
	}
	return files, true, nil
}

func runtimeExecutorIdentityValid(files RuntimeExecutorMTLSFiles, trustPEM []byte) (bool, error) {
	caPEM, err := os.ReadFile(files.CA)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read runtime executor CA: %w", err)
	}
	if !bytes.Equal(bytes.TrimSpace(caPEM), bytes.TrimSpace(trustPEM)) {
		return false, nil
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(trustPEM) {
		return false, errors.New("runtime executor trust bundle contains no certificates")
	}
	return runtimeIdentityPairValid(files.Cert, files.Key, roots, x509.ExtKeyUsageServerAuth, RuntimeExecutorDNSName, "")
}
