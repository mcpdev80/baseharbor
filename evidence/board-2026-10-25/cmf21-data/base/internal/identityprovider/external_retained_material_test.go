package identityprovider

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

func externalCredentialFixture(t *testing.T) (application.Manifest, application.RuntimeFiles, *httptest.Server, string) {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token", "userinfo_endpoint": server.URL + "/userinfo", "jwks_uri": server.URL + "/jwks"})
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("client_id") != "retained-client" || r.Form.Get("client_secret") != "retained-private-value" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	m := application.New("external-credentials", "dev", false, false, false)
	m.Services.Identity = true
	files := application.RuntimeFilesFor(application.Store{Root: t.TempDir()}, m)
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{files.Env, files.ApplicationEnv} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ExternalClientIDEnv, "retained-client")
	t.Setenv(ExternalCAFileEnv, ca)
	secret := filepath.Join(t.TempDir(), "client-secret")
	if err := os.WriteFile(secret, []byte("retained-private-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ExternalClientSecretFileEnv, secret)
	return m, files, server, secret
}

func TestExternalReapplyRetainsConfidentialCredentialAndTrustAcrossAllProjections(t *testing.T) {
	m, files, server, _ := externalCredentialFixture(t)
	resource := capability.Resource{Kind: capability.Identity, Provider: capability.ProviderExternalOIDC}
	binding := capability.Binding{Identity: &capability.IdentityBinding{}}
	first, err := NewExternalDriver(m, files, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Bind(t.Context(), resource, binding); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ExternalClientSecretFileEnv, "")
	t.Setenv(ExternalCAFileEnv, "")
	for attempt := 0; attempt < 2; attempt++ {
		d, err := NewExternalDriver(m, files, server.URL)
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Preflight(t.Context(), resource, binding); err != nil {
			t.Fatal(err)
		}
		if err := d.Bind(t.Context(), resource, binding); err != nil {
			t.Fatal(err)
		}
		if err := d.Verify(t.Context(), resource, binding); err != nil {
			t.Fatal(err)
		}
		// Real local HTTPS consumer request with material read from the workload
		// projection; this is an adapter fixture, not native IDP/OAuth proof.
		data, err := os.ReadFile(filepath.Join(application.WorkloadServiceBindingProjectionDir(files), "identity", "client-secret"))
		if err != nil {
			t.Fatal(err)
		}
		response, err := d.client.PostForm(server.URL+"/token", url.Values{"client_id": {d.clientID}, "client_secret": {strings.TrimSpace(string(data))}})
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			t.Fatal("retained workload credential no longer authenticates")
		}
	}
}

func TestExternalAuthorityChangeCannotReuseOrSilentlyDropExistingMaterial(t *testing.T) {
	m, files, server, _ := externalCredentialFixture(t)
	d, err := NewExternalDriver(m, files, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Bind(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(files.Bindings, "identity", "client-secret")
	before, _ := os.ReadFile(path)
	t.Setenv(ExternalClientSecretFileEnv, "")
	t.Setenv(ExternalCAFileEnv, "")
	for _, change := range []string{"issuer", "client"} {
		issuer := server.URL
		if change == "issuer" {
			issuer += "/new"
		} else {
			t.Setenv(ExternalClientIDEnv, "different-client")
		}
		if _, err := NewExternalDriver(m, files, issuer); err == nil {
			t.Fatal("another authority inherited or lost confidential material")
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("read-only replan changed original credentials")
	}
}

func TestExternalSecretInputRejectsPublicLinkedAndOversizedFiles(t *testing.T) {
	for _, mode := range []string{"public", "symlink", "oversized", "parent-symlink"} {
		t.Run(mode, func(t *testing.T) {
			m, files, server, secret := externalCredentialFixture(t)
			switch mode {
			case "public":
				if err := os.Chmod(secret, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				link := filepath.Join(t.TempDir(), "linked")
				if err := os.Symlink(secret, link); err != nil {
					t.Fatal(err)
				}
				t.Setenv(ExternalClientSecretFileEnv, link)
			case "oversized":
				if err := os.WriteFile(secret, []byte(strings.Repeat("x", (1<<20)+1)), 0600); err != nil {
					t.Fatal(err)
				}
			case "parent-symlink":
				link := filepath.Join(t.TempDir(), "linked-parent")
				if err := os.Symlink(filepath.Dir(secret), link); err != nil {
					t.Fatal(err)
				}
				t.Setenv(ExternalClientSecretFileEnv, filepath.Join(link, filepath.Base(secret)))
			}
			if _, err := NewExternalDriver(m, files, server.URL); err == nil {
				t.Fatal("unsafe credential input accepted")
			}
			if _, err := os.Stat(filepath.Join(files.Bindings, "identity")); !os.IsNotExist(err) {
				t.Fatal("input preflight mutated identity binding")
			}
		})
	}
}

func TestExternalInterruptedBindingNeedsExplicitMaterialForFreshRetry(t *testing.T) {
	m, files, server, secret := externalCredentialFixture(t)
	d, err := NewExternalDriver(m, files, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Bind(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	ca := os.Getenv(ExternalCAFileEnv)
	path := filepath.Join(files.Bindings, "identity", "oidc.client-id")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	t.Setenv(ExternalClientSecretFileEnv, "")
	t.Setenv(ExternalCAFileEnv, "")
	if _, err := NewExternalDriver(m, files, server.URL); err == nil {
		t.Fatal("incomplete owner inferred credential continuity")
	}
	// Restore only from explicitly supplied protected material under the
	// existing adapter; no inference, regeneration or new owner is involved.
	t.Setenv(ExternalClientSecretFileEnv, secret)
	t.Setenv(ExternalCAFileEnv, ca)
	d, err = NewExternalDriver(m, files, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Bind(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	if err := d.Verify(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
}
