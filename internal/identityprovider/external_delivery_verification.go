package identityprovider

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// Self-consistent projections do not prove they are the prepared delivery.
func (d *ExternalDriver) verifyPreparedDelivery() error {
	dir := filepath.Join(d.files.Bindings, application.IdentityBindingName)
	expected := map[string][]byte{"provider": []byte(capability.ProviderExternalOIDC), "oidc.issuer": []byte(d.issuer), "oidc.client-id": []byte(d.clientID), "client-secret": []byte(d.secret), "ca.crt": d.material.trust}
	for name, want := range expected {
		data, err := readExternalProtectedMaterial(filepath.Join(dir, name))
		if os.IsNotExist(err) && len(want) == 0 {
			continue
		}
		if err != nil {
			return errors.New("prepared external identity delivery is missing or unprotected")
		}
		match := bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(want))
		zeroExternalMaterial(data)
		if !match {
			return errors.New("external identity delivery differs from prepared client, issuer or trust material")
		}
	}
	return nil
}
