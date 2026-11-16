package identityprovider

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// The revision is an opaque consent input. No confidential material or
// standalone material fingerprint is projected into provider consequences.
type ExternalIdentityBindingChange struct {
	Changed           bool
	IssuerBefore      string
	IssuerAfter       string
	ClientChanged     bool
	CredentialChanged bool
	TrustChanged      bool
	InputsVerified    bool
	Revision          string `json:"-"`
}

func InspectRegisteredExternalBindingChange(app application.Manifest, files application.RuntimeFiles, dataDir string) (ExternalIdentityBindingChange, error) {
	reg, err := application.ResolveExternalProviderRegistrationAt(dataDir, app, capability.ProviderExternalOIDC)
	if err != nil {
		return ExternalIdentityBindingChange{}, err
	}
	change := ExternalIdentityBindingChange{IssuerAfter: reg.Endpoint}
	dir := filepath.Join(files.Bindings, application.IdentityBindingName)
	values := map[string][]byte{}
	complete := true
	for _, name := range []string{"provider", "oidc.issuer", "oidc.client-id", "client-secret", "ca.crt"} {
		data, err := readExternalProtectedMaterial(filepath.Join(dir, name))
		if err != nil {
			if name == "provider" || name == "oidc.issuer" || name == "oidc.client-id" || !os.IsNotExist(err) {
				complete = false
			}
			continue
		}
		values[name] = data
	}
	defer func() {
		for _, data := range values {
			zeroExternalMaterial(data)
		}
	}()
	change.IssuerBefore = strings.TrimSpace(string(values["oidc.issuer"]))
	complete = complete && strings.TrimSpace(string(values["provider"])) == string(capability.ProviderExternalOIDC)
	if application.VerifyIdentityBinding(app, files) != nil {
		complete = false
	}
	d, prepareErr := NewRegisteredExternalDriver(app, files, dataDir)
	change.InputsVerified = complete && prepareErr == nil
	change.Changed = !change.InputsVerified
	if prepareErr == nil {
		change.ClientChanged = strings.TrimSpace(string(values["oidc.client-id"])) != d.clientID
		change.CredentialChanged = strings.TrimSpace(string(values["client-secret"])) != d.secret
		change.TrustChanged = !bytes.Equal(bytes.TrimSpace(values["ca.crt"]), bytes.TrimSpace(d.material.trust))
		change.Changed = change.Changed || strings.TrimRight(change.IssuerBefore, "/") != strings.TrimRight(d.issuer, "/") || change.ClientChanged || change.CredentialChanged || change.TrustChanged
	}
	raw, err := json.Marshal(reg)
	if err != nil {
		return change, err
	}
	for _, name := range []string{"provider", "oidc.issuer", "oidc.client-id", "client-secret", "ca.crt"} {
		raw = append(raw, 0)
		raw = append(raw, values[name]...)
	}
	for _, name := range []string{ExternalClientIDEnv, ExternalClientSecretFileEnv, ExternalCAFileEnv} {
		raw = append(raw, 0)
		raw = append(raw, os.Getenv(name)...)
	}
	if prepareErr == nil {
		raw = append(raw, 0)
		raw = append(raw, d.secret...)
		raw = append(raw, 0)
		raw = append(raw, d.material.trust...)
	}
	digest := sha256.Sum256(raw)
	zeroExternalMaterial(raw)
	change.Revision = hex.EncodeToString(digest[:])
	return change, nil
}
