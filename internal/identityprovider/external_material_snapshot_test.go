package identityprovider

import (
	"bytes"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

type externalMaterialMutationTransport struct {
	base   http.RoundTripper
	mutate func() error
}

func (r externalMaterialMutationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := r.base.RoundTrip(request)
	if err != nil {
		return response, err
	}
	if err := r.mutate(); err != nil {
		response.Body.Close()
		return nil, err
	}
	return response, nil
}

func TestExternalDiscoveryCannotChangeTrustBeforeDelivery(t *testing.T) {
	m, files, server, _ := externalCredentialFixture(t)
	d, err := NewExternalDriver(m, files, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ca := os.Getenv(ExternalCAFileEnv)
	d.client.Transport = externalMaterialMutationTransport{base: d.client.Transport, mutate: func() error {
		return os.WriteFile(ca, []byte("changed during actual HTTPS discovery\n"), 0600)
	}}
	if err := d.Bind(t.Context(), capability.Resource{}, capability.Binding{}); err == nil {
		t.Fatal("discovery silently substituted the workload trust")
	}
	if _, err := os.Stat(filepath.Join(files.Bindings, application.IdentityBindingName)); !os.IsNotExist(err) {
		t.Fatal("changed trust reached binding mutation")
	}
}

func TestExternalPreparedMaterialChangesPreserveAllExistingDelivery(t *testing.T) {
	for _, change := range []string{"secret-content", "trust-content", "secret-owner", "trust-link", "missing-secret"} {
		t.Run(change, func(t *testing.T) {
			m, files, server, secret := externalCredentialFixture(t)
			first, err := NewExternalDriver(m, files, server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if err := first.Bind(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
				t.Fatal(err)
			}
			prepared, err := NewExternalDriver(m, files, server.URL)
			if err != nil {
				t.Fatal(err)
			}
			resource := capability.Resource{Kind: capability.Identity, Provider: capability.ProviderExternalOIDC}
			binding := capability.Binding{Identity: &capability.IdentityBinding{}}
			if err := prepared.Preflight(t.Context(), resource, binding); err != nil {
				t.Fatal(err)
			}
			paths := []string{files.Env, files.ApplicationEnv}
			for _, root := range []string{files.Bindings, application.WorkloadServiceBindingProjectionDir(files)} {
				for _, name := range []string{"client-secret", "ca.crt", "oidc.issuer", "oidc.client-id"} {
					paths = append(paths, filepath.Join(root, application.IdentityBindingName, name))
				}
			}
			before := map[string][]byte{}
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = data
			}
			ca := os.Getenv(ExternalCAFileEnv)
			switch change {
			case "secret-content":
				err = os.WriteFile(secret, []byte("replacement-private-material\n"), 0600)
			case "trust-content":
				err = os.WriteFile(ca, []byte("changed trust material\n"), 0600)
			case "secret-owner":
				data, readErr := os.ReadFile(secret)
				if readErr != nil {
					t.Fatal(readErr)
				}
				replacement := filepath.Join(t.TempDir(), "replacement")
				if err = os.WriteFile(replacement, data, 0600); err == nil {
					err = os.Rename(replacement, secret)
				}
			case "trust-link":
				original := ca + ".original"
				if err = os.Rename(ca, original); err == nil {
					err = os.Symlink(original, ca)
				}
			case "missing-secret":
				err = os.Remove(secret)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := prepared.Preflight(t.Context(), resource, binding); err == nil {
				t.Fatal("changed preparation remained valid")
			}
			if err := prepared.Bind(t.Context(), resource, binding); err == nil {
				t.Fatal("changed input silently altered identity delivery")
			}
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, before[path]) {
					t.Fatal("failed material revalidation changed an existing delivery projection")
				}
			}
		})
	}
}

func TestExternalPreparedPublicTrustUsesTheSameBytesForTLSAndDelivery(t *testing.T) {
	m, files, server, _ := externalCredentialFixture(t)
	ca := os.Getenv(ExternalCAFileEnv)
	if err := os.Chmod(ca, 0644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(ca)
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewExternalDriver(m, files, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Bind(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	if err := d.Verify(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{files.Bindings, application.WorkloadServiceBindingProjectionDir(files)} {
		data, err := os.ReadFile(filepath.Join(root, application.IdentityBindingName, "ca.crt"))
		if err != nil || !bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(before)) {
			t.Fatal("delivered trust differs from the actual TLS discovery trust")
		}
	}
	credential, err := os.ReadFile(filepath.Join(application.WorkloadServiceBindingProjectionDir(files), application.IdentityBindingName, "client-secret"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := d.client.PostForm(server.URL+"/token", url.Values{"client_id": {d.clientID}, "client_secret": {strings.TrimSpace(string(credential))}})
	if err != nil {
		t.Fatal("actual HTTPS consumer request failed")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatal("delivered credential did not authenticate at the selected HTTPS fixture")
	}
}
