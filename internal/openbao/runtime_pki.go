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

// EnsureRuntimeMTLSIdentity issues all runtime identities through the managed
// OpenBao PKI issuer. BaseHarbor never creates or stores a CA private key in
// CLI/application state.
func EnsureRuntimeMTLSIdentity(ctx context.Context, executor Executor, platformFiles bhruntime.Files, identity ApplicationIdentity, appFiles application.RuntimeFiles, workloadDNSNames []string) (RuntimeMTLSFiles, bool, error) {
	if err := validateApplicationIdentity(identity); err != nil {
		return RuntimeMTLSFiles{}, false, err
	}
	issuer := NewServiceIssuer(executor, platformFiles)
	trust, err := issuer.TrustBundle(ctx)
	if err != nil {
		return RuntimeMTLSFiles{}, false, fmt.Errorf("resolve OpenBao runtime trust bundle: %w", err)
	}

	bindingDir := filepath.Join(appFiles.Bindings, "runtime-identity")
	if err := os.MkdirAll(bindingDir, 0o700); err != nil {
		return RuntimeMTLSFiles{}, false, fmt.Errorf("create runtime mTLS binding directory: %w", err)
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
	valid, err := runtimeMTLSIdentityValid(files, trust.PEM, identity, workloadDNSNames)
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

	identityURI, err := url.Parse("spiffe://baseharbor/apps/" + identity.Name + "/" + identity.Environment)
	if err != nil {
		return RuntimeMTLSFiles{}, false, errors.New("construct runtime client identity URI")
	}
	client, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{
		CommonName: "baseharbor-" + identity.Name + "-" + identity.Environment + "-client",
		URIs:       []*url.URL{identityURI},
		TTL:        30 * 24 * time.Hour,
	})
	if err != nil {
		return RuntimeMTLSFiles{}, false, fmt.Errorf("issue runtime client identity: %w", err)
	}

	workloadCommonName := identity.Name + "." + identity.Environment + ".baseharbor"
	workloadNames := uniqueRuntimeDNSNames(append([]string{"localhost", identity.Name, workloadCommonName}, workloadDNSNames...))
	workload, err := issuer.Issue(ctx, serviceaccess.CertificateRequest{
		CommonName: workloadCommonName,
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
	return files, true, nil
}

func runtimeMTLSIdentityValid(files RuntimeMTLSFiles, trustPEM []byte, identity ApplicationIdentity, workloadDNSNames []string) (bool, error) {
	caPEM, err := os.ReadFile(files.CA)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read runtime mTLS CA: %w", err)
	}
	if !bytes.Equal(bytes.TrimSpace(caPEM), bytes.TrimSpace(trustPEM)) {
		return false, nil
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(trustPEM) {
		return false, errors.New("OpenBao runtime trust bundle contains no certificates")
	}

	brokerOK, err := runtimeIdentityPairValid(files.BrokerCert, files.BrokerKey, roots, x509.ExtKeyUsageServerAuth, "baseharbor-runtime", "")
	if err != nil || !brokerOK {
		return false, err
	}
	loopbackOK, err := runtimeIdentityPairValid(files.BrokerCert, files.BrokerKey, roots, x509.ExtKeyUsageServerAuth, "127.0.0.1", "")
	if err != nil || !loopbackOK {
		return false, err
	}
	expectedURI := "spiffe://baseharbor/apps/" + identity.Name + "/" + identity.Environment
	clientOK, err := runtimeIdentityPairValid(files.ClientCert, files.ClientKey, roots, x509.ExtKeyUsageClientAuth, "", expectedURI)
	if err != nil || !clientOK {
		return false, err
	}
	workloadOK, err := runtimeIdentityPairValid(files.WorkloadCert, files.WorkloadKey, roots, x509.ExtKeyUsageServerAuth, "localhost", "")
	if err != nil || !workloadOK {
		return false, err
	}
	for _, dnsName := range workloadDNSNames {
		dnsName = strings.TrimSpace(dnsName)
		if dnsName == "" {
			continue
		}
		ok, err := runtimeIdentityPairValid(files.WorkloadCert, files.WorkloadKey, roots, x509.ExtKeyUsageServerAuth, dnsName, "")
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

func runtimeIdentityPairValid(certPath, keyPath string, roots *x509.CertPool, usage x509.ExtKeyUsage, dnsName, uri string) (bool, error) {
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
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return false, nil
	}
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return false, nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || time.Now().Add(24*time.Hour).After(cert.NotAfter) {
		return false, nil
	}
	opts := x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{usage}}
	if dnsName != "" {
		opts.DNSName = dnsName
	}
	if _, err := cert.Verify(opts); err != nil {
		return false, nil
	}
	if uri != "" {
		found := false
		for _, candidate := range cert.URIs {
			if candidate.String() == uri {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

func uniqueRuntimeDNSNames(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func writeRuntimeIdentityFile(path string, value []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create runtime identity directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, value, 0o600); err != nil {
		return fmt.Errorf("write runtime identity file: %w", err)
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("prepare runtime identity file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install runtime identity file: %w", err)
	}
	return nil
}
