package identityprovider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/testsupport/runtimeprovider"
	"github.com/mcpdev80/baseharbor/internal/testsupport/serviceissuer"
)

func TestKeycloakHARuntimeFailoverAcceptanceInCI(t *testing.T) {
	if os.Getenv("BASEHARBOR_KEYCLOAK_HA_ACCEPTANCE") != "1" {
		t.Skip("set BASEHARBOR_KEYCLOAK_HA_ACCEPTANCE=1 to run the real Keycloak HA acceptance")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	runtimeProvider, err := runtimeprovider.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	issuer := serviceissuer.New(t)
	root := t.TempDir()
	namespace := "keycloak-ha-acceptance"
	app := application.WithHA(application.WithIdentity(application.New("keycloak-ha-acceptance", "test", false, false, false)), true)
	app.Services.IdentityManagementUI = true

	store := application.Store{Root: filepath.Join(root, "application"), Namespace: namespace}
	appFiles := application.RuntimeFilesFor(store, app)
	if err := os.MkdirAll(appFiles.Bindings, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(appFiles.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{appFiles.Env, appFiles.ApplicationEnv} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	driver := NewKeycloakDriver(runtimeProvider, app, appFiles, issuer, root, namespace)
	resource := capability.Resource{
		Application: app.Name,
		Kind:        capability.Identity,
		Name:        "default",
		Provider:    capability.ProviderKeycloak,
	}
	binding := capability.Binding{
		Resource: resource,
		Identity: &capability.IdentityBinding{Scopes: []string{"openid", "profile", "email"}},
	}
	if err := driver.Preflight(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	if err := driver.Provision(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	files, err := ExistingKeycloakFilesAt(app, root, namespace)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		_ = runtimeProvider.DestroyProject(cleanupCtx, files.Project, files.Compose, files.Env)
	}()

	if err := driver.Bind(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	if err := driver.Verify(ctx, resource, binding); err != nil {
		t.Fatal(err)
	}
	assertKeycloakStableSurfaces(t, ctx, driver)

	environment := mustKeycloakRuntimeEnv(t, files.Env)

	// Keycloak members are active/active behind the stable frontend. Loss of one
	// application member must not change issuer, discovery, JWKS or admin access.
	if err := runtimeProvider.StopProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{"keycloak-1"}, files.Compose); err != nil {
		t.Fatalf("stop Keycloak member: %v", err)
	}
	waitForKeycloakContinuity(t, ctx, driver, resource, binding, "Keycloak member failure")
	if err := runtimeProvider.UpProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{"keycloak-1"}, files.Compose); err != nil {
		t.Fatalf("restart Keycloak member: %v", err)
	}
	waitForKeycloakContinuity(t, ctx, driver, resource, binding, "Keycloak member recovery")

	// The identity cluster is only truthful HA if its PostgreSQL dependency can
	// also fail over. Stop the dynamically detected Patroni primary, not an
	// arbitrary replica.
	primary := mustKeycloakPostgresPrimary(t, ctx, runtimeProvider, files)
	if err := runtimeProvider.StopProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{primary}, files.Compose); err != nil {
		t.Fatalf("stop Keycloak PostgreSQL primary %s: %v", primary, err)
	}
	waitForKeycloakContinuity(t, ctx, driver, resource, binding, "Keycloak PostgreSQL primary failure")
	if err := runtimeProvider.UpProjectFilesSelected(ctx, files.Project, files.Dir, environment, []string{primary}, files.Compose); err != nil {
		t.Fatalf("restart Keycloak PostgreSQL member %s: %v", primary, err)
	}
	waitForKeycloakContinuity(t, ctx, driver, resource, binding, "Keycloak PostgreSQL member recovery")

	if err := driver.RotateClientSecret(ctx); err != nil {
		t.Fatalf("rotate Keycloak client secret: %v", err)
	}
	if err := driver.RotateSigningKey(ctx); err != nil {
		t.Fatalf("rotate Keycloak signing key: %v", err)
	}
	if err := driver.RotateAdminCredential(ctx); err != nil {
		t.Fatalf("rotate Keycloak admin credential: %v", err)
	}
	waitForKeycloakContinuity(t, ctx, driver, resource, binding, "Keycloak credential/signing/admin rotation")

	oldCA, err := os.ReadFile(files.PublicAccess.Material.CA)
	if err != nil {
		t.Fatalf("read Keycloak pre-rotation CA: %v", err)
	}
	issuer.Rotate(t)
	if err := driver.RotatePKI(ctx); err != nil {
		t.Fatalf("rotate Keycloak PKI: %v", err)
	}
	waitForKeycloakContinuity(t, ctx, driver, resource, binding, "Keycloak PKI rotation")
	assertKeycloakOldCARejected(t, ctx, oldCA, files.PublicPort)
}

func assertKeycloakOldCARejected(t *testing.T, ctx context.Context, oldCA []byte, port int) {
	t.Helper()
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(oldCA) {
		t.Fatal("pre-rotation Keycloak CA is invalid")
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    roots,
				ServerName: keycloakPublicHost,
			},
			TLSHandshakeTimeout: 5 * time.Second,
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			},
		},
		Timeout: 5 * time.Second,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+keycloakPublicHost+":"+strconv.Itoa(port)+"/realms/master/.well-known/openid-configuration", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("retired Keycloak CA still validates the stable identity endpoint")
	}
}

func mustKeycloakRuntimeEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			t.Fatalf("invalid Keycloak runtime env line %q", line)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}

func mustKeycloakPostgresPrimary(t *testing.T, ctx context.Context, runtimeProvider bhruntime.RuntimeProvider, files KeycloakFiles) string {
	t.Helper()
	const probe = "import urllib.request,sys;\ntry:\n r=urllib.request.urlopen('http://127.0.0.1:8008/primary', timeout=2); sys.exit(0 if r.status == 200 else 1)\nexcept Exception:\n sys.exit(1)"
	for ordinal := 1; ordinal <= 3; ordinal++ {
		service := fmt.Sprintf("keycloak-db-member-%d", ordinal)
		if _, err := runtimeProvider.ExecProject(ctx, files.Project, files.Compose, files.Env, service, "python3", "-c", probe); err == nil {
			return service
		}
	}
	t.Fatal("no Keycloak Patroni primary found")
	return ""
}

func waitForKeycloakContinuity(t *testing.T, ctx context.Context, driver *KeycloakDriver, resource capability.Resource, binding capability.Binding, phase string) {
	t.Helper()
	deadline := time.Now().Add(75 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if err := driver.Verify(ctx, resource, binding); err == nil {
			if err = assertKeycloakStableSurfacesError(ctx, driver); err == nil {
				return
			}
			last = err
		} else {
			last = err
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("%s did not preserve stable identity/admin surfaces: %v", phase, last)
}

func assertKeycloakStableSurfaces(t *testing.T, ctx context.Context, driver *KeycloakDriver) {
	t.Helper()
	if err := assertKeycloakStableSurfacesError(ctx, driver); err != nil {
		t.Fatal(err)
	}
}

func assertKeycloakStableSurfacesError(ctx context.Context, driver *KeycloakDriver) error {
	if driver.instance.PublicHTTPClient == nil || driver.instance.AdminHTTPClient == nil {
		return fmt.Errorf("Keycloak stable HTTP clients are unavailable")
	}
	issuer := strings.TrimRight(driver.instance.EndpointBaseURL, "/") + "/realms/" + driver.realm
	discovery, err := FetchDiscoveryAt(ctx, driver.instance.PublicHTTPClient, issuer, issuer)
	if err != nil {
		return fmt.Errorf("stable discovery: %w", err)
	}
	if discovery.Issuer != issuer {
		return fmt.Errorf("issuer drift: got %q want %q", discovery.Issuer, issuer)
	}
	jwks, err := fetchKeycloakJWKS(ctx, driver.instance.PublicHTTPClient, driver.instance.EndpointBaseURL, driver.realm)
	if err != nil {
		return fmt.Errorf("stable JWKS: %w", err)
	}
	if len(jwks.Keys) == 0 {
		return fmt.Errorf("stable JWKS contains no keys")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(driver.instance.EndpointBaseURL, "/")+"/admin/master/console/", nil)
	if err != nil {
		return err
	}
	resp, err := driver.instance.AdminHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("stable Keycloak Admin GUI: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("stable Keycloak Admin GUI returned HTTP %d", resp.StatusCode)
	}
	if _, err := driver.adminClient(ctx); err != nil {
		return fmt.Errorf("stable authenticated Keycloak Admin API: %w", err)
	}
	return nil
}
