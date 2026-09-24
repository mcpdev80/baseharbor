package openbao

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type RuntimeMTLSFiles struct {
	CA           string
	BrokerCert   string
	BrokerKey    string
	ClientCert   string
	ClientKey    string
	WorkloadCert string
	WorkloadKey  string
}

// EnsureRuntimeMTLSIdentity preserves the application-scoped runtime identity
// contract while delegating all certificate issuance to the managed service
// issuer. BaseHarbor no longer stores or uses a CA private key in Go code.
func EnsureRuntimeMTLSIdentity(ctx context.Context, executor Executor, platformFiles bhruntime.Files, identity ApplicationIdentity, appFiles application.RuntimeFiles, workloadDNSNames []string) (RuntimeMTLSFiles, bool, error) {
	if err := validateApplicationIdentity(identity); err != nil {
		return RuntimeMTLSFiles{}, false, err
	}
	issuer := NewServiceIssuer(executor, platformFiles)
	trust, err := issuer.TrustBundle(ctx)
	if err != nil {
		return RuntimeMTLSFiles{}, false, fmt.Errorf("resolve runtime identity trust bundle: %w", err)
	}
	ca, err := parseTrustCertificate(trust.PEM)
	if err != nil {
		return RuntimeMTLSFiles{}, false, err
	}

	bindingDir := filepath.Join(appFiles.Bindings, "runtime-identity")
	if err := os.MkdirAll(bindingDir, 0o700); err != nil {
		return RuntimeMTLSFiles{}, false, fmt.Errorf("create runtime mTLS binding directory: %w", err)
	}
	if err := os.Chmod(bindingDir, 0o700); err != nil {
		return RuntimeMTLSFiles{}, false, fmt.Errorf("protect runtime mTLS binding directory: %w", err)
	}
	files := RuntimeMTLSFiles{
		CA:           filepath.Join(bindingDir, "ca.pem"),
		BrokerCert:   filepath.Join(bindingDir, "broker-cert.pem"),
		BrokerKey:    filepath.Join(bindingDir, "broker-key.pem"),
		ClientCert:   filepath.Join(bindingDir, "client-cert.pem"),
		ClientKey:    filepath.Join(bindingDir, "client-key.pem"),
		WorkloadCert: filepath.Join(bindingDir, "workload-cert.pem"),
		WorkloadKey:  filepath.Join(bindingDir, "workload-key.pem"),
	}
	valid, err := runtimeMTLSIdentityValid(files, ca, identity, workloadDNSNames)
	if err != nil {
		return RuntimeMTLSFiles{}, false, err
	}
	if valid {
		return files, false, nil
	}

	broker, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{
		CommonName:  "baseharbor-runtime",
		DNSNames:    []string{"baseharbor-runtime", "baseharbor-secrets", "localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		TTL:         30 * 24 * time.Hour,
	})
	if err != nil {
		return RuntimeMTLSFiles{}, false, fmt.Errorf("issue runtime broker identity: %w", err)
	}
	spiffeURI, err := url.Parse("spiffe://baseharbor/apps/" + identity.Name + "/" + identity.Environment)
	if err != nil {
		return RuntimeMTLSFiles{}, false, errors.New("construct runtime client identity URI")
	}
	client, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{
		CommonName: "baseharbor-" + identity.Name + "-" + identity.Environment + "-client",
		URIs:       []*url.URL{spiffeURI},
		TTL:        30 * 24 * time.Hour,
	})
	if err != nil {
		return RuntimeMTLSFiles{}, false, fmt.Errorf("issue runtime client identity: %w", err)
	}

	workloadNames := []string{"localhost", identity.Name, identity.Name + "." + identity.Environment + ".baseharbor"}
	seen := map[string]struct{}{}
	for _, name := range workloadNames {
		seen[name] = struct{}{}
	}
	for _, name := range workloadDNSNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		workloadNames = append(workloadNames, name)
	}
	workload, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{
		CommonName: identity.Name + "." + identity.Environment + ".baseharbor",
		DNSNames:   workloadNames,
		TTL:        30 * 24 * time.Hour,
	})
	if err != nil {
		return RuntimeMTLSFiles{}, false, fmt.Errorf("issue runtime workload identity: %w", err)
	}

	identityFiles := map[string][]byte{
		files.CA:           trust.PEM,
		files.BrokerCert:   broker.Certificate,
		files.BrokerKey:    broker.PrivateKey,
		files.ClientCert:   client.Certificate,
		files.ClientKey:    client.PrivateKey,
		files.WorkloadCert: workload.Certificate,
		files.WorkloadKey:  workload.PrivateKey,
	}
	for path, data := range identityFiles {
		if err := writeRuntimeIdentityFile(path, data); err != nil {
			return RuntimeMTLSFiles{}, false, err
		}
	}
	if valid, err := runtimeMTLSIdentityValid(files, ca, identity, workloadDNSNames); err != nil {
		return RuntimeMTLSFiles{}, false, err
	} else if !valid {
		return RuntimeMTLSFiles{}, false, errors.New("issued runtime mTLS identity failed verification")
	}
	return files, true, nil
}

