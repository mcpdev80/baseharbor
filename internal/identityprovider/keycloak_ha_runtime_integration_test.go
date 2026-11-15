package identityprovider

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
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
	// This isolated provider test owns its SQL HA dependency. Shared placement
	// requires an already running selected Core and is covered by Core journeys.
	t.Setenv(application.ProviderScopeEnv(capability.ProviderKeycloak), string(capability.ScopeApplication))

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
		if t.Failed() {
			diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 20*time.Second)
			if logs, logErr := runtimeProvider.LogsProject(diagnosticCtx, files.Project, files.Compose, files.Env, "keycloak-1", "keycloak-2", "keycloak-3", "keycloak-db-member-1", "keycloak-db-member-2", "keycloak-db-member-3"); logErr == nil {
				t.Logf("Keycloak HA project logs:\n%s", logs)
			} else {
				t.Logf("Keycloak HA project log capture failed: %v", logErr)
			}
			diagnosticCancel()
		}
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
	waitForKeycloakPostgresMemberReady(t, ctx, runtimeProvider, files, primary)
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
		logKeycloakDatabaseTLSState(t, ctx, runtimeProvider, files)
		t.Fatalf("rotate Keycloak PKI: %v", err)
	}
	waitForKeycloakContinuity(t, ctx, driver, resource, binding, "Keycloak PKI rotation")
	assertKeycloakOldCARejected(t, ctx, oldCA, files.PublicPort)
}

// Log enough public certificate metadata to distinguish a stale PostgreSQL
// leaf from a stale Keycloak JDBC trust projection when a live CA rotation
// fails. The certificate bytes and fingerprints are public material; no keys
// or connection credentials are emitted.
func logKeycloakDatabaseTLSState(t *testing.T, ctx context.Context, runtimeProvider bhruntime.RuntimeProvider, files KeycloakFiles) {
	t.Helper()
	rootPath := filepath.Join(files.Dir, "db-ha", "runtime", "ca.pem")
	leafPath := filepath.Join(files.Dir, "db-ha", "runtime", "server.pem")
	rootPEM, rootErr := os.ReadFile(rootPath)
	leafPEM, leafErr := os.ReadFile(leafPath)
	if rootErr != nil || leafErr != nil {
		t.Logf("Keycloak database TLS file diagnostic: trust_read_error=%v leaf_read_error=%v", rootErr, leafErr)
	} else {
		roots := x509.NewCertPool()
		for rest := rootPEM; len(rest) > 0; {
			block, next := pem.Decode(rest)
			if block == nil {
				break
			}
			rest = next
			if block.Type != "CERTIFICATE" {
				continue
			}
			cert, parseErr := x509.ParseCertificate(block.Bytes)
			if parseErr != nil {
				t.Logf("Keycloak database trust certificate parse error: %v", parseErr)
				continue
			}
			roots.AddCert(cert)
			t.Logf("Keycloak database projected trust anchor: subject=%q serial=%s sha256=%s", cert.Subject.String(), cert.SerialNumber.Text(16), certificateFingerprint(cert))
		}
		block, _ := pem.Decode(leafPEM)
		if block == nil {
			t.Log("Keycloak database projected server leaf is not valid PEM")
		} else if leaf, parseErr := x509.ParseCertificate(block.Bytes); parseErr != nil {
			t.Logf("Keycloak database projected server leaf parse error: %v", parseErr)
		} else {
			_, verifyErr := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "keycloak-db", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
			t.Logf("Keycloak database projected server leaf: subject=%q issuer=%q serial=%s sha256=%s verifies_against_projected_trust=%t verify_error=%v", leaf.Subject.String(), leaf.Issuer.String(), leaf.SerialNumber.Text(16), certificateFingerprint(leaf), verifyErr == nil, verifyErr)
		}
	}

	// Query the live PostgreSQL endpoint from a cluster member and print only
	// the TLS handshake/certificate chain. This lets the next hosted run show
	// whether PostgreSQL still presents a leaf different from the projection.
	command := `timeout 10 openssl s_client -starttls postgres -connect keycloak-db:5432 -showcerts </dev/null 2>&1 | sed -n '1,100p'`
	output, execErr := runtimeProvider.ExecProject(ctx, files.Project, files.Compose, files.Env, "keycloak-db-member-1", "sh", "-ec", command)
	if len(output) > 12000 {
		output = output[:12000] + "[truncated]"
	}
	t.Logf("Keycloak live PostgreSQL TLS probe (error=%v):\n%s", execErr, output)
}

func certificateFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
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

func waitForKeycloakPostgresMemberReady(t *testing.T, ctx context.Context, runtimeProvider bhruntime.RuntimeProvider, files KeycloakFiles, service string) {
	t.Helper()
	executor, ok := runtimeProvider.(keycloakProjectUserExecutor)
	if !ok {
		t.Fatal("Keycloak PostgreSQL readiness requires execution as the postgres user")
	}
	readyCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		attemptCtx, attemptCancel := context.WithTimeout(readyCtx, 5*time.Second)
		_, lastErr = executor.ExecProjectAs(attemptCtx, files.Project, files.Compose, files.Env, service, "postgres", "pg_isready", "-h", "/var/run/postgresql", "-U", "postgres", "-d", "postgres")
		attemptCancel()
		if lastErr == nil {
			return
		}
		select {
		case <-readyCtx.Done():
			t.Fatalf("restarted Keycloak PostgreSQL member %s did not become locally ready within 90s: %v", service, lastErr)
		case <-ticker.C:
		}
	}
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
