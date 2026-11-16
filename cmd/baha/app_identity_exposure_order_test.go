package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/exposure"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type stagedIdentityDriver struct {
	provisions, bindings, verifications int
	verifyErr                           error
}

func (*stagedIdentityDriver) Descriptor() capability.Provider { return capability.Keycloak }
func (*stagedIdentityDriver) Preflight(context.Context, capability.Resource, capability.Binding) error {
	return nil
}
func (d *stagedIdentityDriver) Provision(context.Context, capability.Resource, capability.Binding) error {
	d.provisions++
	return nil
}
func (d *stagedIdentityDriver) Bind(context.Context, capability.Resource, capability.Binding) error {
	d.bindings++
	return nil
}
func (d *stagedIdentityDriver) Verify(context.Context, capability.Resource, capability.Binding) error {
	d.verifications++
	return d.verifyErr
}

func stagedIdentityFixture(t *testing.T) (*managedIdentityExecution, *stagedIdentityDriver, *managedExposureExecution) {
	t.Helper()
	target := configureTestTarget(t)
	m := application.WithIdentityCallbackPath(application.WithWorkloadComponents(application.New("staged", "prod", false, false, false), "api"), "/oauth/callback")
	m.Exposures = []application.HTTPExposureRequirement{{Name: "public-api", Service: "api", Port: 8080, Protocol: "https", Visibility: "public"}}
	files := application.RuntimeFilesFor(application.Store{Root: t.TempDir(), Namespace: target.Name}, m)
	driver := &stagedIdentityDriver{}
	execution, _, err := capability.Prepare(t.Context(), m.Name, []capability.Request{{Requirement: capability.Requirement{Kind: capability.Identity, Name: "default"}, Workload: "application/" + m.Name, Identity: &capability.IdentityBinding{CallbackPaths: m.Identity.CallbackPaths}, Driver: driver}})
	if err != nil {
		t.Fatal(err)
	}
	keycloak := identityprovider.NewKeycloakDriverWithRealization(nil, m, files)
	prepared := &managedIdentityExecution{execution: execution, keycloak: keycloak, provider: capability.ProviderKeycloak, target: target.Name, manifest: m, runtime: identityOriginRuntime{}, files: files}
	state := exposure.PublishedState{Host: "example.com", Routes: []exposure.Route{{Name: "public-api", Service: "api", TargetPort: 8080, Protocol: "https", ProviderProtocol: "https", Visibility: "public", PublishedPort: 28443}}}
	exposed := &managedExposureExecution{driver: exposure.NewLifecycle(state.Host, "existing", identityOriginExposure{state: state})}
	return prepared, driver, exposed
}

func TestManagedIdentityProductionReadinessWaitsForActualPublicExposure(t *testing.T) {
	prepared, driver, exposed := stagedIdentityFixture(t)
	var out bytes.Buffer
	if err := provisionManagedIdentityBeforeExposure(t.Context(), &out, prepared, exposed); err != nil {
		t.Fatal(err)
	}
	if driver.provisions != 1 || driver.bindings != 1 || driver.verifications != 0 || strings.Contains(out.String(), "[READY]") {
		t.Fatal("unrealized public callback was reported ready")
	}
	if err := verifyManagedIdentityAfterExposure(t.Context(), &out, prepared, exposed); err == nil || driver.provisions != 1 {
		t.Fatal("unrealized exposure mutated final identity")
	}
	if err := exposed.driver.Provision(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
		t.Fatal(err)
	}
	if err := verifyManagedIdentityAfterExposure(t.Context(), &out, prepared, exposed); err != nil {
		t.Fatal(err)
	}
	if driver.provisions != 2 || driver.bindings != 2 || driver.verifications != 1 || !strings.Contains(out.String(), "[READY] identity") {
		t.Fatal("actual published callback did not reach common identity reconciliation/verification")
	}
}