func runtimeMTLSIdentityValid(files RuntimeMTLSFiles, ca *x509.Certificate, identity ApplicationIdentity, workloadDNSNames []string) (bool, error) {
	caPEM, err := os.ReadFile(files.CA)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read runtime mTLS CA: %w", err)
	}
	storedCA, err := parseTrustCertificate(caPEM)
	if err != nil || !bytes.Equal(storedCA.Raw, ca.Raw) {
		return false, nil
	}

	brokerOK, err := runtimeIdentityPairValid(files.BrokerCert, files.BrokerKey, ca, x509.ExtKeyUsageServerAuth, "baseharbor-runtime", "")
	if err != nil || !brokerOK {
		return false, err
	}
	loopbackOK, err := runtimeIdentityPairValid(files.BrokerCert, files.BrokerKey, ca, x509.ExtKeyUsageServerAuth, "127.0.0.1", "")
	if err != nil || !loopbackOK {
		return false, err
	}
	expectedURI := "spiffe://baseharbor/apps/" + identity.Name + "/" + identity.Environment
	clientOK, err := runtimeIdentityPairValid(files.ClientCert, files.ClientKey, ca, x509.ExtKeyUsageClientAuth, "", expectedURI)
	if err != nil || !clientOK {
		return false, err
	}
	workloadOK, err := runtimeIdentityPairValid(files.WorkloadCert, files.WorkloadKey, ca, x509.ExtKeyUsageServerAuth, "localhost", "")
	if err != nil || !workloadOK {
		return false, err
	}
	for _, dnsName := range workloadDNSNames {
		dnsName = strings.TrimSpace(dnsName)
		if dnsName == "" {
			continue
		}
		ok, err := runtimeIdentityPairValid(files.WorkloadCert, files.WorkloadKey, ca, x509.ExtKeyUsageServerAuth, dnsName, "")
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

func runtimeIdentityPairValid(certPath, keyPath string, ca *x509.Certificate, usage x509.ExtKeyUsage, dnsName, uri string) (bool, error) {
	certPEM, err := os.ReadFile(certPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read runtime identity certificate: %w", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read runtime identity key: %w", err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil || len(pair.Certificate) == 0 {
		return false, nil
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || time.Now().Add(24*time.Hour).After(cert.NotAfter) {
		return false, nil
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	opts := x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{usage}}
	if dnsName != "" {
		opts.DNSName = dnsName
	}
	if _, err := cert.Verify(opts); err != nil {
		return false, nil
	}
	if uri != "" {
		for _, candidate := range cert.URIs {
			if candidate.String() == uri {
				return true, nil
			}
		}
		return false, nil
	}
	return true, nil
}

func parseTrustCertificate(value []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(value)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("runtime identity trust bundle is invalid")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || !cert.IsCA || time.Now().After(cert.NotAfter) {
		return nil, errors.New("runtime identity trust certificate is invalid or expired")
	}
	return cert, nil
}

func writeRuntimeIdentityFile(path string, value []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create runtime identity directory: %w", err)
	}
	mode := os.FileMode(0o644)
	if strings.HasSuffix(path, "-key.pem") {
		mode = 0o600
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, value, 0o600); err != nil {
		return fmt.Errorf("write runtime identity file: %w", err)
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("prepare runtime identity file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install runtime identity file: %w", err)
	}
	return nil
}
