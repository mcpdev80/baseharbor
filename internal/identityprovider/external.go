package identityprovider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

const (
	ExternalClientIDEnv         = "BASEHARBOR_EXTERNAL_OIDC_CLIENT_ID"
	ExternalClientSecretFileEnv = "BASEHARBOR_EXTERNAL_OIDC_CLIENT_SECRET_FILE"
	ExternalCAFileEnv           = "BASEHARBOR_EXTERNAL_OIDC_CA_FILE"
)

type ExternalDriver struct {
	app      application.Manifest
	files    application.RuntimeFiles
	issuer   string
	clientID string
	secret   string
	client   *http.Client
}

func NewExternalDriver(app application.Manifest, files application.RuntimeFiles, issuer string) (*ExternalDriver, error) {
	clientID := strings.TrimSpace(os.Getenv(ExternalClientIDEnv))
	if clientID == "" {
		return nil, fmt.Errorf("%s is required for external OIDC", ExternalClientIDEnv)
	}
	secret := ""
	if path := strings.TrimSpace(os.Getenv(ExternalClientSecretFileEnv)); path != "" {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect external OIDC client secret: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 {
			return nil, fmt.Errorf("external OIDC client secret must be a protected regular non-symlink file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read external OIDC client secret: %w", err)
		}
		secret = strings.TrimSpace(string(data))
		if secret == "" || strings.ContainsAny(secret, "\r\n") {
			return nil, fmt.Errorf("external OIDC client secret is empty or invalid")
		}
	}
	client, err := externalHTTPClient()
	if err != nil {
		return nil, err
	}
	return &ExternalDriver{
		app: app, files: files, issuer: strings.TrimRight(strings.TrimSpace(issuer), "/"),
		clientID: clientID, secret: secret, client: client,
	}, nil
}

func (d *ExternalDriver) Descriptor() capability.Provider { return capability.ExternalOIDC }

func (d *ExternalDriver) Preflight(ctx context.Context, resource capability.Resource, binding capability.Binding) error {
	if resource.Kind != capability.Identity || resource.Provider != capability.ProviderExternalOIDC {
		return fmt.Errorf("external OIDC provider cannot satisfy %s via %s", resource.Kind, resource.Provider)
	}
	if binding.Identity == nil {
		return fmt.Errorf("identity binding is required")
	}
	_, err := FetchDiscovery(ctx, d.client, d.issuer)
	return err
}

func (d *ExternalDriver) Provision(context.Context, capability.Resource, capability.Binding) error {
	return nil
}

func (d *ExternalDriver) Bind(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	discovery, err := FetchDiscovery(ctx, d.client, d.issuer)
	if err != nil {
		return err
	}
	return application.MaterializeIdentityBinding(d.app, d.files, string(capability.ProviderExternalOIDC), discovery, d.clientID, d.secret, strings.TrimSpace(os.Getenv(ExternalCAFileEnv)))
}

func (d *ExternalDriver) Verify(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	if _, err := FetchDiscovery(ctx, d.client, d.issuer); err != nil {
		return err
	}
	return application.VerifyIdentityBinding(d.app, d.files)
}

func externalHTTPClient() (*http.Client, error) {
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	if path := strings.TrimSpace(os.Getenv(ExternalCAFileEnv)); path != "" {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect external OIDC CA: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("external OIDC CA must be a regular non-symlink file")
		}
		pem, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return nil, fmt.Errorf("read external OIDC CA: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("external OIDC CA contains no certificates")
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}, nil
}