func TestManagedIdentityIncompleteOrFailedCallbackNeverReportsReady(t *testing.T) {
	for _, mode := range []string{"foreign-route", "port", "host", "http-hop", "identity-verification"} {
		t.Run(mode, func(t *testing.T) {
			prepared, driver, _ := stagedIdentityFixture(t)
			var out bytes.Buffer
			if err := provisionManagedIdentityBeforeExposure(t.Context(), &out, prepared, nil); err != nil {
				t.Fatal(err)
			}
			state := exposure.PublishedState{Host: "example.com", Routes: []exposure.Route{{Name: "public-api", Service: "api", TargetPort: 8080, Protocol: "https", ProviderProtocol: "https", Visibility: "public", PublishedPort: 29443}}}
			switch mode {
			case "foreign-route":
				state.Routes[0].Name = "foreign"
			case "port":
				state.Routes[0].PublishedPort = 0
			case "host":
				state.Host = ""
			case "http-hop":
				state.Routes[0].ProviderProtocol = "http"
			case "identity-verification":
				driver.verifyErr = fmt.Errorf("actual identity verification failed")
			}
			exposed := &managedExposureExecution{driver: exposure.NewLifecycle(state.Host, "existing", identityOriginExposure{state: state})}
			if err := exposed.driver.Provision(t.Context(), capability.Resource{}, capability.Binding{}); err != nil {
				t.Fatal(err)
			}
			if err := verifyManagedIdentityAfterExposure(t.Context(), &out, prepared, exposed); err == nil || strings.Contains(out.String(), "[READY]") {
				t.Fatal("missing, insecure or failed callback was reported ready")
			}
		})
	}
}

type stagedExposureRuntime struct{ bhruntime.RuntimeProvider }

func (stagedExposureRuntime) ConfigProject(context.Context, string, string, string) error { return nil }
func (stagedExposureRuntime) UpProject(context.Context, string, string, string) error     { return nil }

func TestManagedIdentityReapplyUsesProtectedExistingOriginsWithoutRewritingOwner(t *testing.T) {
	prepared, _, _ := stagedIdentityFixture(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	certificate := server.TLS.Certificates[0]
	key, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	tlsDir := t.TempDir()
	for name, data := range map[string][]byte{"cert.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), "key.pem": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})} {
		if err := os.WriteFile(filepath.Join(tlsDir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	caddy := exposure.NewDriver(stagedExposureRuntime{}, prepared.manifest, prepared.files, exposure.Deployment{Hostname: "example.com", TLSMode: "existing", TLSDir: tlsDir})
	resource := capability.Resource{Kind: capability.ExposureHTTP, Name: "public-api"}
	binding := capability.Binding{Workload: "service/api", HTTPExposure: &capability.HTTPExposureBinding{Service: "api", TargetPort: 8080, Protocol: "https", Visibility: "public"}}
	if err := caddy.Preflight(t.Context(), resource, binding); err != nil {
		t.Fatal(err)
	}
	if err := caddy.Provision(t.Context(), resource, binding); err != nil {
		t.Fatal(err)
	}
	state, owned, err := exposure.Load(prepared.files)
	if err != nil {
		t.Fatal(err)
	}
	before := map[string][]byte{}
	for _, path := range []string{owned.State, owned.Compose, owned.Env} {
		before[path], err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	origins, err := existingManagedIdentityExposureOrigins(prepared)
	want := fmt.Sprintf("https://example.com:%d", state.Routes[0].PublishedPort)
	if err != nil || len(origins) != 1 || origins[0] != want {
		t.Fatal("reapply guessed future origin instead of reading protected existing renderer owner", err)
	}
	for path, expected := range before {
		actual, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatal("origin inspection rewrote existing exposure owner")
		}
	}
	if err := os.Remove(owned.Compose); err != nil {
		t.Fatal(err)
	}
	if _, err := existingManagedIdentityExposureOrigins(prepared); err == nil {
		t.Fatal("damaged existing exposure was treated as fresh empty callback authority")
	}
}

func TestManagedIdentityDirectConvergenceCannotApproveUnrealizedProductionCallback(t *testing.T) {
	prepared, driver, exposed := stagedIdentityFixture(t)
	var out bytes.Buffer
	if err := provisionAndVerifyManagedIdentity(t.Context(), &out, prepared, exposed); err == nil || driver.provisions != 0 || strings.Contains(out.String(), "[READY]") {
		t.Fatal("direct convergence bypassed actual public exposure readiness")
	}
}
