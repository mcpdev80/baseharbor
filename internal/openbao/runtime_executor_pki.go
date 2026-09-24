package openbao

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	RuntimeExecutorDNSName        = "baseharbor-runtime-executor"
	RuntimeExecutorObserverSPIFFE = "spiffe://baseharbor/platform/runtime-executor-observer"
)

type RuntimeExecutorMTLSFiles struct {
	CA         string
	Cert       string
	Key        string
	ClientCert string
	ClientKey  string
}

func EnsureRuntimeExecutorMTLSIdentity(ctx context.Context, issuer serviceaccess.Issuer, dir string) (RuntimeExecutorMTLSFiles, bool, error) {
	dir = filepath.Clean(dir)
	if dir == "." || dir == "" {
		return RuntimeExecutorMTLSFiles{}, false, errors.New("runtime executor identity directory is required")
	}
	if issuer == nil {
		return RuntimeExecutorMTLSFiles{}, false, errors.New("runtime executor identity issuer is required")
	}
	trust, err := issuer.TrustBundle(ctx)
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, fmt.Errorf("resolve runtime executor trust bundle: %w", err)
	}
	ca, err := parseTrustCertificate(trust.PEM)
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
		CA:         filepath.Join(dir, "ca.pem"),
		Cert:       filepath.Join(dir, "executor-cert.pem"),
		Key:        filepath.Join(dir, "executor-key.pem"),
		ClientCert: filepath.Join(dir, "observer-client-cert.pem"),
		ClientKey:  filepath.Join(dir, "observer-client-key.pem"),
	}
	valid, err := runtimeExecutorIdentityValid(files, ca)
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, err
	}
	if valid {
		return files, false, nil
	}

	cert, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{
		CommonName: RuntimeExecutorDNSName,
		DNSNames:   []string{RuntimeExecutorDNSName},
		TTL:        30 * 24 * time.Hour,
	})
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, fmt.Errorf("issue runtime executor identity: %w", err)
	}
	observerURI, err := url.Parse(RuntimeExecutorObserverSPIFFE)
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, errors.New("construct runtime executor observer identity URI")
	}
	observer, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{
		CommonName: "baseharbor-runtime-executor-observer",
		URIs:       []*url.URL{observerURI},
		TTL:        30 * 24 * time.Hour,
	})
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, fmt.Errorf("issue runtime executor observer identity: %w", err)
	}
	for path, data := range map[string][]byte{
		files.CA:         trust.PEM,
		files.Cert:       cert.Certificate,
		files.Key:        cert.PrivateKey,
		files.ClientCert: observer.Certificate,
		files.ClientKey:  observer.PrivateKey,
	} {
		if err := writeRuntimeIdentityFile(path, data); err != nil {
			return RuntimeExecutorMTLSFiles{}, false, err
		}
	}
	valid, err = runtimeExecutorIdentityValid(files, ca)
	if err != nil {
		return RuntimeExecutorMTLSFiles{}, false, err
	}
	if !valid {
		return RuntimeExecutorMTLSFiles{}, false, errors.New("issued runtime executor identity failed verification")
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
	serverOK, err := runtimeIdentityPairValid(files.Cert, files.Key, ca, x509.ExtKeyUsageServerAuth, RuntimeExecutorDNSName, "")
	if err != nil || !serverOK {
		return serverOK, err
	}
	return runtimeIdentityPairValid(files.ClientCert, files.ClientKey, ca, x509.ExtKeyUsageClientAuth, "", RuntimeExecutorObserverSPIFFE)
}
