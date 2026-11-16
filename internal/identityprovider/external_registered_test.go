package identityprovider

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
)

func registeredExternalFixture(t *testing.T) (application.Manifest, application.RuntimeFiles, string, string, externalprovider.Registration) {
	t.Helper()
	m, files, server, secret := externalCredentialFixture(t)
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	t.Setenv("BASEHARBOR_IDENTITY_PROVIDER", "external-oidc")
	t.Setenv("BASEHARBOR_EXTERNAL_OIDC_ISSUER", "")
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOIDC), "registered-login")
	reg := externalprovider.Registration{ID: "registered-login", ProviderID: "company/oidc", Endpoint: server.URL, Provider: capability.ExternalOIDC, CredentialRef: "file://" + secret, Trust: externalprovider.Trust{Mode: externalprovider.TrustCustomCA, CAReference: os.Getenv(ExternalCAFileEnv)}, Metadata: map[string]string{"client-id": "retained-client"}}
	if err := application.RegisterExternalProviderAt(root, reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{ExternalClientIDEnv, ExternalClientSecretFileEnv, ExternalCAFileEnv} {
		t.Setenv(name, "")
	}
	target, err := deployment.TargetStateRoot("oidc-test")
	if err != nil {
		t.Fatal(err)
	}
	return m, files, root, target, reg
}

func TestRegisteredExternalOIDCUsesActualIssuerAndCanonicalMaterialForHTTPSConsumer(t *testing.T) {
	m, files, root, target, reg := registeredExternalFixture(t)
	before := map[string][]byte{}
	for _, name := range []string{"provider-registry.json", "external-providers.json"} {
		before[name], _ = os.ReadFile(filepath.Join(root, name))
	}
	d, err := NewRegisteredExternalDriver(m, files, target)
	if err != nil {
		t.Fatal(err)
	}
	r := capability.Resource{Kind: capability.Identity, Provider: capability.ProviderExternalOIDC}
	b := capability.Binding{Identity: &capability.IdentityBinding{}}
	if err := d.Preflight(t.Context(), r, b); err != nil {
		t.Fatal("registered reference was treated as an issuer instead of resolving its HTTPS endpoint")
	}
	if err := d.Bind(t.Context(), r, b); err != nil {
		t.Fatal(err)
	}
	if err := d.Verify(t.Context(), r, b); err != nil {
		t.Fatal(err)
	}
	consumerDir := filepath.Join(application.WorkloadServiceBindingProjectionDir(files), application.IdentityBindingName)
	secret, _ := os.ReadFile(filepath.Join(consumerDir, "client-secret"))
	clientID, _ := os.ReadFile(filepath.Join(consumerDir, "oidc.client-id"))
	issuer, _ := os.ReadFile(filepath.Join(consumerDir, "oidc.issuer"))
	if strings.TrimSpace(string(issuer)) != reg.Endpoint {
		t.Fatal("consumer issuer differs from registered endpoint")
	}
	client, err := externalHTTPClientForCA(filepath.Join(consumerDir, "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.PostForm(reg.Endpoint+"/token", url.Values{"client_id": {strings.TrimSpace(string(clientID))}, "client_secret": {strings.TrimSpace(string(secret))}})
	if err != nil {
		t.Fatal("actual delivered consumer trust did not authenticate its HTTPS receiver")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatal("registered credential did not authenticate through delivered workload material")
	}
	for name, data := range before {
		after, _ := os.ReadFile(filepath.Join(root, name))
		if !bytes.Equal(data, after) {
			t.Fatal("external adapter changed canonical registration owner")
		}
	}
}

func TestRegisteredExternalOIDCRegistrationDriftDeniesBeforeDelivery(t *testing.T) {
	m, files, root, target, _ := registeredExternalFixture(t)
	d, err := NewRegisteredExternalDriver(m, files, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Preflight(t.Context(), capability.Resource{Kind: capability.Identity, Provider: capability.ProviderExternalOIDC}, capability.Binding{Identity: &capability.IdentityBinding{}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "external-providers.json")
	data, _ := os.ReadFile(path)
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	// The public registrar correctly refuses an in-place configuration change.
	// Model changed owner bytes independently to exercise prepared revalidation.
	document["registrations"].([]any)[0].(map[string]any)["endpoint"] = "https://changed-issuer.example.test"
	data, _ = json.Marshal(document)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := d.Bind(t.Context(), capability.Resource{}, capability.Binding{}); err == nil {
		t.Fatal("changed canonical issuer retained old delivery authority")
	}
	if _, err := os.Stat(filepath.Join(files.Bindings, application.IdentityBindingName)); !os.IsNotExist(err) {
		t.Fatal("registration drift reached first material delivery")
	}
}

func TestRegisteredExternalOIDCRejectsUnrelatedAmbientMaterial(t *testing.T) {
	m, files, _, target, _ := registeredExternalFixture(t)
	unrelated := filepath.Join(t.TempDir(), "unrelated-credential")
	if err := os.WriteFile(unrelated, []byte("unrelated-private-value"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ExternalClientSecretFileEnv, unrelated)
	if _, err := NewRegisteredExternalDriver(m, files, target); err == nil || strings.Contains(err.Error(), "unrelated-private-value") {
		t.Fatal("unrelated credential silently replaced registered material or was disclosed")
	}
}

func TestRegisteredExternalOIDCNewPublicSystemTargetDoesNotInheritMaterial(t *testing.T) {
	m, files, root, target, source := registeredExternalFixture(t)
	d, err := NewRegisteredExternalDriver(m, files, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Bind(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	if err := application.ReconcileReferenceProviderRegistryAt(target, m); err != nil {
		t.Fatal(err)
	}
	retained, err := NewRegisteredExternalDriver(m, files, target)
	if err != nil || retained.secret != d.secret || !bytes.Equal(retained.material.trust, d.material.trust) {
		t.Fatal("same registered source lost its delivered credential or trust")
	}
	destination := source
	destination.ID = "public-system-login"
	destination.CredentialRef = ""
	destination.Trust = externalprovider.Trust{Mode: externalprovider.TrustSystem}
	if err := application.RegisterExternalProviderAt(root, destination); err != nil {
		t.Fatal(err)
	}
	t.Setenv(application.ProviderExternalReferenceEnv(capability.ProviderExternalOIDC), destination.ID)
	next, err := NewRegisteredExternalDriver(m, files, target)
	if err != nil {
		t.Fatal(err)
	}
	if next.secret != "" || len(next.material.trust) != 0 {
		t.Fatal("new public system-trust registration inherited previous confidential material")
	}
	previousSecret, err := os.ReadFile(filepath.Join(files.Bindings, application.IdentityBindingName, "client-secret"))
	if err != nil || strings.TrimSpace(string(previousSecret)) != d.secret {
		t.Fatal("read-only destination preparation changed source material")
	}
}
