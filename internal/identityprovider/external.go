package identityprovider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

const (
	ExternalClientIDEnv         = "BASEHARBOR_EXTERNAL_OIDC_CLIENT_ID"
	ExternalClientSecretFileEnv = "BASEHARBOR_EXTERNAL_OIDC_CLIENT_SECRET_FILE"
	ExternalCAFileEnv           = "BASEHARBOR_EXTERNAL_OIDC_CA_FILE"
)

type ExternalDriver struct {
	app          application.Manifest
	files        application.RuntimeFiles
	issuer       string
	clientID     string
	secret       string
	client       *http.Client
	material     *externalMaterialSnapshot
	registration *registeredExternalSource
	discovery    *application.IdentityDiscovery
}

func NewExternalDriver(app application.Manifest, files application.RuntimeFiles, issuer string) (*ExternalDriver, error) {
	return newExternalDriver(app, files, issuer, externalDriverInputs{
		clientID:   strings.TrimSpace(os.Getenv(ExternalClientIDEnv)),
		secretFile: strings.TrimSpace(os.Getenv(ExternalClientSecretFileEnv)),
		caFile:     strings.TrimSpace(os.Getenv(ExternalCAFileEnv)),
	})
}

type externalDriverInputs struct {
	clientID, secretFile, caFile          string
	skipRetainedSecret, skipRetainedTrust bool
}

func newExternalDriver(app application.Manifest, files application.RuntimeFiles, issuer string, inputs externalDriverInputs) (*ExternalDriver, error) {
	clientID := inputs.clientID
	if clientID == "" {
		return nil, fmt.Errorf("%s is required for external OIDC", ExternalClientIDEnv)
	}
	secret, retainedCA, err := retainedExternalMaterial(files, issuer, clientID, inputs)
	if err != nil {
		return nil, err
	}
	if path := inputs.secretFile; path != "" {
		data, err := readExternalProtectedMaterial(path)
		if err != nil {
			return nil, fmt.Errorf("read external OIDC client secret: %w", err)
		}
		secret = strings.TrimSpace(string(data))
		for i := range data {
			data[i] = 0
		}
		if secret == "" || strings.ContainsAny(secret, "\r\n") {
			return nil, fmt.Errorf("external OIDC client secret is empty or invalid")
		}
	}
	caPath := inputs.caFile
	if caPath == "" {
		caPath = retainedCA
	}
	material, err := captureExternalMaterial(files, inputs.secretFile, caPath, secret)
	if err != nil {
		return nil, err
	}
	client, err := externalHTTPClientWithTrust(material.trust)
	if err != nil {
		return nil, err
	}
	return &ExternalDriver{
		app: app, files: files, issuer: strings.TrimRight(strings.TrimSpace(issuer), "/"),
		clientID: clientID, secret: secret, client: client, material: material,
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
	if err := d.revalidateExternalSources(); err != nil {
		return err
	}
	discovery, err := FetchDiscovery(ctx, d.client, d.issuer)
	if err != nil {
		return err
	}
	return d.retainDiscovery(discovery)
}

func (d *ExternalDriver) Provision(context.Context, capability.Resource, capability.Binding) error {
	return nil
}

func (d *ExternalDriver) Bind(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	if err := d.revalidateExternalSources(); err != nil {
		return err
	}
	discovery, err := FetchDiscovery(ctx, d.client, d.issuer)
	if err != nil {
		return err
	}
	if err := d.revalidateExternalSources(); err != nil {
		return err
	}
	if err := d.retainDiscovery(discovery); err != nil {
		return err
	}
	if err := application.MaterializeIdentityBindingMaterial(d.app, d.files, string(capability.ProviderExternalOIDC), discovery, d.clientID, d.secret, d.material.trust); err != nil {
		return err
	}
	return d.material.refreshRetainedDeliveryOwners(d.files, d.secret)
}

func (d *ExternalDriver) Verify(ctx context.Context, _ capability.Resource, _ capability.Binding) error {
	if err := d.revalidateExternalSources(); err != nil {
		return err
	}
	discovery, err := FetchDiscovery(ctx, d.client, d.issuer)
	if err != nil {
		return err
	}
	if err := d.revalidateExternalSources(); err != nil {
		return err
	}
	if d.discovery != nil && *d.discovery != discovery {
		return fmt.Errorf("external identity discovery changed after preparation; replan before verification")
	}
	if err := d.verifyPreparedDelivery(discovery); err != nil {
		return err
	}
	return application.VerifyIdentityBinding(d.app, d.files)
}

func externalHTTPClient() (*http.Client, error) {
	return externalHTTPClientForCA(strings.TrimSpace(os.Getenv(ExternalCAFileEnv)))
}

func externalHTTPClientForCA(caPath string) (*http.Client, error) {
	var trust []byte
	if caPath != "" {
		var err error
		trust, err = readExternalMaterial(caPath, false)
		if err != nil {
			return nil, fmt.Errorf("read external OIDC trust: %w", err)
		}
	}
	return externalHTTPClientWithTrust(trust)
}

func externalHTTPClientWithTrust(trust []byte) (*http.Client, error) {
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	if len(trust) > 0 {
		if err := externalprovider.ValidatePublicTrustMaterial(trust); err != nil {
			return nil, err
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(trust) {
			return nil, fmt.Errorf("external OIDC CA contains no certificates")
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}, nil
}
