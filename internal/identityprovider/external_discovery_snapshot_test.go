package identityprovider

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func externalDiscoverySnapshotFixture(t *testing.T) (*ExternalDriver, *atomic.Bool) {
	t.Helper()
	m, files, _, _ := externalCredentialFixture(t)
	changed := &atomic.Bool{}
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		token := "https://tokens.other.example.test/token"
		if changed.Load() {
			token += "-changed"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": server.URL, "authorization_endpoint": "https://login.other.example.test/authorize",
			"token_endpoint": token, "userinfo_endpoint": server.URL + "/userinfo", "jwks_uri": "https://keys.other.example.test/jwks",
		})
	}))
	t.Cleanup(server.Close)
	if err := os.WriteFile(os.Getenv(ExternalCAFileEnv), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	driver, err := NewExternalDriver(m, files, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return driver, changed
}

func TestExternalDiscoverySnapshotRejectsChangesBeforeAndAfterDelivery(t *testing.T) {
	for _, phase := range []string{"before-bind", "after-bind", "consistent-new-delivery"} {
		t.Run(phase, func(t *testing.T) {
			d, changed := externalDiscoverySnapshotFixture(t)
			resource := capability.Resource{Kind: capability.Identity, Provider: capability.ProviderExternalOIDC}
			binding := capability.Binding{Identity: &capability.IdentityBinding{}}
			if err := d.Preflight(t.Context(), resource, binding); err != nil {
				t.Fatal(err)
			}
			if phase == "before-bind" {
				changed.Store(true)
				if err := d.Bind(t.Context(), resource, binding); err == nil {
					t.Fatal("changed verified discovery reached delivery")
				}
				if _, err := os.Stat(filepath.Join(d.files.Bindings, application.IdentityBindingName)); !os.IsNotExist(err) {
					t.Fatal("discovery drift created a binding owner")
				}
				return
			}
			if err := d.Bind(t.Context(), resource, binding); err != nil {
				t.Fatal(err)
			}
			if err := d.Verify(t.Context(), resource, binding); err != nil {
				t.Fatal("valid distinct-host discovery endpoints were rejected", err)
			}
			before, err := os.ReadFile(d.files.ApplicationEnv)
			if err != nil {
				t.Fatal(err)
			}
			changed.Store(true)
			if phase == "consistent-new-delivery" {
				current, err := FetchDiscovery(t.Context(), d.client, d.issuer)
				if err != nil {
					t.Fatal(err)
				}
				if err := application.MaterializeIdentityBindingMaterial(d.app, d.files, string(capability.ProviderExternalOIDC), current, d.clientID, d.secret, d.material.trust); err != nil {
					t.Fatal(err)
				}
			}
			if err := d.Verify(t.Context(), resource, binding); err == nil {
				t.Fatal("old prepared request accepted changed discovery")
			}
			after, err := os.ReadFile(d.files.ApplicationEnv)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("verification changed credential delivery")
			}
		})
	}
}
