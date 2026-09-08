package openbao

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	runtimeCACertPath = "apps/_baseharbor/runtime-pki-ca-cert"
	runtimeCAKeyPath  = "apps/_baseharbor/runtime-pki-ca-key"
)

type RuntimeMTLSFiles struct {
	CA         string
	BrokerCert string
	BrokerKey  string
	ClientCert string
	ClientKey  string
}

// EnsureRuntimeMTLSIdentity issues a per-application broker/server identity and
// client identity from a BaseHarbor-internal CA. The CA private key is stored in
// the manager-only OpenBao namespace and is never persisted in application or
// control-plane filesystem state.
func EnsureRuntimeMTLSIdentity(ctx context.Context, executor Executor, platformFiles bhruntime.Files, identity ApplicationIdentity, appFiles application.RuntimeFiles) (RuntimeMTLSFiles, error) {
	if err := validateApplicationIdentity(identity); err != nil {
		return RuntimeMTLSFiles{}, err
	}
	credentials, err := LoadAdminCredentials(platformFiles)
	if err != nil {
		return RuntimeMTLSFiles{}, err
	}
	managerToken, err := loginManager(ctx, executor, platformFiles, credentials)
	if err != nil {
		return RuntimeMTLSFiles{}, fmt.Errorf("authenticate OpenBao manager for runtime PKI: %w", err)
	}
	caCert, caKey, err := ensureRuntimeCA(ctx, executor, platformFiles, managerToken)
	if err != nil {
		return RuntimeMTLSFiles{}, err
	}

	bindingDir := filepath.Join(appFiles.Bindings, "runtime-identity")
	if err := os.MkdirAll(bindingDir, 0o700); err != nil {
		return RuntimeMTLSFiles{}, fmt.Errorf("create runtime mTLS binding directory: %w", err)
	}
	files := RuntimeMTLSFiles{
		CA:         filepath.Join(bindingDir, "ca.pem"),
		BrokerCert: filepath.Join(bindingDir, "broker-cert.pem"),
		BrokerKey:  filepath.Join(bindingDir, "broker-key.pem"),
		ClientCert: filepath.Join(bindingDir, "client-cert.pem"),
		ClientKey:  filepath.Join(bindingDir, "client-key.pem"),
	}
	brokerCert, brokerKey, err := issueRuntimeCertificate(caCert, caKey, identity, true)
	if err != nil {
		return RuntimeMTLSFiles{}, err
	}
	clientCert, clientKey, err := issueRuntimeCertificate(caCert, caKey, identity, false)
	if err != nil {
		return RuntimeMTLSFiles{}, err
	}
	for path, data := range map[string][]byte{
		files.CA:         pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}),
		files.BrokerCert: brokerCert,
		files.BrokerKey:  brokerKey,
		files.ClientCert: clientCert,
		files.ClientKey:  clientKey,
	} {
		if err := writeRuntimeIdentityFile(path, data); err != nil {
			return RuntimeMTLSFiles{}, err
		}
	}
	return files, nil
}

func ensureRuntimeCA(ctx context.Context, executor Executor, files bhruntime.Files, token string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPEM, certOK, err := readManagerKV(ctx, executor, files, token, runtimeCACertPath)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, keyOK, err := readManagerKV(ctx, executor, files, token, runtimeCAKeyPath)
	if err != nil {
		return nil, nil, err
	}
	if certOK != keyOK {
		return nil, nil, errors.New("BaseHarbor runtime CA state is incomplete; refusing implicit replacement")
	}
	if !certOK {
		certPEM, keyPEM, err = generateRuntimeCA()
		if err != nil {
			return nil, nil, err
		}
		if err := writeManagerKV(ctx, executor, files, token, runtimeCACertPath, certPEM); err != nil {
			return nil, nil, err
		}
		if err := writeManagerKV(ctx, executor, files, token, runtimeCAKeyPath, keyPEM); err != nil {
			return nil, nil, err
		}
	}
	return parseRuntimeCA(certPEM, keyPEM)
}

func generateRuntimeCA() ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate runtime CA key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "BaseHarbor Runtime CA", Organization: []string{"BaseHarbor"}},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create runtime CA certificate: %w", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("encode runtime CA key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), nil
}

func parseRuntimeCA(certPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certBlock, _ := pem.Decode(certPEM)
	keyBlock, _ := pem.Decode(keyPEM)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" || keyBlock == nil || keyBlock.Type != "PRIVATE KEY" {
		return nil, nil, errors.New("BaseHarbor runtime CA state is invalid")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil || !cert.IsCA || time.Now().After(cert.NotAfter) {
		return nil, nil, errors.New("BaseHarbor runtime CA certificate is invalid or expired")
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, errors.New("BaseHarbor runtime CA private key is invalid")
	}
	key, ok := parsedKey.(*ecdsa.PrivateKey)
	if !ok || !key.PublicKey.Equal(cert.PublicKey) {
		return nil, nil, errors.New("BaseHarbor runtime CA certificate and private key do not match")
	}
	return cert, key, nil
}

func issueRuntimeCertificate(ca *x509.Certificate, caKey *ecdsa.PrivateKey, identity ApplicationIdentity, server bool) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate runtime identity key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	commonName := "baseharbor-" + identity.Name + "-" + identity.Environment + "-client"
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName, Organization: []string{"BaseHarbor"}},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(30 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if server {
		template.Subject.CommonName = "baseharbor-secrets"
		template.DNSNames = []string{"baseharbor-secrets"}
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	} else {
		uri, err := url.Parse("spiffe://baseharbor/apps/" + identity.Name + "/" + identity.Environment)
		if err != nil {
			return nil, nil, errors.New("construct runtime client identity URI")
		}
		template.URIs = []*url.URL{uri}
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("issue runtime identity certificate: %w", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("encode runtime identity key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	return serial, nil
}

func readManagerKV(ctx context.Context, executor Executor, files bhruntime.Files, token, path string) ([]byte, bool, error) {
	command := fmt.Sprintf(`if value="$(bao kv get -field=value -mount=baseharbor %s 2>/dev/null)"; then printf 'FOUND\n%%s' "$value"; else printf 'MISSING\n'; fi`, path)
	out, err := execWithToken(ctx, executor, files, token, command)
	if err != nil {
		return nil, false, errors.New("read BaseHarbor runtime CA from OpenBao failed")
	}
	status, value, _ := strings.Cut(out, "\n")
	switch strings.TrimSpace(status) {
	case "MISSING":
		return nil, false, nil
	case "FOUND":
		return []byte(value), true, nil
	default:
		return nil, false, errors.New("read BaseHarbor runtime CA from OpenBao returned an invalid response")
	}
}

func writeManagerKV(ctx context.Context, executor Executor, files bhruntime.Files, token, path string, value []byte) error {
	command := fmt.Sprintf(`exec bao kv put -mount=baseharbor %s value=-`, path)
	if _, err := execWithTokenInput(ctx, executor, files, token, command, value); err != nil {
		return errors.New("store BaseHarbor runtime CA in OpenBao failed")
	}
	return nil
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
