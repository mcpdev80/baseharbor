package identityprovider

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

type registeredExternalSource struct {
	dataDir      string
	app          application.Manifest
	registration externalprovider.Registration
}

func NewRegisteredExternalDriver(app application.Manifest, files application.RuntimeFiles, dataDir string) (*ExternalDriver, error) {
	reg, err := application.ResolveExternalProviderRegistrationAt(dataDir, app, capability.ProviderExternalOIDC)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(reg.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("registered external OIDC issuer must be an absolute credential-free HTTPS endpoint")
	}
	inputs := externalDriverInputs{clientID: strings.TrimSpace(os.Getenv(ExternalClientIDEnv)), secretFile: strings.TrimSpace(os.Getenv(ExternalClientSecretFileEnv)), caFile: strings.TrimSpace(os.Getenv(ExternalCAFileEnv))}
	if client := strings.TrimSpace(reg.Metadata["client-id"]); client != "" {
		if inputs.clientID != "" && inputs.clientID != client {
			return nil, errors.New("external OIDC client selection conflicts with registered client identity")
		}
		inputs.clientID = client
	}
	if reg.CredentialRef != "" {
		path, err := externalLocalMaterialFile(reg.CredentialRef)
		if err != nil {
			return nil, err
		}
		if inputs.secretFile != "" && filepath.Clean(inputs.secretFile) != path {
			return nil, errors.New("external OIDC credential input conflicts with registered material reference")
		}
		inputs.secretFile = path
	}
	if reg.Trust.Mode == externalprovider.TrustMTLS || reg.Trust.Directory != "" || reg.Trust.ClientCertificate != "" || reg.Trust.ClientKey != "" {
		return nil, errors.New("registered OIDC mTLS or certificate-directory delivery requires a compatible consumer adapter")
	}
	if reg.Trust.Mode == externalprovider.TrustSystem && inputs.caFile != "" {
		return nil, errors.New("external OIDC trust input conflicts with registered system trust")
	}
	if reg.Trust.CAReference != "" {
		path, err := externalLocalMaterialFile(reg.Trust.CAReference)
		if err != nil {
			return nil, err
		}
		if inputs.caFile != "" && filepath.Clean(inputs.caFile) != path {
			return nil, errors.New("external OIDC trust input conflicts with registered material reference")
		}
		inputs.caFile = path
	}
	sameReference, err := registeredExternalReferenceWasBound(app, dataDir, reg.ID)
	if err != nil {
		return nil, err
	}
	inputs.skipRetainedSecret = !sameReference
	inputs.skipRetainedTrust = !sameReference || reg.Trust.Mode == externalprovider.TrustSystem
	d, err := newExternalDriver(app, files, reg.Endpoint, inputs)
	if err != nil {
		return nil, err
	}
	d.registration = &registeredExternalSource{dataDir: dataDir, app: app, registration: reg}
	if err := d.revalidateExternalSources(); err != nil {
		return nil, err
	}
	return d, nil
}

func externalLocalMaterialFile(reference string) (string, error) {
	u, err := url.Parse(reference)
	if err != nil || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.Host != "" || u.Scheme != "" && u.Scheme != "file" || !filepath.IsAbs(u.Path) || strings.ContainsAny(u.Path, "\r\n\x00") {
		return "", errors.New("external OIDC material reference requires a supported absolute local file; other reference adapters are not implemented")
	}
	return filepath.Clean(u.Path), nil
}

func (s *registeredExternalSource) revalidate() error {
	if s == nil {
		return nil
	}
	current, err := application.ResolveExternalProviderRegistrationAt(s.dataDir, s.app, capability.ProviderExternalOIDC)
	if err != nil || !reflect.DeepEqual(current, s.registration) {
		return errors.New("external OIDC registration changed or became unverifiable; replan before delivery")
	}
	return nil
}

func (d *ExternalDriver) revalidateExternalSources() error {
	if err := d.material.revalidate(); err != nil {
		return err
	}
	return d.registration.revalidate()
}

// Only the applied consumer binding authorizes reuse of omitted material.
// Equal issuer/client strings do not establish equal registered ownership.
func registeredExternalReferenceWasBound(app application.Manifest, dataDir, reference string) (bool, error) {
	registry, err := (capability.RegistryStore{Path: filepath.Join(dataDir, "provider-registry.json")}).Load()
	if err != nil {
		return false, err
	}
	for _, binding := range registry.Bindings {
		if binding.ApplicationID != app.ApplicationID || binding.Environment != app.Environment || binding.Resource.Provider != capability.ProviderExternalOIDC {
			continue
		}
		for _, instance := range registry.Instances {
			if instance.ID == binding.ProviderInstanceID && instance.Reference == reference && instance.Ownership == capability.OwnershipExternal {
				return true, nil
			}
		}
	}
	return false, nil
}
